#!/usr/bin/env python3
"""Batch ligand preparation worker (Phase A).

Reads SMILES from the ligands table, generates 3D conformers via RDKit,
converts to PDBQT via prepare_ligand4, and writes base64-encoded results
to the staging table for the result-writer to drain.

All configuration is via environment variables:
  SOURCE_DB      -- which source_db to filter ligands by
  BATCH_OFFSET   -- starting row offset
  BATCH_LIMIT    -- number of ligands to process
  MYSQL_HOST     -- MySQL hostname (default: localhost)
  MYSQL_PORT     -- MySQL port (default: 3306)
  MYSQL_USER     -- MySQL user (default: root)
  MYSQL_PASSWORD -- MySQL password (required)
  MYSQL_DATABASE -- MySQL database (default: docking)
"""

import base64
import json
import os
import subprocess
import sys
import tempfile

import mysql.connector
from rdkit import Chem
from rdkit.Chem import AllChem


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


def fetch_ligands(cursor, source_db, batch_limit, batch_offset):
    """Fetch a batch of ligands that need PDBQT preparation.

    Returns list of (id, compound_id, smiles).
    Exits if no ligands are found.
    """
    cursor.execute(
        "SELECT id, compound_id, smiles FROM ligands "
        "WHERE source_db = %s AND pdbqt IS NULL "
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


def smiles_to_mol(smiles):
    """Parse SMILES and generate a 3D conformer. Returns mol or None."""
    mol = Chem.MolFromSmiles(smiles)
    if mol is None:
        return None

    mol = Chem.AddHs(mol)

    # Try default embedding first
    result = AllChem.EmbedMolecule(mol, randomSeed=42)
    if result == -1:
        # Fallback to ETKDGv3 for difficult molecules
        params = AllChem.ETKDGv3()
        params.randomSeed = 42
        result = AllChem.EmbedMolecule(mol, params)
        if result == -1:
            return None

    AllChem.MMFFOptimizeMolecule(mol)
    return mol


def mol_to_pdbqt(mol, tmpdir):
    """Write mol as PDB, run prepare_ligand4, return PDBQT text or None."""
    pdb_path = os.path.join(tmpdir, "ligand.pdb")
    pdbqt_path = os.path.join(tmpdir, "ligand.pdbqt")

    pdb_block = Chem.MolToPDBBlock(mol)
    with open(pdb_path, "w") as fh:
        fh.write(pdb_block)

    cmd = ["prepare_ligand4", "-l", pdb_path, "-o", pdbqt_path]
    try:
        subprocess.run(cmd, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    except subprocess.CalledProcessError as exc:
        print(
            f"WARNING: prepare_ligand4 failed: {exc.stderr.decode(errors='replace')}",
            file=sys.stderr,
        )
        return None

    if not os.path.exists(pdbqt_path):
        return None

    with open(pdbqt_path, "r") as fh:
        return fh.read()


def main():
    cfg = get_config()

    conn = connect_db(cfg)
    cursor = conn.cursor()

    # Fetch ligand batch
    ligands = fetch_ligands(cursor, cfg["source_db"], cfg["batch_limit"], cfg["batch_offset"])
    total = len(ligands)
    print(
        f"Fetched {total} ligands for prep (source_db='{cfg['source_db']}' "
        f"offset={cfg['batch_offset']})",
        flush=True,
    )

    skipped = 0

    for i, (ligand_id, compound_id, smiles) in enumerate(ligands, start=1):
        with tempfile.TemporaryDirectory() as tmpdir:
            # Parse SMILES and generate 3D conformer
            mol = smiles_to_mol(smiles)
            if mol is None:
                print(
                    f"Skipped {i}/{total}: {compound_id} (invalid SMILES or embedding failed)",
                    flush=True,
                )
                skipped += 1
                continue

            # Convert to PDBQT via prepare_ligand4
            pdbqt_text = mol_to_pdbqt(mol, tmpdir)
            if pdbqt_text is None:
                print(
                    f"Skipped {i}/{total}: {compound_id} (prepare_ligand4 failed)",
                    flush=True,
                )
                skipped += 1
                continue

        # Base64-encode and write to staging
        pdbqt_b64 = base64.b64encode(pdbqt_text.encode("utf-8")).decode("ascii")
        payload = json.dumps({
            "ligand_id": ligand_id,
            "pdbqt_b64": pdbqt_b64,
        })
        cursor.execute(
            "INSERT INTO staging (job_type, payload) VALUES ('prep', %s)",
            (payload,),
        )
        conn.commit()

        print(f"Prepared {i}/{total}: {compound_id}", flush=True)

    cursor.close()
    conn.close()
    print(
        f"Batch complete: {total} ligands, {total - skipped} prepared, {skipped} skipped",
        flush=True,
    )


if __name__ == "__main__":
    main()
