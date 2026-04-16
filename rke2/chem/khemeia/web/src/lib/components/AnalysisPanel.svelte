<script lang="ts">
  import Panel from './Panel.svelte';
  import { getJobs, getJob } from '$lib/api';
  import { loadPdb, overlayStructure } from '$lib/viewer';
  import { isAuthenticated } from '$lib/auth';

  let jobs = $state<any[]>([]);
  let jobsLoading = $state(true);
  let jobsError = $state('');
  let selectedJobName = $state<string | null>(null);
  let selectedJob = $state<any | null>(null);
  let jobLoading = $state(false);
  let viewingCompound = $state<string | null>(null);
  let viewError = $state('');

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

  async function handleView(result: any) {
    viewError = '';
    viewingCompound = result.compound_id;
    try {
      // Load the target protein
      const pdbid = selectedJob?.input_data?.pdbid;
      if (pdbid) {
        await loadPdb(pdbid);
      }
      // Overlay the docked ligand pose
      if (result.pose_pdbqt) {
        await overlayStructure(result.pose_pdbqt, 'pdbqt');
      }
    } catch (e: any) {
      viewError = e.message || 'Failed to load structure';
    }
  }

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

  .col-affinity {
    text-align: right;
    width: 110px;
    font-weight: 600;
  }

  th.col-affinity {
    text-align: right;
  }

  .col-action {
    width: 52px;
    text-align: center;
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
</style>
