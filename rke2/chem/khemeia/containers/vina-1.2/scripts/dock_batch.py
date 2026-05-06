#!/usr/bin/env python3
"""Vina 1.2 batch docking worker for v2 multi-engine pipeline.

Fetches receptor PDBQT from S3 (via target-prep ref), fetches ligand
conformer PDBQTs from S3 (via library-prep ref), runs Vina 1.2 CLI in
--batch mode (one subprocess for all ligands), and writes results to
docking_v2_results.

All configuration via environment variables:
  JOB_NAME       - v2 docking job name
  WORKER_NAME    - unique name for this worker pod
  ENGINE         - engine identifier (always "vina-1.2" for this script)
  RECEPTOR_REF   - target-prep job name (resolves receptor_s3_key + binding_site)
  LIBRARY_REF    - library-prep job name (resolves library_compounds)
  EXHAUSTIVENESS - Vina exhaustiveness (default 32)
  SCORING        - scoring function: vina, vinardo, ad4 (default vina)
  BATCH_OFFSET   - starting row offset in library_compounds
  BATCH_LIMIT    - number of compounds to process
  VINA_CPU       - CPU threads to pass to vina --cpu (default: auto-detect)
  POSTGRES_HOST     - PostgreSQL hostname
  POSTGRES_PORT     - PostgreSQL port (default 5432)
  POSTGRES_USER     - PostgreSQL user
  POSTGRES_PASSWORD - PostgreSQL password
  POSTGRES_DB       - PostgreSQL database (default khemeia)
  GARAGE_ENABLED - "true" to use S3 artifact storage
  GARAGE_ENDPOINT   - S3 endpoint URL
  GARAGE_ACCESS_KEY - S3 access key
  GARAGE_SECRET_KEY - S3 secret key
  GARAGE_REGION     - S3 region
"""

import glob
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
except ImportError:
    boto3 = None

DATA_DIR = "/data"
RECEPTOR_PATH = os.path.join(DATA_DIR, "receptor.pdbqt")

_VALID_SCORING = {"vina", "vinardo", "ad4"}
BUCKET_RECEPTORS = "khemeia-receptors"
BUCKET_LIBRARIES = "khemeia-libraries"


def ensure_pdbqt(pdb_bytes: bytes, pdbqt_path: str) -> None:
    """Convert a cleaned PDB to PDBQT via obabel."""
    pdb_path = pdbqt_path + ".pdb"
    try:
        with open(pdb_path, "wb") as fh:
            fh.write(pdb_bytes)
        result = subprocess.run(
            ["obabel", pdb_path, "-O", pdbqt_path, "-xr", "-xn"],
            capture_output=True, text=True, timeout=120,
        )
        if result.returncode != 0 or not os.path.exists(pdbqt_path):
            raise RuntimeError(f"obabel failed (exit {result.returncode}): {result.stderr.strip()}")
        size = os.path.getsize(pdbqt_path)
        if size == 0:
            raise RuntimeError("obabel produced an empty PDBQT file")
        print(f"Receptor converted PDB→PDBQT via obabel ({size} bytes)", flush=True)
    finally:
        if os.path.exists(pdb_path):
            os.unlink(pdb_path)


def _jlog(event: str, **kwargs) -> None:
    payload = {"event": event, "ts": _time.time(), **kwargs}
    print("metric: " + json.dumps(payload, separators=(",", ":")), flush=True)


def require_env(name):
    value = os.environ.get(name)
    if not value:
        print(f"FATAL: required environment variable {name} is not set", flush=True)
        sys.exit(1)
    return value


def get_config():
    scoring = os.environ.get("SCORING", "vina")
    if scoring not in _VALID_SCORING:
        print(f"FATAL: invalid scoring function '{scoring}', must be one of: {sorted(_VALID_SCORING)}", flush=True)
        sys.exit(1)
    vina_cpu = os.environ.get("VINA_CPU")
    return {
        "job_name": require_env("JOB_NAME"),
        "worker_name": require_env("WORKER_NAME"),
        "engine": os.environ.get("ENGINE", "vina-1.2"),
        "receptor_ref": require_env("RECEPTOR_REF"),
        "library_ref": require_env("LIBRARY_REF"),
        "exhaustiveness": int(os.environ.get("EXHAUSTIVENESS", "32")),
        "scoring": scoring,
        "batch_offset": int(require_env("BATCH_OFFSET")),
        "batch_limit": int(require_env("BATCH_LIMIT")),
        "vina_cpu": int(vina_cpu) if vina_cpu else None,
        "pg_host": os.environ.get("POSTGRES_HOST", "localhost"),
        "pg_port": int(os.environ.get("POSTGRES_PORT", "5432")),
        "pg_user": os.environ.get("POSTGRES_USER", "root"),
        "pg_password": require_env("POSTGRES_PASSWORD"),
        "pg_db": os.environ.get("POSTGRES_DB", "khemeia"),
    }


def connect_db(cfg):
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
        print("WARNING: boto3 not installed, S3 artifact fetching disabled", flush=True)
        return None
    return boto3.client(
        "s3",
        endpoint_url=os.environ.get("GARAGE_ENDPOINT"),
        aws_access_key_id=os.environ.get("GARAGE_ACCESS_KEY"),
        aws_secret_access_key=os.environ.get("GARAGE_SECRET_KEY"),
        region_name=os.environ.get("GARAGE_REGION", "garage"),
        config=BotoConfig(signature_version="s3v4"),
    )


def fetch_receptor(cursor, s3, receptor_ref):
    cursor.execute(
        "SELECT receptor_s3_key, binding_site FROM target_prep_results WHERE name = %s",
        (receptor_ref,),
    )
    row = cursor.fetchone()
    if row is None:
        print(f"FATAL: receptor_ref '{receptor_ref}' not found in target_prep_results", flush=True)
        sys.exit(1)

    s3_key, binding_site_json = row
    if not s3_key:
        print(f"FATAL: receptor_s3_key is NULL for '{receptor_ref}'", flush=True)
        sys.exit(1)
    if s3 is None:
        print("FATAL: S3 disabled but receptor stored in S3", flush=True)
        sys.exit(1)

    resp = s3.get_object(Bucket=BUCKET_RECEPTORS, Key=s3_key)
    receptor_pdbqt = resp["Body"].read()

    if binding_site_json is None:
        print(f"FATAL: binding_site is NULL for '{receptor_ref}'", flush=True)
        sys.exit(1)

    if isinstance(binding_site_json, dict):
        bs = binding_site_json
    elif isinstance(binding_site_json, str):
        bs = json.loads(binding_site_json)
    else:
        bs = json.loads(binding_site_json.decode("utf-8"))

    center = bs.get("center")
    size = bs.get("size")
    if center is not None and size is not None:
        center_x, center_y, center_z = (float(v) for v in center)
        size_x, size_y, size_z = (float(v) for v in size)
    else:
        center_x = float(bs.get("center_x", 0))
        center_y = float(bs.get("center_y", 0))
        center_z = float(bs.get("center_z", 0))
        size_x = float(bs.get("size_x", 40))
        size_y = float(bs.get("size_y", 40))
        size_z = float(bs.get("size_z", 40))

    return receptor_pdbqt, center_x, center_y, center_z, size_x, size_y, size_z


def fetch_ligands(cursor, s3, library_ref, batch_limit, batch_offset):
    cursor.execute(
        "SELECT id FROM library_prep_results WHERE name = %s",
        (library_ref,),
    )
    row = cursor.fetchone()
    if row is None:
        print(f"FATAL: library_ref '{library_ref}' not found in library_prep_results", flush=True)
        sys.exit(1)
    library_id = row[0]

    cursor.execute(
        "SELECT id, compound_id, s3_conformer_key FROM library_compounds "
        "WHERE library_id = %s AND filtered = false AND s3_conformer_key IS NOT NULL "
        "ORDER BY id LIMIT %s OFFSET %s",
        (library_id, batch_limit, batch_offset),
    )
    rows = cursor.fetchall()
    if not rows:
        return []
    if s3 is None:
        print("FATAL: S3 disabled but conformers stored in S3", flush=True)
        sys.exit(1)

    ligands = []
    for compound_id_db, compound_id, s3_key in rows:
        try:
            resp = s3.get_object(Bucket=BUCKET_LIBRARIES, Key=s3_key)
            pdbqt_bytes = resp["Body"].read()
            ligands.append((compound_id_db, compound_id, pdbqt_bytes))
        except Exception as exc:
            print(f"WARNING: failed to fetch S3 key {s3_key} for {compound_id}: {exc}", flush=True)

    return ligands


def fetch_already_docked(cursor, job_name, engine):
    cursor.execute(
        "SELECT DISTINCT compound_id FROM docking_v2_results WHERE job_name = %s AND engine = %s",
        (job_name, engine),
    )
    return {row[0] for row in cursor.fetchall()}


def parse_best_affinity(pdbqt_path: str):
    """Return the best affinity from a Vina output PDBQT (first REMARK VINA RESULT line)."""
    with open(pdbqt_path) as fh:
        for line in fh:
            if line.startswith("REMARK VINA RESULT:"):
                parts = line.split()
                if len(parts) >= 4:
                    return float(parts[3])
    return None


def run_vina_batch(receptor_path, ligands, center, size, exhaustiveness, scoring, vina_cpu, tmpdir):
    """Run vina CLI --batch mode. Returns dict of ligand_db_id (str) -> (affinity, pose_bytes)."""
    ligand_dir = os.path.join(tmpdir, "ligands")
    out_dir = os.path.join(tmpdir, "out")
    os.makedirs(ligand_dir, exist_ok=True)
    os.makedirs(out_dir, exist_ok=True)

    # Write ligand files named by db_id so we can match output back
    ligand_files = []
    for ligand_db_id, compound_id, pdbqt_bytes in ligands:
        fname = os.path.join(ligand_dir, f"{ligand_db_id}.pdbqt")
        with open(fname, "wb") as fh:
            fh.write(pdbqt_bytes if isinstance(pdbqt_bytes, bytes) else pdbqt_bytes.encode())
        ligand_files.append(fname)

    cx, cy, cz = center
    sx, sy, sz = size

    cmd = [
        "vina",
        "--receptor", receptor_path,
        "--batch", *ligand_files,
        "--out_dir", out_dir,
        "--center_x", str(cx),
        "--center_y", str(cy),
        "--center_z", str(cz),
        "--size_x", str(sx),
        "--size_y", str(sy),
        "--size_z", str(sz),
        "--exhaustiveness", str(exhaustiveness),
        "--scoring", scoring,
    ]
    if vina_cpu is not None:
        cmd += ["--cpu", str(vina_cpu)]

    print(f"Running: {' '.join(cmd[:6])} ... ({len(ligand_files)} ligands)", flush=True)
    result = subprocess.run(cmd, capture_output=True, text=True, timeout=86400)
    if result.stdout:
        print(result.stdout, flush=True)
    if result.returncode != 0:
        print(f"vina stderr: {result.stderr.strip()}", flush=True)
        raise RuntimeError(f"vina --batch exited with code {result.returncode}")

    # Parse output: vina writes {stem}_out.pdbqt per input file
    results = {}
    for out_file in glob.glob(os.path.join(out_dir, "*_out.pdbqt")):
        stem = os.path.basename(out_file)[: -len("_out.pdbqt")]
        affinity = parse_best_affinity(out_file)
        if affinity is not None:
            with open(out_file, "rb") as fh:
                pose_bytes = fh.read()
            results[stem] = (affinity, pose_bytes)
        else:
            print(f"WARNING: no affinity found in {out_file}", flush=True)

    return results


def main():
    cfg = get_config()
    print(
        f"Vina 1.2 batch worker starting: job={cfg['job_name']} worker={cfg['worker_name']} "
        f"offset={cfg['batch_offset']} limit={cfg['batch_limit']} scoring={cfg['scoring']}",
        flush=True,
    )

    conn = connect_db(cfg)
    cursor = conn.cursor()
    s3 = get_s3_client()

    receptor_pdbqt, cx, cy, cz, sx, sy, sz = fetch_receptor(cursor, s3, cfg["receptor_ref"])
    os.makedirs(DATA_DIR, exist_ok=True)
    if isinstance(receptor_pdbqt, str):
        receptor_pdbqt = receptor_pdbqt.encode("utf-8")
    ensure_pdbqt(receptor_pdbqt, RECEPTOR_PATH)
    print(f"Receptor loaded: center=({cx}, {cy}, {cz}), size=({sx}, {sy}, {sz})", flush=True)

    ligands = fetch_ligands(cursor, s3, cfg["library_ref"], cfg["batch_limit"], cfg["batch_offset"])
    if not ligands:
        print("No ligands found in this batch, nothing to dock", flush=True)
        cursor.close()
        conn.close()
        return

    already_docked = fetch_already_docked(cursor, cfg["job_name"], cfg["engine"])
    if already_docked:
        before = len(ligands)
        ligands = [(lid, cid, pdbqt) for lid, cid, pdbqt in ligands if cid not in already_docked]
        skipped = before - len(ligands)
        if skipped > 0:
            print(f"Skipping {skipped} already-docked compounds", flush=True)

    total = len(ligands)
    if total == 0:
        print("All ligands in this batch already docked, nothing to do", flush=True)
        cursor.close()
        conn.close()
        return

    print(f"Submitting {total} ligands to vina --batch (offset={cfg['batch_offset']})", flush=True)
    t0 = _time.time()
    _jlog("worker_start", job=cfg["job_name"], engine=cfg["engine"],
          worker=cfg["worker_name"], total=total, offset=cfg["batch_offset"])

    # Build lookup by db_id string for matching output files
    ligand_map = {str(lid): (lid, cid) for lid, cid, _ in ligands}

    with tempfile.TemporaryDirectory() as tmpdir:
        try:
            batch_results = run_vina_batch(
                RECEPTOR_PATH, ligands,
                center=(cx, cy, cz),
                size=(sx, sy, sz),
                exhaustiveness=cfg["exhaustiveness"],
                scoring=cfg["scoring"],
                vina_cpu=cfg["vina_cpu"],
                tmpdir=tmpdir,
            )
        except Exception as exc:
            print(f"FATAL: vina --batch failed: {exc}", flush=True)
            cursor.close()
            conn.close()
            sys.exit(1)

    elapsed = _time.time() - t0
    print(f"Vina batch complete in {elapsed:.1f}s — {len(batch_results)}/{total} results", flush=True)

    docked = 0
    failed = total - len(batch_results)
    best_affinity = None

    for i, (db_id_str, (affinity, pose_bytes)) in enumerate(batch_results.items(), start=1):
        if db_id_str not in ligand_map:
            print(f"WARNING: unrecognised output key {db_id_str}", flush=True)
            continue

        ligand_db_id, compound_id = ligand_map[db_id_str]

        docked_blob = None
        if affinity <= -5.0:
            docked_blob = pose_bytes

        cursor.execute(
            "INSERT INTO docking_v2_results "
            "(job_name, engine, compound_id, ligand_id, affinity_kcal_mol, docked_pdbqt) "
            "VALUES (%s, %s, %s, %s, %s, %s)",
            (cfg["job_name"], cfg["engine"], compound_id, ligand_db_id, affinity, docked_blob),
        )
        conn.commit()
        docked += 1
        if best_affinity is None or affinity < best_affinity:
            best_affinity = affinity

        print(f"[{i}/{len(batch_results)}] {compound_id} affinity={affinity:.2f}", flush=True)

    cursor.close()
    conn.close()
    elapsed = _time.time() - t0
    lig_per_sec = docked / elapsed if elapsed > 0 else 0.0
    print(f"Batch complete: {total} attempted, {docked} docked, {failed} failed", flush=True)
    _jlog("batch_complete", job=cfg["job_name"], engine=cfg["engine"],
          worker=cfg["worker_name"], total=total, docked=docked, failed=failed,
          elapsed_s=round(elapsed, 1), lig_per_sec=round(lig_per_sec, 3),
          best_affinity=best_affinity)


if __name__ == "__main__":
    main()
