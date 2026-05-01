#!/usr/bin/env python3
"""ORCA RESP charge worker for Khemeia ligand force field preparation.

Workflow:
  1. Fetch compound SMILES and best docked pose from DB.
  2. Convert pose geometry to XYZ (obabel -h); fall back to RDKit ETKDGv3 conformer.
  3. Run ORCA HF/6-31G* with CHELPG ESP fitting.
  4. Parse CHELPG charges from ORCA output.
  5. Run antechamber to assign GAFF2 atom types + RESP charges → mol2.
  6. Upload mol2 + raw ORCA output to S3: resp/<compound_id>/{LIG.mol2,orca.out}.

The GROMACS worker checks for the mol2 in S3 before falling back to Gasteiger.

All configuration via environment variables:
  JOB_NAME          - RESP job identifier
  COMPOUND_ID       - KHM-* identifier
  DOCK_JOB_NAME     - docking job to pull pose geometry from (optional)
  ORCA_NPROC        - CPU threads for ORCA (default: 4)
  ORCA_MAXCORE      - MB per core for ORCA (default: 2000)
  MYSQL_HOST / MYSQL_PORT / MYSQL_USER / MYSQL_PASSWORD
  GARAGE_ENABLED / GARAGE_ENDPOINT / GARAGE_ACCESS_KEY / GARAGE_SECRET_KEY / GARAGE_REGION
"""

import os
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

BUCKET_RESP = "khemeia-resp"


def require_env(name):
    val = os.environ.get(name)
    if not val:
        print(f"FATAL: {name} not set", flush=True)
        sys.exit(1)
    return val


def get_config():
    return {
        "job_name":      require_env("JOB_NAME"),
        "compound_id":   require_env("COMPOUND_ID"),
        "dock_job_name": os.environ.get("DOCK_JOB_NAME", ""),
        "nproc":         int(os.environ.get("ORCA_NPROC", "4")),
        "maxcore":       int(os.environ.get("ORCA_MAXCORE", "2000")),
        "mysql_host":    os.environ.get("MYSQL_HOST", "localhost"),
        "mysql_port":    int(os.environ.get("MYSQL_PORT", "3306")),
        "mysql_user":    os.environ.get("MYSQL_USER", "root"),
        "mysql_password": require_env("MYSQL_PASSWORD"),
    }


def connect_db(cfg):
    try:
        return mysql.connector.connect(
            host=cfg["mysql_host"], port=cfg["mysql_port"],
            user=cfg["mysql_user"], password=cfg["mysql_password"],
            database="docking",
        )
    except mysql.connector.Error as exc:
        print(f"FATAL: MySQL: {exc}", flush=True)
        sys.exit(1)


def get_s3():
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


def fetch_smiles(cursor, compound_id):
    cursor.execute(
        "SELECT canonical_smiles FROM library_compounds WHERE compound_id=%s LIMIT 1",
        (compound_id,),
    )
    row = cursor.fetchone()
    if row is None:
        print(f"FATAL: no SMILES for {compound_id}", flush=True)
        sys.exit(1)
    return row[0]


def fetch_pose_bytes(cursor, compound_id, dock_job_name):
    """Return raw docked pose bytes (PDBQT or SDF) for the best pose, or None."""
    if not dock_job_name:
        return None
    cursor.execute(
        "SELECT docked_pdbqt FROM docking_v2_results "
        "WHERE job_name=%s AND compound_id=%s "
        "ORDER BY affinity_kcal_mol ASC LIMIT 1",
        (dock_job_name, compound_id),
    )
    row = cursor.fetchone()
    return row[0] if row else None


def pose_to_sdf_with_h(pose_bytes, workdir):
    """Convert pose bytes (PDBQT or SDF) → SDF with explicit H atoms via obabel.

    Returns the SDF path on success, None on failure.
    """
    # Detect format
    if b"$$$$" in pose_bytes or b"\nM  END" in pose_bytes:
        in_fmt, in_file = "sdf", workdir / "pose_raw.sdf"
    else:
        in_fmt, in_file = "pdbqt", workdir / "pose_raw.pdbqt"
    in_file.write_bytes(pose_bytes)

    sdf_h = workdir / "pose_H.sdf"
    extra = ["--firstonly"] if in_fmt == "pdbqt" else []
    result = subprocess.run(
        ["obabel", f"-i{in_fmt}", str(in_file), "-osdf", "-O", str(sdf_h), "-h"] + extra,
        capture_output=True, text=True,
    )
    if result.returncode != 0 or not sdf_h.exists() or sdf_h.stat().st_size == 0:
        print(f"[obabel] pose→SDF+H failed: {result.stderr.strip()}", flush=True)
        return None
    return sdf_h


def smiles_to_sdf_with_h(smiles, workdir):
    """Generate 3D SDF with H from SMILES using RDKit ETKDGv3."""
    from rdkit import Chem
    from rdkit.Chem import AllChem
    mol = Chem.MolFromSmiles(smiles)
    mol = Chem.AddHs(mol)
    AllChem.EmbedMolecule(mol, AllChem.ETKDGv3())
    AllChem.MMFFOptimizeMolecule(mol)
    sdf_path = workdir / "ligand_H.sdf"
    with Chem.SDWriter(str(sdf_path)) as w:
        w.write(mol)
    return sdf_path


def sdf_to_xyz(sdf_path, workdir):
    """Convert SDF to XYZ via obabel (preserves H atom positions)."""
    xyz_path = workdir / "ligand.xyz"
    result = subprocess.run(
        ["obabel", "-isdf", str(sdf_path), "-oxyz", "-O", str(xyz_path)],
        capture_output=True, text=True,
    )
    if result.returncode != 0 or not xyz_path.exists() or xyz_path.stat().st_size == 0:
        raise RuntimeError(f"obabel SDF→XYZ failed: {result.stderr.strip()}")
    return xyz_path


def formal_charge(smiles):
    from rdkit import Chem
    mol = Chem.MolFromSmiles(smiles)
    return sum(atom.GetFormalCharge() for atom in mol.GetAtoms())


def write_orca_input(xyz_path, charge, nproc, maxcore, workdir):
    inp = workdir / "orca.inp"
    inp.write_text(
        f"! HF 6-31G* CHELPG TightSCF\n"
        f"%pal nprocs {nproc} end\n"
        f"%maxcore {maxcore}\n"
        f"\n"
        f"* xyzfile {charge} 1 {xyz_path}\n"
    )
    return inp


def run_orca(inp_path, workdir):
    out_path = workdir / "orca.out"
    print("[orca] Running HF/6-31G* CHELPG...", flush=True)
    with out_path.open("w") as fh:
        result = subprocess.run(
            ["orca", str(inp_path)],
            cwd=str(workdir),
            stdout=fh,
            stderr=subprocess.STDOUT,
        )
    if result.returncode != 0:
        tail = out_path.read_text().splitlines()[-20:]
        print("[orca] FAILED. Last output:", flush=True)
        for ln in tail:
            print(f"[orca] {ln}", flush=True)
        raise RuntimeError(f"ORCA exited {result.returncode}")
    print("[orca] SCF + CHELPG complete", flush=True)
    return out_path


def parse_chelpg_charges(out_path):
    """Extract CHELPG atomic charges from ORCA output.

    ORCA prints the final CHELPG block as:
      CHELPG Charges
      --------------
        0   C :    -0.239341
        1   H :     0.067450
        ...
      Total charge:    0.000000
    """
    charges = []
    in_block = False
    for line in Path(out_path).read_text().splitlines():
        stripped = line.strip()
        if "CHELPG Charges" in stripped:
            in_block = True
            charges = []  # reset — take the last block
            continue
        if not in_block:
            continue
        if stripped.startswith("---") or not stripped:
            continue
        if stripped.startswith("Total charge"):
            in_block = False
            continue
        parts = stripped.split()
        if len(parts) >= 4 and parts[2] == ":":
            try:
                charges.append(float(parts[3]))
            except ValueError:
                pass
    if not charges:
        raise RuntimeError("No CHELPG charges found in ORCA output — check orca.out")
    print(f"[orca] {len(charges)} CHELPG charges parsed", flush=True)
    return charges


def run_antechamber_resp(sdf_h_path, charges, charge, workdir):
    """Assign GAFF2 atom types and attach RESP charges → mol2.

    antechamber -c rc reads charges from a file (one float per line, atom order
    must match the input SDF). Since sdf_h_path was the obabel source for the
    XYZ fed to ORCA, the atom ordering is consistent.
    """
    crg_path = workdir / "resp.crg"
    crg_path.write_text("".join(f"{q:12.6f}\n" for q in charges))

    mol2_path = workdir / "LIG_resp.mol2"
    result = subprocess.run(
        [
            "antechamber",
            "-i", str(sdf_h_path), "-fi", "sdf",
            "-o", str(mol2_path), "-fo", "mol2",
            "-c", "rc", "-cf", str(crg_path),
            "-at", "gaff2",
            "-nc", str(charge),
            "-rn", "LIG",
            "-pf", "y",
        ],
        cwd=str(workdir),
        capture_output=True,
        text=True,
    )
    for ln in result.stdout.splitlines():
        print(f"[antechamber] {ln}", flush=True)
    if result.returncode != 0:
        print(f"[antechamber stderr] {result.stderr}", flush=True)
        raise RuntimeError(f"antechamber failed (exit {result.returncode})")
    if not mol2_path.exists():
        raise RuntimeError("antechamber did not produce mol2 output")
    print(f"[antechamber] mol2 with RESP charges written", flush=True)
    return mol2_path


def upload_s3(s3, bucket, key, path):
    if s3 is None:
        print(f"[s3] S3 disabled, skipping {key}", flush=True)
        return
    s3.upload_file(str(path), bucket, key)
    print(f"[s3] Uploaded → s3://{bucket}/{key}", flush=True)


def main():
    cfg = get_config()
    print(
        f"ORCA RESP worker: job={cfg['job_name']} compound={cfg['compound_id']} "
        f"nproc={cfg['nproc']} maxcore={cfg['maxcore']} MB/core",
        flush=True,
    )

    conn = connect_db(cfg)
    cursor = conn.cursor()
    s3 = get_s3()

    smiles = fetch_smiles(cursor, cfg["compound_id"])
    pose_bytes = fetch_pose_bytes(cursor, cfg["compound_id"], cfg["dock_job_name"])
    charge = formal_charge(smiles)
    print(f"SMILES: {smiles}  formal charge: {charge:+d}", flush=True)

    with tempfile.TemporaryDirectory(prefix="resp_") as tmpdir:
        wd = Path(tmpdir)

        # Geometry: prefer docked pose with explicit H, fall back to RDKit conformer
        sdf_h_path = None
        if pose_bytes:
            sdf_h_path = pose_to_sdf_with_h(pose_bytes, wd)
            if sdf_h_path:
                print("[prep] Using docked pose geometry for ORCA", flush=True)
            else:
                print("[prep] Pose conversion failed; generating RDKit conformer", flush=True)
        if sdf_h_path is None:
            sdf_h_path = smiles_to_sdf_with_h(smiles, wd)
            print("[prep] RDKit ETKDGv3 conformer generated", flush=True)

        xyz_path = sdf_to_xyz(sdf_h_path, wd)

        # ORCA: HF/6-31G* CHELPG
        inp_path = write_orca_input(xyz_path, charge, cfg["nproc"], cfg["maxcore"], wd)
        out_path = run_orca(inp_path, wd)
        charges = parse_chelpg_charges(out_path)

        # antechamber: GAFF2 atom types + RESP charges → mol2
        mol2_path = run_antechamber_resp(sdf_h_path, charges, charge, wd)

        # Upload results
        prefix = f"resp/{cfg['compound_id']}"
        upload_s3(s3, BUCKET_RESP, f"{prefix}/LIG.mol2", mol2_path)
        upload_s3(s3, BUCKET_RESP, f"{prefix}/orca.out", out_path)

    print(
        f"RESP complete: {cfg['compound_id']} → s3://{BUCKET_RESP}/resp/{cfg['compound_id']}/LIG.mol2",
        flush=True,
    )
    cursor.close()
    conn.close()


if __name__ == "__main__":
    main()
