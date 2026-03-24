import os
import sys
import subprocess
import argparse
from rdkit import Chem
from rdkit.Chem import AllChem

def parse_args():
    """
    Parse command-line arguments.
    """
    parser = argparse.ArgumentParser(description="Prepare ligands from an SDF file for AutoDock4.")
    parser.add_argument("sdf_file", type=str, help="Path to the input SDF file containing multiple ligands.")
    parser.add_argument("output_dir", type=str, help="Directory to store the output PDBQT files.")
    parser.add_argument("--format", type=str, choices=["pdb", "mol2"], default="pdb", help="Intermediate format for ligands before running prepare_ligand4 (pdb or mol2).")
    return parser.parse_args()

def convert_ligand_to_format(mol, output_dir, index, output_format):
    """
    Convert a single ligand to the specified format (PDB or MOL2).
    
    Parameters:
    - mol: RDKit molecule object.
    - output_dir: Directory to save the converted ligand files.
    - index: Index of the ligand for naming.
    - output_format: 'pdb' or 'mol2'.
    """
    if not mol:
        print(f"Ligand {index} is invalid, skipping.")
        return None

    # Generate conformer if none exists
    if mol.GetNumConformers() == 0:
        AllChem.EmbedMolecule(mol)

    # Define file paths
    file_extension = output_format
    ligand_filename = os.path.join(output_dir, f"ligand_{index}.{file_extension}")

    # Write to the specified format
    if output_format == "pdb":
        Chem.MolToPDBFile(mol, ligand_filename)
    elif output_format == "mol2":
        with open(ligand_filename, 'w') as f:
            f.write(Chem.MolToMol2Block(mol))
    
    return ligand_filename

def run_prepare_ligand(ligand_file, output_dir, index):
    """
    Run the prepare_ligand4.py script on the given ligand file.
    
    Parameters:
    - ligand_file: Path to the input ligand file (PDB or MOL2).
    - output_dir: Directory to save the output PDBQT files.
    - index: Index of the ligand for naming the output file.
    """
    output_pdbqt = os.path.join(output_dir, f"ligand_{index}.pdbqt")
    command = f"prepare_ligand4 -l {ligand_file} -o {output_pdbqt}"

    try:
        subprocess.run(command, shell=True, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        print(f"Ligand {index} processed successfully.")
    except subprocess.CalledProcessError as e:
        print(f"Error processing ligand {index}: {e.stderr.decode()}")

def main():
    # Parse command-line arguments
    args = parse_args()
    sdf_file = args.sdf_file
    output_dir = args.output_dir
    output_format = args.format

    # Ensure output directory exists
    if not os.path.exists(output_dir):
        os.makedirs(output_dir)

    # Load ligands from the SDF file using RDKit
    suppl = Chem.SDMolSupplier(sdf_file, removeHs=False)
    
    # Process each ligand
    for index, mol in enumerate(suppl):
        if mol is None:
            print(f"Ligand {index + 1} is invalid or missing.")
            continue

        # Convert ligand to the desired format
        ligand_file = convert_ligand_to_format(mol, output_dir, index + 1, output_format)
        if ligand_file:
            # Run prepare_ligand4.py on the converted ligand file
            run_prepare_ligand(ligand_file, output_dir, index + 1)

if __name__ == "__main__":
    main()
