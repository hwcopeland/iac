# WP-8: Structure Ingestion and Reporting

## Owner

TBD

## Scope

Build two capabilities: (1) structure re-entry from experimental results (co-crystal
structures, cryo-EM maps) that feed back into the computational pipeline, and (2) a
report generation system that produces audience-appropriate HTML and PDF reports from any
pipeline stage. Together, these close the loop between computational predictions and
experimental validation.

### Current state

No structure ingestion or reporting capability exists in the platform. Experimental structures
cannot be fed back into the pipeline after docking, and there is no way to generate summary
reports from pipeline results. All output is currently viewed through the UI (WP-7) or
queried via the API.

## Deliverables

### Structure re-entry

1. **IngestStructureJob CRD** (apiVersion: `khemeia.io/v1alpha1`, kind: `IngestStructureJob`)
   - Spec: `source` (enum: `co-crystal`, `cryo-em`, `user-upload`), `pdbId` (for published
     structures), `uploadRef` (S3 key for user uploads), `resolution` (angstroms),
     `ligandId` (residue name of the bound ligand in the experimental structure),
     `targetRef` (optional: reference to existing TargetPrep to link provenance)
   - Status: `phase`, `refinedStructure` (S3 key), `ligandPose` (S3 key),
     `rmsdToComputational` (if linked to an existing docked pose), `startTime`,
     `completionTime`, `provenance`

2. **Structure re-entry pipeline:**
   - Input: PDB file (co-crystal) or density map (cryo-EM MRC/CCP4 format)
   - For co-crystal structures:
     a. Download from RCSB (by PDB ID) or accept user upload
     b. Extract the bound ligand coordinates
     c. Run through the WP-1 receptor preparation pipeline (PDBFixer cleaning)
     d. Compute RMSD between the experimental ligand pose and the best computational pose
        (if a DockJob exists for this target-ligand pair)
     e. Output: prepared receptor, experimental ligand pose (SDF), RMSD comparison
   - For cryo-EM structures:
     a. Accept the deposited model (PDB) and optionally the density map (MRC)
     b. Note: cryo-EM fitting and refinement are out of scope. Accept the deposited model
        as-is.
     c. Run through the same receptor prep and ligand extraction as co-crystal
   - Provenance: link the experimental structure to the computational predictions it
     validates (or contradicts)

3. **API endpoints for structure ingestion:**
   - `POST /api/v1/structures/ingest` -- submit a structure ingestion job
   - `GET /api/v1/structures/jobs/{name}` -- get IngestStructureJob status
   - `GET /api/v1/structures/jobs/{name}/comparison` -- get RMSD comparison between
     experimental and computational poses

### Reporting

4. **ReportJob CRD** (apiVersion: `khemeia.io/v1alpha1`, kind: `ReportJob`)
   - Spec: `template` (enum: `internal`, `collaborator`, `regulatory`), `pipelineRef`
     (reference to any pipeline job -- the report scope is determined by the referenced
     job and all its upstream provenance), `sections` (optional array of section names to
     include; default: all sections for the template), `format` (enum: `html`, `pdf`,
     default: `html`), `title`, `author`
   - Status: `phase`, `reportRef` (S3 key to generated report), `pageCount`,
     `startTime`, `completionTime`, `provenance`

5. **Report generation container** (`zot.hwcopeland.net/chem/report-gen:latest`)
   - **HTML-first**: Generate reports as self-contained HTML (inline CSS, embedded images
     as base64 or inline SVG). HTML reports are the primary deliverable -- they open in any
     browser, are searchable, and are easy to share.
   - **PDF export**: Convert HTML to PDF via WeasyPrint (preferred, pure-Python) or
     Playwright (fallback, heavier but handles complex CSS). PDF is a secondary format for
     audiences that require it (collaborators, regulatory).
   - Template engine: Jinja2 (Python) rendering HTML templates
   - Embedded visualizations: 2D structure depictions (RDKit SVG), score charts (matplotlib
     SVG), interaction diagrams (ProLIF SVG from WP-3 data)
   - Self-contained: no external asset dependencies. Reports must render correctly when
     disconnected from the Khemeia server.

6. **Report templates per audience:**

   | Template | Audience | Content Focus |
   |----------|----------|---------------|
   | `internal` | The scientist running the pipeline | Full technical detail: all scores, all ADMET endpoints, interaction maps, per-residue energies, provenance chain, raw data tables. Dense, comprehensive. |
   | `collaborator` | External collaborators, medicinal chemists | Executive summary, top hits with ADMET traffic-lights, selectivity summary, SAR analysis highlights, recommended next steps. No raw data tables, no provenance internals. |
   | `regulatory` | Regulatory submissions (IND-enabling) | Structured sections: computational methods description, validation against benchmarks, compound characterization, ADMET profiling with model versions and confidence intervals, data provenance for reproducibility. Formal tone. |

7. **Report sections** (selected per template):

   | Section | Internal | Collaborator | Regulatory |
   |---------|----------|-------------|------------|
   | Executive summary | Yes | Yes | Yes |
   | Target preparation details | Yes | No | Yes |
   | Library composition and filters | Yes | Brief | Yes |
   | Docking results (top N) | Yes (full) | Yes (top 10) | Yes (top 20) |
   | Pose refinement and MM-GBSA | Yes | Summary | Yes |
   | ADMET profiles | Yes (all endpoints) | Yes (traffic-lights) | Yes (all + confidence) |
   | Selectivity panel results | Yes (heatmap) | Yes (summary) | Yes (full) |
   | FEP results | Yes | If available | Yes |
   | Generative expansion summary | Yes | Brief | Yes |
   | Provenance chain | Yes (full DAG) | No | Yes (methods only) |
   | Benchmark validation | If available | No | Yes (required) |
   | Appendix: raw data | Yes | No | Selected |

8. **API endpoints for reporting:**
   - `POST /api/v1/reports/generate` -- submit a report generation job
   - `GET /api/v1/reports/jobs/{name}` -- get ReportJob status
   - `GET /api/v1/reports/jobs/{name}/download` -- download the generated report (HTML or PDF)
   - `GET /api/v1/reports/templates` -- list available report templates with descriptions

### Tests

9. **Tests:**
   - Unit tests for structure ingestion: given a known PDB file (e.g., 7jrn), extract bound
     ligand, verify ligand coordinates match expected centroid
   - Unit tests for RMSD calculation between two ligand poses
   - Unit tests for report rendering: given a mock dataset, render each template, verify
     HTML output contains expected sections
   - Integration test: ingest PDB 7jrn via API, verify receptor prep output matches WP-1
     output for the same PDB
   - Integration test: generate an `internal` report for a completed DockJob, verify the
     HTML file is non-empty and contains the expected section headings
   - E2E smoke test: submit a ReportJob via API, poll until Succeeded, download the HTML
     report, verify it opens in a browser (returns valid HTML)

## Acceptance Criteria

1. Ingesting PDB ID `7jrn` via `POST /api/v1/structures/ingest` extracts the bound ligand
   (TTT), produces a cleaned receptor, and stores both in S3 with provenance.
2. If a DockJob has previously been run against 7jrn, the IngestStructureJob status includes
   `rmsdToComputational` with a value in angstroms comparing the co-crystal ligand pose to
   the best docked pose.
3. The `internal` report template renders an HTML file containing all sections listed in the
   section matrix above. Each section heading is present as an HTML element (verifiable by
   parsing).
4. The `collaborator` report template omits raw data tables, provenance internals, and
   per-residue energy decompositions. It includes an executive summary and top-10 hit list.
5. PDF export produces a valid PDF file (parseable by a PDF reader) with the same content
   as the HTML version. Page count is included in the ReportJob status.
6. Reports are self-contained: the HTML file renders correctly when opened from the local
   filesystem with no network access. All images are inlined (base64 or SVG).
7. The provenance chain in the `internal` report traces from the reported compound back
   through all pipeline stages that produced it, including job names and timestamps.
8. Report generation for a 50-compound DockJob with ADMET data completes within 5 minutes.

## Dependencies

| Relationship | WP | Detail |
|---|---|---|
| Blocked by | WP-1 | Structure ingestion uses the receptor preparation pipeline |
| Blocked by | WP-9 | S3 storage for reports and ingested structures, CRD framework |
| Consumes | WP-3 | Report includes docking results and refinement data |
| Consumes | WP-4 | Report includes ADMET profiles |
| Consumes | WP-6 | Report includes selectivity and FEP results (if available) |
| Consumes | WP-5 | Report includes generative SAR summary (if available) |
| Integrates with | WP-7 | UI triggers report generation via button; displays download links |

## Out of Scope

- Cryo-EM map fitting or model refinement (accept deposited models only)
- Interactive reports (reports are static HTML/PDF, not dashboards)
- Report scheduling (automatic periodic reports; reports are generated on-demand)
- Multi-language report generation
- LaTeX/Word export formats
- Custom branding or logo injection (use a consistent Khemeia header)
- SBML or COMBINE archive export

## Open Questions

1. **WeasyPrint vs Playwright for PDF**: WeasyPrint is pure Python, lighter, and easier to
   containerize, but has limited CSS support (no CSS Grid, limited Flexbox). Playwright
   produces pixel-perfect PDFs via headless Chromium but adds ~400 MB to the container image.
   Which is acceptable given the cluster's image storage constraints?

2. **Regulatory template scope**: What specific regulatory framework does the `regulatory`
   template target? FDA IND? EMA? ICH M7 (mutagenicity)? Each has different expectations
   for computational chemistry documentation. Start with a generic "computational methods"
   template and refine per regulatory feedback?

3. **Cryo-EM density map handling**: Should the pipeline store the density map (potentially
   hundreds of MB to GB) or just a reference to EMDB? Storing locally enables future
   visualization; referencing externally saves storage.

4. **Report versioning**: If the same pipeline is re-reported after additional data (e.g.,
   more compounds docked), should the new report supersede the old one, or should both be
   retained with version numbers?

5. **Chart styling**: Should report charts match the UI dark theme, or use a print-friendly
   light theme? Different audiences may have different preferences.

## Technical Constraints

- **Container base**: `python:3.11-slim` with RDKit (for 2D depictions), Jinja2, matplotlib,
  and WeasyPrint (or Playwright). Multi-stage build to minimize image size.
- **Self-contained HTML**: All assets (CSS, images, fonts) must be inlined. No CDN
  references. Use base64 encoding for raster images and inline SVG for vector graphics.
- **S3 storage**: Reports are stored as S3 objects (WP-9 MinIO). Key format:
  `reports/{job-name}/{report-name}.{html|pdf}`. Set appropriate content-type headers
  for direct browser viewing.
- **Template location**: Jinja2 templates stored in the report-gen container at
  `/templates/{internal,collaborator,regulatory}/`. Mounted from a ConfigMap if templates
  need to be updated without rebuilding the container.
- **RMSD calculation**: Use RDKit `AllChem.AlignMol()` for ligand pose RMSD. Both poses must
  be in the same coordinate frame (same receptor). Handle symmetry-corrected RMSD for
  symmetric molecules.
- **Provenance traversal**: The report generator queries the provenance system (WP-9) to
  build the full pipeline chain for a given job. The provenance API must support
  "ancestors-of" queries.
