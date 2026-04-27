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

// ─── Canvas Props ───

function applyCanvasProps(): void {
  if (!plugin?.canvas3d) return;
  plugin.canvas3d.setProps({
    renderer: { ...plugin.canvas3d.props.renderer, backgroundColor: 0x0d1117 },
    trackball: { ...plugin.canvas3d.props.trackball, maxWheelDelta: 0.005 },
  });
}

// ─── Initialization ───

let _viewerContainer: HTMLDivElement | null = null;

export async function init(container: HTMLDivElement): Promise<void> {
  _viewerContainer = container;
  const instance = await molstar.Viewer.create(container, {
    layoutIsExpanded: false,
    layoutShowControls: true,
    layoutShowRemoteState: false,
    layoutShowSequence: true,
    layoutShowLog: false,
    layoutShowLeftPanel: false,
    collapseLeftPanel: true,
    collapseRightPanel: true,
    viewportShowControls: true,
    viewportShowExpand: false,
    viewportShowSettings: true,
    viewportShowSelectionMode: true,
    viewportShowAnimation: false,
    viewportShowTrajectoryControls: false,
    viewportShowReset: true,
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
}

// ─── Interaction Events ───

function setupInteractions(): void {
  if (!plugin?.canvas3d) return;

  const safeExtract = (reprLoci: any): AtomInfo | null => {
    try {
      const loci = reprLoci?.loci ?? reprLoci;
      if (!loci || loci.kind !== 'element-loci' || !loci.elements?.length) return null;

      const { StructureElement, StructureProperties } = getLib().structure;
      const stats = StructureElement.Stats.ofLoci(loci);
      const loc = stats.firstElementLoc;
      if (!loc?.unit) return null;

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
  };

  const bi = plugin.behaviors?.interaction;
  if (bi) {
    bi.hover.subscribe((event: any) => {
      if (hoverCallback) hoverCallback(safeExtract(event?.current));
    });
    bi.click.subscribe((event: any) => {
      if (clickCallback) clickCallback(safeExtract(event?.current));
    });
  }
}

export function onHover(cb: InteractionCallback): void {
  hoverCallback = cb;
}

export function onClick(cb: InteractionCallback): void {
  clickCallback = cb;
}

// ─── Structure State Tracking ───
// Track loaded structures by a logical label so we can replace/remove them
// without nuking the whole scene.

let currentStructureText: string | null = null;
let currentStructureFormat: string = 'pdb';

// Map of label → state tree ref for structures we've loaded.
// This lets us remove a specific structure without clearing the whole scene.
const _structureRefs = new Map<string, string>();

export function getCurrentStructureText(): string | null {
  return currentStructureText;
}

// ─── Helpers ───

// Convert PDBQT to PDB: filter to valid PDB records, strip Vina charge/type columns
function pdbqtToPdb(pdbqt: string): string {
  return pdbqt
    .split('\n')
    .filter(line =>
      line.startsWith('ATOM') || line.startsWith('HETATM') ||
      line.startsWith('TER') || line.startsWith('END') ||
      line.startsWith('REMARK') || line.startsWith('CONECT')
    )
    .map((line) => {
      if (line.startsWith('ATOM') || line.startsWith('HETATM')) {
        return line.substring(0, 66).padEnd(80);
      }
      return line;
    })
    .join('\n');
}

const FORMAT_MAP: Record<string, string> = {
  pdb: 'pdb', pdbqt: 'pdb', cif: 'mmcif', mmcif: 'mmcif',
  mol: 'mol', mol2: 'mol2', sdf: 'sdf', xyz: 'xyz',
};

/** Convert data to Molstar-compatible format */
function prepareData(data: string, format: string): { content: string; fmt: string } {
  const lowerFmt = format.toLowerCase();
  let content = data;
  if (lowerFmt === 'pdbqt') {
    content = pdbqtToPdb(data);
  }
  return { content, fmt: FORMAT_MAP[lowerFmt] || format };
}

/** Build a proper SortedArray from a plain number array for Molstar Loci. */
function toSortedArray(indices: number[]): any {
  try {
    const { SortedArray } = getLib().int;
    return SortedArray.ofSortedArray(new Int32Array(indices));
  } catch {
    // Fallback — return raw array (may work in some Molstar versions)
    return new Int32Array(indices);
  }
}

/** Get the state ref for a structure in the hierarchy */
function getStructureRef(index: number = 0): string | null {
  const structures = plugin?.managers?.structure?.hierarchy?.current?.structures ?? [];
  return structures[index]?.cell?.transform?.ref ?? null;
}

/** Get all structure refs */
function getAllStructureRefs(): { ref: string; structRef: any }[] {
  const structures = plugin?.managers?.structure?.hierarchy?.current?.structures ?? [];
  return structures.filter((s: any) => s.cell?.transform?.ref).map((s: any) => ({
    ref: s.cell.transform.ref,
    structRef: s,
  }));
}

// ─── Loading Structures (Additive — no clear) ───

/**
 * Load a structure into the viewer. Clears the scene first, then loads fresh.
 * The persistent-scene approach (removeStructureByLabel) was fragile — walking
 * the parent chain to find the root data node could delete wrong nodes or miss
 * entirely, causing the viewer to flash-load then go blank. Using plugin.clear()
 * is reliable; overlays are re-added after loadFile() by the caller anyway.
 */
export async function loadFile(data: string, format: string, preserveCamera = false): Promise<void> {
  if (!viewerInstance) throw new Error('Viewer not initialized');
  currentStructureText = data;
  currentStructureFormat = format;

  const { content, fmt } = prepareData(data, format);
  const cam = preserveCamera ? saveCameraSnapshot() : null;

  // Clear the whole scene — reliable replacement for the fragile
  // removeStructureByLabel('primary') parent-chain walk
  await plugin.clear();
  _structureRefs.clear();
  _pocketViewRefs = [];
  _surfaceRefs = [];

  // Load via Molstar Viewer API
  const blob = new Blob([content], { type: 'text/plain' });
  const url = URL.createObjectURL(blob);
  try {
    await viewerInstance.loadStructureFromUrl(url, fmt, false);
    applyCanvasProps();

    // Track the newly loaded structure
    const structures = plugin.managers?.structure?.hierarchy?.current?.structures ?? [];
    if (structures.length > 0) {
      const last = structures[structures.length - 1];
      if (last?.cell?.transform?.ref) {
        _structureRefs.set('primary', last.cell.transform.ref);
      }
    }
  } finally {
    setTimeout(() => URL.revokeObjectURL(url), 5000);
  }

  if (cam) {
    requestAnimationFrame(() => restoreCameraSnapshot(cam));
  }
}

export async function loadPdb(id: string): Promise<void> {
  if (!viewerInstance) throw new Error('Viewer not initialized');

  // Clear the whole scene before loading (same rationale as loadFile)
  await plugin.clear();
  _structureRefs.clear();
  _pocketViewRefs = [];
  _surfaceRefs = [];

  await viewerInstance.loadStructureFromUrl(
    `https://files.rcsb.org/download/${id.toUpperCase()}.cif`,
    'mmcif',
    false
  );
  applyCanvasProps();

  // Track it
  const structures = plugin.managers?.structure?.hierarchy?.current?.structures ?? [];
  if (structures.length > 0) {
    const last = structures[structures.length - 1];
    if (last?.cell?.transform?.ref) {
      _structureRefs.set('primary', last.cell.transform.ref);
    }
  }

  // Fetch PDB text for editing support
  try {
    const resp = await fetch(`https://files.rcsb.org/download/${id.toUpperCase()}.pdb`);
    if (resp.ok) {
      currentStructureText = await resp.text();
      currentStructureFormat = 'pdb';
    }
  } catch {}
}

/**
 * Overlay a structure on top of existing ones (e.g., docked ligand).
 * Tracked with label 'overlay-N' for cleanup.
 */
export async function overlayStructure(data: string, format: string): Promise<void> {
  if (!viewerInstance) throw new Error('Viewer not initialized');
  const { content, fmt } = prepareData(data, format);
  const blob = new Blob([content], { type: 'text/plain' });
  const url = URL.createObjectURL(blob);
  try {
    await viewerInstance.loadStructureFromUrl(url, fmt, false);
    applyCanvasProps();

    // Track with unique overlay label
    const structures = plugin.managers?.structure?.hierarchy?.current?.structures ?? [];
    if (structures.length > 0) {
      const last = structures[structures.length - 1];
      if (last?.cell?.transform?.ref) {
        const label = `overlay-${Date.now()}`;
        _structureRefs.set(label, last.cell.transform.ref);
      }
    }
  } finally {
    setTimeout(() => URL.revokeObjectURL(url), 5000);
  }
}

/** Remove a tracked structure by label without clearing the scene. */
async function removeStructureByLabel(label: string): Promise<void> {
  const ref = _structureRefs.get(label);
  if (!ref || !plugin) return;
  try {
    // Walk up to the root data node for this structure and remove the whole subtree
    const cell = plugin.state.data.cells.get(ref);
    if (cell) {
      // Find the root download/data node (parent chain)
      let rootRef = ref;
      let current = cell;
      while (current?.transform?.parent && current.transform.parent !== plugin.state.data.tree.root.ref) {
        rootRef = current.transform.parent;
        current = plugin.state.data.cells.get(rootRef);
        if (!current) break;
      }
      await plugin.build().delete(rootRef).commit();
    }
  } catch {}
  _structureRefs.delete(label);
}

/** Remove all overlay structures, keeping the primary. */
export async function clearOverlays(): Promise<void> {
  const overlayLabels = [..._structureRefs.keys()].filter(k => k.startsWith('overlay-'));
  for (const label of overlayLabels) {
    await removeStructureByLabel(label);
  }
}

/** Clear everything — full reset. */
export async function clearStructures(): Promise<void> {
  if (!plugin) return;
  await plugin.clear();
  _structureRefs.clear();
  _pocketViewRefs = [];
  _surfaceRefs = [];
  applyCanvasProps();
}

// ─── Camera ───

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

export function resetCamera(): void {
  plugin?.managers?.camera?.reset?.();
}

// ─── Structure Component Parsing ───

export type StructureComponent = {
  id: string;
  type: 'polymer' | 'ligand' | 'water' | 'ion';
  chainId: string;
  label: string;
  residueCount: number;
  atomCount: number;
  residueName?: string;
  residueId?: number;
};

export function parseStructureComponents(pdbText: string): StructureComponent[] {
  const lines = pdbText.split('\n');
  const chains = new Map<string, { atoms: number; residues: Set<string> }>();
  const ligands = new Map<string, { atoms: number; chainId: string; resName: string; resId: number }>();
  let waterAtoms = 0;

  for (const line of lines) {
    const recType = line.substring(0, 6).trim();
    if (recType !== 'ATOM' && recType !== 'HETATM') continue;

    const chainId = line.substring(21, 22).trim() || '_';
    const resName = line.substring(17, 20).trim();
    const resId = parseInt(line.substring(22, 26).trim()) || 0;

    if (recType === 'ATOM') {
      let chain = chains.get(chainId);
      if (!chain) { chain = { atoms: 0, residues: new Set() }; chains.set(chainId, chain); }
      chain.atoms++;
      chain.residues.add(`${resName}-${resId}`);
    } else {
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

  for (const [chainId, data] of [...chains.entries()].sort((a, b) => a[0].localeCompare(b[0]))) {
    components.push({
      id: `chain-${chainId}`, type: 'polymer', chainId,
      label: `Chain ${chainId}`, residueCount: data.residues.size, atomCount: data.atoms,
    });
  }

  for (const lig of [...ligands.values()].sort((a, b) => a.chainId.localeCompare(b.chainId) || a.resId - b.resId)) {
    const isIon = lig.atoms <= 2;
    components.push({
      id: `ligand-${lig.chainId}-${lig.resId}-${lig.resName}`,
      type: isIon ? 'ion' : 'ligand', chainId: lig.chainId,
      label: isIon ? lig.resName : `${lig.resName} ${lig.resId}`,
      residueCount: 1, atomCount: lig.atoms, residueName: lig.resName, residueId: lig.resId,
    });
  }

  if (waterAtoms > 0) {
    components.push({
      id: 'water', type: 'water', chainId: '', label: 'Water',
      residueCount: Math.floor(waterAtoms / 3) || waterAtoms, atomCount: waterAtoms,
    });
  }

  return components;
}

// ─── Component Isolation ───

let isolatedComponentId: string | null = null;
let fullStructureText: string | null = null;

export function getIsolatedComponent(): string | null {
  return isolatedComponentId;
}

function filterPdbForComponent(pdbText: string, comp: StructureComponent): string {
  const lines = pdbText.split('\n');
  const filtered = lines.filter(line => {
    const recType = line.substring(0, 6).trim();
    if (recType !== 'ATOM' && recType !== 'HETATM') return true;
    const chainId = line.substring(21, 22).trim() || '_';
    const resName = line.substring(17, 20).trim();
    const resId = parseInt(line.substring(22, 26).trim()) || 0;
    switch (comp.type) {
      case 'polymer': return recType === 'ATOM' && chainId === comp.chainId;
      case 'ligand':
      case 'ion': return recType === 'HETATM' && chainId === comp.chainId && resName === comp.residueName && resId === comp.residueId;
      case 'water': return recType === 'HETATM' && (resName === 'HOH' || resName === 'WAT' || resName === 'DOD');
      default: return true;
    }
  });
  return filtered.join('\n');
}

export async function toggleIsolateComponent(comp: StructureComponent): Promise<void> {
  if (!currentStructureText) return;
  if (isolatedComponentId === comp.id) {
    if (fullStructureText) {
      currentStructureText = fullStructureText;
      await loadFile(fullStructureText, currentStructureFormat);
    }
    isolatedComponentId = null;
    fullStructureText = null;
  } else {
    const source = fullStructureText ?? currentStructureText;
    if (!fullStructureText) fullStructureText = currentStructureText;
    const subset = filterPdbForComponent(source, comp);
    await loadFile(subset, currentStructureFormat);
    isolatedComponentId = comp.id;
  }
}

// ─── Trajectory Support ───

export async function loadTrajectory(frames: string[], format: string = 'xyz'): Promise<void> {
  if (!viewerInstance) throw new Error('Viewer not initialized');
  if (frames.length === 0) return;
  let combined: string;
  if (format === 'pdb') {
    combined = frames.map((f, i) => `MODEL     ${i + 1}\n${f}\nENDMDL`).join('\n');
  } else {
    combined = frames.join('\n');
  }
  await loadFile(combined, format);
}

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
  const p = plugin;
  const cubeFormat = p.dataFormats.get('cube');
  if (!cubeFormat) throw new Error('Cube format not registered');

  await p.dataTransaction(async () => {
    const data = await p.builders.data.rawData({ data: cubeData, label: 'Cube File' });
    const parsed = await cubeFormat.parse(p, data);
    if (cubeFormat.visuals) {
      await cubeFormat.visuals(p, parsed);
    }
  });
  applyCanvasProps();
}

// ─── ESP-Mapped Density Surface ───

const BOHR_TO_ANG = 0.529177210903;

function parseCubeGrid(cubeText: string) {
  const lines = cubeText.split('\n');
  let idx = 2;
  const headerLine = lines[idx++].trim().split(/\s+/);
  const natoms = Math.abs(parseInt(headerLine[0]));
  const origin = [
    parseFloat(headerLine[1]) * BOHR_TO_ANG,
    parseFloat(headerLine[2]) * BOHR_TO_ANG,
    parseFloat(headerLine[3]) * BOHR_TO_ANG,
  ];
  const axes: number[][] = [];
  const dims: number[] = [];
  for (let a = 0; a < 3; a++) {
    const parts = lines[idx++].trim().split(/\s+/);
    dims.push(parseInt(parts[0]));
    axes.push([
      parseFloat(parts[1]) * BOHR_TO_ANG,
      parseFloat(parts[2]) * BOHR_TO_ANG,
      parseFloat(parts[3]) * BOHR_TO_ANG,
    ]);
  }
  idx += natoms;
  const data = new Float64Array(dims[0] * dims[1] * dims[2]);
  let di = 0;
  for (; idx < lines.length && di < data.length; idx++) {
    const line = lines[idx].trim();
    if (!line) continue;
    const vals = line.split(/\s+/);
    for (const v of vals) {
      if (di < data.length) data[di++] = parseFloat(v);
    }
  }
  let vmin = Infinity, vmax = -Infinity;
  for (let i = 0; i < data.length; i++) {
    const v = data[i];
    if (isFinite(v)) { if (v < vmin) vmin = v; if (v > vmax) vmax = v; }
  }
  return { origin, axes, dims, data, vmin, vmax };
}

function makeCubeSampler(grid: ReturnType<typeof parseCubeGrid>) {
  const { origin, axes, dims, data } = grid;
  const dx = axes[0][0], dy = axes[1][1], dz = axes[2][2];
  const ni = dims[0], nj = dims[1], nk = dims[2];
  return (x: number, y: number, z: number): number => {
    const fi = (x - origin[0]) / dx;
    const fj = (y - origin[1]) / dy;
    const fk = (z - origin[2]) / dz;
    const i0 = Math.floor(fi), j0 = Math.floor(fj), k0 = Math.floor(fk);
    if (i0 < 0 || i0 >= ni - 1 || j0 < 0 || j0 >= nj - 1 || k0 < 0 || k0 >= nk - 1) return NaN;
    const di = fi - i0, dj = fj - j0, dk = fk - k0;
    const at = (i: number, j: number, k: number) => i * nj * nk + j * nk + k;
    return (
      data[at(i0, j0, k0)] * (1 - di) * (1 - dj) * (1 - dk) +
      data[at(i0, j0, k0 + 1)] * (1 - di) * (1 - dj) * dk +
      data[at(i0, j0 + 1, k0)] * (1 - di) * dj * (1 - dk) +
      data[at(i0, j0 + 1, k0 + 1)] * (1 - di) * dj * dk +
      data[at(i0 + 1, j0, k0)] * di * (1 - dj) * (1 - dk) +
      data[at(i0 + 1, j0, k0 + 1)] * di * (1 - dj) * dk +
      data[at(i0 + 1, j0 + 1, k0)] * di * dj * (1 - dk) +
      data[at(i0 + 1, j0 + 1, k0 + 1)] * di * dj * dk
    );
  };
}

let _espSampler: ((x: number, y: number, z: number) => number) | null = null;
let _espRange = 0.05;
let _espThemeRegistered = false;

function ensureESPTheme() {
  if (_espThemeRegistered || !plugin) return;
  const { Volume } = getLib().volume;
  const themeRegistry = plugin.representation.volume.themes.colorThemeRegistry;
  const provider = {
    name: 'esp-on-density',
    label: 'ESP on Density',
    category: 'Misc',
    factory: (_ctx: any, props: any) => {
      const sampler = _espSampler;
      const range = _espRange;
      if (!sampler) {
        return { factory: provider.factory, granularity: 'uniform' as const, color: () => 0xcccccc, props, description: 'ESP' };
      }
      const colorFn = (location: any) => {
        if (!location || !location.position) return 0xcccccc;
        const pos = location.position;
        const val = sampler(pos[0], pos[1], pos[2]);
        if (!isFinite(val)) return 0xcccccc;
        const t = Math.max(-1, Math.min(1, val / range));
        if (t < 0) {
          const f = 1 + t;
          return (255 << 16) | (Math.round(f * 255) << 8) | Math.round(f * 255);
        } else {
          const f = 1 - t;
          return (Math.round(f * 255) << 16) | (Math.round(f * 255) << 8) | 255;
        }
      };
      return {
        factory: provider.factory, granularity: 'vertex' as const,
        preferSmoothing: true, color: colorFn, props,
        description: 'Electrostatic potential mapped onto density surface',
      };
    },
    getParams: () => ({}),
    defaultValues: {},
    isApplicable: (ctx: any) => !!ctx.volume && !Volume.Segmentation?.get?.(ctx.volume),
  };
  themeRegistry.add(provider);
  _espThemeRegistered = true;
}

export async function loadDensityWithESP(densityCube: string, espCube: string): Promise<void> {
  if (!viewerInstance) throw new Error('Viewer not initialized');
  const p = plugin;
  const cubeFormat = p.dataFormats.get('cube');
  if (!cubeFormat) throw new Error('Cube format not registered');

  const espGrid = parseCubeGrid(espCube);
  _espSampler = makeCubeSampler(espGrid);
  const absMax = Math.max(Math.abs(espGrid.vmin), Math.abs(espGrid.vmax));
  _espRange = Math.max(0.01, Math.min(absMax * 0.8, 0.1));

  ensureESPTheme();
  await p.clear();
  _structureRefs.clear();

  await p.dataTransaction(async () => {
    const data = await p.builders.data.rawData({ data: densityCube, label: 'Electron Density' });
    const parsed = await cubeFormat.parse(p, data);
    const volumeData = parsed.volume?.cell?.obj?.data;
    const { StateTransforms } = getLib().plugin;
    const builder = p.build();
    if (volumeData) {
      const { Volume } = getLib().volume;
      const isoValue = Volume.IsoValue.absolute(0.02);
      builder.to(parsed.volume).apply(StateTransforms.Representation.VolumeRepresentation3D, {
        type: { name: 'isosurface', params: { isoValue, alpha: 0.9, visuals: ['solid'] } },
        colorTheme: { name: 'esp-on-density', params: {} },
        sizeTheme: { name: 'uniform', params: {} },
        quality: 'highest', doubleSided: true, flatShaded: false,
      });
    }
    if (parsed.structure) {
      builder.to(parsed.structure).apply(StateTransforms.Representation.StructureRepresentation3D, {
        type: { name: 'ball-and-stick', params: { sizeFactor: 0.2 } },
        colorTheme: { name: 'element-symbol', params: {} },
        sizeTheme: { name: 'physical', params: {} },
      });
    }
    await builder.commit();
  });
  applyCanvasProps();
}

// ─── Viewer API: Focus, Highlight, Sequence ───

/** Build a proper Loci for a residue. Uses SortedArray for correct OrderedSet. */
function buildResidueLoci(chainId: string, resId: number): any {
  const structures = plugin?.managers?.structure?.hierarchy?.current?.structures;
  if (!structures?.length) return null;

  const structData = structures[0]?.cell?.obj?.data;
  if (!structData) return null;

  const { StructureElement, StructureProperties } = getLib().structure;

  const lociElements: any[] = [];
  for (const unit of structData.units) {
    const indices: number[] = [];
    for (let i = 0, len = unit.elements.length; i < len; i++) {
      const elIdx = unit.elements[i];
      const loc = StructureElement.Location.create(structData, unit, elIdx);
      const c = StructureProperties.chain.auth_asym_id(loc);
      const r = StructureProperties.residue.auth_seq_id(loc);
      if (c === chainId && r === resId) {
        indices.push(i);
      }
    }
    if (indices.length > 0) {
      lociElements.push({ unit, indices: toSortedArray(indices) });
    }
  }

  if (lociElements.length === 0) return null;
  return StructureElement.Loci(structData, lociElements);
}

export function focusResidue(chainId: string, resId: number): void {
  if (!plugin) return;
  try {
    const loci = buildResidueLoci(chainId, resId);
    if (loci) plugin.managers.camera.focusLoci(loci, { durationMs: 250 });
  } catch {}
}

export function highlightResidue(chainId: string, resId: number): void {
  if (!plugin) return;
  try {
    const loci = buildResidueLoci(chainId, resId);
    if (!loci) return;
    plugin.managers.interactivity.lociSelects.deselectAll();
    plugin.managers.interactivity.lociSelects.select({ loci });
  } catch {}
}

export function getSequence(): { chainId: string; residues: { resId: number; resName: string }[] }[] {
  if (!plugin) return [];
  try {
    const structures = plugin.managers?.structure?.hierarchy?.current?.structures;
    if (!structures?.length) return [];
    const structData = structures[0]?.cell?.obj?.data;
    if (!structData) return [];
    const { StructureElement, StructureProperties } = getLib().structure;
    const chainMap = new Map<string, Map<number, string>>();
    for (const unit of structData.units) {
      for (let i = 0, len = unit.elements.length; i < len; i++) {
        const elIdx = unit.elements[i];
        const loc = StructureElement.Location.create(structData, unit, elIdx);
        const cId = StructureProperties.chain.auth_asym_id(loc);
        const rId = StructureProperties.residue.auth_seq_id(loc);
        const rName = StructureProperties.residue.label_comp_id(loc);
        if (!chainMap.has(cId)) chainMap.set(cId, new Map());
        chainMap.get(cId)!.set(rId, rName);
      }
    }
    const result: { chainId: string; residues: { resId: number; resName: string }[] }[] = [];
    for (const [chainId, residueMap] of [...chainMap.entries()].sort((a, b) => a[0].localeCompare(b[0]))) {
      const residues = [...residueMap.entries()].sort((a, b) => a[0] - b[0]).map(([rId, rName]) => ({ resId: rId, resName: rName }));
      result.push({ chainId, residues });
    }
    return result;
  } catch { return []; }
}

export function focusLastStructure(): void {
  if (!plugin) return;
  try {
    const structures = plugin.managers?.structure?.hierarchy?.current?.structures;
    if (!structures?.length) return;
    const last = structures[structures.length - 1];
    if (last?.cell?.obj?.data) {
      const loci = getLib().structure.Structure.Loci(last.cell.obj.data);
      plugin.managers.camera.focusLoci(loci, { durationMs: 250 });
    }
  } catch {
    plugin.managers?.camera?.reset?.();
  }
}

// ─── Representations (In-Place Updates) ───

let _currentRepresentation: string = 'cartoon';
let _reprChangeListeners: ((repr: string) => void)[] = [];

export function getCurrentRepresentation(): string { return _currentRepresentation; }
export function onRepresentationChange(cb: (repr: string) => void): void { _reprChangeListeners.push(cb); }

/**
 * Change representation type for all structures. Uses in-place update when
 * possible (same number of structures), falls back to delete+add if needed.
 */
export async function setRepresentation(type: string): Promise<void> {
  if (!plugin) return;
  try {
    const { StateTransforms } = getLib().plugin;

    // Clear pocket view and surface overlays
    await clearPocketView();
    await clearSurfaceRefs();

    await plugin.dataTransaction(async () => {
      const structures = plugin.managers?.structure?.hierarchy?.current?.structures ?? [];

      for (const structRef of structures) {
        // Collect existing representation refs
        const reprRefs: string[] = [];
        for (const comp of (structRef.components ?? [])) {
          for (const repr of (comp.representations ?? [])) {
            if (repr.cell?.transform?.ref) reprRefs.push(repr.cell.transform.ref);
          }
        }
        for (const repr of (structRef.representations ?? [])) {
          if (repr.cell?.transform?.ref) reprRefs.push(repr.cell.transform.ref);
        }

        const ref = structRef.cell?.transform?.ref;
        if (!ref) continue;

        if (reprRefs.length === 1) {
          // In-place update: change the existing representation's type
          const builder = plugin.build();
          builder.to(reprRefs[0]).update(StateTransforms.Representation.StructureRepresentation3D, (old: any) => ({
            ...old,
            type: { name: type, params: {} },
          }));
          await builder.commit();
        } else {
          // Multiple or zero representations — delete all, add one
          if (reprRefs.length > 0) {
            const delBuilder = plugin.build();
            for (const r of reprRefs) { try { delBuilder.delete(r); } catch {} }
            await delBuilder.commit();
          }
          await plugin.build()
            .to(ref)
            .apply(StateTransforms.Representation.StructureRepresentation3D, {
              type: { name: type, params: {} },
              colorTheme: { name: 'element-symbol', params: {} },
              sizeTheme: { name: 'physical', params: {} },
            })
            .commit();
        }
      }
    });

    _currentRepresentation = type;
    for (const cb of _reprChangeListeners) cb(type);
  } catch (e) {
    console.error('setRepresentation failed:', e);
  }
}

/**
 * Update color theme on all existing representations (in-place).
 */
export async function setColorTheme(theme: string): Promise<void> {
  if (!plugin) return;
  try {
    await plugin.dataTransaction(async () => {
      const structures = plugin.managers?.structure?.hierarchy?.current?.structures;
      if (!structures?.length) return;
      const { StateTransforms } = getLib().plugin;

      for (const structRef of structures) {
        const reprRefs: string[] = [];
        for (const comp of (structRef.components ?? [])) {
          for (const repr of (comp.representations ?? [])) {
            if (repr.cell?.transform?.ref) reprRefs.push(repr.cell.transform.ref);
          }
        }
        for (const repr of (structRef.representations ?? [])) {
          if (repr.cell?.transform?.ref) reprRefs.push(repr.cell.transform.ref);
        }

        for (const ref of reprRefs) {
          const cell = plugin.state.data.cells.get(ref);
          if (!cell?.params?.values) continue;
          plugin.build().to(ref).update(StateTransforms.Representation.StructureRepresentation3D, (old: any) => ({
            ...old,
            colorTheme: { name: theme, params: {} },
          })).commit();
        }
      }
    });
  } catch {}
}

/** Add a molecular surface on top of existing representations. */
export async function showSurface(colorTheme: string = 'partial-charge'): Promise<void> {
  if (!plugin) return;
  try {
    const ref = getStructureRef();
    if (!ref) return;
    const { StateTransforms } = getLib().plugin;
    const update = plugin.build();
    const node = update.to(ref).apply(StateTransforms.Representation.StructureRepresentation3D, {
      type: { name: 'molecular-surface', params: { alpha: 0.7 } },
      colorTheme: { name: colorTheme, params: {} },
      sizeTheme: { name: 'physical', params: {} },
    });
    await update.commit();
    try { if (node.ref) _surfaceRefs.push(node.ref); } catch {}
  } catch {}
}

/** Remove molecular surface representations. */
export async function hideSurface(): Promise<void> {
  await clearSurfaceRefs();
}

// ─── Interaction Lines ───
// 2D canvas overlay approach was attempted but causes severe lag (didDraw
// fires every frame) and incorrect positioning. Disabled until we can either
// build a custom Molstar ES module bundle exposing the Lines geometry API,
// or integrate with Molstar's native non-covalent interaction detection.
//
// Interaction data is still returned by the backend and displayed in the
// pocket residue table (type pills, distances). The 3D visualization of
// specific atom-atom interactions requires native Molstar Shape support
// which is not accessible through the pre-built UMD viewer bundle.

export type InteractionLine = {
  type: string;
  rec_x: number; rec_y: number; rec_z: number;
  lig_x: number; lig_y: number; lig_z: number;
};

export function drawInteractionLines(_lines: InteractionLine[], _activeTypes?: Set<string>): void {}
export function removeInteractionLines(): void {}

// ─── Custom Dark-Theme Color Themes ───

let _darkChargeThemeRegistered = false;
let _darkElementThemeRegistered = false;

const POSITIVE_RESIDUES = new Set(['LYS', 'ARG', 'HIS']);
const NEGATIVE_RESIDUES = new Set(['ASP', 'GLU']);

function ensureDarkChargeTheme() {
  if (_darkChargeThemeRegistered || !plugin) return;
  const themeRegistry = plugin.representation.structure.themes.colorThemeRegistry;
  const { StructureProperties } = getLib().structure;
  const provider = {
    name: 'dark-residue-charge', label: 'Charge (Dark)', category: 'Misc',
    factory: (_ctx: any, props: any) => {
      const colorFn = (location: any) => {
        try {
          const resName = StructureProperties.residue.label_comp_id(location);
          if (POSITIVE_RESIDUES.has(resName)) return 0x58a6ff;
          if (NEGATIVE_RESIDUES.has(resName)) return 0xf85149;
          return 0x3b434d;
        } catch { return 0x3b434d; }
      };
      return { factory: provider.factory, granularity: 'group' as const, color: colorFn, props, description: 'Residue charge (dark theme)' };
    },
    getParams: () => ({}), defaultValues: {}, isApplicable: () => true,
  };
  themeRegistry.add(provider);
  _darkChargeThemeRegistered = true;
}

const ELEMENT_COLORS: Record<string, number> = {
  C: 0x606870, N: 0x3050F8, O: 0xFF0D0D, S: 0xFFFF30, P: 0xFF8000,
  H: 0x8b949e, FE: 0xE06633, ZN: 0x7D80B0, CA: 0x3DFF00, MG: 0x8AFF00,
};
const DEFAULT_ELEMENT_COLOR = 0x8b949e;

function ensureDarkElementTheme() {
  if (_darkElementThemeRegistered || !plugin) return;
  const themeRegistry = plugin.representation.structure.themes.colorThemeRegistry;
  const { StructureProperties } = getLib().structure;
  const provider = {
    name: 'dark-element-symbol', label: 'Element (Dark)', category: 'Misc',
    factory: (_ctx: any, props: any) => {
      const colorFn = (location: any) => {
        try {
          const el = String(StructureProperties.atom.type_symbol(location)).toUpperCase();
          return ELEMENT_COLORS[el] ?? DEFAULT_ELEMENT_COLOR;
        } catch { return DEFAULT_ELEMENT_COLOR; }
      };
      return { factory: provider.factory, granularity: 'group' as const, color: colorFn, props, description: 'Element symbol (dark theme)' };
    },
    getParams: () => ({}), defaultValues: {}, isApplicable: () => true,
  };
  themeRegistry.add(provider);
  _darkElementThemeRegistered = true;
}

// ─── Pocket View (MolScript Components) ───

let _pocketViewRefs: string[] = [];

export async function clearPocketView(): Promise<void> {
  if (!plugin) return;
  const builder = plugin.build();
  let hasDeletes = false;
  for (const ref of _pocketViewRefs) {
    try {
      if (plugin.state.data.cells.get(ref)) { builder.delete(ref); hasDeletes = true; }
    } catch {}
  }
  if (hasDeletes) await builder.commit();
  _pocketViewRefs = [];
}

/**
 * Show protein as faded cartoon with pocket residues as ball-and-stick.
 * Preserves ligand (HETATM/UNL) component representations so the docked
 * ligand stays visible. Only modifies polymer representations.
 */
export async function showPocketView(residues: { chain_id: string; res_id: number }[]): Promise<void> {
  if (!plugin) return;
  try {
    const structures = plugin.managers?.structure?.hierarchy?.current?.structures;
    if (!structures?.length) return;
    const structRef = structures[0];
    const ref = structRef?.cell?.transform?.ref;
    if (!ref) return;

    const { StateTransforms } = getLib().plugin;

    await clearPocketView();

    // Only remove POLYMER representations — leave ligand/HETATM components alone.
    // Molstar creates components like "polymer", "ligand", "water", etc.
    // We identify polymer components by checking if their key/label contains 'polymer'
    // or if they're top-level (non-component) representations on the structure.
    const polymerReprRefs: string[] = [];
    for (const comp of (structRef.components ?? [])) {
      const key = comp.key ?? '';
      const label = comp.cell?.obj?.label ?? '';
      const isLigand = key.includes('ligand') || key.includes('non-standard') ||
                        label.toLowerCase().includes('ligand') || label.toLowerCase().includes('het') ||
                        label.includes('UNL');
      if (!isLigand) {
        for (const repr of (comp.representations ?? [])) {
          if (repr.cell?.transform?.ref) polymerReprRefs.push(repr.cell.transform.ref);
        }
      }
    }
    // Also remove top-level (non-component) representations
    for (const repr of (structRef.representations ?? [])) {
      if (repr.cell?.transform?.ref) polymerReprRefs.push(repr.cell.transform.ref);
    }

    if (polymerReprRefs.length > 0) {
      const delBuilder = plugin.build();
      for (const r of polymerReprRefs) delBuilder.delete(r);
      await delBuilder.commit();
    }

    // Build pocket view representations
    await plugin.dataTransaction(async () => {
      // 1. Whole protein as faded cartoon (applied to structure root —
      //    Molstar will only render polymer backbone for cartoon type)
      const cartoonBuilder = plugin.build();
      const cartoonNode = cartoonBuilder.to(ref).apply(StateTransforms.Representation.StructureRepresentation3D, {
        type: { name: 'cartoon', params: { sizeFactor: 0.2, alpha: 0.15 } },
        colorTheme: { name: 'uniform', params: { value: 0x484f58 } },
        sizeTheme: { name: 'uniform', params: { value: 0.2 } },
      });
      await cartoonBuilder.commit();
      try { if (cartoonNode.ref) _pocketViewRefs.push(cartoonNode.ref); } catch {}

      // 2. Pocket residues via MolScript component
      if (residues.length > 0) {
        ensureDarkElementTheme();

        try {
          const MS = getLib().script?.MolScriptBuilder ?? getLib().molScript?.MolScriptBuilder;
          if (MS) {
            // Build OR expression for all pocket residues
            const residueTests = residues.map(r =>
              MS.core.logic.and([
                MS.core.rel.eq([MS.struct.atomProperty.macromolecular.auth_asym_id(), r.chain_id]),
                MS.core.rel.eq([MS.struct.atomProperty.macromolecular.auth_seq_id(), r.res_id]),
              ])
            );
            const expr = MS.struct.generator.atomGroups({
              'residue-test': residues.length === 1 ? residueTests[0] : MS.core.logic.or(residueTests),
            });

            // Create component from expression, then add ball-and-stick
            const compBuilder = plugin.build();
            const compNode = compBuilder.to(ref).apply(StateTransforms.Model.StructureComponent, {
              type: { name: 'expression', params: expr },
              label: 'Pocket Residues',
            });
            compNode.apply(StateTransforms.Representation.StructureRepresentation3D, {
              type: { name: 'ball-and-stick', params: { sizeFactor: 0.15 } },
              colorTheme: { name: 'dark-element-symbol', params: {} },
              sizeTheme: { name: 'physical', params: {} },
            });
            await compBuilder.commit();

            // Track component ref for cleanup
            try { if (compNode.ref) _pocketViewRefs.push(compNode.ref); } catch {}
          }
        } catch (e) {
          console.warn('MolScript pocket component failed, falling back to PDB filter:', e);
          // Fallback: load filtered PDB as overlay (old approach)
          await _fallbackPocketOverlay(residues, ref);
        }
      }
    });

    applyCanvasProps();
  } catch (e) {
    console.error('showPocketView failed:', e);
  }
}

/** Fallback pocket overlay using filtered PDB text (if MolScript fails). */
async function _fallbackPocketOverlay(residues: { chain_id: string; res_id: number }[], _structRef: string): Promise<void> {
  if (!currentStructureText) return;
  let pdbText = currentStructureText;
  if (currentStructureFormat === 'pdbqt') {
    pdbText = pdbText.split('\n')
      .filter(line => line.startsWith('ATOM') || line.startsWith('TER') || line.startsWith('END'))
      .map(line => line.startsWith('ATOM') ? line.substring(0, 66).padEnd(80) : line)
      .join('\n');
  }
  const resSet = new Set(residues.map(r => `${r.chain_id}:${r.res_id}`));
  const out: string[] = [];
  for (const line of pdbText.split('\n')) {
    if (line.startsWith('ATOM') || line.startsWith('HETATM')) {
      const chain = line[21]?.trim() || 'A';
      const resSeq = parseInt(line.substring(22, 26).trim(), 10);
      if (resSet.has(`${chain}:${resSeq}`)) out.push(line);
    } else if (line.startsWith('TER') || line.startsWith('END')) {
      out.push(line);
    }
  }
  if (!out.some(l => l.startsWith('END'))) out.push('END');
  const pocketPdb = out.join('\n');
  if (!out.some(l => l.startsWith('ATOM') || l.startsWith('HETATM'))) return;

  const blob = new Blob([pocketPdb], { type: 'text/plain' });
  const url = URL.createObjectURL(blob);
  try { await viewerInstance.loadStructureFromUrl(url, 'pdb', false); }
  finally { setTimeout(() => URL.revokeObjectURL(url), 5000); }

  const structures = plugin.managers?.structure?.hierarchy?.current?.structures ?? [];
  const pocketStruct = structures[structures.length - 1];
  if (pocketStruct?.cell?.transform?.ref) {
    const pocketRef = pocketStruct.cell.transform.ref;
    const { StateTransforms } = getLib().plugin;
    // Remove auto-added representations
    const autoReprs = [
      ...(pocketStruct.components ?? []).flatMap((c: any) => c.representations ?? []),
      ...(pocketStruct.representations ?? []),
    ];
    if (autoReprs.length > 0) {
      const d = plugin.build();
      for (const repr of autoReprs) { if (repr.cell?.transform?.ref) d.delete(repr.cell.transform.ref); }
      await d.commit();
    }
    const bsBuilder = plugin.build();
    const bsNode = bsBuilder.to(pocketRef).apply(StateTransforms.Representation.StructureRepresentation3D, {
      type: { name: 'ball-and-stick', params: { sizeFactor: 0.15 } },
      colorTheme: { name: 'dark-element-symbol', params: {} },
      sizeTheme: { name: 'physical', params: {} },
    });
    await bsBuilder.commit();
    try { if (bsNode.ref) _pocketViewRefs.push(bsNode.ref); } catch {}
    _pocketViewRefs.push(pocketRef);
  }
}

// ─── Pocket Surface ───

let _surfaceRefs: string[] = [];

const DARK_THEME_MAP: Record<string, string> = {
  'residue-charge': 'dark-residue-charge',
  'element-symbol': 'dark-element-symbol',
  'hydrophobicity': 'hydrophobicity',
};

async function clearSurfaceRefs(): Promise<void> {
  if (!plugin || _surfaceRefs.length === 0) return;
  const builder = plugin.build();
  let hasDeletes = false;
  for (const ref of _surfaceRefs) {
    try {
      if (plugin.state.data.cells.get(ref)) { builder.delete(ref); hasDeletes = true; }
    } catch {}
  }
  if (hasDeletes) await builder.commit();
  _surfaceRefs = [];
}

export async function togglePocketSurface(show: boolean, colorTheme: string = 'residue-charge', alpha: number = 0.8): Promise<void> {
  if (!plugin) return;
  try {
    // If we have an existing surface and just need to update it (not remove)
    if (show && _surfaceRefs.length === 1) {
      const existingRef = _surfaceRefs[0];
      const cell = plugin.state.data.cells.get(existingRef);
      if (cell?.params?.values) {
        ensureDarkChargeTheme();
        ensureDarkElementTheme();
        const resolvedTheme = DARK_THEME_MAP[colorTheme] ?? colorTheme;
        const { StateTransforms } = getLib().plugin;
        // In-place update of the existing surface
        await plugin.build().to(existingRef).update(StateTransforms.Representation.StructureRepresentation3D, (old: any) => ({
          ...old,
          type: { name: 'gaussian-surface', params: { ...old.type?.params, smoothness: 1.5, alpha } },
          colorTheme: { name: resolvedTheme, params: {} },
        })).commit();
        return;
      }
    }

    await clearSurfaceRefs();
    if (!show) return;

    ensureDarkChargeTheme();
    ensureDarkElementTheme();

    const ref = getStructureRef();
    if (!ref) return;
    const { StateTransforms } = getLib().plugin;
    const resolvedTheme = DARK_THEME_MAP[colorTheme] ?? colorTheme;

    const update = plugin.build();
    const node = update.to(ref).apply(StateTransforms.Representation.StructureRepresentation3D, {
      type: { name: 'gaussian-surface', params: { smoothness: 1.5, alpha } },
      colorTheme: { name: resolvedTheme, params: {} },
      sizeTheme: { name: 'physical', params: {} },
    });
    await update.commit();
    try { if (node.ref) _surfaceRefs.push(node.ref); } catch {}

    applyCanvasProps();
  } catch (e) {
    console.error('togglePocketSurface failed:', e);
  }
}
