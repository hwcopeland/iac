#!/usr/bin/env python3
"""GROMACS GPU molecular dynamics worker for Khemeia v2 pipeline.

Takes the top docked pose for a compound from docking_v2_results and runs a
protein-ligand MD simulation:

  1. Fetch receptor (PDBQT → PDB) from S3 + docked pose from DB.
  2. Prepare protein topology — CHARMM36m via gmx pdb2gmx.
  3. Prepare ligand topology — OpenFF GAFF-2.11 via openff-toolkit.
  4. Assemble complex, solvate (TIP3P-CHARMM), neutralise with NaCl.
  5. Energy minimise → NVT equilibration → NPT equilibration → Production MD.
  6. Store trajectory (.xtc) and energy (.edr) in S3; write summary to DB.

All configuration via environment variables:
  JOB_NAME          - MD job identifier
  WORKER_NAME       - unique pod name
  COMPOUND_ID       - KHM-* identifier to simulate
  RECEPTOR_REF      - target-prep job name (resolves receptor S3 key)
  DOCK_JOB_NAME     - docking v2 job to pull the pose from
  DOCK_ENGINE       - which engine's pose to use (default: best available)
  MD_NSTEPS         - production MD steps (default: 500000 = 1 ns at 2 fs)
  MD_BOX_PADDING    - nm from protein to box edge (default: 1.2)
  MYSQL_HOST / MYSQL_PORT / MYSQL_USER / MYSQL_PASSWORD
  GARAGE_ENABLED / GARAGE_ENDPOINT / GARAGE_ACCESS_KEY / GARAGE_SECRET_KEY / GARAGE_REGION
"""

import json
import os
import shutil
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

MDP_DIR = Path("/mdp")
BUCKET_RECEPTORS = "khemeia-receptors"
BUCKET_MD = "khemeia-md"

# PDBQT records with no PDB equivalent
_PDBQT_ONLY = frozenset({"ROOT", "ENDROOT", "BRANCH", "ENDBRANCH", "TORSDOF"})


def require_env(name):
    val = os.environ.get(name)
    if not val:
        print(f"FATAL: {name} not set", flush=True)
        sys.exit(1)
    return val


def get_config():
    return {
        "job_name":     require_env("JOB_NAME"),
        "worker_name":  require_env("WORKER_NAME"),
        "compound_id":  require_env("COMPOUND_ID"),
        "receptor_ref": require_env("RECEPTOR_REF"),
        "dock_job_name": require_env("DOCK_JOB_NAME"),
        "dock_engine":  os.environ.get("DOCK_ENGINE", ""),
        "nsteps":       int(os.environ.get("MD_NSTEPS", "500000")),
        "box_padding":  float(os.environ.get("MD_BOX_PADDING", "1.2")),
        "mysql_host":   os.environ.get("MYSQL_HOST", "localhost"),
        "mysql_port":   int(os.environ.get("MYSQL_PORT", "3306")),
        "mysql_user":   os.environ.get("MYSQL_USER", "root"),
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


def pdbqt_to_pdb(pdbqt_bytes):
    lines = []
    for line in pdbqt_bytes.decode("utf-8", errors="replace").splitlines():
        record = line[:6].strip().upper()
        if record in _PDBQT_ONLY:
            continue
        if record in ("ATOM", "HETATM"):
            lines.append(line[:66].ljust(66) + "\n")
        else:
            lines.append(line + "\n")
    return "".join(lines).encode("utf-8")


def fetch_receptor_pdb(cursor, s3, receptor_ref):
    cursor.execute(
        "SELECT receptor_s3_key FROM target_prep_results WHERE name = %s",
        (receptor_ref,),
    )
    row = cursor.fetchone()
    if row is None:
        print(f"FATAL: receptor_ref '{receptor_ref}' not found", flush=True)
        sys.exit(1)
    data = s3.get_object(Bucket=BUCKET_RECEPTORS, Key=row[0])["Body"].read()
    return pdbqt_to_pdb(data)


def fetch_docked_pose(cursor, compound_id, dock_job_name, dock_engine):
    """Return (engine, affinity, pose_bytes) for the best available pose."""
    if dock_engine:
        cursor.execute(
            "SELECT engine, affinity_kcal_mol, docked_pdbqt "
            "FROM docking_v2_results "
            "WHERE job_name=%s AND compound_id=%s AND engine=%s "
            "ORDER BY affinity_kcal_mol ASC LIMIT 1",
            (dock_job_name, compound_id, dock_engine),
        )
    else:
        cursor.execute(
            "SELECT engine, affinity_kcal_mol, docked_pdbqt "
            "FROM docking_v2_results "
            "WHERE job_name=%s AND compound_id=%s "
            "ORDER BY affinity_kcal_mol ASC LIMIT 1",
            (dock_job_name, compound_id),
        )
    row = cursor.fetchone()
    if row is None:
        print(f"FATAL: no docked pose for {compound_id} in {dock_job_name}", flush=True)
        sys.exit(1)
    engine, affinity, pose_bytes = row
    print(f"Pose: engine={engine} affinity={affinity} kcal/mol", flush=True)
    return engine, affinity, pose_bytes


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


def run_gmx(args, cwd, description):
    """Run a gmx subcommand, streaming output. Exits on non-zero return."""
    cmd = ["gmx", "-quiet"] + args
    print(f"[gmx] {description}", flush=True)
    result = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True)
    if result.stdout:
        for line in result.stdout.splitlines():
            print(f"[gmx] {line}", flush=True)
    if result.stderr:
        for line in result.stderr.splitlines():
            print(f"[gmx] {line}", flush=True)
    if result.returncode != 0:
        print(f"FATAL: gmx {args[0]} failed (exit {result.returncode})", flush=True)
        sys.exit(result.returncode)


def prepare_ligand_topology(smiles, pose_sdf_path, workdir):
    """Generate GROMACS topology for the ligand using ACPYPE + GAFF2.

    Uses the 3D coordinates from the docked pose (pose_sdf_path) so the
    starting conformation is the docked geometry. Falls back to an RDKit
    conformer if no pose file is available.

    Returns paths to (LIG_GMX.gro, LIG_GMX.itp) in the ACPYPE output dir.
    """
    workdir = Path(workdir)

    if pose_sdf_path and Path(pose_sdf_path).exists():
        lig_input = str(pose_sdf_path)
        print("[acpype] Using docked pose SDF as ligand input", flush=True)
    else:
        from rdkit import Chem
        from rdkit.Chem import AllChem
        mol = Chem.MolFromSmiles(smiles)
        mol = Chem.AddHs(mol)
        AllChem.EmbedMolecule(mol, AllChem.ETKDGv3())
        lig_sdf = str(workdir / "ligand_3d.sdf")
        with Chem.SDWriter(lig_sdf) as w:
            w.write(mol)
        lig_input = lig_sdf
        print("[acpype] Generated RDKit conformer from SMILES", flush=True)

    # ACPYPE: GAFF2 atom types, Gasteiger charges (no AmberTools required)
    result = subprocess.run(
        ["acpype", "-i", lig_input, "-b", "LIG", "-c", "gas", "-a", "gaff2"],
        cwd=str(workdir),
        capture_output=True,
        text=True,
    )
    for line in result.stdout.splitlines():
        print(f"[acpype] {line}", flush=True)
    if result.returncode != 0:
        print(f"[acpype stderr] {result.stderr}", flush=True)
        raise RuntimeError(f"ACPYPE failed (exit {result.returncode})")

    acpype_dir = workdir / "LIG.acpype"
    gro = acpype_dir / "LIG_GMX.gro"
    itp = acpype_dir / "LIG_GMX.itp"
    print(f"[acpype] Topology written to {acpype_dir}", flush=True)
    return gro, itp



def prepare_protein_topology(receptor_pdb_path, workdir):
    """Run gmx pdb2gmx with CHARMM36m, TIP3P-CHARMM water, no H addition.

    Returns (protein.gro, topol.top).
    """
    workdir = Path(workdir)
    run_gmx(
        [
            "pdb2gmx",
            "-f", str(receptor_pdb_path),
            "-o", "protein.gro",
            "-p", "topol.top",
            "-ff", "charmm36m-jul2022",
            "-water", "tip3p",
            "-ignh",
        ],
        cwd=str(workdir),
        description="pdb2gmx — CHARMM36m protein topology",
    )
    return workdir / "protein.gro", workdir / "topol.top"


def assemble_complex(protein_gro, ligand_gro, protein_top, ligand_itp, workdir):
    """Combine protein.gro + ligand.gro → complex.gro and patch topology.

    Appends the ligand .itp include and molecule entry to topol.top.
    Returns complex.gro path.
    """
    workdir = Path(workdir)
    complex_gro = workdir / "complex.gro"

    p_lines = Path(protein_gro).read_text().splitlines()
    l_lines = Path(ligand_gro).read_text().splitlines()

    # GRO format: line 0 = title, line 1 = atom count, lines 2..N-1 = atoms, last = box
    p_atoms = [ln for ln in p_lines[2:-1] if ln.strip()]
    l_atoms = [ln for ln in l_lines[2:-1] if ln.strip()]
    box = p_lines[-1]  # keep protein box dimensions
    total = len(p_atoms) + len(l_atoms)

    with complex_gro.open("w") as fh:
        fh.write("Protein-Ligand complex\n")
        fh.write(f"{total}\n")
        for ln in p_atoms:
            fh.write(ln + "\n")
        for ln in l_atoms:
            fh.write(ln + "\n")
        fh.write(box + "\n")

    print(f"[assemble] Complex: {len(p_atoms)} protein + {len(l_atoms)} ligand atoms", flush=True)

    # Patch topol.top: add ligand itp include before [ system ], add molecule entry at end.
    top_text = Path(protein_top).read_text()
    itp_include = f'\n; Ligand topology\n#include "{ligand_itp}"\n'

    # Insert before first [ system ] section
    if "[ system ]" in top_text:
        top_text = top_text.replace("[ system ]", itp_include + "[ system ]", 1)
    else:
        top_text += itp_include

    # Append ligand to [ molecules ] — detect molecule name from itp
    lig_mol_name = _detect_mol_name(ligand_itp)
    if "[ molecules ]" in top_text:
        top_text = top_text.rstrip() + f"\n{lig_mol_name}   1\n"
    else:
        top_text += f"\n[ molecules ]\n{lig_mol_name}   1\n"

    Path(protein_top).write_text(top_text)
    return complex_gro


def _detect_mol_name(itp_path):
    """Extract the first moleculetype name from an .itp file."""
    in_moltype = False
    for line in Path(itp_path).read_text().splitlines():
        stripped = line.strip()
        if stripped == "[ moleculetype ]":
            in_moltype = True
            continue
        if in_moltype and stripped and not stripped.startswith(";"):
            return stripped.split()[0]
    return "LIG"


def solvate_and_ions(complex_gro, topol_top, box_padding, workdir):
    """Add TIP3P water box and NaCl ions at 0.15 M."""
    workdir = Path(workdir)

    # Define periodic box
    run_gmx(
        ["editconf", "-f", "complex.gro", "-o", "boxed.gro",
         "-c", "-d", str(box_padding), "-bt", "dodecahedron"],
        cwd=str(workdir),
        description=f"editconf — dodecahedron box (padding={box_padding} nm)",
    )

    # Solvate
    run_gmx(
        ["solvate", "-cp", "boxed.gro", "-cs", "spc216.gro",
         "-o", "solvated.gro", "-p", "topol.top"],
        cwd=str(workdir),
        description="solvate — TIP3P water",
    )

    # Add ions (requires a dummy EM run first to generate tpr)
    run_gmx(
        ["grompp", "-f", str(MDP_DIR / "em.mdp"), "-c", "solvated.gro",
         "-p", "topol.top", "-o", "ions.tpr", "-maxwarn", "2"],
        cwd=str(workdir),
        description="grompp — prepare for ion addition",
    )
    run_gmx(
        ["genion", "-s", "ions.tpr", "-o", "ionised.gro",
         "-p", "topol.top", "-pname", "NA", "-nname", "CL",
         "-neutral", "-conc", "0.15"],
        cwd=str(workdir),
        description="genion — NaCl 0.15 M + neutralise",
    )

    return workdir / "ionised.gro"


def run_md_step(name, mdp, input_gro, topol_top, workdir, prev_cpt=None, gpu=True):
    """grompp + mdrun for one MD step. Returns output .gro path."""
    workdir = Path(workdir)
    tpr = workdir / f"{name}.tpr"
    grompp_args = [
        "grompp", "-f", str(mdp), "-c", str(input_gro),
        "-p", str(topol_top), "-o", str(tpr), "-maxwarn", "2",
    ]
    if prev_cpt:
        grompp_args += ["-t", str(prev_cpt)]
    run_gmx(grompp_args, cwd=str(workdir), description=f"grompp — {name}")

    mdrun_args = [
        "mdrun", "-v", "-deffnm", name,
        "-ntmpi", "1", "-ntomp", "4",
    ]
    if gpu:
        mdrun_args += ["-gpu_id", "0", "-nb", "gpu", "-pme", "gpu"]

    run_gmx(mdrun_args, cwd=str(workdir), description=f"mdrun — {name}")
    return workdir / f"{name}.gro", workdir / f"{name}.cpt"


def upload_s3(s3, bucket, key, path):
    if s3 is None:
        print(f"[s3] S3 disabled, skipping upload of {path}", flush=True)
        return
    s3.upload_file(str(path), bucket, key)
    print(f"[s3] Uploaded {path} → s3://{bucket}/{key}", flush=True)


def write_result(cursor, conn, cfg, engine, affinity, duration_s, traj_key, energy_key):
    cursor.execute(
        "INSERT INTO md_results "
        "(job_name, compound_id, dock_engine, dock_affinity_kcal_mol, "
        " duration_s, trajectory_s3_key, energy_s3_key, created_at) "
        "VALUES (%s, %s, %s, %s, %s, %s, %s, NOW())",
        (cfg["job_name"], cfg["compound_id"], engine, affinity,
         round(duration_s), traj_key, energy_key),
    )
    conn.commit()


def main():
    cfg = get_config()
    print(
        f"GROMACS MD worker: job={cfg['job_name']} compound={cfg['compound_id']} "
        f"receptor={cfg['receptor_ref']} nsteps={cfg['nsteps']}",
        flush=True,
    )

    conn = connect_db(cfg)
    cursor = conn.cursor()
    s3 = get_s3()

    receptor_pdb = fetch_receptor_pdb(cursor, s3, cfg["receptor_ref"])
    dock_engine, dock_affinity, pose_bytes = fetch_docked_pose(
        cursor, cfg["compound_id"], cfg["dock_job_name"], cfg["dock_engine"],
    )
    smiles = fetch_smiles(cursor, cfg["compound_id"])

    with tempfile.TemporaryDirectory(prefix="md_") as tmpdir:
        wd = Path(tmpdir)

        # Write receptor PDB
        receptor_path = wd / "receptor.pdb"
        receptor_path.write_bytes(receptor_pdb)

        # Write docked pose and normalise to SDF for ACPYPE.
        # Vina PDBQT is multi-model; ACPYPE's internal obabel call chokes on it.
        # We run our own obabel conversion first (first conformer only → SDF).
        pose_path = None
        if pose_bytes:
            if b"$$$$" in pose_bytes or b"\nM  END" in pose_bytes:
                pose_path = wd / "pose.sdf"
                pose_path.write_bytes(pose_bytes)
            else:
                raw_pdbqt = wd / "pose_raw.pdbqt"
                raw_pdbqt.write_bytes(pose_bytes)
                sdf_out = wd / "pose.sdf"
                conv = subprocess.run(
                    ["obabel", "-ipdbqt", str(raw_pdbqt), "-osdf", "-O", str(sdf_out), "--firstonly"],
                    capture_output=True, text=True,
                )
                if conv.returncode == 0 and sdf_out.exists() and sdf_out.stat().st_size > 0:
                    pose_path = sdf_out
                    print(f"[pose] Converted PDBQT→SDF ({sdf_out.stat().st_size} bytes)", flush=True)
                else:
                    print(f"[pose] obabel conversion failed (rc={conv.returncode}), using raw PDBQT", flush=True)
                    pose_path = raw_pdbqt

        # --- Topology preparation ---
        print("Preparing ligand topology (OpenFF GAFF-2.11)...", flush=True)
        ligand_gro, ligand_itp = prepare_ligand_topology(smiles, pose_path, wd)

        print("Preparing protein topology (CHARMM36m)...", flush=True)
        protein_gro, topol_top = prepare_protein_topology(receptor_path, wd)

        print("Assembling complex...", flush=True)
        complex_gro = assemble_complex(protein_gro, ligand_gro, topol_top, ligand_itp, wd)

        # Patch nsteps in production MDP
        md_mdp = wd / "md_run.mdp"
        md_mdp_text = (MDP_DIR / "md.mdp").read_text()
        md_mdp_text = md_mdp_text.replace("nsteps      = 500000", f"nsteps      = {cfg['nsteps']}")
        md_mdp.write_text(md_mdp_text)

        # --- Solvation ---
        print("Solvating system...", flush=True)
        ionised_gro = solvate_and_ions(complex_gro, topol_top, cfg["box_padding"], wd)

        t0 = _time.time()

        # --- MD pipeline ---
        print("Step 1/4: Energy minimisation...", flush=True)
        em_gro, _ = run_md_step("em", MDP_DIR / "em.mdp", ionised_gro, topol_top, wd, gpu=False)

        print("Step 2/4: NVT equilibration (100 ps)...", flush=True)
        nvt_gro, nvt_cpt = run_md_step("nvt", MDP_DIR / "nvt.mdp", em_gro, topol_top, wd)

        print("Step 3/4: NPT equilibration (100 ps)...", flush=True)
        npt_gro, npt_cpt = run_md_step(
            "npt", MDP_DIR / "npt.mdp", nvt_gro, topol_top, wd, prev_cpt=nvt_cpt,
        )

        print(f"Step 4/4: Production MD ({cfg['nsteps']} steps)...", flush=True)
        _, _ = run_md_step(
            "md", md_mdp, npt_gro, topol_top, wd, prev_cpt=npt_cpt,
        )

        duration = _time.time() - t0
        print(f"MD complete in {duration:.0f}s", flush=True)

        # --- Upload results ---
        prefix = f"md/{cfg['job_name']}/{cfg['compound_id']}"
        traj_key = f"{prefix}/md.xtc"
        energy_key = f"{prefix}/md.edr"
        upload_s3(s3, BUCKET_MD, traj_key, wd / "md.xtc")
        upload_s3(s3, BUCKET_MD, energy_key, wd / "md.edr")

        write_result(cursor, conn, cfg, dock_engine, dock_affinity, duration, traj_key, energy_key)
        print(f"Result written to DB: {cfg['compound_id']} duration={duration:.0f}s", flush=True)

    cursor.close()
    conn.close()


if __name__ == "__main__":
    main()
