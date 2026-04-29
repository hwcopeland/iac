#!/usr/bin/env python3
"""Vina-GPU 2.1 batch docking worker for the v2 multi-engine pipeline."""

import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path

import mysql.connector

try:
    import boto3
    from botocore.config import Config as BotoConfig
except ImportError:
    boto3 = None

try:
    import resource
except ImportError:  # pragma: no cover - Linux worker always has resource
    resource = None

VINA_GPU_BIN = os.environ.get("VINA_GPU_BIN", "/usr/local/bin/vina-gpu")
OPENCL_BINARY_PATH = os.environ.get("VINA_GPU_OPENCL_BINARY_PATH", "/opt/vina-gpu/bin")
DATA_DIR = "/data"
RECEPTOR_PATH = os.path.join(DATA_DIR, "receptor.pdbqt")
BUCKET_RECEPTORS = "khemeia-receptors"
BUCKET_LIBRARIES = "khemeia-libraries"
_RESULT_RE = re.compile(r"^REMARK VINA RESULT:\s+(-?\d+(?:\.\d+)?)", re.MULTILINE)
_SAFE_NAME_RE = re.compile(r"[^A-Za-z0-9._-]+")


def require_env(name):
    value = os.environ.get(name)
    if not value:
        print(f"FATAL: required environment variable {name} is not set", flush=True)
        sys.exit(1)
    return value


def parse_binding_site(binding_site_json):
    if isinstance(binding_site_json, str):
        binding_site = json.loads(binding_site_json)
    else:
        binding_site = json.loads(binding_site_json.decode("utf-8"))

    if "center" in binding_site and "size" in binding_site:
        center = binding_site["center"]
        size = binding_site["size"]
    else:
        center = [
            binding_site.get("center_x", 0.0),
            binding_site.get("center_y", 0.0),
            binding_site.get("center_z", 0.0),
        ]
        size = [
            binding_site.get("size_x", 40.0),
            binding_site.get("size_y", 40.0),
            binding_site.get("size_z", 40.0),
        ]

    if len(center) != 3 or len(size) != 3:
        raise ValueError(f"invalid binding_site payload: {binding_site!r}")

    return tuple(float(v) for v in center), tuple(float(v) for v in size)


def get_config():
    engine = os.environ.get("ENGINE", "vina-gpu")
    default_threads = 10000 if engine == "vina-gpu-batch" else 8000
    return {
        "job_name": require_env("JOB_NAME"),
        "worker_name": require_env("WORKER_NAME"),
        "engine": engine,
        "receptor_ref": require_env("RECEPTOR_REF"),
        "library_ref": require_env("LIBRARY_REF"),
        "exhaustiveness": int(os.environ.get("EXHAUSTIVENESS", "32")),
        "batch_offset": int(require_env("BATCH_OFFSET")),
        "batch_limit": int(require_env("BATCH_LIMIT")),
        "threads": int(os.environ.get("VINA_GPU_THREADS", str(default_threads))),
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
    if binding_site_json is None:
        print(f"FATAL: binding_site is NULL for '{receptor_ref}'", flush=True)
        sys.exit(1)
    if s3 is None:
        print("FATAL: S3 disabled but receptor stored in S3", flush=True)
        sys.exit(1)

    receptor_pdbqt = s3.get_object(Bucket=BUCKET_RECEPTORS, Key=s3_key)["Body"].read()
    center, size = parse_binding_site(binding_site_json)
    return receptor_pdbqt, center, size


def fetch_ligands(cursor, s3, library_ref, batch_limit, batch_offset):
    cursor.execute("SELECT id FROM library_prep_results WHERE name = %s", (library_ref,))
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
    for ligand_db_id, compound_id, s3_key in rows:
        try:
            pdbqt_bytes = s3.get_object(Bucket=BUCKET_LIBRARIES, Key=s3_key)["Body"].read()
            ligands.append((ligand_db_id, compound_id, pdbqt_bytes))
        except Exception as exc:
            print(f"WARNING: failed to fetch S3 key {s3_key} for {compound_id}: {exc}", flush=True)
    return ligands


def fetch_already_docked(cursor, job_name, engine):
    cursor.execute(
        "SELECT DISTINCT compound_id FROM docking_v2_results WHERE job_name = %s AND engine = %s",
        (job_name, engine),
    )
    return {row[0] for row in cursor.fetchall()}


def _safe_stem(compound_id, index):
    return f"{index:05d}_{_SAFE_NAME_RE.sub('_', compound_id)}"


def _set_stack_limit():
    if resource is None:
        return
    soft, hard = resource.getrlimit(resource.RLIMIT_STACK)
    target = 8 * 1024 * 1024
    if hard not in (resource.RLIM_INFINITY, -1) and hard < target:
        target = hard
    if soft == resource.RLIM_INFINITY or soft < target:
        resource.setrlimit(resource.RLIMIT_STACK, (target, hard))


def parse_output_pose(output_path):
    if not output_path.exists():
        return None, None
    contents = output_path.read_text()
    match = _RESULT_RE.search(contents)
    if not match:
        return None, None
    return float(match.group(1)), contents.encode("utf-8")


def run_vina_gpu_batch(receptor_path, ligands, center, size, exhaustiveness, threads):
    with tempfile.TemporaryDirectory(prefix="vina_gpu_") as tmpdir:
        root = Path(tmpdir)
        ligand_dir = root / "ligands"
        output_dir = root / "out"
        ligand_dir.mkdir()
        output_dir.mkdir()

        manifest = []
        for index, (ligand_db_id, compound_id, pdbqt_bytes) in enumerate(ligands, start=1):
            stem = _safe_stem(compound_id, index)
            ligand_path = ligand_dir / f"{stem}.pdbqt"
            ligand_path.write_bytes(pdbqt_bytes if isinstance(pdbqt_bytes, bytes) else pdbqt_bytes.encode("utf-8"))
            manifest.append((ligand_db_id, compound_id, stem))

        cmd = [
            VINA_GPU_BIN,
            "--receptor", receptor_path,
            "--ligand_directory", str(ligand_dir),
            "--output_directory", str(output_dir),
            "--opencl_binary_path", OPENCL_BINARY_PATH,
            "--center_x", str(center[0]),
            "--center_y", str(center[1]),
            "--center_z", str(center[2]),
            "--size_x", str(size[0]),
            "--size_y", str(size[1]),
            "--size_z", str(size[2]),
            "--thread", str(threads),
            "--search_depth", str(max(1, exhaustiveness)),
        ]

        env = os.environ.copy()
        env.setdefault("LD_LIBRARY_PATH", "/run/opengl-driver/lib:/usr/lib/x86_64-linux-gnu")
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=3600,
            cwd=tmpdir,
            env=env,
            preexec_fn=_set_stack_limit,
        )
        if result.returncode != 0:
            print(f"WARNING: Vina-GPU failed (code {result.returncode})", flush=True)
            if result.stdout:
                print(result.stdout[-2000:], flush=True)
            if result.stderr:
                print(result.stderr[-2000:], flush=True)
            return {}, result

        parsed = {}
        for ligand_db_id, compound_id, stem in manifest:
            affinity, pose = parse_output_pose(output_dir / f"{stem}_out.pdbqt")
            parsed[compound_id] = (ligand_db_id, affinity, pose)
        return parsed, result


def main():
    cfg = get_config()
    print(
        f"Vina-GPU batch worker starting: job={cfg['job_name']} worker={cfg['worker_name']} "
        f"offset={cfg['batch_offset']} limit={cfg['batch_limit']} threads={cfg['threads']}",
        flush=True,
    )

    conn = connect_db(cfg)
    cursor = conn.cursor()
    s3 = get_s3_client()

    receptor_pdbqt, center, size = fetch_receptor(cursor, s3, cfg["receptor_ref"])
    os.makedirs(DATA_DIR, exist_ok=True)
    with open(RECEPTOR_PATH, "wb") as fh:
        fh.write(receptor_pdbqt if isinstance(receptor_pdbqt, bytes) else receptor_pdbqt.encode("utf-8"))

    print(f"Receptor loaded: center={center}, size={size}", flush=True)

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

    print(f"Docking {total} ligands (offset={cfg['batch_offset']})", flush=True)
    parsed_results, process_result = run_vina_gpu_batch(
        RECEPTOR_PATH,
        ligands,
        center=center,
        size=size,
        exhaustiveness=cfg["exhaustiveness"],
        threads=cfg["threads"],
    )

    docked = 0
    failed = 0
    for ligand_db_id, compound_id, _ in ligands:
        _, affinity, pose = parsed_results.get(compound_id, (ligand_db_id, None, None))
        if affinity is None:
            print(f"WARNING: no result for {compound_id}", flush=True)
            failed += 1
            continue

        cursor.execute(
            "INSERT INTO docking_v2_results "
            "(job_name, engine, compound_id, ligand_id, affinity_kcal_mol, docked_pdbqt) "
            "VALUES (%s, %s, %s, %s, %s, %s)",
            (cfg["job_name"], cfg["engine"], compound_id, ligand_db_id, affinity, pose),
        )
        conn.commit()
        docked += 1
        if docked % 50 == 0 or docked == total:
            print(f"Progress: processed={docked + failed}/{total} (docked={docked}, failed={failed})", flush=True)

    if process_result.stdout:
        print(process_result.stdout[-4000:], flush=True)
    if process_result.stderr:
        print(process_result.stderr[-2000:], flush=True)

    cursor.close()
    conn.close()
    print(f"Batch complete: {total} attempted, {docked} docked, {failed} failed", flush=True)


if __name__ == "__main__":
    main()
