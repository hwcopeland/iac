# WP-7: UI / Human-in-the-Loop

## Owner

TBD

## Scope

Fix critical bugs in the existing SvelteKit + Molstar UI, then extend it with the Hit tab
(inline ADMET), stage progress visualization, gate/steering controls, provenance browsing, and
report generation triggers. Scientists interact with the SBDD pipeline through this UI -- it
is the primary feedback surface for all other work packages.

### Current state

The existing prototype (`web/`) is a SvelteKit application with Svelte 5 runes, backed by the
Go API (`api/`). Current capabilities:

- **Molstar 3D viewer**: integrated but has critical rendering bugs (see below)
- **Explorer panel**: structure browser, docking job listing
- **Analysis panel**: receptor-contacts aggregation (`handlers_analysis.go`), ProLIF
  interaction maps (`handlers_prolif.go`)
- **Calculations panel**: quantum chemistry job submission (NWChem, Psi4, QE, DFRATOM)
- **Results panel**: charts, 3D viewer, raw data tabs, trajectory player
- **Interaction network**: 2D network visualization (D3-based)
- **Auth**: OIDC via Authentik (`lib/auth.ts`)
- **Command palette**: keyboard-driven navigation

### Critical bugs (fix first)

These four bugs block user feedback on all other work packages. They are the highest priority
items in the entire specification.

**Bug 1: 3D viewer crashes on load.** The Molstar viewer initializes (`lib/viewer.ts`),
renders a single frame, then goes blank (white or black canvas). The `initViewer()` call
in `+page.svelte` defers to `requestAnimationFrame`, but the viewer container may not be
fully laid out at that point. Symptoms: brief flash of the structure, then an empty canvas.
No console error in production builds (error is swallowed).

**Bug 2: Zoom broken.** Mouse wheel zoom in the Molstar viewer produces erratic behavior
(jumps to extreme zoom levels or does nothing). This may be a Molstar canvas event handling
conflict with the SvelteKit layout. **Fix approach: disable zoom cleanly until the root
cause is identified.** Show a tooltip or status bar message ("Zoom temporarily disabled")
so the user knows it is intentional, not broken.

**Bug 3: Interaction map does not refresh on ligand change.** When the user selects a
different compound in the results list, the ProLIF interaction map (`InteractionNetwork.svelte`)
retains the previous compound's map. The reactive binding between the selected compound and
the interaction map fetch (`lib/api.ts` -> `handlers_prolif.go`) is not triggering on compound
change.

**Bug 4: Receptor-contacts scoring uses raw frequency instead of influence score.** The
current `AggregateResidueContact` struct in `handlers_analysis.go` reports `contact_frequency`
(fraction of top-N compounds that contact this residue). This metric over-weights residues
that are geometrically close to the binding site but make weak, non-specific contacts.

**Replace frequency with an INFLUENCE SCORE** that combines three factors:

| Factor | Weight | Description |
|--------|--------|-------------|
| Contact frequency | 0.4 | Fraction of top-N compounds contacting this residue (existing metric) |
| Affinity weighting | 0.35 | Weighted average where higher-affinity compounds contribute more. A residue contacted by a -10 kcal/mol hit contributes more than one contacted only by -6 kcal/mol hits. Normalize affinities to [0, 1] across the compound set. |
| Interaction classification | 0.25 | Classify each interaction as beneficial (H-bond, salt bridge, pi-stacking) or neutral (hydrophobic contact, van der Waals). Beneficial interactions score 1.0; neutral score 0.5. Average across all interactions for that residue. |

**Influence Score formula:**

```
influence = 0.4 * contact_freq
          + 0.35 * affinity_weighted_freq
          + 0.25 * beneficial_interaction_ratio
```

Where:
- `contact_freq` = compounds_contacting / total_compounds_analyzed (existing)
- `affinity_weighted_freq` = sum(normalized_affinity[i] for compounds contacting this
  residue) / sum(normalized_affinity[i] for all compounds)
- `beneficial_interaction_ratio` = count(beneficial interactions) / count(all interactions)
  for this residue. Beneficial = H-bond, ionic, pi-stacking. Neutral = hydrophobic, contact.

**Display both values in the UI.** Show the influence score as the primary sort/color metric.
Show raw contact frequency as a secondary column. Add a tooltip on the influence score column
header that explains the formula and the three contributing factors.

## Deliverables

### Phase 1: Bug fixes (P0)

1. **Fix 3D viewer crash** (Bug 1): Ensure the Molstar viewer container is fully laid out
   before initialization. Use a `ResizeObserver` or `MutationObserver` to confirm container
   dimensions are non-zero before calling `PluginContext.create()`. Add a loading spinner
   while the viewer initializes.

2. **Disable zoom cleanly** (Bug 2): Remove or intercept the mouse wheel event handler on
   the Molstar canvas. Display a status bar message: "Scroll zoom disabled -- use the
   toolbar zoom controls" (if toolbar zoom controls exist) or "Scroll zoom temporarily
   disabled" (if no alternative exists). Log the underlying issue as a tracked bug for
   future resolution.

3. **Fix interaction map reactivity** (Bug 3): Audit the reactive chain from compound
   selection (results list click) through `networkCompoundId` state to the
   `InteractionNetwork.svelte` component. Ensure the API call to
   `/api/v1/docking/interaction-map/{jobName}/{compoundId}` fires on every compound change.
   Verify with a test: select compound A, see map A; select compound B, see map B refresh.

4. **Implement influence score** (Bug 4): Add `influence_score` field to the
   `AggregateResidueContact` struct in `handlers_analysis.go`. Compute it using the formula
   above. Return both `contact_frequency` and `influence_score` in the API response. Update
   `AnalysisPanel.svelte` to sort and color-code by influence score. Add tooltip.

### Phase 2: Hit tab with inline ADMET

5. **Hit/Compound tab:**
   - New tab in the main panel (alongside Explorer, Analysis, Calculations)
   - Lists compounds from a completed docking job, sorted by consensus score or influence
   - Per-compound row: compound ID (`KHM-*`), SMILES (truncated), affinity, consensus rank,
     influence score, MPO score (from WP-4), expand button
   - Expanded view: full ADMET profile inline (rendered from the WP-4 per-compound JSON
     schema), including all endpoint values, confidence, model agreement indicators, and
     applicability domain warnings
   - ADMET traffic-light indicators: green (favorable), yellow (borderline), red (unfavorable)
     per endpoint
   - Compound detail panel: 2D structure rendering (RDKit depict or SMILES-to-SVG), all
     scores, full ADMET table, link to 3D viewer

### Phase 3: Stage progress and gates

6. **Pipeline stage progress visualization:**
   - Horizontal pipeline diagram showing all stages (WP-1 through WP-6) with status
     indicators per stage (not started, running, succeeded, failed)
   - Click a stage to see: job name, start time, duration, input/output summary, link to
     detailed results
   - Real-time status updates via polling (every 10s) or event bus subscription (WP-9)

7. **Gate and steering controls:**
   - At each stage boundary, show a gate: "Proceed to [next stage]?" with a summary of
     inputs from the completed stage
   - Steering actions: pause, resume, branch (create a new branch of the pipeline with
     modified parameters), and skip (advance past a stage)
   - Branch provenance: when a user branches, the new pipeline run carries a `branchedFrom`
     reference to the original run
   - Gate auto-advance: configurable per-stage. If auto-advance is ON, the next stage starts
     automatically when the gate conditions are met (e.g., "at least 10 hits with affinity
     < -7 kcal/mol"). If OFF, the user must manually approve.

### Phase 4: Provenance and reports

8. **Provenance browser:**
   - Given any compound or job, display the full provenance chain as a directed acyclic graph
     (DAG)
   - Nodes: jobs (TargetPrep, LibraryPrep, DockJob, RefineJob, ADMETJob, GenerateJob, etc.)
   - Edges: data flow (which job produced input for which downstream job)
   - Click a node to see job details, parameters, timestamps, and output artifacts
   - Navigate: "Show me everything that led to this compound" and "Show me everything
     downstream of this target prep"

9. **Report generation trigger:**
   - Button in the Hit tab and in the stage progress view: "Generate Report"
   - Triggers a WP-8 ReportJob with the current pipeline state
   - Shows report generation status and links to completed reports

### Tests

10. **Tests:**
    - Playwright E2E: load the app, verify the 3D viewer renders and is visible after 5
      seconds (Bug 1 regression test)
    - Playwright E2E: select two different compounds in sequence, verify the interaction map
      SVG changes (Bug 3 regression test)
    - API test: request `/api/v1/docking/receptor-contacts/{jobName}`, verify response
      includes both `contact_frequency` and `influence_score` fields, and that
      `influence_score` is a float between 0 and 1
    - Unit test (Go): given known contact data (affinities, interaction types, frequencies),
      verify influence score matches expected value
    - Component test: render the Hit tab with mock ADMET data, verify all endpoint values
      display and traffic-light colors match thresholds

## Acceptance Criteria

1. The 3D viewer renders a protein structure and remains visible (no blank canvas) for at
   least 60 seconds of idle time. Tested on Chrome and Firefox.
2. Mouse wheel scrolling over the 3D viewer does not cause erratic zoom. Either zoom is
   disabled with a visible notification, or it works smoothly.
3. Selecting compound A then compound B in the results list causes the interaction map to
   update both times. The second map must differ from the first (for different compounds).
4. The `/api/v1/docking/receptor-contacts/{jobName}` endpoint returns `influence_score` for
   each residue. For a test dataset where residue ASP-189 has high frequency + high affinity
   contacts + H-bond interactions, its influence score is higher than a residue with the same
   frequency but only hydrophobic contacts with low-affinity compounds.
5. The UI tooltip on the influence score column header displays the formula and explains all
   three factors. Both influence score and raw frequency are visible in the same view.
6. The Hit tab displays at least: compound ID, affinity, consensus rank, MPO score. Expanding
   a row shows the full ADMET profile with traffic-light indicators.
7. The ADMET inline display parses the WP-4 JSON schema without transformation -- the JSON
   fetched from the ADMET API is rendered directly.
8. The pipeline stage progress view shows the correct status for at least one completed stage
   (e.g., a finished DockJob shows "Succeeded" with timing data).
9. The provenance browser renders a DAG for a compound that has been through at least two
   pipeline stages (e.g., library prep -> docking).

## Dependencies

| Relationship | WP | Detail |
|---|---|---|
| Blocked by | None | Bug fixes can begin immediately on the existing codebase |
| Integrates with | WP-3 | Displays docking results, interaction maps, energy decomposition |
| Integrates with | WP-4 | Displays inline ADMET profiles (WP-4 JSON schema) |
| Integrates with | WP-6 | Displays selectivity heatmap and FEP results |
| Integrates with | WP-9 | Provenance system for the provenance browser; event bus for live status |
| Integrates with | WP-8 | Triggers report generation |
| Schema contract | WP-4 | Per-compound ADMET JSON schema must be agreed before Hit tab implementation |

## Out of Scope

- Mobile or tablet-optimized layout (desktop-first)
- Offline mode or progressive web app (PWA)
- Multi-user collaboration features (shared sessions, real-time co-editing)
- Internationalization (English only)
- Custom theme support beyond the existing dark theme
- Direct editing of pipeline parameters mid-run (steering allows pause/branch, not in-place
  parameter mutation)

## Open Questions

1. **Molstar version**: What version of Molstar is currently bundled? Is an upgrade feasible
   to resolve the viewer crash and zoom bugs, or would an upgrade introduce new regressions?

2. **Real-time updates**: Should the UI poll the API on an interval (simple, current
   approach) or subscribe to the WP-9 event bus (NATS/Redis Streams) via WebSocket for
   push-based updates? Polling is simpler but adds latency; push is responsive but requires
   WebSocket infrastructure.

3. **ADMET traffic-light thresholds**: Who defines the green/yellow/red thresholds for each
   ADMET endpoint? Options: (a) hard-coded sensible defaults from literature, (b) configurable
   per-project by the user, (c) derived from the MPO preset (WP-4). Likely a combination:
   defaults from literature, overridable per project.

4. **Provenance DAG scale**: For a compound that has been through multiple generative loops
   (WP-5), the provenance DAG can become large. Should the UI show the full DAG or a
   collapsed view with expand-on-demand?

5. **Accessibility**: Are there accessibility requirements (WCAG compliance level)? The
   traffic-light colors need colorblind-safe alternatives (pattern or icon in addition to
   color).

## Technical Constraints

- **SvelteKit + Svelte 5**: All new components use Svelte 5 runes (`$state`, `$derived`,
  `$effect`). Do not introduce Svelte 4 patterns (stores, reactive declarations).
- **Go API backend**: All new endpoints follow the existing pattern in `api/handlers*.go`.
  Use the plugin system for new route registration where applicable.
- **Molstar**: The 3D viewer is Molstar (`web/src/lib/viewer.ts`). Bug fixes must work within
  the existing Molstar integration. If Molstar is upgraded, regression-test all existing 3D
  features (structure loading, representation changes, surface coloring, selection info).
- **Auth**: All new endpoints and UI routes must respect the existing OIDC auth flow
  (`lib/auth.ts`, `api/auth.go`). No anonymous access.
- **Existing components**: The current component tree is: `+page.svelte` -> `Toolbar`,
  `ExplorerPanel`, `AnalysisPanel`, `CalculationsPanel`, `StructureBrowser`,
  `InteractionNetwork`, `SelectionInfo`, `StatusBar`, `CommandPalette`, `Toast`, plus the
  `results/` sub-components (`ResultsPanel`, `ChartsTab`, `ThreeDTab`, `SummaryTab`,
  `RawTab`, `TrajectoryPlayer`). New tabs and panels must integrate with this structure.
- **API contract**: The influence score field is additive to the existing
  `AggregateResidueContact` struct. Do not remove `contact_frequency` -- both fields must
  be present.
