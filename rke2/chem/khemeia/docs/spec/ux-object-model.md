# Khemeia UX Object Model

## Purpose

This document describes the fundamental design philosophy behind the Khemeia platform and is
the primary reference for UI redesign decisions. Every interface element, API shape, and
controller behavior should be evaluated against the model defined here.

---

## Core Thesis

**The user does not submit jobs. The user works with objects.**

A job is an implementation detail — a verb applied to one or more objects that produces new
objects. The user's mental model is:

> "I have *this data*. I want to know *this thing* about it."

The platform's job is to make that question answerable without the user having to understand
the underlying compute. The controller manages that machinery invisibly. The UI surfaces
objects and lets the user ask questions of them.

---

## The Object Model

### What is a Molecular Data Object?

A Molecular Data Object (MDO) is any persistent, referenceable piece of chemical data in the
platform. Every MDO has:

- A stable ID (never changes after creation)
- A type (see below)
- A chemical identity or chemical relationship to other objects
- A provenance chain (how it was produced)
- An owner and a session/workspace it belongs to
- A state (`pending`, `ready`, `error`, `archived`)

Objects are **not** ephemeral. A DockingResult produced six months ago is still there. A
CompoundLibrary built from a ChEMBL query is still linked to the original query parameters.
A RefinedPose is still linked to the Compound it is a pose of.

### Object Types

```
MolecularTarget
  └── protein structure (PDB/CIF/mmCIF)
  └── binding site definition (residue list + centroid + radius)
  └── prepared topology (protonated, solvated, force-field assigned)

CompoundLibrary
  └── named, versioned list of Compounds
  └── source: file upload, SMILES paste, database query, SAR expansion
  └── may overlap with other libraries (same Compound appears in many libraries)

Compound
  └── canonical SMILES (the chemical identity key)
  └── assigned stable ID (KHM-XXXXX)
  └── computed descriptors (MW, logP, TPSA, ...)
  └── may be a member of many CompoundLibraries

DockingResult
  └── (Compound × MolecularTarget) pose + score
  └── interaction fingerprint (ProLIF)
  └── belongs to a DockJob run

RefinedPose
  └── (Compound × MolecularTarget) pose after MD refinement
  └── MD trajectory object (frames, energy series)
  └── links back to the DockingResult it started from

ADMETProfile
  └── per-endpoint ADMET predictions for a Compound
  └── MPO score under one or more profiles
  └── links to the Compound and the job run

QCResult
  └── quantum chemistry calculation output (energy, charges, orbital data)
  └── links to the Compound or RefinedPose it was computed on

SAMSet (SAR Analog Matrix)
  └── a set of Compounds derived from a parent Compound by SAR expansion
  └── chemical lineage: each Compound records what transformation produced it
  └── becomes a new CompoundLibrary with backlinks to parent

SelectivityMap
  └── FEP/RBFE relative binding free energies across a target panel
  └── links to the RefinedPoses that seeded it

Report
  └── a snapshot of the workspace at a point in time
  └── embeds objects by reference (not by copy)
```

### Chemical Identity vs. Job Identity

These are separate and both matter.

**Job identity**: how an object was computed. "This ADMETProfile was produced by
admet-1777777452533097055 from libprep-XYZ." Standard provenance graph.

**Chemical identity**: what the object *is* chemically. "This Compound (KHM-04821) is the
same molecule regardless of which library it appears in, which docking run scored it, or
which SAR expansion it was derived from." Two DockingResults for KHM-04821 against two
different targets are linked not by job lineage but by the fact that they describe the
same molecule.

The controller tracks both. The UI surfaces chemical identity prominently — when a user
looks at a Compound, they see everything the platform knows about it across all jobs.

---

## Jobs as Transformations

A job is a pure function over objects:

```
job(input_objects, parameters) → output_objects
```

The controller handles:
- Scheduling the compute
- Linking output objects to input objects (provenance)
- Managing job lifecycle (Pending → Running → Succeeded/Failed)
- Making output objects available to the user's session

The user never needs to know what Kubernetes Job ran, what image was used, or what node it
landed on. Those details live in job metadata, accessible if wanted, but never surfaced as
primary information.

### Transformation Table

| Job Type | Input Objects | Output Objects | Chemical Relationship |
|---|---|---|---|
| TargetPrep | raw PDB/CIF file | MolecularTarget | same protein, prepared |
| LibraryPrep | SMILES list / SDF / DB query | CompoundLibrary | compounds assigned stable IDs |
| DockJob | MolecularTarget + CompoundLibrary | DockingResult[] | each Compound posed against Target |
| RefineJob | MolecularTarget + DockingResult[] | RefinedPose[] | same Compound, MD-refined |
| ADMETJob | CompoundLibrary | ADMETProfile[] | per-Compound property predictions |
| MDJob | MolecularTarget + RefinedPose | RefinedPose (+ trajectory) | same Compound, longer simulation |
| GenerateJob | Compound(s) + SAR context | SAMSet → CompoundLibrary | structurally related analogs |
| QCJob | Compound or RefinedPose | QCResult | same Compound, quantum descriptors |
| FEPJob | MolecularTarget + RefinedPose[] | SelectivityMap | same Compounds, rigorous ΔΔG |
| ReportJob | any objects | Report | snapshot of current workspace |

---

## Session and Workspace

### Session

A session is an authenticated user context. The controller ties job dispatch, object
visibility, and real-time status updates to the session. Jobs submitted in a session appear
in that session's activity feed. Objects created by those jobs belong to the session unless
explicitly shared.

Sessions are persistent across browser restarts. A user closes the tab and reopens it and
their objects and job statuses are still there.

### Workspace

A workspace is a named collection of objects grouped by scientific context — typically a
single drug discovery campaign (e.g., "EGFR-v2 inhibitor campaign Q2-2026"). A workspace
contains:

- One or more MolecularTargets
- One or more CompoundLibraries
- All DockingResults, RefinedPoses, ADMETProfiles produced within that campaign
- The job history for that campaign
- Notes and annotations

Workspaces are the unit of sharing. A user can invite collaborators to a workspace.
All objects in a workspace are visible to all workspace members.

A user can have multiple workspaces. The UI starts at the workspace level — choose a
workspace, then work within it.

---

## The Workflow as Object Graph

The intended use pattern is not a fixed pipeline. It is a **directed graph of object
transformations** where the user decides what questions to ask and in what order.

Typical flow for a new campaign:

```
[upload PDB]
     │
     ▼
MolecularTarget ──── TargetPrep ──── MolecularTarget (prepared)
                                              │
[paste SMILES / query ChEMBL]               │
     │                                       │
     ▼                                       │
CompoundLibrary ── LibraryPrep ── CompoundLibrary (prepared)
                                              │
                                              ▼
                                         DockJob
                                              │
                                    DockingResult[] ── user inspects ──►
                                              │                          │
                                              │                    [select top hits]
                                              ▼                          │
                                         RefineJob ◄─────────────────────┘
                                              │
                                    RefinedPose[] ──── MDJob ──── RefinedPose (trajectory)
                                              │
                              ┌───────────────┼───────────────┐
                              ▼               ▼               ▼
                          ADMETJob         QCJob           FEPJob
                              │               │               │
                       ADMETProfile[]    QCResult[]    SelectivityMap
                              │
                    [filter by MPO score]
                              │
                              ▼
                    CompoundLibrary (hits subset)
                              │
                          GenerateJob
                              │
                    SAMSet → new CompoundLibrary
                              │
                        [loop: dock analogs]
```

At every node the user can inspect objects, annotate them, branch the graph, or stop.
The graph is never forced — no stage auto-advances unless the user configures it to.

---

## User Interaction Principles

### 1. Start with the object, not the job

The user's first action is always "I have this data" — drop a PDB, paste SMILES, query a
database. The platform immediately creates an object and shows it. The user then chooses
what to do with it. The job submission form is secondary; the object is primary.

### 2. Jobs are invisible until they matter

A running job shows as a status indicator on the output object slot ("RefinedPose —
computing…"). The user does not need to navigate to a job queue. When the job finishes, the
object appears. If it fails, the object slot shows an error with context. Job details
(logs, resource usage, container image) are one click deeper, not on the primary surface.

### 3. Objects remember where they came from

Every object carries its full provenance chain. The user can always answer: "Where did this
come from?" and "What has been done with this?" The UI surfaces this as a lineage view on
any object — a compact DAG showing ancestors and descendants.

### 4. The same compound is the same compound

If KHM-04821 appears in three different libraries and has been docked twice against two
different targets, the Compound page for KHM-04821 shows all of that together. The user
never has to cross-reference manually.

### 5. Parameters are part of the object

A RefinedPose knows its force field, simulation length, and timestep. An ADMETProfile knows
which model version and MPO profile were used. These are stored on the object, not buried
in job logs.

### 6. The user steers, the controller drives

The user makes scientific decisions: which hits to refine, which analogs to generate, when
to stop expanding. The controller makes compute decisions: which node, how many parallel
jobs, retry policy, resource allocation. These responsibilities never cross.

---

## Implications for UI Redesign

The current UI is organized around job types (Docking, Calculations, ADMET). It should be
reorganized around object types (Targets, Libraries, Compounds, Results).

| Current (job-centric) | Target (object-centric) |
|---|---|
| "Submit docking job" | "Dock this library against this target" (action on selected objects) |
| "ADMET tab" | "Predict properties" (action on a selected CompoundLibrary or Compound) |
| "Results panel" | The DockingResult object, opened from the Compound or Library view |
| "Explorer panel" | Workspace browser — shows all objects, filterable by type and state |
| "Calculations panel" | "Run QC" action on a selected Compound or RefinedPose |

### Primary layout

```
┌─────────────────┬──────────────────────────────────────────────────────────┐
│ Workspace       │  Object Detail / Viewer                                  │
│ Browser         │                                                          │
│                 │  [Selected object fills this space]                      │
│  Targets (2)    │                                                          │
│  Libraries (4)  │  ┌──────────────────────────────────────────────────┐   │
│  Compounds (n)  │  │ KHM-04821                                        │   │
│  Results (12)   │  │ MW 342  logP 2.4  TPSA 87  Ro5 ✓               │   │
│  Jobs (running) │  │                                                  │   │
│                 │  │ Docked against: EGFR (2x), BRAF (1x)            │   │
│  ─────────────  │  │ Best pose: −9.4 kcal/mol (EGFR-prepared)        │   │
│  Activity feed  │  │ ADMET: MPO 72  hERG ✗  DILI ✓                  │   │
│  (live updates) │  │                                                  │   │
│                 │  │ [View 3D]  [Dock again]  [Generate analogs]      │   │
└─────────────────┴──────────────────────────────────────────────────────────┘
```

The 3D viewer, analysis panels, and trajectory player remain as they are — they attach to
the selected object rather than living in fixed tabs.

### Object actions

Every object has a context-sensitive action set based on its type and state:

- **MolecularTarget**: Run docking, Run MD, View structure, Edit binding site
- **CompoundLibrary**: Dock against target, Predict ADMET, Filter, Export, Generate analogs
- **Compound**: View all results, Predict ADMET, Run QC, View lineage, Compare poses
- **DockingResult**: Refine pose, Run MD, Inspect interactions, Flag for follow-up
- **RefinedPose**: Run MD (extend), Run FEP, Export structure, View trajectory
- **ADMETProfile**: View full profile, Compare to another compound, Export

Actions that require selecting a second object (e.g., "Dock *this library* against *this
target*") use a picker that shows compatible objects in the current workspace.

---

## Controller Responsibilities

The controller is the single source of truth for object state. It:

1. **Creates objects** when users upload data or when jobs produce output
2. **Assigns stable IDs** (chemical identity deduplication for Compounds)
3. **Dispatches jobs** when users trigger transformations
4. **Manages job lifecycle** (create, schedule, monitor, retry, cancel)
5. **Links objects** through both job provenance and chemical identity
6. **Enforces session scope** — objects belong to sessions and workspaces
7. **Streams status updates** to connected sessions (event bus → WebSocket)

The controller never makes scientific decisions. It does not decide which compounds to
refine or which parameters to use. Those come from the user or from explicit user-configured
automation rules (gate policies).

---

## Open Questions

1. **Object versioning**: If a user re-runs TargetPrep on the same PDB with different
   parameters, is the result a new MolecularTarget or a new version of the existing one?
   Recommend: new object with a `derivedFrom` pointer. Versioning is implicit in the lineage
   graph, not explicit in the ID.

2. **Compound deduplication**: Two uploads of the same canonical SMILES should produce the
   same KHM-XXXXX ID. When does deduplication happen — at LibraryPrep time or at upload?
   Recommend: at LibraryPrep time, by canonical SMILES comparison.

3. **Workspace vs. session scope**: Should objects be session-scoped (private until
   explicitly shared) or workspace-scoped (visible to all workspace members immediately)?
   Recommend: workspace-scoped. Sessions are just the active context, not a visibility
   boundary.

4. **Cross-workspace objects**: A MolecularTarget prepared in one campaign should be
   reusable in another without re-running TargetPrep. How is cross-workspace object
   reference handled? Recommend: objects can be "linked" into a workspace from another
   workspace, read-only. The source workspace retains ownership.

5. **Automation and gate policies**: Users may want "if DockJob finishes with ≥ 10 hits
   at affinity < −8 kcal/mol, automatically run RefineJob on the top 10." This implies
   a policy engine attached to gate transitions. Scope for v0.5 or later.
