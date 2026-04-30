#!/usr/bin/env python3
"""Gnina batch docking worker for v2 multi-engine pipeline.

Fetches receptor PDBQT from S3 (via target-prep ref), fetches ligand
conformer PDBQTs from S3 (via library-prep ref), runs gnina CLI for
each ligand, and writes results to docking_v2_results.

Gnina provides CNN-scored docking (GPU-accelerated) and produces both
Vina-style affinity scores and CNN score/affinity for each pose.

All configuration via environment variables:
  JOB_NAME       - v2 docking job name
  WORKER_NAME    - unique name for this worker pod
  ENGINE         - engine identifier (always "gnina" for this script)
  RECEPTOR_REF   - target-prep job name (resolves receptor_s3_key + binding_site)
  LIBRARY_REF    - library-prep job name (resolves library_compounds)
  EXHAUSTIVENESS - search exhaustiveness (default 32)
  SCORING        - scoring mode (default "default" for CNN+Vina hybrid)
  BATCH_OFFSET   - starting row offset in library_compounds
  BATCH_LIMIT    - number of compounds to process
  MYSQL_HOST     - MySQL hostname
  MYSQL_PORT     - MySQL port (default 3306)
  MYSQL_USER     - MySQL user
  MYSQL_PASSWORD - MySQL password
  GARAGE_ENABLED - "true" to use S3 artifact storage
  GARAGE_ENDPOINT   - S3 endpoint URL
  GARAGE_ACCESS_KEY - S3 access key
  GARAGE_SECRET_KEY - S3 secret key
  GARAGE_REGION     - S3 region
"""

import json
import os
import re
import subprocess
import sys
import tempfile
import time as _time

import mysql.connector

try:
    import boto3
    from botocore.config import Config as BotoConfig
except ImportError:
    boto3 = None

GNINA_BIN = "/usr/local/bin/gnina"
DATA_DIR = "/data"
RECEPTOR_PATH = os.path.join(DATA_DIR, "receptor.pdbqt")

BUCKET_RECEPTORS = "khemeia-receptors"
BUCKET_LIBRARIES = "khemeia-libraries"

# Gnina log output parsers (mode lines with CNN scores)
_MODE_RE = re.compile(
    r"^\s+(\d+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)"
)
_MODE_VINA_RE = re.compile(
    r"^\s+(\d+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)"
)


def _jlog(event: str, **kwargs) -> None:
    """Emit a structured JSON metric line for Promtail/Loki ingestion."""
    payload = {"event": event, "ts": _time.time(), **kwargs}
    print("metric: " + json.dumps(payload, separators=(",", ":")), flush=True)


def require_env(name):
    """Return the value of a required environment variable, or exit."""
    value = os.environ.get(name)
    if not value:
        print(f"FATAL: required environment variable {name} is not set", flush=True)
        sys.exit(1)
    return value


def get_config():
    """Read all configuration from environment variables."""
    return {
        "job_name": require_env("JOB_NAME"),
        "worker_name": require_env("WORKER_NAME"),
        "engine": os.environ.get("ENGINE", "gnina"),
        "receptor_ref": require_env("RECEPTOR_REF"),
        "library_ref": require_env("LIBRARY_REF"),
        "exhaustiveness": int(os.environ.get("EXHAUSTIVENESS", "32")),
        "scoring": os.environ.get("SCORING", "default"),
        "batch_offset": int(require_env("BATCH_OFFSET")),
        "batch_limit": int(require_env("BATCH_LIMIT")),
        "mysql_host": os.environ.get("MYSQL_HOST", "localhost"),
        "mysql_port": int(os.environ.get("MYSQL_PORT", "3306")),
        "mysql_user": os.environ.get("MYSQL_USER", "root"),
        "mysql_password": require_env("MYSQL_PASSWORD"),
    }


def connect_db(cfg):
    """Connect to MySQL. Exits on failure."""
    try:
        return mysql.connector.connect(
            host=cfg["mysql_host"],
            port=cfg["mysql_port"],
            user=cfg["mysql_user"],
            password=cfg["mysql_password"],
            database="docking",
        )
    except mysql.connector.Error as exc:
        print(f"FATAL: MySQL connection failed: {exc}", flush=True)
        sys.exit(1)


def get_s3_client():
    """Create an S3 client for Garage artifact storage. Returns None if disabled."""
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
    """Fetch receptor PDBQT and binding site from target_prep_results.

    Returns (receptor_pdbqt_bytes, center_x, center_y, center_z, size_x, size_y, size_z).
    """
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

    if isinstance(binding_site_json, str):
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
    """Fetch a batch of library compounds with their S3 conformer keys.

    Returns list of (id, compound_id, pdbqt_bytes) tuples.
    """
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
        "WHERE library_id = %s AND filtered = 0 AND s3_conformer_key IS NOT NULL "
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
    """Return set of compound_ids already in docking_v2_results for this job+engine."""
    cursor.execute(
        "SELECT DISTINCT compound_id FROM docking_v2_results WHERE job_name = %s AND engine = %s",
        (job_name, engine),
    )
    return {row[0] for row in cursor.fetchall()}


def parse_gnina_log(log_path):
    """Parse Gnina log file for mode-1 results.

    Returns dict with keys: affinity, cnn_score, cnn_affinity (or None on failure).
    """
    if not os.path.exists(log_path):
        return None

    with open(log_path) as f:
        for line in f:
            m = _MODE_RE.match(line)
            if m and m.group(1) == "1":
                return {
                    "affinity": float(m.group(2)),
                    "cnn_score": float(m.group(5)),
                    "cnn_affinity": float(m.group(6)),
                }
            m = _MODE_VINA_RE.match(line)
            if m and m.group(1) == "1":
                return {
                    "affinity": float(m.group(2)),
                    "cnn_score": 0.0,
                    "cnn_affinity": 0.0,
                }
    return None


def run_gnina(receptor_path, ligand_pdbqt, center, size, exhaustiveness, n_poses=9):
    """Run gnina CLI for a single ligand. Returns (affinity, cnn_score, cnn_affinity, docked_pdbqt) or (None, None, None, None)."""
    tmpdir = tempfile.mkdtemp(prefix="gnina_")
    lig_path = os.path.join(tmpdir, "ligand.pdbqt")
    out_path = os.path.join(tmpdir, "docked.pdbqt")
    log_path = os.path.join(tmpdir, "docked.log")

    try:
        with open(lig_path, "wb") as f:
            if isinstance(ligand_pdbqt, str):
                f.write(ligand_pdbqt.encode("utf-8"))
            else:
                f.write(ligand_pdbqt)

        cx, cy, cz = center
        sx, sy, sz = size

        cmd = [
            GNINA_BIN,
            "--receptor", receptor_path,
            "--ligand", lig_path,
            "--center_x", str(cx),
            "--center_y", str(cy),
            "--center_z", str(cz),
            "--size_x", str(sx),
            "--size_y", str(sy),
            "--size_z", str(sz),
            "--exhaustiveness", str(exhaustiveness),
            "--num_modes", str(n_poses),
            "--out", out_path,
            "--log", log_path,
        ]

        with open(log_path, "w") as log_fh:
            proc = subprocess.Popen(
                cmd, stdout=log_fh, stderr=subprocess.PIPE, text=True,
            )
            try:
                _, stderr_out = proc.communicate(timeout=600)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.communicate()
                print("WARNING: gnina timed out (600s)", flush=True)
                return None, None, None, None
            if stderr_out:
                for line in stderr_out.splitlines():
                    print(f"[gnina] {line}", flush=True)
        result_returncode = proc.returncode
        if result_returncode != 0:
            print(f"WARNING: gnina exited with code {result_returncode}", flush=True)
            return None, None, None, None

        parsed = parse_gnina_log(log_path)
        if parsed is None:
            return None, None, None, None

        docked_pdbqt = None
        if os.path.exists(out_path):
            with open(out_path) as f:
                docked_pdbqt = f.read()

        return parsed["affinity"], parsed["cnn_score"], parsed["cnn_affinity"], docked_pdbqt

    finally:
        for f in [lig_path, out_path, log_path]:
            if os.path.exists(f):
                os.unlink(f)
        if os.path.isdir(tmpdir):
            os.rmdir(tmpdir)


def main():
    cfg = get_config()
    print(
        f"Gnina batch worker starting: job={cfg['job_name']} worker={cfg['worker_name']} "
        f"offset={cfg['batch_offset']} limit={cfg['batch_limit']}",
        flush=True,
    )

    conn = connect_db(cfg)
    cursor = conn.cursor()
    s3 = get_s3_client()

    # Fetch receptor and write to disk
    receptor_pdbqt, cx, cy, cz, sx, sy, sz = fetch_receptor(cursor, s3, cfg["receptor_ref"])
    os.makedirs(DATA_DIR, exist_ok=True)
    with open(RECEPTOR_PATH, "wb") as fh:
        if isinstance(receptor_pdbqt, str):
            fh.write(receptor_pdbqt.encode("utf-8"))
        else:
            fh.write(receptor_pdbqt)

    print(
        f"Receptor loaded: center=({cx}, {cy}, {cz}), size=({sx}, {sy}, {sz})",
        flush=True,
    )

    # Fetch ligand batch
    ligands = fetch_ligands(cursor, s3, cfg["library_ref"], cfg["batch_limit"], cfg["batch_offset"])
    if not ligands:
        print("No ligands found in this batch, nothing to dock", flush=True)
        cursor.close()
        conn.close()
        return

    # Resume support
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

    print(f"Docking {total} ligands (offset={cfg['batch_offset']})", flush=True)
    t0 = _time.time()
    _jlog("worker_start", job=cfg["job_name"], engine=cfg["engine"],
          worker=cfg["worker_name"], total=total, offset=cfg["batch_offset"])
    docked = 0
    failed = 0
    best_affinity = None

    for i, (ligand_db_id, compound_id, pdbqt_bytes) in enumerate(ligands, start=1):
        try:
            affinity, cnn_score, cnn_affinity, docked_pdbqt = run_gnina(
                RECEPTOR_PATH, pdbqt_bytes,
                center=(cx, cy, cz),
                size=(sx, sy, sz),
                exhaustiveness=cfg["exhaustiveness"],
            )

            if affinity is None:
                print(f"WARNING: no result for {compound_id}", flush=True)
                failed += 1
                continue

            docked_blob = None
            if docked_pdbqt is not None and affinity <= -5.0:
                docked_blob = docked_pdbqt.encode("utf-8") if isinstance(docked_pdbqt, str) else docked_pdbqt

            cursor.execute(
                "INSERT INTO docking_v2_results "
                "(job_name, engine, compound_id, ligand_id, affinity_kcal_mol, cnn_score, cnn_affinity, docked_pdbqt) "
                "VALUES (%s, %s, %s, %s, %s, %s, %s, %s)",
                (cfg["job_name"], cfg["engine"], compound_id, ligand_db_id, affinity, cnn_score, cnn_affinity, docked_blob),
            )
            conn.commit()
            docked += 1
            if best_affinity is None or affinity < best_affinity:
                best_affinity = affinity

            elapsed = _time.time() - t0
            print(f"[{i}/{total}] {compound_id} affinity={affinity:.2f} cnn={cnn_score:.3f} elapsed={elapsed:.1f}s", flush=True)
            _jlog("progress", job=cfg["job_name"], engine=cfg["engine"],
                  worker=cfg["worker_name"], processed=i, total=total,
                  docked=docked, failed=failed, elapsed_s=round(elapsed, 1),
                  compound_id=compound_id, affinity=affinity, cnn_score=cnn_score, cnn_affinity=cnn_affinity)

        except Exception as exc:
            print(f"WARNING: docking failed for {compound_id}: {exc}", flush=True)
            failed += 1
            continue

    cursor.close()
    conn.close()
    elapsed = _time.time() - t0
    lig_per_sec = docked / elapsed if elapsed > 0 else 0.0
    print(
        f"Batch complete: {total} attempted, {docked} docked, {failed} failed",
        flush=True,
    )
    _jlog("batch_complete", job=cfg["job_name"], engine=cfg["engine"],
          worker=cfg["worker_name"], total=total, docked=docked, failed=failed,
          elapsed_s=round(elapsed, 1), lig_per_sec=round(lig_per_sec, 3),
          best_affinity=best_affinity)


if __name__ == "__main__":
    main()
