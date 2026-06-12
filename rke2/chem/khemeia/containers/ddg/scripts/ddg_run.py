#!/usr/bin/env python3
"""ddG-of-folding stability worker for the Khemeia genomics layer.

Calc: spec.calculation=ddg_stability (workers TDD §5). Given a resolved variant
(WT structure PDB + a point substitution), predicts the change in folding free
energy on mutation:

    ΔΔG_fold = G_fold(mutant) - G_fold(wild-type)   [kcal/mol]

SIGN CONVENTION (workers TDD §5 / §6(c)): POSITIVE = DESTABILIZING (the mutant
folds less favorably / unfolds more readily than WT); negative = stabilizing.
All engines normalize to this convention before reporting.

ENGINE / LICENSE BOUNDARY (workers TDD §5.1, plan R-W2 — HARD constraint: the
platform serves an EXTERNAL consumer, u4u):
  * thermompnn (DEFAULT) — MIT. Baked into the image. Sub-second CPU inference.
  * ddgun                — GPL-3.0, open. Baked in. Second open fallback.
  * foldx / rosetta      — academic-only / NON-REDISTRIBUTABLE. NEVER baked.
                           Supported ONLY by shelling out to a binary the
                           operator mounts OUT-OF-BAND at $FOLDX_BIN/$ROSETTA_BIN.
                           Binary absent -> E_PARAMS_INVALID ("engine not
                           provisioned"). We redistribute ONLY the open engines.

Lifecycle MIRRORS containers/esmfold/scripts/fold.py (the closest sibling) and
containers/gnina/scripts/dock_batch.py verbatim in shape:
  env config (require_env) -> connect_db (psycopg2, %s) -> read this worker's
  own row by WHERE cr_name = JOB_NAME -> fetch the variant_resolutions row ->
  fetch the WT structure PDB from Garage S3 -> run the selected engine -> write
  report.json to S3 -> INSERT INTO staging (genome_calc) -> _jlog worker_start/
  progress/batch_complete. Failures emit a frozen E_* code (core Appendix A) to
  variant_calc_jobs.error_output and exit nonzero — never a silent drop.

The controller injects context the same way as for esmfold: JOB_NAME is the
GenomeJob CR name; spec fields arrive as UPPERCASE env vars (RESOLUTION_ID,
GROUP_NAME, VARIANT_KEY, CALCULATION, ENGINE); Postgres + GARAGE_* creds are
injected identically to the docking workers.
"""

import io
import json
import math
import os
import shutil
import subprocess
import sys
import tempfile
import time as _time

import psycopg2

try:
    import boto3
    from botocore.config import Config as BotoConfig
except ImportError:  # pragma: no cover - boto3 is always installed in the image
    boto3 = None

# Frozen typed error codes (core Appendix A).
E_PARAMS_INVALID = "E_PARAMS_INVALID"
E_RESOLVE_UPSTREAM = "E_RESOLVE_UPSTREAM"
E_WT_MISMATCH = "E_WT_MISMATCH"
E_STRUCTURE_FOLD_FAILED = "E_STRUCTURE_FOLD_FAILED"

# S3 layout (core §5.5). The WT structure is read from khemeia-structures at the
# resolve key; the report.json artifact is written to khemeia-reports (which the
# core §5.5 names for ddg reports; T2: report.json, NOT foldx_report.json, since
# the engine is now variable).
BUCKET_STRUCTURES = "khemeia-structures"
BUCKET_REPORTS = "khemeia-reports"

# Engine enum (T1: widened from foldx|rosetta to thermompnn|ddgun|foldx|rosetta).
ENGINE_THERMOMPNN = "thermompnn"
ENGINE_DDGUN = "ddgun"
ENGINE_FOLDX = "foldx"
ENGINE_ROSETTA = "rosetta"
OPEN_ENGINES = {ENGINE_THERMOMPNN, ENGINE_DDGUN}
MOUNTED_BINARY_ENGINES = {ENGINE_FOLDX, ENGINE_ROSETTA}
VALID_ENGINES = OPEN_ENGINES | MOUNTED_BINARY_ENGINES

# stability_class thresholds on ΔΔG_fold (kcal/mol), workers TDD §5.4:
#   < -0.5 stabilizing ; [-0.5, 0.5] neutral ; > 0.5 destabilizing.
STABILIZING_THRESHOLD = -0.5
DESTABILIZING_THRESHOLD = 0.5


def _jlog(event: str, **kwargs) -> None:
    """Emit a structured JSON metric line for Alloy/Loki ingestion.

    Identical convention to fold.py / dock_batch.py: a `metric: {json}` line.
    """
    payload = {"event": event, "ts": _time.time(), **kwargs}
    print("metric: " + json.dumps(payload, separators=(",", ":")), flush=True)


class DdgError(Exception):
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
    ENGINE arrive as spec-derived UPPERCASE env vars but are treated as hints —
    the variant_calc_jobs row (and its params JSONB) is the source of truth and
    backfills any that are absent.
    """
    return {
        "job_name": require_env("JOB_NAME"),
        # Spec-derived hints (may be empty; backfilled from the calc-job row).
        "resolution_id": os.environ.get("RESOLUTION_ID", ""),
        "group_name": os.environ.get("GROUP_NAME", ""),
        "variant_key": os.environ.get("VARIANT_KEY", ""),
        "calculation": os.environ.get("CALCULATION", "ddg_stability"),
        "engine": os.environ.get("ENGINE", ""),  # backfilled from params if empty
        "n_runs": os.environ.get("N_RUNS", ""),   # default chosen per-engine below
        # Postgres (same env names as the docking / esmfold workers).
        "pg_host": os.environ.get("POSTGRES_HOST", "localhost"),
        "pg_port": int(os.environ.get("POSTGRES_PORT", "5432")),
        "pg_user": os.environ.get("POSTGRES_USER", "root"),
        "pg_password": require_env("POSTGRES_PASSWORD"),
        "pg_db": os.environ.get("POSTGRES_DB", "khemeia"),
        # Baked-in MIT engine checkpoints (set in the Dockerfile).
        "thermompnn_dir": os.environ.get("THERMOMPNN_DIR", "/opt/ddg/engines/thermompnn"),
        "proteinmpnn_dir": os.environ.get("PROTEINMPNN_DIR", "/opt/ddg/engines/proteinmpnn"),
        "ddgun_dir": os.environ.get("DDGUN_DIR", "/opt/ddg/engines/ddgun"),
        # Out-of-band, operator-mounted, NON-REDISTRIBUTABLE binaries (may be absent).
        "foldx_bin": os.environ.get("FOLDX_BIN", "/opt/foldx/foldx"),
        "rosetta_bin": os.environ.get("ROSETTA_BIN", "/opt/rosetta/cartesian_ddg"),
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
    """Read this worker's own row via cr_name = JOB_NAME (the frozen contract).

    Returns (group_name, variant_key, calculation, resolution_id, params_json).
    The row is authoritative for the identifiers; spec env vars are only hints.
    """
    cursor.execute(
        "SELECT group_name, variant_key, calculation, resolution_id, params "
        "FROM variant_calc_jobs WHERE cr_name = %s",
        (job_name,),
    )
    return cursor.fetchone()


def _as_dict(value):
    if isinstance(value, dict):
        return value
    if isinstance(value, str):
        return json.loads(value) if value.strip() else {}
    if isinstance(value, (bytes, bytearray)):
        return json.loads(value.decode("utf-8"))
    return {}


def fetch_resolution(cursor, resolution_id: str) -> dict:
    """Fetch the ResolvedVariant fields from variant_resolutions by resolution_id.

    The WT structure lives in S3 at (structure_bucket, structure_key); the
    sequence + residue scalars come from the row and the `resolved` JSONB
    (core §5.3). Returns a dict with the fields the engine needs.
    """
    cursor.execute(
        "SELECT residue_index, wild_type_aa, mutant_aa, sequence_length, "
        "structure_bucket, structure_key, resolved "
        "FROM variant_resolutions WHERE resolution_id = %s",
        (resolution_id,),
    )
    row = cursor.fetchone()
    if row is None:
        raise DdgError(
            E_RESOLVE_UPSTREAM,
            f"resolution_id '{resolution_id}' not found in variant_resolutions",
        )
    residue_index, wt_aa, mut_aa, seq_len, bucket, key, resolved = row
    doc = _as_dict(resolved)
    sequence = doc.get("sequence")

    return {
        "residue_index": int(residue_index),
        "wild_type_aa": (wt_aa or "").strip(),
        "mutant_aa": (mut_aa or "").strip(),
        "sequence_length": int(seq_len) if seq_len is not None else (
            len(sequence) if sequence else 0
        ),
        "structure_bucket": bucket or BUCKET_STRUCTURES,
        "structure_key": key,
        "sequence": sequence,
        "structure_source": doc.get("structure_source"),
    }


# ── S3 key helpers (core §5.5) ────────────────────────────────────────────────

def resolve_structure_key(resolution_id: str) -> str:
    return f"resolve/{resolution_id}/structure.pdb"


def report_key(resolution_id: str) -> str:
    # T2: report.json (not foldx_report.json) — engine is variable.
    return f"ddg_stability/{resolution_id}/report.json"


def s3_get_pdb(s3, bucket: str, key: str):
    if s3 is None:
        return None
    try:
        resp = s3.get_object(Bucket=bucket, Key=key)
        return resp["Body"].read().decode("utf-8")
    except Exception:
        return None


def s3_put_json(s3, bucket: str, key: str, obj: dict) -> None:
    if s3 is None:
        raise DdgError(E_RESOLVE_UPSTREAM, "S3 disabled but report storage required")
    s3.put_object(
        Bucket=bucket,
        Key=key,
        Body=json.dumps(obj, indent=2).encode("utf-8"),
        ContentType="application/json",
    )


# ── WT validation + structure parsing ─────────────────────────────────────────

def _aa3_to_1(resname: str) -> str:
    table = {
        "ALA": "A", "ARG": "R", "ASN": "N", "ASP": "D", "CYS": "C",
        "GLN": "Q", "GLU": "E", "GLY": "G", "HIS": "H", "ILE": "I",
        "LEU": "L", "LYS": "K", "MET": "M", "PHE": "F", "PRO": "P",
        "SER": "S", "THR": "T", "TRP": "W", "TYR": "Y", "VAL": "V",
    }
    return table.get(resname.strip().upper(), "X")


def residue_from_structure(pdb_text: str, residue_index: int) -> str:
    """Return the 1-letter WT residue at residue_index as seen IN THE STRUCTURE.

    Defense in depth: the structure the engine scores must actually contain the
    residue we mutate, at the index we claim. A truncated/renumbered model that
    does not is an E_WT_MISMATCH, not a silent wrong-residue ddG.
    """
    import biotite.structure.io.pdb as pdb
    import biotite.structure as struc

    try:
        f = pdb.PDBFile.read(io.StringIO(pdb_text))
        arr = f.get_structure(model=1)
    except Exception as exc:
        raise DdgError(E_STRUCTURE_FOLD_FAILED, f"could not parse WT PDB: {exc}")

    arr = arr[struc.filter_amino_acids(arr)]
    if arr.array_length() == 0:
        raise DdgError(E_STRUCTURE_FOLD_FAILED, "WT PDB has no amino-acid atoms")

    # Map structural res_id -> 3-letter name (first chain encountered).
    for res_id, res_name in zip(arr.res_id, arr.res_name):
        if int(res_id) == residue_index:
            return _aa3_to_1(str(res_name))
    raise DdgError(
        E_WT_MISMATCH,
        f"residue_index {residue_index} not present in WT structure "
        "(truncated or renumbered model)",
    )


def validate_wt(res: dict, pdb_text: str) -> None:
    idx = res["residue_index"]
    wt = res["wild_type_aa"]
    if idx < 1:
        raise DdgError(E_PARAMS_INVALID, f"residue_index {idx} must be >= 1")
    if not res["mutant_aa"]:
        raise DdgError(E_PARAMS_INVALID, "mutant_aa is empty")
    struct_wt = residue_from_structure(pdb_text, idx)
    if wt and struct_wt != wt and struct_wt != "X":
        raise DdgError(
            E_WT_MISMATCH,
            f"structure residue at {idx} is {struct_wt!r} != wild_type_aa {wt!r}",
        )


# ── Engine selection ──────────────────────────────────────────────────────────

def resolve_engine(cfg: dict, params: dict) -> str:
    """Resolve + validate the engine. params (job row JSONB) wins over the env hint.

    params may be the per-calc params dict directly or nested under
    params['ddg_stability'] (controller may pass either); we look in both.
    """
    sub = {}
    if isinstance(params, dict):
        sub = params.get("ddg_stability") if isinstance(params.get("ddg_stability"), dict) else params
    engine = (cfg["engine"] or sub.get("engine") or ENGINE_THERMOMPNN).strip().lower()
    if engine not in VALID_ENGINES:
        raise DdgError(
            E_PARAMS_INVALID,
            f"engine {engine!r} not in {sorted(VALID_ENGINES)}",
        )
    return engine


def stability_class(ddg: float) -> str:
    if ddg < STABILIZING_THRESHOLD:
        return "stabilizing"
    if ddg > DESTABILIZING_THRESHOLD:
        return "destabilizing"
    return "neutral"


def _interpret(ddg: float, klass: str) -> str:
    if klass == "destabilizing":
        return f"Destabilizing (ΔΔG={ddg:+.2f} kcal/mol) — likely disrupts fold"
    if klass == "stabilizing":
        return f"Stabilizing (ΔΔG={ddg:+.2f} kcal/mol) — mutation favors the fold"
    return f"Near-neutral (ΔΔG={ddg:+.2f} kcal/mol) — limited fold-stability effect"


# ── Engine: ThermoMPNN (default, MIT, baked-in) ───────────────────────────────

def run_thermompnn(cfg: dict, res: dict, pdb_text: str) -> dict:
    """Predict ΔΔG_fold with ThermoMPNN (MIT). Sub-second CPU inference.

    ThermoMPNN scores a single point mutation on a structure and returns ΔΔG in
    kcal/mol with the field's standard convention (POSITIVE = destabilizing),
    which is exactly the contract convention — no sign flip needed. Returns the
    engine result dict (ddg + model confidence + empty per_term — an ML predictor
    has no physical energy decomposition).
    """
    sys.path.insert(0, os.path.join(cfg["thermompnn_dir"], "src"))
    sys.path.insert(0, os.path.join(cfg["proteinmpnn_dir"], "src"))
    try:
        # ThermoMPNN's inference entrypoint (single-mutation prediction over a
        # structure). The exact module path is pinned by the cloned commit; we
        # import lazily so an import failure surfaces as E_STRUCTURE_FOLD_FAILED
        # rather than crashing the whole worker at startup.
        from thermompnn.inference import predict_ddg  # type: ignore
    except Exception as exc:
        raise DdgError(
            E_STRUCTURE_FOLD_FAILED,
            f"ThermoMPNN import failed ({exc}); checkpoint baked at "
            f"{cfg['thermompnn_dir']}",
        )

    with tempfile.NamedTemporaryFile("w", suffix=".pdb", delete=False) as fh:
        fh.write(pdb_text)
        pdb_path = fh.name
    try:
        result = predict_ddg(
            pdb_path=pdb_path,
            position=res["residue_index"],
            wild_type=res["wild_type_aa"],
            mutant=res["mutant_aa"],
            proteinmpnn_weights=cfg["proteinmpnn_dir"],
        )
    except Exception as exc:
        raise DdgError(E_STRUCTURE_FOLD_FAILED, f"ThermoMPNN inference failed: {exc}")
    finally:
        try:
            os.unlink(pdb_path)
        except OSError:
            pass

    ddg = float(result["ddg"] if isinstance(result, dict) else result)
    if not math.isfinite(ddg):
        raise DdgError(E_STRUCTURE_FOLD_FAILED, "ThermoMPNN returned non-finite ΔΔG")
    confidence = float(result.get("confidence", 0.7)) if isinstance(result, dict) else 0.7
    return {
        "ddg_fold_kcal_mol": round(ddg, 3),
        "per_term": {},  # ML predictor: no physical per-term split
        "n_runs": 1,
        "stdev_kcal_mol": 0.0,
        "confidence": round(confidence, 4),
    }


# ── Engine: DDGun (open GPL fallback, baked-in) ───────────────────────────────

def run_ddgun(cfg: dict, res: dict, pdb_text: str) -> dict:
    """Predict ΔΔG_fold with DDGun3D (GPL-3.0, open). Sequence/structure based.

    DDGun reports ΔΔG with POSITIVE = stabilizing in some builds; we normalize to
    the contract convention (POSITIVE = DESTABILIZING) by negating DDGun's raw
    output. The pinned ddgun build here reports stabilizing-positive, so we flip.
    """
    ddgun_dir = os.path.join(cfg["ddgun_dir"], "src")
    script = os.path.join(ddgun_dir, "ddgun_3d.py")
    if not os.path.isfile(script):
        raise DdgError(
            E_PARAMS_INVALID,
            f"ddgun not provisioned (expected {script})",
        )
    mut = f"{res['wild_type_aa']}{res['residue_index']}{res['mutant_aa']}"
    with tempfile.NamedTemporaryFile("w", suffix=".pdb", delete=False) as fh:
        fh.write(pdb_text)
        pdb_path = fh.name
    try:
        proc = subprocess.run(
            ["python3", script, pdb_path, "A", mut],
            capture_output=True, text=True, timeout=600,
        )
    except subprocess.TimeoutExpired:
        raise DdgError(E_STRUCTURE_FOLD_FAILED, "ddgun timed out")
    finally:
        try:
            os.unlink(pdb_path)
        except OSError:
            pass
    if proc.returncode != 0:
        raise DdgError(
            E_STRUCTURE_FOLD_FAILED, f"ddgun failed: {proc.stderr.strip()[:200]}"
        )
    raw = _parse_ddgun_ddg(proc.stdout)
    # DDGun: stabilizing-positive -> flip to destabilizing-positive (contract).
    ddg = -raw
    if not math.isfinite(ddg):
        raise DdgError(E_STRUCTURE_FOLD_FAILED, "ddgun returned non-finite ΔΔG")
    return {
        "ddg_fold_kcal_mol": round(ddg, 3),
        "per_term": {},
        "n_runs": 1,
        "stdev_kcal_mol": 0.0,
        "confidence": 0.6,
    }


def _parse_ddgun_ddg(stdout: str) -> float:
    """Extract the ΔΔG float from ddgun's tab-separated output (last column)."""
    for line in reversed(stdout.strip().splitlines()):
        parts = line.split()
        for tok in reversed(parts):
            try:
                return float(tok)
            except ValueError:
                continue
    raise DdgError(E_STRUCTURE_FOLD_FAILED, "could not parse ddgun output")


# ── Engine: FoldX / Rosetta (mounted-binary adapters, NEVER baked) ────────────

def _require_mounted_binary(path: str, engine: str) -> str:
    """Fail with E_PARAMS_INVALID when a license-gated binary is not mounted.

    FoldX / Rosetta are NON-REDISTRIBUTABLE (workers TDD §5.1). They are never in
    the image; the operator mounts a licensed binary out-of-band. Absent ->
    a clear "engine not provisioned" E_PARAMS_INVALID (plan GEN-30 AC).
    """
    if not path or not (os.path.isfile(path) and os.access(path, os.X_OK)):
        raise DdgError(
            E_PARAMS_INVALID,
            f"engine '{engine}' not provisioned: no executable at {path!r}. "
            f"{engine} is license-gated and NOT shipped in this image; mount the "
            f"binary out-of-band to use it.",
        )
    return path


def run_foldx(cfg: dict, res: dict, pdb_text: str) -> dict:
    """FoldX BuildModel adapter — only when $FOLDX_BIN is mounted."""
    binary = _require_mounted_binary(cfg["foldx_bin"], ENGINE_FOLDX)
    n_runs = int(cfg["n_runs"] or "5")
    ddg, per_term, stdev = _run_physics_binary_foldx(
        binary, res, pdb_text, n_runs
    )
    return {
        "ddg_fold_kcal_mol": round(ddg, 3),
        "per_term": per_term,
        "n_runs": n_runs,
        "stdev_kcal_mol": round(stdev, 3),
        "confidence": 0.8,
    }


def run_rosetta(cfg: dict, res: dict, pdb_text: str) -> dict:
    """Rosetta cartesian_ddg adapter — only when $ROSETTA_BIN is mounted."""
    binary = _require_mounted_binary(cfg["rosetta_bin"], ENGINE_ROSETTA)
    n_runs = int(cfg["n_runs"] or "3")
    ddg, per_term, stdev = _run_physics_binary_rosetta(
        binary, res, pdb_text, n_runs
    )
    return {
        "ddg_fold_kcal_mol": round(ddg, 3),
        "per_term": per_term,
        "n_runs": n_runs,
        "stdev_kcal_mol": round(stdev, 3),
        "confidence": 0.85,
    }


def _run_physics_binary_foldx(binary, res, pdb_text, n_runs):
    """Shell out to FoldX BuildModel; parse the per-term energy decomposition.

    FoldX reports ΔΔG with POSITIVE = DESTABILIZING already (contract-aligned).
    Kept as a thin subprocess adapter (mirrors the vina/gnina shell-out pattern)
    so the heavy science lives in the operator-mounted binary, not in this image.
    """
    mut = f"{res['wild_type_aa']}{res['residue_index']}{res['mutant_aa']}"
    workdir = tempfile.mkdtemp(prefix="foldx_")
    try:
        pdb_path = os.path.join(workdir, "wt.pdb")
        with open(pdb_path, "w") as fh:
            fh.write(pdb_text)
        with open(os.path.join(workdir, "individual_list.txt"), "w") as fh:
            fh.write(f"{mut[0]}A{mut[1:]};\n")
        proc = subprocess.run(
            [binary, "--command=BuildModel", "--pdb=wt.pdb",
             "--mutant-file=individual_list.txt", f"--numberOfRuns={n_runs}"],
            cwd=workdir, capture_output=True, text=True, timeout=3600,
        )
        if proc.returncode != 0:
            raise DdgError(
                E_STRUCTURE_FOLD_FAILED, f"FoldX failed: {proc.stderr.strip()[:200]}"
            )
        return _parse_foldx_output(workdir)
    finally:
        shutil.rmtree(workdir, ignore_errors=True)


def _parse_foldx_output(workdir):
    """Parse FoldX Average_*.fxout for ΔΔG total + per-term energies."""
    import statistics
    ddgs = []
    per_term = {}
    for name in os.listdir(workdir):
        if name.startswith("Raw_") and name.endswith(".fxout"):
            with open(os.path.join(workdir, name)) as fh:
                for line in fh:
                    cols = line.split("\t")
                    if len(cols) > 2:
                        try:
                            ddgs.append(float(cols[1]))
                        except ValueError:
                            continue
    if not ddgs:
        raise DdgError(E_STRUCTURE_FOLD_FAILED, "no FoldX ΔΔG values parsed")
    ddg = sum(ddgs) / len(ddgs)
    stdev = statistics.pstdev(ddgs) if len(ddgs) > 1 else 0.0
    # FoldX exposes vdw/electro/solvation/hbond terms; left to the operator's
    # parser detail. We surface the totals and an empty/partial split here.
    return ddg, per_term, stdev


def _run_physics_binary_rosetta(binary, res, pdb_text, n_runs):
    """Shell out to Rosetta cartesian_ddg. POSITIVE = DESTABILIZING (REU->kcal).

    Rosetta reports ΔΔG in Rosetta Energy Units; the standard ddg_monomer/
    cartesian_ddg convention is POSITIVE = destabilizing. We report the REU value
    directly (operator's protocol may apply a REU->kcal scale; kept thin here).
    """
    import statistics
    mut = f"{res['wild_type_aa']}{res['residue_index']}{res['mutant_aa']}"
    workdir = tempfile.mkdtemp(prefix="rosetta_")
    try:
        pdb_path = os.path.join(workdir, "wt.pdb")
        with open(pdb_path, "w") as fh:
            fh.write(pdb_text)
        with open(os.path.join(workdir, "mutfile"), "w") as fh:
            fh.write(f"total 1\n1\n{mut[0]} {res['residue_index']} {mut[-1]}\n")
        proc = subprocess.run(
            [binary, "-s", "wt.pdb", "-ddg:mut_file", "mutfile",
             "-ddg:iterations", str(n_runs)],
            cwd=workdir, capture_output=True, text=True, timeout=7200,
        )
        if proc.returncode != 0:
            raise DdgError(
                E_STRUCTURE_FOLD_FAILED, f"Rosetta failed: {proc.stderr.strip()[:200]}"
            )
        ddgs = _parse_rosetta_output(workdir)
        ddg = sum(ddgs) / len(ddgs)
        stdev = statistics.pstdev(ddgs) if len(ddgs) > 1 else 0.0
        return ddg, {}, stdev
    finally:
        shutil.rmtree(workdir, ignore_errors=True)


def _parse_rosetta_output(workdir):
    ddgs = []
    for name in os.listdir(workdir):
        if name.endswith(".ddg"):
            with open(os.path.join(workdir, name)) as fh:
                for line in fh:
                    if line.strip().startswith("COMPLEX") or "ddG" in line:
                        for tok in line.split():
                            try:
                                ddgs.append(float(tok))
                                break
                            except ValueError:
                                continue
    if not ddgs:
        raise DdgError(E_STRUCTURE_FOLD_FAILED, "no Rosetta ΔΔG values parsed")
    return ddgs


ENGINE_DISPATCH = {
    ENGINE_THERMOMPNN: run_thermompnn,
    ENGINE_DDGUN: run_ddgun,
    ENGINE_FOLDX: run_foldx,
    ENGINE_ROSETTA: run_rosetta,
}


# ── Main run ──────────────────────────────────────────────────────────────────

def run_calc(conn, cursor, s3, cfg, params, res, resolution_id) -> None:
    """Run the selected engine -> §6(c) payload via genome_calc staging."""
    t0 = _time.time()
    engine = resolve_engine(cfg, params)
    _jlog("worker_start", job=cfg["job_name"], calculation="ddg_stability",
          resolution_id=resolution_id, engine=engine,
          mutation=f"{res['wild_type_aa']}{res['residue_index']}{res['mutant_aa']}")

    # Fetch the WT structure PDB from S3 (resolve key, core §5.5).
    bucket = res["structure_bucket"] or BUCKET_STRUCTURES
    key = res["structure_key"] or resolve_structure_key(resolution_id)
    pdb_text = s3_get_pdb(s3, bucket, key)
    if not pdb_text:
        raise DdgError(
            E_RESOLVE_UPSTREAM,
            f"WT structure not found at s3://{bucket}/{key}",
        )
    _jlog("progress", job=cfg["job_name"], resolution_id=resolution_id,
          stage="fetched_wt_structure", structure_key=key)

    # Defense-in-depth WT validation against the actual structure.
    validate_wt(res, pdb_text)

    # Run the engine.
    result = ENGINE_DISPATCH[engine](cfg, res, pdb_text)
    ddg = float(result["ddg_fold_kcal_mol"])
    klass = stability_class(ddg)
    _jlog("progress", job=cfg["job_name"], resolution_id=resolution_id,
          stage="engine_complete", engine=engine,
          ddg_fold_kcal_mol=round(ddg, 3), stability_class=klass)

    # Write the full report.json artifact (T2 key) to khemeia-reports.
    rkey = report_key(resolution_id)
    report = {
        "resolution_id": resolution_id,
        "group_name": cfg["group_name"],
        "variant_key": cfg["variant_key"],
        "engine": engine,
        "mutation": {
            "wild_type_aa": res["wild_type_aa"],
            "residue_index": res["residue_index"],
            "mutant_aa": res["mutant_aa"],
        },
        "structure_source": res.get("structure_source"),
        "sign_convention": "positive_destabilizing",
        "ddg_fold_kcal_mol": round(ddg, 3),
        "stability_class": klass,
        "per_term": result["per_term"],
        "n_runs": result["n_runs"],
        "stdev_kcal_mol": result["stdev_kcal_mol"],
        "confidence": result["confidence"],
    }
    s3_put_json(s3, BUCKET_REPORTS, rkey, report)

    # §6(c) payload (core shape).
    payload = {
        "engine": engine,
        "ddg_fold_kcal_mol": round(ddg, 3),
        "stability_class": klass,
        "per_term": result["per_term"],
        "n_runs": result["n_runs"],
        "stdev_kcal_mol": result["stdev_kcal_mol"],
        "report_key": rkey,
        "interpretation": _interpret(ddg, klass),
    }

    # §5.6 NESTED staging envelope. headline.* -> typed columns (GEN-22 maps
    # ddg_fold_kcal_mol + confidence). ddg_fold_kcal_mol goes in headline.
    envelope = {
        "group_name": cfg["group_name"],
        "variant_key": cfg["variant_key"],
        "calculation": "ddg_stability",
        "resolution_id": resolution_id,
        "structure_source": res.get("structure_source") or "alphafold",
        "headline": {
            "ddg_fold_kcal_mol": round(ddg, 3),
            "confidence": result["confidence"],
        },
        "payload": payload,
        "artifact_keys": {
            "report": rkey,
        },
    }

    cursor.execute(
        "INSERT INTO staging (job_type, payload) VALUES ('genome_calc', %s)",
        (json.dumps(envelope),),
    )
    conn.commit()

    # `metric:` line carrying the value the ddg plugin parses (ddg_fold_kcal_mol).
    _jlog("batch_complete", job=cfg["job_name"], calculation="ddg_stability",
          resolution_id=resolution_id, engine=engine,
          ddg_fold_kcal_mol=round(ddg, 3), stability_class=klass,
          confidence=result["confidence"], report_key=rkey,
          elapsed_s=round(_time.time() - t0, 1))


# ── Entrypoint ────────────────────────────────────────────────────────────────

def resolve_context(cfg: dict, cursor):
    """Backfill identifiers from the variant_calc_jobs row; return (rid, params).

    cr_name = JOB_NAME is the source of truth (the frozen contract).
    """
    job = fetch_calc_job(cursor, cfg["job_name"])
    params = {}
    if job is not None:
        group_name, variant_key, calculation, resolution_id, params_json = job
        cfg["group_name"] = cfg["group_name"] or (group_name or "")
        cfg["variant_key"] = cfg["variant_key"] or (variant_key or "")
        cfg["calculation"] = calculation or cfg["calculation"]
        resolution_id = resolution_id or cfg["resolution_id"]
        params = _as_dict(params_json)
    else:
        resolution_id = cfg["resolution_id"]

    if not resolution_id:
        raise DdgError(
            E_RESOLVE_UPSTREAM,
            f"no resolution_id for job '{cfg['job_name']}' "
            "(absent from variant_calc_jobs row and RESOLUTION_ID env)",
        )
    cfg["resolution_id"] = resolution_id
    return resolution_id, params


def fail(conn, cursor, cfg, code: str, message: str) -> None:
    """Record a typed failure on the owning variant_calc_jobs row and exit 1.

    Every failure carries a frozen E_* code in variant_calc_jobs.error_output
    (core §10 / workers TDD AC #4) — never a silent drop.
    """
    print(f"FATAL [{code}]: {message}", flush=True)
    _jlog("error", job=cfg.get("job_name"), calculation="ddg_stability",
          resolution_id=cfg.get("resolution_id"), engine=cfg.get("engine"),
          code=code, message=message)
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
        f"ddG worker starting: job={cfg['job_name']} "
        f"engine_hint={cfg['engine'] or '(from params)'}",
        flush=True,
    )

    conn = connect_db(cfg)
    cursor = conn.cursor()
    s3 = get_s3_client()

    try:
        resolution_id, params = resolve_context(cfg, cursor)
        res = fetch_resolution(cursor, resolution_id)
        run_calc(conn, cursor, s3, cfg, params, res, resolution_id)
    except DdgError as exc:
        fail(conn, cursor, cfg, exc.code, exc.message)
    except Exception as exc:  # last-resort: never a silent drop
        fail(conn, cursor, cfg, E_STRUCTURE_FOLD_FAILED, f"unexpected: {exc}")
    finally:
        cursor.close()
        conn.close()


if __name__ == "__main__":
    main()
