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
BUCKET_MD = "khemeia-trajectories"

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


def strip_hetatm(pdb_bytes):
    """Remove HETATM records (ions, cofactors, crystallographic waters).

    pdb2gmx requires a clean protein-only PDB; HETATM residues mixed into a
    protein chain cause a fatal 'inconsistent type' error.
    """
    lines = []
    for line in pdb_bytes.decode("utf-8", errors="replace").splitlines():
        record = line[:6].strip().upper()
        if record not in ("HETATM",):
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


def run_gmx(args, cwd, description, stdin_input=None):
    """Run a gmx subcommand, streaming output. Exits on non-zero return."""
    cmd = ["gmx", "-quiet"] + args
    print(f"[gmx] {description}", flush=True)
    result = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, input=stdin_input)
    if result.stdout:
        for line in result.stdout.splitlines():
            print(f"[gmx] {line}", flush=True)
    if result.stderr:
        for line in result.stderr.splitlines():
            print(f"[gmx] {line}", flush=True)
    if result.returncode != 0:
        print(f"FATAL: gmx {args[0]} failed (exit {result.returncode})", flush=True)
        sys.exit(result.returncode)


import re as _re

# GRO coordinates always use 3 decimal places (%8.3f).  When a value overflows
# the 8-char field the subsequent fields are shifted, so fixed-position slicing
# breaks.  Match exactly-3-decimal floats to parse robustly in all cases.
_GRO_COORD_RE = _re.compile(r'[-+]?\d+\.\d{3}')


def _parse_gro_coords(line):
    """Return (x, y, z) from a GRO atom line, robust to overflowed fields."""
    nums = _GRO_COORD_RE.findall(line[20:])
    if len(nums) < 3:
        raise ValueError(f"cannot parse coords from: {line!r}")
    return float(nums[0]), float(nums[1]), float(nums[2])


def _gro_has_bad_coords(gro_path, max_nm=500.0):
    """Return True if any atom coordinate magnitude exceeds max_nm."""
    for line in Path(gro_path).read_text().splitlines()[2:-1]:
        if not line.strip():
            continue
        try:
            x, y, z = _parse_gro_coords(line)
            if abs(x) > max_nm or abs(y) > max_nm or abs(z) > max_nm:
                return True
        except ValueError:
            continue
    return False


def _parse_sdf_coords_nm(sdf_path):
    """Return list of (x, y, z) in nm from the SDF/MOL atom block (first conformer).
    Returns [] if any coordinate is non-finite (inf/nan).
    """
    import math as _math
    lines = Path(sdf_path).read_text().splitlines()
    try:
        natoms = int(lines[3][:3])
    except (IndexError, ValueError):
        return []
    coords = []
    for line in lines[4:4 + natoms]:
        parts = line.split()
        if len(parts) >= 3:
            try:
                x, y, z = float(parts[0]) / 10, float(parts[1]) / 10, float(parts[2]) / 10
                if not (_math.isfinite(x) and _math.isfinite(y) and _math.isfinite(z)):
                    print(f"[acpype] non-finite coord in SDF: ({x}, {y}, {z})", flush=True)
                    return []
                if abs(x) > 500 or abs(y) > 500 or abs(z) > 500:
                    print(f"[acpype] unreasonable coord in SDF (>{500} nm): ({x:.1f}, {y:.1f}, {z:.1f})", flush=True)
                    return []
                coords.append((x, y, z))
            except ValueError:
                pass
    return coords


def _overwrite_gro_coords(gro_path, coords_nm):
    """Replace GRO atom coordinates with coords_nm (list of (x,y,z) in nm).

    Keeps all atom meta-data (residue name, atom name, number) unchanged so
    the GRO still matches the ACPYPE topology.  If the atom count does not
    match, returns False without modifying the file.
    """
    gro_lines = Path(gro_path).read_text().splitlines(keepends=True)
    atom_lines = [ln for ln in gro_lines[2:-1] if ln.strip()]
    if len(atom_lines) != len(coords_nm):
        print(f"[acpype] coord count mismatch: GRO {len(atom_lines)} vs SDF {len(coords_nm)}", flush=True)
        return False
    new_lines = list(gro_lines[:2])
    for ln, (x, y, z) in zip(atom_lines, coords_nm):
        new_lines.append(f"{ln[:20]}{x:8.3f}{y:8.3f}{z:8.3f}\n")
    new_lines.append(gro_lines[-1])
    Path(gro_path).write_text("".join(new_lines))
    return True


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

    # ACPYPE: GAFF2 atom types, Gasteiger charges
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

    # tleap regenerates H atom positions from GAFF2 parameters, ignoring input
    # coordinates — H atoms end up at arbitrary positions (millions of nm).
    # Fix: add H atoms to the docked pose SDF with obabel, then overwrite all
    # ACPYPE GRO coordinates directly from that SDF.  ACPYPE/antechamber
    # preserves SDF heavy-atom ordering, and obabel appends H in the same
    # sequence, so atom index N in GRO == atom index N in pose_H.sdf.
    if pose_sdf_path and Path(pose_sdf_path).exists():
        pose_h_sdf = workdir / "pose_H.sdf"
        h_result = subprocess.run(
            ["obabel", "-isdf", str(pose_sdf_path), "-osdf", "-O", str(pose_h_sdf), "-h"],
            capture_output=True, text=True,
        )
        obabel_ok = h_result.returncode == 0 and pose_h_sdf.exists() and pose_h_sdf.stat().st_size > 0
        # Try H-added SDF first; _parse_sdf_coords_nm returns [] if any coord is inf/nan
        coords, src_label = [], "none"
        if obabel_ok:
            coords = _parse_sdf_coords_nm(pose_h_sdf)
            src_label = "pose_H.sdf"
        if not coords:
            coords = _parse_sdf_coords_nm(pose_sdf_path)
            src_label = "pose.sdf (fallback)"
        print(f"[acpype] coord injection: src={src_label} natoms={len(coords)}", flush=True)
        if coords:
            print(f"[acpype] coord range: x=[{min(c[0] for c in coords):.3f},{max(c[0] for c in coords):.3f}] "
                  f"y=[{min(c[1] for c in coords):.3f},{max(c[1] for c in coords):.3f}] "
                  f"z=[{min(c[2] for c in coords):.3f},{max(c[2] for c in coords):.3f}] nm", flush=True)
            ok = _overwrite_gro_coords(gro, coords)
            if not ok:
                print("[acpype] WARNING: atom count mismatch; GRO coords unchanged", flush=True)
            else:
                bad_after = _gro_has_bad_coords(gro, max_nm=500.0)
                if bad_after:
                    print("[acpype] ERROR: GRO still has bad coords after overwrite! Dumping:", flush=True)
                    for i, ln in enumerate(Path(gro).read_text().splitlines()[2:-1]):
                        if not ln.strip(): continue
                        try:
                            x, y, z = _parse_gro_coords(ln)
                            if abs(x) > 500 or abs(y) > 500 or abs(z) > 500:
                                print(f"[acpype]   atom {i}: ({x:.3f}, {y:.3f}, {z:.3f}) nm", flush=True)
                        except Exception:
                            pass
                else:
                    print("[acpype] GRO coordinates overwritten from docked pose", flush=True)

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
            "-ff", "amber99sb-ildn",
            "-water", "tip3p",
            "-ignh",
            "-missing",
        ],
        cwd=str(workdir),
        description="pdb2gmx — CHARMM36m protein topology",
    )
    return workdir / "protein.gro", workdir / "topol.top"


def _split_itp_atomtypes(itp_path):
    """Return (atomtypes_text, rest_text) from an ACPYPE-generated .itp file.

    GROMACS requires [ atomtypes ] to appear before any [ moleculetype ].
    ACPYPE puts both in one file, so we must split and include them separately.
    """
    lines = Path(itp_path).read_text().splitlines(keepends=True)
    atomtype_lines, other_lines = [], []
    in_atomtypes = False
    for line in lines:
        stripped = line.strip()
        if stripped == "[ atomtypes ]":
            in_atomtypes = True
            atomtype_lines.append(line)
        elif in_atomtypes and stripped.startswith("["):
            in_atomtypes = False
            other_lines.append(line)
        elif in_atomtypes:
            atomtype_lines.append(line)
        else:
            other_lines.append(line)
    return "".join(atomtype_lines), "".join(other_lines)


def assemble_complex(protein_gro, ligand_gro, protein_top, ligand_itp, workdir):
    """Combine protein.gro + ligand.gro → complex.gro and patch topology.

    GROMACS requires [ atomtypes ] before any [ moleculetype ], so the ligand
    atomtypes are split out and inserted before the protein moleculetype block.
    The ligand molecule topology is included before [ system ].
    Returns complex.gro path.
    """
    workdir = Path(workdir)
    complex_gro = workdir / "complex.gro"

    p_lines = Path(protein_gro).read_text().splitlines()
    l_lines = Path(ligand_gro).read_text().splitlines()

    # GRO format: line 0 = title, line 1 = atom count, lines 2..N-1 = atoms, last = box
    p_atoms = [ln for ln in p_lines[2:-1] if ln.strip()]
    l_atoms = [ln for ln in l_lines[2:-1] if ln.strip()]
    box = p_lines[-1]
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
    if _gro_has_bad_coords(complex_gro, max_nm=500.0):
        print("[assemble] WARNING: complex.gro has atoms with |coord| > 500 nm — dumping:", flush=True)
        for i, ln in enumerate(Path(complex_gro).read_text().splitlines()[2:-1]):
            if not ln.strip(): continue
            try:
                x, y, z = _parse_gro_coords(ln)
                if abs(x) > 500 or abs(y) > 500 or abs(z) > 500:
                    print(f"[assemble]   line {i+2}: atom={ln[10:15].strip()} ({x:.3f},{y:.3f},{z:.3f}) nm", flush=True)
            except Exception:
                pass

    # Split ligand ITP: [ atomtypes ] must go before any [ moleculetype ]
    atomtypes_text, mol_text = _split_itp_atomtypes(ligand_itp)
    lig_atomtypes_itp = workdir / "LIG_atomtypes.itp"
    lig_mol_itp = workdir / "LIG_mol.itp"
    lig_atomtypes_itp.write_text(atomtypes_text)
    lig_mol_itp.write_text(mol_text)

    top_text = Path(protein_top).read_text()

    # Insert ligand atomtypes after the force field #include (which defines [ defaults ]).
    # GROMACS requires [ atomtypes ] to follow [ defaults ], so we can't prepend before FF.
    # pdb2gmx may inline [ moleculetype ] or only #include chain .itp files — handle both.
    at_include = f'; Ligand atom types\n#include "{lig_atomtypes_itp}"\n'
    lines = top_text.splitlines(keepends=True)
    ff_idx = next(
        (i for i, ln in enumerate(lines) if ln.strip().startswith("#include") and ".ff/" in ln),
        None,
    )
    if ff_idx is not None:
        lines.insert(ff_idx + 1, "\n" + at_include)
        top_text = "".join(lines)
    elif "[ moleculetype ]" in top_text:
        top_text = top_text.replace("[ moleculetype ]", "\n" + at_include + "[ moleculetype ]", 1)
    else:
        top_text = "\n" + at_include + top_text

    # Insert ligand molecule include before [ system ]
    mol_include = f'\n; Ligand molecule topology\n#include "{lig_mol_itp}"\n'
    if "[ system ]" in top_text:
        top_text = top_text.replace("[ system ]", mol_include + "[ system ]", 1)
    else:
        top_text += mol_include

    # Append ligand to [ molecules ]
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


def _make_protein_lig_index(gro_path, ndx_out):
    """Create index.ndx with a Protein_LIG group = Protein | <ligand>.

    GROMACS MDP tc-grps requires all groups to exist in the index file.
    Protein_LIG is not auto-generated, so we build it here.
    """
    # First pass: dump default groups to discover numbers for Protein and ligand
    result = subprocess.run(
        ["gmx", "-quiet", "make_ndx", "-f", str(gro_path), "-o", str(ndx_out)],
        capture_output=True, text=True, input="q\n",
    )
    output = result.stdout + result.stderr
    protein_idx, lig_idx, max_idx = None, None, 0
    for line in output.splitlines():
        m = _re.match(r"\s*(\d+)\s+(\S+)", line)
        if m:
            idx = int(m.group(1))
            name = m.group(2)
            max_idx = max(max_idx, idx)
            if name == "Protein":
                protein_idx = idx
            elif name in ("MOL", "Other") and lig_idx is None:
                lig_idx = idx
    if protein_idx is None or lig_idx is None:
        print(f"[make_ndx] WARNING: could not find Protein/MOL groups; index unchanged\n{output}", flush=True)
        return
    new_idx = max_idx + 1
    ndx_input = f"{protein_idx} | {lig_idx}\nname {new_idx} Protein_LIG\nq\n"
    result2 = subprocess.run(
        ["gmx", "-quiet", "make_ndx", "-f", str(gro_path), "-o", str(ndx_out)],
        capture_output=True, text=True, input=ndx_input,
    )
    if result2.returncode != 0:
        raise RuntimeError(f"make_ndx failed:\n{result2.stdout}{result2.stderr}")
    print(f"[make_ndx] Protein_LIG group created (groups {protein_idx}|{lig_idx} → {new_idx})", flush=True)


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
        stdin_input="SOL\n",
    )

    index_ndx = workdir / "index.ndx"
    _make_protein_lig_index(workdir / "ionised.gro", index_ndx)
    return workdir / "ionised.gro", index_ndx


def run_md_step(name, mdp, input_gro, topol_top, workdir, prev_cpt=None, gpu=True, index_ndx=None):
    """grompp + mdrun for one MD step. Returns output .gro path."""
    workdir = Path(workdir)
    tpr = workdir / f"{name}.tpr"
    grompp_args = [
        "grompp", "-f", str(mdp), "-c", str(input_gro),
        "-p", str(topol_top), "-o", str(tpr), "-maxwarn", "2",
    ]
    if prev_cpt:
        grompp_args += ["-t", str(prev_cpt)]
    if index_ndx:
        grompp_args += ["-n", str(index_ndx)]
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

        # Write receptor PDB — strip HETATM (ions/cofactors) before pdb2gmx
        receptor_path = wd / "receptor.pdb"
        receptor_path.write_bytes(strip_hetatm(receptor_pdb))

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
        ionised_gro, index_ndx = solvate_and_ions(complex_gro, topol_top, cfg["box_padding"], wd)

        t0 = _time.time()

        # --- MD pipeline ---
        print("Step 1/4: Energy minimisation...", flush=True)
        em_gro, _ = run_md_step("em", MDP_DIR / "em.mdp", ionised_gro, topol_top, wd, gpu=False, index_ndx=index_ndx)

        print("Step 2/4: NVT equilibration (100 ps)...", flush=True)
        nvt_gro, nvt_cpt = run_md_step("nvt", MDP_DIR / "nvt.mdp", em_gro, topol_top, wd, index_ndx=index_ndx)

        print("Step 3/4: NPT equilibration (100 ps)...", flush=True)
        npt_gro, npt_cpt = run_md_step(
            "npt", MDP_DIR / "npt.mdp", nvt_gro, topol_top, wd, prev_cpt=nvt_cpt, index_ndx=index_ndx,
        )

        print(f"Step 4/4: Production MD ({cfg['nsteps']} steps)...", flush=True)
        _, _ = run_md_step(
            "md", md_mdp, npt_gro, topol_top, wd, prev_cpt=npt_cpt, index_ndx=index_ndx,
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
