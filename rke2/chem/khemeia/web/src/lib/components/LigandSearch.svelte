<script lang="ts">
  import Panel from './Panel.svelte';
  import { searchLigands, importFromChEMBL } from '$lib/api';
  import type { Compound } from '$lib/api';

  let { onImported = (_db: string) => {} }: { onImported?: (db: string) => void } = $props();

  // Search state
  let query = $state('');
  let debounceTimer = $state<ReturnType<typeof setTimeout> | null>(null);
  let loading = $state(false);
  let error = $state('');

  // Filter state
  let filtersOpen = $state(false);
  let mwMin = $state('100');
  let mwMax = $state('500');
  let logpMin = $state('-2');
  let logpMax = $state('5');
  let hbdMax = $state('5');
  let hbaMax = $state('10');
  let maxPhase = $state('');
  let ro5Only = $state(false);

  // Results state
  let compounds = $state<Compound[]>([]);
  let total = $state(0);
  let offset = $state(0);
  let limit = $state(50);

  // Selection state
  let selected = $state<Set<string>>(new Set());

  // Import state
  let showImportForm = $state(false);
  let importDbName = $state('');
  let importing = $state(false);
  let importMsg = $state('');
  let importError = $state('');

  function debouncedSearch() {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      offset = 0;
      doSearch();
    }, 300);
  }

  $effect(() => {
    return () => {
      if (debounceTimer) clearTimeout(debounceTimer);
    };
  });

  async function doSearch() {
    if (!query.trim()) {
      compounds = [];
      total = 0;
      return;
    }
    loading = true;
    error = '';
    try {
      const params: Record<string, string> = {
        q: query.trim(),
        limit: String(limit),
        offset: String(offset),
      };
      if (mwMin) params.mw_min = mwMin;
      if (mwMax) params.mw_max = mwMax;
      if (logpMin) params.logp_min = logpMin;
      if (logpMax) params.logp_max = logpMax;
      if (hbdMax) params.hbd_max = hbdMax;
      if (hbaMax) params.hba_max = hbaMax;
      if (maxPhase) params.max_phase = maxPhase;
      if (ro5Only) params.ro5_violations = '0';

      const res = await searchLigands(params);
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

  function applyFilters() {
    offset = 0;
    doSearch();
  }

  function prevPage() {
    if (offset <= 0) return;
    offset = Math.max(0, offset - limit);
    doSearch();
  }

  function nextPage() {
    if (offset + limit >= total) return;
    offset += limit;
    doSearch();
  }

  function toggleSelect(id: string) {
    const next = new Set(selected);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    selected = next;
  }

  function toggleSelectAll() {
    const pageIds = compounds.map(c => c.chembl_id);
    const allSelected = pageIds.every(id => selected.has(id));
    const next = new Set(selected);
    if (allSelected) {
      for (const id of pageIds) next.delete(id);
    } else {
      for (const id of pageIds) next.add(id);
    }
    selected = next;
  }

  function isAllPageSelected(): boolean {
    if (compounds.length === 0) return false;
    return compounds.every(c => selected.has(c.chembl_id));
  }

  async function handleImport() {
    if (!importDbName.trim() || selected.size === 0) return;
    importing = true;
    importError = '';
    importMsg = '';
    try {
      const res = await importFromChEMBL(Array.from(selected), importDbName.trim());
      importMsg = `Imported ${res.imported} compound${res.imported !== 1 ? 's' : ''} into "${res.source_db}"`;
      selected = new Set();
      showImportForm = false;
      importDbName = '';
      onImported(res.source_db);
    } catch (e: any) {
      importError = e.message || 'Import failed';
    } finally {
      importing = false;
    }
  }

  function phaseColor(phase: number): string {
    switch (phase) {
      case 4: return '#3fb950';
      case 3: return '#d29922';
      case 2: return '#e3822a';
      case 1: return '#f85149';
      default: return '#484f58';
    }
  }

  function truncate(s: string, max: number): string {
    if (!s) return '';
    return s.length > max ? s.slice(0, max) + '\u2026' : s;
  }

  let showEnd = $derived(Math.min(offset + limit, total));
</script>

<div class="ligand-search">
  <Panel title="ChEMBL Compound Search">
    <!-- Search bar -->
    <div class="search-bar">
      <input
        type="text"
        class="form-input search-input"
        placeholder="Search by name or ChEMBL ID..."
        bind:value={query}
        oninput={debouncedSearch}
      />
      {#if loading}
        <span class="search-spinner">...</span>
      {/if}
    </div>

    <!-- Filter controls -->
    <button class="filter-toggle" onclick={() => (filtersOpen = !filtersOpen)}>
      Filters {filtersOpen ? '\u25BC' : '\u25B6'}
    </button>

    {#if filtersOpen}
      <div class="filters">
        <div class="filter-row">
          <label class="filter-label">MW</label>
          <input type="number" class="filter-input" bind:value={mwMin} placeholder="min" />
          <span class="filter-sep">\u2013</span>
          <input type="number" class="filter-input" bind:value={mwMax} placeholder="max" />
        </div>
        <div class="filter-row">
          <label class="filter-label">LogP</label>
          <input type="number" class="filter-input" step="0.1" bind:value={logpMin} placeholder="min" />
          <span class="filter-sep">\u2013</span>
          <input type="number" class="filter-input" step="0.1" bind:value={logpMax} placeholder="max" />
        </div>
        <div class="filter-row">
          <label class="filter-label">HBD max</label>
          <input type="number" class="filter-input" bind:value={hbdMax} />
        </div>
        <div class="filter-row">
          <label class="filter-label">HBA max</label>
          <input type="number" class="filter-input" bind:value={hbaMax} />
        </div>
        <div class="filter-row">
          <label class="filter-label">Phase</label>
          <select class="form-select filter-select" bind:value={maxPhase}>
            <option value="">Any</option>
            <option value="0">0</option>
            <option value="1">1</option>
            <option value="2">2</option>
            <option value="3">3</option>
            <option value="4">4</option>
          </select>
        </div>
        <div class="filter-row">
          <label class="filter-label">
            <input type="checkbox" bind:checked={ro5Only} />
            Ro5 compliant
          </label>
        </div>
        <button class="apply-btn" onclick={applyFilters}>Apply Filters</button>
      </div>
    {/if}

    <!-- Status messages -->
    {#if importMsg}
      <p class="success-msg">{importMsg}</p>
    {/if}
    {#if error}
      <p class="error-msg">{error}</p>
    {/if}

    <!-- Selection controls -->
    {#if selected.size > 0}
      <div class="selection-bar">
        <span class="selection-count">{selected.size} selected</span>
        {#if !showImportForm}
          <button class="import-btn" onclick={() => (showImportForm = true)}>
            Import Selected
          </button>
        {/if}
      </div>

      {#if showImportForm}
        <div class="import-form">
          <label class="filter-label">Database name for import:</label>
          <div class="import-row">
            <input
              type="text"
              class="form-input"
              placeholder="e.g. my-aspirin-set"
              bind:value={importDbName}
            />
            <button
              class="submit-btn import-go"
              onclick={handleImport}
              disabled={importing || !importDbName.trim()}
            >
              {importing ? 'Importing...' : 'Import'}
            </button>
            <button class="cancel-btn" onclick={() => (showImportForm = false)}>
              Cancel
            </button>
          </div>
          {#if importError}
            <p class="error-msg">{importError}</p>
          {/if}
        </div>
      {/if}
    {/if}

    <!-- Results table -->
    {#if compounds.length > 0}
      <div class="results-table-wrap">
        <table class="results-table">
          <thead>
            <tr>
              <th class="col-check">
                <input type="checkbox" checked={isAllPageSelected()} onchange={toggleSelectAll} />
              </th>
              <th class="col-id">ChEMBL ID</th>
              <th class="col-name">Name</th>
              <th class="col-smiles">SMILES</th>
              <th class="col-num">MW</th>
              <th class="col-num">LogP</th>
              <th class="col-num">HBD</th>
              <th class="col-num">HBA</th>
              <th class="col-phase">Phase</th>
            </tr>
          </thead>
          <tbody>
            {#each compounds as compound}
              <tr class:row-selected={selected.has(compound.chembl_id)}>
                <td class="col-check">
                  <input
                    type="checkbox"
                    checked={selected.has(compound.chembl_id)}
                    onchange={() => toggleSelect(compound.chembl_id)}
                  />
                </td>
                <td class="col-id mono">{compound.chembl_id}</td>
                <td class="col-name" title={compound.pref_name}>{truncate(compound.pref_name, 20)}</td>
                <td class="col-smiles mono" title={compound.smiles}>{truncate(compound.smiles, 25)}</td>
                <td class="col-num">{compound.mw?.toFixed(1)}</td>
                <td class="col-num">{compound.logp?.toFixed(1)}</td>
                <td class="col-num">{compound.hbd}</td>
                <td class="col-num">{compound.hba}</td>
                <td class="col-phase">
                  <span class="phase-badge" style="background: {phaseColor(compound.max_phase)}">
                    {compound.max_phase}
                  </span>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <!-- Pagination -->
      <div class="pagination">
        <button class="page-btn" onclick={prevPage} disabled={offset === 0}>Prev</button>
        <span class="page-info">Showing {offset + 1}\u2013{showEnd} of {total}</span>
        <button class="page-btn" onclick={nextPage} disabled={offset + limit >= total}>Next</button>
      </div>
    {:else if query.trim() && !loading}
      <p class="empty-small">No compounds found.</p>
    {/if}
  </Panel>
</div>

<style>
  .ligand-search {
    display: flex;
    flex-direction: column;
  }

  .search-bar {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 6px;
  }

  .search-input {
    flex: 1;
  }

  .search-spinner {
    color: var(--text-muted, #484f58);
    font-size: 11px;
    flex-shrink: 0;
  }

  .filter-toggle {
    background: none;
    border: none;
    color: var(--text-secondary, #8b949e);
    font-size: 11px;
    cursor: pointer;
    padding: 2px 0;
    margin-bottom: 4px;
    font-family: inherit;
  }

  .filter-toggle:hover {
    color: var(--accent, #58a6ff);
  }

  .filters {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 8px;
    background: rgba(0, 0, 0, 0.2);
    border-radius: 4px;
    margin-bottom: 8px;
  }

  .filter-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .filter-label {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-secondary, #8b949e);
    min-width: 55px;
    text-transform: uppercase;
    letter-spacing: 0.3px;
  }

  .filter-input {
    background: rgba(0, 0, 0, 0.3);
    border: 1px solid rgba(48, 54, 61, 0.6);
    color: var(--text-primary, #e6edf3);
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    padding: 3px 6px;
    border-radius: 4px;
    width: 65px;
    outline: none;
  }

  .filter-input:focus {
    border-color: var(--accent, #58a6ff);
  }

  .filter-select {
    font-size: 11px;
    padding: 3px 6px;
    width: auto;
  }

  .filter-sep {
    color: var(--text-muted, #484f58);
    font-size: 11px;
  }

  .apply-btn {
    background: rgba(88, 166, 255, 0.1);
    border: 1px solid rgba(88, 166, 255, 0.3);
    color: var(--accent, #58a6ff);
    font-size: 11px;
    padding: 4px 10px;
    border-radius: 4px;
    cursor: pointer;
    align-self: flex-end;
  }

  .apply-btn:hover {
    background: rgba(88, 166, 255, 0.2);
  }

  .selection-bar {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 0;
    margin-bottom: 4px;
  }

  .selection-count {
    font-size: 11px;
    font-weight: 600;
    color: var(--accent, #58a6ff);
    background: rgba(88, 166, 255, 0.1);
    padding: 2px 8px;
    border-radius: 10px;
  }

  .import-btn {
    background: var(--accent, #58a6ff);
    border: none;
    color: #000;
    font-size: 11px;
    font-weight: 600;
    padding: 4px 10px;
    border-radius: 4px;
    cursor: pointer;
  }

  .import-btn:hover {
    opacity: 0.9;
  }

  .import-form {
    padding: 8px;
    background: rgba(0, 0, 0, 0.2);
    border-radius: 4px;
    margin-bottom: 8px;
  }

  .import-row {
    display: flex;
    gap: 6px;
    margin-top: 4px;
  }

  .import-row .form-input {
    flex: 1;
    font-size: 11px;
    padding: 4px 6px;
  }

  .import-go {
    font-size: 11px;
    padding: 4px 10px;
    width: auto;
  }

  .cancel-btn {
    background: none;
    border: 1px solid rgba(48, 54, 61, 0.6);
    color: var(--text-secondary, #8b949e);
    font-size: 11px;
    padding: 4px 8px;
    border-radius: 4px;
    cursor: pointer;
  }

  .cancel-btn:hover {
    color: var(--text-primary, #e6edf3);
  }

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

  .empty-small {
    color: var(--text-muted, #484f58);
    font-size: 12px;
    text-align: center;
    padding: 12px 0;
  }

  /* Results table */
  .results-table-wrap {
    overflow-x: auto;
    margin-top: 6px;
  }

  .results-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 11px;
  }

  .results-table th {
    text-align: left;
    font-size: 10px;
    font-weight: 600;
    color: var(--text-secondary, #8b949e);
    text-transform: uppercase;
    letter-spacing: 0.3px;
    padding: 4px 8px;
    border-bottom: 1px solid rgba(48, 54, 61, 0.6);
    white-space: nowrap;
  }

  .results-table td {
    padding: 4px 8px;
    color: var(--text-primary, #e6edf3);
    border-bottom: 1px solid rgba(48, 54, 61, 0.3);
    white-space: nowrap;
  }

  .results-table tbody tr:hover {
    background: rgba(255, 255, 255, 0.03);
  }

  .row-selected {
    background: rgba(88, 166, 255, 0.05) !important;
  }

  .col-check {
    width: 24px;
    text-align: center;
  }

  .col-id {
    min-width: 90px;
  }

  .col-name {
    min-width: 80px;
    max-width: 140px;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .col-smiles {
    min-width: 100px;
    max-width: 180px;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .col-num {
    text-align: right;
    min-width: 40px;
  }

  .col-phase {
    text-align: center;
    width: 44px;
  }

  .mono {
    font-family: 'SF Mono', monospace;
  }

  .phase-badge {
    display: inline-block;
    font-size: 10px;
    font-weight: 700;
    color: #fff;
    padding: 1px 6px;
    border-radius: 3px;
    min-width: 18px;
    text-align: center;
  }

  /* Pagination */
  .pagination {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    padding: 8px 0 2px;
  }

  .page-btn {
    background: none;
    border: 1px solid rgba(48, 54, 61, 0.6);
    color: var(--text-secondary, #8b949e);
    font-size: 11px;
    padding: 3px 10px;
    border-radius: 4px;
    cursor: pointer;
  }

  .page-btn:hover:not(:disabled) {
    color: var(--text-primary, #e6edf3);
    border-color: var(--accent, #58a6ff);
  }

  .page-btn:disabled {
    opacity: 0.3;
    cursor: not-allowed;
  }

  .page-info {
    font-size: 11px;
    color: var(--text-secondary, #8b949e);
  }

  /* Checkbox styling */
  .results-table input[type="checkbox"],
  .filter-row input[type="checkbox"] {
    accent-color: var(--accent, #58a6ff);
  }
</style>
