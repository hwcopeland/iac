<script lang="ts">
  import Panel from './Panel.svelte';
  import InteractionNetwork from './InteractionNetwork.svelte';
  import { getJobs, getJob, getPocketAnalysis, getReceptorContacts, getFingerprints, getLigandSmiles, getDockingResult } from '$lib/api';
  import type { PocketResidue, PocketAnalysis, ResidueContact, ReceptorContactsResponse, FingerprintCompound } from '$lib/api';
  import { loadFile, focusLastStructure, focusResidue, highlightResidue, drawInteractionLines, showPocketView, togglePocketSurface, clearPocketView } from '$lib/viewer';
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

  let { onNetworkToggle = (_show: boolean, _smiles: string, _residues: any[], _jobName: string, _compoundId: string) => {},
        onSurfaceChange = (_theme: string | null) => {} }:
    {
      onNetworkToggle?: (show: boolean, smiles: string, residues: any[], jobName: string, compoundId: string) => void;
      onSurfaceChange?: (theme: string | null) => void;
    } = $props();

  let viewedSmiles = $state('');
  let showNetwork = $state(false);

  function toggleNetwork() {
    showNetwork = !showNetwork;
    onNetworkToggle(showNetwork, viewedSmiles, pocket?.pocket_residues ?? [],
      selectedJob?.name ?? '', viewingCompound ?? '');
  }
  let currentPage = $state(1);
  let perPage = $state(10);
  let totalResults = $state(0);

  // Pocket analysis state
  let pocket = $state<PocketAnalysis | null>(null);
  let pocketLoading = $state(false);
  let pocketError = $state('');
  let pocketCutoff = $state(6.0);
  let pocketOpen = $state(true);
  let showSurfaceMesh = $state(false);
  let surfaceType = $state('charge');
  let surfaceAlpha = $state(0.8);

  // Receptor contacts state
  let receptorContacts = $state<ReceptorContactsResponse | null>(null);
  let rcLoading = $state(false);
  let rcError = $state('');

  // Fingerprints state
  let fpCompounds = $state<FingerprintCompound[]>([]);
  let fpLoading = $state(false);
  let fpError = $state('');
  let fpTotal = $state(0);

  // Top-N selector for analysis
  let analysisTopN = $state(100);

  // Expanded compound in Hit table (shows full ADMET profile)
  let expandedCompoundId = $state<string | null>(null);

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

  async function selectJob(name: string, page = 1) {
    selectedJobName = name;
    jobLoading = true;
    viewingCompound = null;
    viewError = '';
    currentPage = page;
    try {
      selectedJob = await getJob(PLUGIN_SLUG, name, page, perPage);
      totalResults = selectedJob?.output_data?.total_results ?? selectedJob?.docking_results?.length ?? 0;
    } catch (e: any) {
      selectedJob = null;
    } finally {
      jobLoading = false;
    }
  }

  /** Extract unique HETATM/ATOM lines from MODEL 1 of a Vina PDBQT.
   *  Vina's BRANCH tree duplicates atoms — deduplicate by atom serial number. */
  function extractBestPoseLines(pdbqt: string): string[] {
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
        const serial = line.substring(6, 11).trim();
        if (!seen.has(serial)) {
          seen.add(serial);
          out.push(line);
        }
      }
    }
    return out;
  }

  /** Extract element symbol from PDBQT atom type (col 77+).
   *  PDBQT types: C, A (aromatic C), N, NA, NS, O, OA, OS, S, SA, H, HD, etc.
   *  Maps to PDB element: C, C, N, N, N, O, O, O, S, S, H, H, etc. */
  function pdbqtElement(line: string): string {
    // PDBQT atom type is at column 77+ (0-indexed)
    const atomType = line.substring(77).trim();
    if (!atomType) {
      // Fallback: derive from atom name (cols 12-15)
      const name = line.substring(12, 16).trim();
      return name.replace(/[0-9]/g, '').substring(0, 1).toUpperCase() || 'C';
    }
    // Map PDBQT types to elements
    const first = atomType[0];
    if (first === 'A') return ' C'; // aromatic carbon
    if (atomType === 'HD' || atomType === 'HS') return ' H';
    if (atomType === 'OA' || atomType === 'OS') return ' O';
    if (atomType === 'NA' || atomType === 'NS') return ' N';
    if (atomType === 'SA') return ' S';
    // Single-letter types: C, N, O, S, H, P, F, Cl, Br, I
    if (atomType.length === 1) return ' ' + atomType;
    // Two-letter elements (Cl, Br, Fe, Zn, etc.)
    return atomType.substring(0, 2).padStart(2);
  }

  /** Convert PDBQT line to proper PDB with element symbol in cols 77-78. */
  function pdbqtLineToPdb(line: string, recType?: string): string {
    const base = (recType ?? line.substring(0, 6)) + line.substring(6, 66);
    const element = pdbqtElement(line);
    // PDB format: cols 1-66 = coordinates, 67-76 = padding, 77-78 = element
    return base.padEnd(76) + element.padStart(2);
  }

  /** Convert receptor PDBQT to PDB lines with proper element symbols. */
  function receptorToPdbLines(pdbqt: string): string[] {
    return pdbqt.split('\n')
      .filter(line => line.startsWith('ATOM'))
      .map(line => pdbqtLineToPdb(line));
  }

  /** Convert ligand PDBQT lines to HETATM with residue name UNL + element symbols. */
  function ligandToPdbLines(pdbqtLines: string[]): string[] {
    return pdbqtLines.map(line => {
      // Force HETATM + residue name UNL, keep coordinates, add element
      const base = 'HETATM' + line.substring(6, 17) + 'UNL' + line.substring(20, 66);
      const element = pdbqtElement(line);
      return base.padEnd(76) + element.padStart(2);
    });
  }

  /** Combine receptor + ligand into a single PDB so Molstar detects
   *  non-covalent interactions (H-bonds, hydrophobic, etc.) natively. */
  function buildCombinedPdb(receptorPdbqt: string, ligandPdbqt: string): string {
    const recLines = receptorToPdbLines(receptorPdbqt);
    const ligPdbqtLines = extractBestPoseLines(ligandPdbqt);
    const ligLines = ligandToPdbLines(ligPdbqtLines);
    return [...recLines, 'TER', ...ligLines, 'END'].join('\n');
  }

  async function handleView(result: any) {
    viewError = '';
    viewingCompound = result.compound_id;

    try {
      const receptor = selectedJob?.receptor_pdbqt;
      if (receptor && result.pose_pdbqt) {
        const combinedPdb = buildCombinedPdb(receptor, result.pose_pdbqt);
        await loadFile(combinedPdb, 'pdb');
      } else if (receptor) {
        await loadFile(receptor, 'pdbqt');
      }

      // Fetch pocket analysis — this applies pocket view + focuses ligand
      await fetchPocket(result.compound_id);

      // Fetch SMILES for interaction network
      const smiles = await getLigandSmiles(result.compound_id);
      if (smiles) viewedSmiles = smiles;

      // Auto-show interaction network if we have pocket data
      if (pocket?.pocket_residues?.length && viewedSmiles) {
        showNetwork = true;
        onNetworkToggle(true, viewedSmiles, pocket.pocket_residues,
          selectedJob?.name ?? '', result.compound_id);
      }
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
      // Show pocket view — protein as thin cartoon, interacting residues as sticks
      if (pocket?.pocket_residues?.length) {
        await showPocketView(pocket.pocket_residues.map(r => ({ chain_id: r.chain_id, res_id: r.res_id })));
      }
      // Focus on the ligand after pocket view is applied
      setTimeout(() => focusLastStructure(), 100);
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
      receptorContacts = await getReceptorContacts(selectedJob.name, analysisTopN);
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
      const res = await getFingerprints(selectedJob.name, analysisTopN);
      fpCompounds = res.compounds || [];
      fpTotal = res.total || 0;
    } catch (e: any) {
      fpError = e.message || 'Failed to load fingerprints';
    } finally {
      fpLoading = false;
    }
  }

  function exportCSV() {
    if (!fpCompounds.length) return;
    const header = 'Rank,CompoundID,SMILES,Affinity,MW,LogP,HBA,HBD,PSA,QED,Lipinski,Veber,LeadLike,P450Risk,HighPSA';
    const rows = fpCompounds.map((c, i) => {
      const a = c.admet;
      return [
        i + 1,
        c.compound_id,
        `"${(c.smiles || '').replace(/"/g, '""')}"`,
        c.affinity?.toFixed(2) ?? '',
        c.mw?.toFixed(1) ?? '',
        c.logp?.toFixed(2) ?? '',
        c.hba ?? '',
        c.hbd ?? '',
        c.psa?.toFixed(1) ?? '',
        c.qed?.toFixed(3) ?? '',
        a?.lipinski ? 'PASS' : 'FAIL',
        a?.veber ? 'PASS' : 'FAIL',
        a?.lead_like ? 'YES' : 'NO',
        a?.p450_risk ? 'YES' : 'NO',
        a?.high_psa ? 'YES' : 'NO',
      ].join(',');
    });
    const csv = [header, ...rows].join('\n');
    const blob = new Blob([csv], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${selectedJob?.name ?? 'compounds'}-top-${fpCompounds.length}.csv`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 5000);
  }

  async function viewCompoundFromTable(compoundId: string) {
    if (!selectedJob?.name) return;
    try {
      const result = await getDockingResult(selectedJob.name, compoundId);
      if (result) {
        handleView(result);
      } else {
        viewError = `No docking result found for ${compoundId}`;
      }
    } catch (e: any) {
      viewError = e.message || 'Failed to fetch compound';
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
  <!-- Phase 1: Job selection (hidden once a job is selected) -->
  {#if !selectedJob}
    <Panel title="Jobs">
      {#if jobsLoading}
        <p class="loading">Loading jobs...</p>
      {:else if jobsError}
        <div class="error-box">
          <p class="error-title">Failed to load jobs</p>
          <button class="retry-btn" onclick={loadJobsList}>Retry</button>
        </div>
      {:else if jobs.length === 0}
        <p class="empty">No docking jobs found.</p>
      {:else}
        {#each jobs as job}
          <button class="job-row" onclick={() => selectJob(job.name)}>
            <span class="job-row-name">{job.name.replace('docking-', '')}</span>
            <span class="status-badge {statusClass(job.status)}">{job.status}</span>
          </button>
        {/each}
      {/if}
    </Panel>
  {/if}

  {#if selectedJob && !jobLoading}
    <!-- Phase 2: Results (collapsible, collapses when viewing a compound) -->
    <Panel title={viewingCompound ? `Ligand: ${viewingCompound}` : `Results (${totalResults})`}>
      {#if viewingCompound}
        <!-- Back button + stats when viewing a compound -->
        <div class="viewing-header">
          <button class="back-btn" onclick={() => { viewingCompound = null; pocket = null; showSurfaceMesh = false; showNetwork = false; clearPocketView(); onSurfaceChange(null); }}>
            Back to Results
          </button>
          {#each results.filter((r) => r.compound_id === viewingCompound).slice(0, 1) as viewedResult}
            <span class="viewing-affinity mono">{viewedResult.affinity_kcal_mol.toFixed(2)} kcal/mol</span>
          {/each}
        </div>
        {#if viewError}
          <p class="error-msg">{viewError}</p>
        {/if}
      {:else}
        <!-- Results table -->
        <div class="results-table-wrap">
          <table class="results-table">
            <thead>
              <tr>
                <th class="col-rank">#</th>
                <th class="col-compound">Compound</th>
                <th class="col-affinity">Affinity</th>
                <th class="col-action"></th>
              </tr>
            </thead>
            <tbody>
              {#each results as result, i}
                <tr class:top-hit={i < 3} class:alt={i % 2 === 1}>
                  <td class="col-rank">{(currentPage-1)*perPage + i + 1}</td>
                  <td class="col-compound mono">{result.compound_id}</td>
                  <td class="col-affinity mono">{(result.affinity_kcal_mol ?? 0).toFixed(2)}</td>
                  <td class="col-action">
                    <button class="view-btn" onclick={() => handleView(result)}>View</button>
                  </td>
                </tr>
              {:else}
                <tr><td colspan="4" class="empty-row">No docking results for this job.</td></tr>
              {/each}
            </tbody>
          </table>
        </div>
        {#if totalResults > perPage}
          <div class="pagination">
            <button class="page-btn" onclick={() => selectJob(selectedJobName!, currentPage - 1)} disabled={currentPage <= 1}>Prev</button>
            <span class="page-info">{(currentPage-1)*perPage+1}-{Math.min(currentPage*perPage, totalResults)} of {totalResults}</span>
            <button class="page-btn" onclick={() => selectJob(selectedJobName!, currentPage + 1)} disabled={currentPage * perPage >= totalResults}>Next</button>
          </div>
        {/if}
      {/if}
    </Panel>

    <!-- Phase 3: Compound analysis (only when viewing) -->
    {#if viewingCompound}
      <!-- ProLIF interaction map toggle -->
      {#if pocket && pocket.pocket_residues.length > 0}
        <button class="net-toggle" onclick={toggleNetwork}>
          {showNetwork ? 'Hide' : 'Show'} Interaction Map
        </button>
      {/if}

      <!-- Binding Pocket -->
      <Panel title="Binding Pocket">
        {#if pocketLoading}
          <p class="loading">Analyzing pocket...</p>
        {:else if pocketError}
          <p class="error-msg">{pocketError}</p>
        {:else if pocket}
          <!-- Interaction type toggles -->
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
            <span class="pocket-count">{filteredResidues.length} res</span>
          </div>

          <!-- Residue table -->
          {#if filteredResidues.length > 0}
            <div class="pocket-table-wrap">
              <table class="pocket-table">
                <thead>
                  <tr>
                    <th>Res</th>
                    <th>Dist</th>
                    <th>Type</th>
                  </tr>
                </thead>
                <tbody>
                  {#each filteredResidues as res}
                    <tr class="pocket-row" onclick={() => handleResidueClick(res)} onmouseenter={() => handleResidueHover(res)}>
                      <td class="mono">{res.res_name}{res.res_id}.{res.chain_id}</td>
                      <td class="mono">{res.min_distance.toFixed(1)}</td>
                      <td class="ix-cell">
                        {#each res.interactions.filter(ix => ix !== 'contact') as ix}
                          {@const style = interactionColors[ix] || interactionColors.contact}
                          <span class="ix-pill" style="background:{style.bg};color:{style.text}">{style.label}</span>
                        {/each}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {:else}
            <p class="empty">No residues within {pocketCutoff.toFixed(1)} Å cutoff.</p>
          {/if}

          <!-- Surface options -->
          <div class="surface-section">
            <p class="section-label">Surface</p>
            <div class="surface-btns">
              <button class="surface-btn" class:active={showSurfaceMesh && surfaceType === 'charge'}
                onclick={() => { surfaceType = 'charge'; showSurfaceMesh = true; togglePocketSurface(true, 'residue-charge', surfaceAlpha); onSurfaceChange('residue-charge'); }}>
                Charge
              </button>
              <button class="surface-btn" class:active={showSurfaceMesh && surfaceType === 'hydro'}
                onclick={() => { surfaceType = 'hydro'; showSurfaceMesh = true; togglePocketSurface(true, 'hydrophobicity', surfaceAlpha); onSurfaceChange('hydrophobicity'); }}>
                Hydrophobic
              </button>
              <button class="surface-btn" class:active={showSurfaceMesh && surfaceType === 'element'}
                onclick={() => { surfaceType = 'element'; showSurfaceMesh = true; togglePocketSurface(true, 'element-symbol', surfaceAlpha); onSurfaceChange('element-symbol'); }}>
                Element
              </button>
              <button class="surface-btn" class:active={!showSurfaceMesh}
                onclick={() => { showSurfaceMesh = false; togglePocketSurface(false); onSurfaceChange(null); }}>
                Off
              </button>
            </div>
          </div>

          <!-- Advanced section (collapsed) -->
          <details class="adv-section">
            <summary class="adv-summary">Advanced</summary>
            <label class="cutoff-label">
              Cutoff
              <input type="range" min="3" max="8" step="0.5" bind:value={pocketCutoff} onchange={handleCutoffChange} class="cutoff-slider" />
              <span class="cutoff-val">{pocketCutoff.toFixed(1)}A</span>
            </label>
            <label class="cutoff-label">
              Surface Opacity
              <input type="range" min="0.1" max="1" step="0.1" bind:value={surfaceAlpha}
                onchange={() => { if (showSurfaceMesh) togglePocketSurface(true, surfaceType === 'charge' ? 'residue-charge' : surfaceType === 'hydro' ? 'hydrophobicity' : 'element-symbol', surfaceAlpha); }}
                class="cutoff-slider" />
              <span class="cutoff-val">{Math.round(surfaceAlpha * 100)}%</span>
            </label>
          </details>
        {/if}
      </Panel>
    {/if}

    <!-- Job-level analysis (always visible when job selected) -->
    {#if results.length > 0 && !viewingCompound}
      <Panel title="Receptor Contacts">
        {#if !receptorContacts && !rcLoading}
          <div class="topn-row">
            <select class="topn-select" bind:value={analysisTopN}>
              <option value={50}>Top 50</option>
              <option value={100}>Top 100</option>
              <option value={200}>Top 200</option>
              <option value={500}>Top 500</option>
            </select>
            <button class="analyze-btn" onclick={fetchReceptorContacts}>Analyze</button>
          </div>
        {/if}
        {#if rcLoading}
          <p class="loading">Analyzing...</p>
        {:else if receptorContacts}
          {#if receptorContacts.residue_contacts.length > 0}
            <p class="rc-summary">Top {receptorContacts.top_n} compounds, {receptorContacts.total_compounds_analyzed} analyzed</p>
            <div class="pocket-table-wrap">
              <table class="pocket-table">
                <thead><tr>
                  <th>Res</th>
                  <th title="Influence score: 0.4×frequency + 0.35×affinity-weighted freq + 0.25×beneficial interaction ratio. Higher = more important for binding.">Influence</th>
                  <th>Freq</th>
                  <th>Dist</th>
                </tr></thead>
                <tbody>
                  {#each receptorContacts.residue_contacts.slice(0, 25) as rc}
                    <tr class="pocket-row" onclick={() => focusResidue(rc.chain_id, rc.res_id)}>
                      <td class="mono">{rc.res_name}{rc.res_id}.{rc.chain_id}</td>
                      <td><div class="freq-bar-wrap"><div class="influence-bar" style="width:{Math.min(rc.influence_score * 100, 100)}%"></div><span class="freq-label">{(rc.influence_score * 100).toFixed(0)}</span></div></td>
                      <td class="mono freq-secondary">{(rc.contact_frequency*100).toFixed(0)}%</td>
                      <td class="mono">{rc.avg_distance.toFixed(1)}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {:else}
            <p class="empty">No receptor contacts found.</p>
          {/if}
        {:else if rcError}
          <p class="error-msg">{rcError}</p>
        {/if}
      </Panel>

      <Panel title="Top Compounds">
        {#if !fpCompounds.length && !fpLoading && !fpError}
          <button class="analyze-btn" onclick={fetchFingerprints}>Load Top {analysisTopN} + ADMET</button>
        {/if}
        {#if fpLoading}
          <p class="loading">Loading compounds &amp; ADMET...</p>
        {:else if fpError}
          <p class="error-msg">{fpError}</p>
        {/if}
        {#if fpCompounds.length > 0}
          <p class="rc-summary">{fpTotal} compounds — showing top {Math.min(fpCompounds.length, 50)}</p>
          <div class="pocket-table-wrap" style="max-height:400px">
            <table class="pocket-table">
              <thead><tr><th>#</th><th>ID</th><th>Aff.</th><th>MW</th><th>LogP</th><th>QED</th><th>ADMET</th></tr></thead>
              <tbody>
                {#each fpCompounds.slice(0, 50) as comp, i}
                  <tr class="pocket-row" onclick={() => { expandedCompoundId = expandedCompoundId === comp.compound_id ? null : comp.compound_id; }}>
                    <td>{i+1}</td>
                    <td class="mono">{comp.compound_id}</td>
                    <td class="mono">{(comp.affinity ?? 0).toFixed(1)}</td>
                    <td class="mono">{comp.mw?.toFixed(0) ?? '—'}</td>
                    <td class="mono">{comp.logp?.toFixed(1) ?? '—'}</td>
                    <td class="mono">{comp.qed?.toFixed(2) ?? '—'}</td>
                    <td class="admet-cell">
                      {#if comp.admet}
                        {#if comp.admet.lipinski}<span class="admet-pill pass">Lip</span>{:else}<span class="admet-pill fail">Lip</span>{/if}
                        {#if comp.admet.veber}<span class="admet-pill pass">Veb</span>{:else}<span class="admet-pill fail">Veb</span>{/if}
                        {#if comp.admet.lead_like}<span class="admet-pill pass">Lead</span>{/if}
                        {#if comp.admet.p450_risk}<span class="admet-pill warn">CYP</span>{/if}
                        {#if comp.admet.high_psa}<span class="admet-pill fail">PSA</span>{/if}
                      {/if}
                    </td>
                  </tr>
                  {#if expandedCompoundId === comp.compound_id}
                    <tr class="admet-detail-row">
                      <td colspan="7">
                        <div class="admet-detail">
                          <div class="admet-detail-header">
                            <span class="admet-detail-title">{comp.compound_id}</span>
                            <button class="view-btn" onclick={(e) => { e.stopPropagation(); viewCompoundFromTable(comp.compound_id); }}>View 3D</button>
                          </div>
                          <div class="admet-props-grid">
                            <div class="admet-prop">
                              <span class="admet-prop-label">MW</span>
                              <span class="admet-prop-val" class:warn={comp.mw != null && comp.mw > 500}>{comp.mw?.toFixed(1) ?? '—'}</span>
                            </div>
                            <div class="admet-prop">
                              <span class="admet-prop-label">LogP</span>
                              <span class="admet-prop-val" class:warn={comp.logp != null && comp.logp > 5}>{comp.logp?.toFixed(2) ?? '—'}</span>
                            </div>
                            <div class="admet-prop">
                              <span class="admet-prop-label">HBA</span>
                              <span class="admet-prop-val" class:warn={comp.hba != null && comp.hba > 10}>{comp.hba ?? '—'}</span>
                            </div>
                            <div class="admet-prop">
                              <span class="admet-prop-label">HBD</span>
                              <span class="admet-prop-val" class:warn={comp.hbd != null && comp.hbd > 5}>{comp.hbd ?? '—'}</span>
                            </div>
                            <div class="admet-prop">
                              <span class="admet-prop-label">PSA</span>
                              <span class="admet-prop-val" class:warn={comp.psa != null && comp.psa > 140}>{comp.psa?.toFixed(1) ?? '—'} A</span>
                            </div>
                            <div class="admet-prop">
                              <span class="admet-prop-label">QED</span>
                              <span class="admet-prop-val" class:good={comp.qed != null && comp.qed >= 0.5}>{comp.qed?.toFixed(3) ?? '—'}</span>
                            </div>
                            <div class="admet-prop">
                              <span class="admet-prop-label">RO5 Violations</span>
                              <span class="admet-prop-val" class:warn={comp.ro5_violations != null && comp.ro5_violations > 1}>{comp.ro5_violations ?? '—'}</span>
                            </div>
                            <div class="admet-prop">
                              <span class="admet-prop-label">Affinity</span>
                              <span class="admet-prop-val">{(comp.affinity ?? 0).toFixed(2)} kcal/mol</span>
                            </div>
                          </div>
                          {#if comp.admet}
                            <div class="admet-flags-detail">
                              <span class="admet-flag" class:pass={comp.admet.lipinski} class:fail={!comp.admet.lipinski}>Lipinski {comp.admet.lipinski ? 'PASS' : 'FAIL'}</span>
                              <span class="admet-flag" class:pass={comp.admet.veber} class:fail={!comp.admet.veber}>Veber {comp.admet.veber ? 'PASS' : 'FAIL'}</span>
                              <span class="admet-flag" class:pass={comp.admet.lead_like} class:neutral={!comp.admet.lead_like}>Lead-like {comp.admet.lead_like ? 'YES' : 'NO'}</span>
                              <span class="admet-flag" class:pass={comp.admet.good_qed} class:neutral={!comp.admet.good_qed}>QED {comp.admet.good_qed ? 'GOOD' : 'LOW'}</span>
                              {#if comp.admet.p450_risk}<span class="admet-flag warn">CYP Metabolism Risk</span>{/if}
                              {#if comp.admet.high_psa}<span class="admet-flag fail">High PSA (poor absorption)</span>{/if}
                            </div>
                          {/if}
                        </div>
                      </td>
                    </tr>
                  {/if}
                {/each}
              </tbody>
            </table>
          </div>

          <!-- ADMET Summary -->
          {@const admetCompounds = fpCompounds.filter(c => c.admet)}
          {#if admetCompounds.length === 0}
            <p class="empty">ADMET properties unavailable (compounds not in ChEMBL).</p>
          {:else}
            <div class="admet-summary">
              <p class="section-label">ADMET Summary (Top {admetCompounds.length})</p>
              <div class="admet-stats">
                <div class="admet-stat">
                  <span class="admet-stat-val">{admetCompounds.filter(c => c.admet?.lipinski).length}</span>
                  <span class="admet-stat-lbl">Lipinski Pass</span>
                </div>
                <div class="admet-stat">
                  <span class="admet-stat-val">{admetCompounds.filter(c => c.admet?.veber).length}</span>
                  <span class="admet-stat-lbl">Veber Pass</span>
                </div>
                <div class="admet-stat">
                  <span class="admet-stat-val">{admetCompounds.filter(c => c.admet?.lead_like).length}</span>
                  <span class="admet-stat-lbl">Lead-Like</span>
                </div>
                <div class="admet-stat">
                  <span class="admet-stat-val">{admetCompounds.filter(c => c.admet?.good_qed).length}</span>
                  <span class="admet-stat-lbl">Good QED</span>
                </div>
                <div class="admet-stat warn">
                  <span class="admet-stat-val">{admetCompounds.filter(c => c.admet?.p450_risk).length}</span>
                  <span class="admet-stat-lbl">CYP Risk</span>
                </div>
                <div class="admet-stat warn">
                  <span class="admet-stat-val">{admetCompounds.filter(c => c.admet?.high_psa).length}</span>
                  <span class="admet-stat-lbl">High PSA</span>
                </div>
              </div>
            </div>
          {/if}

          <button class="export-btn" onclick={exportCSV}>Export CSV</button>
        {/if}
      </Panel>
    {/if}

  {:else if jobLoading}
    <Panel title="Loading">
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

  .job-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    width: 100%;
    padding: 6px 8px;
    background: rgba(0,0,0,0.15);
    border: 1px solid transparent;
    border-radius: 4px;
    cursor: pointer;
    text-align: left;
    margin-bottom: 2px;
    transition: all 0.15s;
  }

  .job-row:hover { background: rgba(255,255,255,0.05); border-color: rgba(48,54,61,0.6); }

  .job-row-name {
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    color: var(--text-primary, #e6edf3);
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .viewing-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  .back-btn {
    background: none;
    border: 1px solid rgba(48,54,61,0.6);
    color: var(--text-secondary, #8b949e);
    font-size: 10px;
    padding: 3px 8px;
    border-radius: 4px;
    cursor: pointer;
  }

  .back-btn:hover { color: var(--text-primary, #e6edf3); }

  .viewing-affinity {
    font-size: 13px;
    font-weight: 700;
    color: var(--accent, #58a6ff);
  }

  .section-label {
    font-size: 10px;
    font-weight: 600;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.3px;
    margin: 8px 0 4px;
  }

  .surface-section {
    border-top: 1px solid rgba(48,54,61,0.3);
    padding-top: 6px;
    margin-top: 6px;
  }

  .surface-btns {
    display: flex;
    gap: 4px;
  }

  .surface-btn {
    font-size: 10px;
    font-weight: 600;
    padding: 3px 8px;
    border-radius: 4px;
    border: 1px solid rgba(48,54,61,0.6);
    background: none;
    color: var(--text-secondary, #8b949e);
    cursor: pointer;
  }

  .surface-btn.active {
    background: rgba(63,185,80,0.15);
    border-color: #3fb950;
    color: #3fb950;
  }

  .surface-btn:hover { opacity: 0.8; }

  .adv-section {
    border-top: 1px solid rgba(48,54,61,0.2);
    margin-top: 8px;
    padding-top: 4px;
  }

  .adv-summary {
    font-size: 10px;
    color: var(--text-muted, #484f58);
    cursor: pointer;
    padding: 2px 0;
  }

  .adv-summary:hover { color: var(--text-secondary, #8b949e); }

  .surface-toggle {
    font-size: 10px;
    font-weight: 600;
    padding: 3px 8px;
    border-radius: 4px;
    border: 1px solid rgba(63,185,80,0.3);
    background: none;
    color: #3fb950;
    cursor: pointer;
    white-space: nowrap;
  }

  .surface-toggle.active {
    background: rgba(63,185,80,0.15);
    border-color: #3fb950;
  }

  .surface-toggle:hover { opacity: 0.8; }

  .net-toggle {
    width: 100%;
    background: rgba(88,166,255,0.08);
    border: 1px solid rgba(88,166,255,0.2);
    color: var(--accent, #58a6ff);
    font-size: 11px;
    font-weight: 600;
    padding: 5px 10px;
    border-radius: 4px;
    cursor: pointer;
    margin-bottom: 4px;
  }

  .net-toggle:hover { background: rgba(88,166,255,0.15); }

  /* ---- Pagination ---- */
  .pagination {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 8px 0 2px;
  }

  .page-btn {
    background: none;
    border: 1px solid rgba(48,54,61,0.6);
    color: var(--text-secondary, #8b949e);
    font-size: 11px;
    padding: 3px 10px;
    border-radius: 4px;
    cursor: pointer;
  }

  .page-btn:hover:not(:disabled) { color: var(--text-primary, #e6edf3); border-color: var(--accent, #58a6ff); }
  .page-btn:disabled { opacity: 0.3; cursor: not-allowed; }

  .page-info {
    font-size: 11px;
    color: var(--text-secondary, #8b949e);
  }

  .page-size-select {
    background: rgba(0,0,0,0.3);
    border: 1px solid rgba(48,54,61,0.6);
    color: var(--text-primary, #e6edf3);
    font-size: 11px;
    padding: 2px 4px;
    border-radius: 4px;
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

  .influence-bar {
    height: 8px;
    background: linear-gradient(90deg, #d29922, #f85149);
    border-radius: 4px;
    min-width: 2px;
  }

  .freq-secondary {
    color: var(--text-muted, #484f58);
    font-size: 9px;
  }

  /* ---- ADMET Detail (expanded compound row) ---- */
  .admet-detail-row td {
    padding: 0 !important;
    border-bottom: 1px solid rgba(48,54,61,0.4);
  }

  .admet-detail {
    padding: 8px;
    background: rgba(0,0,0,0.2);
    border-top: 1px solid rgba(48,54,61,0.3);
  }

  .admet-detail-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 6px;
  }

  .admet-detail-title {
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    font-weight: 700;
    color: var(--accent, #58a6ff);
  }

  .admet-props-grid {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 4px;
    margin-bottom: 6px;
  }

  .admet-prop {
    display: flex;
    flex-direction: column;
    padding: 3px 6px;
    background: rgba(0,0,0,0.15);
    border-radius: 4px;
  }

  .admet-prop-label {
    font-size: 8px;
    font-weight: 600;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.3px;
  }

  .admet-prop-val {
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    color: var(--text-primary, #e6edf3);
  }

  .admet-prop-val.warn {
    color: #d29922;
  }

  .admet-prop-val.good {
    color: #3fb950;
  }

  .admet-flags-detail {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  .admet-flag {
    font-size: 9px;
    font-weight: 600;
    padding: 2px 6px;
    border-radius: 4px;
  }

  .admet-flag.pass {
    background: rgba(63,185,80,0.15);
    color: #3fb950;
  }

  .admet-flag.fail {
    background: rgba(248,81,73,0.1);
    color: #f85149;
  }

  .admet-flag.warn {
    background: rgba(210,153,34,0.15);
    color: #d29922;
  }

  .admet-flag.neutral {
    background: rgba(48,54,61,0.3);
    color: var(--text-muted, #484f58);
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

  .topn-row {
    display: flex;
    gap: 6px;
    align-items: stretch;
  }

  .topn-select {
    background: rgba(0,0,0,0.3);
    border: 1px solid rgba(48,54,61,0.6);
    color: var(--text-primary, #e6edf3);
    font-size: 11px;
    padding: 4px 6px;
    border-radius: 6px;
    outline: none;
  }

  .topn-row .analyze-btn {
    flex: 1;
  }

  .export-btn {
    width: 100%;
    margin-top: 6px;
    background: none;
    border: 1px solid rgba(48,54,61,0.6);
    color: var(--text-secondary, #8b949e);
    font-size: 10px;
    font-weight: 600;
    padding: 4px 10px;
    border-radius: 4px;
    cursor: pointer;
  }

  .export-btn:hover {
    color: var(--text-primary, #e6edf3);
    border-color: var(--accent, #58a6ff);
  }

  .empty-row {
    text-align: center;
    color: var(--text-muted, #484f58);
    font-size: 12px;
    padding: 16px 8px !important;
  }

  /* ---- ADMET pills & summary ---- */
  .admet-cell {
    display: flex;
    gap: 2px;
    flex-wrap: wrap;
  }

  .admet-pill {
    font-size: 8px;
    font-weight: 700;
    padding: 1px 4px;
    border-radius: 4px;
    white-space: nowrap;
    line-height: 1.3;
  }

  .admet-pill.pass {
    background: rgba(63,185,80,0.15);
    color: #3fb950;
  }

  .admet-pill.fail {
    background: rgba(248,81,73,0.1);
    color: #f85149;
  }

  .admet-pill.warn {
    background: rgba(210,153,34,0.15);
    color: #d29922;
  }

  .admet-summary {
    border-top: 1px solid rgba(48,54,61,0.3);
    padding-top: 6px;
    margin-top: 6px;
  }

  .admet-stats {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 4px;
  }

  .admet-stat {
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 4px;
    background: rgba(63,185,80,0.06);
    border-radius: 4px;
  }

  .admet-stat.warn {
    background: rgba(210,153,34,0.06);
  }

  .admet-stat-val {
    font-size: 14px;
    font-weight: 700;
    color: var(--text-primary, #e6edf3);
    font-family: 'SF Mono', monospace;
  }

  .admet-stat-lbl {
    font-size: 9px;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.2px;
  }
</style>
