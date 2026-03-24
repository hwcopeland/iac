#!/usr/bin/env python3

import os
import sys
import subprocess
import argparse

def parse_args():
    """
    Parse command-line arguments.
    """
    parser = argparse.ArgumentParser(description="Perform docking using AutoDock Vina.")
    parser.add_argument("pdbid", type=str, help="PDB ID of the protein (e.g., 7jrn).")
    parser.add_argument("batch_label", type=str, help="Batch label (e.g., TTT_A).")
    return parser.parse_args()

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
        x, y, z = [float(coord) for coord in line.split()]
    return x, y, z

def docking(pdbid, batch_label):
    # Paths and filenames
    AUTODOCK = "/autodock/vina"  # Ensure this path is correct
    receptor_pdbqt = f"{pdbid}.pdbqt"
    grid_center_file = "grid_center.txt"
    size = [40, 40, 40]  # Adjust grid size as necessary

    # Check if receptor PDBQT exists
    if not os.path.isfile(receptor_pdbqt):
        print(f"Error: Receptor PDBQT file '{receptor_pdbqt}' not found.")
        sys.exit(1)

    # Check if grid center file exists
    if not os.path.isfile(grid_center_file):
        print(f"Error: Grid center file '{grid_center_file}' not found.")
        sys.exit(1)

    # Read grid center from file
    x, y, z = read_grid_center(grid_center_file)
    grid_center = [x, y, z]
    print(f"Grid Center: {grid_center}, Grid Size: {size}")

    # Define the directory containing the PDBQT ligand files and output directory
    pdbqt_dir = batch_label
    output_dir = os.path.join(pdbqt_dir, "docked")

    # Ensure the output directory exists
    os.makedirs(output_dir, exist_ok=True)

    # Check if the input directory exists
    if not os.path.isdir(pdbqt_dir):
        print(f"Error: Input directory '{pdbqt_dir}' not found.")
        sys.exit(1)

    # List all relevant PDBQT files in the batch-specific folder
    pdbqt_files = [f for f in os.listdir(pdbqt_dir) if f.endswith(".pdbqt")]

    if not pdbqt_files:
        print(f"Error: No PDBQT files found in folder '{pdbqt_dir}' for batch label '{batch_label}'.")
        sys.exit(1)

    # Iterate over each ligand and perform docking
    for pdbqt_file in pdbqt_files:
        ligand_path = os.path.join(pdbqt_dir, pdbqt_file)
        output_prefix = os.path.join(output_dir, os.path.splitext(pdbqt_file)[0])

        command = (
            f"{AUTODOCK} --receptor {receptor_pdbqt} --ligand {ligand_path} "
            f"--center_x {x} --center_y {y} --center_z {z} "
            f"--size_x {size[0]} --size_y {size[1]} --size_z {size[2]} "
            f"--out {output_prefix}.pdbqt --log {output_prefix}.log"
        )

        try:
            subprocess.run(command, shell=True, check=True)
            print(f"Docking completed for ligand '{ligand_path}'. Results saved to '{output_prefix}.pdbqt'.")
        except subprocess.CalledProcessError as e:
            print(f"Error: AutoDock Vina execution failed for ligand '{ligand_path}'.")
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
