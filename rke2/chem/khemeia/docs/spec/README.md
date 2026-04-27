# Khemeia SBDD Work Package Specifications

## Preamble

Khemeia is a Kubernetes-orchestrated framework for computational chemistry targeting
Structure-Based Drug Discovery (SBDD). It runs on a self-managed bare-metal RKE2 cluster
with open-source, CNCF-aligned tooling. Scientists work alongside the workflow, not
downstream of it.

The platform currently has a working docking prototype:
- **Go API** (`api/`) with a YAML-driven plugin system
- **SvelteKit frontend** (`web/`) with Svelte 5 runes
- **Molstar 3D viewer** (integrated but has known bugs)
- **AutoDock Vina** backend with parallel fan-out docking
- **ProLIF** sidecar for interaction fingerprinting
- **Result-writer** service for staging table drain
- **OIDC authentication** via Authentik
- **Flux GitOps** deployment

All new work must produce:
1. Containerized components (images pushed to `zot.hwcopeland.net/chem/`)
2. Declarative job definitions (CRD + plugin YAML where applicable)
3. Structured I/O schemas (JSON over REST, SQL for persistence)
4. Full provenance metadata (lineage from source through every transformation)
5. Tests (unit, integration, and at minimum one end-to-end smoke test per WP)

---

## Work Packages

| WP | Title | Owner | Priority | Status |
|----|-------|-------|----------|--------|
| [WP-1](wp-1-target-intake.md) | Target Intake and Binding-Site Definition | TBD | P1 (hot path) | Not started |
| [WP-2](wp-2-library-prep.md) | Library Intake and Preparation | TBD | P1 (hot path) | Not started |
| [WP-3](wp-3-docking-refinement.md) | Docking and Pose Refinement | TBD | P1 (hot path) | Not started |
| [WP-4](wp-4-admet.md) | ADMET Triage (Expanded) | TBD | P1 | Not started |
| [WP-5](wp-5-generative-sar.md) | Generative SAR Expansion | TBD | P2 | Not started |
| [WP-6](wp-6-selectivity-fep.md) | Selectivity and Free-Energy Refinement | TBD | P2 | Not started |
| [WP-7](wp-7-ui.md) | UI / Human-in-the-Loop | TBD | P0 (bug fixes) | Not started |
| [WP-8](wp-8-reporting.md) | Structure Ingestion and Reporting | TBD | P2 | Not started |
| [WP-9](wp-9-infrastructure.md) | Cross-Cutting Infrastructure | TBD | P1 (parallel) | Not started |

---

## Dependency Graph

```
                    WP-9 (Infrastructure)
                    =====================
                    Runs in parallel from
                    day one. All WPs depend
                    on provenance, object
                    store, event bus, and
                    CRD framework.
                           |
          +----------------+----------------+
          |                |                |
          v                v                v
       WP-1            WP-7 (P0)        WP-4
    Target Intake      UI Bug Fixes     ADMET
          |               |                |
          v               |                |
       WP-2              |           (schema contract
    Library Prep          |            with WP-7)
          |               |                |
          v               v                v
       WP-3 ------------> WP-7 ----------> WP-4
    Docking/Refine    Hit/Compound Tab   Inline ADMET
          |
          +--------+--------+
          |        |        |
          v        v        v
       WP-5    WP-6     WP-8
    Generative  FEP    Reporting
       SAR    Selectivity
          |
          v
       Loop back to WP-1
       (with provenance)
```

### Critical path (vertical slice)

```
WP-1 --> WP-2 --> WP-3
```

This is the hot path for delivering the first end-to-end SBDD run. WP-1 produces a prepared
receptor with binding site coordinates. WP-2 produces a filtered, 3D-conformer library with
stable IDs. WP-3 docks that library against that receptor and refines the top poses.

### Integration contracts that must be defined early

1. **WP-4 <-> WP-7**: ADMET result schema. WP-4 produces per-compound ADMET blocks; WP-7
   renders them inline in the Hit/Compound tab. Agree on the JSON schema before either WP
   begins implementation.
2. **WP-9 <-> all WPs**: Provenance metadata format, CRD base spec, event bus topic naming,
   and S3 bucket/prefix conventions.
3. **WP-3 <-> WP-6**: Pose format handoff. WP-6 RBFE/ABFE pipelines consume the refined
   poses produced by WP-3.

---

## Coordination Notes

1. **WP-7 UI bug fixes are HIGHEST PRIORITY.** The existing prototype has blocking bugs in
   the 3D viewer, zoom, interaction map, and receptor contacts scoring. These block user
   feedback on all other WPs. Fix them first.

2. **WP-9 infrastructure runs in parallel from day one.** Provenance, object store, event
   bus, and CRD framework are cross-cutting. Start immediately; other WPs consume these
   as they become available.

3. **WP-1, WP-2, WP-3 is the hot path** for the first vertical slice. Deliver target intake
   through docking results before branching into ADMET, generative, or FEP work.

4. **WP-4 and WP-7 have a tight integration contract.** ADMET is shown inline in the
   Hit/Compound tab, not in a separate report. Coordinate the ADMET result schema early
   so both teams build against the same interface.

5. **No OpenMM in any new code paths.** GROMACS is the MD and FEP engine for this platform.
   This applies to WP-3 (pose refinement MD), WP-6 (RBFE/ABFE), and any future MD work.

6. **Engine choices: pick, document rationale, move on.** Do not block on finding the perfect
   tool. Choose the best available, document why, and iterate later if needed.

---

## Existing Codebase Reference

Agents implementing these WPs should familiarize themselves with the current state:

| Component | Path | Notes |
|-----------|------|-------|
| Go API server | `api/` | Plugin system, auth, handlers, parallel docking |
| SvelteKit frontend | `web/` | Svelte 5 runes, Molstar integration |
| Plugin definitions | `plugins/` | YAML-driven backend declarations |
| Container images | `containers/` | autodock-vina, dfratom, nwchem, prolif-runner, psi4 |
| Result writer | `result-writer/` | Staging table drain service |
| K8s manifests | `deploy/` | Deployment, RBAC, services, job templates |
| Tests | `tests/` | API tests, Playwright UI tests |
| Benchmarks | `benchmark/` | Docking benchmark script |
| Architecture spec | `../compute-infrastructure/docs/spec/architecture.md` | Cluster topology, current pipeline |
| Security spec | `../compute-infrastructure/docs/spec/security.md` | Auth posture, known exposures |
