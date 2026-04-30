#!/usr/bin/env python3
"""DiffDock batch docking worker for v2 multi-engine pipeline.

DiffDock is a blind docking engine — no binding site box is required.
Unlike vina/gnina, ligand inputs are SMILES strings read directly from
library_compounds (no S3 conformer fetch). The receptor PDBQT from S3 is
cleaned and written as a PDB file for BioPython/DiffDock consumption.

DiffDock outputs a confidence score (higher = more confident) per pose.
This is stored negated in affinity_kcal_mol so the "lower is better"
ranking convention is preserved: confidence 0.8 → -0.8 (ranks first),
confidence -3.0 → 3.0 (ranks last).

All configuration via environment variables:
  JOB_NAME                  - v2 docking job name
  WORKER_NAME               - unique name for this worker pod
  ENGINE                    - engine identifier (always "diffdock")
  RECEPTOR_REF              - target-prep job name (resolves receptor_s3_key)
  LIBRARY_REF               - library-prep job name (resolves library_compounds)
  BATCH_OFFSET              - starting row offset in library_compounds
  BATCH_LIMIT               - number of compounds to process
  DIFFDOCK_INFERENCE_STEPS  - diffusion steps (default 20; fewer = faster)
  DIFFDOCK_SAMPLES          - poses per ligand (default 10)
  DIFFDOCK_BATCH_SIZE       - GPU batch size for DiffDock inference (default 10)
  MYSQL_HOST                - MySQL hostname
  MYSQL_PORT                - MySQL port (default 3306)
  MYSQL_USER                - MySQL user
  MYSQL_PASSWORD            - MySQL password
  GARAGE_ENABLED            - "true" to use S3 artifact storage
  GARAGE_ENDPOINT           - S3 endpoint URL
  GARAGE_ACCESS_KEY         - S3 access key
  GARAGE_SECRET_KEY         - S3 secret key
  GARAGE_REGION             - S3 region
"""

import json
import os
import re
import subprocess
import sys
import tempfile
import time as _time
from pathlib import Path

import mysql.connector

try:
    import boto3
    from botocore.config import Config as BotoConfig
except ImportError:
    boto3 = None

DIFFDOCK_DIR = "/opt/diffdock"
DIFFDOCK_INFERENCE = os.path.join(DIFFDOCK_DIR, "inference.py")
DIFFDOCK_SCORE_MODEL = os.path.join(DIFFDOCK_DIR, "workdir/v1.1/score_model")
DIFFDOCK_CONF_MODEL = os.path.join(DIFFDOCK_DIR, "workdir/v1.1/confidence_model")
DATA_DIR = "/data"
RECEPTOR_PATH = os.path.join(DATA_DIR, "receptor.pdb")

BUCKET_RECEPTORS = "khemeia-receptors"

_SAFE_NAME_RE = re.compile(r"[^A-Za-z0-9._-]+")
_CONFIDENCE_RE = re.compile(r"confidence(-?[\d.]+)")
# PDBQT-specific record types that have no PDB equivalent
_PDBQT_ONLY = frozenset({"ROOT", "ENDROOT", "BRANCH", "ENDBRANCH", "TORSDOF"})


def _jlog(event: str, **kwargs) -> None:
    """Emit a structured JSON metric line for Promtail/Loki ingestion."""
    payload = {"event": event, "ts": _time.time(), **kwargs}
    print("metric: " + json.dumps(payload, separators=(",", ":")), flush=True)


def require_env(name):
    value = os.environ.get(name)
    if not value:
        print(f"FATAL: required environment variable {name} is not set", flush=True)
        sys.exit(1)
    return value


def get_config():
    return {
        "job_name": require_env("JOB_NAME"),
        "worker_name": require_env("WORKER_NAME"),
        "engine": os.environ.get("ENGINE", "diffdock"),
        "receptor_ref": require_env("RECEPTOR_REF"),
        "library_ref": require_env("LIBRARY_REF"),
        "batch_offset": int(require_env("BATCH_OFFSET")),
        "batch_limit": int(require_env("BATCH_LIMIT")),
        "inference_steps": int(os.environ.get("DIFFDOCK_INFERENCE_STEPS", "20")),
        "samples": int(os.environ.get("DIFFDOCK_SAMPLES", "10")),
        "batch_size": int(os.environ.get("DIFFDOCK_BATCH_SIZE", "10")),
        "mysql_host": os.environ.get("MYSQL_HOST", "localhost"),
        "mysql_port": int(os.environ.get("MYSQL_PORT", "3306")),
        "mysql_user": os.environ.get("MYSQL_USER", "root"),
        "mysql_password": require_env("MYSQL_PASSWORD"),
    }


def connect_db(cfg):
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


def pdbqt_to_pdb(pdbqt_bytes):
    """Strip PDBQT-specific records/columns so BioPython parses the file cleanly.

    PDBQT files are near-identical to PDB with extra ROOT/BRANCH records and
    partial charge columns. We drop the non-PDB records and truncate ATOM/HETATM
    lines to 66 columns (removing the charge field at 71-78).
    """
    lines = []
    text = pdbqt_bytes.decode("utf-8", errors="replace")
    for line in text.splitlines():
        record = line[:6].strip().upper()
        if record in _PDBQT_ONLY:
            continue
        if record in ("ATOM", "HETATM"):
            lines.append(line[:66].ljust(66) + "\n")
        else:
            lines.append(line + "\n")
    return "".join(lines).encode("utf-8")


def fetch_receptor(cursor, s3, receptor_ref):
    """Fetch receptor PDBQT from S3. Returns raw bytes."""
    cursor.execute(
        "SELECT receptor_s3_key FROM target_prep_results WHERE name = %s",
        (receptor_ref,),
    )
    row = cursor.fetchone()
    if row is None:
        print(f"FATAL: receptor_ref '{receptor_ref}' not found in target_prep_results", flush=True)
        sys.exit(1)
    s3_key = row[0]
    if not s3_key:
        print(f"FATAL: receptor_s3_key is NULL for '{receptor_ref}'", flush=True)
        sys.exit(1)
    if s3 is None:
        print("FATAL: S3 disabled but receptor stored in S3", flush=True)
        sys.exit(1)
    return s3.get_object(Bucket=BUCKET_RECEPTORS, Key=s3_key)["Body"].read()


def fetch_ligands(cursor, library_ref, batch_limit, batch_offset):
    """Fetch SMILES directly from library_compounds — DiffDock takes SMILES, not conformers.

    Returns list of (ligand_db_id, compound_id, canonical_smiles) tuples.
    """
    cursor.execute("SELECT id FROM library_prep_results WHERE name = %s", (library_ref,))
    row = cursor.fetchone()
    if row is None:
        print(f"FATAL: library_ref '{library_ref}' not found in library_prep_results", flush=True)
        sys.exit(1)
    library_id = row[0]

    cursor.execute(
        "SELECT id, compound_id, canonical_smiles FROM library_compounds "
        "WHERE library_id = %s AND filtered = 0 AND canonical_smiles IS NOT NULL "
        "ORDER BY id LIMIT %s OFFSET %s",
        (library_id, batch_limit, batch_offset),
    )
    return list(cursor.fetchall())


def fetch_already_docked(cursor, job_name, engine):
    cursor.execute(
        "SELECT DISTINCT compound_id FROM docking_v2_results WHERE job_name = %s AND engine = %s",
        (job_name, engine),
    )
    return {row[0] for row in cursor.fetchall()}


def _safe_stem(compound_id, index):
    return f"{index:05d}_{_SAFE_NAME_RE.sub('_', compound_id)}"


def run_diffdock(receptor_path, ligands, inference_steps, samples, batch_size):
    """Run DiffDock inference for all ligands in a single subprocess call.

    Returns dict: compound_id → (ligand_db_id, affinity_score, sdf_bytes).
    affinity_score = -confidence so "lower is better" ranking convention holds.
    Partial results are recovered if the process crashes mid-batch.
    """
    with tempfile.TemporaryDirectory(prefix="diffdock_") as tmpdir:
        root = Path(tmpdir)
        out_dir = root / "results"
        out_dir.mkdir()
        csv_path = root / "input.csv"

        manifest = []  # (ligand_db_id, compound_id, stem)
        with csv_path.open("w") as fh:
            fh.write("complex_name,protein_path,ligand_description,protein_sequence\n")
            for index, (ligand_db_id, compound_id, smiles) in enumerate(ligands, start=1):
                stem = _safe_stem(compound_id, index)
                # Escape SMILES: commas in SMILES are rare but possible; quote the field
                smiles_clean = smiles.replace('"', "")
                fh.write(f'{stem},{receptor_path},"{smiles_clean}",\n')
                manifest.append((ligand_db_id, compound_id, stem))

        cmd = [
            "python3", DIFFDOCK_INFERENCE,
            "--protein_ligand_csv", str(csv_path),
            "--out_dir", str(out_dir),
            "--inference_steps", str(inference_steps),
            "--samples_per_complex", str(samples),
            "--batch_size", str(batch_size),
            "--no_final_step_noise",
            "--model_dir", DIFFDOCK_SCORE_MODEL,
            "--confidence_model_dir", DIFFDOCK_CONF_MODEL,
        ]

        print(
            f"DiffDock inference: {len(ligands)} ligands, {inference_steps} steps, "
            f"{samples} samples/ligand, batch_size={batch_size}",
            flush=True,
        )
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=7200,
            cwd=DIFFDOCK_DIR,
        )

        if result.returncode != 0:
            print(
                f"WARNING: DiffDock exited with code {result.returncode}, attempting partial recovery",
                flush=True,
            )
        if result.stdout:
            print(result.stdout[-4000:], flush=True)
        if result.stderr:
            print(result.stderr[-2000:], flush=True)

        # Collect results — scan output dir regardless of exit code (partial recovery)
        parsed = {}
        for ligand_db_id, compound_id, stem in manifest:
            complex_dir = out_dir / stem
            if not complex_dir.is_dir():
                continue

            # Sort by rank number; rank1 is highest-confidence pose
            def _rank_key(p):
                m = re.search(r"rank(\d+)", p.name)
                return int(m.group(1)) if m else 999

            rank_files = sorted(complex_dir.glob("rank*.sdf"), key=_rank_key)
            if not rank_files:
                continue

            rank1 = rank_files[0]
            m = _CONFIDENCE_RE.search(rank1.name)
            if m is None:
                continue

            confidence = float(m.group(1))
            # Negate: confidence 0.8 (great) → -0.8 (lower = better), -3.0 → 3.0 (higher = worse)
            affinity_score = -confidence
            parsed[compound_id] = (ligand_db_id, affinity_score, rank1.read_bytes())

        if result.returncode != 0:
            print(f"Partial recovery: {len(parsed)}/{len(manifest)} ligands recovered", flush=True)

        return parsed


def main():
    cfg = get_config()
    print(
        f"DiffDock batch worker starting: job={cfg['job_name']} worker={cfg['worker_name']} "
        f"offset={cfg['batch_offset']} limit={cfg['batch_limit']} "
        f"steps={cfg['inference_steps']} samples={cfg['samples']}",
        flush=True,
    )

    conn = connect_db(cfg)
    cursor = conn.cursor()
    s3 = get_s3_client()

    # Fetch receptor PDBQT, convert to clean PDB for BioPython
    receptor_pdbqt = fetch_receptor(cursor, s3, cfg["receptor_ref"])
    os.makedirs(DATA_DIR, exist_ok=True)
    with open(RECEPTOR_PATH, "wb") as fh:
        fh.write(pdbqt_to_pdb(receptor_pdbqt))
    print(f"Receptor written to {RECEPTOR_PATH}", flush=True)

    # DiffDock reads SMILES directly — no S3 conformer fetch needed
    ligands = fetch_ligands(cursor, cfg["library_ref"], cfg["batch_limit"], cfg["batch_offset"])
    if not ligands:
        print("No ligands found in this batch, nothing to dock", flush=True)
        cursor.close()
        conn.close()
        return

    # Resume support
    already_docked = fetch_already_docked(cursor, cfg["job_name"], cfg["engine"])
    if already_docked:
        before = len(ligands)
        ligands = [(lid, cid, smi) for lid, cid, smi in ligands if cid not in already_docked]
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

    parsed_results = run_diffdock(
        RECEPTOR_PATH,
        ligands,
        inference_steps=cfg["inference_steps"],
        samples=cfg["samples"],
        batch_size=cfg["batch_size"],
    )

    docked = 0
    failed = 0
    best_affinity = None
    for ligand_db_id, compound_id, _ in ligands:
        result = parsed_results.get(compound_id)
        if result is None:
            print(f"WARNING: no result for {compound_id}", flush=True)
            failed += 1
            continue

        _, affinity_score, sdf_bytes = result
        cursor.execute(
            "INSERT INTO docking_v2_results "
            "(job_name, engine, compound_id, ligand_id, affinity_kcal_mol, docked_pdbqt) "
            "VALUES (%s, %s, %s, %s, %s, %s)",
            (cfg["job_name"], cfg["engine"], compound_id, ligand_db_id, affinity_score, sdf_bytes),
        )
        conn.commit()
        docked += 1
        if best_affinity is None or affinity_score < best_affinity:
            best_affinity = affinity_score
        if docked % 10 == 0 or docked + failed == total:
            elapsed = _time.time() - t0
            print(f"Progress: {docked + failed}/{total} (docked={docked}, failed={failed})", flush=True)
            _jlog("progress", job=cfg["job_name"], engine=cfg["engine"],
                  worker=cfg["worker_name"], processed=docked + failed, total=total,
                  docked=docked, failed=failed, elapsed_s=round(elapsed, 1))

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
