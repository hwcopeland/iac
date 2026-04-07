// RDKit WASM wrapper — lazy-loaded on first use

let RDKit: any = null;

async function ensureRDKit() {
  if (RDKit) return RDKit;
  const mod = await import('@rdkit/rdkit');
  const initRDKit = mod.default || mod;
  RDKit = await initRDKit({
    locateFile: (file: string) => `/${file}`,
  });
  console.log('RDKit WASM loaded');
  return RDKit;
}

export async function smilesToMolBlock(smiles: string): Promise<string | null> {
  const rdkit = await ensureRDKit();
  const mol = rdkit.get_mol(smiles);
  if (!mol || !mol.is_valid()) { mol?.delete(); return null; }
  try {
    mol.set_new_coords(true); // true = 3D
    const block = mol.get_molblock();
    mol.delete();
    return block;
  } catch {
    const block = mol.get_molblock();
    mol.delete();
    return block;
  }
}

export async function validateSmiles(smiles: string): Promise<boolean> {
  const rdkit = await ensureRDKit();
  const mol = rdkit.get_mol(smiles);
  const valid = mol && mol.is_valid();
  mol?.delete();
  return !!valid;
}

export async function canonicalizeSmiles(smiles: string): Promise<string | null> {
  const rdkit = await ensureRDKit();
  const mol = rdkit.get_mol(smiles);
  if (!mol || !mol.is_valid()) { mol?.delete(); return null; }
  const s = mol.get_smiles();
  mol.delete();
  return s;
}

export async function getMolProperties(smiles: string): Promise<{
  formula: string; mw: number; logp: number;
  hba: number; hbd: number; tpsa: number; rotatable: number;
} | null> {
  const rdkit = await ensureRDKit();
  const mol = rdkit.get_mol(smiles);
  if (!mol || !mol.is_valid()) { mol?.delete(); return null; }
  try {
    const desc = JSON.parse(mol.get_descriptors());
    mol.delete();
    return {
      formula: desc.MolecularFormula ?? '',
      mw: desc.exactmw ?? desc.amw ?? 0,
      logp: desc.CrippenClogP ?? 0,
      hba: desc.NumHBA ?? 0,
      hbd: desc.NumHBD ?? 0,
      tpsa: desc.tpsa ?? 0,
      rotatable: desc.NumRotatableBonds ?? 0,
    };
  } catch { mol.delete(); return null; }
}

export async function getSVG(smiles: string, width = 200, height = 150): Promise<string | null> {
  const rdkit = await ensureRDKit();
  const mol = rdkit.get_mol(smiles);
  if (!mol || !mol.is_valid()) { mol?.delete(); return null; }
  const svg = mol.get_svg_with_highlights(JSON.stringify({ width, height }));
  mol.delete();
  return svg;
}

export async function molBlockToSmiles(molBlock: string): Promise<string | null> {
  const rdkit = await ensureRDKit();
  const mol = rdkit.get_mol(molBlock);
  if (!mol || !mol.is_valid()) { mol?.delete(); return null; }
  const s = mol.get_smiles();
  mol.delete();
  return s;
}

export async function addHydrogens(smiles: string): Promise<string | null> {
  const rdkit = await ensureRDKit();
  const mol = rdkit.get_mol(smiles);
  if (!mol || !mol.is_valid()) { mol?.delete(); return null; }
  const withH = mol.add_hs();
  mol.delete();
  const mol2 = rdkit.get_mol(withH);
  if (!mol2 || !mol2.is_valid()) { mol2?.delete(); return null; }
  mol2.set_new_coords(true);
  const block = mol2.get_molblock();
  mol2.delete();
  return block;
}

export async function getAtomCount(smiles: string): Promise<number> {
  const rdkit = await ensureRDKit();
  const mol = rdkit.get_mol(smiles);
  if (!mol || !mol.is_valid()) { mol?.delete(); return 0; }
  const n = mol.get_num_atoms();
  mol.delete();
  return n;
}

export async function getBondCount(smiles: string): Promise<number> {
  const rdkit = await ensureRDKit();
  const mol = rdkit.get_mol(smiles);
  if (!mol || !mol.is_valid()) { mol?.delete(); return 0; }
  const n = mol.get_num_bonds();
  mol.delete();
  return n;
}
