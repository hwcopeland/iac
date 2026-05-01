#!/usr/bin/env python3
"""PySCF CHELPG charge worker for Khemeia ligand force field preparation.

Runs HF/6-31G* via PySCF, fits CHELPG charges with a custom Connolly-surface
grid + constrained least squares, then assigns GAFF2 atom types via antechamber.
The mol2 is stored in S3 and consumed automatically by the GROMACS worker before
it falls back to Gasteiger charges.

Workflow:
  1. Fetch compound SMILES and best docked pose from DB.
  2. Add explicit H to pose via obabel; fall back to RDKit ETKDGv3 conformer.
  3. PySCF RHF/6-31G* → density matrix.
  4. Compute ESP on Connolly-style surface grid via PySCF int1e_grids.
  5. Fit CHELPG charges (constrained LS, sum = formal charge).
  6. antechamber -c rc assigns GAFF2 atom types + embeds charges → mol2.
  7. Upload mol2 + pyscf log to s3://khemeia-resp/resp/<compound_id>/.

Configuration via environment variables:
  JOB_NAME          - RESP job identifier
  COMPOUND_ID       - KHM-* identifier
  DOCK_JOB_NAME     - docking job for pose geometry (optional)
  PYSCF_NPROC       - CPU threads for PySCF (default: 4)
  PYSCF_MEMORY_MB   - memory for PySCF in MB (default: 4000)
  MYSQL_HOST / MYSQL_PORT / MYSQL_USER / MYSQL_PASSWORD
  GARAGE_ENABLED / GARAGE_ENDPOINT / GARAGE_ACCESS_KEY / GARAGE_SECRET_KEY / GARAGE_REGION
"""

import os
import subprocess
import sys
import tempfile
from pathlib import Path

import numpy as np
from scipy.linalg import solve

import mysql.connector

try:
    import boto3
    from botocore.config import Config as BotoConfig
except ImportError:
    boto3 = None

ANG2BOHR = 1.8897259886

# Bondi VDW radii (Angstrom) used by CHELPG
_VDW = {
    "H": 1.20, "C": 1.70, "N": 1.55, "O": 1.52, "F": 1.47,
    "P": 1.80, "S": 1.80, "Cl": 1.75, "Br": 1.85, "I": 1.98,
}
_VDW_DEFAULT = 1.70

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
        "nproc":         int(os.environ.get("PYSCF_NPROC", "4")),
        "memory_mb":     int(os.environ.get("PYSCF_MEMORY_MB", "4000")),
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
    """Convert pose (PDBQT or SDF) → SDF with explicit H via obabel."""
    if b"$$$$" in pose_bytes or b"\nM  END" in pose_bytes:
        in_fmt, in_file = "sdf", workdir / "pose_raw.sdf"
    else:
        in_fmt, in_file = "pdbqt", workdir / "pose_raw.pdbqt"
    in_file.write_bytes(pose_bytes)

    extra = ["--firstonly"] if in_fmt == "pdbqt" else []
    sdf_h = workdir / "pose_H.sdf"
    result = subprocess.run(
        ["obabel", f"-i{in_fmt}", str(in_file), "-osdf", "-O", str(sdf_h), "-h"] + extra,
        capture_output=True, text=True,
    )
    if result.returncode != 0 or not sdf_h.exists() or sdf_h.stat().st_size == 0:
        print(f"[obabel] pose→SDF+H failed: {result.stderr.strip()}", flush=True)
        return None
    return sdf_h


def smiles_to_sdf_with_h(smiles, workdir):
    """Generate 3D SDF with H from SMILES using RDKit ETKDGv3 + MMFF94."""
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


def formal_charge(smiles):
    from rdkit import Chem
    mol = Chem.MolFromSmiles(smiles)
    return sum(atom.GetFormalCharge() for atom in mol.GetAtoms())


def parse_sdf_atoms(sdf_path):
    """Return list of (symbol, x_ang, y_ang, z_ang) from SDF V2000 atom block."""
    lines = Path(sdf_path).read_text().splitlines()
    try:
        natoms = int(lines[3][:3])
    except (IndexError, ValueError):
        raise RuntimeError(f"Cannot parse atom count from SDF: {sdf_path}")
    atoms = []
    for line in lines[4:4 + natoms]:
        parts = line.split()
        if len(parts) >= 4:
            atoms.append((parts[3], float(parts[0]), float(parts[1]), float(parts[2])))
    if len(atoms) != natoms:
        raise RuntimeError(f"SDF atom count mismatch: header={natoms} parsed={len(atoms)}")
    return atoms


def _fib_sphere(n, r, center):
    """n uniform points on sphere of radius r centred at center (Fibonacci method)."""
    gr = (1 + 5 ** 0.5) / 2
    i = np.arange(n)
    theta = np.arccos(1 - 2 * (i + 0.5) / n)
    phi = 2 * np.pi * i / gr
    pts = np.column_stack([
        r * np.sin(theta) * np.cos(phi),
        r * np.sin(theta) * np.sin(phi),
        r * np.cos(theta),
    ]) + center
    return pts


def make_chelpg_grid(coords_bohr, symbols, pts_per_shell=100):
    """Build CHELPG ESP-fitting grid in Bohr.

    Generates shells at 1.4–2.8× VDW for each atom, then keeps only points
    that are outside 1.4×VDW of every atom (so not buried inside the molecule).
    """
    vdw_bohr = np.array([_VDW.get(s, _VDW_DEFAULT) * ANG2BOHR for s in symbols])
    shells = [1.4, 1.6, 1.8, 2.0, 2.2, 2.4, 2.6, 2.8]
    pts = np.vstack([
        _fib_sphere(pts_per_shell, sc * vdw_bohr[i], coords_bohr[i])
        for sc in shells
        for i in range(len(symbols))
    ])
    # dists[g, i] = distance from grid point g to atom i
    dists = np.linalg.norm(pts[:, None, :] - coords_bohr[None, :, :], axis=2)
    keep = np.all(dists >= 1.4 * vdw_bohr[None, :], axis=1)
    grid = pts[keep]
    print(f"[chelpg] Grid: {keep.sum()}/{len(pts)} points", flush=True)
    return grid


def compute_esp(mol, dm, grid_bohr):
    """Total ESP (nuclear + electronic) at grid_bohr points, in Hartree/e."""
    # Nuclear
    esp = np.zeros(len(grid_bohr))
    for i in range(mol.natm):
        Z = mol.atom_charge(i)
        diffs = grid_bohr - mol.atom_coord(i)
        esp += Z / np.linalg.norm(diffs, axis=1)
    # Electronic (int1e_grids: shape (ngrids, nao, nao))
    v = mol.intor("int1e_grids", grids=grid_bohr)
    esp -= np.einsum("gij,ij->g", v, dm)
    return esp


def fit_chelpg_charges(grid_bohr, esp, coords_bohr, total_charge):
    """Constrained least-squares CHELPG charge fit.

    Minimise ||A·q - v||² subject to Σq = total_charge via Lagrange multiplier.
    """
    n = len(coords_bohr)
    A = 1.0 / np.linalg.norm(
        grid_bohr[:, None, :] - coords_bohr[None, :, :], axis=2
    )  # (ngrids, natom)
    M = np.zeros((n + 1, n + 1))
    M[:n, :n] = A.T @ A
    M[:n, n] = 1.0
    M[n, :n] = 1.0
    rhs = np.zeros(n + 1)
    rhs[:n] = A.T @ esp
    rhs[n] = total_charge
    return solve(M, rhs)[:n]


def run_pyscf_chelpg(sdf_h_path, total_charge, nproc, memory_mb):
    """Run PySCF RHF/6-31G* + CHELPG charge fitting.

    Returns numpy array of charges in the same atom order as sdf_h_path.
    """
    from pyscf import gto, scf

    atoms_ang = parse_sdf_atoms(sdf_h_path)
    symbols = [a[0] for a in atoms_ang]
    coords_ang = np.array([[a[1], a[2], a[3]] for a in atoms_ang])
    coords_bohr = coords_ang * ANG2BOHR

    print(f"[pyscf] RHF/6-31G* on {len(atoms_ang)} atoms ({nproc} threads, {memory_mb} MB)...", flush=True)

    mol = gto.Mole()
    mol.atom = [[s, tuple(c)] for s, c in zip(symbols, coords_ang)]
    mol.basis = "6-31g*"
    mol.charge = total_charge
    mol.spin = 0
    mol.verbose = 3
    mol.max_memory = memory_mb
    mol.build()

    mf = scf.RHF(mol)
    mf.max_cycle = 200
    import pyscf
    pyscf.lib.num_threads(nproc)
    mf.kernel()
    if not mf.converged:
        raise RuntimeError("PySCF RHF did not converge")
    print(f"[pyscf] HF energy: {mf.e_tot:.8f} Hartree", flush=True)

    dm = mf.make_rdm1()
    grid = make_chelpg_grid(coords_bohr, symbols)
    esp = compute_esp(mol, dm, grid)
    charges = fit_chelpg_charges(grid, esp, coords_bohr, float(total_charge))

    print("[pyscf] CHELPG charges:", flush=True)
    for sym, q in zip(symbols, charges):
        print(f"[pyscf]   {sym}: {q:+.4f}", flush=True)
    print(f"[pyscf]   sum: {charges.sum():+.6f}", flush=True)

    return charges


def run_antechamber(sdf_h_path, charges, total_charge, workdir):
    """Assign GAFF2 atom types and embed CHELPG charges → mol2.

    antechamber -c rc reads one charge per line (atom order = SDF order).
    sdf_h_path is the same SDF used for the PySCF geometry, so ordering matches.
    """
    crg_path = workdir / "chelpg.crg"
    crg_path.write_text("".join(f"{q:12.6f}\n" for q in charges))

    mol2_path = workdir / "LIG_resp.mol2"
    result = subprocess.run(
        [
            "antechamber",
            "-i", str(sdf_h_path), "-fi", "sdf",
            "-o", str(mol2_path), "-fo", "mol2",
            "-c", "rc", "-cf", str(crg_path),
            "-at", "gaff2",
            "-nc", str(total_charge),
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
        raise RuntimeError("antechamber produced no mol2 output")
    print("[antechamber] mol2 written with CHELPG charges", flush=True)
    return mol2_path


def upload_s3(s3, bucket, key, path):
    if s3 is None:
        print(f"[s3] disabled — skipping {key}", flush=True)
        return
    s3.upload_file(str(path), bucket, key)
    print(f"[s3] → s3://{bucket}/{key}", flush=True)


def main():
    cfg = get_config()
    print(
        f"PySCF CHELPG worker: job={cfg['job_name']} compound={cfg['compound_id']} "
        f"nproc={cfg['nproc']} memory={cfg['memory_mb']} MB",
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

        # Geometry: docked pose with H preferred; RDKit conformer as fallback
        sdf_h_path = None
        if pose_bytes:
            sdf_h_path = pose_to_sdf_with_h(pose_bytes, wd)
            if sdf_h_path:
                print("[prep] Using docked pose geometry for PySCF", flush=True)
        if sdf_h_path is None:
            sdf_h_path = smiles_to_sdf_with_h(smiles, wd)
            print("[prep] RDKit ETKDGv3 conformer generated", flush=True)

        # PySCF HF/6-31G* + CHELPG
        charges = run_pyscf_chelpg(sdf_h_path, charge, cfg["nproc"], cfg["memory_mb"])

        # antechamber: GAFF2 types + CHELPG charges → mol2
        mol2_path = run_antechamber(sdf_h_path, charges, charge, wd)

        # Upload to S3
        prefix = f"resp/{cfg['compound_id']}"
        upload_s3(s3, BUCKET_RESP, f"{prefix}/LIG.mol2", mol2_path)

    print(
        f"RESP complete: {cfg['compound_id']} → s3://{BUCKET_RESP}/resp/{cfg['compound_id']}/LIG.mol2",
        flush=True,
    )
    cursor.close()
    conn.close()


if __name__ == "__main__":
    main()
