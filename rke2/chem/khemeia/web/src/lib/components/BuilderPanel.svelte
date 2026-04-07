<script lang="ts">
  import Panel from './Panel.svelte';
  import { loadFile } from '$lib/viewer';
  import { smilesToMolBlock, validateSmiles, getMolProperties, getSVG } from '$lib/rdkit';

  let smiles = $state('');
  let building = $state(false);
  let error = $state('');
  let props = $state<any>(null);
  let svgHtml = $state('');
  let valid = $state<boolean | null>(null);
  let history = $state<string[]>([]);

  let debounceTimer: any = null;

  function onSmilesInput() {
    error = '';
    props = null;
    svgHtml = '';
    valid = null;
    if (!smiles.trim()) return;

    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(async () => {
      const s = smiles.trim();
      valid = await validateSmiles(s);
      if (!valid) { error = 'Invalid SMILES'; return; }
      const [svg, p] = await Promise.all([getSVG(s, 280, 160), getMolProperties(s)]);
      if (svg) svgHtml = svg;
      if (p) props = p;
    }, 300);
  }

  async function handleBuild() {
    const s = smiles.trim();
    if (!s) return;
    building = true;
    error = '';
    try {
      const molBlock = await smilesToMolBlock(s);
      if (!molBlock) { error = 'Failed to generate 3D'; return; }
      await loadFile(molBlock, 'mol');
      // Add to history (deduplicate, most recent first)
      history = [s, ...history.filter(h => h !== s)].slice(0, 10);
    } catch (e: any) {
      error = e.message || 'Build failed';
    } finally {
      building = false;
    }
  }

  function loadFromHistory(s: string) {
    smiles = s;
    onSmilesInput();
  }
</script>

<div class="builder-panels">
  <Panel title="SMILES Builder">
    <div class="smiles-section">
      <div class="input-row">
        <input
          type="text"
          class="text-input"
          placeholder="e.g. c1ccccc1, CC(=O)O"
          bind:value={smiles}
          oninput={onSmilesInput}
          onkeydown={(e) => e.key === 'Enter' && handleBuild()}
          spellcheck="false"
          class:invalid={valid === false}
          class:valid-input={valid === true}
        />
        <button class="btn" onclick={handleBuild} disabled={building || !smiles.trim() || valid === false}>
          {building ? '...' : '3D'}
        </button>
      </div>

      {#if error}
        <p class="error">{error}</p>
      {/if}

      {#if svgHtml}
        <div class="preview">{@html svgHtml}</div>
      {/if}

      {#if props}
        <div class="props-grid">
          <div class="prop"><span class="prop-label">Formula</span><span class="prop-value">{props.formula}</span></div>
          <div class="prop"><span class="prop-label">MW</span><span class="prop-value">{props.mw.toFixed(1)}</span></div>
          <div class="prop"><span class="prop-label">LogP</span><span class="prop-value">{props.logp.toFixed(2)}</span></div>
          <div class="prop"><span class="prop-label">HBA</span><span class="prop-value">{props.hba}</span></div>
          <div class="prop"><span class="prop-label">HBD</span><span class="prop-value">{props.hbd}</span></div>
          <div class="prop"><span class="prop-label">TPSA</span><span class="prop-value">{props.tpsa.toFixed(1)}</span></div>
          <div class="prop"><span class="prop-label">RotBonds</span><span class="prop-value">{props.rotatable}</span></div>
        </div>
      {/if}
    </div>
  </Panel>

  {#if history.length > 0}
    <Panel title="Recent" collapsed={true}>
      <div class="history-list">
        {#each history as h}
          <button class="history-item" onclick={() => loadFromHistory(h)} title={h}>
            <span class="history-smiles">{h}</span>
          </button>
        {/each}
      </div>
    </Panel>
  {/if}
</div>

<style>
  .builder-panels { display: flex; flex-direction: column; }

  .smiles-section {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .input-row { display: flex; gap: 8px; }

  .text-input {
    flex: 1;
    background: rgba(0,0,0,0.3);
    border: 1px solid rgba(48,54,61,0.6);
    color: var(--text-primary, #e6edf3);
    font-family: 'SF Mono', monospace;
    font-size: 13px;
    padding: 8px 10px;
    border-radius: 6px;
    outline: none;
    transition: border-color 0.15s;
  }

  .text-input:focus { border-color: var(--accent, #58a6ff); }
  .text-input.invalid { border-color: var(--danger, #f85149); }
  .text-input.valid-input { border-color: var(--success, #3fb950); }
  .text-input::placeholder { color: rgba(72,79,88,0.8); }

  .btn {
    background: var(--accent, #58a6ff);
    border: none;
    color: #000;
    font-size: 13px;
    font-weight: 600;
    padding: 8px 14px;
    border-radius: 6px;
    cursor: pointer;
    white-space: nowrap;
  }

  .btn:hover:not(:disabled) { opacity: 0.9; }
  .btn:disabled { opacity: 0.4; cursor: not-allowed; }

  .error { color: var(--danger, #f85149); font-size: 12px; }

  .preview {
    background: rgba(255,255,255,0.03);
    border-radius: 8px;
    padding: 8px;
    display: flex;
    justify-content: center;
  }

  .preview :global(svg) { max-width: 100%; height: auto; }

  .props-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 4px;
  }

  .prop {
    display: flex;
    justify-content: space-between;
    padding: 3px 8px;
    background: rgba(0,0,0,0.2);
    border-radius: 4px;
  }

  .prop-label {
    font-size: 10px;
    color: var(--text-secondary, #8b949e);
    text-transform: uppercase;
  }

  .prop-value {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-primary, #e6edf3);
    font-family: 'SF Mono', monospace;
  }

  .history-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .history-item {
    background: rgba(0,0,0,0.2);
    border: 1px solid transparent;
    border-radius: 4px;
    padding: 4px 8px;
    cursor: pointer;
    text-align: left;
    transition: all 0.15s;
  }

  .history-item:hover {
    background: rgba(255,255,255,0.05);
    border-color: rgba(48,54,61,0.6);
  }

  .history-smiles {
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    color: var(--text-secondary, #8b949e);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: block;
  }
</style>
