#!/usr/bin/env python3
"""PGx drug-response docking orchestrator (calc ``pgx_docking``) — the genomics
capstone (workers TDD §7).

This is the HIGHEST-complexity genome worker: a *composite* that does NOT add new
science or a new image. It runs inside the EXISTING ``gnina`` container image
(gnina is the default docking engine) and orchestrates the already-deployed
Khemeia compute stack to compare a drug's binding against the WILD-TYPE vs the
MUTANT protein for a single resolved missense variant:

  WT structure   (resolve/{rid}/structure.pdb — AlphaFold DB or ESMFold WT)
  MUTANT struct. (esmfold/{rid}/mut.pdb       — ALWAYS ESMFold; AlphaFold has no
                                                 mutant models)
  drug -> ligand (library-prep /standardize  — RDKit MolStandardize + ETKDG 3D +
                                                 Meeko PDBQT, reused verbatim)
  pocket boxing  (p2rank /predict            — binding box on BOTH structures)
  receptor prep  (target-prep /prepare       — receptor PDBQT + grid per box)
  docking        (gnina CLI                  — dock the drug vs WT and vs mutant)
  fingerprints   (prolif-runner /interaction-map — interaction set on both poses)

then DIFFS the two runs:
  ddg_bind_kcal_mol  = mut_affinity - wt_affinity   (positive => mutant binds weaker)
  fp_delta_tanimoto  = Tanimoto(WT ifp, mut ifp)    (1.0 => identical contacts)

Lifecycle MIRRORS containers/esmfold/scripts/fold.py and
containers/p2rank/scripts/pocket_proximity.py and gnina/dock_batch.py:
  env config (require_env) -> connect_db (psycopg2, %s) -> fetch inputs
  (variant_calc_jobs row by cr_name=JOB_NAME + variant_resolutions row + WT/mut
  PDBs from Garage S3 via boto3) -> compute (drug->ligand, box, dock x2,
  fingerprint, diff) -> INSERT INTO staging (genome_calc) -> _jlog
  worker_start/progress/batch_complete. On hard failure it writes a frozen E_*
  code (core Appendix A) to variant_calc_jobs.error_output and exits nonzero —
  never a silent drop. Per-drug failures are isolated into `failures[]`.

============================  KEY INTEGRATION GAP  ============================
The MUTANT structure (esmfold/{rid}/mut.pdb) is produced ONLY by the `esmfold`
calc (role B / GEN-20). AlphaFold supplies WT only. Therefore an `esmfold` calc
MUST have run for this resolution_id BEFORE pgx_docking runs, OR the controller
(GEN-12) must mint an `esmfold` GenomeJob as a `parentJob` dependency and gate
pgx_docking on it via `areDependenciesReady` (core §5.2 / workers TDD §7.4).
If the mutant PDB is absent this worker fails fast with E_STRUCTURE_FOLD_FAILED
(per workers TDD §7.2 step 1) rather than silently docking against WT twice. The
calc-ordering wiring may not be in place yet — see the report for GEN-12.
==============================================================================
"""

import io
import json
import os
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

import urllib.request

# ── Frozen typed error codes (core Appendix A) ────────────────────────────────
# Mutant structure missing is a calc-ordering/dependency failure. Workers TDD
# §7.2 step 1 names E_STRUCTURE_FOLD_FAILED for "mutant must exist -> if absent";
# we conform to that rather than inventing a code, and surface the esmfold-before-
# pgx ordering dependency loudly in the message + report.
E_STRUCTURE_FOLD_FAILED = "E_STRUCTURE_FOLD_FAILED"
E_RESOLVE_UPSTREAM = "E_RESOLVE_UPSTREAM"
E_NO_LIGAND_STRUCTURE = "E_NO_LIGAND_STRUCTURE"   # per-drug: name/SMILES unresolvable
E_PARAMS_INVALID = "E_PARAMS_INVALID"

# ── S3 layout (core §5.5) ─────────────────────────────────────────────────────
BUCKET_STRUCTURES = "khemeia-structures"     # resolve/{rid}/structure.pdb, esmfold/{rid}/mut.pdb
BUCKET_POSES = "khemeia-poses"               # pgx_docking/{rid}/{drug}/{wt,mut}_pose.pdbqt
BUCKET_FINGERPRINTS = "khemeia-fingerprints" # pgx_docking/{rid}/{drug}/{wt,mut}_ifp.json + delta.json

GNINA_BIN = "/usr/local/bin/gnina"

# ── Reused sidecars (deploy/*.yaml ClusterIP Services on port 80 -> :8000) ────
DEFAULT_LIBRARY_PREP_URL = "http://library-prep.chem.svc.cluster.local"
DEFAULT_TARGET_PREP_URL = "http://target-prep.chem.svc.cluster.local"
DEFAULT_P2RANK_URL = "http://p2rank.chem.svc.cluster.local"
DEFAULT_PROLIF_URL = "http://prolif-runner.chem.svc.cluster.local"

# Bundled curated PGx drug-name -> SMILES table (workers TDD §7.3 / R-W6). Ships
# in the image at data/pgx_drug_smiles.csv; a bare drug name resolves offline and
# deterministically before any PubChem fallback.
DRUG_TABLE_PATH = os.environ.get(
    "PGX_DRUG_TABLE", "/scripts/data/pgx_drug_smiles.csv"
)
# PubChem name->SMILES fallback (cached) for drugs outside the curated table.
PUBCHEM_BASE = "https://pubchem.ncbi.nlm.nih.gov/rest/pug"

DEFAULT_EXHAUSTIVENESS = 32
DEFAULT_BOX_SIZE = 22.5  # Angstrom edge when p2rank gives a center but no extent.


def _jlog(event: str, **kwargs) -> None:
    """Emit a structured JSON metric line for Alloy/Loki (mirrors fold.py).

    The pgx-docking plugin parses `ddg_bind_kcal_mol` (reduce max) and `tanimoto`
    (reduce min) off these `metric:` lines (workers TDD §7.8).
    """
    payload = {"event": event, "ts": _time.time(), **kwargs}
    print("metric: " + json.dumps(payload, separators=(",", ":")), flush=True)


class PgxError(Exception):
    """Carries a frozen E_* code so callers map failures to the contract."""

    def __init__(self, code: str, message: str):
        super().__init__(message)
        self.code = code
        self.message = message


class DrugError(Exception):
    """Per-drug failure (isolated into failures[]; does NOT fail the calc)."""

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
    variant_calc_jobs. RESOLUTION_ID / GROUP_NAME / VARIANT_KEY / DRUGS / ENGINE /
    EXHAUSTIVENESS / POCKET_STRATEGY arrive as spec-derived UPPERCASE env hints;
    the variant_calc_jobs.params JSONB is the source of truth and backfills them.
    """
    return {
        "job_name": require_env("JOB_NAME"),
        # Spec-derived hints (backfilled from the calc-job row / params).
        "resolution_id": os.environ.get("RESOLUTION_ID", ""),
        "group_name": os.environ.get("GROUP_NAME", ""),
        "variant_key": os.environ.get("VARIANT_KEY", ""),
        "calculation": os.environ.get("CALCULATION", "pgx_docking"),
        "engine": os.environ.get("ENGINE", "gnina"),        # gnina (default) | vina | diffdock
        "exhaustiveness": int(os.environ.get("EXHAUSTIVENESS", str(DEFAULT_EXHAUSTIVENESS))),
        "pocket_strategy": os.environ.get("POCKET_STRATEGY", "p2rank"),
        # Reused sidecar endpoints.
        "library_prep_url": os.environ.get("LIBRARY_PREP_URL", DEFAULT_LIBRARY_PREP_URL).rstrip("/"),
        "target_prep_url": os.environ.get("TARGET_PREP_URL", DEFAULT_TARGET_PREP_URL).rstrip("/"),
        "p2rank_url": os.environ.get("P2RANK_URL", DEFAULT_P2RANK_URL).rstrip("/"),
        "prolif_url": os.environ.get("PROLIF_URL", DEFAULT_PROLIF_URL).rstrip("/"),
        "sidecar_timeout_s": int(os.environ.get("SIDECAR_TIMEOUT_S", "360")),
        "pubchem_enabled": os.environ.get("PUBCHEM_ENABLED", "true") == "true",
        # Postgres (same env names as the docking / esmfold workers).
        "pg_host": os.environ.get("POSTGRES_HOST", "localhost"),
        "pg_port": int(os.environ.get("POSTGRES_PORT", "5432")),
        "pg_user": os.environ.get("POSTGRES_USER", "root"),
        "pg_password": require_env("POSTGRES_PASSWORD"),
        "pg_db": os.environ.get("POSTGRES_DB", "khemeia"),
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


def _as_dict(params):
    if isinstance(params, dict):
        return params
    if isinstance(params, str):
        try:
            return json.loads(params)
        except json.JSONDecodeError:
            return {}
    if params is not None:
        try:
            return json.loads(params.decode("utf-8"))
        except Exception:
            return {}
    return {}


# ── Input resolution ──────────────────────────────────────────────────────────

def fetch_calc_job(cursor, job_name: str):
    """Read this worker's own row via cr_name = JOB_NAME (the frozen contract)."""
    cursor.execute(
        "SELECT group_name, variant_key, calculation, resolution_id, params "
        "FROM variant_calc_jobs WHERE cr_name = %s",
        (job_name,),
    )
    return cursor.fetchone()


def fetch_resolution(cursor, resolution_id: str) -> dict:
    """Fetch the ResolvedVariant fields pgx_docking needs.

    We need residue_index (for context/logging) and the WT structure location
    (structure_bucket/_key) recorded by the resolve stage; the mutant key is
    always the canonical ESMFold mut key (core §5.5).
    """
    cursor.execute(
        "SELECT residue_index, wild_type_aa, mutant_aa, structure_bucket, "
        "structure_key, resolved FROM variant_resolutions WHERE resolution_id = %s",
        (resolution_id,),
    )
    row = cursor.fetchone()
    if row is None:
        raise PgxError(
            E_RESOLVE_UPSTREAM,
            f"resolution_id '{resolution_id}' not found in variant_resolutions",
        )
    residue_index, wt_aa, mut_aa, bucket, key, resolved = row
    doc = _as_dict(resolved)
    return {
        "residue_index": int(residue_index),
        "wild_type_aa": (wt_aa or "").strip(),
        "mutant_aa": (mut_aa or "").strip(),
        # WT: prefer the recorded resolve location; fall back to the canonical key.
        "wt_bucket": bucket or BUCKET_STRUCTURES,
        "wt_key": key or f"resolve/{resolution_id}/structure.pdb",
        # Mutant: ALWAYS the ESMFold mut model (AlphaFold has no mutant models).
        "mut_bucket": BUCKET_STRUCTURES,
        "mut_key": f"esmfold/{resolution_id}/mut.pdb",
        "structure_source": doc.get("structure_source"),
    }


def s3_get_text(s3, bucket: str, key: str):
    """Return object text for a key, or None if it is absent / unreadable."""
    if s3 is None:
        raise PgxError(E_RESOLVE_UPSTREAM, "S3 disabled but structures are required")
    try:
        resp = s3.get_object(Bucket=bucket, Key=key)
        return resp["Body"].read().decode("utf-8")
    except Exception:
        return None


def s3_put_text(s3, bucket: str, key: str, text: str, content_type: str) -> None:
    if s3 is None:
        return  # artifacts are best-effort; the headline still lands in staging.
    s3.put_object(
        Bucket=bucket, Key=key,
        Body=text.encode("utf-8") if isinstance(text, str) else text,
        ContentType=content_type,
    )


def load_structures(s3, res: dict, resolution_id: str):
    """Fetch the WT and MUTANT PDBs. Missing mutant => the calc-ordering failure.

    This is the dependency gate: the mutant structure exists only if an `esmfold`
    calc ran for this resolution (workers TDD §7.4). Failing here (not silently
    docking WT vs WT) is what makes the WT-vs-mut diff meaningful.
    """
    wt_pdb = s3_get_text(s3, res["wt_bucket"], res["wt_key"])
    if not wt_pdb:
        raise PgxError(
            E_RESOLVE_UPSTREAM,
            f"WT structure missing at s3://{res['wt_bucket']}/{res['wt_key']} "
            "(resolve stage did not produce it)",
        )
    mut_pdb = s3_get_text(s3, res["mut_bucket"], res["mut_key"])
    if not mut_pdb:
        raise PgxError(
            E_STRUCTURE_FOLD_FAILED,
            f"MUTANT structure missing at s3://{res['mut_bucket']}/{res['mut_key']}. "
            "pgx_docking REQUIRES the ESMFold mutant model; an `esmfold` calc must "
            f"run for resolution {resolution_id} BEFORE pgx_docking (controller "
            "GEN-12 must mint/await it as a parentJob — calc-ordering dependency).",
        )
    return wt_pdb, mut_pdb


# ── HTTP helper for the reused sidecars ───────────────────────────────────────

def _post_json(url: str, body: dict, timeout_s: int) -> dict:
    data = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(
        url, data=data, headers={"Content-Type": "application/json"}, method="POST"
    )
    with urllib.request.urlopen(req, timeout=timeout_s) as resp:
        return json.loads(resp.read().decode("utf-8"))


# ── Drug -> dockable ligand (workers TDD §7.3) ────────────────────────────────

_AA = set("ACDEFGHIKLMNPQRSTVWY")


def _looks_like_smiles(token: str) -> bool:
    """Heuristic: a SMILES has bond/ring/branch glyphs a drug NAME never has.

    Resolution order (workers TDD §7.3): SMILES -> name table -> PubChem. A token
    with any of these characters is treated as SMILES and sent straight to
    library-prep; otherwise it is a drug name to look up.
    """
    if any(c in token for c in "()[]=#@+/\\1234567890"):
        # Guard: a few drug names contain digits but no SMILES glyphs; require a
        # structural glyph, not just a digit, to call it SMILES.
        if any(c in token for c in "()[]=#@+/\\"):
            return True
        # digit-only differences (e.g. "6-mercaptopurine") are names, not SMILES.
        return False
    return False


def load_drug_table(path: str) -> dict:
    """Load the curated PGx drug-name -> SMILES table (CSV: name,smiles[,...]).

    Keys are lowercased + stripped for case-insensitive name matching. Aliases on
    the same row (extra columns after smiles) are also indexed to the same SMILES.
    Missing file is non-fatal — name resolution then relies on PubChem only.
    """
    table: dict[str, str] = {}
    if not os.path.exists(path):
        print(f"WARNING: drug table {path} not found; relying on PubChem fallback",
              flush=True)
        return table
    with open(path) as fh:
        for raw in fh:
            line = raw.strip()
            if not line or line.startswith("#"):
                continue
            parts = [p.strip() for p in line.split(",")]
            if len(parts) < 2 or not parts[0] or not parts[1]:
                continue
            name, smiles = parts[0], parts[1]
            if name.lower() == "name" and smiles.lower() == "smiles":
                continue  # header
            table[name.lower()] = smiles
            for alias in parts[2:]:
                if alias:
                    table[alias.lower()] = smiles
    return table


def pubchem_name_to_smiles(name: str, timeout_s: int):
    """Cached-elsewhere PubChem name->canonical SMILES fallback. None on miss."""
    url = (
        f"{PUBCHEM_BASE}/compound/name/{urllib.request.quote(name)}"
        "/property/CanonicalSMILES/JSON"
    )
    try:
        with urllib.request.urlopen(url, timeout=timeout_s) as resp:
            doc = json.loads(resp.read().decode("utf-8"))
        props = doc.get("PropertyTable", {}).get("Properties", [])
        if props and props[0].get("CanonicalSMILES"):
            return props[0]["CanonicalSMILES"]
    except Exception as exc:
        print(f"WARNING: PubChem lookup failed for {name!r}: {exc}", flush=True)
    return None


def resolve_drug_to_smiles(drug: str, drug_table: dict, cfg: dict):
    """Resolve a drug token (SMILES | name) to a SMILES string.

    Returns (smiles, source) where source ∈ {smiles, drug_table, pubchem}. Raises
    DrugError(E_NO_LIGAND_STRUCTURE) if a name cannot be resolved (per-drug; the
    batch continues).
    """
    token = drug.strip()
    if _looks_like_smiles(token):
        return token, "smiles"
    # Drug name -> curated table (offline, deterministic) -> PubChem fallback.
    hit = drug_table.get(token.lower())
    if hit:
        return hit, "drug_table"
    if cfg["pubchem_enabled"]:
        smiles = pubchem_name_to_smiles(token, cfg["sidecar_timeout_s"])
        if smiles:
            return smiles, "pubchem"
    raise DrugError(
        E_NO_LIGAND_STRUCTURE,
        f"drug '{drug}' could not be resolved to a SMILES "
        "(not SMILES, not in the curated PGx table, PubChem miss)",
    )


def standardize_to_pdbqt(smiles: str, cfg: dict) -> str:
    """Reuse library-prep /standardize to get a charged, 3D, Meeko PDBQT ligand.

    This is the SAME standardization path the SBDD pipeline uses (RDKit
    MolStandardize + ETKDG conformer + Meeko) — pgx_docking reuses it rather than
    re-implementing ligand prep (workers TDD §7.3). Raises DrugError on failure.
    """
    url = f"{cfg['library_prep_url']}/standardize"
    body = {
        "smiles_list": [smiles],
        # Skip drug-likeness filters: PGx drugs are dosed compounds, not a screen.
        "filters": {"lipinski": False, "veber": False, "pains": False,
                    "brenk": False, "reos": False},
        "generate_3d": True,
    }
    try:
        doc = _post_json(url, body, cfg["sidecar_timeout_s"])
    except Exception as exc:
        raise DrugError(
            E_NO_LIGAND_STRUCTURE,
            f"library-prep /standardize call failed for {smiles!r}: {exc}",
        )
    compounds = doc.get("compounds") or []
    if not compounds:
        raise DrugError(E_NO_LIGAND_STRUCTURE, f"library-prep returned no compound for {smiles!r}")
    rec = compounds[0]
    pdbqt = rec.get("pdbqt_data")
    if not pdbqt:
        err = rec.get("error") or "no pdbqt_data (conformer/Meeko failure)"
        raise DrugError(
            E_NO_LIGAND_STRUCTURE,
            f"library-prep could not produce a dockable ligand for {smiles!r}: {err}",
        )
    return pdbqt


# ── Pocket boxing (reuse p2rank) + receptor prep (reuse target-prep) ──────────

def box_from_p2rank(pdb_text: str, cfg: dict):
    """POST a structure to p2rank /predict; return the top pocket's docking box.

    Reuses the running p2rank sidecar (no duplicated pocket detection). Returns
    (center [x,y,z], size [sx,sy,sz]) using the top-ranked pocket center. p2rank
    gives a center but not an extent, so we use a fixed cubic box (DEFAULT_BOX_SIZE)
    — the standard approach for pocket-centered docking.
    """
    url = f"{cfg['p2rank_url']}/predict"
    try:
        doc = _post_json(url, {"pdb_data": pdb_text}, cfg["sidecar_timeout_s"])
    except Exception as exc:
        raise PgxError(E_RESOLVE_UPSTREAM, f"p2rank /predict failed: {exc}")
    if isinstance(doc, dict) and doc.get("error"):
        raise PgxError(E_RESOLVE_UPSTREAM, f"p2rank predict error: {doc['error']}")
    pockets = doc.get("pockets", []) if isinstance(doc, dict) else []
    if not pockets:
        raise PgxError(
            E_RESOLVE_UPSTREAM,
            "p2rank found no pocket to box for docking (cannot define a grid)",
        )
    top = min(pockets, key=lambda p: int(p.get("rank", 9999)))
    center = top.get("center") or [0.0, 0.0, 0.0]
    cx, cy, cz = float(center[0]), float(center[1]), float(center[2])
    size = [DEFAULT_BOX_SIZE, DEFAULT_BOX_SIZE, DEFAULT_BOX_SIZE]
    return [cx, cy, cz], size


def prep_receptor(pdb_text: str, center, size, cfg: dict) -> str:
    """Reuse target-prep /prepare (custom-box mode) -> cleaned receptor PDBQT.

    target-prep returns a cleaned receptor PDB; gnina accepts PDB receptors, so we
    use the cleaned receptor PDB directly as the receptor file. (Meeko receptor
    PDBQT is not exposed by /prepare; gnina reads PDB receptors natively.)
    """
    url = f"{cfg['target_prep_url']}/prepare"
    body = {"pdb_data": pdb_text, "mode": "custom-box",
            "center": center, "size": size}
    try:
        doc = _post_json(url, body, cfg["sidecar_timeout_s"])
    except Exception as exc:
        raise PgxError(E_RESOLVE_UPSTREAM, f"target-prep /prepare failed: {exc}")
    if doc.get("error"):
        raise PgxError(E_RESOLVE_UPSTREAM, f"target-prep error: {doc['error']}")
    receptor_pdb = doc.get("receptor_pdb")
    if not receptor_pdb:
        raise PgxError(E_RESOLVE_UPSTREAM, "target-prep returned no receptor_pdb")
    return receptor_pdb


# ── Docking (reuse the gnina CLI in this image) ───────────────────────────────

def run_gnina_dock(receptor_text: str, ligand_pdbqt: str, center, size,
                   exhaustiveness: int):
    """Dock one ligand against one receptor with the in-image gnina CLI.

    Returns (best_affinity_kcal_mol, docked_pose_pdbqt). Mirrors dock_batch.py's
    invocation (CNN rescore, mode-1 affinity from the log). Raises DrugError on a
    docking failure so the per-drug isolation in the orchestrator catches it.
    """
    tmpdir = tempfile.mkdtemp(prefix="pgx_dock_")
    rec_path = os.path.join(tmpdir, "receptor.pdb")
    lig_path = os.path.join(tmpdir, "ligand.pdbqt")
    out_path = os.path.join(tmpdir, "docked.pdbqt")
    log_path = os.path.join(tmpdir, "docked.log")
    try:
        with open(rec_path, "w") as fh:
            fh.write(receptor_text)
        with open(lig_path, "w") as fh:
            fh.write(ligand_pdbqt)
        cx, cy, cz = center
        sx, sy, sz = size
        cmd = [
            GNINA_BIN,
            "--receptor", rec_path,
            "--ligand", lig_path,
            "--center_x", str(cx), "--center_y", str(cy), "--center_z", str(cz),
            "--size_x", str(sx), "--size_y", str(sy), "--size_z", str(sz),
            "--exhaustiveness", str(exhaustiveness),
            "--num_modes", "9",
            "--out", out_path, "--log", log_path,
            "--cnn_scoring", "rescore",
        ]
        proc = subprocess.Popen(cmd, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, text=True)
        try:
            out, err = proc.communicate(timeout=1800)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.communicate()
            raise DrugError(E_NO_LIGAND_STRUCTURE, "gnina docking timed out (1800s)")
        if proc.returncode != 0:
            raise DrugError(
                E_NO_LIGAND_STRUCTURE,
                f"gnina exited {proc.returncode}: {(err or '')[:200]}",
            )
        affinity = parse_gnina_affinity(log_path)
        if affinity is None:
            raise DrugError(E_NO_LIGAND_STRUCTURE, "gnina produced no parseable affinity")
        pose = ""
        if os.path.exists(out_path):
            with open(out_path) as fh:
                pose = fh.read()
        return affinity, pose
    finally:
        for f in (rec_path, lig_path, out_path, log_path):
            if os.path.exists(f):
                os.unlink(f)
        if os.path.isdir(tmpdir):
            os.rmdir(tmpdir)


def parse_gnina_affinity(log_path: str):
    """Parse mode-1 affinity from a gnina log (CNN table or Vina fallback)."""
    import re
    mode_re = re.compile(r"^\s+(\d+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)")
    vina_re = re.compile(r"^\s+(\d+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)")
    if not os.path.exists(log_path):
        return None
    with open(log_path) as fh:
        for line in fh:
            m = mode_re.match(line)
            if m and m.group(1) == "1":
                return float(m.group(2))
            m = vina_re.match(line)
            if m and m.group(1) == "1":
                return float(m.group(2))
    return None


def first_pose_pdbqt(multi_pose_pdbqt: str) -> str:
    """Extract MODEL 1 (the best pose) from a multi-MODEL gnina output PDBQT.

    prolif-runner already takes MODEL 1 from a ligand PDBQT, but we slice it here
    too so the persisted pose artifact and the fingerprint input agree.
    """
    has_models = any(l.lstrip().startswith("MODEL") for l in multi_pose_pdbqt.splitlines())
    if not has_models:
        return multi_pose_pdbqt
    lines = []
    capturing = False
    for line in multi_pose_pdbqt.splitlines():
        s = line.strip()
        if s.startswith("MODEL"):
            parts = s.split()
            if len(parts) >= 2 and parts[1] != "1":
                break
            capturing = True
            lines.append(line)
            continue
        if s.startswith("ENDMDL"):
            lines.append(line)
            break
        if capturing:
            lines.append(line)
    return "\n".join(lines) + "\n"


# ── Interaction fingerprint + Tanimoto delta (reuse prolif-runner) ────────────

def interaction_set(receptor_text: str, ligand_pose_pdbqt: str, cfg: dict, tag: str):
    """Call prolif-runner /interaction-map; return a set of (residue, type) tuples.

    The receptor PDB is accepted by prolif-runner's PDBQT->PDB receptor path
    (it keeps ATOM lines), so we pass the cleaned receptor PDB as receptor_pdbqt.
    Returns a set; an empty set is a valid result (no detected contacts).
    """
    url = f"{cfg['prolif_url']}/interaction-map"
    body = {"receptor_pdbqt": receptor_text,
            "ligand_pdbqt": ligand_pose_pdbqt,
            "compound_id": tag}
    try:
        doc = _post_json(url, body, cfg["sidecar_timeout_s"])
    except Exception as exc:
        # Fingerprinting failure is per-drug (the affinities still diff).
        raise DrugError(E_NO_LIGAND_STRUCTURE,
                        f"prolif-runner /interaction-map failed ({tag}): {exc}")
    if doc.get("error"):
        raise DrugError(E_NO_LIGAND_STRUCTURE,
                        f"prolif-runner error ({tag}): {doc['error']}")
    contacts = set()
    for it in doc.get("interactions", []):
        contacts.add((str(it.get("residue")), str(it.get("type"))))
    return contacts


def tanimoto(wt_set: set, mut_set: set) -> float:
    """Tanimoto similarity of two interaction sets. 1.0 when both empty (identical).

    fp_delta_tanimoto in [0,1]: 1.0 => identical interaction fingerprints (mutation
    did not perturb contacts), 0.0 => fully disjoint.
    """
    if not wt_set and not mut_set:
        return 1.0
    inter = len(wt_set & mut_set)
    union = len(wt_set | mut_set)
    return round(inter / union, 4) if union else 1.0


def contacts_to_list(s: set):
    return [{"residue": r, "type": t} for (r, t) in sorted(s)]


# ── Per-drug docking + diff ───────────────────────────────────────────────────

def dock_one_drug(drug, drug_table, cfg, s3, resolution_id,
                  wt_pdb, mut_pdb, wt_box, mut_box):
    """Resolve, dock (WT+mut), fingerprint, and diff a single drug.

    Returns a per-drug result dict. Raises DrugError for per-drug isolation —
    callers record it in failures[] and continue with the next drug.
    """
    smiles, drug_source = resolve_drug_to_smiles(drug, drug_table, cfg)
    ligand_pdbqt = standardize_to_pdbqt(smiles, cfg)

    wt_center, wt_size = wt_box
    mut_center, mut_size = mut_box

    # Receptor prep per structure (the pocket — and thus the box — can shift).
    wt_receptor = prep_receptor(wt_pdb, wt_center, wt_size, cfg)
    mut_receptor = prep_receptor(mut_pdb, mut_center, mut_size, cfg)

    wt_aff, wt_pose = run_gnina_dock(wt_receptor, ligand_pdbqt, wt_center, wt_size,
                                     cfg["exhaustiveness"])
    mut_aff, mut_pose = run_gnina_dock(mut_receptor, ligand_pdbqt, mut_center, mut_size,
                                       cfg["exhaustiveness"])

    wt_pose1 = first_pose_pdbqt(wt_pose) if wt_pose else ""
    mut_pose1 = first_pose_pdbqt(mut_pose) if mut_pose else ""

    # Persist poses (best-effort) to khemeia-poses (core §5.5).
    drug_slug = _slug(drug)
    wt_pose_key = f"pgx_docking/{resolution_id}/{drug_slug}/wt_pose.pdbqt"
    mut_pose_key = f"pgx_docking/{resolution_id}/{drug_slug}/mut_pose.pdbqt"
    if wt_pose1:
        s3_put_text(s3, BUCKET_POSES, wt_pose_key, wt_pose1, "chemical/x-pdbqt")
    if mut_pose1:
        s3_put_text(s3, BUCKET_POSES, mut_pose_key, mut_pose1, "chemical/x-pdbqt")

    # Interaction fingerprints on both poses + Tanimoto delta.
    wt_ifp = interaction_set(wt_receptor, wt_pose1, cfg, f"{drug_slug}-wt") if wt_pose1 else set()
    mut_ifp = interaction_set(mut_receptor, mut_pose1, cfg, f"{drug_slug}-mut") if mut_pose1 else set()
    tani = tanimoto(wt_ifp, mut_ifp)
    lost = contacts_to_list(wt_ifp - mut_ifp)     # present in WT, gone in mutant
    gained = contacts_to_list(mut_ifp - wt_ifp)   # new in mutant

    # Persist fingerprints + delta (best-effort) to khemeia-fingerprints.
    wt_ifp_key = f"pgx_docking/{resolution_id}/{drug_slug}/wt_ifp.json"
    mut_ifp_key = f"pgx_docking/{resolution_id}/{drug_slug}/mut_ifp.json"
    delta_key = f"pgx_docking/{resolution_id}/{drug_slug}/delta.json"
    s3_put_text(s3, BUCKET_FINGERPRINTS, wt_ifp_key,
                json.dumps(contacts_to_list(wt_ifp)), "application/json")
    s3_put_text(s3, BUCKET_FINGERPRINTS, mut_ifp_key,
                json.dumps(contacts_to_list(mut_ifp)), "application/json")
    delta_doc = {"tanimoto": tani, "lost_contacts": lost, "gained_contacts": gained}
    s3_put_text(s3, BUCKET_FINGERPRINTS, delta_key, json.dumps(delta_doc),
                "application/json")

    # Diff: ddg_bind = mut_affinity - wt_affinity (positive => mutant binds weaker).
    ddg_bind = round(mut_aff - wt_aff, 3)

    return {
        "drug": drug,
        "resolved_smiles": smiles,
        "drug_source": drug_source,
        "engine": cfg["engine"],
        "wt_affinity_kcal_mol": round(wt_aff, 3),
        "mut_affinity_kcal_mol": round(mut_aff, 3),
        "ddg_bind_kcal_mol": ddg_bind,
        "fingerprint_delta": {
            "tanimoto": tani,
            "lost_contacts": lost,
            "gained_contacts": gained,
        },
        "wt_pose_key": wt_pose_key if wt_pose1 else None,
        "mut_pose_key": mut_pose_key if mut_pose1 else None,
        "wt_ifp_key": wt_ifp_key,
        "mut_ifp_key": mut_ifp_key,
        "delta_key": delta_key,
        "interpretation": _interpret_drug(drug, ddg_bind, tani),
    }


def _slug(text: str) -> str:
    keep = [c if (c.isalnum() or c in "-_") else "-" for c in text.strip().lower()]
    s = "".join(keep).strip("-")
    return s or "drug"


def _interpret_drug(drug, ddg_bind, tani) -> str:
    if ddg_bind > 0.5:
        binding = f"{drug} binds the mutant WEAKER (ddG_bind=+{ddg_bind} kcal/mol)"
    elif ddg_bind < -0.5:
        binding = f"{drug} binds the mutant STRONGER (ddG_bind={ddg_bind} kcal/mol)"
    else:
        binding = f"{drug} binding affinity largely unchanged (ddG_bind={ddg_bind} kcal/mol)"
    if tani < 0.5:
        contacts = "with a substantial interaction-fingerprint shift"
    elif tani < 0.85:
        contacts = "with a moderate interaction-fingerprint shift"
    else:
        contacts = "with a near-identical interaction fingerprint"
    return f"{binding} {contacts} (Tanimoto={tani})."


# ── Orchestration ─────────────────────────────────────────────────────────────

def parse_drugs(cfg: dict, params: dict):
    """Extract the explicit drug list (core R4: no implicit panel).

    Env DRUGS may be JSON list or comma-separated; params.pgx_docking.drugs wins.
    Empty => E_PARAMS_INVALID (a PGx calc without drugs is meaningless and would
    blow the cost bound).
    """
    drugs = None
    pgx = params.get("pgx_docking", params) if isinstance(params, dict) else {}
    if isinstance(pgx, dict) and pgx.get("drugs"):
        drugs = pgx["drugs"]
    if drugs is None:
        env = os.environ.get("DRUGS", "").strip()
        if env:
            try:
                drugs = json.loads(env)
            except json.JSONDecodeError:
                drugs = [d.strip() for d in env.split(",") if d.strip()]
    if not drugs:
        raise PgxError(
            E_PARAMS_INVALID,
            "pgx_docking requires an explicit non-empty `drugs` list "
            "(core R4: no implicit full panel)",
        )
    if isinstance(drugs, str):
        drugs = [drugs]
    return [str(d) for d in drugs if str(d).strip()]


def run_pgx_docking(conn, cursor, s3, cfg, res, resolution_id, drugs) -> None:
    t0 = _time.time()
    _jlog("worker_start", job=cfg["job_name"], calculation="pgx_docking",
          resolution_id=resolution_id, engine=cfg["engine"], n_drugs=len(drugs),
          exhaustiveness=cfg["exhaustiveness"], residue_index=res["residue_index"])

    if cfg["engine"] != "gnina":
        # gnina is the in-image default engine. vina/diffdock are documented
        # params values (workers TDD §7.1) but are separate images; this single
        # orchestrator pod ships the gnina CLI only.
        raise PgxError(
            E_PARAMS_INVALID,
            f"engine '{cfg['engine']}' not runnable from the gnina orchestrator pod "
            "(only the in-image gnina engine is wired; vina/diffdock are separate images)",
        )

    wt_pdb, mut_pdb = load_structures(s3, res, resolution_id)
    _jlog("progress", job=cfg["job_name"], resolution_id=resolution_id,
          stage="structures_loaded",
          structure_source=res.get("structure_source") or "resolve")

    # Box the pocket on BOTH structures (the pocket may shift on mutation).
    wt_box = box_from_p2rank(wt_pdb, cfg)
    mut_box = box_from_p2rank(mut_pdb, cfg)
    _jlog("progress", job=cfg["job_name"], resolution_id=resolution_id,
          stage="pockets_boxed", wt_center=wt_box[0], mut_center=mut_box[0])

    drug_table = load_drug_table(DRUG_TABLE_PATH)

    per_drug = []
    failures = []
    headline_ddg = None     # max |ddg_bind| across drugs (core §6(a))
    headline_tani = None    # min tanimoto across drugs (core §6(a))
    worst_conf = None       # lowest fingerprint similarity drives confidence

    for i, drug in enumerate(drugs, start=1):
        try:
            result = dock_one_drug(drug, drug_table, cfg, s3, resolution_id,
                                   wt_pdb, mut_pdb, wt_box, mut_box)
            per_drug.append(result)
            ddg = result["ddg_bind_kcal_mol"]
            tani = result["fingerprint_delta"]["tanimoto"]
            if headline_ddg is None or abs(ddg) > abs(headline_ddg):
                headline_ddg = ddg
            if headline_tani is None or tani < headline_tani:
                headline_tani = tani
            if worst_conf is None or tani < worst_conf:
                worst_conf = tani
            _jlog("progress", job=cfg["job_name"], resolution_id=resolution_id,
                  stage="drug_complete", processed=i, total=len(drugs), drug=drug,
                  ddg_bind_kcal_mol=ddg, tanimoto=tani,
                  wt_affinity=result["wt_affinity_kcal_mol"],
                  mut_affinity=result["mut_affinity_kcal_mol"])
        except DrugError as exc:
            failures.append({"drug": drug, "code": exc.code, "error": exc.message})
            print(f"WARNING: drug '{drug}' failed [{exc.code}]: {exc.message}", flush=True)
            _jlog("progress", job=cfg["job_name"], resolution_id=resolution_id,
                  stage="drug_failed", processed=i, total=len(drugs), drug=drug,
                  code=exc.code)

    # All drugs failed -> the calc Fails (workers TDD §7.7).
    if not per_drug:
        raise PgxError(
            E_NO_LIGAND_STRUCTURE,
            f"all {len(drugs)} drug(s) failed for resolution {resolution_id}: "
            + "; ".join(f"{f['drug']}({f['code']})" for f in failures),
        )

    confidence = round(worst_conf, 4) if worst_conf is not None else None

    # §6(a) payload (core §6(a) / workers TDD §7.5 shape).
    payload = {
        "engine": cfg["engine"],
        "pocket_strategy": cfg["pocket_strategy"],
        "exhaustiveness": cfg["exhaustiveness"],
        "structure_source": res.get("structure_source") or "resolve",
        "wt_structure_key": res["wt_key"],
        "mut_structure_key": res["mut_key"],
        "per_drug": per_drug,
        "failures": failures,
        "interpretation": _interpret_batch(per_drug, headline_ddg, headline_tani),
    }

    # §5.6 NESTED staging envelope. headline.* -> typed columns (GEN-22 maps
    # ddg_bind_kcal_mol / fp_delta_tanimoto). headline ddg = max|ddg_bind|,
    # tanimoto = min across drugs (core §6(a)).
    envelope = {
        "group_name": cfg["group_name"],
        "variant_key": cfg["variant_key"],
        "calculation": "pgx_docking",
        "resolution_id": resolution_id,
        "structure_source": res.get("structure_source") or "resolve",
        "headline": {
            "ddg_bind_kcal_mol": headline_ddg,
            "fp_delta_tanimoto": headline_tani,
            "confidence": confidence,
        },
        "payload": payload,
        "artifact_keys": {
            "wt_poses": [d["wt_pose_key"] for d in per_drug if d.get("wt_pose_key")],
            "mut_poses": [d["mut_pose_key"] for d in per_drug if d.get("mut_pose_key")],
            "fingerprints": [d["delta_key"] for d in per_drug if d.get("delta_key")],
        },
    }

    cursor.execute(
        "INSERT INTO staging (job_type, payload) VALUES ('genome_calc', %s)",
        (json.dumps(envelope),),
    )
    conn.commit()

    # `metric:` line carrying the values the pgx-docking plugin parses (workers
    # TDD §7.8: ddg_bind_kcal_mol reduce=max, tanimoto reduce=min).
    _jlog("batch_complete", job=cfg["job_name"], resolution_id=resolution_id,
          n_drugs=len(drugs), n_completed=len(per_drug), n_failed=len(failures),
          ddg_bind_kcal_mol=headline_ddg, tanimoto=headline_tani,
          confidence=confidence, elapsed_s=round(_time.time() - t0, 1))


def _interpret_batch(per_drug, headline_ddg, headline_tani) -> str:
    if not per_drug:
        return "No drug produced a WT-vs-mutant docking result."
    n = len(per_drug)
    return (
        f"Docked {n} drug(s) against WT vs mutant. Largest binding shift "
        f"ddG_bind={headline_ddg} kcal/mol; smallest interaction-fingerprint "
        f"similarity Tanimoto={headline_tani} (lower => the mutation reshapes the "
        "drug's contacts most for that drug)."
    )


# ── Entrypoint ────────────────────────────────────────────────────────────────

def resolve_context(cfg: dict, cursor):
    """Backfill identifiers from variant_calc_jobs (cr_name=JOB_NAME is truth).

    Returns (resolution_id, params_dict). params.pgx_docking carries drugs/engine/
    exhaustiveness/pocket_strategy; env hints fill only blanks.
    """
    job = fetch_calc_job(cursor, cfg["job_name"])
    params = {}
    if job is not None:
        group_name, variant_key, calculation, resolution_id, raw_params = job
        cfg["group_name"] = cfg["group_name"] or (group_name or "")
        cfg["variant_key"] = cfg["variant_key"] or (variant_key or "")
        cfg["calculation"] = calculation or cfg["calculation"]
        resolution_id = resolution_id or cfg["resolution_id"]
        params = _as_dict(raw_params)
        pgx = params.get("pgx_docking", params) if isinstance(params, dict) else {}
        if isinstance(pgx, dict):
            if not os.environ.get("ENGINE") and pgx.get("engine"):
                cfg["engine"] = pgx["engine"]
            if not os.environ.get("EXHAUSTIVENESS") and pgx.get("exhaustiveness") is not None:
                try:
                    cfg["exhaustiveness"] = int(pgx["exhaustiveness"])
                except (TypeError, ValueError):
                    pass
            if not os.environ.get("POCKET_STRATEGY") and pgx.get("pocket_strategy"):
                cfg["pocket_strategy"] = pgx["pocket_strategy"]
    else:
        resolution_id = cfg["resolution_id"]

    if not resolution_id:
        raise PgxError(
            E_RESOLVE_UPSTREAM,
            f"no resolution_id for job '{cfg['job_name']}' "
            "(absent from variant_calc_jobs row and RESOLUTION_ID env)",
        )
    cfg["resolution_id"] = resolution_id
    return resolution_id, params


def fail(conn, cursor, cfg, code: str, message: str) -> None:
    """Record a typed failure on the owning variant_calc_jobs row and exit 1.

    Mirrors fold.py / pocket_proximity.py: every hard failure carries a frozen
    E_* code in variant_calc_jobs.error_output (core §10 / workers TDD AC #4).
    """
    print(f"FATAL [{code}]: {message}", flush=True)
    _jlog("error", job=cfg.get("job_name"), calculation="pgx_docking",
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
        f"PGx docking orchestrator starting: job={cfg['job_name']} "
        f"engine={cfg['engine']} exhaustiveness={cfg['exhaustiveness']}",
        flush=True,
    )

    conn = connect_db(cfg)
    cursor = conn.cursor()
    s3 = get_s3_client()

    try:
        resolution_id, params = resolve_context(cfg, cursor)
        drugs = parse_drugs(cfg, params)
        res = fetch_resolution(cursor, resolution_id)
        run_pgx_docking(conn, cursor, s3, cfg, res, resolution_id, drugs)
    except PgxError as exc:
        fail(conn, cursor, cfg, exc.code, exc.message)
    except Exception as exc:  # last-resort: never a silent drop
        fail(conn, cursor, cfg, E_RESOLVE_UPSTREAM, f"unexpected: {exc}")
    finally:
        cursor.close()
        conn.close()


if __name__ == "__main__":
    main()
