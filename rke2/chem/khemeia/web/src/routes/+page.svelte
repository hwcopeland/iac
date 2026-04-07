<script lang="ts">
  import { onMount } from 'svelte';
  import { init, onHover, onClick } from '$lib/viewer';
  import type { AtomInfo } from '$lib/viewer';
  import { restoreSession } from '$lib/auth';
  import Toolbar from '$lib/components/Toolbar.svelte';
  import ExplorerPanel from '$lib/components/ExplorerPanel.svelte';
  import StructureBrowser from '$lib/components/StructureBrowser.svelte';
  import BuilderPanel from '$lib/components/BuilderPanel.svelte';
  import CalculationsPanel from '$lib/components/CalculationsPanel.svelte';
  import SelectionInfo from '$lib/components/SelectionInfo.svelte';
  import StatusBar from '$lib/components/StatusBar.svelte';
  import CommandPalette from '$lib/components/CommandPalette.svelte';
  import Toast from '$lib/components/Toast.svelte';

  type Tab = 'explorer' | 'builder' | 'calculations';

  let activeTab = $state<Tab>('explorer');
  let viewerContainer = $state<HTMLDivElement>(undefined as unknown as HTMLDivElement);
  let hoverInfo = $state<AtomInfo | null>(null);
  let selectionInfo = $state<AtomInfo | null>(null);
  let commandPaletteOpen = $state(false);
  let panelVisible = $state(true);
  let structureBrowser = $state<StructureBrowser>(undefined as unknown as StructureBrowser);
  let authReady = $state(false);

  onMount(() => {
    initViewer();
    setupKeybinds();
    restoreSession().finally(() => { authReady = true; });
  });

  async function initViewer() {
    try {
      await init(viewerContainer);
      onHover((info) => { hoverInfo = info; });
      onClick((info) => { selectionInfo = info; });
    } catch (e) {
      console.error('Failed to initialize Mol* viewer:', e);
    }
  }

  function setupKeybinds() {
    window.addEventListener('keydown', (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      if (mod && e.key === 'k') {
        e.preventDefault();
        commandPaletteOpen = !commandPaletteOpen;
      }
      if (e.key === 'Escape') {
        commandPaletteOpen = false;
      }
      // Tab switching: 1/2/3
      if (!mod && !e.altKey && !e.shiftKey) {
        if (e.key === '1') activeTab = 'explorer';
        else if (e.key === '2') activeTab = 'builder';
        else if (e.key === '3') activeTab = 'calculations';
      }
      // Toggle panel: Ctrl/Cmd+B
      if (mod && e.key === 'b') {
        e.preventDefault();
        panelVisible = !panelVisible;
      }
    });
  }
</script>

<div class="app">
  <Toolbar bind:activeTab onCommandPalette={() => (commandPaletteOpen = true)} {authReady} />

  <div class="main">
    <div class="viewer-area">
      <div class="viewer-container" bind:this={viewerContainer}></div>

      {#if selectionInfo}
        <div class="selection-overlay">
          <SelectionInfo info={selectionInfo} />
        </div>
      {/if}
    </div>

    {#if panelVisible}
      <aside class="side-panel">
        <div class="side-panel-scroll">
          {#if activeTab === 'explorer'}
            <ExplorerPanel onStructureLoad={() => structureBrowser?.refresh()} />
            <StructureBrowser bind:this={structureBrowser} />
          {:else if activeTab === 'builder'}
            <BuilderPanel />
          {:else if activeTab === 'calculations'}
            <CalculationsPanel />
          {/if}
        </div>
      </aside>
    {/if}
  </div>

  <StatusBar {hoverInfo} />
  <CommandPalette bind:open={commandPaletteOpen} />
  <Toast />
</div>

<style>
  .app {
    display: flex;
    flex-direction: column;
    height: 100vh;
    width: 100vw;
    overflow: hidden;
  }

  .main {
    flex: 1;
    display: flex;
    overflow: hidden;
    position: relative;
  }

  .viewer-area {
    flex: 1;
    position: relative;
    overflow: hidden;
  }

  .viewer-container {
    position: absolute;
    inset: 0;
  }

  .viewer-container :global(.msp-plugin) {
    width: 100% !important;
    height: 100% !important;
  }

  .selection-overlay {
    position: absolute;
    bottom: 12px;
    left: 12px;
    z-index: 50;
  }

  .side-panel {
    width: 320px;
    flex-shrink: 0;
    background: rgba(13, 17, 23, 0.6);
    backdrop-filter: var(--panel-blur);
    border-left: 1px solid var(--border-default);
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .side-panel-scroll {
    flex: 1;
    overflow-y: auto;
    padding: 8px;
  }
</style>
