<script lang="ts">
  import Panel from './Panel.svelte';
  import MDTrajectoryBrowser from './MDTrajectoryBrowser.svelte';
  import { loadPdb, loadFile, resetCamera, isReady, focusPocketCenter } from '$lib/viewer';
  import { onMount } from 'svelte';
  import { login } from '$lib/auth';
  import { pipeline } from '$lib/pipeline.svelte.ts';

  let {
    onStructureLoad = () => {},
    onMDView = undefined,
  }: {
    onStructureLoad?: () => void;
    onMDView?: (
      frames: string[],
      energy: { time: number[]; potential: number[]; temperature: number[] } | null,
      compoundId: string
    ) => void;
  } = $props();

  let pdbId = $state('');
  let loading = $state(false);
  let error = $state('');
  let fileInput = $state<HTMLInputElement>(undefined as unknown as HTMLInputElement);

  async function handleLoadPdb() {
    if (!pdbId.trim() || !isReady()) return;
    loading = true;
    error = '';
    try {
      await loadPdb(pdbId.trim());
      onStructureLoad();
    } catch (e: any) {
      error = e.message || 'Failed to load structure';
    } finally {
      loading = false;
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') handleLoadPdb();
  }

  async function handleFileUpload(e: Event) {
    const target = e.target as HTMLInputElement;
    const file = target.files?.[0];
    if (!file || !isReady()) return;

    loading = true;
    error = '';
    try {
      const text = await file.text();
      const ext = file.name.split('.').pop()?.toLowerCase() || 'pdb';
      await loadFile(text, ext);
      onStructureLoad();
    } catch (err: any) {
      error = err.message || 'Failed to load file';
    } finally {
      loading = false;
    }
  }

  function handleReset() {
    resetCamera();
  }

  onMount(() => { pipeline.init(); });
</script>

<div class="explorer-panels">
  <Panel title="Load Structure">
    <div class="input-row">
      <input
        type="text"
        class="text-input"
        placeholder="PDB ID (e.g. 1crn)"
        bind:value={pdbId}
        onkeydown={handleKeydown}
      />
      <button class="btn btn-accent" onclick={handleLoadPdb} disabled={loading || !pdbId.trim()}>
        {loading ? 'Loading...' : 'Load'}
      </button>
    </div>
    <button class="link-btn" onclick={() => fileInput.click()}>
      Upload file (.pdb, .cif, .mol, .sdf, .xyz)
    </button>
    <input
      bind:this={fileInput}
      type="file"
      accept=".pdb,.cif,.mmcif,.mol,.mol2,.sdf,.xyz"
      onchange={handleFileUpload}
      style="display: none"
    />
    {#if error}
      <p class="error-msg">{error}</p>
    {/if}
  </Panel>

  <Panel title="Controls">
    <div class="btn-row">
      <button class="btn btn-small" onclick={handleReset}>Reset View</button>
    </div>
  </Panel>

  {#if onMDView}
    <MDTrajectoryBrowser onView={onMDView} />
  {/if}

  <!-- Session expired banner -->
  {#if pipeline.sessionExpired}
    <div class="session-banner">
      <span class="session-msg">Session expired — jobs still running server-side.</span>
      <button class="session-login-btn" onclick={() => login()}>Sign in</button>
    </div>
  {/if}

  <!-- Workgroup selector -->
  <div class="wg-section">
    <div class="wg-row">
      {#if pipeline.wgRenaming}
        <input
          class="wg-name-input"
          bind:value={pipeline.wgRenameValue}
          onblur={() => pipeline.commitRename()}
          onkeydown={(e) => { if (e.key === 'Enter') pipeline.commitRename(); if (e.key === 'Escape') pipeline.cancelRename(); }}
        />
      {:else}
        <select
          class="wg-select"
          value={pipeline.activeWorkgroupId}
          onchange={(e) => pipeline.switchWorkgroup((e.target as HTMLSelectElement).value)}
        >
          {#each pipeline.workgroups as wg}
            <option value={wg.id}>{wg.name}</option>
          {/each}
        </select>
      {/if}
      <div class="wg-btns">
        <button class="wg-btn" title="Rename workgroup" onclick={() => pipeline.startRename()}>✎</button>
        <button class="wg-btn" title="New workgroup" onclick={() => pipeline.newWorkgroup()}>+</button>
        {#if pipeline.workgroups.length > 1}
          <button class="wg-btn danger" title="Delete workgroup" onclick={() => pipeline.deleteWorkgroup(pipeline.activeWorkgroupId)}>✕</button>
        {/if}
      </div>
    </div>
  </div>

  <!-- Stage 1: Target Preparation -->
  <div id="stage-target">
    <Panel title="Target Preparation">
      {#snippet children()}
        <div class="stage-header" role="button" tabindex="0"
          onclick={() => pipeline.updateStage('target', { collapsed: !pipeline.stages.target.collapsed })}
          onkeydown={(e) => { if (e.key === 'Enter') pipeline.updateStage('target', { collapsed: !pipeline.stages.target.collapsed }); }}>
          <span class="stage-label">1. Target Preparation</span>
          <span class="status-badge {pipeline.statusBadgeClass(pipeline.stages.target.status)}">{pipeline.stages.target.status}</span>
        </div>

        {#if pipeline.stages.target.status === 'pending' || pipeline.stages.target.status === 'failed'}
          <form class="stage-form" onsubmit={(e) => { e.preventDefault(); pipeline.handleTargetSubmit(); }}>
            <div class="form-field">
              <label class="form-label" for="ep-pdb">PDB ID <span class="required">*</span></label>
              <input id="ep-pdb" type="text" class="form-input" placeholder="e.g. 1AKE" bind:value={pipeline.pdbId} required />
            </div>
            <div class="form-field">
              <label class="form-label" for="ep-mode">Binding Site Mode</label>
              <select id="ep-mode" class="form-select" bind:value={pipeline.bindingSiteMode}>
                <option value="native-ligand">Native Ligand</option>
                <option value="custom-box">Custom Box</option>
                <option value="pocket-detection">Pocket Detection (P2Rank)</option>
              </select>
            </div>
            {#if pipeline.bindingSiteMode === 'native-ligand'}
              <div class="form-field">
                <label class="form-label" for="ep-lig">Native Ligand ID <span class="required">*</span></label>
                <input id="ep-lig" type="text" class="form-input" placeholder="e.g. ATP" bind:value={pipeline.nativeLigandId} required />
              </div>
              {#if !pipeline.nativeLigandId.trim()}
                <p class="validation-msg">Native Ligand ID is required for this mode.</p>
              {/if}
            {/if}
            {#if pipeline.bindingSiteMode === 'custom-box'}
              <div class="box-grid">
                <div class="box-group">
                  <span class="box-group-label">Center (x, y, z)</span>
                  <div class="box-inputs">
                    <input type="number" step="0.1" class="form-input box-input" placeholder="x" bind:value={pipeline.boxCenterX} />
                    <input type="number" step="0.1" class="form-input box-input" placeholder="y" bind:value={pipeline.boxCenterY} />
                    <input type="number" step="0.1" class="form-input box-input" placeholder="z" bind:value={pipeline.boxCenterZ} />
                  </div>
                </div>
                <div class="box-group">
                  <span class="box-group-label">Size (x, y, z)</span>
                  <div class="box-inputs">
                    <input type="number" step="0.1" min="1" class="form-input box-input" placeholder="x" bind:value={pipeline.boxSizeX} />
                    <input type="number" step="0.1" min="1" class="form-input box-input" placeholder="y" bind:value={pipeline.boxSizeY} />
                    <input type="number" step="0.1" min="1" class="form-input box-input" placeholder="z" bind:value={pipeline.boxSizeZ} />
                  </div>
                </div>
              </div>
            {/if}
            <button type="submit" class="submit-btn" disabled={pipeline.targetSubmitting || !pipeline.targetFormValid()}>
              {pipeline.targetSubmitting ? 'Submitting...' : 'Prepare Target'}
            </button>
          </form>
        {/if}

        {#if pipeline.stages.target.status === 'running'}
          <div class="running-indicator">
            <span class="pulse-dot"></span>
            <span class="running-text">Preparing target {pipeline.stages.target.jobName ?? ''}...</span>
          </div>
        {/if}

        {#if pipeline.stages.target.error}
          <p class="error-msg">{pipeline.stages.target.error}</p>
        {/if}

        {#if pipeline.canAdvance('target')}
          <div class="result-info">
            <div class="result-row-actions">
              <span class="done-label">Ready: {pipeline.stages.target.jobName}</span>
              <button class="action-icon-btn" disabled={pipeline.receptorLoading}
                onclick={() => pipeline.loadTargetInViewer(pipeline.stages.target.jobName!, pipeline.targetPrepResult)}
                title="Load receptor into viewer">
                {pipeline.receptorLoading ? '⟳' : '⬡ Load 3D'}
              </button>
            </div>
          </div>
          {#if pipeline.targetPrepResult?.binding_site}
            <div class="result-info">
              <span class="result-label">Binding Site</span>
              <div class="result-detail">
                <span class="result-field">Center: ({pipeline.targetPrepResult.binding_site.center.map((v: number) => v.toFixed(1)).join(', ')})</span>
                <span class="result-field">Size: ({pipeline.targetPrepResult.binding_site.size.map((v: number) => v.toFixed(1)).join(', ')})</span>
              </div>
            </div>
          {/if}
          {#if pipeline.targetPrepResult?.pockets?.length}
            <div class="result-info">
              <span class="result-label">Detected Pockets ({pipeline.targetPrepResult.pockets.length})</span>
              {#each pipeline.targetPrepResult.pockets.slice(0, 5) as pocket, i}
                <div class="pocket-row" class:selected={pipeline.selectedPocketIdx === i}>
                  <button class="pocket-focus-btn" type="button"
                    onclick={() => { pipeline.selectedPocketIdx = i; focusPocketCenter(pocket.center); }}>
                    <span class="pocket-rank">#{pocket.rank}</span>
                    <span class="pocket-score">consensus: {pocket.consensus_score?.toFixed(2) ?? '?'}</span>
                    <span class="pocket-center">({pocket.center.map((v: number) => v.toFixed(1)).join(', ')})</span>
                  </button>
                  <button class="pocket-select-btn" type="button"
                    disabled={pipeline.selectedPocketIdx === i && pipeline.targetPrepResult.selected_pocket === i}
                    onclick={async () => {
                      if (!pipeline.stages.target.jobName) return;
                      pipeline.selectedPocketIdx = i;
                      try {
                        const { selectPocket } = await import('$lib/api');
                        await selectPocket(pipeline.stages.target.jobName, i);
                        pipeline.targetPrepResult = { ...pipeline.targetPrepResult, selected_pocket: i };
                      } catch (e) {
                        pipeline.updateStage('target', { error: (e as Error).message || 'Pocket selection failed' });
                      }
                    }}>
                    {pipeline.targetPrepResult.selected_pocket === i ? 'Selected' : 'Select'}
                  </button>
                </div>
              {/each}
            </div>
          {/if}
        {/if}
      {/snippet}
    </Panel>
  </div>

  <!-- Stage 2: Library Preparation -->
  <div id="stage-library">
    <Panel title="Library Preparation">
      {#snippet children()}
        <div class="stage-header" role="button" tabindex="0"
          onclick={() => pipeline.updateStage('library', { collapsed: !pipeline.stages.library.collapsed })}
          onkeydown={(e) => { if (e.key === 'Enter') pipeline.updateStage('library', { collapsed: !pipeline.stages.library.collapsed }); }}>
          <span class="stage-label">2. Library Preparation</span>
          <span class="status-badge {pipeline.statusBadgeClass(pipeline.stages.library.status)}">{pipeline.stages.library.status}</span>
        </div>

        {#if pipeline.stages.library.status === 'pending' || pipeline.stages.library.status === 'failed'}
          <form class="stage-form" onsubmit={(e) => { e.preventDefault(); pipeline.handleLibrarySubmit(); }}>
            <div class="form-field">
              <label class="form-label" for="ep-lsource">Source</label>
              <select id="ep-lsource" class="form-select" bind:value={pipeline.libSource}>
                <option value="smiles">SMILES Input</option>
                <option value="chembl">ChEMBL Filter</option>
              </select>
            </div>

            {#if pipeline.libSource === 'smiles'}
              <div class="form-field">
                <label class="form-label" for="ep-smiles">SMILES (one per line)</label>
                <textarea id="ep-smiles" class="form-textarea" rows="6"
                  placeholder="CC(=O)Oc1ccccc1C(=O)O&#10;c1ccccc1"
                  bind:value={pipeline.smilesText}></textarea>
                <div class="smiles-meta">
                  <span class="compound-count">{pipeline.smilesCount} compound{pipeline.smilesCount !== 1 ? 's' : ''}</span>
                  <button type="button" class="load-example-btn" onclick={() => { pipeline.smilesText = pipeline.EXAMPLE_SMILES; }}>
                    Load example
                  </button>
                </div>
              </div>
            {:else}
              <div class="form-field">
                <label class="form-label" for="ep-chtarget">ChEMBL Target ID</label>
                <input id="ep-chtarget" type="text" class="form-input" placeholder="e.g. CHEMBL25" bind:value={pipeline.chemblTarget} />
              </div>
              <div class="form-field">
                <label class="form-label" for="ep-phase">Min Clinical Phase</label>
                <select id="ep-phase" class="form-select" bind:value={pipeline.chemblMaxPhase}>
                  <option value={0}>Any</option>
                  <option value={1}>Phase I+</option>
                  <option value={2}>Phase II+</option>
                  <option value={3}>Phase III+</option>
                  <option value={4}>Approved</option>
                </select>
              </div>
              <div class="form-field">
                <label class="form-label">MW Range</label>
                <div class="range-row">
                  <input type="number" step="1" class="form-input" placeholder="Min" bind:value={pipeline.chemblMwMin} />
                  <span class="range-sep">-</span>
                  <input type="number" step="1" class="form-input" placeholder="Max" bind:value={pipeline.chemblMwMax} />
                </div>
              </div>
              <div class="form-field">
                <label class="form-label">LogP Range</label>
                <div class="range-row">
                  <input type="number" step="0.1" class="form-input" placeholder="Min" bind:value={pipeline.chemblLogpMin} />
                  <span class="range-sep">-</span>
                  <input type="number" step="0.1" class="form-input" placeholder="Max" bind:value={pipeline.chemblLogpMax} />
                </div>
              </div>
              <div class="form-field">
                <label class="form-label" for="ep-hba">HBA Max</label>
                <input id="ep-hba" type="number" step="1" min="0" class="form-input" placeholder="e.g. 10" bind:value={pipeline.chemblHbaMax} />
              </div>
              <div class="form-field">
                <label class="form-label" for="ep-hbd">HBD Max</label>
                <input id="ep-hbd" type="number" step="1" min="0" class="form-input" placeholder="e.g. 5" bind:value={pipeline.chemblHbdMax} />
              </div>
            {/if}

            <div class="filter-toggles">
              <label class="toggle-label"><input type="checkbox" bind:checked={pipeline.filterLipinski} /><span>Lipinski</span></label>
              <label class="toggle-label"><input type="checkbox" bind:checked={pipeline.filterVeber} /><span>Veber</span></label>
              <label class="toggle-label"><input type="checkbox" bind:checked={pipeline.filterPAINS} /><span>PAINS</span></label>
            </div>

            <button type="submit" class="submit-btn" disabled={pipeline.libSubmitting || !pipeline.libFormValid()}>
              {pipeline.libSubmitting ? 'Submitting...' : 'Prepare Library'}
            </button>
          </form>
          <div class="attach-sep-row"><span class="attach-sep">or attach to existing job</span></div>
          <div class="input-row">
            <input type="text" class="form-input" placeholder="Job name (e.g. pipeline-lib-…)" bind:value={pipeline.libAttachName} />
            <button type="button" class="action-icon-btn"
              disabled={!pipeline.libAttachName.trim() || pipeline.libAttaching}
              onclick={() => pipeline.handleLibraryAttach()}>
              {pipeline.libAttaching ? '…' : 'Attach'}
            </button>
          </div>
        {/if}

        {#if pipeline.stages.library.status === 'running'}
          {#if pipeline.libraryStatus?.total_count > 0}
            {@const pct = Math.min(100, Math.round((pipeline.libraryStatus.processed_count / pipeline.libraryStatus.total_count) * 100))}
            <div class="running-indicator">
              <span class="pulse-dot"></span>
              <span class="running-text">{pipeline.libraryStatus.processed_count.toLocaleString()} / {pipeline.libraryStatus.total_count.toLocaleString()} ({pct}%)</span>
            </div>
            <div class="lib-progress"><div class="lib-progress-bar" style="width: {pct}%"></div></div>
          {:else}
            <div class="running-indicator">
              <span class="pulse-dot"></span>
              <span class="running-text">
                {#if pipeline.libraryStatus?.compound_count > 0}
                  {pipeline.libraryStatus.compound_count.toLocaleString()} compounds found — standardizing &amp; filtering...
                {:else}
                  Resolving compounds from source...
                {/if}
              </span>
            </div>
          {/if}
        {/if}

        {#if pipeline.stages.library.error}
          <p class="error-msg">{pipeline.stages.library.error}</p>
        {/if}

        {#if pipeline.canAdvance('library')}
          <div class="result-info">
            <span class="result-label">
              {pipeline.libraryStatus?.compound_count ?? pipeline.libraryCompoundSample.length} compounds ready
            </span>
            {#if pipeline.libraryCompoundSample.length > 0}
              <div class="compound-sample">
                {#each pipeline.libraryCompoundSample.slice(0, 6) as cpd}
                  <div class="compound-chip" title={cpd.smiles ?? cpd.compound_id}>
                    <span class="compound-chip-id">{cpd.compound_id?.slice(0, 18)}</span>
                    {#if cpd.mw}<span class="compound-chip-mw">{cpd.mw?.toFixed(0)} Da</span>{/if}
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
      {/snippet}
    </Panel>
  </div>
</div>

<style>
  .explorer-panels {
    display: flex;
    flex-direction: column;
  }

  .input-row {
    display: flex;
    gap: 8px;
    margin-bottom: 8px;
  }

  .text-input {
    flex: 1;
    background: var(--bg-input);
    border: 1px solid var(--border-default);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: 13px;
    padding: 6px 10px;
    border-radius: var(--radius-sm);
    transition: border-color var(--transition-fast);
  }

  .text-input:focus {
    border-color: var(--border-focus);
    outline: none;
  }

  .text-input::placeholder {
    color: var(--text-muted);
  }

  .btn {
    background: var(--accent-subtle);
    border: 1px solid transparent;
    color: var(--accent);
    font-size: 12px;
    font-weight: 500;
    padding: 6px 12px;
    border-radius: var(--radius-sm);
    cursor: pointer;
    transition: all var(--transition-fast);
    font-family: var(--font-sans);
    white-space: nowrap;
  }

  .btn:hover:not(:disabled) {
    background: rgba(88, 166, 255, 0.25);
  }

  .btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .btn-accent {
    background: var(--accent);
    color: var(--bg-base);
    font-weight: 600;
  }

  .btn-accent:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .btn-small {
    font-size: 11px;
    padding: 4px 10px;
  }

  .btn-small.active {
    background: var(--accent);
    color: var(--bg-base);
  }

  .btn-row {
    display: flex;
    gap: 4px;
  }

  .link-btn {
    background: none;
    border: none;
    color: var(--text-secondary);
    font-size: 12px;
    cursor: pointer;
    padding: 2px 0;
    text-decoration: underline;
    text-underline-offset: 2px;
    font-family: var(--font-sans);
  }

  .link-btn:hover {
    color: var(--accent);
  }

  .error-msg {
    color: var(--danger);
    font-size: 12px;
    margin-top: 6px;
  }

  /* Workgroup selector */
  .wg-section { padding: 10px 0 4px; }
  .wg-row { display: flex; align-items: center; gap: 6px; }
  .wg-select {
    flex: 1;
    background: rgba(0,0,0,0.3);
    border: 1px solid rgba(48,54,61,0.6);
    color: var(--text-primary, #e6edf3);
    font-size: 12px;
    font-family: var(--font-sans);
    padding: 4px 6px;
    border-radius: 4px;
    outline: none;
    cursor: pointer;
  }
  .wg-select:focus { border-color: var(--accent, #58a6ff); }
  .wg-name-input {
    flex: 1;
    background: rgba(0,0,0,0.3);
    border: 1px solid var(--accent, #58a6ff);
    color: var(--text-primary, #e6edf3);
    font-size: 12px;
    font-family: var(--font-sans);
    padding: 4px 6px;
    border-radius: 4px;
    outline: none;
  }
  .wg-btns { display: flex; gap: 3px; }
  .wg-btn {
    background: rgba(48,54,61,0.3);
    border: 1px solid rgba(48,54,61,0.5);
    color: var(--text-secondary, #8b949e);
    font-size: 12px;
    width: 24px;
    height: 24px;
    border-radius: 3px;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    line-height: 1;
  }
  .wg-btn:hover { color: var(--text-primary, #e6edf3); border-color: rgba(88,166,255,0.4); }
  .wg-btn.danger { color: #f85149; }
  .wg-btn.danger:hover { background: rgba(248,81,73,0.1); }

  .stage-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; cursor: pointer; }
  .stage-label { font-size: 12px; font-weight: 600; color: var(--text-primary, #e6edf3); }
  .status-badge { font-size: 10px; font-weight: 600; text-transform: uppercase; padding: 1px 6px; border-radius: 3px; white-space: nowrap; }
  .status-badge.pending { background: rgba(88,166,255,0.1); color: var(--accent, #58a6ff); }
  .status-badge.running { background: rgba(210,153,34,0.15); color: #d29922; }
  .status-badge.completed { background: rgba(63,185,80,0.15); color: #3fb950; }
  .status-badge.failed { background: rgba(248,81,73,0.15); color: #f85149; }

  .stage-form { display: flex; flex-direction: column; gap: 10px; }
  .form-field { display: flex; flex-direction: column; gap: 3px; }
  .form-label { font-size: 11px; font-weight: 600; color: var(--text-secondary, #8b949e); text-transform: uppercase; letter-spacing: 0.3px; }
  .required { color: #f85149; }
  .form-input, .form-select, .form-textarea {
    background: rgba(0,0,0,0.3);
    border: 1px solid rgba(48,54,61,0.6);
    color: var(--text-primary, #e6edf3);
    font-family: 'SF Mono', monospace;
    font-size: 12px;
    padding: 6px 8px;
    border-radius: 4px;
    outline: none;
    width: 100%;
    transition: border-color 0.15s;
  }
  .form-input:focus, .form-select:focus, .form-textarea:focus { border-color: var(--accent, #58a6ff); }
  .form-textarea { resize: vertical; min-height: 60px; }
  .validation-msg { color: #f85149; font-size: 11px; margin: 0; }

  .box-grid { display: flex; flex-direction: column; gap: 8px; }
  .box-group { display: flex; flex-direction: column; gap: 3px; }
  .box-group-label { font-size: 11px; font-weight: 600; color: var(--text-secondary, #8b949e); text-transform: uppercase; letter-spacing: 0.3px; }
  .box-inputs { display: flex; gap: 6px; }
  .box-input { flex: 1; min-width: 0; }

  .submit-btn {
    background: var(--accent, #58a6ff);
    border: none;
    color: #000;
    font-size: 13px;
    font-weight: 600;
    padding: 8px 12px;
    border-radius: 6px;
    cursor: pointer;
    width: 100%;
  }
  .submit-btn:hover:not(:disabled) { opacity: 0.9; }
  .submit-btn:disabled { opacity: 0.4; cursor: not-allowed; }

  .filter-toggles { display: flex; gap: 12px; flex-wrap: wrap; }
  .toggle-label { display: flex; align-items: center; gap: 4px; font-size: 12px; color: var(--text-secondary, #8b949e); cursor: pointer; }
  .toggle-label input[type="checkbox"] { accent-color: var(--accent, #58a6ff); }

  .smiles-meta { display: flex; align-items: center; justify-content: space-between; margin-top: 2px; }
  .compound-count { font-size: 11px; color: var(--text-muted, #484f58); }
  .load-example-btn { background: none; border: none; color: var(--accent, #58a6ff); font-size: 11px; cursor: pointer; padding: 0; }
  .load-example-btn:hover { text-decoration: underline; }

  .range-row { display: flex; align-items: center; gap: 6px; }
  .range-sep { color: var(--text-muted, #484f58); font-size: 12px; flex-shrink: 0; }

  .running-indicator { display: flex; align-items: center; gap: 8px; padding: 8px 0; }
  .pulse-dot { width: 8px; height: 8px; border-radius: 50%; background: #d29922; animation: pulse 1.5s ease-in-out infinite; flex-shrink: 0; }
  @keyframes pulse { 0%, 100% { opacity: 1; transform: scale(1); } 50% { opacity: 0.5; transform: scale(0.8); } }
  .running-text { font-size: 12px; color: #d29922; }
  .lib-progress { height: 4px; background: rgba(88,166,255,0.1); border-radius: 2px; overflow: hidden; margin: 0 0 4px; }
  .lib-progress-bar { height: 100%; background: #58a6ff; border-radius: 2px; transition: width 0.5s ease; }
  .attach-sep-row { display: flex; align-items: center; margin: 8px 0 4px; }
  .attach-sep { font-size: 11px; color: var(--text-muted, #484f58); }

  .result-info { display: flex; flex-direction: column; gap: 4px; padding: 8px 0; border-top: 1px solid rgba(48,54,61,0.4); }
  .result-row-actions { display: flex; align-items: center; justify-content: space-between; }
  .result-label { font-size: 11px; font-weight: 600; color: var(--text-secondary, #8b949e); text-transform: uppercase; letter-spacing: 0.3px; }
  .result-detail { display: flex; gap: 12px; flex-wrap: wrap; }
  .result-field { font-size: 11px; font-family: 'SF Mono', monospace; color: var(--text-primary, #e6edf3); }

  .action-icon-btn {
    background: rgba(88,166,255,0.1);
    border: 1px solid rgba(88,166,255,0.3);
    color: var(--accent, #58a6ff);
    font-size: 11px;
    padding: 2px 8px;
    border-radius: 3px;
    cursor: pointer;
    white-space: nowrap;
  }
  .action-icon-btn:hover:not(:disabled) { background: rgba(88,166,255,0.2); }
  .action-icon-btn:disabled { opacity: 0.4; cursor: not-allowed; }

  .pocket-row { display: flex; gap: 8px; align-items: center; font-size: 11px; font-family: 'SF Mono', monospace; color: var(--text-secondary, #8b949e); padding: 2px 0; }
  .pocket-row.selected { color: var(--accent, #58a6ff); }
  .pocket-focus-btn { background: none; border: none; color: inherit; font: inherit; cursor: pointer; display: flex; gap: 6px; align-items: center; flex: 1; text-align: left; padding: 0; }
  .pocket-focus-btn:hover { color: var(--text-primary, #e6edf3); }
  .pocket-rank { font-weight: 700; }
  .pocket-score, .pocket-center { color: var(--text-muted, #484f58); font-size: 10px; }
  .pocket-select-btn { background: rgba(88,166,255,0.1); border: 1px solid rgba(88,166,255,0.3); color: var(--accent, #58a6ff); font-size: 10px; padding: 1px 6px; border-radius: 3px; cursor: pointer; white-space: nowrap; }
  .pocket-select-btn:disabled { opacity: 0.4; cursor: not-allowed; }

  .compound-sample { display: flex; flex-direction: column; gap: 2px; margin-top: 4px; }
  .compound-chip { display: flex; align-items: center; justify-content: space-between; padding: 2px 6px; background: rgba(0,0,0,0.2); border-radius: 3px; border: 1px solid rgba(48,54,61,0.4); }
  .compound-chip-id { font-size: 10px; font-family: 'SF Mono', monospace; color: var(--text-secondary, #8b949e); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .compound-chip-mw { font-size: 10px; color: var(--text-muted, #484f58); white-space: nowrap; margin-left: 8px; }

  .done-label { font-size: 11px; color: #3fb950; font-family: 'SF Mono', monospace; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 160px; }

  .session-banner { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 8px; background: rgba(210,153,34,0.1); border: 1px solid rgba(210,153,34,0.3); border-radius: 4px; margin-bottom: 8px; }
  .session-msg { font-size: 11px; color: #d29922; }
  .session-login-btn { background: rgba(210,153,34,0.2); border: 1px solid rgba(210,153,34,0.5); color: #d29922; font-size: 11px; padding: 2px 10px; border-radius: 3px; cursor: pointer; }
</style>
