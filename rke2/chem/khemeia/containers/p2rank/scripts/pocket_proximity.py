#!/usr/bin/env python3
"""Pocket-proximity worker for the Khemeia genomics layer (calc `pocket_proximity`).

The LIGHTEST genome worker (workers TDD §6): given a resolved structure and a
variant residue, decide whether the residue sits in / near a druggable pocket and
report the distance to the nearest pocket. No new science, no new image — this
script ships inside the EXISTING p2rank container image and reuses two things:

  1. The p2rank Flask sidecar (`POST /predict {pdb_data}`) for pocket detection
     (containers/p2rank/app.py) — NO duplicated pocket-detection code.
  2. The residue->nearest-pocket distance math ported from the controller's
     api/handlers_pocket.go (fixed-width PDB column parsing + Euclidean `dist`
     + nearest-atom residue distance), applied to a RESIDUE instead of a ligand.

Lifecycle MIRRORS containers/esmfold/scripts/fold.py / gnina/dock_batch.py:
  env config (require_env) -> connect_db (psycopg2, %s) -> fetch inputs
  (variant_calc_jobs row by cr_name=JOB_NAME + variant_resolutions row + WT PDB
  from Garage S3 via boto3) -> compute (p2rank predict + distance math) ->
  INSERT INTO staging (genome_calc) -> _jlog worker_start/progress/batch_complete.
Failures emit a frozen E_* code (core Appendix A) onto variant_calc_jobs.error_output
and exit nonzero — never a silent drop.

The controller injects context via buildCRDJobEnv: JOB_NAME is the GenomeJob CR
name; spec fields arrive as UPPERCASE env vars (RESOLUTION_ID, GROUP_NAME,
VARIANT_KEY, DETECTOR, CUTOFF_ANG); Postgres + GARAGE_* creds are injected the
same way as for the docking / esmfold workers. The worker reads its own
variant_calc_jobs row by `WHERE cr_name = JOB_NAME` (the frozen contract) so it
recovers group_name/variant_key/resolution_id even if a spec field is absent.
"""

import io
import json
import math
import os
import sys
import time as _time

import psycopg2

try:
    import boto3
    from botocore.config import Config as BotoConfig
except ImportError:  # pragma: no cover - boto3 is always installed in the image
    boto3 = None

try:
    import urllib.request
except ImportError:  # pragma: no cover
    urllib = None

# Frozen typed error codes (core Appendix A). Pocket proximity reuses the
# resolution / WT-mismatch codes; "no pockets" is NOT an error (workers TDD §6.5).
E_RESOLVE_UPSTREAM = "E_RESOLVE_UPSTREAM"
E_WT_MISMATCH = "E_WT_MISMATCH"
E_PARAMS_INVALID = "E_PARAMS_INVALID"

# S3 layout (core §5.5). The WT structure lives in khemeia-structures; the pocket
# artifact lands in khemeia-reports (reused bucket).
BUCKET_STRUCTURES = "khemeia-structures"
BUCKET_REPORTS = "khemeia-reports"

# Default proximity cutoff (Angstrom) — ported intent from handlers_pocket.go's
# residue-contact cutoff; workers TDD §6.2 default is 6.0 A for residue->pocket.
DEFAULT_CUTOFF_ANG = 6.0

# p2rank sidecar (deploy/target-prep.yaml Service: p2rank.chem.svc.cluster.local).
DEFAULT_P2RANK_URL = "http://p2rank.chem.svc.cluster.local:8000"


def _jlog(event: str, **kwargs) -> None:
    """Emit a structured JSON metric line for Alloy/Loki ingestion.

    Identical convention to fold.py / dock_batch.py: a `metric: {json}` line.
    The pocket-proximity plugin parses `distance_ang` and `in_pocket` off these.
    """
    payload = {"event": event, "ts": _time.time(), **kwargs}
    print("metric: " + json.dumps(payload, separators=(",", ":")), flush=True)


class PocketError(Exception):
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
    variant_calc_jobs. RESOLUTION_ID / GROUP_NAME / VARIANT_KEY / DETECTOR /
    CUTOFF_ANG arrive as spec-derived UPPERCASE env vars but are treated as hints —
    the variant_calc_jobs row is the source of truth and backfills any absent.
    """
    return {
        "job_name": require_env("JOB_NAME"),
        # Spec-derived hints (may be empty; backfilled from the calc-job row).
        "resolution_id": os.environ.get("RESOLUTION_ID", ""),
        "group_name": os.environ.get("GROUP_NAME", ""),
        "variant_key": os.environ.get("VARIANT_KEY", ""),
        "calculation": os.environ.get("CALCULATION", "pocket_proximity"),
        "detector": os.environ.get("DETECTOR", "p2rank"),  # p2rank | fpocket
        "cutoff_ang": float(os.environ.get("CUTOFF_ANG", str(DEFAULT_CUTOFF_ANG))),
        "p2rank_url": os.environ.get("P2RANK_URL", DEFAULT_P2RANK_URL).rstrip("/"),
        "p2rank_timeout_s": int(os.environ.get("P2RANK_TIMEOUT_S", "360")),
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


# ── Input resolution ──────────────────────────────────────────────────────────

def fetch_calc_job(cursor, job_name: str):
    """Read this worker's own row via cr_name = JOB_NAME (the frozen contract).

    Returns (group_name, variant_key, calculation, resolution_id, params_json) or
    None. The row is authoritative for the identifiers; spec env vars are hints.
    """
    cursor.execute(
        "SELECT group_name, variant_key, calculation, resolution_id, params "
        "FROM variant_calc_jobs WHERE cr_name = %s",
        (job_name,),
    )
    return cursor.fetchone()


def fetch_resolution(cursor, resolution_id: str) -> dict:
    """Fetch the ResolvedVariant fields needed for pocket proximity.

    The full ResolvedVariant doc lives in the `resolved` JSONB (core §5.3); the
    scalar columns mirror it. We need residue_index (the variant site) and the
    structure location (structure_bucket/_key) where the resolved PDB lives.
    """
    cursor.execute(
        "SELECT residue_index, wild_type_aa, structure_bucket, structure_key, "
        "resolved FROM variant_resolutions WHERE resolution_id = %s",
        (resolution_id,),
    )
    row = cursor.fetchone()
    if row is None:
        raise PocketError(
            E_RESOLVE_UPSTREAM,
            f"resolution_id '{resolution_id}' not found in variant_resolutions",
        )
    residue_index, wt_aa, bucket, key, resolved = row

    if isinstance(resolved, dict):
        doc = resolved
    elif isinstance(resolved, str):
        doc = json.loads(resolved)
    elif resolved is not None:
        doc = json.loads(resolved.decode("utf-8"))
    else:
        doc = {}

    return {
        "residue_index": int(residue_index),
        "wild_type_aa": (wt_aa or "").strip(),
        # Prefer the recorded structure location; fall back to the canonical
        # resolve key in khemeia-structures (core §5.5).
        "structure_bucket": bucket or BUCKET_STRUCTURES,
        "structure_key": key or f"resolve/{resolution_id}/structure.pdb",
        "structure_source": doc.get("structure_source"),
    }


def s3_get_pdb(s3, bucket: str, key: str) -> str:
    """Return PDB text for a key; raise E_RESOLVE_UPSTREAM if it cannot be read."""
    if s3 is None:
        raise PocketError(
            E_RESOLVE_UPSTREAM, "S3 disabled but the resolved structure is required"
        )
    try:
        resp = s3.get_object(Bucket=bucket, Key=key)
        return resp["Body"].read().decode("utf-8")
    except Exception as exc:
        raise PocketError(
            E_RESOLVE_UPSTREAM,
            f"failed to fetch structure s3://{bucket}/{key}: {exc}",
        )


def s3_put_json(s3, bucket: str, key: str, obj) -> None:
    if s3 is None:
        return  # artifact is best-effort; the headline still lands in staging.
    s3.put_object(
        Bucket=bucket,
        Key=key,
        Body=json.dumps(obj, separators=(",", ":")).encode("utf-8"),
        ContentType="application/json",
    )


# ── PDB parsing + distance math (ported from api/handlers_pocket.go) ───────────
#
# handlers_pocket.go parses fixed-width PDB(QT) ATOM/HETATM records and computes
# Euclidean distances atom-to-atom (its `dist`), then takes the per-residue
# minimum distance to the ligand. Here we port the SAME column layout and the
# SAME nearest-atom distance, but the "probe" is the variant RESIDUE's atoms and
# the targets are (a) each pocket centroid and (b) each pocket-lining residue.
#
# PDB column layout (1-indexed; cols 31-54 X/Y/Z, 23-26 resSeq, 13-16 atom name,
# 18-20 resName, 22 chainID) — identical to parsePDBQTAtom in handlers_pocket.go.


class Atom:
    __slots__ = ("name", "res_name", "chain_id", "res_id", "x", "y", "z")

    def __init__(self, name, res_name, chain_id, res_id, x, y, z):
        self.name = name
        self.res_name = res_name
        self.chain_id = chain_id
        self.res_id = res_id
        self.x = x
        self.y = y
        self.z = z


def parse_pdb_atom(line: str):
    """Port of parsePDBQTAtom (handlers_pocket.go): fixed-width ATOM/HETATM parse.

    Returns an Atom or None for non-coordinate / malformed lines.
    """
    if len(line) < 54:
        return None
    record = line[:6].strip()
    if record not in ("ATOM", "HETATM"):
        return None

    atom_name = line[12:16].strip()
    res_name = line[17:20].strip() if len(line) >= 20 else ""

    chain_id = "A"
    if len(line) >= 22:
        c = line[21]
        chain_id = "A" if c == " " else c  # default chain when blank (Go parity)

    res_id = 0
    if len(line) >= 26:
        try:
            res_id = int(line[22:26].strip())
        except ValueError:
            res_id = 0

    try:
        x = float(line[30:38].strip())
        y = float(line[38:46].strip())
        z = float(line[46:54].strip())
    except ValueError:
        return None

    return Atom(atom_name, res_name, chain_id, res_id, x, y, z)


def parse_all_atoms(pdb_text: str):
    """Port of parseAllAtoms: parse every ATOM/HETATM record from a PDB string."""
    atoms = []
    for line in pdb_text.splitlines():
        a = parse_pdb_atom(line)
        if a is not None:
            atoms.append(a)
    return atoms


def dist(ax, ay, az, bx, by, bz) -> float:
    """Port of `dist` (handlers_pocket.go): Euclidean distance between two points."""
    dx = ax - bx
    dy = ay - by
    dz = az - bz
    return math.sqrt(dx * dx + dy * dy + dz * dz)


def variant_atoms(atoms, residue_index: int):
    """All atoms belonging to the variant residue (matched by PDB residue number).

    The resolved PDB is single-chain (resolve/ESMFold output); residue_index is
    1-based and matches the PDB residue sequence number. Returns the atom list
    (empty if the residue is absent — a truncated model -> E_WT_MISMATCH-class).
    """
    return [a for a in atoms if a.res_id == residue_index]


def variant_ca_or_atoms(atoms, residue_index: int):
    """Prefer the variant residue's CA atom; fall back to all its atoms.

    handlers_pocket.go uses nearest-atom contacts; for a residue probe we measure
    from the CA when present (stable backbone reference) and otherwise from every
    side-chain atom (nearest-atom), matching the Go nearest-atom semantics.
    """
    res = variant_atoms(atoms, residue_index)
    cas = [a for a in res if a.name == "CA"]
    return cas if cas else res


def min_distance_point_to_atoms(px, py, pz, probe_atoms) -> float:
    """Nearest-atom distance from a point (pocket centroid) to the probe residue.

    This is the residue-analog of handlers_pocket.go's per-residue min-distance to
    the ligand: scan the probe atoms, keep the minimum Euclidean distance.
    """
    best = math.inf
    for a in probe_atoms:
        d = dist(a.x, a.y, a.z, px, py, pz)
        if d < best:
            best = d
    return best


def min_distance_atoms_to_atoms(probe_atoms, target_atoms) -> float:
    """Nearest-atom distance between two atom sets (probe residue vs pocket-lining
    residues). Direct port of the nested nearest-atom scan in classifyPocket."""
    best = math.inf
    for p in probe_atoms:
        for t in target_atoms:
            d = dist(p.x, p.y, p.z, t.x, t.y, t.z)
            if d < best:
                best = d
    return best


def parse_p2rank_residue_token(token: str):
    """Parse a p2rank residue token into (res_name, res_id, chain).

    app.py emits residues compacted as "GLU117.A" (NAME+SEQ.CHAIN). Returns
    (name, int seq, chain) or None if it cannot be parsed.
    """
    if "." in token:
        head, chain = token.rsplit(".", 1)
    else:
        head, chain = token, "A"
    # Split trailing digits off the residue name (e.g. GLU117 -> GLU, 117).
    i = len(head)
    while i > 0 and head[i - 1].isdigit():
        i -= 1
    name = head[:i]
    try:
        seq = int(head[i:])
    except ValueError:
        return None
    return name, seq, chain


# ── p2rank invocation (reuse the running sidecar) ──────────────────────────────

def run_p2rank(cfg: dict, pdb_text: str):
    """POST the structure to the p2rank sidecar `/predict` and return its pockets.

    Reuses containers/p2rank/app.py over HTTP — no duplicated pocket detection.
    Each pocket dict carries: rank, score, probability, center [x,y,z], residues.
    """
    url = f"{cfg['p2rank_url']}/predict"
    body = json.dumps({"pdb_data": pdb_text}).encode("utf-8")
    req = urllib.request.Request(
        url, data=body, headers={"Content-Type": "application/json"}, method="POST"
    )
    try:
        with urllib.request.urlopen(req, timeout=cfg["p2rank_timeout_s"]) as resp:
            doc = json.loads(resp.read().decode("utf-8"))
    except Exception as exc:
        # Sidecar unreachable / errored -> upstream failure (retryable). Workers
        # TDD §6.5: persistent sidecar failure -> the job Fails (not "no pockets").
        raise PocketError(
            E_RESOLVE_UPSTREAM, f"p2rank sidecar call failed ({url}): {exc}"
        )
    if isinstance(doc, dict) and "error" in doc:
        raise PocketError(
            E_RESOLVE_UPSTREAM, f"p2rank predict error: {doc.get('error')}"
        )
    return doc.get("pockets", []) if isinstance(doc, dict) else []


# ── Compute ───────────────────────────────────────────────────────────────────

def compute_proximity(atoms, residue_index, pockets, cutoff_ang):
    """Distance from the variant residue to the nearest pocket (centroid + lining).

    For each pocket we compute BOTH:
      - centroid distance: variant residue (nearest atom) -> pocket center, and
      - lining distance:   variant residue (nearest atom) -> nearest pocket-lining
        residue atom (handlers_pocket.go nearest-atom semantics),
    then take the smaller as the pocket's distance_ang (the residue is "near" the
    pocket if it is near the surface OR the centroid). The nearest pocket overall
    is the one with the smallest distance_ang.

    Returns (nearest_pocket dict or None, within_cutoff bool, in_pocket bool).
    """
    probe = variant_ca_or_atoms(atoms, residue_index)
    if not probe:
        raise PocketError(
            E_WT_MISMATCH,
            f"variant residue {residue_index} has no atoms in the structure "
            "(truncated/mismatched model)",
        )

    # Index structure atoms by (chain, res_id) for fast pocket-lining lookup.
    by_residue = {}
    for a in atoms:
        by_residue.setdefault((a.chain_id, a.res_id), []).append(a)

    nearest = None
    for pk in pockets:
        center = pk.get("center") or [0.0, 0.0, 0.0]
        cx, cy, cz = float(center[0]), float(center[1]), float(center[2])
        centroid_d = min_distance_point_to_atoms(cx, cy, cz, probe)

        # Distance to the nearest pocket-lining residue atom + membership check.
        lining_d = math.inf
        residue_in_pocket = False
        for tok in pk.get("residues", []):
            parsed = parse_p2rank_residue_token(tok)
            if parsed is None:
                continue
            _name, seq, chain = parsed
            if seq == residue_index:
                residue_in_pocket = True
            for key in ((chain, seq), ("A", seq)):
                target = by_residue.get(key)
                if target:
                    d = min_distance_atoms_to_atoms(probe, target)
                    if d < lining_d:
                        lining_d = d
                    break

        pocket_d = min(centroid_d, lining_d)
        cand = {
            "rank": int(pk.get("rank", 0)),
            "druggability_score": round(float(pk.get("probability", pk.get("score", 0.0))), 4),
            "distance_ang": round(pocket_d, 2),
            "centroid_distance_ang": round(centroid_d, 2),
            "lining_distance_ang": (round(lining_d, 2) if math.isfinite(lining_d) else None),
            "in_pocket": residue_in_pocket,
            "pocket_residues": list(pk.get("residues", [])),
        }
        if nearest is None or cand["distance_ang"] < nearest["distance_ang"]:
            nearest = cand

    if nearest is None:
        # No pockets detected — a real, informative result (workers TDD §6.5).
        return None, False, False

    within = nearest["distance_ang"] <= cutoff_ang
    in_pocket = bool(nearest["in_pocket"])
    return nearest, within, in_pocket


# ── Run ───────────────────────────────────────────────────────────────────────

def pockets_artifact_key(resolution_id: str) -> str:
    # The prompt's frozen artifact key: pocket_proximity/{rid}/pockets.csv.
    return f"pocket_proximity/{resolution_id}/pockets.csv"


def pockets_to_csv(pockets) -> str:
    """Render the p2rank pocket set as CSV for the artifact (matches *.csv plugin
    pattern + the prompt's pockets.csv key)."""
    out = io.StringIO()
    out.write("rank,score,probability,center_x,center_y,center_z,n_residues,residues\n")
    for pk in pockets:
        center = pk.get("center") or [0.0, 0.0, 0.0]
        residues = " ".join(pk.get("residues", []))
        out.write(
            f"{pk.get('rank', '')},{pk.get('score', '')},{pk.get('probability', '')},"
            f"{center[0]},{center[1]},{center[2]},{len(pk.get('residues', []))},{residues}\n"
        )
    return out.getvalue()


def run_pocket_proximity(conn, cursor, s3, cfg, res, resolution_id) -> None:
    t0 = _time.time()
    _jlog("worker_start", job=cfg["job_name"], calculation="pocket_proximity",
          resolution_id=resolution_id, detector=cfg["detector"],
          cutoff_ang=cfg["cutoff_ang"], residue_index=res["residue_index"])

    if cfg["detector"] != "p2rank":
        # Only the p2rank sidecar is wired here; fpocket is a documented alt
        # detector (workers TDD §6.1) but not reachable from this image.
        raise PocketError(
            E_PARAMS_INVALID,
            f"detector '{cfg['detector']}' not supported by this worker (p2rank only)",
        )

    pdb_text = s3_get_pdb(s3, res["structure_bucket"], res["structure_key"])
    atoms = parse_all_atoms(pdb_text)
    if not atoms:
        raise PocketError(
            E_RESOLVE_UPSTREAM,
            f"no atoms parsed from s3://{res['structure_bucket']}/{res['structure_key']}",
        )

    pockets = run_p2rank(cfg, pdb_text)
    _jlog("progress", job=cfg["job_name"], resolution_id=resolution_id,
          stage="pockets_detected", n_pockets=len(pockets))

    nearest, within, in_pocket = compute_proximity(
        atoms, res["residue_index"], pockets, cfg["cutoff_ang"]
    )

    pockets_key = pockets_artifact_key(resolution_id)
    if pockets:
        s3_put_json(s3, BUCKET_REPORTS, pockets_key, pockets)  # raw detector set
        # CSV companion at the same key family for the *.csv plugin artifact.
        if s3 is not None:
            s3.put_object(
                Bucket=BUCKET_REPORTS, Key=pockets_key,
                Body=pockets_to_csv(pockets).encode("utf-8"),
                ContentType="text/csv",
            )

    distance_ang = nearest["distance_ang"] if nearest else None
    druggability = nearest["druggability_score"] if nearest else None

    # §6(d) payload (core §6(d) / workers TDD §6.3 shape).
    payload = {
        "detector": cfg["detector"],
        "nearest_pocket": (
            {
                "rank": nearest["rank"],
                "druggability_score": nearest["druggability_score"],
                "distance_ang": nearest["distance_ang"],
                "pocket_residues": nearest["pocket_residues"],
            }
            if nearest else None
        ),
        "within_cutoff": within,
        "in_pocket": in_pocket,
        "n_pockets": len(pockets),
        "cutoff_ang": cfg["cutoff_ang"],
        "pockets_key": pockets_key if pockets else None,
        "interpretation": _interpret(nearest, within, in_pocket, cfg["cutoff_ang"]),
    }

    # §5.6 NESTED staging envelope. headline.* -> typed columns (GEN-22 maps
    # pocket_proximity_flag / pocket_distance_ang); confidence is the nearest
    # pocket's druggability score (workers TDD §6.3). distance is null when no
    # pocket — a valid result, not a failure (workers TDD §6.5).
    envelope = {
        "group_name": cfg["group_name"],
        "variant_key": cfg["variant_key"],
        "calculation": "pocket_proximity",
        "resolution_id": resolution_id,
        "structure_source": res.get("structure_source") or "resolve",
        "headline": {
            "pocket_proximity_flag": bool(within),
            "pocket_distance_ang": distance_ang,
            "confidence": druggability,
        },
        "payload": payload,
        "artifact_keys": {
            "pockets": pockets_key if pockets else None,
        },
    }

    cursor.execute(
        "INSERT INTO staging (job_type, payload) VALUES ('genome_calc', %s)",
        (json.dumps(envelope),),
    )
    conn.commit()

    # `metric:` line carrying the values the pocket-proximity plugin parses
    # (workers TDD §6.6: distance_ang, in_pocket).
    _jlog("batch_complete", job=cfg["job_name"], resolution_id=resolution_id,
          n_pockets=len(pockets), within_cutoff=bool(within),
          in_pocket=bool(in_pocket),
          distance_ang=(distance_ang if distance_ang is not None else -1.0),
          elapsed_s=round(_time.time() - t0, 1))


def _interpret(nearest, within, in_pocket, cutoff) -> str:
    if nearest is None:
        return "No druggable pocket detected near the variant residue"
    d = nearest["distance_ang"]
    if in_pocket:
        return f"Mutated residue lines the top druggable pocket ({d} Ang) — likely functional"
    if within:
        return f"Mutated residue within {cutoff} Ang of a druggable pocket ({d} Ang)"
    return f"Mutated residue distant from any druggable pocket (nearest {d} Ang)"


# ── Entrypoint ────────────────────────────────────────────────────────────────

def resolve_context(cfg: dict, cursor) -> str:
    """Backfill identifiers from the variant_calc_jobs row; return resolution_id.

    cr_name = JOB_NAME is the source of truth (the frozen contract). params (JSONB)
    may carry detector / cutoff_ang overrides; env hints win only as a fallback.
    """
    job = fetch_calc_job(cursor, cfg["job_name"])
    if job is not None:
        group_name, variant_key, calculation, resolution_id, params = job
        cfg["group_name"] = cfg["group_name"] or (group_name or "")
        cfg["variant_key"] = cfg["variant_key"] or (variant_key or "")
        cfg["calculation"] = calculation or cfg["calculation"]
        resolution_id = resolution_id or cfg["resolution_id"]
        # Pull detector / cutoff_ang from params if present and not env-overridden.
        doc = _as_dict(params)
        pp = doc.get("pocket_proximity", doc) if isinstance(doc, dict) else {}
        if isinstance(pp, dict):
            if not os.environ.get("DETECTOR") and pp.get("detector"):
                cfg["detector"] = pp["detector"]
            if not os.environ.get("CUTOFF_ANG") and pp.get("cutoff_ang") is not None:
                try:
                    cfg["cutoff_ang"] = float(pp["cutoff_ang"])
                except (TypeError, ValueError):
                    pass
    else:
        resolution_id = cfg["resolution_id"]

    if not resolution_id:
        raise PocketError(
            E_RESOLVE_UPSTREAM,
            f"no resolution_id for job '{cfg['job_name']}' "
            "(absent from variant_calc_jobs row and RESOLUTION_ID env)",
        )
    cfg["resolution_id"] = resolution_id
    return resolution_id


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


def fail(conn, cursor, cfg, code: str, message: str) -> None:
    """Record a typed failure on the owning variant_calc_jobs row and exit 1.

    Mirrors fold.py: every failure carries a frozen E_* code in
    variant_calc_jobs.error_output (core §10 / workers TDD AC #4).
    """
    print(f"FATAL [{code}]: {message}", flush=True)
    _jlog("error", job=cfg.get("job_name"), calculation="pocket_proximity",
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
        f"Pocket-proximity worker starting: job={cfg['job_name']} "
        f"detector={cfg['detector']} cutoff_ang={cfg['cutoff_ang']}",
        flush=True,
    )

    conn = connect_db(cfg)
    cursor = conn.cursor()
    s3 = get_s3_client()

    try:
        resolution_id = resolve_context(cfg, cursor)
        res = fetch_resolution(cursor, resolution_id)
        run_pocket_proximity(conn, cursor, s3, cfg, res, resolution_id)
    except PocketError as exc:
        fail(conn, cursor, cfg, exc.code, exc.message)
    except Exception as exc:  # last-resort: never a silent drop
        fail(conn, cursor, cfg, E_RESOLVE_UPSTREAM, f"unexpected: {exc}")
    finally:
        cursor.close()
        conn.close()


if __name__ == "__main__":
    main()
