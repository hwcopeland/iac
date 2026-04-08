declare const molstar: any;

let viewerInstance: any = null;
let plugin: any = null;

// Molstar API accessed via molstar.lib.*
function getLib() {
  return molstar.lib;
}

export type AtomInfo = {
  element: string;
  atomName: string;
  residueName: string;
  residueId: number;
  chainId: string;
  x: number;
  y: number;
  z: number;
};

type InteractionCallback = (info: AtomInfo | null) => void;

let hoverCallback: InteractionCallback | null = null;
let clickCallback: InteractionCallback | null = null;

export function isReady(): boolean {
  return viewerInstance !== null;
}

export function getPlugin(): any {
  return plugin;
}

// Reapply our canvas props after any clear/reload that resets them
function applyCanvasProps(): void {
  if (!plugin?.canvas3d) return;
  plugin.canvas3d.setProps({
    renderer: { ...plugin.canvas3d.props.renderer, backgroundColor: 0x0d1117 },
    trackball: { ...plugin.canvas3d.props.trackball, maxWheelDelta: 0.005 },
  });
}

export async function init(container: HTMLDivElement): Promise<void> {
  const instance = await molstar.Viewer.create(container, {
    layoutIsExpanded: false,
    layoutShowControls: false,
    layoutShowRemoteState: false,
    layoutShowSequence: false,
    layoutShowLog: false,
    layoutShowLeftPanel: false,
    collapseLeftPanel: true,
    collapseRightPanel: true,
    viewportShowControls: false,
    viewportShowExpand: false,
    viewportShowSettings: false,
    viewportShowSelectionMode: false,
    viewportShowAnimation: false,
    viewportShowTrajectoryControls: false,
    viewportShowReset: false,
    viewportShowScreenshotControls: false,
    viewportShowToggleFullscreen: false,
  });

  viewerInstance = instance;
  plugin = instance.plugin;

  if (plugin.canvas3d) {
    applyCanvasProps();
  }

  new ResizeObserver(() => plugin.canvas3d?.handleResize()).observe(container);
  container.addEventListener('wheel', (e: WheelEvent) => e.preventDefault(), { passive: false });

  setupInteractions();
  // Viewer ready
}

function extractAtomInfo(reprLoci: any): AtomInfo | null {
  try {
    const loci = reprLoci.loci ?? reprLoci;
    if (!loci || loci.kind !== 'element-loci' || !loci.elements?.length) return null;

    const { StructureElement, StructureProperties } = getLib().structure;

    const e = loci.elements[0];
    const unit = e.unit;
    // e.indices could be an OrderedSet or plain array — get first element
    let idx = 0;
    if (e.indices) {
      if (typeof e.indices[0] === 'number') {
        idx = e.indices[0];
      } else if (e.indices.min !== undefined) {
        idx = e.indices.min;
      }
    }
    const elIdx = unit.elements[idx];
    if (elIdx === undefined) return null;

    const loc = StructureElement.Location.create(loci.structure, unit, elIdx);

    return {
      element: String(StructureProperties.atom.type_symbol(loc)),
      atomName: StructureProperties.atom.label_atom_id(loc),
      residueName: StructureProperties.residue.label_comp_id(loc),
      residueId: StructureProperties.residue.auth_seq_id(loc),
      chainId: StructureProperties.chain.auth_asym_id(loc),
      x: StructureProperties.atom.x(loc),
      y: StructureProperties.atom.y(loc),
      z: StructureProperties.atom.z(loc),
    };
  } catch {
    return null;
  }
}

function setupInteractions(): void {
  if (!plugin?.canvas3d?.interaction) {
    // No canvas3d interaction available
    return;
  }

  // Use behaviors.interaction instead of canvas3d.interaction
  // canvas3d.interaction is the raw source; behaviors.interaction is the proxied version
  // that Molstar's own code uses
  const bi = plugin.behaviors?.interaction;
  if (bi) {
    bi.hover.subscribe((event: any) => {
      if (hoverCallback) {
        hoverCallback(extractAtomInfo(event.current));
      }
    });
    bi.click.subscribe((event: any) => {
      if (clickCallback) clickCallback(extractAtomInfo(event.current));
    });
  } else {
    // Fallback to canvas3d.interaction (raw source)
    plugin.canvas3d.interaction.hover.subscribe((event: any) => {
      if (hoverCallback) hoverCallback(extractAtomInfo(event.current));
    });
    plugin.canvas3d.interaction.click.subscribe((event: any) => {
      if (clickCallback) clickCallback(extractAtomInfo(event.current));
    });
  }

}

export function onHover(cb: InteractionCallback): void {
  hoverCallback = cb;
}

export function onClick(cb: InteractionCallback): void {
  clickCallback = cb;
}

// Store raw structure text for editing
let currentStructureText: string | null = null;
let currentStructureFormat: string = 'pdb';

export function getCurrentStructureText(): string | null {
  return currentStructureText;
}

export async function loadPdb(id: string): Promise<void> {
  if (!viewerInstance) throw new Error('Viewer not initialized');
  // Load CIF into viewer (fast, reliable)
  await viewerInstance.loadStructureFromUrl(
    `https://files.rcsb.org/download/${id.toUpperCase()}.cif`,
    'mmcif',
    false
  );
  applyCanvasProps();
  // Also fetch PDB in background for editing support
  try {
    const resp = await fetch(`https://files.rcsb.org/download/${id.toUpperCase()}.pdb`);
    if (resp.ok) {
      currentStructureText = await resp.text();
      currentStructureFormat = 'pdb';
      // PDB text cached for editing
    }
  } catch {
    // PDB fetch failed — editing unavailable for this structure
  }
}

export async function clearStructures(): Promise<void> {
  if (!plugin) return;
  await plugin.clear();
}

// Save/restore camera across edits so the view doesn't jump
function saveCameraSnapshot(): any {
  try {
    return plugin?.canvas3d?.camera?.getSnapshot?.()
      ?? plugin?.canvas3d?.camera?.snapshot;
  } catch { return null; }
}

function restoreCameraSnapshot(snapshot: any): void {
  if (!snapshot || !plugin?.canvas3d) return;
  try {
    plugin.canvas3d.requestCameraReset({ snapshot, durationMs: 0 });
  } catch {}
}

export async function loadFile(data: string, format: string, preserveCamera = false): Promise<void> {
  if (!viewerInstance) throw new Error('Viewer not initialized');
  currentStructureText = data;
  currentStructureFormat = format;
  const formatMap: Record<string, string> = {
    pdb: 'pdb', cif: 'mmcif', mmcif: 'mmcif',
    mol: 'mol', mol2: 'mol2', sdf: 'sdf', xyz: 'xyz',
  };
  const fmt = formatMap[format.toLowerCase()] || format;
  // Save camera before clearing
  const cam = preserveCamera ? saveCameraSnapshot() : null;
  // Clear existing structures before loading modified data
  await clearStructures();
  const blob = new Blob([data], { type: 'text/plain' });
  const url = URL.createObjectURL(blob);
  await viewerInstance.loadStructureFromUrl(url, fmt, false);
  setTimeout(() => URL.revokeObjectURL(url), 5000);
  // Reapply our canvas settings (clear resets them)
  applyCanvasProps();
  // Restore camera after edit reload
  if (cam) {
    requestAnimationFrame(() => restoreCameraSnapshot(cam));
  }
}

export function resetCamera(): void {
  plugin?.managers?.camera?.reset?.();
}


// ─── Structure Component Parsing ───

export type StructureComponent = {
  id: string;           // unique key e.g. "chain-A", "ligand-A-501-ATP"
  type: 'polymer' | 'ligand' | 'water' | 'ion';
  chainId: string;
  label: string;        // display name e.g. "Chain A", "ATP 501"
  residueCount: number;
  atomCount: number;
  residueName?: string; // for ligands/ions: 3-letter code
  residueId?: number;   // for ligands/ions: sequence number
};

// Parse PDB text into a list of structural components
export function parseStructureComponents(pdbText: string): StructureComponent[] {
  const lines = pdbText.split('\n');
  // Track chains: map chainId → { atoms, residues (set of resId) }
  const chains = new Map<string, { atoms: number; residues: Set<string> }>();
  // Track ligands: map "chain-resName-resId" → { atoms, chainId, resName, resId }
  const ligands = new Map<string, { atoms: number; chainId: string; resName: string; resId: number }>();
  let waterAtoms = 0;

  for (const line of lines) {
    const recType = line.substring(0, 6).trim();
    if (recType !== 'ATOM' && recType !== 'HETATM') continue;

    const chainId = line.substring(21, 22).trim() || '_';
    const resName = line.substring(17, 20).trim();
    const resId = parseInt(line.substring(22, 26).trim()) || 0;

    if (recType === 'ATOM') {
      // Polymer atom
      let chain = chains.get(chainId);
      if (!chain) { chain = { atoms: 0, residues: new Set() }; chains.set(chainId, chain); }
      chain.atoms++;
      chain.residues.add(`${resName}-${resId}`);
    } else {
      // HETATM — water, ion, or ligand
      if (resName === 'HOH' || resName === 'WAT' || resName === 'DOD') {
        waterAtoms++;
      } else {
        const key = `${chainId}-${resName}-${resId}`;
        let lig = ligands.get(key);
        if (!lig) { lig = { atoms: 0, chainId, resName, resId }; ligands.set(key, lig); }
        lig.atoms++;
      }
    }
  }

  const components: StructureComponent[] = [];

  // Add polymer chains (sorted)
  for (const [chainId, data] of [...chains.entries()].sort((a, b) => a[0].localeCompare(b[0]))) {
    components.push({
      id: `chain-${chainId}`,
      type: 'polymer',
      chainId,
      label: `Chain ${chainId}`,
      residueCount: data.residues.size,
      atomCount: data.atoms,
    });
  }

  // Add ligands (sorted by chain, then resId)
  const sortedLigands = [...ligands.values()].sort((a, b) =>
    a.chainId.localeCompare(b.chainId) || a.resId - b.resId
  );
  for (const lig of sortedLigands) {
    // Classify small ions vs ligands by atom count
    const isIon = lig.atoms <= 2;
    components.push({
      id: `ligand-${lig.chainId}-${lig.resId}-${lig.resName}`,
      type: isIon ? 'ion' : 'ligand',
      chainId: lig.chainId,
      label: isIon ? `${lig.resName}` : `${lig.resName} ${lig.resId}`,
      residueCount: 1,
      atomCount: lig.atoms,
      residueName: lig.resName,
      residueId: lig.resId,
    });
  }

  // Add water if present
  if (waterAtoms > 0) {
    components.push({
      id: 'water',
      type: 'water',
      chainId: '',
      label: 'Water',
      residueCount: Math.floor(waterAtoms / 3) || waterAtoms, // ~3 atoms per HOH
      atomCount: waterAtoms,
    });
  }

  return components;
}

// ─── Molstar Visibility Control ───

// Track isolation state: null = showing everything, string = isolated component id
let isolatedComponentId: string | null = null;
// Store the full PDB text before isolation so we can restore
let fullStructureText: string | null = null;

export function getIsolatedComponent(): string | null {
  return isolatedComponentId;
}

// Filter PDB text to only include atoms matching a component
function filterPdbForComponent(pdbText: string, comp: StructureComponent): string {
  const lines = pdbText.split('\n');
  const filtered = lines.filter(line => {
    const recType = line.substring(0, 6).trim();
    if (recType !== 'ATOM' && recType !== 'HETATM') return true; // keep headers, END, etc.

    const chainId = line.substring(21, 22).trim() || '_';
    const resName = line.substring(17, 20).trim();
    const resId = parseInt(line.substring(22, 26).trim()) || 0;

    switch (comp.type) {
      case 'polymer':
        return recType === 'ATOM' && chainId === comp.chainId;
      case 'ligand':
      case 'ion':
        return recType === 'HETATM' && chainId === comp.chainId
          && resName === comp.residueName && resId === comp.residueId;
      case 'water':
        return recType === 'HETATM' && (resName === 'HOH' || resName === 'WAT' || resName === 'DOD');
      default:
        return true;
    }
  });
  return filtered.join('\n');
}

// Toggle isolation: click once = show only that component, click again = show all
export async function toggleIsolateComponent(comp: StructureComponent): Promise<void> {
  if (!currentStructureText) return;

  if (isolatedComponentId === comp.id) {
    // Already isolated on this component — restore full structure
    if (fullStructureText) {
      currentStructureText = fullStructureText;
      await loadFile(fullStructureText, currentStructureFormat);
    }
    isolatedComponentId = null;
    fullStructureText = null;
  } else {
    // Isolate this component
    const source = fullStructureText ?? currentStructureText;
    if (!fullStructureText) fullStructureText = currentStructureText;
    const subset = filterPdbForComponent(source, comp);
    await loadFile(subset, currentStructureFormat);
    isolatedComponentId = comp.id;
  }
}

// ─── Trajectory Support ───

/**
 * Load a multi-frame trajectory into the viewer.
 * Concatenates individual frame strings into a single multi-model file.
 * For XYZ format: frames are joined directly (each has its own atom count header).
 * For PDB format: each frame is wrapped in MODEL/ENDMDL records.
 */
export async function loadTrajectory(frames: string[], format: string = 'xyz'): Promise<void> {
  if (!viewerInstance) throw new Error('Viewer not initialized');
  if (frames.length === 0) return;

  let combined: string;
  if (format === 'pdb') {
    combined = frames.map((f, i) => `MODEL     ${i + 1}\n${f}\nENDMDL`).join('\n');
  } else {
    // XYZ and other formats: frames concatenate directly
    combined = frames.join('\n');
  }
  await loadFile(combined, format);
}

/**
 * Get the number of models (frames) in the currently loaded structure.
 * Returns 0 if no structure is loaded or models cannot be determined.
 */
export function getTrajectoryLength(): number {
  try {
    const structures = plugin?.managers?.structure?.hierarchy?.current?.structures;
    if (!structures?.length) return 0;
    return structures[0]?.cell?.obj?.data?.models?.length ?? 0;
  } catch { return 0; }
}

// ─── Volume Data (Cube Files) ───

export async function loadCubeFile(cubeData: string): Promise<void> {
  if (!viewerInstance) throw new Error('Viewer not initialized');

  // Use Molstar's built-in cube format provider which handles both
  // the molecular structure and volumetric data (orbital isosurfaces).
  const p = plugin;
  const cubeFormat = p.dataFormats.get('cube');
  if (!cubeFormat) throw new Error('Cube format not registered');

  await p.dataTransaction(async () => {
    // Load string data into the state tree
    const data = await p.builders.data.rawData({ data: cubeData, label: 'Cube File' });
    // Parse cube → volume + structure, then add default visuals
    const parsed = await cubeFormat.parse(p, data);
    if (cubeFormat.visuals) {
      await cubeFormat.visuals(p, parsed);
    }
  });

  applyCanvasProps();
}

// ─── Structure Overlay (Docking Poses) ───

// Convert PDBQT to PDB by stripping the charge/type columns (71-79)
function pdbqtToPdb(pdbqt: string): string {
  return pdbqt
    .split('\n')
    .map((line) => {
      if (line.startsWith('ATOM') || line.startsWith('HETATM')) {
        return line.substring(0, 66).padEnd(80);
      }
      return line;
    })
    .join('\n');
}

export async function overlayStructure(data: string, format: string): Promise<void> {
  if (!viewerInstance) throw new Error('Viewer not initialized');
  const formatMap: Record<string, string> = {
    pdb: 'pdb',
    pdbqt: 'pdb',
    mol: 'mol',
    mol2: 'mol2',
    sdf: 'sdf',
    xyz: 'xyz',
    cif: 'mmcif',
    mmcif: 'mmcif',
  };
  const lowerFmt = format.toLowerCase();
  const content = lowerFmt === 'pdbqt' ? pdbqtToPdb(data) : data;
  const fmt = formatMap[lowerFmt] || format;
  const blob = new Blob([content], { type: 'text/plain' });
  const url = URL.createObjectURL(blob);
  try {
    // Don't clear existing structures -- overlay on top
    await viewerInstance.loadStructureFromUrl(url, fmt, false);
    applyCanvasProps();
  } finally {
    setTimeout(() => URL.revokeObjectURL(url), 5000);
  }
}
