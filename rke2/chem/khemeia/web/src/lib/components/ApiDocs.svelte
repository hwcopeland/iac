<script lang="ts">
  let { open = $bindable(false) }: { open?: boolean } = $props();

  function close() { open = false; }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') close();
  }

  type Param = { name: string; type: string; required?: boolean; desc: string };
  type Endpoint = {
    method: 'GET' | 'POST' | 'DELETE';
    path: string;
    desc: string;
    body?: Param[];
    query?: Param[];
    returns?: string;
  };
  type Section = { title: string; tag: string; endpoints: Endpoint[] };

  const sections: Section[] = [
    {
      title: 'Target Preparation',
      tag: 'WP-1',
      endpoints: [
        {
          method: 'POST', path: '/api/v1/targets/prepare',
          desc: 'Submit a target preparation job. Downloads the PDB, cleans the structure, adds hydrogens at the specified pH, and defines the binding site.',
          body: [
            { name: 'pdb_id', type: 'string', required: true, desc: 'RCSB PDB accession code (e.g. "1AKE")' },
            { name: 'binding_site_mode', type: 'enum', required: true, desc: '"native-ligand" | "custom-box" | "pocket-detection"' },
            { name: 'native_ligand_id', type: 'string', desc: 'Three-letter residue code of the co-crystallised ligand. Required for native-ligand mode.' },
            { name: 'custom_box', type: 'object', desc: '{ center: [x,y,z], size: [x,y,z] } in Å. Required for custom-box mode.' },
            { name: 'padding', type: 'float', desc: 'Box padding in Å around the native ligand (default 10.0).' },
            { name: 'ph', type: 'float', desc: 'pH for protonation state assignment (default 7.4).' },
            { name: 'keep_cofactors', type: 'string[]', desc: 'Residue codes to retain in the receptor (default ["ZN","MG","CA","FE"]).' },
          ],
          returns: '202 { name, status: "Pending" }',
        },
        {
          method: 'GET', path: '/api/v1/targets/{name}',
          desc: 'Poll the status of a target prep job. Returns phase, binding site geometry, and detected pockets when complete.',
          returns: '{ name, phase, pdb_id, binding_site_mode, binding_site: { center, size }, pockets: [...], selected_pocket, error_output }',
        },
        {
          method: 'GET', path: '/api/v1/targets/{name}/receptor',
          desc: 'Download the cleaned receptor PDB file ready for docking.',
          returns: 'text/plain — PDB coordinates',
        },
        {
          method: 'GET', path: '/api/v1/targets/{name}/pockets',
          desc: 'List fpocket/P2Rank detected pockets with consensus scores and centres.',
          returns: '{ pockets: [{ rank, center, size, consensus_score, fpocket_score, p2rank_score }] }',
        },
        {
          method: 'POST', path: '/api/v1/targets/{name}/pockets/{index}/select',
          desc: 'Select a detected pocket to use as the docking box for downstream jobs.',
          returns: '200 { selected_pocket: index }',
        },
      ],
    },
    {
      title: 'Library Preparation',
      tag: 'WP-2',
      endpoints: [
        {
          method: 'POST', path: '/api/v1/libraries/prepare',
          desc: 'Submit a compound library prep job. Standardises SMILES, applies drug-likeness filters, generates 3D conformers, and produces docking-ready PDBQT files.',
          body: [
            { name: 'source', type: 'enum', required: true, desc: '"smiles" | "chembl" | "sdf" | "enamine"' },
            { name: 'name', type: 'string', desc: 'Human-readable library name.' },
            { name: 'smiles_list', type: 'string[]', desc: 'One SMILES string per compound. Used when source="smiles".' },
            { name: 'chembl', type: 'object', desc: 'ChEMBL filter params: { q, mw_min, mw_max, logp_min, logp_max, hba_max, hbd_max, max_phase, ro5 }. Used when source="chembl".' },
            { name: 'filters', type: 'object', desc: '{ lipinski, veber, pains, brenk, reos } — boolean flags. Defaults: lipinski/veber/pains=true, brenk/reos=false.' },
          ],
          returns: '202 { name, status: "Pending" }',
        },
        {
          method: 'GET', path: '/api/v1/libraries/{name}',
          desc: 'Poll library prep status.',
          returns: '{ name, phase, compound_count, filtered_count, source, error_output }',
        },
        {
          method: 'GET', path: '/api/v1/libraries/{name}/compounds',
          desc: 'Paginated list of prepared compounds with properties.',
          query: [
            { name: 'page', type: 'int', desc: 'Page number (default 1).' },
            { name: 'per_page', type: 'int', desc: 'Results per page (default 25, max 500).' },
          ],
          returns: '{ total, page, per_page, compounds: [{ compound_id, smiles, mw, logp, hba, hbd, inchi_key }] }',
        },
      ],
    },
    {
      title: 'Molecular Docking',
      tag: 'WP-3',
      endpoints: [
        {
          method: 'POST', path: '/api/v1/docking/v2/submit',
          desc: 'Submit a multi-engine docking campaign. Spawns one K8s Job per engine; results are consensus-ranked across engines.',
          body: [
            { name: 'receptor_ref', type: 'string', required: true, desc: 'Target prep job name.' },
            { name: 'library_ref', type: 'string', required: true, desc: 'Library prep job name.' },
            { name: 'engines', type: 'string[]', required: true, desc: '"vina-1.2" | "gnina" | "vina-gpu" | "diffdock"' },
            { name: 'exhaustiveness', type: 'int', desc: 'Vina exhaustiveness (default 32).' },
            { name: 'consensus', type: 'bool', desc: 'Compute cross-engine consensus scores (default true).' },
            { name: 'top_n_refine', type: 'int', desc: 'Top-N poses to carry forward for downstream refinement.' },
          ],
          returns: '202 { name, status, engines }',
        },
        {
          method: 'GET', path: '/api/v1/docking/v2/jobs',
          desc: 'List recent docking jobs.',
          returns: '{ jobs: [{ name, status, engines, created_at }] }',
        },
        {
          method: 'GET', path: '/api/v1/docking/v2/jobs/{name}',
          desc: 'Per-engine progress and aggregate stats for a docking job.',
          returns: '{ name, status, engines, engine_progress: [{ engine, status, result_count, best_affinity }], total_ligands, docked_ligands, best_affinity, consensus_ready }',
        },
        {
          method: 'GET', path: '/api/v1/docking/v2/jobs/{name}/summary',
          desc: 'Affinity distribution histogram and top-hit cutoff counts.',
          returns: '{ unique_compounds, best_affinity, cutoff_counts: { "-7.0": N, ... } }',
        },
        {
          method: 'GET', path: '/api/v1/docking/v2/jobs/{name}/results',
          desc: 'Paginated consensus-ranked docking results.',
          query: [
            { name: 'page', type: 'int', desc: 'Page number (default 1).' },
            { name: 'per_page', type: 'int', desc: 'Results per page (default 25).' },
            { name: 'min_affinity', type: 'float', desc: 'Filter by minimum affinity (kcal/mol).' },
          ],
          returns: '{ total_results, results: [{ compound_id, smiles, consensus_rank, consensus_score, per_engine: [{ engine, raw_score, pose_rank }] }] }',
        },
        {
          method: 'GET', path: '/api/v1/docking/v2/jobs/{name}/results/{compoundId}/pose',
          desc: 'Download the best-scoring docked pose as PDBQT.',
          returns: 'text/plain — PDBQT file',
        },
        {
          method: 'GET', path: '/api/v1/docking/pocket/{jobName}/{compoundId}',
          desc: 'Pocket residue contacts and interaction types for a docked compound.',
          query: [{ name: 'cutoff', type: 'float', desc: 'Contact distance cutoff in Å (default 5.0).' }],
          returns: '{ compound_id, pocket_residues: [{ chain_id, res_id, res_name, interaction_types }] }',
        },
        {
          method: 'GET', path: '/api/v1/docking/analysis/receptor-contacts/{jobName}',
          desc: 'Aggregate contact frequency across all docked poses for a job.',
          query: [{ name: 'top', type: 'int', desc: 'Return the top-N most contacted residues (default 50).' }],
          returns: '{ residues: [{ chain_id, res_id, res_name, contact_count, interaction_types }] }',
        },
        {
          method: 'GET', path: '/api/v1/docking/analysis/fingerprints/{jobName}',
          desc: 'Interaction fingerprints for top-scoring poses.',
          query: [{ name: 'top', type: 'int', desc: 'Number of top poses to fingerprint (default 100).' }],
          returns: '{ fingerprints: [{ compound_id, vector: number[] }] }',
        },
      ],
    },
    {
      title: 'ADMET Prediction',
      tag: 'WP-4',
      endpoints: [
        {
          method: 'POST', path: '/api/v1/admet/predict',
          desc: 'Submit an ADMET prediction job against a compound library using Stanford Chemprop-D models.',
          body: [
            { name: 'library_ref', type: 'string', required: true, desc: 'Library prep job name.' },
            { name: 'mpo_profile', type: 'enum', desc: '"oral" | "cns" | "oncology" | "antimicrobial" (default "oral").' },
            { name: 'compound_refs', type: 'string[]', desc: 'Subset of compound IDs to score. Omit to score the entire library.' },
          ],
          returns: '202 { name, status }',
        },
        {
          method: 'GET', path: '/api/v1/admet/jobs/{name}',
          desc: 'Poll ADMET job status.',
          returns: '{ name, phase, library_ref, mpo_profile, total_compounds, predicted_count, failed_count, avg_mpo_score }',
        },
        {
          method: 'GET', path: '/api/v1/admet/jobs/{name}/results',
          desc: 'Paginated per-compound ADMET results sorted by MPO score.',
          query: [
            { name: 'page', type: 'int', desc: 'Page number.' },
            { name: 'per_page', type: 'int', desc: 'Results per page.' },
          ],
          returns: '{ total, results: [{ compound_id, smiles, mpo_score, mpo_profile, endpoints: {...}, flags }] }',
        },
        {
          method: 'GET', path: '/api/v1/admet/compound/{compoundId}',
          desc: 'Full ADMET endpoint breakdown for a single compound.',
          returns: '{ compound_id, smiles, mpo_score, endpoints: { hia, bbb, cyp_inhibition, ... }, flags }',
        },
        {
          method: 'GET', path: '/api/v1/admet/presets',
          desc: 'List available MPO profiles and their endpoint weights.',
          returns: '{ presets: [{ name, endpoints: { name: weight } }] }',
        },
      ],
    },
    {
      title: 'MD Simulation',
      tag: 'WP-5',
      endpoints: [
        {
          method: 'POST', path: '/api/v1/md/submit',
          desc: 'Submit a GROMACS MD simulation for the top-N docking hits. Runs full EM→NVT→NPT→production MD per compound on the GPU node.',
          body: [
            { name: 'dock_job_name', type: 'string', required: true, desc: 'Docking job to pull top hits from.' },
            { name: 'receptor_ref', type: 'string', required: true, desc: 'Target prep job name.' },
            { name: 'top_n', type: 'int', desc: 'Number of top docking hits to simulate (default 5).' },
            { name: 'affinity_cutoff', type: 'float', desc: 'Only simulate compounds with affinity ≤ this value in kcal/mol.' },
            { name: 'md_nsteps', type: 'int', desc: 'Production MD steps (default 500000 = 1 ns at 2 fs/step).' },
            { name: 'force_field', type: 'enum', desc: '"amber99sb-ildn" | "amber14sb" | "charmm36m" (default "amber99sb-ildn").' },
            { name: 'ligand_ff', type: 'enum', desc: '"gaff2" | "gaff" (default "gaff2").' },
            { name: 'use_resp', type: 'bool', desc: 'Use HF/6-31G* RESP charges for ligand (slower, more accurate).' },
          ],
          returns: '202 { name, status }',
        },
        {
          method: 'GET', path: '/api/v1/md/jobs/{name}',
          desc: 'Per-compound MD progress and phase information.',
          returns: '{ name, status, compounds: [{ compound_id, status, dock_affinity_kcal_mol, duration_s }], progress: { phase, step, total_steps } }',
        },
        {
          method: 'GET', path: '/api/v1/md/jobs/{name}/results',
          desc: 'Completed compound results with trajectory availability flags.',
          returns: '{ results: [{ compound_id, dock_affinity_kcal_mol, duration_s, has_trajectory, has_energy }] }',
        },
        {
          method: 'GET', path: '/api/v1/md/jobs/{name}/trajectory/{compoundId}',
          desc: 'Stream multi-model PDB trajectory frames from S3.',
          returns: 'text/plain — multi-MODEL PDB file',
        },
        {
          method: 'GET', path: '/api/v1/md/jobs/{name}/energy/{compoundId}',
          desc: 'Energy timeseries data (potential energy, temperature).',
          returns: '{ time: number[], potential: number[], temperature: number[] }',
        },
      ],
    },
    {
      title: 'Ligand Databases',
      tag: '',
      endpoints: [
        {
          method: 'GET', path: '/api/v1/ligand-databases',
          desc: 'List all available ligand databases and their compound counts.',
          returns: '{ databases: [{ name, compound_count }] }',
        },
        {
          method: 'GET', path: '/api/v1/ligands/search',
          desc: 'Search ligands by SMILES substructure or name across all databases.',
          query: [
            { name: 'q', type: 'string', required: true, desc: 'SMILES or name query.' },
            { name: 'db', type: 'string', desc: 'Restrict to a specific database.' },
            { name: 'limit', type: 'int', desc: 'Max results (default 50).' },
          ],
          returns: '{ results: [{ compound_id, smiles, name, mw }] }',
        },
        {
          method: 'POST', path: '/api/v1/ligands/import-from-chembl',
          desc: 'Import compounds from the local ChEMBL database into a named ligand database.',
          body: [
            { name: 'db_name', type: 'string', required: true, desc: 'Target database name (created if absent).' },
            { name: 'q', type: 'string', desc: 'ChEMBL target ID or free-text query.' },
            { name: 'mw_min', type: 'float', desc: 'Molecular weight lower bound.' },
            { name: 'mw_max', type: 'float', desc: 'Molecular weight upper bound.' },
            { name: 'max_phase', type: 'int', desc: 'Minimum clinical phase (1–4).' },
          ],
          returns: '202 { db_name, imported_count }',
        },
      ],
    },
    {
      title: 'QC Plugins',
      tag: '',
      endpoints: [
        {
          method: 'GET', path: '/api/v1/plugins',
          desc: 'List installed QC calculation plugins (Psi4, NWChem, Quantum ESPRESSO, DFratom, AutoDock Vina legacy).',
          returns: '{ plugins: [{ name, slug, input: [{ name, type, required, default, description, enum, max }], output: [...] }] }',
        },
        {
          method: 'POST', path: '/api/v1/{slug}/submit',
          desc: 'Submit a plugin job. The request body is a flat key-value map matching the plugin\'s input field schema.',
          returns: '202 { name }',
        },
        {
          method: 'GET', path: '/api/v1/{slug}/jobs',
          desc: 'List all jobs for this plugin.',
          returns: '{ jobs: [{ name, status, created_at }] }',
        },
        {
          method: 'GET', path: '/api/v1/{slug}/jobs/{name}',
          desc: 'Get job status and output.',
          returns: '{ name, status, input, output, created_at, started_at, completed_at }',
        },
        {
          method: 'DELETE', path: '/api/v1/{slug}/jobs/{name}',
          desc: 'Delete a job record and its artifacts.',
          returns: '204 No Content',
        },
        {
          method: 'GET', path: '/api/v1/{slug}/artifacts/{jobName}/{file}',
          desc: 'Download an output artifact by filename.',
          returns: 'application/octet-stream',
        },
      ],
    },
    {
      title: 'Provenance',
      tag: '',
      endpoints: [
        {
          method: 'POST', path: '/api/v1/provenance/record',
          desc: 'Record a provenance relationship between pipeline artifacts.',
          body: [
            { name: 'artifact_id', type: 'string', required: true, desc: 'S3 key or logical artifact identifier.' },
            { name: 'artifact_type', type: 'string', required: true, desc: 'Type label (e.g. "receptor", "library", "pose").' },
            { name: 'job_name', type: 'string', required: true, desc: 'Producing job name.' },
            { name: 'parent_ids', type: 'string[]', desc: 'Parent artifact IDs this record was derived from.' },
          ],
          returns: '201 { id }',
        },
        {
          method: 'GET', path: '/api/v1/provenance/job/{name}',
          desc: 'List all provenance records for a given job.',
          returns: '{ records: [{ id, artifact_id, artifact_type, job_name, created_at }] }',
        },
        {
          method: 'GET', path: '/api/v1/provenance/{id}/ancestors',
          desc: 'Full ancestor chain (recursive CTE) for a provenance record.',
          returns: '{ ancestors: [{ id, artifact_id, artifact_type, job_name, depth }] }',
        },
        {
          method: 'GET', path: '/api/v1/provenance/{id}/descendants',
          desc: 'Full descendant chain for a provenance record.',
          returns: '{ descendants: [...] }',
        },
      ],
    },
    {
      title: 'Tokens & Health',
      tag: '',
      endpoints: [
        {
          method: 'POST', path: '/api/v1/tokens',
          desc: 'Create a long-lived API token (scoped to the authenticated user).',
          body: [{ name: 'name', type: 'string', required: true, desc: 'Human-readable token label.' }],
          returns: '201 { id, token, name, created_at } — token value shown only once.',
        },
        {
          method: 'GET', path: '/api/v1/tokens',
          desc: 'List your API tokens (values are not returned after creation).',
          returns: '{ tokens: [{ id, name, created_at, last_used_at }] }',
        },
        {
          method: 'DELETE', path: '/api/v1/tokens/{id}',
          desc: 'Revoke a token immediately.',
          returns: '204 No Content',
        },
        {
          method: 'GET', path: '/health',
          desc: 'Liveness probe — always returns 200 if the process is running.',
          returns: '200 { status: "ok" }',
        },
        {
          method: 'GET', path: '/readyz',
          desc: 'Readiness probe — returns 200 only when the database connection is healthy.',
          returns: '200 { status: "ready" }',
        },
      ],
    },
  ];

  const methodColor: Record<string, string> = {
    GET: '#3fb950',
    POST: '#58a6ff',
    DELETE: '#f85149',
  };
</script>

{#if open}
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="overlay" role="dialog" aria-modal="true" aria-label="API Documentation" onkeydown={handleKeydown}>
  <div class="backdrop" onclick={close}></div>
  <div class="panel">
    <div class="panel-header">
      <div class="header-left">
        <span class="panel-title">Khemeia API</span>
        <span class="panel-sub">v1 · Base URL: <code>/api/v1</code> · Auth: <code>Authorization: Bearer &lt;token&gt;</code></span>
      </div>
      <button class="close-btn" onclick={close} aria-label="Close">✕</button>
    </div>

    <div class="panel-body">
      <div class="intro">
        All endpoints require a JWT bearer token obtained via the OIDC flow (<code>/auth/callback</code>)
        or a long-lived API token created via <code>POST /api/v1/tokens</code>.
        Async jobs return <strong>202 Accepted</strong> immediately; poll the status endpoint until
        <code>phase</code> is <code>"succeeded"</code> or <code>"failed"</code>.
      </div>

      {#each sections as section}
        <div class="section">
          <div class="section-heading">
            <span class="section-title">{section.title}</span>
            {#if section.tag}<span class="section-tag">{section.tag}</span>{/if}
          </div>

          {#each section.endpoints as ep}
            <div class="endpoint">
              <div class="ep-line">
                <span class="method" style="color:{methodColor[ep.method]}">{ep.method}</span>
                <code class="path">{ep.path}</code>
              </div>
              <p class="ep-desc">{ep.desc}</p>

              {#if ep.body && ep.body.length > 0}
                <div class="params-block">
                  <span class="params-label">Request body (JSON)</span>
                  <table class="params-table">
                    {#each ep.body as p}
                      <tr>
                        <td class="p-name"><code>{p.name}</code>{#if p.required}<span class="req">*</span>{/if}</td>
                        <td class="p-type">{p.type}</td>
                        <td class="p-desc">{p.desc}</td>
                      </tr>
                    {/each}
                  </table>
                </div>
              {/if}

              {#if ep.query && ep.query.length > 0}
                <div class="params-block">
                  <span class="params-label">Query params</span>
                  <table class="params-table">
                    {#each ep.query as p}
                      <tr>
                        <td class="p-name"><code>{p.name}</code>{#if p.required}<span class="req">*</span>{/if}</td>
                        <td class="p-type">{p.type}</td>
                        <td class="p-desc">{p.desc}</td>
                      </tr>
                    {/each}
                  </table>
                </div>
              {/if}

              {#if ep.returns}
                <div class="returns"><span class="returns-label">Returns</span> <code class="returns-val">{ep.returns}</code></div>
              {/if}
            </div>
          {/each}
        </div>
      {/each}
    </div>
  </div>
</div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 1000;
    display: flex;
    align-items: stretch;
    justify-content: flex-end;
  }

  .backdrop {
    position: absolute;
    inset: 0;
    background: rgba(0, 0, 0, 0.55);
    backdrop-filter: blur(2px);
  }

  .panel {
    position: relative;
    width: min(760px, 90vw);
    background: #0d1117;
    border-left: 1px solid rgba(48, 54, 61, 0.8);
    display: flex;
    flex-direction: column;
    overflow: hidden;
    box-shadow: -8px 0 32px rgba(0, 0, 0, 0.5);
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 20px;
    border-bottom: 1px solid rgba(48, 54, 61, 0.6);
    background: rgba(13, 17, 23, 0.95);
    flex-shrink: 0;
    gap: 12px;
  }

  .header-left {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
  }

  .panel-title {
    font-size: 15px;
    font-weight: 700;
    color: var(--text-primary, #e6edf3);
    letter-spacing: 0.3px;
  }

  .panel-sub {
    font-size: 11px;
    color: var(--text-muted, #484f58);
  }

  .panel-sub code {
    font-family: 'SF Mono', monospace;
    color: var(--text-secondary, #8b949e);
    background: rgba(255,255,255,0.04);
    padding: 1px 4px;
    border-radius: 3px;
  }

  .close-btn {
    background: none;
    border: none;
    color: var(--text-muted, #484f58);
    font-size: 16px;
    cursor: pointer;
    padding: 4px 6px;
    border-radius: 4px;
    flex-shrink: 0;
    line-height: 1;
  }
  .close-btn:hover { color: var(--text-primary, #e6edf3); background: rgba(255,255,255,0.05); }

  .panel-body {
    flex: 1;
    overflow-y: auto;
    padding: 0 20px 32px;
  }

  .intro {
    font-size: 12px;
    color: var(--text-secondary, #8b949e);
    line-height: 1.6;
    padding: 16px 0 8px;
    border-bottom: 1px solid rgba(48, 54, 61, 0.4);
    margin-bottom: 4px;
  }

  .intro code {
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    color: var(--text-primary, #e6edf3);
    background: rgba(255,255,255,0.05);
    padding: 1px 5px;
    border-radius: 3px;
  }

  .intro strong { color: var(--text-primary, #e6edf3); font-weight: 600; }

  .section {
    padding: 20px 0 4px;
    border-bottom: 1px solid rgba(48, 54, 61, 0.35);
  }
  .section:last-child { border-bottom: none; }

  .section-heading {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 12px;
  }

  .section-title {
    font-size: 12px;
    font-weight: 700;
    color: var(--text-primary, #e6edf3);
    text-transform: uppercase;
    letter-spacing: 0.6px;
  }

  .section-tag {
    font-size: 10px;
    font-weight: 600;
    color: var(--accent, #58a6ff);
    background: rgba(88, 166, 255, 0.1);
    border: 1px solid rgba(88, 166, 255, 0.25);
    border-radius: 3px;
    padding: 1px 6px;
  }

  .endpoint {
    padding: 10px 0 10px 12px;
    border-left: 2px solid rgba(48, 54, 61, 0.5);
    margin-bottom: 8px;
  }
  .endpoint:hover { border-left-color: rgba(88, 166, 255, 0.35); }

  .ep-line {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 5px;
  }

  .method {
    font-size: 10px;
    font-weight: 700;
    font-family: 'SF Mono', monospace;
    letter-spacing: 0.5px;
    width: 44px;
    flex-shrink: 0;
  }

  .path {
    font-family: 'SF Mono', monospace;
    font-size: 12px;
    color: var(--text-primary, #e6edf3);
  }

  .ep-desc {
    font-size: 12px;
    color: var(--text-secondary, #8b949e);
    line-height: 1.55;
    margin: 0 0 6px;
  }

  .params-block {
    margin-top: 8px;
  }

  .params-label {
    font-size: 10px;
    font-weight: 600;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.4px;
    display: block;
    margin-bottom: 4px;
  }

  .params-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 11px;
  }

  .params-table tr { border-top: 1px solid rgba(48,54,61,0.3); }
  .params-table tr:first-child { border-top: none; }
  .params-table td { padding: 3px 8px 3px 0; vertical-align: top; }

  .p-name code {
    font-family: 'SF Mono', monospace;
    color: var(--text-primary, #e6edf3);
    font-size: 11px;
  }
  .p-name { width: 170px; }
  .req { color: #f85149; margin-left: 2px; }
  .p-type { color: var(--accent, #58a6ff); width: 60px; font-family: 'SF Mono', monospace; }
  .p-desc { color: var(--text-secondary, #8b949e); }

  .returns {
    font-size: 11px;
    margin-top: 6px;
    display: flex;
    align-items: baseline;
    gap: 6px;
    flex-wrap: wrap;
  }

  .returns-label {
    font-size: 10px;
    font-weight: 600;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.4px;
    flex-shrink: 0;
  }

  .returns-val {
    font-family: 'SF Mono', monospace;
    color: #3fb950;
    font-size: 11px;
    word-break: break-all;
  }
</style>
