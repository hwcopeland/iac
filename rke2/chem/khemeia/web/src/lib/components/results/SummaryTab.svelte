<script lang="ts">
  let { job, pluginSlug, onView3D }: { job: any; pluginSlug: string; onView3D?: () => void } = $props();

  /** Fields to skip in the summary table (arrays, text blobs, raw output) */
  const SKIP_FIELDS = new Set([
    'log', 'output_file', 'scf_energies', 'ir_frequencies', 'ir_intensities',
    'raman_frequencies', 'raman_intensities', 'nmr_shifts', 'nmr_elements',
    'trajectory', 'coordinates', 'atoms', 'orbitals',
  ]);

  /** Convert snake_case field name to a readable label */
  function humanize(key: string): string {
    return key.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
  }

  /** Format a value for display */
  function formatValue(value: any): string {
    if (typeof value === 'number') {
      return Number.isInteger(value) ? value.toString() : value.toFixed(6);
    }
    if (typeof value === 'boolean') return value ? 'Yes' : 'No';
    return String(value);
  }

  /** Fields that appear in the DFRATOM energy decomposition table instead of the generic table */
  const DFRATOM_ENERGY_FIELDS = new Set([
    'total_energy', 'rest_mass_energy', 'kinetic_energy', 'potential_energy', 'virial_ratio',
  ]);

  /** DFRATOM energy decomposition rows: [display label, field key, unit] */
  const DFRATOM_ENERGY_ROWS: [string, string, string][] = [
    ['Total Energy', 'total_energy', 'Ha'],
    ['Rest Mass Energy', 'rest_mass_energy', 'Ha'],
    ['Kinetic Energy <T>', 'kinetic_energy', 'Ha'],
    ['Potential Energy <V>', 'potential_energy', 'Ha'],
    ['Virial Ratio', 'virial_ratio', ''],
  ];

  /** Whether this job has DFRATOM energy decomposition data */
  let hasDfratomEnergies = $derived(
    pluginSlug === 'dfratom' &&
    job?.output_data &&
    (job.output_data.kinetic_energy != null || job.output_data.potential_energy != null)
  );

  /** Format an energy value: parse scientific notation string, display with full precision */
  function formatEnergy(value: any): string {
    if (value == null) return '--';
    const s = String(value);
    const n = Number(s);
    if (isNaN(n)) return s;
    // Show 10 significant digits for Hartree energies
    return n.toPrecision(10);
  }

  /** Filter output_data to scalar fields only */
  let scalarFields = $derived.by(() => {
    const data = job?.output_data;
    if (!data || typeof data !== 'object') return [];
    return Object.entries(data).filter(([key, value]) => {
      if (SKIP_FIELDS.has(key)) return false;
      // Hide energy fields from generic table when shown in DFRATOM decomposition
      if (hasDfratomEnergies && DFRATOM_ENERGY_FIELDS.has(key)) return false;
      if (Array.isArray(value)) return false;
      if (typeof value === 'object' && value !== null) return false;
      if (typeof value === 'string' && value.length > 200) return false;
      return true;
    });
  });

  /** Compute elapsed time string */
  let elapsed = $derived.by(() => {
    if (!job?.created_at) return null;
    const start = new Date(job.created_at);
    const end = job.completed_at ? new Date(job.completed_at) : null;
    if (!end) return null;
    const seconds = Math.round((end.getTime() - start.getTime()) / 1000);
    if (seconds < 60) return `${seconds}s`;
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    if (mins < 60) return `${mins}m ${secs}s`;
    const hrs = Math.floor(mins / 60);
    return `${hrs}h ${mins % 60}m`;
  });

  function statusClass(status: string): string {
    const s = status?.toLowerCase();
    if (s === 'completed') return 'completed';
    if (s === 'failed') return 'failed';
    if (s === 'running') return 'running';
    return 'pending';
  }
</script>

<div class="summary-tab">
  <div class="summary-meta">
    <div class="meta-row">
      <span class="meta-label">Job</span>
      <span class="meta-value mono">{job.name}</span>
    </div>
    <div class="meta-row">
      <span class="meta-label">Status</span>
      <span class="status-badge {statusClass(job.status)}">{job.status}</span>
    </div>
    {#if job.created_at}
      <div class="meta-row">
        <span class="meta-label">Submitted</span>
        <span class="meta-value">{new Date(job.created_at).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}</span>
      </div>
    {/if}
    {#if elapsed}
      <div class="meta-row">
        <span class="meta-label">Duration</span>
        <span class="meta-value mono">{elapsed}</span>
      </div>
    {/if}
    <div class="meta-row">
      <span class="meta-label">Plugin</span>
      <span class="meta-value">{pluginSlug}</span>
    </div>
  </div>

  {#if hasDfratomEnergies}
    <div class="energy-decomposition">
      <div class="table-header">
        <span>Energy Decomposition</span>
        <span>Value</span>
      </div>
      {#each DFRATOM_ENERGY_ROWS as [label, key, unit], i}
        {#if job.output_data[key] != null}
          <div class="table-row" class:alt={i % 2 === 1} class:virial={key === 'virial_ratio'}>
            <span class="row-label">{label}</span>
            <span class="row-value">
              {formatEnergy(job.output_data[key])}{#if unit}&nbsp;{unit}{/if}
            </span>
          </div>
        {/if}
      {/each}
    </div>
  {/if}

  {#if scalarFields.length > 0}
    <div class="summary-table">
      <div class="table-header">
        <span>Property</span>
        <span>Value</span>
      </div>
      {#each scalarFields as [key, value], i}
        <div class="table-row" class:alt={i % 2 === 1}>
          <span class="row-label">{humanize(key)}</span>
          <span class="row-value">{formatValue(value)}</span>
        </div>
      {/each}
    </div>
  {:else if !job.error_output && !hasDfratomEnergies}
    <p class="no-data">No output data available.</p>
  {/if}

  {#if job.docking_results?.length}
    <div class="docking-results">
      <div class="docking-header">
        <h4 class="docking-title">Docking Results</h4>
        <span class="docking-count">{job.docking_results.length} compounds</span>
      </div>
      <div class="results-table-wrap">
        <table class="results-table">
          <thead>
            <tr>
              <th class="col-rank">#</th>
              <th class="col-compound">Compound</th>
              <th class="col-affinity">Affinity (kcal/mol)</th>
            </tr>
          </thead>
          <tbody>
            {#each job.docking_results as result, i}
              <tr class:top-hit={i < 3} class:alt={i % 2 === 1}>
                <td class="col-rank">{i + 1}</td>
                <td class="col-compound mono">{result.compound_id}</td>
                <td class="col-affinity mono">{result.affinity_kcal_mol.toFixed(2)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}

  {#if onView3D && job.status?.toLowerCase() === 'completed'}
    <button class="view-3d-btn" onclick={onView3D}>
      View Molecule & Artifacts
    </button>
  {/if}

  {#if job.error_output}
    <div class="error-box">
      <p class="error-label">Error Output</p>
      <pre class="error-pre">{job.error_output}</pre>
    </div>
  {/if}
</div>

<style>
  .summary-tab {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .summary-meta {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .meta-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 3px 0;
  }

  .meta-label {
    font-size: 11px;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.3px;
    font-weight: 600;
  }

  .meta-value {
    font-size: 12px;
    color: var(--text-primary, #e6edf3);
  }

  .meta-value.mono {
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

  .summary-table {
    display: flex;
    flex-direction: column;
    border: 1px solid rgba(48,54,61,0.4);
    border-radius: 6px;
    overflow: hidden;
  }

  .table-header {
    display: flex;
    justify-content: space-between;
    padding: 6px 10px;
    background: rgba(0,0,0,0.3);
    font-size: 10px;
    font-weight: 600;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.3px;
  }

  .table-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 5px 10px;
    transition: background 0.1s;
  }

  .table-row:hover {
    background: rgba(255,255,255,0.03);
  }

  .table-row.alt {
    background: rgba(0,0,0,0.15);
  }

  .table-row.alt:hover {
    background: rgba(0,0,0,0.25);
  }

  .row-label {
    font-size: 11px;
    color: var(--text-secondary, #8b949e);
  }

  .row-value {
    font-family: 'SF Mono', monospace;
    font-size: 12px;
    font-weight: 600;
    color: var(--text-primary, #e6edf3);
  }

  .energy-decomposition {
    display: flex;
    flex-direction: column;
    border: 1px solid rgba(88,166,255,0.2);
    border-radius: 6px;
    overflow: hidden;
    background: rgba(88,166,255,0.03);
  }

  .table-row.virial {
    border-top: 1px solid rgba(48,54,61,0.4);
  }

  .no-data {
    font-size: 12px;
    color: var(--text-muted, #484f58);
    text-align: center;
    padding: 12px 0;
  }

  .docking-results {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .docking-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .docking-title {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-secondary, #8b949e);
    text-transform: uppercase;
    letter-spacing: 0.3px;
    margin: 0;
  }

  .docking-count {
    font-size: 10px;
    color: var(--text-muted, #484f58);
  }

  .results-table-wrap {
    border: 1px solid rgba(48,54,61,0.4);
    border-radius: 6px;
    overflow: hidden;
    max-height: 400px;
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
    padding: 5px 10px;
    font-size: 10px;
    font-weight: 600;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.3px;
    text-align: left;
  }

  .results-table td {
    padding: 4px 10px;
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
    width: 36px;
    text-align: center;
  }

  .col-affinity {
    text-align: right;
    width: 120px;
    font-weight: 600;
  }

  th.col-affinity {
    text-align: right;
  }

  .view-3d-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    width: 100%;
    padding: 8px 12px;
    font-size: 12px;
    font-weight: 600;
    color: var(--accent, #58a6ff);
    background: rgba(88,166,255,0.08);
    border: 1px solid rgba(88,166,255,0.2);
    border-radius: 6px;
    cursor: pointer;
    transition: all 0.15s;
  }

  .view-3d-btn:hover {
    background: rgba(88,166,255,0.15);
    border-color: rgba(88,166,255,0.4);
  }

  .error-box {
    background: rgba(248,81,73,0.05);
    border: 1px solid rgba(248,81,73,0.2);
    border-radius: 6px;
    padding: 10px;
  }

  .error-label {
    font-size: 10px;
    font-weight: 600;
    color: var(--danger, #f85149);
    text-transform: uppercase;
    margin-bottom: 6px;
  }

  .error-pre {
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    color: var(--text-secondary, #8b949e);
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 200px;
    overflow-y: auto;
    margin: 0;
  }
</style>
