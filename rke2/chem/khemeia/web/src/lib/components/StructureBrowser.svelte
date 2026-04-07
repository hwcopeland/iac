<script lang="ts">
  import Panel from './Panel.svelte';
  import { getCurrentStructureText, parseStructureComponents, toggleIsolateComponent, getIsolatedComponent } from '$lib/viewer';
  import type { StructureComponent } from '$lib/viewer';

  let components = $state<StructureComponent[]>([]);
  let structureLoaded = $state(false);
  let isolatedId = $state<string | null>(null);

  const typeIcons: Record<string, string> = {
    polymer: '\u25C6',
    ligand: '\u25CF',
    ion: '\u25CB',
    water: '\u223C',
  };

  const typeColors: Record<string, string> = {
    polymer: '#58a6ff',
    ligand: '#3fb950',
    ion: '#d29922',
    water: '#8b949e',
  };

  export function refresh() {
    const pdb = getCurrentStructureText();
    if (pdb) {
      components = parseStructureComponents(pdb);
      structureLoaded = true;
      isolatedId = null;
    } else {
      components = [];
      structureLoaded = false;
    }
  }

  async function handleClick(comp: StructureComponent) {
    await toggleIsolateComponent(comp);
    isolatedId = getIsolatedComponent();
  }

  function formatCount(comp: StructureComponent): string {
    if (comp.type === 'polymer') return `${comp.residueCount} res`;
    if (comp.type === 'water') return `${comp.residueCount} mol`;
    return `${comp.atomCount} at`;
  }

  function grouped() {
    return {
      polymers: components.filter(c => c.type === 'polymer'),
      ligands: components.filter(c => c.type === 'ligand'),
      ions: components.filter(c => c.type === 'ion'),
      water: components.find(c => c.type === 'water') ?? null,
    };
  }
</script>

{#if structureLoaded && components.length > 0}
  <Panel title="Structure">
    <div class="browser">
      {#if isolatedId}
        <button class="show-all-btn" onclick={() => handleClick(components.find(c => c.id === isolatedId)!)}>
          Show All
        </button>
      {/if}

      {#each grouped().polymers as comp}
        <button
          class="comp-row"
          class:isolated={isolatedId === comp.id}
          class:dimmed={isolatedId !== null && isolatedId !== comp.id}
          onclick={() => handleClick(comp)}
        >
          <span class="comp-icon" style="color: {typeColors[comp.type]}">{typeIcons[comp.type]}</span>
          <span class="comp-label">{comp.label}</span>
          <span class="comp-count">{formatCount(comp)}</span>
        </button>
      {/each}

      {#if grouped().ligands.length > 0}
        <div class="section-divider"></div>
        {#each grouped().ligands as comp}
          <button
            class="comp-row"
            class:isolated={isolatedId === comp.id}
            class:dimmed={isolatedId !== null && isolatedId !== comp.id}
            onclick={() => handleClick(comp)}
          >
            <span class="comp-icon" style="color: {typeColors[comp.type]}">{typeIcons[comp.type]}</span>
            <span class="comp-label">{comp.label}</span>
            <span class="comp-count">{formatCount(comp)}</span>
          </button>
        {/each}
      {/if}

      {#if grouped().ions.length > 0}
        <div class="section-divider"></div>
        {#each grouped().ions as comp}
          <button
            class="comp-row"
            class:isolated={isolatedId === comp.id}
            class:dimmed={isolatedId !== null && isolatedId !== comp.id}
            onclick={() => handleClick(comp)}
          >
            <span class="comp-icon" style="color: {typeColors[comp.type]}">{typeIcons[comp.type]}</span>
            <span class="comp-label">{comp.label}</span>
            <span class="comp-count">{formatCount(comp)}</span>
          </button>
        {/each}
      {/if}

      {#if grouped().water}
        {@const w = grouped().water!}
        <div class="section-divider"></div>
        <button
          class="comp-row"
          class:isolated={isolatedId === w.id}
          class:dimmed={isolatedId !== null && isolatedId !== w.id}
          onclick={() => handleClick(w)}
        >
          <span class="comp-icon" style="color: {typeColors.water}">{typeIcons.water}</span>
          <span class="comp-label">Water</span>
          <span class="comp-count">{formatCount(w)}</span>
        </button>
      {/if}
    </div>
  </Panel>
{/if}

<style>
  .browser {
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .show-all-btn {
    background: rgba(88, 166, 255, 0.1);
    border: 1px solid rgba(88, 166, 255, 0.3);
    border-radius: 4px;
    padding: 4px 8px;
    color: var(--accent, #58a6ff);
    font-size: 11px;
    font-weight: 500;
    cursor: pointer;
    margin-bottom: 4px;
    transition: all 0.15s;
  }

  .show-all-btn:hover {
    background: rgba(88, 166, 255, 0.2);
  }

  .comp-row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 5px 8px;
    background: rgba(0, 0, 0, 0.15);
    border: 1px solid transparent;
    border-radius: 4px;
    cursor: pointer;
    transition: all 0.15s;
    text-align: left;
    width: 100%;
  }

  .comp-row:hover {
    background: rgba(255, 255, 255, 0.05);
    border-color: rgba(48, 54, 61, 0.6);
  }

  .comp-row.isolated {
    background: rgba(88, 166, 255, 0.1);
    border-color: var(--accent, #58a6ff);
  }

  .comp-row.dimmed {
    opacity: 0.4;
  }

  .comp-icon {
    font-size: 10px;
    flex-shrink: 0;
    width: 14px;
    text-align: center;
  }

  .comp-label {
    font-size: 12px;
    font-weight: 500;
    color: var(--text-primary, #e6edf3);
    flex: 1;
    white-space: nowrap;
  }

  .comp-count {
    font-size: 10px;
    color: var(--text-secondary, #8b949e);
    font-family: 'SF Mono', monospace;
    white-space: nowrap;
  }

  .section-divider {
    height: 1px;
    background: rgba(48, 54, 61, 0.4);
    margin: 4px 0;
  }
</style>
