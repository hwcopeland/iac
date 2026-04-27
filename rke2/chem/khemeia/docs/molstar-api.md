# Molstar API Reference

Single source of truth for developers working on the Khemeia molecular viewer
(`web/src/lib/viewer.ts`). Covers the Molstar APIs used throughout the codebase,
including patterns, pitfalls, and Khemeia-specific conventions.

**Molstar version**: 5.8.0 (MIT license)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Viewer Creation and Options](#2-viewer-creation-and-options)
3. [Canvas3D and Camera](#3-canvas3d-and-camera)
4. [Structure Hierarchy](#4-structure-hierarchy)
5. [State Builder Pattern](#5-state-builder-pattern)
6. [Representations](#6-representations)
7. [Color Themes](#7-color-themes)
8. [Structure Components and Selections](#8-structure-components-and-selections)
9. [Loci](#9-loci)
10. [Interaction Events](#10-interaction-events)
11. [Shape Representations](#11-shape-representations)
12. [Anti-Patterns](#12-anti-patterns)
13. [Quick Reference](#13-quick-reference)

---

## 1. Overview

Khemeia loads Molstar as a **UMD global bundle**, not as ES modules. The bundle is loaded
via a `<script>` tag in `app.html`:

```html
<link rel="stylesheet" href="/molstar.css" />
<script src="/molstar.js"></script>
```

This exposes a global `molstar` object. All API access flows through two entry points:

| Entry Point | What It Gives You |
|---|---|
| `molstar.Viewer.create(container, options)` | A `Viewer` instance whose `.plugin` property is the core Molstar plugin context |
| `molstar.lib.*` | Access to internal Molstar modules: `molstar.lib.structure`, `molstar.lib.plugin`, `molstar.lib.script`, `molstar.lib.volume`, `molstar.lib.int` |

In `viewer.ts`, the library accessor is:

```js
function getLib() {
  return molstar.lib;
}
```

The global `plugin` variable holds `viewer.plugin` after initialization. Nearly every
Molstar operation goes through this object.

---

## 2. Viewer Creation and Options

### Creating a Viewer

```js
const viewer = await molstar.Viewer.create(container, options);
const plugin = viewer.plugin;
```

`container` is an `HTMLDivElement`. The viewer renders its WebGL canvas inside it.

### ViewerOptions

All fields are optional. These are the options Khemeia uses at initialization:

**Layout options** -- control which Molstar UI panels are visible:

| Option | Type | Description |
|---|---|---|
| `layoutIsExpanded` | `boolean` | Start with the layout fully expanded |
| `layoutShowControls` | `boolean` | Show the Molstar controls UI |
| `layoutShowRemoteState` | `boolean` | Show the remote state panel |
| `layoutShowSequence` | `boolean` | Show the sequence bar |
| `layoutShowLog` | `boolean` | Show the log panel |
| `layoutShowLeftPanel` | `boolean` | Show the left panel |
| `collapseLeftPanel` | `boolean` | Start with the left panel collapsed |
| `collapseRightPanel` | `boolean` | Start with the right panel collapsed |

**Viewport options** -- control viewport toolbar buttons:

| Option | Type | Description |
|---|---|---|
| `viewportShowControls` | `boolean` | Show viewport overlay controls |
| `viewportShowExpand` | `boolean` | Show the expand button |
| `viewportShowSettings` | `boolean` | Show the settings gear |
| `viewportShowSelectionMode` | `boolean` | Show the selection mode toggle |
| `viewportShowAnimation` | `boolean` | Show animation controls |
| `viewportShowTrajectoryControls` | `boolean` | Show trajectory frame controls |
| `viewportShowReset` | `boolean` | Show the camera reset button |
| `viewportShowScreenshotControls` | `boolean` | Show the screenshot button |
| `viewportShowToggleFullscreen` | `boolean` | Show the fullscreen toggle |

**Rendering options:**

| Option | Type | Description |
|---|---|---|
| `disableAntialiasing` | `boolean` | Turn off antialiasing |
| `pixelScale` | `number` | Pixel density multiplier |
| `transparency` | `string` | Transparency mode: `'blended'`, `'wboit'`, or `'dpoit'` |

### Khemeia's defaults

```js
await molstar.Viewer.create(container, {
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
```

### Viewer Methods

| Method | Signature | Description |
|---|---|---|
| `loadStructureFromUrl` | `(url: string, format?: string, isBinary?: boolean, options?: object)` | Load a structure from a URL |
| `loadStructureFromData` | `(data: string \| Uint8Array, format: string, options?: object)` | Load a structure from raw data |
| `loadPdb` | `(pdb: string, options?: object)` | Load a PDB by ID from RCSB |
| `handleResize` | `()` | Recalculate viewport dimensions |
| `dispose` | `()` | Destroy the viewer and release resources |

Khemeia uses `loadStructureFromUrl` for both remote fetches and local data (via
`URL.createObjectURL`):

```js
const blob = new Blob([data], { type: 'text/plain' });
const url = URL.createObjectURL(blob);
await viewer.loadStructureFromUrl(url, 'pdb', false);
setTimeout(() => URL.revokeObjectURL(url), 5000);
```

---

## 3. Canvas3D and Camera

### Setting Canvas Properties

**Canonical approach** (via PluginCommands):

```js
import { PluginCommands } from 'molstar/lib/mol-plugin/commands';

PluginCommands.Canvas3D.SetSettings(plugin, {
  settings: {
    renderer: { ...plugin.canvas3d.props.renderer, backgroundColor: 0x0d1117 },
    trackball: { ...plugin.canvas3d.props.trackball, maxWheelDelta: 0.005 },
  },
});
```

**Lower-level** (used in Khemeia's `applyCanvasProps()`):

```js
plugin.canvas3d.setProps({
  renderer: { ...plugin.canvas3d.props.renderer, backgroundColor: 0x0d1117 },
  trackball: { ...plugin.canvas3d.props.trackball, maxWheelDelta: 0.005 },
});
```

Both work. The `PluginCommands` approach goes through the command system and integrates
with undo/redo. Direct `setProps` is simpler for fire-and-forget changes like background
color.

**Important**: `plugin.clear()` resets canvas props. Khemeia calls `applyCanvasProps()`
after every `clear()` or `loadStructureFromUrl()` to restore the dark background.

### Camera

| Operation | API |
|---|---|
| Focus on a loci | `plugin.managers.camera.focusLoci(loci, { durationMs: 250 })` |
| Reset to default view | `plugin.managers.camera.reset()` |
| Get camera snapshot | `plugin.canvas3d.camera.getSnapshot()` or `plugin.canvas3d.camera.snapshot` |
| Restore camera snapshot | `plugin.canvas3d.requestCameraReset({ snapshot, durationMs: 0 })` |
| Set snapshot via command | `PluginCommands.Camera.SetSnapshot(plugin, { snapshot, durationMs })` |

Khemeia uses snapshot save/restore to preserve the user's viewpoint across structure
reloads:

```js
const snapshot = plugin.canvas3d.camera.getSnapshot();
// ... reload structure ...
plugin.canvas3d.requestCameraReset({ snapshot, durationMs: 0 });
```

### Resize Handling

Molstar does not automatically respond to container size changes. Use a `ResizeObserver`:

```js
new ResizeObserver(() => plugin.canvas3d?.handleResize()).observe(container);
```

---

## 4. Structure Hierarchy

The structure hierarchy is Molstar's tree of loaded structures, their components, and
their representations. Access it at:

```js
plugin.managers.structure.hierarchy.current
```

### Hierarchy Shape

```
StructureHierarchy
  structures: StructureRef[]

StructureRef
  cell: StateObjectCell
    .obj?.data          -> Structure (the molecular data)
    .transform.ref      -> string (state ref ID for this node)
  components: StructureComponentRef[]
  representations: StructureRepresentationRef[]  (top-level, not inside a component)

StructureComponentRef
  cell: StateObjectCell
  representations: StructureRepresentationRef[]

StructureRepresentationRef
  cell: StateObjectCell
    .transform.ref      -> string (state ref ID)
    .params.values       -> current representation parameters
```

### Common Access Patterns

```js
// Get all loaded structures
const structures = plugin.managers.structure.hierarchy.current.structures;

// Get the first structure's molecular data
const structData = structures[0].cell.obj?.data;

// Get the state ref string for the first structure
const ref = structures[0].cell.transform.ref;

// Iterate all representations across all components
for (const struct of structures) {
  for (const comp of struct.components ?? []) {
    for (const repr of comp.representations ?? []) {
      const reprRef = repr.cell.transform.ref;
      const reprParams = repr.cell.params.values;
    }
  }
  // Also check top-level representations (not inside a component)
  for (const repr of struct.representations ?? []) {
    // ...
  }
}
```

### Getting Model Count (Trajectory Frames)

```js
const models = structures[0].cell.obj?.data?.models?.length ?? 0;
```

---

## 5. State Builder Pattern

The state builder is the core pattern for modifying Molstar's internal state tree. All
structural changes -- adding representations, creating components, deleting nodes -- go
through this mechanism.

### Basic Pattern

```js
const update = plugin.build();             // create a builder
update
  .to(parentRef)                           // navigate to a node by state ref string
  .apply(SomeTransform, params)            // add a child transform
  .apply(AnotherTransform, params);        // chain another transform
await update.commit();                     // execute all changes atomically
```

### Delete a Node

```js
await plugin.build().delete(ref).commit();
```

### Update an Existing Node

```js
const { StateTransforms } = molstar.lib.plugin;

plugin.build()
  .to(ref)
  .update(StateTransforms.Representation.StructureRepresentation3D, (old) => ({
    ...old,
    colorTheme: { name: 'chain-id', params: {} },
  }))
  .commit();
```

### Batch Operations

Build everything in one `plugin.build()...commit()` call when possible. Each `commit()`
triggers a state update cycle.

```js
// Good: one commit
const builder = plugin.build();
for (const ref of refsToDelete) {
  builder.delete(ref);
}
await builder.commit();

// Bad: N commits
for (const ref of refsToDelete) {
  await plugin.build().delete(ref).commit();  // triggers N state updates
}
```

### Data Transactions

Wrap multiple operations in a transaction for atomicity and optional undo support:

```js
await plugin.dataTransaction(async () => {
  // All state changes inside here are batched
  const data = await plugin.builders.data.rawData({ data: cubeData, label: 'Cube File' });
  const parsed = await cubeFormat.parse(plugin, data);
  // ...
}, { canUndo: 'My Operation' });
```

### Clearing Everything

```js
await plugin.clear();
```

This removes all structures, representations, and data from the state tree. It also
resets canvas props -- call `applyCanvasProps()` afterward if you need custom settings.

---

## 6. Representations

### Built-In Representation Types

Use these as the `type.name` field when creating representations:

| Type Name | Description |
|---|---|
| `cartoon` | Ribbon/tube for secondary structure |
| `backbone` | Backbone trace only |
| `ball-and-stick` | Atoms as spheres, bonds as cylinders |
| `spacefill` | Van der Waals spheres (CPK) |
| `gaussian-surface` | Smooth analytical surface |
| `molecular-surface` | Connolly/solvent-excluded surface |
| `line` | Wireframe bonds |
| `point` | Point cloud of atom positions |
| `putty` | Tube with variable radius (e.g., B-factor) |
| `label` | Text labels at atom positions |
| `ellipsoid` | Anisotropic displacement ellipsoids |
| `orientation` | Unit cell / orientation indicators |
| `carbohydrate` | SNFG carbohydrate symbols |
| `gaussian-volume` | Volumetric Gaussian representation |
| `plane` | Planar ring indicators |
| `polyhedron` | Coordination polyhedra |

### Adding a Representation (State Builder)

```js
const { StateTransforms } = molstar.lib.plugin;

plugin.build()
  .to(structureOrComponentRef)
  .apply(StateTransforms.Representation.StructureRepresentation3D, {
    type: { name: 'cartoon', params: { sizeFactor: 0.2, alpha: 0.5 } },
    colorTheme: { name: 'uniform', params: { value: 0x484f58 } },
    sizeTheme: { name: 'physical', params: {} },
  })
  .commit();
```

### Adding a Representation (Component Manager)

Higher-level API that uses the component system:

```js
plugin.managers.structure.component.addRepresentation(components, 'cartoon');
```

Or with the builder helpers:

```js
const polymer = await plugin.builders.structure.tryCreateComponentStatic(structureRef, 'polymer');
await plugin.builders.structure.representation.addRepresentation(polymer, {
  type: 'cartoon',
  color: 'chain-id',
});
```

### Common Params (Shared by All Types)

| Param | Type | Description |
|---|---|---|
| `alpha` | `number` (0-1) | Opacity. Khemeia uses 0.15 for ghost protein, 0.7-0.9 for surfaces |
| `quality` | `string` | `'auto'`, `'medium'`, `'high'`, `'low'`, `'custom'`, `'highest'`, `'higher'`, `'lower'`, `'lowest'` |
| `material` | `object` | `{ metalness, roughness, bumpiness }` |
| `doubleSided` | `boolean` | Render both sides of surfaces |
| `flatShaded` | `boolean` | Flat shading instead of smooth |
| `ignoreLight` | `boolean` | Ignore scene lighting |
| `xrayShaded` | `boolean` | X-ray style transparency |
| `celShaded` | `boolean` | Cel/toon shading |

### Type-Specific Params

**cartoon:**

| Param | Type | Description |
|---|---|---|
| `sizeFactor` | `number` | Tube/ribbon thickness multiplier |
| `tubularHelices` | `boolean` | Render helices as tubes |
| `helixProfile` | `string` | `'square'`, `'elliptical'`, `'rounded'` |
| `aspectRatio` | `number` | Width-to-height ratio of the ribbon |
| `arrowFactor` | `number` | Arrow size at strand ends |
| `linearSegments` | `number` | Smoothness along the backbone |
| `radialSegments` | `number` | Cross-section smoothness |
| `colorMode` | `string` | `'default'`, `'interpolate'` |

**ball-and-stick:**

| Param | Type | Description |
|---|---|---|
| `sizeFactor` | `number` | Atom/bond size. Khemeia uses 0.15-0.2 |
| `sizeAspectRatio` | `number` | Bond width relative to atom size |
| `includeTypes` | `string[]` | Bond types to show: `covalent`, `metal-coordination`, `hydrogen-bond`, `disulfide`, `aromatic`, `computed` |
| `excludeTypes` | `string[]` | Bond types to hide |
| `ignoreHydrogens` | `boolean` | Hide hydrogen atoms |
| `aromaticBonds` | `boolean` | Show aromatic ring indicators |
| `multipleBonds` | `string` | `'offset'`, `'off'`, `'symmetric'` |

**gaussian-surface:**

| Param | Type | Description |
|---|---|---|
| `smoothness` | `number` | Surface smoothing. Khemeia uses 1.5 |
| `radiusOffset` | `number` | Expand/contract the surface |
| `resolution` | `number` | Grid resolution |
| `alpha` | `number` | Surface opacity |
| `tryUseGpu` | `boolean` | Use GPU for surface computation |
| `ignoreHydrogens` | `boolean` | Exclude H atoms from surface |

**molecular-surface:**

| Param | Type | Description |
|---|---|---|
| `probeRadius` | `number` | Solvent probe radius (angstroms) |
| `probePositions` | `number` | Number of probe positions |
| `resolution` | `number` | Surface mesh resolution |

---

## 7. Color Themes

### Built-In Theme Names

| Theme Name | Colors By | Key Params |
|---|---|---|
| `uniform` | Single color | `{ value: 0xRRGGBB }` |
| `element-symbol` | Atom element (CPK) | `{ carbonColor, saturation, lightness }` |
| `chain-id` | Chain identifier | `{ palette, asymId: 'label'\|'auth' }` |
| `entity-id` | Entity | -- |
| `residue-name` | Residue type | -- |
| `residue-charge` | Residue charge | -- |
| `secondary-structure` | Helix/sheet/coil | -- |
| `hydrophobicity` | Hydrophobicity scale | `{ scale: 'DGwif'\|'DGwoct'\|'Oct-IF' }` |
| `molecule-type` | Protein/nucleic/etc. | -- |
| `polymer-id` | Polymer chain | -- |
| `sequence-id` | Residue sequence number | -- |
| `atom-id` | Atom serial number | -- |
| `model-index` | Model/frame number | -- |
| `structure-index` | Structure index | -- |
| `illustrative` | Illustrative style | -- |
| `partial-charge` | Partial atomic charge | -- |
| `formal-charge` | Formal charge | -- |
| `occupancy` | Occupancy / uncertainty | -- |
| `cartoon` | Cartoon-style | -- |

### Updating the Theme on Existing Representations

**Canonical high-level approach:**

```js
await plugin.dataTransaction(async () => {
  for (const s of plugin.managers.structure.hierarchy.current.structures) {
    await plugin.managers.structure.component.updateRepresentationsTheme(
      s.components,
      { color: 'element-symbol' }
    );
  }
});
```

**State builder approach** (used in Khemeia's `setColorTheme()`):

```js
const { StateTransforms } = molstar.lib.plugin;

const cell = plugin.state.data.cells.get(reprRef);
if (cell?.params?.values) {
  plugin.build()
    .to(reprRef)
    .update(StateTransforms.Representation.StructureRepresentation3D, (old) => ({
      ...old,
      colorTheme: { name: 'element-symbol', params: {} },
    }))
    .commit();
}
```

### Registering a Custom Color Theme

Khemeia registers three custom themes: `esp-on-density`, `dark-residue-charge`, and
`dark-element-symbol`. Here is the pattern:

```js
const provider = {
  name: 'my-custom-theme',
  label: 'My Theme',
  category: 'Misc',
  factory: (ctx, props) => ({
    factory: provider.factory,
    granularity: 'group',    // 'uniform' | 'instance' | 'group' | 'groupInstance' | 'vertex'
    color: (location) => {
      // location has .unit and .element
      // Use StructureProperties to read atom/residue/chain data
      const resName = StructureProperties.residue.label_comp_id(location);
      return 0xRRGGBB;
    },
    props,
    description: 'My custom colors',
  }),
  getParams: () => ({}),
  defaultValues: {},
  isApplicable: () => true,
};

// Register with the structure theme registry
plugin.representation.structure.themes.colorThemeRegistry.add(provider);

// For volume themes, use the volume registry instead:
// plugin.representation.volume.themes.colorThemeRegistry.add(provider);
```

**Granularity options:**

| Granularity | Meaning | Use When |
|---|---|---|
| `'uniform'` | Same color everywhere | Fallback / error state |
| `'instance'` | Per-instance (chain copy) | Symmetry coloring |
| `'group'` | Per-residue group | Most residue-level coloring |
| `'groupInstance'` | Per-residue per-instance | Residue + symmetry |
| `'vertex'` | Per-vertex position | Continuous property mapping (ESP) |

**Note on surfaces**: `gaussian-surface` interpolates colors per-vertex. If your custom
theme uses `'group'` granularity, colors appear per-residue, which works for most
use cases. Use `'vertex'` only when you need position-dependent coloring (like ESP
mapping), and set `preferSmoothing: true` on the returned theme object.

### Khemeia's Custom Themes

**`dark-residue-charge`** -- charge coloring optimized for the `#0d1117` background:

| Charge | Color | Hex |
|---|---|---|
| Positive (LYS, ARG, HIS) | Blue | `0x58a6ff` |
| Negative (ASP, GLU) | Red | `0xf85149` |
| Neutral | Dark grey | `0x3b434d` |

**`dark-element-symbol`** -- CPK coloring with carbon as neutral grey:

| Element | Color | Hex |
|---|---|---|
| C | Neutral grey | `0x606870` |
| N | Blue | `0x3050F8` |
| O | Red | `0xFF0D0D` |
| S | Yellow | `0xFFFF30` |
| P | Orange | `0xFF8000` |
| H | Light grey | `0x8b949e` |

**`esp-on-density`** -- electrostatic potential mapped onto an electron density isosurface.
Uses `'vertex'` granularity with a Red (negative) to White (neutral) to Blue (positive)
color ramp. Registered on the **volume** theme registry, not the structure registry.

---

## 8. Structure Components and Selections

Components let you apply a representation to a **subset** of atoms (e.g., only pocket
residues, only the polymer backbone, only ligands). This is the mechanism for scoped
representations.

### Static Component Types

Built-in subsets you can create without writing a selection expression:

| Type Name | Selects |
|---|---|
| `'all'` | Everything |
| `'polymer'` | Polymer chains |
| `'protein'` | Protein chains only |
| `'nucleic'` | Nucleic acid chains only |
| `'water'` | Water molecules |
| `'branched'` | Branched entities (carbohydrates) |
| `'ligand'` | Ligand molecules |
| `'ion'` | Ions |
| `'lipid'` | Lipid molecules |
| `'coarse'` | Coarse-grained entities |
| `'non-standard'` | Non-standard residues |

**Creating a static component:**

```js
const polymer = await plugin.builders.structure.tryCreateComponentStatic(
  structureRef,
  'polymer'
);

await plugin.builders.structure.representation.addRepresentation(polymer, {
  type: 'cartoon',
  color: 'chain-id',
});
```

### Expression-Based Components (Custom Selections)

Use `MolScriptBuilder` to define which atoms belong to a component.

```js
const MS = molstar.lib.script.MolScriptBuilder;
```

**Select specific residues by chain and residue ID:**

```js
const expr = MS.struct.generator.atomGroups({
  'residue-test': MS.core.logic.or([
    MS.core.logic.and([
      MS.core.rel.eq([MS.struct.atomProperty.macromolecular.auth_asym_id(), 'A']),
      MS.core.rel.eq([MS.struct.atomProperty.macromolecular.auth_seq_id(), 42]),
    ]),
    MS.core.logic.and([
      MS.core.rel.eq([MS.struct.atomProperty.macromolecular.auth_asym_id(), 'A']),
      MS.core.rel.eq([MS.struct.atomProperty.macromolecular.auth_seq_id(), 107]),
    ]),
  ]),
});
```

**Create the component, then add a representation to it:**

```js
const { StateTransforms } = molstar.lib.plugin;

const update = plugin.build();

const component = update
  .to(structureRef)
  .apply(StateTransforms.Model.StructureComponent, {
    type: { name: 'expression', params: expr },
    label: 'Pocket Residues',
  });

component.apply(StateTransforms.Representation.StructureRepresentation3D, {
  type: { name: 'ball-and-stick', params: { sizeFactor: 0.15 } },
  colorTheme: { name: 'element-symbol', params: {} },
  sizeTheme: { name: 'physical', params: {} },
});

await update.commit();
```

This creates two state tree nodes in one commit: the component (atom subset) and its
representation. The representation only applies to atoms in the component.

### MolScript Property Accessors

Commonly used property accessors within `MS.struct.atomProperty.macromolecular`:

| Accessor | Returns | Corresponds To |
|---|---|---|
| `auth_asym_id()` | Chain ID (author) | PDB column 22 |
| `auth_seq_id()` | Residue number (author) | PDB columns 23-26 |
| `label_comp_id()` | Residue name | PDB columns 18-20 |
| `label_atom_id()` | Atom name | PDB columns 13-16 |
| `type_symbol()` | Element symbol | PDB columns 77-78 |

---

## 9. Loci

A Loci is a set of selected atoms within a structure. Loci are used for highlighting,
selecting, and camera focusing. They are **not** plain arrays -- they use Molstar's
internal `OrderedSet` types.

### Building a Loci from Residue IDs

**Method 1: Manual iteration** (used in Khemeia's `focusResidue()` and `highlightResidue()`):

```js
const { StructureElement, StructureProperties } = molstar.lib.structure;

const structData = structures[0].cell.obj?.data;
const lociElements = [];

for (const unit of structData.units) {
  const indices = [];
  for (let i = 0; i < unit.elements.length; i++) {
    const elIdx = unit.elements[i];
    const loc = StructureElement.Location.create(structData, unit, elIdx);
    const chain = StructureProperties.chain.auth_asym_id(loc);
    const resId = StructureProperties.residue.auth_seq_id(loc);
    if (chain === 'A' && resId === 42) {
      indices.push(i);
    }
  }
  if (indices.length > 0) {
    lociElements.push({ unit, indices });
  }
}

const loci = StructureElement.Loci(structData, lociElements);
```

**Method 2: Using SortedArray** (required for some Molstar internals):

```js
const { SortedArray } = molstar.lib.int;

// indices MUST be an Int32Array wrapped in SortedArray, not a plain number[]
lociElements.push({
  unit,
  indices: SortedArray.ofSortedArray(new Int32Array(indices)),
});
```

**Method 3: Using Structure.Loci** (for focusing on an entire structure):

```js
const { Structure } = molstar.lib.structure;
const loci = Structure.Loci(structData);
```

### Highlighting

Highlighting shows a visual overlay without changing the selection state:

```js
plugin.managers.interactivity.lociHighlights.highlightOnly({ loci });
plugin.managers.interactivity.clearHighlights();
```

### Selecting

Selection marks atoms persistently (survives hover events):

```js
plugin.managers.interactivity.lociSelects.select({ loci });
plugin.managers.interactivity.lociSelects.deselectAll();
```

### Camera Focus

```js
plugin.managers.camera.focusLoci(loci, { durationMs: 250 });
```

---

## 10. Interaction Events

Subscribe to hover and click events on atoms in the 3D viewport.

```js
plugin.behaviors.interaction.hover.subscribe((event) => {
  const reprLoci = event.current;
  // reprLoci is a Representation.Loci wrapper
});

plugin.behaviors.interaction.click.subscribe((event) => {
  const reprLoci = event.current;
});
```

### Extracting Atom Information

The safest way to extract atom data from a Representation.Loci is through
`StructureElement.Stats`:

```js
const { StructureElement, StructureProperties } = molstar.lib.structure;

function extractAtomInfo(reprLoci) {
  const loci = reprLoci?.loci ?? reprLoci;
  if (!loci || loci.kind !== 'element-loci' || !loci.elements?.length) return null;

  // Stats.ofLoci handles all OrderedSet types internally
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
}
```

### StructureProperties Namespaces

| Namespace | Properties |
|---|---|
| `StructureProperties.atom` | `type_symbol`, `label_atom_id`, `x`, `y`, `z`, `id`, `occupancy`, `B_iso_or_equiv` |
| `StructureProperties.residue` | `label_comp_id`, `auth_seq_id`, `label_seq_id`, `pdbx_formal_charge` |
| `StructureProperties.chain` | `auth_asym_id`, `label_asym_id`, `label_entity_id` |

All property accessors take a `StructureElement.Location` and return the value.

---

## 11. Shape Representations

For drawing custom 3D geometry (lines, spheres, cylinders) that is not derived from
molecular structure data.

### ShapeProvider Interface

```js
const shapeProvider = {
  label: 'My Lines',
  data: yourData,
  params: GeometryType.Params,
  getShape: (ctx, data, props) => {
    return Shape.create(
      'my-shape',     // name
      data,           // data reference
      geometry,       // Lines, Mesh, Spheres, Cylinders, etc.
      colorFn,        // (groupId) => Color
      sizeFn,         // (groupId) => number
      groupFn         // (groupId) => string (label)
    );
  },
  geometryUtils: GeometryType.Utils,
};
```

### Available Geometry Types

| Type | Description |
|---|---|
| `Lines` | Line segments (dashed or solid) |
| `Mesh` | Triangle meshes |
| `Spheres` | Sphere primitives |
| `Cylinders` | Cylinder primitives |
| `Points` | Point cloud |
| `Text` | 3D text labels |
| `DirectVolume` | Volumetric rendering |

### Adding to State

1. Create an `SO.Shape.Provider` node in the state tree
2. Apply `StateTransforms.Representation.ShapeRepresentation3D`

### Built-In Measurement Transforms

For distance/angle measurements, Molstar provides ready-made transforms:

| Transform | Measures |
|---|---|
| `StructureSelectionsDistance3D` | Distance between two selections. Params: `linesColor`, `linesSize`, `dashLength` |
| `StructureSelectionsAngle3D` | Angle between three selections |
| `StructureSelectionsDihedral3D` | Dihedral angle between four selections |

---

## 12. Anti-Patterns

These are specific mistakes discovered in the Khemeia codebase or likely to occur when
working with the Molstar API. Avoid them.

### 1. Applying Representation Directly to a Structure Ref

**Wrong:**
```js
builder.to(structureRef).apply(StructureRepresentation3D, { ... });
```

This bypasses the component system. It works, but the representation is not tracked as
part of a component, making cleanup and theme updates unreliable. The representation
may not appear in `structure.components[].representations[]` and will only show up in
`structure.representations[]` (the top-level fallback).

**Correct:**
```js
// Create a component first, then add the representation to it
const comp = builder.to(structureRef).apply(StateTransforms.Model.StructureComponent, {
  type: { name: 'all', params: {} },
});
comp.apply(StateTransforms.Representation.StructureRepresentation3D, { ... });
```

**When Khemeia does it anyway**: For quick overlays (pocket view, surfaces) where the
entire structure is the target and cleanup is handled by tracking refs manually.

### 2. Manual Representation Deletion Loops

**Wrong:**
```js
for (const repr of allRepresentations) {
  await plugin.build().delete(repr.cell.transform.ref).commit();
}
```

Each `.commit()` triggers a full state update. With N representations, this causes N
re-renders.

**Correct:**
```js
const builder = plugin.build();
for (const repr of allRepresentations) {
  builder.delete(repr.cell.transform.ref);
}
await builder.commit();  // one state update
```

Or use the manager:
```js
plugin.managers.structure.component.clear(structures);
```

### 3. Passing Plain number[] as Loci Indices

**Wrong:**
```js
lociElements.push({ unit, indices: [0, 1, 2, 3] });
const loci = StructureElement.Loci(structData, lociElements);
```

`StructureElement.Loci` expects `{ unit, indices: OrderedSet }` where `OrderedSet` must be
a `SortedArray` (backed by `Int32Array`) or an `Interval`. Plain `number[]` will silently
produce a broken loci that fails to match any atoms.

**Correct:**
```js
const { SortedArray } = molstar.lib.int;
lociElements.push({
  unit,
  indices: SortedArray.ofSortedArray(new Int32Array([0, 1, 2, 3])),
});
```

**Khemeia workaround**: In some places, Khemeia passes plain `number[]` and it
happens to work because Molstar's internal code coerces the type in certain paths. This
is fragile and should not be relied upon for new code.

### 4. Single Commits Inside Loops

**Wrong:**
```js
for (const struct of structures) {
  const builder = plugin.build();
  builder.to(struct.ref).apply(Transform, params);
  await builder.commit();  // N commits = N state updates
}
```

**Correct:**
```js
const builder = plugin.build();
for (const struct of structures) {
  builder.to(struct.ref).apply(Transform, params);
}
await builder.commit();  // 1 commit
```

### 5. Not Using createStructureRepresentationParams()

Manually constructing `{ type: { name, params }, colorTheme: { name, params } }` is
fragile because the params shape varies by representation type and Molstar version.
The `createStructureRepresentationParams` helper validates params against the registry
and fills in defaults.

In practice, Khemeia constructs params manually because it uses the UMD global bundle
where the helper is less accessible. When doing so, keep params minimal and let Molstar
use its defaults for unspecified fields.

---

## 13. Quick Reference

| Task | API |
|---|---|
| Create viewer | `molstar.Viewer.create(container, options)` |
| Get plugin | `viewer.plugin` |
| Get internal modules | `molstar.lib.structure`, `molstar.lib.plugin`, etc. |
| Load from URL | `viewer.loadStructureFromUrl(url, format, isBinary)` |
| Load from data | `viewer.loadStructureFromData(data, format)` |
| Clear all | `plugin.clear()` |
| Set background color | `plugin.canvas3d.setProps({ renderer: { ...plugin.canvas3d.props.renderer, backgroundColor: 0x0d1117 } })` |
| Handle resize | `plugin.canvas3d.handleResize()` |
| Get all structures | `plugin.managers.structure.hierarchy.current.structures` |
| Get structure data | `structures[0].cell.obj?.data` |
| Get state ref | `structures[0].cell.transform.ref` |
| Build state update | `plugin.build().to(ref).apply(Transform, params).commit()` |
| Delete node | `plugin.build().delete(ref).commit()` |
| Update node | `plugin.build().to(ref).update(Transform, old => ({...old, changes})).commit()` |
| Batch operations | `plugin.dataTransaction(async () => { ... })` |
| Create static component | `plugin.builders.structure.tryCreateComponentStatic(ref, 'polymer')` |
| Create expression component | `.apply(StateTransforms.Model.StructureComponent, { type: { name: 'expression', params: expr } })` |
| Add representation | `.apply(StateTransforms.Representation.StructureRepresentation3D, { type, colorTheme, sizeTheme })` |
| Update theme (high-level) | `plugin.managers.structure.component.updateRepresentationsTheme(components, { color: name })` |
| Update theme (builder) | `builder.to(reprRef).update(Transform, old => ({...old, colorTheme: { name, params }}))` |
| Register structure theme | `plugin.representation.structure.themes.colorThemeRegistry.add(provider)` |
| Register volume theme | `plugin.representation.volume.themes.colorThemeRegistry.add(provider)` |
| Focus camera on atoms | `plugin.managers.camera.focusLoci(loci, { durationMs: 250 })` |
| Reset camera | `plugin.managers.camera.reset()` |
| Save camera | `plugin.canvas3d.camera.getSnapshot()` |
| Restore camera | `plugin.canvas3d.requestCameraReset({ snapshot, durationMs: 0 })` |
| Highlight atoms | `plugin.managers.interactivity.lociHighlights.highlightOnly({ loci })` |
| Clear highlights | `plugin.managers.interactivity.clearHighlights()` |
| Select atoms | `plugin.managers.interactivity.lociSelects.select({ loci })` |
| Deselect all | `plugin.managers.interactivity.lociSelects.deselectAll()` |
| Subscribe to hover | `plugin.behaviors.interaction.hover.subscribe(event => { ... })` |
| Subscribe to click | `plugin.behaviors.interaction.click.subscribe(event => { ... })` |
| Get model/frame count | `structures[0].cell.obj?.data?.models?.length` |
| Access data formats | `plugin.dataFormats.get('cube')` |
| Load raw data | `plugin.builders.data.rawData({ data, label })` |
