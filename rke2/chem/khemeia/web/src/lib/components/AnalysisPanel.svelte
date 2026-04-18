<script lang="ts">
  import Panel from './Panel.svelte';
  import { getJobs, getJob, getPocketAnalysis, getReceptorContacts, getFingerprints } from '$lib/api';
  import type { PocketResidue, PocketAnalysis, ResidueContact, ReceptorContactsResponse, FingerprintCompound } from '$lib/api';
  import { loadFile, overlayStructure, focusLastStructure, focusResidue, highlightResidue, drawInteractionLines } from '$lib/viewer';
  import type { InteractionLine } from '$lib/viewer';
  import { isAuthenticated } from '$lib/auth';

  let jobs = $state<any[]>([]);
  let jobsLoading = $state(true);
  let jobsError = $state('');
  let selectedJobName = $state<string | null>(null);
  let selectedJob = $state<any | null>(null);
  let jobLoading = $state(false);
  let viewingCompound = $state<string | null>(null);
  let viewError = $state('');

  // Pocket analysis state
  let pocket = $state<PocketAnalysis | null>(null);
  let pocketLoading = $state(false);
  let pocketError = $state('');
  let pocketCutoff = $state(5.0);
  let pocketOpen = $state(true);

  // Receptor contacts state
  let receptorContacts = $state<ReceptorContactsResponse | null>(null);
  let rcLoading = $state(false);
  let rcError = $state('');

  // Fingerprints state
  let fpCompounds = $state<FingerprintCompound[]>([]);
  let fpLoading = $state(false);
  let fpError = $state('');
  let fpTotal = $state(0);

  const PLUGIN_SLUG = 'docking';

  $effect(() => {
    if (isAuthenticated()) {
      loadJobsList();
    } else {
      jobsLoading = false;
    }
  });

  async function loadJobsList() {
    jobsLoading = true;
    jobsError = '';
    try {
      const res = await getJobs(PLUGIN_SLUG);
      jobs = Array.isArray(res) ? res : res.jobs || [];
      // Auto-select the most recent job
      if (jobs.length > 0 && !selectedJobName) {
        await selectJob(jobs[0].name);
      }
    } catch (e: any) {
      jobsError = e.message || 'Failed to load docking jobs';
    } finally {
      jobsLoading = false;
    }
  }

  async function selectJob(name: string) {
    selectedJobName = name;
    jobLoading = true;
    viewingCompound = null;
    viewError = '';
    try {
      selectedJob = await getJob(PLUGIN_SLUG, name);
    } catch (e: any) {
      selectedJob = null;
    } finally {
      jobLoading = false;
    }
  }

  /** Extract only unique HETATM/ATOM lines from MODEL 1 of a Vina PDBQT.
   *  Vina's BRANCH tree duplicates atoms — deduplicate by atom serial number. */
  function extractBestPose(pdbqt: string): string {
    const lines = pdbqt.split('\n');
    const seen = new Set<string>();
    const out: string[] = [];
    let inFirstModel = false;
    for (const line of lines) {
      if (line.startsWith('MODEL')) {
        if (inFirstModel) break;
        inFirstModel = true;
        continue;
      }
      if (line.startsWith('ENDMDL')) break;
      if (inFirstModel && (line.startsWith('HETATM') || line.startsWith('ATOM'))) {
        // Atom serial is columns 7-11 (0-indexed 6-10)
        const serial = line.substring(6, 11).trim();
        if (!seen.has(serial)) {
          seen.add(serial);
          out.push(line);
        }
      }
    }
    return out.join('\n');
  }

  async function handleView(result: any) {
    viewError = '';
    viewingCompound = result.compound_id;
    try {
      // Load the preprocessed receptor
      const receptor = selectedJob?.receptor_pdbqt;
      if (receptor) {
        await loadFile(receptor, 'pdbqt');
      }
      // Overlay only the best docked pose (MODEL 1, deduplicated)
      if (result.pose_pdbqt) {
        const bestPose = extractBestPose(result.pose_pdbqt);
        await overlayStructure(bestPose, 'pdbqt');
        // Short delay to let Molstar finish rendering, then focus on ligand
        setTimeout(() => focusLastStructure(), 200);
      }
      // Fetch pocket analysis
      fetchPocket(result.compound_id);
    } catch (e: any) {
      viewError = e.message || 'Failed to load structure';
    }
  }

  async function fetchPocket(compoundId: string) {
    if (!selectedJob?.name) return;
    pocketLoading = true;
    pocketError = '';
    pocket = null;
    try {
      pocket = await getPocketAnalysis(selectedJob.name, compoundId, pocketCutoff);
      if (pocket?.interaction_lines?.length) {
        drawInteractionLines(pocket.interaction_lines, activeInteractions);
      }
    } catch (e: any) {
      pocketError = e.message || 'Pocket analysis failed';
    } finally {
      pocketLoading = false;
    }
  }

  function handleCutoffChange() {
    if (viewingCompound) fetchPocket(viewingCompound);
  }

  function handleResidueClick(res: PocketResidue) {
    focusResidue(res.chain_id, res.res_id);
  }

  function handleResidueHover(res: PocketResidue) {
    highlightResidue(res.chain_id, res.res_id);
  }

  async function fetchReceptorContacts() {
    if (!selectedJob?.name) return;
    rcLoading = true;
    rcError = '';
    try {
      receptorContacts = await getReceptorContacts(selectedJob.name, 50);
    } catch (e: any) {
      rcError = e.message || 'Failed to load receptor contacts';
    } finally {
      rcLoading = false;
    }
  }

  async function fetchFingerprints() {
    if (!selectedJob?.name) return;
    fpLoading = true;
    fpError = '';
    try {
      const res = await getFingerprints(selectedJob.name, 100);
      fpCompounds = res.compounds || [];
      fpTotal = res.total || 0;
    } catch (e: any) {
      fpError = e.message || 'Failed to load fingerprints';
    } finally {
      fpLoading = false;
    }
  }

  const interactionColors: Record<string, { bg: string; text: string; label: string }> = {
    hbond: { bg: 'rgba(88,166,255,0.15)', text: '#58a6ff', label: 'H-bond' },
    hydrophobic: { bg: 'rgba(139,148,158,0.15)', text: '#8b949e', label: 'Hydro' },
    ionic: { bg: 'rgba(210,153,34,0.15)', text: '#d29922', label: 'Ionic' },
    dipole: { bg: 'rgba(187,51,187,0.15)', text: '#bb33bb', label: 'Dipole' },
    contact: { bg: 'rgba(48,54,61,0.3)', text: '#484f58', label: 'Contact' },
  };

  // Interaction type toggles — which types to show in the table
  let activeInteractions = $state<Set<string>>(new Set(['hbond', 'hydrophobic', 'ionic', 'dipole', 'contact']));

  function toggleInteraction(type: string) {
    const next = new Set(activeInteractions);
    if (next.has(type)) next.delete(type);
    else next.add(type);
    activeInteractions = next;
    // Redraw lines with updated filter
    if (pocket?.interaction_lines?.length) {
      drawInteractionLines(pocket.interaction_lines, activeInteractions);
    }
  }

  let filteredResidues = $derived(
    pocket?.pocket_residues.filter(r =>
      r.interactions.some(ix => activeInteractions.has(ix))
    ) ?? []
  );

  function statusClass(status: string): string {
    const s = status?.toLowerCase();
    if (s === 'completed') return 'completed';
    if (s === 'failed') return 'failed';
    if (s === 'running') return 'running';
    return 'pending';
  }

  function formatDate(d: string): string {
    if (!d) return '';
    const date = new Date(d);
    return date.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  }

  // Stats computations
  let results = $derived(selectedJob?.docking_results ?? []);
  let totalResults = $derived(results.length);
  let bestAffinity = $derived(
    results.length > 0 ? results[0].affinity_kcal_mol : null
  );
  let meanAffinity = $derived(
    results.length > 0
      ? results.reduce((sum: number, r: any) => sum + r.affinity_kcal_mol, 0) / results.length
      : null
  );

  let elapsed = $derived.by(() => {
    if (!selectedJob?.created_at) return null;
    const start = new Date(selectedJob.created_at);
    const end = selectedJob.completed_at ? new Date(selectedJob.completed_at) : new Date();
    const seconds = Math.round((end.getTime() - start.getTime()) / 1000);
    if (seconds < 60) return `${seconds}s`;
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    if (mins < 60) return `${mins}m ${secs}s`;
    const hrs = Math.floor(mins / 60);
    return `${hrs}h ${mins % 60}m`;
  });
</script>

<div class="analysis-panels">
  <Panel title="Docking Jobs">
    {#if jobsLoading}
      <p class="loading">Loading jobs...</p>
    {:else if jobsError}
      <div class="error-box">
        <p class="error-title">Failed to load jobs</p>
        <p class="error-detail">{jobsError}</p>
        <button class="retry-btn" onclick={loadJobsList}>Retry</button>
      </div>
    {:else if jobs.length === 0}
      <p class="empty">No docking jobs found.</p>
    {:else}
      <select
        class="job-select"
        value={selectedJobName}
        onchange={(e) => selectJob((e.target as HTMLSelectElement).value)}
      >
        {#each jobs as job}
          <option value={job.name}>
            {job.name} [{job.status}]
          </option>
        {/each}
      </select>
    {/if}
  </Panel>

  {#if selectedJob && !jobLoading}
    <Panel title="Stats">
      <div class="stats-bar">
        <div class="stat">
          <span class="stat-label">Results</span>
          <span class="stat-value">{totalResults}</span>
        </div>
        {#if bestAffinity !== null}
          <div class="stat">
            <span class="stat-label">Best</span>
            <span class="stat-value mono">{bestAffinity.toFixed(2)}</span>
          </div>
        {/if}
        {#if meanAffinity !== null}
          <div class="stat">
            <span class="stat-label">Mean</span>
            <span class="stat-value mono">{meanAffinity.toFixed(2)}</span>
          </div>
        {/if}
        <div class="stat">
          <span class="stat-label">Status</span>
          <span class="status-badge {statusClass(selectedJob.status)}">{selectedJob.status}</span>
        </div>
        {#if elapsed}
          <div class="stat">
            <span class="stat-label">Elapsed</span>
            <span class="stat-value mono">{elapsed}</span>
          </div>
        {/if}
      </div>
    </Panel>

    {#if results.length > 0}
      <Panel title="Results">
        <div class="results-table-wrap">
          <table class="results-table">
            <thead>
              <tr>
                <th class="col-rank">#</th>
                <th class="col-compound">Compound</th>
                <th class="col-affinity">Affinity (kcal/mol)</th>
                <th class="col-action"></th>
              </tr>
            </thead>
            <tbody>
              {#each results as result, i}
                <tr class:top-hit={i < 3} class:alt={i % 2 === 1}>
                  <td class="col-rank">{i + 1}</td>
                  <td class="col-compound mono">{result.compound_id}</td>
                  <td class="col-affinity mono">{result.affinity_kcal_mol.toFixed(2)}</td>
                  <td class="col-action">
                    <button
                      class="view-btn"
                      class:active={viewingCompound === result.compound_id}
                      onclick={() => handleView(result)}
                    >
                      {viewingCompound === result.compound_id ? 'Viewing' : 'View'}
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
        {#if viewError}
          <p class="error-msg">{viewError}</p>
        {/if}
      </Panel>

      {#if viewingCompound}
        <Panel title="Binding Pocket">
          <div class="pocket-header">
            <label class="cutoff-label">
              Cutoff
              <input
                type="range"
                min="3" max="8" step="0.5"
                bind:value={pocketCutoff}
                onchange={handleCutoffChange}
                class="cutoff-slider"
              />
              <span class="cutoff-val">{pocketCutoff.toFixed(1)}A</span>
            </label>
          </div>

          {#if pocketLoading}
            <p class="loading">Analyzing pocket...</p>
          {:else if pocketError}
            <p class="error-msg">{pocketError}</p>
          {:else if pocket}
            <div class="pocket-toggles">
              {#each Object.entries(interactionColors) as [type, style]}
                {@const count = pocket.pocket_residues.filter(r => r.interactions.includes(type)).length}
                {#if count > 0}
                  <button
                    class="ix-toggle"
                    class:active={activeInteractions.has(type)}
                    style="background:{activeInteractions.has(type) ? style.bg : 'transparent'};color:{style.text};border-color:{style.text}"
                    onclick={() => toggleInteraction(type)}
                  >
                    {count} {style.label}
                  </button>
                {/if}
              {/each}
              <span class="pocket-count">{filteredResidues.length} residues</span>
            </div>

            {#if filteredResidues.length > 0}
              <div class="pocket-table-wrap">
                <table class="pocket-table">
                  <thead>
                    <tr>
                      <th>Chain</th>
                      <th>Res</th>
                      <th>Name</th>
                      <th>Dist (A)</th>
                      <th>Interactions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each filteredResidues as res}
                      <tr
                        class="pocket-row"
                        onclick={() => handleResidueClick(res)}
                        onmouseenter={() => handleResidueHover(res)}
                      >
                        <td>{res.chain_id}</td>
                        <td class="mono">{res.res_id}</td>
                        <td class="mono">{res.res_name}</td>
                        <td class="mono">{res.min_distance.toFixed(1)}</td>
                        <td class="ix-cell">
                          {#each res.interactions as ix}
                            {@const style = interactionColors[ix] || interactionColors.contact}
                            <span class="ix-pill" style="background:{style.bg};color:{style.text}">
                              {style.label}
                            </span>
                          {/each}
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            {:else}
              <p class="empty">No residues within {pocketCutoff.toFixed(1)}A</p>
            {/if}
          {/if}
        </Panel>
      {/if}

      <!-- Receptor Interaction Map (job-level, not per-compound) -->
      <Panel title="Receptor Interaction Map">
        {#if !receptorContacts && !rcLoading}
          <button class="analyze-btn" onclick={fetchReceptorContacts} disabled={rcLoading}>
            Analyze Top 50 Binders
          </button>
        {/if}
        {#if rcLoading}
          <p class="loading">Analyzing receptor contacts...</p>
        {:else if rcError}
          <p class="error-msg">{rcError}</p>
        {:else if receptorContacts}
          <p class="rc-summary">{receptorContacts.residue_contacts.length} residues contacted by {receptorContacts.total_compounds_analyzed} compounds</p>
          <div class="pocket-table-wrap">
            <table class="pocket-table">
              <thead>
                <tr>
                  <th>Res</th>
                  <th>Name</th>
                  <th>Freq</th>
                  <th>Avg Dist</th>
                  <th>H-bond</th>
                  <th>Hydro</th>
                  <th>Ionic</th>
                </tr>
              </thead>
              <tbody>
                {#each receptorContacts.residue_contacts.slice(0, 30) as rc}
                  <tr class="pocket-row" onclick={() => focusResidue(rc.chain_id, rc.res_id)}>
                    <td class="mono">{rc.chain_id}{rc.res_id}</td>
                    <td class="mono">{rc.res_name}</td>
                    <td>
                      <div class="freq-bar-wrap">
                        <div class="freq-bar" style="width:{rc.contact_frequency * 100}%"></div>
                        <span class="freq-label">{(rc.contact_frequency * 100).toFixed(0)}%</span>
                      </div>
                    </td>
                    <td class="mono">{rc.avg_distance.toFixed(1)}</td>
                    <td class="mono">{rc.interaction_counts?.hbond || 0}</td>
                    <td class="mono">{rc.interaction_counts?.hydrophobic || 0}</td>
                    <td class="mono">{rc.interaction_counts?.ionic || 0}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </Panel>

      <!-- Compound Fingerprints -->
      <Panel title="Top Compounds">
        {#if !fpCompounds.length && !fpLoading}
          <button class="analyze-btn" onclick={fetchFingerprints} disabled={fpLoading}>
            Load Top 100
          </button>
        {/if}
        {#if fpLoading}
          <p class="loading">Loading compounds...</p>
        {:else if fpError}
          <p class="error-msg">{fpError}</p>
        {:else if fpCompounds.length > 0}
          <div class="pocket-table-wrap">
            <table class="pocket-table">
              <thead>
                <tr>
                  <th>#</th>
                  <th>Compound</th>
                  <th>Affinity</th>
                  <th>SMILES</th>
                </tr>
              </thead>
              <tbody>
                {#each fpCompounds as comp, i}
                  <tr>
                    <td>{i + 1}</td>
                    <td class="mono">{comp.compound_id}</td>
                    <td class="mono">{comp.affinity.toFixed(1)}</td>
                    <td class="smiles-cell" title={comp.smiles}>{comp.smiles.length > 30 ? comp.smiles.slice(0, 30) + '...' : comp.smiles}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </Panel>

    {:else if selectedJob.status?.toLowerCase() === 'completed'}
      <Panel title="Results">
        <p class="empty">No docking results available.</p>
      </Panel>
    {/if}
  {:else if jobLoading}
    <Panel title="Stats">
      <p class="loading">Loading job details...</p>
    </Panel>
  {/if}
</div>

<style>
  .analysis-panels {
    display: flex;
    flex-direction: column;
  }

  .loading, .empty {
    color: var(--text-muted, #484f58);
    font-size: 13px;
    padding: 8px 0;
    text-align: center;
  }

  .error-box {
    padding: 8px 0;
    text-align: center;
  }

  .error-title {
    color: var(--danger, #f85149);
    font-weight: 600;
    font-size: 13px;
    margin-bottom: 4px;
  }

  .error-detail {
    color: var(--text-secondary, #8b949e);
    font-size: 12px;
    margin-bottom: 8px;
  }

  .error-msg {
    color: var(--danger, #f85149);
    font-size: 12px;
    margin-top: 8px;
  }

  .retry-btn {
    background: rgba(88,166,255,0.1);
    border: 1px solid rgba(88,166,255,0.3);
    color: var(--accent, #58a6ff);
    font-size: 11px;
    padding: 4px 10px;
    border-radius: 4px;
    cursor: pointer;
  }

  .job-select {
    width: 100%;
    background: rgba(0,0,0,0.3);
    border: 1px solid rgba(48,54,61,0.6);
    color: var(--text-primary, #e6edf3);
    font-family: 'SF Mono', monospace;
    font-size: 12px;
    padding: 6px 8px;
    border-radius: 4px;
    outline: none;
    transition: border-color 0.15s;
  }

  .job-select:focus {
    border-color: var(--accent, #58a6ff);
  }

  /* Stats bar */
  .stats-bar {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 4px;
  }

  .stat {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 3px 8px;
    background: rgba(0,0,0,0.2);
    border-radius: 4px;
  }

  .stat-label {
    font-size: 10px;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.3px;
    font-weight: 600;
  }

  .stat-value {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-primary, #e6edf3);
  }

  .stat-value.mono {
    font-family: 'SF Mono', monospace;
  }

  .status-badge {
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    padding: 1px 6px;
    border-radius: 3px;
    white-space: nowrap;
  }

  .status-badge.pending { background: rgba(88,166,255,0.1); color: var(--accent, #58a6ff); }
  .status-badge.running { background: rgba(210,153,34,0.15); color: #d29922; }
  .status-badge.completed { background: rgba(63,185,80,0.15); color: #3fb950; }
  .status-badge.failed { background: rgba(248,81,73,0.15); color: #f85149; }

  /* Results table */
  .results-table-wrap {
    border: 1px solid rgba(48,54,61,0.4);
    border-radius: 6px;
    overflow: hidden;
    max-height: 600px;
    overflow-y: auto;
  }

  .results-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 11px;
    table-layout: fixed;
  }

  .results-table thead {
    position: sticky;
    top: 0;
    z-index: 1;
  }

  .results-table th {
    background: rgba(0,0,0,0.3);
    padding: 5px 8px;
    font-size: 10px;
    font-weight: 600;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.3px;
    text-align: left;
  }

  .results-table td {
    padding: 4px 8px;
    color: var(--text-primary, #e6edf3);
    transition: background 0.1s;
  }

  .results-table td.mono {
    font-family: 'SF Mono', monospace;
    font-size: 11px;
  }

  .results-table tr:hover td {
    background: rgba(255,255,255,0.03);
  }

  .results-table tr.alt td {
    background: rgba(0,0,0,0.15);
  }

  .results-table tr.alt:hover td {
    background: rgba(0,0,0,0.25);
  }

  .results-table tr.top-hit td {
    background: rgba(63,185,80,0.08);
  }

  .results-table tr.top-hit:hover td {
    background: rgba(63,185,80,0.14);
  }

  .results-table tr.top-hit.alt td {
    background: rgba(63,185,80,0.06);
  }

  .results-table tr.top-hit.alt:hover td {
    background: rgba(63,185,80,0.12);
  }

  .col-rank {
    width: 32px;
    text-align: center;
  }

  .col-compound {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .col-affinity {
    text-align: right;
    width: 110px;
    font-weight: 600;
  }

  th.col-affinity {
    text-align: right;
  }

  .col-action {
    width: 60px;
    text-align: right;
    padding-right: 4px;
    white-space: nowrap;
  }

  .view-btn {
    background: rgba(88,166,255,0.08);
    border: 1px solid rgba(88,166,255,0.2);
    color: var(--accent, #58a6ff);
    font-size: 10px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 3px;
    cursor: pointer;
    transition: all 0.15s;
  }

  .view-btn:hover {
    background: rgba(88,166,255,0.15);
    border-color: rgba(88,166,255,0.4);
  }

  .view-btn.active {
    background: rgba(88,166,255,0.2);
    border-color: var(--accent, #58a6ff);
  }

  /* ---- Pocket Analysis ---- */
  .pocket-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }

  .cutoff-label {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 10px;
    font-weight: 600;
    color: var(--text-secondary, #8b949e);
    text-transform: uppercase;
  }

  .cutoff-slider {
    width: 100px;
    accent-color: var(--accent, #58a6ff);
  }

  .cutoff-val {
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    color: var(--text-primary, #e6edf3);
    min-width: 35px;
  }

  .pocket-toggles {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    margin-bottom: 8px;
    align-items: center;
  }

  .pocket-count {
    font-size: 10px;
    color: var(--text-muted, #484f58);
    margin-left: auto;
  }

  .ix-toggle {
    font-size: 10px;
    font-weight: 600;
    padding: 2px 7px;
    border-radius: 8px;
    border: 1px solid;
    cursor: pointer;
    opacity: 0.5;
    transition: opacity 0.15s;
  }

  .ix-toggle.active {
    opacity: 1;
  }

  .ix-toggle:hover {
    opacity: 0.8;
  }

  .pocket-table-wrap {
    border: 1px solid rgba(48,54,61,0.4);
    border-radius: 6px;
    overflow: hidden;
    max-height: 300px;
    overflow-y: auto;
  }

  .pocket-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 10px;
  }

  .pocket-table th {
    text-align: left;
    font-size: 9px;
    font-weight: 600;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    padding: 3px 6px;
    border-bottom: 1px solid rgba(48,54,61,0.6);
    position: sticky;
    top: 0;
    background: rgba(13,17,23,0.95);
  }

  .pocket-table td {
    padding: 3px 6px;
    color: var(--text-primary, #e6edf3);
    border-bottom: 1px solid rgba(48,54,61,0.2);
  }

  .pocket-row {
    cursor: pointer;
  }

  .pocket-row:hover td {
    background: rgba(88,166,255,0.05);
  }

  .ix-cell {
    display: flex;
    gap: 3px;
    flex-wrap: wrap;
  }

  .ix-pill {
    font-size: 9px;
    font-weight: 600;
    padding: 1px 5px;
    border-radius: 6px;
    white-space: nowrap;
  }

  /* ---- Analysis Panels ---- */
  .analyze-btn {
    background: var(--accent, #58a6ff);
    border: none;
    color: #000;
    font-size: 12px;
    font-weight: 600;
    padding: 6px 12px;
    border-radius: 6px;
    cursor: pointer;
    width: 100%;
  }

  .analyze-btn:hover:not(:disabled) { opacity: 0.9; }
  .analyze-btn:disabled { opacity: 0.4; cursor: not-allowed; }

  .rc-summary {
    font-size: 11px;
    color: var(--text-secondary, #8b949e);
    margin-bottom: 6px;
  }

  .freq-bar-wrap {
    display: flex;
    align-items: center;
    gap: 4px;
    min-width: 80px;
  }

  .freq-bar {
    height: 8px;
    background: linear-gradient(90deg, #3fb950, #58a6ff);
    border-radius: 4px;
    min-width: 2px;
  }

  .freq-label {
    font-size: 9px;
    color: var(--text-muted, #484f58);
    white-space: nowrap;
  }

  .smiles-cell {
    font-family: 'SF Mono', monospace;
    font-size: 9px;
    color: var(--text-muted, #484f58);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 150px;
    cursor: help;
  }
</style>
