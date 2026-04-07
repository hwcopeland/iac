<script lang="ts">
  import { loadPdb, resetCamera } from '$lib/viewer';

  let { open = $bindable(false) }: { open: boolean } = $props();
  let query = $state('');
  let selectedIndex = $state(0);
  let inputEl = $state<HTMLInputElement>(undefined as unknown as HTMLInputElement);

  type Action = {
    id: string;
    label: string;
    hint?: string;
    handler: () => void | Promise<void>;
  };

  const actions: Action[] = [
    { id: 'load-1crn', label: 'Load Crambin (1CRN)', hint: 'demo structure', handler: () => { loadPdb('1crn'); close(); } },
    { id: 'load-4hhb', label: 'Load Hemoglobin (4HHB)', hint: 'demo structure', handler: () => { loadPdb('4hhb'); close(); } },
    { id: 'load-1ubq', label: 'Load Ubiquitin (1UBQ)', hint: 'demo structure', handler: () => { loadPdb('1ubq'); close(); } },
    { id: 'reset', label: 'Reset Camera', hint: 'center view', handler: () => { resetCamera(); close(); } },
  ];

  let filtered = $derived(
    query.trim()
      ? actions.filter((a) => a.label.toLowerCase().includes(query.toLowerCase()))
      : actions
  );

  $effect(() => {
    if (open && inputEl) {
      query = '';
      selectedIndex = 0;
      // defer focus to next tick
      requestAnimationFrame(() => inputEl?.focus());
    }
  });

  function close() {
    open = false;
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      close();
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      selectedIndex = Math.min(selectedIndex + 1, filtered.length - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      selectedIndex = Math.max(selectedIndex - 1, 0);
    } else if (e.key === 'Enter' && filtered.length > 0) {
      e.preventDefault();
      filtered[selectedIndex].handler();
    }
  }

  function handleBackdrop(e: MouseEvent) {
    if ((e.target as HTMLElement).classList.contains('palette-backdrop')) {
      close();
    }
  }
</script>

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="palette-backdrop" onclick={handleBackdrop}>
    <div class="palette" role="dialog" aria-label="Command palette">
      <input
        bind:this={inputEl}
        type="text"
        class="palette-input"
        placeholder="Type a command..."
        bind:value={query}
        onkeydown={handleKeydown}
      />
      <ul class="palette-list">
        {#each filtered as action, i}
          <li>
            <button
              class="palette-item"
              class:selected={i === selectedIndex}
              onclick={() => action.handler()}
              onmouseenter={() => (selectedIndex = i)}
            >
              <span class="palette-label">{action.label}</span>
              {#if action.hint}
                <span class="palette-hint">{action.hint}</span>
              {/if}
            </button>
          </li>
        {/each}
        {#if filtered.length === 0}
          <li class="palette-empty">No matching commands</li>
        {/if}
      </ul>
    </div>
  </div>
{/if}

<style>
  .palette-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    justify-content: center;
    padding-top: 20vh;
    z-index: 1000;
  }

  .palette {
    background: var(--bg-surface);
    backdrop-filter: var(--panel-blur);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    width: 480px;
    max-height: 360px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    align-self: flex-start;
  }

  .palette-input {
    background: transparent;
    border: none;
    border-bottom: 1px solid var(--border-default);
    color: var(--text-primary);
    font-size: 15px;
    padding: 14px 16px;
    font-family: var(--font-sans);
  }

  .palette-input:focus {
    outline: none;
  }

  .palette-input::placeholder {
    color: var(--text-muted);
  }

  .palette-list {
    list-style: none;
    overflow-y: auto;
    padding: 4px;
  }

  .palette-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    width: 100%;
    padding: 8px 12px;
    border: none;
    background: none;
    color: var(--text-primary);
    font-size: 13px;
    border-radius: var(--radius-sm);
    cursor: pointer;
    transition: background var(--transition-fast);
    font-family: var(--font-sans);
    text-align: left;
  }

  .palette-item:hover,
  .palette-item.selected {
    background: var(--accent-subtle);
  }

  .palette-label {
    font-weight: 500;
  }

  .palette-hint {
    color: var(--text-muted);
    font-size: 11px;
  }

  .palette-empty {
    color: var(--text-muted);
    font-size: 13px;
    padding: 12px 16px;
    text-align: center;
  }
</style>
