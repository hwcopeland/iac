<script lang="ts">
  import Panel from './Panel.svelte';
  import PlotlyChart from './charts/PlotlyChart.svelte';
  import { searchLigands, importFromFilter } from '$lib/api';
  import type { Compound } from '$lib/api';

  let { onImported = (_db: string) => {} }: { onImported?: (db: string) => void } = $props();

  // Filter state
  let mwMin = $state('100');
  let mwMax = $state('500');
  let logpMin = $state('-2');
  let logpMax = $state('5');
  let hbdMax = $state('5');
  let hbaMax = $state('10');
  let maxPhase = $state('');
  let ro5Only = $state(false);
  let textQuery = $state('');

  // Data state
  let compounds = $state<Compound[]>([]);
  let total = $state(0);
  let loading = $state(false);
  let error = $state('');

  // Import state
  let importDbName = $state('');
  let importing = $state(false);
  let importMsg = $state('');
  let importError = $state('');

  // Debounce timer
  let debounceTimer = $state<ReturnType<typeof setTimeout> | null>(null);

  function buildParams(): Record<string, string> {
    const params: Record<string, string> = {
      limit: '2000',
      offset: '0',
    };
    if (textQuery.trim()) params.q = textQuery.trim();
    if (mwMin) params.mw_min = mwMin;
    if (mwMax) params.mw_max = mwMax;
    if (logpMin) params.logp_min = logpMin;
    if (logpMax) params.logp_max = logpMax;
    if (hbdMax) params.hbd_max = hbdMax;
    if (hbaMax) params.hba_max = hbaMax;
    if (maxPhase) params.max_phase = maxPhase;
    if (ro5Only) params.ro5 = 'true';
    return params;
  }

  // Reactive derived that captures all filter values as a single string key.
  // When any filter changes, this string changes, triggering the effect below.
  let filterKey = $derived(
    `${mwMin}|${mwMax}|${logpMin}|${logpMax}|${hbdMax}|${hbaMax}|${maxPhase}|${ro5Only}|${textQuery}`
  );

  $effect(() => {
    // Read filterKey to establish reactive dependency on all filters
    const _key = filterKey;
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => doSearch(), 500);
    return () => {
      if (debounceTimer) clearTimeout(debounceTimer);
    };
  });

  async function doSearch() {
    loading = true;
    error = '';
    try {
      const res = await searchLigands(buildParams());
      compounds = res.compounds;
      total = res.total;
    } catch (e: any) {
      error = e.message || 'Search failed';
      compounds = [];
      total = 0;
    } finally {
      loading = false;
    }
  }

  // Phase color mapping
  const phaseColors: Record<number, string> = {
    0: '#484f58',
    1: '#f85149',
    2: '#e3822a',
    3: '#d29922',
    4: '#3fb950',
  };

  const phaseLabels: Record<number, string> = {
    0: 'Preclinical',
    1: 'Phase I',
    2: 'Phase II',
    3: 'Phase III',
    4: 'Approved',
  };

  function truncate(s: string, max: number): string {
    if (!s) return '';
    return s.length > max ? s.slice(0, max) + '\u2026' : s;
  }

  // Plotly chart data - scatter plot colored by phase
  let chartData = $derived.by(() => {
    if (compounds.length === 0) return [];

    // Group compounds by phase for separate trace per phase (enables legend)
    const groups = new Map<number, Compound[]>();
    for (const c of compounds) {
      const phase = c.max_phase ?? 0;
      if (!groups.has(phase)) groups.set(phase, []);
      groups.get(phase)!.push(c);
    }

    const traces: any[] = [];
    for (const [phase, comps] of [...groups.entries()].sort((a, b) => a[0] - b[0])) {
      traces.push({
        x: comps.map(c => c.mw),
        y: comps.map(c => c.logp),
        text: comps.map(c =>
          `<b>${c.chembl_id}</b><br>` +
          `${c.pref_name || '(unnamed)'}<br>` +
          `MW: ${c.mw?.toFixed(1)} | LogP: ${c.logp?.toFixed(2)}<br>` +
          `SMILES: ${truncate(c.smiles, 40)}`
        ),
        hoverinfo: 'text',
        type: 'scattergl' as const,
        mode: 'markers' as const,
        marker: {
          color: phaseColors[phase] ?? '#484f58',
          size: 5,
          opacity: 0.75,
          line: { width: 0 },
        },
        name: phaseLabels[phase] ?? `Phase ${phase}`,
      });
    }

    return traces;
  });

  // Plotly layout with Ro5 boundary box
  let chartLayout = $derived.by(() => ({
    height: 300,
    xaxis: {
      title: { text: 'Molecular Weight', font: { size: 11, color: '#8b949e' } },
      gridcolor: 'rgba(48,54,61,0.4)',
      zerolinecolor: 'rgba(48,54,61,0.6)',
      color: '#8b949e',
    },
    yaxis: {
      title: { text: 'LogP', font: { size: 11, color: '#8b949e' } },
      gridcolor: 'rgba(48,54,61,0.4)',
      zerolinecolor: 'rgba(48,54,61,0.6)',
      color: '#8b949e',
    },
    paper_bgcolor: '#0d1117',
    plot_bgcolor: 'rgba(13,17,23,0.8)',
    font: { color: '#8b949e', family: 'SF Mono, monospace', size: 10 },
    margin: { l: 55, r: 12, t: 10, b: 45 },
    showlegend: true,
    legend: {
      orientation: 'h' as const,
      x: 0,
      y: 1.12,
      font: { size: 10, color: '#8b949e' },
      bgcolor: 'rgba(0,0,0,0)',
    },
    // Ro5 boundary lines: MW <= 500, LogP <= 5
    shapes: [
      // Vertical line at MW = 500
      {
        type: 'line',
        x0: 500, x1: 500,
        y0: 0, y1: 1,
        yref: 'paper',
        line: { color: 'rgba(88,166,255,0.35)', width: 1.5, dash: 'dash' },
      },
      // Horizontal line at LogP = 5
      {
        type: 'line',
        x0: 0, x1: 1,
        xref: 'paper',
        y0: 5, y1: 5,
        line: { color: 'rgba(88,166,255,0.35)', width: 1.5, dash: 'dash' },
      },
    ],
    annotations: [
      {
        x: 500,
        y: 1,
        yref: 'paper',
        text: 'MW 500',
        showarrow: false,
        font: { size: 9, color: 'rgba(88,166,255,0.6)', family: 'SF Mono, monospace' },
        xanchor: 'left',
        yanchor: 'top',
        xshift: 3,
      },
      {
        x: 1,
        xref: 'paper',
        y: 5,
        text: 'LogP 5',
        showarrow: false,
        font: { size: 9, color: 'rgba(88,166,255,0.6)', family: 'SF Mono, monospace' },
        xanchor: 'right',
        yanchor: 'bottom',
        yshift: 3,
      },
    ],
  }));

  let chartConfig = { responsive: true, displayModeBar: false };

  // Import handler
  async function handleImport() {
    if (!importDbName.trim()) return;
    importing = true;
    importError = '';
    importMsg = '';
    try {
      const params = buildParams();
      params.source_db = importDbName.trim();
      // Remove limit/offset -- import all matches
      delete params.limit;
      delete params.offset;
      const res = await importFromFilter(params);
      importMsg = `Imported ${res.imported} compound${res.imported !== 1 ? 's' : ''} into "${res.source_db}"`;
      importDbName = '';
      onImported(res.source_db);
    } catch (e: any) {
      importError = e.message || 'Import failed';
    } finally {
      importing = false;
    }
  }
</script>

<div class="ligand-search">
  <Panel title="Chemical Space Explorer">
    <!-- Filter bar -->
    <div class="filter-bar">
      <div class="filter-group">
        <label class="filter-label">MW</label>
        <input type="number" class="filter-input" bind:value={mwMin} placeholder="min" />
        <span class="filter-sep">&ndash;</span>
        <input type="number" class="filter-input" bind:value={mwMax} placeholder="max" />
      </div>

      <div class="filter-group">
        <label class="filter-label">LogP</label>
        <input type="number" class="filter-input" step="0.1" bind:value={logpMin} placeholder="min" />
        <span class="filter-sep">&ndash;</span>
        <input type="number" class="filter-input" step="0.1" bind:value={logpMax} placeholder="max" />
      </div>

      <div class="filter-group">
        <label class="filter-label">HBD</label>
        <input type="number" class="filter-input filter-input-narrow" bind:value={hbdMax} placeholder="max" />
      </div>

      <div class="filter-group">
        <label class="filter-label">HBA</label>
        <input type="number" class="filter-input filter-input-narrow" bind:value={hbaMax} placeholder="max" />
      </div>

      <div class="filter-group">
        <label class="filter-label">Phase</label>
        <select class="filter-select" bind:value={maxPhase}>
          <option value="">Any</option>
          <option value="0">0</option>
          <option value="1">1</option>
          <option value="2">2</option>
          <option value="3">3</option>
          <option value="4">4</option>
        </select>
      </div>

      <label class="filter-checkbox">
        <input type="checkbox" bind:checked={ro5Only} />
        Ro5
      </label>

      <div class="filter-group filter-group-search">
        <input
          type="text"
          class="filter-input filter-input-search"
          placeholder="Name or ID..."
          bind:value={textQuery}
        />
      </div>

      <span class="count-badge" class:loading>
        {#if loading}
          ...
        {:else}
          {total.toLocaleString()} compounds
        {/if}
      </span>
    </div>

    <!-- Error message -->
    {#if error}
      <p class="error-msg">{error}</p>
    {/if}

    <!-- Success message -->
    {#if importMsg}
      <p class="success-msg">{importMsg}</p>
    {/if}

    <!-- Scatter plot -->
    <div class="chart-area">
      {#if compounds.length > 0}
        <PlotlyChart data={chartData} layout={chartLayout} config={chartConfig} />
      {:else if !loading}
        <div class="chart-empty">
          <span>No compounds in current filter range</span>
        </div>
      {:else}
        <div class="chart-empty">
          <span>Loading...</span>
        </div>
      {/if}
    </div>

    <!-- Import bar (sticky bottom) -->
    <div class="import-bar">
      <span class="import-count">
        {total.toLocaleString()} compounds match
      </span>
      <input
        type="text"
        class="filter-input import-name-input"
        placeholder="Name this set..."
        bind:value={importDbName}
      />
      <button
        class="import-btn"
        onclick={handleImport}
        disabled={importing || !importDbName.trim() || total === 0}
      >
        {importing ? 'Importing...' : 'Import All'}
      </button>
    </div>

    {#if importError}
      <p class="error-msg import-error">{importError}</p>
    {/if}
  </Panel>
</div>

<style>
  .ligand-search {
    display: flex;
    flex-direction: column;
  }

  /* ---- Filter bar ---- */
  .filter-bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 10px;
    padding: 6px 0 10px;
  }

  .filter-group {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-shrink: 0;
  }

  .filter-group-search {
    flex: 1;
    min-width: 100px;
  }

  .filter-label {
    font-size: 10px;
    font-weight: 600;
    color: var(--text-secondary, #8b949e);
    text-transform: uppercase;
    letter-spacing: 0.3px;
    white-space: nowrap;
  }

  .filter-input {
    background: rgba(0, 0, 0, 0.3);
    border: 1px solid rgba(48, 54, 61, 0.6);
    color: var(--text-primary, #e6edf3);
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    padding: 4px 6px;
    border-radius: 4px;
    width: 58px;
    outline: none;
    transition: border-color 0.15s;
  }

  .filter-input:focus {
    border-color: var(--accent, #58a6ff);
  }

  .filter-input-narrow {
    width: 42px;
  }

  .filter-input-search {
    width: 100%;
  }

  .filter-select {
    background: rgba(0, 0, 0, 0.3);
    border: 1px solid rgba(48, 54, 61, 0.6);
    color: var(--text-primary, #e6edf3);
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    padding: 4px 6px;
    border-radius: 4px;
    outline: none;
    transition: border-color 0.15s;
    width: auto;
  }

  .filter-select:focus {
    border-color: var(--accent, #58a6ff);
  }

  .filter-sep {
    color: var(--text-muted, #484f58);
    font-size: 11px;
  }

  .filter-checkbox {
    display: flex;
    align-items: center;
    gap: 3px;
    font-size: 10px;
    font-weight: 600;
    color: var(--text-secondary, #8b949e);
    text-transform: uppercase;
    letter-spacing: 0.3px;
    white-space: nowrap;
    cursor: pointer;
  }

  .filter-checkbox input[type="checkbox"] {
    accent-color: var(--accent, #58a6ff);
  }

  .count-badge {
    font-size: 10px;
    font-weight: 600;
    color: var(--accent, #58a6ff);
    background: rgba(88, 166, 255, 0.1);
    padding: 3px 8px;
    border-radius: 10px;
    white-space: nowrap;
    flex-shrink: 0;
  }

  .count-badge.loading {
    opacity: 0.5;
  }

  /* ---- Chart area ---- */
  .chart-area {
    min-height: 300px;
    border-radius: 4px;
    overflow: hidden;
  }

  .chart-empty {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 300px;
    color: var(--text-muted, #484f58);
    font-size: 12px;
    background: rgba(0, 0, 0, 0.15);
    border-radius: 4px;
  }

  /* ---- Import bar ---- */
  .import-bar {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 0 2px;
    border-top: 1px solid rgba(48, 54, 61, 0.4);
    margin-top: 8px;
    position: sticky;
    bottom: 0;
    background: var(--bg-surface, rgba(22, 27, 34, 0.95));
    z-index: 1;
  }

  .import-count {
    font-size: 11px;
    color: var(--text-secondary, #8b949e);
    white-space: nowrap;
    flex-shrink: 0;
  }

  .import-name-input {
    flex: 1;
    min-width: 120px;
  }

  .import-btn {
    background: var(--accent, #58a6ff);
    border: none;
    color: #000;
    font-size: 11px;
    font-weight: 600;
    padding: 5px 14px;
    border-radius: 4px;
    cursor: pointer;
    white-space: nowrap;
    flex-shrink: 0;
    transition: opacity 0.15s;
  }

  .import-btn:hover:not(:disabled) {
    opacity: 0.9;
  }

  .import-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  /* ---- Messages ---- */
  .success-msg {
    color: #3fb950;
    font-size: 11px;
    margin: 4px 0;
  }

  .error-msg {
    color: var(--danger, #f85149);
    font-size: 11px;
    margin: 4px 0;
  }

  .import-error {
    margin-top: 2px;
  }
</style>
