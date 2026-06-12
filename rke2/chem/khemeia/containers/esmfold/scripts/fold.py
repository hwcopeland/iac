#!/usr/bin/env python3
"""ESMFold structure-prediction worker for the Khemeia genomics layer.

Double duty (workers TDD §4.0), distinguished by the calc job's `calculation`
field plus a `role` hint the controller injects:

  ROLE=resolve_fallback (calculation=esmfold, AlphaFold DB missed):
    Fold the WILD-TYPE sequence only -> structure.pdb. Upload to
    khemeia-structures at the resolution's resolve key, compute global mean
    pLDDT, and UPDATE variant_resolutions (structure_source='esmfold',
    structure_bucket/_key, plddt_global). Emits NO staging row — a resolution is
    not a calc result.

  ROLE=calc (calculation=esmfold, the default):
    Fold WT and MUTANT sequences -> wt.pdb + mut.pdb. Compute pLDDT (both) and
    CA-RMSD(WT<->mut) via Kabsch superposition, plus a local pLDDT delta at the
    mutated site. Upload both structures to khemeia-structures and write a
    `genome_calc` staging row carrying the §6(b) result envelope for the
    result-writer (GEN-22) to map onto variant_results.

Lifecycle MIRRORS containers/gnina/scripts/dock_batch.py verbatim in shape:
  env config (require_env) -> connect_db (psycopg2, %s) -> fetch inputs
  (Postgres rows + Garage S3 via boto3) -> compute on GPU -> INSERT INTO staging
  (genome_calc) / UPDATE variant_resolutions -> _jlog worker_start/progress/
  batch_complete. Failures emit a frozen E_* code (core Appendix A), never a
  silent drop.

The controller injects context via buildCRDJobEnv (crd_controller.go): JOB_NAME
is the GenomeJob CR name; spec fields arrive as UPPERCASE env vars
(RESOLUTION_ID, GROUP_NAME, VARIANT_KEY, CALCULATION, ROLE, MODEL); Postgres +
GARAGE_* creds are injected the same way as for the docking workers. The worker
also reads its own row by `WHERE cr_name = JOB_NAME` so it can recover
group_name/variant_key/resolution_id even if a spec field is absent.

Weights load from the node-local PVC mounted at $TORCH_HOME / $ESM_CACHE (GEN-21),
NOT baked into the image (workers TDD §4.1).
"""

import hashlib
import io
import json
import os
import sys
import time as _time

import psycopg2

try:
    import boto3
    from botocore.config import Config as BotoConfig
except ImportError:  # pragma: no cover - boto3 is always installed in the image
    boto3 = None

# Frozen typed error codes (core Appendix A).
E_STRUCTURE_FOLD_FAILED = "E_STRUCTURE_FOLD_FAILED"
E_WT_MISMATCH = "E_WT_MISMATCH"
E_RESOLVE_UPSTREAM = "E_RESOLVE_UPSTREAM"

# S3 layout (core §5.5). resolution_id is the resolution-scoped jobName so
# artifacts dedup with the cache.
BUCKET_STRUCTURES = "khemeia-structures"

# Length cap (workers TDD §4.5 / R-W3): ESMFold memory scales ~L^2; beyond this
# the 3070 OOMs. Over-length -> E_STRUCTURE_FOLD_FAILED rather than a hard crash.
DEFAULT_MAX_RESIDUES = 1024

# Local window (± residues) for the per-site pLDDT delta (workers TDD §4.2).
DEFAULT_LOCAL_WINDOW = 5
# Heuristic: a local pLDDT drop beyond this magnitude flags destabilizing_local.
DEFAULT_DESTABILIZING_THRESHOLD = -5.0


def _jlog(event: str, **kwargs) -> None:
    """Emit a structured JSON metric line for Alloy/Loki ingestion.

    Identical convention to dock_batch.py: a `metric: {json}` line.
    """
    payload = {"event": event, "ts": _time.time(), **kwargs}
    print("metric: " + json.dumps(payload, separators=(",", ":")), flush=True)


class FoldError(Exception):
    """Carries a frozen E_* code so callers map failures to the contract."""

    def __init__(self, code: str, message: str):
        super().__init__(message)
        self.code = code
        self.message = message


def require_env(name: str) -> str:
    value = os.environ.get(name)
    if not value:
        print(f"FATAL: required environment variable {name} is not set", flush=True)
        sys.exit(1)
    return value


def get_config() -> dict:
    """Read all configuration from environment variables.

    JOB_NAME (the GenomeJob CR name) is the authoritative key into
    variant_calc_jobs. RESOLUTION_ID / GROUP_NAME / VARIANT_KEY / CALCULATION /
    ROLE / MODEL arrive as spec-derived UPPERCASE env vars but are treated as
    hints — the variant_calc_jobs row is the source of truth and backfills any
    that are absent.
    """
    return {
        "job_name": require_env("JOB_NAME"),
        # Spec-derived hints (may be empty; backfilled from the calc-job row).
        "resolution_id": os.environ.get("RESOLUTION_ID", ""),
        "group_name": os.environ.get("GROUP_NAME", ""),
        "variant_key": os.environ.get("VARIANT_KEY", ""),
        "calculation": os.environ.get("CALCULATION", "esmfold"),
        "role": os.environ.get("ROLE", "calc"),  # calc | resolve_fallback
        "model": os.environ.get("MODEL", "wt_and_mut"),  # wt_and_mut | wt_only
        "max_residues": int(os.environ.get("MAX_RESIDUES", str(DEFAULT_MAX_RESIDUES))),
        "local_window": int(os.environ.get("LOCAL_WINDOW", str(DEFAULT_LOCAL_WINDOW))),
        "weights_preload": os.environ.get("WEIGHTS_PRELOAD", "false") == "true",
        # Postgres (same env names as the docking workers).
        "pg_host": os.environ.get("POSTGRES_HOST", "localhost"),
        "pg_port": int(os.environ.get("POSTGRES_PORT", "5432")),
        "pg_user": os.environ.get("POSTGRES_USER", "root"),
        "pg_password": require_env("POSTGRES_PASSWORD"),
        "pg_db": os.environ.get("POSTGRES_DB", "khemeia"),
        # Cache path for ESMFold/ESM-2 weights on the node-local PVC (GEN-21).
        "weights_dir": os.environ.get("TORCH_HOME")
        or os.environ.get("ESM_CACHE")
        or "/opt/esmfold/weights",
    }


def connect_db(cfg: dict):
    try:
        return psycopg2.connect(
            host=cfg["pg_host"],
            port=cfg["pg_port"],
            user=cfg["pg_user"],
            password=cfg["pg_password"],
            database=cfg["pg_db"],
        )
    except psycopg2.Error as exc:
        print(f"FATAL: PostgreSQL connection failed: {exc}", flush=True)
        sys.exit(1)


def get_s3_client():
    if os.environ.get("GARAGE_ENABLED") != "true":
        return None
    if boto3 is None:
        print("WARNING: boto3 not installed, S3 disabled", flush=True)
        return None
    return boto3.client(
        "s3",
        endpoint_url=os.environ.get("GARAGE_ENDPOINT"),
        aws_access_key_id=os.environ.get("GARAGE_ACCESS_KEY"),
        aws_secret_access_key=os.environ.get("GARAGE_SECRET_KEY"),
        region_name=os.environ.get("GARAGE_REGION", "garage"),
        config=BotoConfig(signature_version="s3v4"),
    )


# ── Input resolution ──────────────────────────────────────────────────────────

def fetch_calc_job(cursor, job_name: str):
    """Read this worker's own row via cr_name = JOB_NAME (the prompt's contract).

    Returns (group_name, variant_key, calculation, resolution_id, params_json).
    The row is authoritative for the identifiers; spec env vars are only hints.
    """
    cursor.execute(
        "SELECT group_name, variant_key, calculation, resolution_id, params "
        "FROM variant_calc_jobs WHERE cr_name = %s",
        (job_name,),
    )
    row = cursor.fetchone()
    if row is None:
        # resolve_fallback jobs may be minted without a calc-job row (they write
        # a resolution, not a calc result). Fall back to env-only context.
        return None
    return row


def fetch_resolution(cursor, resolution_id: str):
    """Fetch the ResolvedVariant fields from variant_resolutions by resolution_id.

    The full ResolvedVariant doc (incl. `sequence`) lives in the `resolved`
    JSONB (core §5.3); the scalar columns mirror it. Returns a dict with the
    fields the fold needs.
    """
    cursor.execute(
        "SELECT residue_index, wild_type_aa, mutant_aa, sequence_length, "
        "structure_bucket, structure_key, resolved "
        "FROM variant_resolutions WHERE resolution_id = %s",
        (resolution_id,),
    )
    row = cursor.fetchone()
    if row is None:
        raise FoldError(
            E_RESOLVE_UPSTREAM,
            f"resolution_id '{resolution_id}' not found in variant_resolutions",
        )
    residue_index, wt_aa, mut_aa, seq_len, bucket, key, resolved = row

    if isinstance(resolved, dict):
        doc = resolved
    elif isinstance(resolved, str):
        doc = json.loads(resolved)
    elif resolved is not None:
        doc = json.loads(resolved.decode("utf-8"))
    else:
        doc = {}

    sequence = doc.get("sequence")
    if not sequence:
        raise FoldError(
            E_RESOLVE_UPSTREAM,
            f"resolution '{resolution_id}' has no `sequence` in resolved doc",
        )

    return {
        "residue_index": int(residue_index),
        "wild_type_aa": (wt_aa or "").strip(),
        "mutant_aa": (mut_aa or "").strip(),
        "sequence_length": int(seq_len) if seq_len is not None else len(sequence),
        "structure_bucket": bucket,
        "structure_key": key,
        "sequence": sequence,
        "structure_source": doc.get("structure_source"),
    }


def validate_wt(res: dict, resolution_id: str) -> None:
    """Defense-in-depth WT check (workers TDD §4.2 step 2).

    The adapter already guards this; re-validate so a drifted resolution never
    silently produces a wrong-residue mutant model.
    """
    seq = res["sequence"]
    idx = res["residue_index"]
    wt = res["wild_type_aa"]
    if idx < 1 or idx > len(seq):
        raise FoldError(
            E_WT_MISMATCH,
            f"residue_index {idx} out of range for sequence length {len(seq)}",
        )
    actual = seq[idx - 1]
    if wt and actual != wt:
        raise FoldError(
            E_WT_MISMATCH,
            f"sequence[{idx}]={actual!r} != wild_type_aa {wt!r}",
        )


def apply_mutation(sequence: str, residue_index: int, mutant_aa: str) -> str:
    """Return the sequence with mutant_aa substituted at the 1-based index."""
    i = residue_index - 1
    return sequence[:i] + mutant_aa + sequence[i + 1:]


# ── S3 key helpers (core §5.5) ────────────────────────────────────────────────

def resolve_structure_key(resolution_id: str) -> str:
    return f"resolve/{resolution_id}/structure.pdb"


def wt_structure_key(resolution_id: str) -> str:
    return f"esmfold/{resolution_id}/wt.pdb"


def mut_structure_key(resolution_id: str) -> str:
    return f"esmfold/{resolution_id}/mut.pdb"


def s3_put_pdb(s3, bucket: str, key: str, pdb_text: str) -> None:
    if s3 is None:
        raise FoldError(
            E_RESOLVE_UPSTREAM, "S3 disabled but structure storage required"
        )
    s3.put_object(
        Bucket=bucket,
        Key=key,
        Body=pdb_text.encode("utf-8"),
        ContentType="chemical/x-pdb",
    )


def s3_get_pdb(s3, bucket: str, key: str):
    """Return PDB text for a key, or None if absent (used for the cached-WT path)."""
    if s3 is None:
        return None
    try:
        resp = s3.get_object(Bucket=bucket, Key=key)
        return resp["Body"].read().decode("utf-8")
    except Exception:
        return None


# ── ESMFold compute ───────────────────────────────────────────────────────────

_MODEL = None  # process-global; one fold job folds WT + mut on the same model.


def load_model(weights_dir: str, preload: bool):
    """Load esmfold_v1 with weights from the node-local PVC cache ($TORCH_HOME).

    fair-esm's loader uses torch.hub, which reads/writes under $TORCH_HOME/hub.
    We set TORCH_HOME to the PVC mount so a warm cache loads from disk (~seconds)
    and never re-downloads. On a cold/empty PVC, downloading is only allowed when
    WEIGHTS_PRELOAD=true (the GEN-21 warm Job path); otherwise a cold cache is a
    misconfiguration and we fail loudly rather than silently pulling GBs mid-job.
    """
    global _MODEL
    if _MODEL is not None:
        return _MODEL

    os.makedirs(weights_dir, exist_ok=True)
    # Point torch.hub at the PVC so weights resolve from / persist to the cache.
    os.environ["TORCH_HOME"] = weights_dir

    hub_dir = os.path.join(weights_dir, "hub", "checkpoints")
    cache_present = os.path.isdir(hub_dir) and any(os.scandir(hub_dir))
    if not cache_present and not preload:
        raise FoldError(
            E_RESOLVE_UPSTREAM,
            f"ESMFold weights cache empty at {weights_dir} and WEIGHTS_PRELOAD "
            "is not set; run the esmfold-weights-warm Job first (GEN-21)",
        )

    try:
        import torch
        import esm

        model = esm.pretrained.esmfold_v1()
        model = model.eval()
        if torch.cuda.is_available():
            model = model.cuda()
        # Chunked attention keeps long sequences within the 3070's memory.
        try:
            model.set_chunk_size(128)
        except Exception:
            pass
        _MODEL = model
        return _MODEL
    except FoldError:
        raise
    except Exception as exc:  # torch/esm import or load failure
        raise FoldError(
            E_STRUCTURE_FOLD_FAILED, f"ESMFold model load failed: {exc}"
        )


def fold_sequence(model, sequence: str, max_residues: int):
    """Fold one sequence -> (pdb_text, plddt_mean, per_residue_plddt).

    ESMFold returns per-residue pLDDT in the 0-100 range. We return the global
    mean and the per-residue vector (for the local-site delta).
    """
    if len(sequence) > max_residues:
        raise FoldError(
            E_STRUCTURE_FOLD_FAILED,
            f"sequence length {len(sequence)} exceeds cap {max_residues} "
            "(ESMFold memory ~ L^2; would OOM the GPU)",
        )
    try:
        import torch

        with torch.no_grad():
            output = model.infer(sequence)
            pdb_text = model.output_to_pdb(output)[0]
            # plddt: [B, L, 37] atom-level confidence in 0-1; mean over atoms ->
            # per-residue, then *100 for the 0-100 convention used in the contract.
            plddt = output["plddt"]
            per_res = plddt[0].mean(dim=-1).detach().cpu().numpy() * 100.0
        plddt_mean = float(per_res.mean())
        return pdb_text, plddt_mean, per_res
    except FoldError:
        raise
    except RuntimeError as exc:
        # CUDA OOM surfaces as RuntimeError — map to the frozen fold-failed code.
        raise FoldError(
            E_STRUCTURE_FOLD_FAILED, f"fold failed (len={len(sequence)}): {exc}"
        )
    except Exception as exc:
        raise FoldError(
            E_STRUCTURE_FOLD_FAILED, f"fold failed (len={len(sequence)}): {exc}"
        )


def ca_rmsd(wt_pdb: str, mut_pdb: str) -> float:
    """CA-atom RMSD between two models via Kabsch superposition (workers TDD §4.2).

    Uses biotite's superimpose over the shared CA set (WT and mutant have equal
    length — a point substitution — so CA correspondence is 1:1).
    """
    import biotite.structure.io.pdb as pdb
    import biotite.structure as struc
    import numpy as np

    def ca_array(pdb_text: str):
        f = pdb.PDBFile.read(io.StringIO(pdb_text))
        arr = f.get_structure(model=1)
        return arr[struc.filter_amino_acids(arr) & (arr.atom_name == "CA")]

    wt_ca = ca_array(wt_pdb)
    mut_ca = ca_array(mut_pdb)
    n = min(wt_ca.array_length(), mut_ca.array_length())
    if n == 0:
        raise FoldError(E_STRUCTURE_FOLD_FAILED, "no CA atoms for RMSD")
    wt_ca = wt_ca[:n]
    mut_ca = mut_ca[:n]
    fitted, _ = struc.superimpose(wt_ca, mut_ca)
    return float(struc.rmsd(wt_ca, fitted))


def local_plddt_delta(wt_plddt, mut_plddt, residue_index: int, window: int) -> float:
    """mut local mean pLDDT - WT local mean pLDDT over residue_index ± window."""
    import numpy as np

    i = residue_index - 1
    lo = max(0, i - window)
    hi = min(len(wt_plddt), i + window + 1)
    wt_local = float(np.mean(wt_plddt[lo:hi]))
    mut_local = float(np.mean(mut_plddt[lo:hi]))
    return mut_local - wt_local


# ── Roles ─────────────────────────────────────────────────────────────────────

def run_resolve_fallback(conn, cursor, s3, cfg, res, resolution_id) -> None:
    """Fold WT only -> structure.pdb; update variant_resolutions. No staging row."""
    t0 = _time.time()
    _jlog("worker_start", job=cfg["job_name"], role="resolve_fallback",
          resolution_id=resolution_id, calculation="esmfold",
          seq_len=res["sequence_length"])

    model = load_model(cfg["weights_dir"], cfg["weights_preload"])
    wt_pdb, plddt_mean, _ = fold_sequence(model, res["sequence"], cfg["max_residues"])
    _jlog("progress", job=cfg["job_name"], role="resolve_fallback",
          resolution_id=resolution_id, stage="folded_wt",
          plddt_mean=round(plddt_mean, 2))

    key = resolve_structure_key(resolution_id)
    s3_put_pdb(s3, BUCKET_STRUCTURES, key, wt_pdb)

    cursor.execute(
        "UPDATE variant_resolutions SET structure_source = 'esmfold', "
        "structure_bucket = %s, structure_key = %s, plddt_global = %s "
        "WHERE resolution_id = %s",
        (BUCKET_STRUCTURES, key, plddt_mean, resolution_id),
    )
    conn.commit()

    _jlog("batch_complete", job=cfg["job_name"], role="resolve_fallback",
          resolution_id=resolution_id, structure_key=key,
          plddt_global=round(plddt_mean, 2),
          elapsed_s=round(_time.time() - t0, 1))


def run_calc(conn, cursor, s3, cfg, res, resolution_id) -> None:
    """Fold WT + mutant -> §6(b) payload via genome_calc staging."""
    t0 = _time.time()
    _jlog("worker_start", job=cfg["job_name"], role="calc",
          resolution_id=resolution_id, calculation="esmfold",
          model=cfg["model"], seq_len=res["sequence_length"])

    model = load_model(cfg["weights_dir"], cfg["weights_preload"])

    # WT: reuse the cached resolve WT structure when present (idempotent,
    # resolution-scoped key — workers TDD §4.0). We still need per-residue pLDDT
    # for the local delta, which the cached PDB does not carry, so fold WT here.
    # The cached structure is honoured as the canonical resolve artifact; the
    # esmfold/{rid}/wt.pdb we write is the calc-scoped WT model.
    wt_pdb, wt_plddt_mean, wt_per_res = fold_sequence(
        model, res["sequence"], cfg["max_residues"]
    )
    _jlog("progress", job=cfg["job_name"], role="calc",
          resolution_id=resolution_id, stage="folded_wt",
          plddt_mean=round(wt_plddt_mean, 2))

    mut_sequence = apply_mutation(
        res["sequence"], res["residue_index"], res["mutant_aa"]
    )
    mut_pdb, mut_plddt_mean, mut_per_res = fold_sequence(
        model, mut_sequence, cfg["max_residues"]
    )
    _jlog("progress", job=cfg["job_name"], role="calc",
          resolution_id=resolution_id, stage="folded_mut",
          plddt_mean=round(mut_plddt_mean, 2))

    wt_key = wt_structure_key(resolution_id)
    mut_key = mut_structure_key(resolution_id)
    s3_put_pdb(s3, BUCKET_STRUCTURES, wt_key, wt_pdb)
    s3_put_pdb(s3, BUCKET_STRUCTURES, mut_key, mut_pdb)

    rmsd = ca_rmsd(wt_pdb, mut_pdb)
    delta = local_plddt_delta(
        wt_per_res, mut_per_res, res["residue_index"], cfg["local_window"]
    )
    destabilizing = delta < DEFAULT_DESTABILIZING_THRESHOLD

    # §6(b) payload (core §6(b) verbatim shape).
    payload = {
        "wt": {"plddt_mean": round(wt_plddt_mean, 2), "structure_key": wt_key},
        "mut": {"plddt_mean": round(mut_plddt_mean, 2), "structure_key": mut_key},
        "rmsd_ang": round(rmsd, 3),
        "plddt_delta_at_site": round(delta, 2),
        "destabilizing_local": bool(destabilizing),
        "interpretation": _interpret(rmsd, delta, destabilizing),
    }

    # §5.6 staging envelope. headline.* -> typed columns (GEN-22 maps these);
    # confidence is a calibrated function of the mutant model pLDDT (workers
    # TDD §4.4: plddt_mean/100).
    envelope = {
        "group_name": cfg["group_name"],
        "variant_key": cfg["variant_key"],
        "calculation": "esmfold",
        "resolution_id": resolution_id,
        "structure_source": res.get("structure_source") or "esmfold",
        "headline": {
            "esmfold_plddt": round(mut_plddt_mean, 2),
            "esmfold_rmsd_ang": round(rmsd, 3),
            "confidence": round(mut_plddt_mean / 100.0, 4),
        },
        "payload": payload,
        "artifact_keys": {
            "wt_structure": wt_key,
            "mut_structure": mut_key,
        },
    }

    cursor.execute(
        "INSERT INTO staging (job_type, payload) VALUES ('genome_calc', %s)",
        (json.dumps(envelope),),
    )
    conn.commit()

    # `metric:` line carrying the values the esmfold plugin parses (workers
    # TDD §4.7: rmsd_ang, mut_plddt).
    _jlog("batch_complete", job=cfg["job_name"], role="calc",
          resolution_id=resolution_id, rmsd_ang=round(rmsd, 3),
          mut_plddt=round(mut_plddt_mean, 2), wt_plddt=round(wt_plddt_mean, 2),
          plddt_delta_at_site=round(delta, 2),
          destabilizing_local=bool(destabilizing),
          elapsed_s=round(_time.time() - t0, 1))


def _interpret(rmsd: float, delta: float, destabilizing: bool) -> str:
    backbone = "modest backbone perturbation" if rmsd < 2.0 else "notable backbone shift"
    if destabilizing:
        return f"Mutation locally lowers model confidence; {backbone}"
    return f"Mutation has limited local confidence effect; {backbone}"


# ── Entrypoint ────────────────────────────────────────────────────────────────

def resolve_context(cfg: dict, cursor) -> str:
    """Backfill identifiers from the variant_calc_jobs row; return resolution_id.

    cr_name = JOB_NAME is the source of truth. resolve_fallback jobs may have no
    calc-job row, so we fall back to the injected RESOLUTION_ID env in that case.
    """
    job = fetch_calc_job(cursor, cfg["job_name"])
    if job is not None:
        group_name, variant_key, calculation, resolution_id, _params = job
        # Row wins; only fill blanks (env hints are secondary).
        cfg["group_name"] = cfg["group_name"] or (group_name or "")
        cfg["variant_key"] = cfg["variant_key"] or (variant_key or "")
        cfg["calculation"] = calculation or cfg["calculation"]
        resolution_id = resolution_id or cfg["resolution_id"]
    else:
        resolution_id = cfg["resolution_id"]

    if not resolution_id:
        raise FoldError(
            E_RESOLVE_UPSTREAM,
            f"no resolution_id for job '{cfg['job_name']}' "
            "(absent from variant_calc_jobs row and RESOLUTION_ID env)",
        )
    cfg["resolution_id"] = resolution_id
    return resolution_id


def fail(conn, cursor, cfg, code: str, message: str) -> None:
    """Record a typed failure on the owning variant_calc_jobs row and exit 1.

    Mirrors the contract that every failure carries a frozen E_* code in
    variant_calc_jobs.error_output (core §10 / workers TDD AC #4). resolve_
    fallback jobs without a calc-job row simply log + exit (the controller marks
    dependents Failed via the CR status).
    """
    print(f"FATAL [{code}]: {message}", flush=True)
    _jlog("error", job=cfg.get("job_name"), role=cfg.get("role"),
          resolution_id=cfg.get("resolution_id"), code=code, message=message)
    try:
        cursor.execute(
            "UPDATE variant_calc_jobs SET status = 'Failed', error_output = %s, "
            "completed_at = CURRENT_TIMESTAMP WHERE cr_name = %s",
            (f"{code}: {message}", cfg["job_name"]),
        )
        conn.commit()
    except Exception:
        pass
    sys.exit(1)


def main() -> None:
    cfg = get_config()
    print(
        f"ESMFold worker starting: job={cfg['job_name']} role={cfg['role']} "
        f"calc={cfg['calculation']} model={cfg['model']}",
        flush=True,
    )

    conn = connect_db(cfg)
    cursor = conn.cursor()
    s3 = get_s3_client()

    try:
        resolution_id = resolve_context(cfg, cursor)
        res = fetch_resolution(cursor, resolution_id)
        validate_wt(res, resolution_id)

        if cfg["role"] == "resolve_fallback" or cfg["model"] == "wt_only":
            run_resolve_fallback(conn, cursor, s3, cfg, res, resolution_id)
        else:
            run_calc(conn, cursor, s3, cfg, res, resolution_id)
    except FoldError as exc:
        fail(conn, cursor, cfg, exc.code, exc.message)
    except Exception as exc:  # last-resort: never a silent drop
        fail(conn, cursor, cfg, E_STRUCTURE_FOLD_FAILED, f"unexpected: {exc}")
    finally:
        cursor.close()
        conn.close()


if __name__ == "__main__":
    main()
