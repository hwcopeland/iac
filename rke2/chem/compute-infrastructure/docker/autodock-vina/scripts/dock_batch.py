#!/usr/bin/env python3
"""Batch docking worker.

Fetches receptor PDBQT and pre-computed ligand PDBQTs from MySQL, runs
AutoDock Vina for each ligand, and writes results to the staging table.

Replaces the previous ligandprepv2.py + dockingv2.py + export_energies_mysql.py
pipeline with a single DB-driven batch worker.

All configuration is via environment variables:
  WORKFLOW_NAME  — workflow identifier
  PDBID          — PDB ID of the receptor
  NATIVE_LIGAND  — native ligand ID
  SOURCE_DB      — ligand source database filter
  BATCH_OFFSET   — starting row offset in ligands table
  BATCH_LIMIT    — number of ligands to process
  MYSQL_HOST     — MySQL hostname (default: localhost)
  MYSQL_PORT     — MySQL port (default: 3306)
  MYSQL_USER     — MySQL user (default: root)
  MYSQL_PASSWORD — MySQL password (required)
  MYSQL_DATABASE — MySQL database (default: docking)
  POSE_THRESHOLD — affinity threshold for saving docked poses (default: -7.0)
"""

import json
import os
import re
import subprocess
import sys
import tempfile

import mysql.connector

VINA_BIN = "/autodock/vina"
DATA_DIR = "/data"
RECEPTOR_PATH = os.path.join(DATA_DIR, "receptor.pdbqt")
GRID_SIZE = 40

# Matches mode-1 result line in Vina log output:
#   "   1       -7.1      0.000      0.000"
_MODE1_RE = re.compile(r"^\s+1\s+([-\d.]+)\s+")


def require_env(name):
    """Return the value of a required environment variable, or exit."""
    value = os.environ.get(name)
    if not value:
        print(f"FATAL: required environment variable {name} is not set", file=sys.stderr)
        sys.exit(1)
    return value


def get_config():
    """Read all configuration from environment variables."""
    return {
        "workflow_name": require_env("WORKFLOW_NAME"),
        "pdb_id": require_env("PDBID"),
        "native_ligand": require_env("NATIVE_LIGAND"),
        "source_db": require_env("SOURCE_DB"),
        "batch_offset": int(require_env("BATCH_OFFSET")),
        "batch_limit": int(require_env("BATCH_LIMIT")),
        "mysql_host": os.environ.get("MYSQL_HOST", "localhost"),
        "mysql_port": int(os.environ.get("MYSQL_PORT", "3306")),
        "mysql_user": os.environ.get("MYSQL_USER", "root"),
        "mysql_password": require_env("MYSQL_PASSWORD"),
        "mysql_database": os.environ.get("MYSQL_DATABASE", "docking"),
    }


def connect_db(cfg):
    """Connect to MySQL. Exits on failure."""
    try:
        return mysql.connector.connect(
            host=cfg["mysql_host"],
            port=cfg["mysql_port"],
            user=cfg["mysql_user"],
            password=cfg["mysql_password"],
            database=cfg["mysql_database"],
        )
    except mysql.connector.Error as exc:
        print(f"FATAL: MySQL connection failed: {exc}", file=sys.stderr)
        sys.exit(1)


def fetch_receptor(cursor, workflow_name):
    """Fetch receptor PDBQT and grid center from docking_workflows table.

    Returns (receptor_pdbqt_text, grid_x, grid_y, grid_z).
    Exits if the workflow is not found.
    """
    cursor.execute(
        "SELECT receptor_pdbqt, grid_center_x, grid_center_y, grid_center_z "
        "FROM docking_workflows WHERE name = %s",
        (workflow_name,),
    )
    row = cursor.fetchone()
    if row is None:
        print(
            f"FATAL: no receptor data for workflow '{workflow_name}' in docking_workflows",
            file=sys.stderr,
        )
        sys.exit(1)

    receptor_pdbqt, gx, gy, gz = row
    if receptor_pdbqt is None:
        print("FATAL: receptor_pdbqt is NULL in docking_workflows", file=sys.stderr)
        sys.exit(1)
    return receptor_pdbqt, float(gx), float(gy), float(gz)


def fetch_ligands(cursor, source_db, batch_limit, batch_offset):
    """Fetch a batch of ligands with pre-computed PDBQTs.

    Returns list of (id, compound_id, pdbqt_text).
    Exits if no ligands are found.
    """
    cursor.execute(
        "SELECT id, compound_id, pdbqt FROM ligands "
        "WHERE source_db = %s AND pdbqt IS NOT NULL "
        "ORDER BY id LIMIT %s OFFSET %s",
        (source_db, batch_limit, batch_offset),
    )
    rows = cursor.fetchall()
    if not rows:
        print(
            f"FATAL: no ligands found for source_db='{source_db}' "
            f"offset={batch_offset} limit={batch_limit}",
            file=sys.stderr,
        )
        sys.exit(1)
    return rows


def run_vina(ligand_pdbqt_path, grid_x, grid_y, grid_z):
    """Run Vina and return (mode-1 affinity, docked_output_path), or (None, None) on failure."""
    out_path = os.path.join(DATA_DIR, "docked.pdbqt")
    log_path = os.path.join(DATA_DIR, "docked.log")

    cmd = [
        VINA_BIN,
        "--receptor", RECEPTOR_PATH,
        "--ligand", ligand_pdbqt_path,
        "--center_x", str(grid_x),
        "--center_y", str(grid_y),
        "--center_z", str(grid_z),
        "--size_x", str(GRID_SIZE),
        "--size_y", str(GRID_SIZE),
        "--size_z", str(GRID_SIZE),
        "--out", out_path,
        "--log", log_path,
    ]

    try:
        subprocess.run(cmd, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    except subprocess.CalledProcessError as exc:
        print(
            f"WARNING: Vina failed for {ligand_pdbqt_path}: {exc.stderr.decode(errors='replace')}",
            file=sys.stderr,
        )
        return None, None

    affinity = parse_vina_log(log_path)
    return affinity, out_path if affinity is not None else None


def parse_vina_log(log_path):
    """Parse mode-1 affinity from a Vina log file. Returns float or None."""
    try:
        with open(log_path, "r") as fh:
            for line in fh:
                m = _MODE1_RE.match(line)
                if m:
                    return float(m.group(1))
    except (OSError, ValueError) as exc:
        print(f"WARNING: could not parse {log_path}: {exc}", file=sys.stderr)
    return None


def main():
    cfg = get_config()

    conn = connect_db(cfg)
    cursor = conn.cursor()

    # Fetch receptor data and write PDBQT to disk
    receptor_pdbqt, grid_x, grid_y, grid_z = fetch_receptor(cursor, cfg["workflow_name"])

    os.makedirs(DATA_DIR, exist_ok=True)
    with open(RECEPTOR_PATH, "wb") as fh:
        if isinstance(receptor_pdbqt, str):
            fh.write(receptor_pdbqt.encode("utf-8"))
        else:
            fh.write(receptor_pdbqt)

    print(
        f"Receptor loaded for workflow '{cfg['workflow_name']}': "
        f"grid_center=({grid_x}, {grid_y}, {grid_z})",
        file=sys.stderr,
    )

    # Fetch ligand batch
    ligands = fetch_ligands(cursor, cfg["source_db"], cfg["batch_limit"], cfg["batch_offset"])
    total = len(ligands)
    print(f"Fetched {total} ligands (offset={cfg['batch_offset']})", file=sys.stderr)

    # Process each ligand
    for i, (ligand_id, compound_id, pdbqt_text) in enumerate(ligands, start=1):
        # Write ligand PDBQT to a temp file
        ligand_path = os.path.join(DATA_DIR, f"ligand_{ligand_id}.pdbqt")
        try:
            with open(ligand_path, "wb") as fh:
                if isinstance(pdbqt_text, str):
                    fh.write(pdbqt_text.encode("utf-8"))
                else:
                    fh.write(pdbqt_text)

            affinity, docked_path = run_vina(ligand_path, grid_x, grid_y, grid_z)

            if affinity is None:
                print(
                    f"Skipped ligand {i}/{total}: {compound_id} (Vina failure or unparseable log)",
                    file=sys.stderr,
                )
                continue

            # Save docked poses (all 9 modes) for top hits only
            pose_threshold = float(os.environ.get("POSE_THRESHOLD", "-7.0"))
            docked_pdbqt = None
            if affinity <= pose_threshold and docked_path and os.path.exists(docked_path):
                with open(docked_path, "r") as fh:
                    docked_pdbqt = fh.read()

            # Write result to staging table
            result = {
                "workflow_name": cfg["workflow_name"],
                "pdb_id": cfg["pdb_id"],
                "ligand_id": ligand_id,
                "compound_id": compound_id,
                "affinity_kcal_mol": affinity,
            }
            if docked_pdbqt is not None:
                result["docked_pdbqt"] = docked_pdbqt
            payload = json.dumps(result)
            cursor.execute(
                "INSERT INTO staging (job_type, payload) VALUES ('dock', %s)",
                (payload,),
            )
            conn.commit()

            print(
                f"Processed ligand {i}/{total}: {compound_id} affinity={affinity}",
                file=sys.stderr,
            )

        finally:
            # Clean up temp ligand file
            if os.path.exists(ligand_path):
                os.remove(ligand_path)

    cursor.close()
    conn.close()
    print(f"Batch complete: {total} ligands processed", file=sys.stderr)


if __name__ == "__main__":
    main()
