declare const molstar: any;

let plugin: any = null;

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
  return plugin !== null;
}

export function getPlugin(): any {
  return plugin;
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
  plugin = instance.plugin;

  if (plugin.canvas3d) {
    plugin.canvas3d.setProps({
      renderer: { ...plugin.canvas3d.props.renderer, backgroundColor: 0x0d1117 },
      trackball: { ...plugin.canvas3d.props.trackball, zoomSpeed: 3, scrollZoomSpeed: 0.3 },
    });
  }

  new ResizeObserver(() => plugin.canvas3d?.handleResize()).observe(container);
  container.addEventListener('wheel', (e: WheelEvent) => e.preventDefault(), { passive: false });

  setupInteractions();
}

function extractAtomInfo(loci: any): AtomInfo | null {
  try {
    if (!loci || loci.kind !== 'element-loci' || !loci.elements || loci.elements.length === 0) {
      return null;
    }
    const e = loci.elements[0];
    if (!e || !e.unit) return null;

    const unit = e.unit;
    const idx = e.indices?.[0] ?? 0;
    const elementIdx = unit.elements?.[idx];
    if (elementIdx === undefined) return null;

    const model = unit.model;
    if (!model) return null;

    const atoms = model.atomicHierarchy?.atoms;
    const residues = model.atomicHierarchy?.residues;
    const chains = model.atomicHierarchy?.chains;
    const conformation = model.atomicConformation;

    const residueIndex = model.atomicHierarchy?.residueAtomSegments?.index?.[elementIdx];

    const element = atoms?.type_symbol?.value?.(elementIdx) ?? '?';
    const atomName = atoms?.label_atom_id?.value?.(elementIdx) ?? '?';
    const residueName = residues?.label_comp_id?.value?.(residueIndex) ?? '?';
    const residueId = residues?.label_seq_id?.value?.(residueIndex) ?? 0;
    const chainId = chains?.label_asym_id?.value?.(
      model.atomicHierarchy?.chainAtomSegments?.index?.[elementIdx]
    ) ?? '?';

    const x = conformation?.x?.[elementIdx] ?? 0;
    const y = conformation?.y?.[elementIdx] ?? 0;
    const z = conformation?.z?.[elementIdx] ?? 0;

    return { element, atomName, residueName, residueId, chainId, x, y, z };
  } catch {
    return null;
  }
}

function setupInteractions(): void {
  if (!plugin?.canvas3d?.interaction) return;

  plugin.canvas3d.interaction.hover.subscribe((event: any) => {
    if (hoverCallback) {
      const info = extractAtomInfo(event?.current?.loci);
      hoverCallback(info);
    }
  });

  plugin.canvas3d.interaction.click.subscribe((event: any) => {
    if (clickCallback) {
      const info = extractAtomInfo(event?.current?.loci);
      clickCallback(info);
    }
  });
}

export function onHover(cb: InteractionCallback): void {
  hoverCallback = cb;
}

export function onClick(cb: InteractionCallback): void {
  clickCallback = cb;
}

export async function loadPdb(id: string): Promise<void> {
  if (!plugin) throw new Error('Viewer not initialized');
  plugin.clear();
  const url = `https://files.rcsb.org/download/${id.toUpperCase()}.cif`;
  await plugin.loadStructureFromUrl(url, 'mmcif', false);
}

export async function loadFile(data: string, format: string): Promise<void> {
  if (!plugin) throw new Error('Viewer not initialized');
  plugin.clear();

  const formatMap: Record<string, string> = {
    pdb: 'pdb',
    cif: 'mmcif',
    mmcif: 'mmcif',
    mol: 'mol',
    mol2: 'mol2',
    sdf: 'sdf',
    xyz: 'xyz',
  };
  const fmt = formatMap[format.toLowerCase()] || format;

  const blob = new Blob([data], { type: 'text/plain' });
  const url = URL.createObjectURL(blob);
  try {
    await plugin.loadStructureFromUrl(url, fmt, false);
  } finally {
    URL.revokeObjectURL(url);
  }
}

export function resetCamera(): void {
  try {
    plugin?.managers?.camera?.reset?.();
  } catch {
    // Camera reset is best-effort
  }
}

export function toggleSpin(enabled: boolean): void {
  try {
    if (plugin?.canvas3d) {
      plugin.canvas3d.setProps({
        trackball: { ...plugin.canvas3d.props.trackball, spin: enabled },
      });
    }
  } catch {
    // Spin toggle is best-effort
  }
}
