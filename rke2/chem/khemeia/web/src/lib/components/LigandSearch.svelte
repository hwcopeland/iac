<script lang="ts">
  import { onMount } from 'svelte';
  import Panel from './Panel.svelte';
  import { searchLigands, importFromFilter } from '$lib/api';

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

  // Results state
  let total = $state<number | null>(null);
  let loading = $state(false);
  let error = $state('');
  let searched = $state(false);

  // Import state
  let importDbName = $state('');
  let importing = $state(false);
  let importMsg = $state('');
  let importError = $state('');

  function buildParams(): Record<string, string> {
    const params: Record<string, string> = {
      limit: '0',
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

  async function doSearch() {
    loading = true;
    error = '';
    try {
      const res = await searchLigands(buildParams());
      total = res.total;
      searched = true;
    } catch (e: any) {
      error = e.message || 'Search failed';
      total = null;
    } finally {
      loading = false;
    }
  }

  function buildTypedParams(): Record<string, any> {
    const p: Record<string, any> = {};
    if (textQuery.trim()) p.q = textQuery.trim();
    if (mwMin) p.mw_min = parseFloat(mwMin);
    if (mwMax) p.mw_max = parseFloat(mwMax);
    if (logpMin) p.logp_min = parseFloat(logpMin);
    if (logpMax) p.logp_max = parseFloat(logpMax);
    if (hbdMax) p.hbd_max = parseInt(hbdMax);
    if (hbaMax) p.hba_max = parseInt(hbaMax);
    if (maxPhase) p.max_phase = parseFloat(maxPhase);
    if (ro5Only) p.ro5 = true;
    return p;
  }

  async function handleImport() {
    if (!importDbName.trim() || !total) return;
    importing = true;
    importError = '';
    importMsg = '';
    try {
      const params = buildTypedParams();
      params.source_db = importDbName.trim();
      const res = await importFromFilter(params);
      importMsg = `Imported ${res.imported.toLocaleString()} compounds into "${res.source_db}"`;
      importDbName = '';
      onImported(res.source_db);
    } catch (e: any) {
      importError = e.message || 'Import failed';
    } finally {
      importing = false;
    }
  }

  // Search on mount
  onMount(() => { doSearch(); });
</script>

<div class="ligand-search">
  <Panel title="ChEMBL Compound Filter">
    <div class="filters">
      <div class="filter-row">
        <label class="filter-label">MW</label>
        <input type="number" class="filter-input" bind:value={mwMin} placeholder="min" />
        <span class="sep">&ndash;</span>
        <input type="number" class="filter-input" bind:value={mwMax} placeholder="max" />
      </div>
      <div class="filter-row">
        <label class="filter-label">LogP</label>
        <input type="number" class="filter-input" step="0.1" bind:value={logpMin} placeholder="min" />
        <span class="sep">&ndash;</span>
        <input type="number" class="filter-input" step="0.1" bind:value={logpMax} placeholder="max" />
      </div>
      <div class="filter-row">
        <label class="filter-label">HBD</label>
        <input type="number" class="filter-input small" bind:value={hbdMax} placeholder="max" />
        <label class="filter-label">HBA</label>
        <input type="number" class="filter-input small" bind:value={hbaMax} placeholder="max" />
      </div>
      <div class="filter-row">
        <label class="filter-label">Phase</label>
        <select class="filter-select" bind:value={maxPhase}>
          <option value="">Any</option>
          <option value="0">0</option>
          <option value="1">1</option>
          <option value="2">2</option>
          <option value="3">3</option>
          <option value="4">4 (Approved)</option>
        </select>
        <label class="check-label">
          <input type="checkbox" bind:checked={ro5Only} />
          Ro5
        </label>
      </div>
      <div class="filter-row">
        <input
          type="text"
          class="filter-input search-input"
          placeholder="Search name or ChEMBL ID..."
          bind:value={textQuery}
        />
      </div>

      <button class="search-btn" onclick={doSearch} disabled={loading}>
        {loading ? 'Searching...' : 'Search'}
      </button>
    </div>

    {#if error}
      <p class="msg error">{error}</p>
    {/if}

    {#if searched && total !== null}
      <div class="result-box">
        <span class="result-count">{total.toLocaleString()}</span>
        <span class="result-label">compounds found</span>
      </div>

      {#if total > 0}
        <div class="import-section">
          <input
            type="text"
            class="filter-input import-input"
            placeholder="Name this ligand set..."
            bind:value={importDbName}
          />
          <button
            class="import-btn"
            onclick={handleImport}
            disabled={importing || !importDbName.trim()}
          >
            {importing ? 'Importing...' : `Import ${total.toLocaleString()} compounds`}
          </button>
        </div>
      {/if}
    {/if}

    {#if importMsg}
      <p class="msg success">{importMsg}</p>
    {/if}
    {#if importError}
      <p class="msg error">{importError}</p>
    {/if}
  </Panel>
</div>

<style>
  .ligand-search {
    display: flex;
    flex-direction: column;
  }

  .filters {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .filter-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .filter-label {
    font-size: 10px;
    font-weight: 600;
    color: var(--text-secondary, #8b949e);
    text-transform: uppercase;
    letter-spacing: 0.3px;
    min-width: 32px;
  }

  .filter-input {
    background: rgba(0, 0, 0, 0.3);
    border: 1px solid rgba(48, 54, 61, 0.6);
    color: var(--text-primary, #e6edf3);
    font-family: 'SF Mono', monospace;
    font-size: 12px;
    padding: 5px 8px;
    border-radius: 4px;
    width: 70px;
    outline: none;
  }

  .filter-input:focus {
    border-color: var(--accent, #58a6ff);
  }

  .filter-input.small {
    width: 50px;
  }

  .search-input {
    width: 100%;
    flex: 1;
  }

  .filter-select {
    background: rgba(0, 0, 0, 0.3);
    border: 1px solid rgba(48, 54, 61, 0.6);
    color: var(--text-primary, #e6edf3);
    font-size: 12px;
    padding: 5px 8px;
    border-radius: 4px;
    outline: none;
  }

  .filter-select:focus {
    border-color: var(--accent, #58a6ff);
  }

  .sep {
    color: var(--text-muted, #484f58);
  }

  .check-label {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 10px;
    font-weight: 600;
    color: var(--text-secondary, #8b949e);
    text-transform: uppercase;
    cursor: pointer;
    margin-left: 8px;
  }

  .check-label input {
    accent-color: var(--accent, #58a6ff);
  }

  .search-btn {
    background: var(--accent, #58a6ff);
    border: none;
    color: #000;
    font-size: 13px;
    font-weight: 600;
    padding: 8px 12px;
    border-radius: 6px;
    cursor: pointer;
    width: 100%;
    margin-top: 4px;
  }

  .search-btn:hover:not(:disabled) { opacity: 0.9; }
  .search-btn:disabled { opacity: 0.4; cursor: not-allowed; }

  .result-box {
    display: flex;
    align-items: baseline;
    gap: 8px;
    padding: 12px 0;
    border-bottom: 1px solid rgba(48, 54, 61, 0.4);
    margin-top: 8px;
  }

  .result-count {
    font-family: 'SF Mono', monospace;
    font-size: 24px;
    font-weight: 700;
    color: var(--accent, #58a6ff);
  }

  .result-label {
    font-size: 13px;
    color: var(--text-secondary, #8b949e);
  }

  .import-section {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 10px 0;
  }

  .import-input {
    width: 100%;
  }

  .import-btn {
    background: #3fb950;
    border: none;
    color: #000;
    font-size: 13px;
    font-weight: 600;
    padding: 8px 12px;
    border-radius: 6px;
    cursor: pointer;
    width: 100%;
  }

  .import-btn:hover:not(:disabled) { opacity: 0.9; }
  .import-btn:disabled { opacity: 0.4; cursor: not-allowed; }

  .msg {
    font-size: 12px;
    margin: 8px 0 0;
  }

  .msg.success { color: #3fb950; }
  .msg.error { color: #f85149; }
</style>
