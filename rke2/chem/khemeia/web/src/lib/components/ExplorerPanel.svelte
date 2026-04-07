<script lang="ts">
  import Panel from './Panel.svelte';
  import { loadPdb, loadFile, resetCamera, isReady } from '$lib/viewer';

  let { onStructureLoad = () => {} }: { onStructureLoad?: () => void } = $props();

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
</style>
