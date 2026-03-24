#!/usr/bin/env python3

import os
import subprocess
import argparse
from Bio.PDB import PDBParser, PDBIO, Select

# Define paths to MGLTools and AutoGrid executables
AUTOGRID = "/autodock/autogrid4"  # Adjust if your AutoGrid executable path is different

def run_command(command):
    """Execute a shell command."""
    try:
        subprocess.run(command, shell=True, check=True)
    except subprocess.CalledProcessError as e:
        print(f"Error executing command: {command}")
        print(e)
        exit(1)

def download_protein(protein_id):
    """Download the protein PDB file using wget."""
    filename = f'{protein_id}.pdb'
    url = f'https://files.rcsb.org/download/{filename}'  
    try:
        subprocess.run(['wget','-O', filename, url], check=True)
        if os.path.exists(filename):
            print(f"Downloaded and saved protein PDB to {filename}")
        else:
            print(f"Failed to download {filename}")
            exit(2)
    except subprocess.CalledProcessError as e:
        print(f"Error occurred while downloading {protein_id}: {e}")
        exit(3)

def remove_ligand_from_pdb(pdb_file, ligand_id, output_pdb_file):
    """Remove ligand specified by ligand_id from the PDB file and save to output_pdb_file."""
    parser = PDBParser(QUIET=True)
    structure = parser.get_structure('receptor', pdb_file)
    io = PDBIO()
    io.set_structure(structure)
    
    class NotLigandSelect(Select):
        def accept_residue(self, residue):
            if residue.get_resname() == ligand_id:
                return False  # Exclude this residue
            else:
                return True   # Include all other residues

    io.save(output_pdb_file, NotLigandSelect())
    print(f"Ligand '{ligand_id}' removed from PDB file. Saved as '{output_pdb_file}'")

def prepare_receptor(pdb_file):
    """Prepare the receptor PDBQT file using prepare_receptor4.py."""
    receptor_pdbqt = os.path.splitext(pdb_file)[0] + ".pdbqt"
    if os.path.exists(receptor_pdbqt):
        print(f"Receptor PDBQT file already exists: {receptor_pdbqt}")
        return  # Skip preparation if file exists

    command = (
        f"prepare_receptor4 "
        f"-r {pdb_file} -o {receptor_pdbqt} -U nphs_lps_waters_nonstdres"
    )
    run_command(command)
    print(f"Prepared receptor PDBQT: {receptor_pdbqt}")

def calculate_grid_center(pdb_file, ligand_id):
    """Calculate the center of mass of the native ligand specified by ligand_id."""
    parser = PDBParser(QUIET=True)
    structure = parser.get_structure('receptor', pdb_file)
    
    ligand_atoms = []
    
    for model in structure:
        for chain in model:
            for residue in chain:
                if residue.get_resname() == ligand_id:
                    ligand_atoms.extend(residue.get_atoms())
    
    if not ligand_atoms:
        print(f"No atoms found for ligand ID '{ligand_id}' in {pdb_file}. Cannot calculate grid center.")
        exit(1)
    
    # Calculate the center of the ligand
    center = [0.0, 0.0, 0.0]
    for atom in ligand_atoms:
        coord = atom.get_coord()
        center[0] += coord[0]
        center[1] += coord[1]
        center[2] += coord[2]
    
    center = [coord / len(ligand_atoms) for coord in center]
    print(f"Calculated grid center for ligand '{ligand_id}': {center}")
    
    # Save the center to a file
    with open('grid_center.txt', 'w') as f:
        f.write(f"{center[0]} {center[1]} {center[2]}\n")
    
    return center

def main():
    """Main function to parse arguments and execute steps."""
    parser = argparse.ArgumentParser(description="Prepare protein (receptor) for AutoDock4.")
    parser.add_argument("--protein_id", required=True, help="PDB ID of the protein (e.g., 7jrn).")
    parser.add_argument("--ligand_id", required=True, help="ID of the native ligand (e.g., LIG).")

    args = parser.parse_args()

    pdb_id = args.protein_id.lower()
    ligand_id = args.ligand_id
    receptor_pdb = f"{pdb_id}.pdb"
    receptor_pdb_clean = f"{pdb_id}.pdb"

    # Step 1: Download the protein PDB file
    download_protein(pdb_id)

    # Step 2: Calculate grid center from the original PDB file
    grid_center = calculate_grid_center(receptor_pdb, ligand_id)

    # Step 3: Remove the ligand from the PDB file
    remove_ligand_from_pdb(receptor_pdb, ligand_id, receptor_pdb_clean)

    # Step 4: Prepare the receptor using the cleaned PDB file
    prepare_receptor(receptor_pdb_clean)

if __name__ == "__main__":
    main()
