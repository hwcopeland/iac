<script lang="ts">
  import Panel from './Panel.svelte';
  import {
    focusResidue,
    setRepresentation,
    setColorTheme,
    showSurface,
    hideSurface,
    getSequence,
  } from '$lib/viewer';

  // ── Residue jump state ──
  let chains = $state<string[]>([]);
  let selectedChain = $state('');
  let residueInput = $state('');

  // ── Representation state ──
  type RepOption = { label: string; value: string };
  const repOptions: RepOption[] = [
    { label: 'Cartoon', value: 'cartoon' },
    { label: 'Sticks', value: 'ball-and-stick' },
    { label: 'Surface', value: 'molecular-surface' },
    { label: 'Spacefill', value: 'spacefill' },
  ];
  let activeRep = $state('cartoon');

  // ── Color theme state ──
  type ColorOption = { label: string; value: string };
  const colorOptions: ColorOption[] = [
    { label: 'Element', value: 'element-symbol' },
    { label: 'Chain', value: 'chain-id' },
    { label: 'Hydro', value: 'hydrophobicity' },
    { label: 'Charge', value: 'partial-charge' },
    { label: '2\u00B0 Struct', value: 'secondary-structure' },
  ];
  let activeColor = $state('element-symbol');

  // ── Surface toggle state ──
  let surfaceOn = $state(false);

  /** Refresh chain list from the viewer. Called externally after structure loads. */
  export function refreshChains() {
    const seq = getSequence();
    chains = seq.map((s) => s.chainId);
    if (chains.length > 0 && !chains.includes(selectedChain)) {
      selectedChain = chains[0];
    }
  }

  function handleGo() {
    const resId = parseInt(residueInput, 10);
    if (!selectedChain || isNaN(resId)) return;
    focusResidue(selectedChain, resId);
  }

  function handleResidueKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') handleGo();
  }

  async function handleRep(value: string) {
    activeRep = value;
    await setRepresentation(value);
  }

  async function handleColor(value: string) {
    activeColor = value;
    await setColorTheme(value);
    // If surface is on, update its color theme too
    if (surfaceOn) {
      await showSurface(value);
    }
  }

  async function toggleSurface() {
    surfaceOn = !surfaceOn;
    if (surfaceOn) {
      await showSurface(activeColor);
    } else {
      await hideSurface();
    }
  }
</script>

<Panel title="Visualization">
  <div class="controls">
    <!-- Residue Jump -->
    <div class="control-section">
      <span class="section-label">Residue</span>
      <div class="jump-row">
        <select class="chain-select" bind:value={selectedChain}>
          {#each chains as chain}
            <option value={chain}>{chain}</option>
          {/each}
        </select>
        <input
          type="number"
          class="res-input"
          placeholder="#"
          bind:value={residueInput}
          onkeydown={handleResidueKeydown}
        />
        <button class="go-btn" onclick={handleGo} disabled={!selectedChain || !residueInput}>
          Go
        </button>
      </div>
    </div>

    <!-- Representation -->
    <div class="control-section">
      <span class="section-label">Representation</span>
      <div class="btn-group">
        {#each repOptions as opt}
          <button
            class="group-btn"
            class:active={activeRep === opt.value}
            onclick={() => handleRep(opt.value)}
          >
            {opt.label}
          </button>
        {/each}
      </div>
    </div>

    <!-- Color Theme -->
    <div class="control-section">
      <span class="section-label">Color</span>
      <div class="btn-group">
        {#each colorOptions as opt}
          <button
            class="group-btn"
            class:active={activeColor === opt.value}
            onclick={() => handleColor(opt.value)}
          >
            {opt.label}
          </button>
        {/each}
      </div>
    </div>

    <!-- Surface Toggle -->
    <div class="control-section">
      <div class="toggle-row">
        <button
          class="toggle-btn"
          class:active={surfaceOn}
          onclick={toggleSurface}
        >
          {surfaceOn ? 'Hide Surface' : 'Show Surface'}
        </button>
      </div>
    </div>
  </div>
</Panel>

<style>
  .controls {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .control-section {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .section-label {
    font-size: 10px;
    font-weight: 600;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.4px;
  }

  /* ── Residue Jump ── */
  .jump-row {
    display: flex;
    gap: 4px;
    align-items: center;
  }

  .chain-select {
    width: 48px;
    background: var(--bg-input, #0d1117);
    border: 1px solid var(--border-default, #30363d);
    color: var(--text-primary, #e6edf3);
    font-family: var(--font-mono, 'SF Mono', monospace);
    font-size: 11px;
    padding: 4px 6px;
    border-radius: 4px;
    cursor: pointer;
    appearance: none;
    -webkit-appearance: none;
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='8' height='5'%3E%3Cpath d='M0 0l4 5 4-5z' fill='%23484f58'/%3E%3C/svg%3E");
    background-repeat: no-repeat;
    background-position: right 4px center;
    padding-right: 14px;
  }

  .chain-select:focus {
    border-color: var(--border-focus, #58a6ff);
    outline: none;
  }

  .res-input {
    flex: 1;
    min-width: 0;
    background: var(--bg-input, #0d1117);
    border: 1px solid var(--border-default, #30363d);
    color: var(--text-primary, #e6edf3);
    font-family: var(--font-mono, 'SF Mono', monospace);
    font-size: 11px;
    padding: 4px 6px;
    border-radius: 4px;
    -moz-appearance: textfield;
  }

  .res-input::-webkit-inner-spin-button,
  .res-input::-webkit-outer-spin-button {
    -webkit-appearance: none;
    margin: 0;
  }

  .res-input:focus {
    border-color: var(--border-focus, #58a6ff);
    outline: none;
  }

  .res-input::placeholder {
    color: var(--text-muted, #484f58);
  }

  .go-btn {
    background: var(--accent-subtle, rgba(88, 166, 255, 0.15));
    border: 1px solid transparent;
    color: var(--accent, #58a6ff);
    font-size: 10px;
    font-weight: 600;
    padding: 4px 10px;
    border-radius: 4px;
    cursor: pointer;
    font-family: var(--font-sans);
    transition: all 0.15s;
    white-space: nowrap;
  }

  .go-btn:hover:not(:disabled) {
    background: rgba(88, 166, 255, 0.25);
  }

  .go-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  /* ── Button Groups ── */
  .btn-group {
    display: flex;
    flex-wrap: wrap;
    gap: 3px;
  }

  .group-btn {
    background: rgba(0, 0, 0, 0.15);
    border: 1px solid var(--border-default, #30363d);
    color: var(--text-secondary, #8b949e);
    font-size: 10px;
    font-weight: 500;
    padding: 4px 8px;
    border-radius: 4px;
    cursor: pointer;
    transition: all 0.15s;
    font-family: var(--font-sans);
    white-space: nowrap;
  }

  .group-btn:hover {
    background: rgba(255, 255, 255, 0.05);
    color: var(--text-primary, #e6edf3);
    border-color: rgba(48, 54, 61, 0.8);
  }

  .group-btn.active {
    background: var(--accent, #58a6ff);
    color: var(--bg-base, #0d1117);
    border-color: var(--accent, #58a6ff);
    font-weight: 600;
  }

  /* ── Surface Toggle ── */
  .toggle-row {
    display: flex;
  }

  .toggle-btn {
    background: rgba(0, 0, 0, 0.15);
    border: 1px solid var(--border-default, #30363d);
    color: var(--text-secondary, #8b949e);
    font-size: 10px;
    font-weight: 500;
    padding: 4px 10px;
    border-radius: 4px;
    cursor: pointer;
    transition: all 0.15s;
    font-family: var(--font-sans);
  }

  .toggle-btn:hover {
    background: rgba(255, 255, 255, 0.05);
    color: var(--text-primary, #e6edf3);
  }

  .toggle-btn.active {
    background: var(--accent, #58a6ff);
    color: var(--bg-base, #0d1117);
    border-color: var(--accent, #58a6ff);
    font-weight: 600;
  }
</style>
