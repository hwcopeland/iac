<script lang="ts">
  import Panel from './Panel.svelte';

  let element = $state('');
  let smiles = $state('');

  const tools = [
    { id: 'place', label: 'Place' },
    { id: 'bond', label: 'Bond' },
    { id: 'delete', label: 'Delete' },
  ] as const;

  let activeTool = $state<string | null>(null);

  function handleBuild() {
    // Stub: SMILES building will be implemented in v0.2
  }
</script>

<div class="builder-panels">
  <Panel title="Element">
    <input
      type="text"
      class="text-input full"
      placeholder="Element symbol (e.g. C, N, O)"
      bind:value={element}
    />
  </Panel>

  <Panel title="SMILES">
    <div class="input-row">
      <input
        type="text"
        class="text-input"
        placeholder="e.g. CCO"
        bind:value={smiles}
      />
      <button class="btn btn-accent" onclick={handleBuild} disabled={!smiles.trim()}>
        Build
      </button>
    </div>
  </Panel>

  <Panel title="Tools">
    <div class="btn-row">
      {#each tools as tool}
        <button
          class="btn btn-small"
          class:active={activeTool === tool.id}
          onclick={() => (activeTool = activeTool === tool.id ? null : tool.id)}
        >
          {tool.label}
        </button>
      {/each}
    </div>
    <p class="hint">Canvas editing coming in v0.2</p>
  </Panel>
</div>

<style>
  .builder-panels {
    display: flex;
    flex-direction: column;
  }

  .input-row {
    display: flex;
    gap: 8px;
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

  .text-input.full {
    width: 100%;
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
    margin-bottom: 8px;
  }

  .hint {
    color: var(--text-muted);
    font-size: 11px;
    font-style: italic;
  }
</style>
