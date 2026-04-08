<script lang="ts">
  let { job, pluginSlug }: { job: any; pluginSlug: string } = $props();

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

  /** Filter output_data to scalar fields only */
  let scalarFields = $derived.by(() => {
    const data = job?.output_data;
    if (!data || typeof data !== 'object') return [];
    return Object.entries(data).filter(([key, value]) => {
      if (SKIP_FIELDS.has(key)) return false;
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
  {:else if !job.error_output}
    <p class="no-data">No output data available.</p>
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

  .no-data {
    font-size: 12px;
    color: var(--text-muted, #484f58);
    text-align: center;
    padding: 12px 0;
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
