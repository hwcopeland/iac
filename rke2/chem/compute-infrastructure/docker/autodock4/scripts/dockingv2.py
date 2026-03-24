#!/usr/bin/env python3

import os
import sys
import subprocess
import argparse

def parse_args():
    """
    Parse command-line arguments.
    """
    parser = argparse.ArgumentParser(description="Perform docking using AutoDock4.")
    parser.add_argument("pdbid", type=str, help="PDB ID of the protein (e.g., 7jrn).")
    parser.add_argument("batch_label", type=str, help="Batch label (e.g., TTT_A).")
    return parser.parse_args()

def extract_atom_types(pdbqt_file):
    """
    Extract the atom types from a PDBQT file.

    Parameters:
    - pdbqt_file: The path to the PDBQT file (either receptor or ligand).

    Returns:
    - A set of atom types found in the PDBQT file.
    """
    atom_types = set()
    with open(pdbqt_file, 'r') as f:
        for line in f:
            if line.startswith("ATOM") or line.startswith("HETATM"):
                atom_type = line.strip().split()[-1]  # The last item is the atom type
                atom_types.add(atom_type)
    return atom_types

def read_grid_center(filename):
    """
    Read the grid center coordinates from the grid_center.txt file.

    Parameters:
    - filename: The path to the grid_center.txt file.

    Returns:
    - A list containing the x, y, z coordinates of the grid center.
    """
    with open(filename, 'r') as f:
        line = f.readline().strip()
        return [float(coord) for coord in line.split()]
        
def prepare_gpf(pdb_id, receptor_types, ligand_types, grid_center):
    """Generate the GPF (Grid Parameter File) for AutoGrid."""
    gpf_file = f"{pdb_id}.gpf"
    receptor_pdbqt = f"{pdb_id}.pdbqt"

    with open(gpf_file, 'w') as f:
        f.write(f"npts 60 60 60\n")
        f.write("gridfld receptor.maps.fld\n")
        f.write("spacing 0.375\n")
        f.write(f"receptor_types {receptor_types}\n")
        f.write(f"ligand_types {ligand_types}\n")
        f.write(f"receptor {receptor_pdbqt}\n")
        f.write(f"gridcenter {grid_center[0]:.4f} {grid_center[1]:.4f} {grid_center[2]:.4f}\n")
        f.write("smooth 0.5\n")
        # Corrected: Define maps for each ligand atom type
        for atom_type in ligand_types.split():
            f.write(f"map receptor.{atom_type}.map\n")
        f.write("elecmap receptor.e.map\n")
        f.write("dsolvmap receptor.d.map\n")
        f.write("dielectric -0.1465\n")
    print(f"Generated GPF file: {gpf_file}")

def prepare_dpf(receptor_pdbqt, ligand_pdbqt, dpf_file, grid_center, size, pdbid, ligand_atom_types):
    """
    Generate a docking parameter file (DPF) for AutoDock4 using the LGA method.
    """
    with open(dpf_file, 'w') as f:
        # AutoDock DPF settings
        f.write("autodock_parameter_version 4.2 # used by autodock to validate parameter set\n")
        f.write("outlev 1 # diagnostic output level\n")
        f.write("intelec # calculate internal electrostatics\n")
        f.write("seed pid time # seeds for random generator\n")
        f.write("ligand_types " + " ".join(ligand_atom_types) + " # atoms types in ligand\n")
        f.write(f"fld receptor.maps.fld # grid_data_file\n")
        for atom_type in ligand_atom_types:
            f.write(f"map receptor.{atom_type}.map # atom-specific affinity map\n")
        f.write(f"elecmap receptor.e.map # electrostatics map\n")
        f.write(f"desolvmap receptor.d.map # desolvation map\n")
        f.write(f"move {ligand_pdbqt} # small molecule\n")
        f.write(f"about {grid_center[0]} {grid_center[1]} {grid_center[2]} # small molecule center\n")
        f.write("tran0 random # initial coordinates/A or random\n")
        f.write("quat0 random # initial orientation\n")
        f.write("ndihe 0 # number of active torsions\n")
        f.write("tstep 2.0 # translation step/A\n")
        f.write("qstep 50.0 # quaternion step/deg\n")
        f.write("dstep 50.0 # torsion step/deg\n")
        f.write("torsdof 0.274 # torsional degrees of freedom\n")
        f.write("rmstol 2.0 # cluster_tolerance/A\n")
        f.write("extnrg 1000.0 # external grid energy\n")
        f.write("e0max 0.0 10000 # max initial energy; max number of retries\n")
        f.write("ga_pop_size 150 # number of individuals in population\n")
        f.write("ga_num_evals 2500000 # maximum number of energy evaluations\n")
        f.write("ga_num_generations 27000 # maximum number of generations\n")
        f.write("ga_elitism 1 # top individuals to survive to next generation\n")
        f.write("ga_mutation_rate 0.02 # rate of gene mutation\n")
        f.write("ga_crossover_rate 0.8 # rate of crossover\n")
        f.write("ga_window_size 10 # number of preceding generations to consider\n")
        f.write("ga_cauchy_alpha 0.0 # Alpha parameter of Cauchy distribution\n")
        f.write("ga_cauchy_beta 1.0 # Beta parameter Cauchy distribution\n")
        f.write("set_ga # set the above parameters for GA or LGA\n")
        f.write("sw_max_its 300 # iterations of Solis & Wets local search\n")
        f.write("sw_max_succ 4 # consecutive successes before changing rho\n")
        f.write("sw_max_fail 4 # consecutive failures before changing rho\n")
        f.write("sw_rho 1.0 # size of local search space to sample\n")
        f.write("sw_lb_rho 0.01 # lower bound on rho\n")
        f.write("ls_search_freq 0.06 # probability of performing local search\n")
        f.write("set_psw1 # set the above pseudo-Solis & Wets parameters\n")
        f.write("unbound_model extended # state of unbound ligand\n")
        f.write("ga_run 10 # do this many hybrid GA-LS runs\n")
        f.write("analysis # perform a ranked cluster analysis\n")
    print(f"Generated DPF file: {dpf_file}")

def run_autogrid(pdbid):
    """
    Run AutoGrid to generate the grid maps for docking.
    """
    gpf_file = f"{pdbid}.gpf"
    log_file = f"{pdbid}.glg"
    
    # Run AutoGrid
    command = f"/autodock/autogrid4 -p {gpf_file} -l {log_file}"
    try:
        subprocess.run(command, shell=True, check=True)
        print(f"AutoGrid completed successfully. Log saved to {log_file}.")
    except subprocess.CalledProcessError as e:
        print(f"Error: AutoGrid execution failed.")
        print(e)
        sys.exit(1)

def docking(pdbid, batch_label):
    # Paths and filenames
    AUTODOCK = "/autodock/autodock4"  # Ensure this path is correct
    receptor_pdbqt = f"{pdbid}.pdbqt"
    grid_center_file = "grid_center.txt"
    size = [60, 60, 60]  # Adjust grid size as necessary

    # Check if receptor PDBQT exists
    if not os.path.isfile(receptor_pdbqt):
        print(f"Error: Receptor PDBQT file '{receptor_pdbqt}' not found.")
        sys.exit(1)

    # Check if grid center file exists
    if not os.path.isfile(grid_center_file):
        print(f"Error: Grid center file '{grid_center_file}' not found.")
        sys.exit(1)

    # Read grid center from file
    grid_center = read_grid_center(grid_center_file)
    print(f"Grid Center: {grid_center}, Grid Size: {size}")

    # Extract atom types from the receptor PDBQT file
    receptor_atom_types = extract_atom_types(receptor_pdbqt)
    supported_atom_types = {'A', 'C', 'HD', 'N', 'NA', 'OA', 'SA', 'S', 'H', 'F', 'Cl', 'Br', 'I', 'P', 'Mg', 'Mn', 'Zn', 'Ca', 'Fe'}
    receptor_atom_types = receptor_atom_types.intersection(supported_atom_types)
    receptor_types = " ".join(sorted(receptor_atom_types))
    print(f"Receptor Atom Types: {receptor_types}")

    # Define the directory containing the PDBQT ligand files and output directory
    pdbqt_dir = batch_label
    output_dir = os.path.join(pdbqt_dir, "docked")

    # Ensure the output directory exists
    os.makedirs(output_dir, exist_ok=True)

    # List all relevant PDBQT files in the batch-specific folder
    pdbqt_files = [f for f in os.listdir(pdbqt_dir) if f.endswith(".pdbqt")]

    if not pdbqt_files:
        print(f"Error: No PDBQT files found in folder '{pdbqt_dir}' for batch label '{batch_label}'.")
        sys.exit(1)

    # Collect all unique ligand atom types from all ligands
    all_ligand_atom_types = set()
    for pdbqt_file in pdbqt_files:
        ligand_path = os.path.join(pdbqt_dir, pdbqt_file)
        ligand_atom_types = extract_atom_types(ligand_path)
        ligand_atom_types = ligand_atom_types.intersection(supported_atom_types)
        all_ligand_atom_types.update(ligand_atom_types)

    ligand_types = " ".join(sorted(all_ligand_atom_types))
    print(f"All Ligand Atom Types: {ligand_types}")

    # Prepare and run AutoGrid
    prepare_gpf(pdbid, receptor_types, ligand_types=ligand_types, grid_center=grid_center)
    run_autogrid(pdbid)

    # Iterate over each ligand and perform docking
    for pdbqt_file in pdbqt_files:
        ligand_path = os.path.join(pdbqt_dir, pdbqt_file)
        output_prefix = os.path.join(output_dir, os.path.splitext(pdbqt_file)[0])
        dpf_file = f"{output_prefix}.dpf"

        # Extract atom types from ligand PDBQT file
        ligand_atom_types = extract_atom_types(ligand_path)
        ligand_atom_types = ligand_atom_types.intersection(supported_atom_types)
        print(f"Ligand: {pdbqt_file}, Atom Types: {ligand_atom_types}")

        # Generate DPF file
        prepare_dpf(receptor_pdbqt, ligand_path, dpf_file, grid_center, size, pdbid, ligand_atom_types)

        # Run AutoDock4
        command = f"{AUTODOCK} -p {dpf_file} -l {output_prefix}.dlg"
        try:
            subprocess.run(command, shell=True, check=True)
            print(f"Docking completed for ligand '{ligand_path}'. Results saved to '{output_prefix}.dlg'.")
        except subprocess.CalledProcessError as e:
            print(f"Error: AutoDock4 execution failed for ligand '{ligand_path}'.")
            print(e)
            continue

    print(f"All docking runs completed. Results are in '{output_dir}'.")

def main():
    # Parse command-line arguments
    args = parse_args()
    pdbid = args.pdbid.lower()
    batch_label = args.batch_label

    # Perform docking
    docking(pdbid, batch_label)

if __name__ == "__main__":
    main()