<script lang="ts">
  import { onMount } from 'svelte';
  import { init, onHover, onClick } from '$lib/viewer';
  import type { AtomInfo } from '$lib/viewer';
  import { isAuthenticated, login, restoreSession } from '$lib/auth';
  import Toolbar from '$lib/components/Toolbar.svelte';
  import ExplorerPanel from '$lib/components/ExplorerPanel.svelte';
  import StructureBrowser from '$lib/components/StructureBrowser.svelte';
  import AnalysisPanel from '$lib/components/AnalysisPanel.svelte';
  import InteractionNetwork from '$lib/components/InteractionNetwork.svelte';
  import CalculationsPanel from '$lib/components/CalculationsPanel.svelte';
  import PipelinePanel from '$lib/components/PipelinePanel.svelte';
  import MDTrajectoryOverlay from '$lib/components/MDTrajectoryOverlay.svelte';
  import SelectionInfo from '$lib/components/SelectionInfo.svelte';
  import { focusResidue, setRepresentation, onRepresentationChange } from '$lib/viewer';
  import StatusBar from '$lib/components/StatusBar.svelte';
  import CommandPalette from '$lib/components/CommandPalette.svelte';
  import Toast from '$lib/components/Toast.svelte';

  type Tab = 'explorer' | 'analysis' | 'calculations' | 'pipeline';

  let activeTab = $state<Tab>('explorer');
  let viewerContainer = $state<HTMLDivElement>(undefined as unknown as HTMLDivElement);
  let hoverInfo = $state<AtomInfo | null>(null);
  let selectionInfo = $state<AtomInfo | null>(null);
  let commandPaletteOpen = $state(false);
  let panelVisible = $state(true);
  let structureBrowser = $state<StructureBrowser>(undefined as unknown as StructureBrowser);
  let authReady = $state(false);
  let networkSmiles = $state('');
  let networkResidues = $state<any[]>([]);
  let networkJobName = $state('');
  let networkCompoundId = $state('');
  let showNetwork = $state(false);
  let surfaceLegend = $state<string | null>(null);
  let showMDTrajectory = $state(false);
  let mdTrajFrames = $state<string[]>([]);
  let mdTrajEnergy = $state<{ time: number[]; potential: number[]; temperature: number[] } | null>(null);
  let mdTrajCompound = $state('');
  let currentRepr = $state('cartoon');

  const SURFACE_LEGENDS: Record<string, { label: string; items: { color: string; text: string }[] }> = {
    'residue-charge': {
      label: 'Charge',
      items: [
        { color: '#f85149', text: 'Negative (ASP, GLU)' },
        { color: '#3b434d', text: 'Neutral' },
        { color: '#58a6ff', text: 'Positive (LYS, ARG, HIS)' },
      ]
    },
    'hydrophobicity': {
      label: 'Hydrophobicity',
      items: [
        { color: '#58a6ff', text: 'Hydrophilic' },
        { color: '#e6edf3', text: 'Neutral' },
        { color: '#d29922', text: 'Hydrophobic' },
      ]
    },
    'element-symbol': {
      label: 'Element',
      items: [
        { color: '#606870', text: 'Carbon' },
        { color: '#3050F8', text: 'Nitrogen' },
        { color: '#FF0D0D', text: 'Oxygen' },
        { color: '#FFFF30', text: 'Sulfur' },
      ]
    },
  };
  let viewerInitialized = false;

  onMount(() => {
    restoreSession().finally(() => {
      authReady = true;
      if (isAuthenticated()) {
        initApp();
      }
    });
  });

  function initApp() {
    if (viewerInitialized) return;
    viewerInitialized = true;
    // Defer viewer init to next tick so the DOM has rendered the app container
    requestAnimationFrame(() => {
      initViewer();
    });
    setupKeybinds();
  }

  async function initViewer() {
    try {
      await init(viewerContainer);
      onHover((info) => { hoverInfo = info; });
      onClick((info) => { selectionInfo = info; });
      onRepresentationChange((repr) => { currentRepr = repr; });
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
      // Tab switching: 1/2/3 (only when not typing in an input/textarea/select)
      const tag = (e.target as HTMLElement)?.tagName;
      if (!mod && !e.altKey && !e.shiftKey && tag !== 'INPUT' && tag !== 'TEXTAREA' && tag !== 'SELECT') {
        if (e.key === '1') activeTab = 'explorer';
        else if (e.key === '2') activeTab = 'analysis';
        else if (e.key === '3') activeTab = 'calculations';
        else if (e.key === '4') activeTab = 'pipeline';
      }
      // Toggle panel: Ctrl/Cmd+B
      if (mod && e.key === 'b') {
        e.preventDefault();
        panelVisible = !panelVisible;
      }
    });
  }
</script>

{#if !authReady}
  <!-- Loading: prevent flash while auth state is being determined -->
{:else if !isAuthenticated()}
  <div class="splash">
    <div class="splash-content">
      <div class="splash-logo">
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200" width="120" height="120">
          <polygon
            points="100.0,10.0 170.3,43.9 187.7,120.0 139.2,181.2 60.8,181.2 12.3,120.0 29.7,43.9"
            fill="#326ce5"
            stroke="#4a8af4"
            stroke-width="2"
            stroke-linejoin="round"
          />
          <polygon
            points="100,44 148.5,72 148.5,128 100,156 51.5,128 51.5,72"
            fill="none"
            stroke="#ffffff"
            stroke-width="3.5"
            stroke-linejoin="round"
          />
          <line x1="106" y1="55" x2="141" y2="76" stroke="#ffffff" stroke-width="2" stroke-opacity="0.5" />
          <line x1="141" y1="124" x2="106" y2="145" stroke="#ffffff" stroke-width="2" stroke-opacity="0.5" />
          <line x1="59" y1="124" x2="59" y2="76" stroke="#ffffff" stroke-width="2" stroke-opacity="0.5" />
          <text
            x="100"
            y="100"
            text-anchor="middle"
            dominant-baseline="central"
            font-family="-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif"
            font-size="46"
            font-weight="700"
            fill="#ffffff"
          >Kh</text>
        </svg>
      </div>

      <h1 class="splash-title">Khemeia</h1>
      <p class="splash-subtitle">Computational Chemistry Platform</p>

      <button class="splash-signin" onclick={() => login()}>Sign In</button>
    </div>
  </div>
{:else}
  <div class="app">
    <Toolbar bind:activeTab onCommandPalette={() => (commandPaletteOpen = true)} {authReady} />

    <div class="main">
      <div class="viewer-area">
        <div class="viewer-container" bind:this={viewerContainer}></div>

        <div class="repr-dropdown">
          <select class="repr-select" value={currentRepr} onchange={(e) => setRepresentation((e.target as HTMLSelectElement).value)}>
            <option value="cartoon">Ribbon</option>
            <option value="ball-and-stick">Ball & Stick</option>
            <option value="spacefill">Spacefill</option>
            <option value="backbone">Backbone</option>
            <option value="line">Line</option>
            <option value="molecular-surface">Surface</option>
          </select>
        </div>

        {#if selectionInfo}
          <div class="selection-overlay">
            <SelectionInfo info={selectionInfo} />
          </div>
        {/if}

        {#if surfaceLegend && SURFACE_LEGENDS[surfaceLegend]}
          {@const legend = SURFACE_LEGENDS[surfaceLegend]}
          <div class="surface-legend">
            <span class="legend-title">{legend.label}</span>
            {#each legend.items as item}
              <div class="legend-item">
                <span class="legend-swatch" style="background:{item.color}"></span>
                <span class="legend-text">{item.text}</span>
              </div>
            {/each}
          </div>
        {/if}

        {#if showNetwork && networkSmiles && networkResidues.length > 0}
          {#key networkCompoundId}
            <div class="network-overlay">
              <InteractionNetwork
                smiles={networkSmiles}
                residues={networkResidues}
                jobName={networkJobName}
                compoundId={networkCompoundId}
                onResidueClick={(r) => focusResidue(r.chain_id, r.res_id)}
              />
            </div>
          {/key}
        {/if}

        {#if showMDTrajectory && mdTrajFrames.length > 0}
          {#key mdTrajCompound}
            <div class="md-trajectory-overlay">
              <MDTrajectoryOverlay
                frames={mdTrajFrames}
                energy={mdTrajEnergy}
                compoundId={mdTrajCompound}
                onClose={() => showMDTrajectory = false}
              />
            </div>
          {/key}
        {/if}
      </div>

      {#if panelVisible}
        <aside class="side-panel">
          <div class="side-panel-scroll">
            {#if activeTab === 'explorer'}
              <ExplorerPanel onStructureLoad={() => structureBrowser?.refresh()} />
              <StructureBrowser bind:this={structureBrowser} />
            {:else if activeTab === 'analysis'}
              <AnalysisPanel
                onNetworkToggle={(show, smiles, residues, jn, cid) => { showNetwork = show; networkSmiles = smiles; networkResidues = residues; networkJobName = jn; networkCompoundId = cid; }}
                onSurfaceChange={(theme) => { surfaceLegend = theme; }}
              />
            {:else if activeTab === 'calculations'}
              <CalculationsPanel />
            {:else if activeTab === 'pipeline'}
              <PipelinePanel
                onMDView={(frames, energy, compoundId) => {
                  mdTrajFrames = frames;
                  mdTrajEnergy = energy;
                  mdTrajCompound = compoundId;
                  showMDTrajectory = true;
                }}
              />
            {/if}
          </div>
        </aside>
      {/if}
    </div>

    <StatusBar {hoverInfo} />
    <CommandPalette bind:open={commandPaletteOpen} />
    <Toast />
  </div>
{/if}

<style>
  /* ---------- Splash page ---------- */

  .splash {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100vh;
    width: 100vw;
    background: var(--bg-base);
  }

  .splash-content {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0;
    max-width: 400px;
    padding: 32px;
  }

  .splash-logo {
    margin-bottom: 24px;
  }

  .splash-title {
    font-family: var(--font-sans);
    font-size: 42px;
    font-weight: 700;
    color: var(--text-primary);
    letter-spacing: -0.5px;
    margin-bottom: 6px;
  }

  .splash-subtitle {
    font-size: 15px;
    color: var(--text-muted);
    letter-spacing: 0.3px;
    margin-bottom: 32px;
  }

  .splash-features {
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 12px;
    width: 100%;
    margin-bottom: 36px;
  }

  .splash-features li {
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: 14px;
    color: var(--text-secondary);
    padding: 10px 16px;
    border-radius: var(--radius-md);
    background: rgba(255, 255, 255, 0.03);
    border: 1px solid var(--border-default);
  }

  .feature-icon {
    font-size: 16px;
    color: var(--accent);
    width: 20px;
    text-align: center;
    flex-shrink: 0;
  }

  .splash-signin {
    width: 100%;
    padding: 12px 24px;
    font-family: var(--font-sans);
    font-size: 15px;
    font-weight: 600;
    color: var(--bg-base);
    background: var(--accent);
    border: none;
    border-radius: var(--radius-md);
    cursor: pointer;
    transition: background var(--transition-fast);
    margin-bottom: 32px;
  }

  .splash-signin:hover {
    background: var(--accent-hover);
  }

  .splash-footer {
    font-size: 12px;
    color: var(--text-muted);
    letter-spacing: 0.2px;
  }

  /* ---------- App layout ---------- */

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

  .repr-dropdown {
    position: absolute;
    top: 8px;
    right: 8px;
    z-index: 50;
  }

  .repr-select {
    background: rgba(13,17,23,0.8);
    border: 1px solid rgba(48,54,61,0.6);
    color: var(--text-primary, #e6edf3);
    font-size: 11px;
    padding: 4px 8px;
    border-radius: 4px;
    outline: none;
    cursor: pointer;
    backdrop-filter: blur(8px);
  }

  .repr-select:hover {
    border-color: var(--accent, #58a6ff);
  }

  .selection-overlay {
    position: absolute;
    bottom: 12px;
    left: 12px;
    z-index: 50;
  }

  .surface-legend {
    position: absolute;
    bottom: 12px;
    left: 12px;
    z-index: 50;
    background: rgba(13,17,23,0.85);
    border: 1px solid rgba(48,54,61,0.6);
    border-radius: 6px;
    padding: 6px 10px;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .legend-title {
    font-size: 9px;
    font-weight: 600;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.3px;
  }

  .legend-item {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .legend-swatch {
    width: 12px;
    height: 8px;
    border-radius: 2px;
    flex-shrink: 0;
  }

  .legend-text {
    font-size: 10px;
    color: var(--text-secondary, #8b949e);
  }

  .network-overlay {
    position: absolute;
    top: 12px;
    left: 12px;
    z-index: 50;
    width: 420px;
    max-width: 50%;
  }

  .md-trajectory-overlay {
    position: absolute;
    bottom: 12px;
    right: 12px;
    z-index: 50;
    width: 380px;
    max-width: 48%;
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
