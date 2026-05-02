<script lang="ts">
  import { loadFullTrajectory, gotoTrajectoryFrame, focusLigand } from '$lib/viewer';
  import PlotlyChart from './charts/PlotlyChart.svelte';

  let {
    frames,
    energy,
    compoundId,
    onClose,
  }: {
    frames: string[];
    energy: { time: number[]; potential: number[]; temperature: number[] } | null;
    compoundId: string;
    onClose: () => void;
  } = $props();

  let currentFrame = $state(0);
  let playing = $state(false);
  let speed = $state(200);
  let tickId = $state<ReturnType<typeof setTimeout> | null>(null);
  // trajReady: full multi-model PDB has been loaded into Molstar.
  // trajLoading: initial parse in progress (can be seconds for large files).
  let trajReady = $state(false);
  let trajLoading = $state(false);
  let showEnergy = $state(true);
  let frameError = $state<string | null>(null);

  async function showFrame(index: number) {
    if (!trajReady || index < 0 || index >= frames.length) return;
    currentFrame = index;
    frameError = null;
    try {
      await gotoTrajectoryFrame(index);
    } catch (err: any) {
      console.error('[MDTraj] gotoTrajectoryFrame error:', err);
      frameError = err?.message ?? String(err);
    }
  }

  function tick() {
    if (!playing) return;
    const next = (currentFrame + 1) % frames.length;
    showFrame(next).then(() => {
      if (playing) tickId = setTimeout(tick, speed);
    });
  }

  function play() {
    if (!trajReady) return;
    playing = true;
    tickId = setTimeout(tick, speed);
  }

  function pause() {
    playing = false;
    if (tickId !== null) { clearTimeout(tickId); tickId = null; }
  }

  function togglePlay() { playing ? pause() : play(); }
  function stepBack()    { pause(); showFrame((currentFrame - 1 + frames.length) % frames.length); }
  function stepForward() { pause(); showFrame((currentFrame + 1) % frames.length); }

  function onSlider(e: Event) {
    pause();
    showFrame(parseInt((e.target as HTMLInputElement).value, 10));
  }

  // Load all frames as a single multi-model PDB on mount.
  // Navigation is then O(ms) via ModelFromTrajectory modelIndex update.
  $effect(() => {
    if (frames.length === 0) return;
    const fullPdb = frames.join('');
    trajLoading = true;
    trajReady = false;
    frameError = null;
    loadFullTrajectory(fullPdb)
      .then(() => { trajReady = true; focusLigand(); })
      .catch((err: any) => {
        console.error('[MDTraj] loadFullTrajectory error:', err);
        frameError = err?.message ?? String(err);
      })
      .finally(() => { trajLoading = false; });
  });

  let energyChartData = $derived.by(() => {
    if (!energy) return [];
    const series: any[] = [{
      x: energy.time,
      y: energy.potential,
      type: 'scatter',
      mode: 'lines',
      name: 'Potential (kJ/mol)',
      line: { color: '#58a6ff', width: 1.5 },
      yaxis: 'y',
    }];
    if (energy.temperature.length > 0) {
      series.push({
        x: energy.time,
        y: energy.temperature,
        type: 'scatter',
        mode: 'lines',
        name: 'Temp (K)',
        line: { color: '#d29922', width: 1, dash: 'dot' },
        yaxis: 'y2',
      });
    }
    return series;
  });

  const energyLayout = {
    paper_bgcolor: 'rgba(0,0,0,0)',
    plot_bgcolor: 'rgba(0,0,0,0)',
    font: { color: '#8b949e', size: 10 },
    xaxis: {
      title: { text: 'Time (ps)', standoff: 2, font: { size: 9 } },
      color: '#484f58', gridcolor: 'rgba(48,54,61,0.4)', zerolinecolor: 'rgba(48,54,61,0.4)',
      tickfont: { size: 9 },
    },
    yaxis: {
      title: { text: 'E (kJ/mol)', standoff: 2, font: { size: 9 } },
      color: '#484f58', gridcolor: 'rgba(48,54,61,0.4)', side: 'left',
      tickfont: { size: 9 }, exponentformat: 'SI', showexponent: 'all',
    },
    yaxis2: {
      title: { text: 'T (K)', standoff: 2, font: { size: 9 } },
      color: '#484f58', overlaying: 'y', side: 'right', showgrid: false,
      tickfont: { size: 9 }, range: [270, 330],
    },
    legend: { font: { size: 9 }, bgcolor: 'rgba(0,0,0,0)', orientation: 'h', x: 0, y: 1.15 },
    margin: { t: 20, b: 32, l: 52, r: 44 },
    height: 150,
    showlegend: true,
  };
</script>

<div class="md-overlay">
  <!-- Header -->
  <div class="overlay-header">
    <span class="overlay-title">MD Trajectory</span>
    <span class="overlay-compound">{compoundId}</span>
    <div class="overlay-actions">
      {#if energy}
        <button class="icon-btn" class:active={showEnergy} onclick={() => showEnergy = !showEnergy} title="Toggle energy chart">
          <svg width="12" height="12" viewBox="0 0 12 12"><polyline points="1,9 4,5 7,7 11,2" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>
        </button>
      {/if}
      <button class="icon-btn close-btn" onclick={onClose} title="Close">
        <svg width="10" height="10" viewBox="0 0 10 10"><line x1="1" y1="1" x2="9" y2="9" stroke="currentColor" stroke-width="1.5"/><line x1="9" y1="1" x2="1" y2="9" stroke="currentColor" stroke-width="1.5"/></svg>
      </button>
    </div>
  </div>

  <!-- Energy chart -->
  {#if showEnergy && energy && energy.time.length > 0}
    <div class="energy-section">
      <PlotlyChart data={energyChartData} layout={energyLayout} />
    </div>
  {/if}

  <!-- Trajectory load state / error -->
  {#if trajLoading}
    <div class="traj-status">
      <span class="loading-dot"></span> Parsing trajectory…
    </div>
  {:else if frameError}
    <div class="frame-error">{frameError}</div>
  {/if}

  <!-- Playback controls -->
  <div class="controls">
    <div class="ctrl-row">
      <button class="ctrl-btn" onclick={() => { pause(); showFrame(0); }} title="First frame" disabled={!trajReady || frames.length <= 1}>
        <svg width="11" height="11" viewBox="0 0 11 11"><line x1="1" y1="1" x2="1" y2="10" stroke="currentColor" stroke-width="1.5"/><path d="M3 5.5l6-4v8z" fill="currentColor"/></svg>
      </button>
      <button class="ctrl-btn" onclick={stepBack} title="Previous" disabled={!trajReady || frames.length <= 1}>
        <svg width="11" height="11" viewBox="0 0 11 11"><path d="M9 5.5l-7-4v8z" fill="currentColor"/></svg>
      </button>
      <button class="ctrl-btn play-btn" onclick={togglePlay} title={playing ? 'Pause' : 'Play'} disabled={!trajReady || frames.length <= 1}>
        {#if playing}
          <svg width="11" height="11" viewBox="0 0 11 11"><rect x="1" y="1" width="3" height="9" rx="1" fill="currentColor"/><rect x="7" y="1" width="3" height="9" rx="1" fill="currentColor"/></svg>
        {:else}
          <svg width="11" height="11" viewBox="0 0 11 11"><path d="M2 1l8 4.5-8 4.5z" fill="currentColor"/></svg>
        {/if}
      </button>
      <button class="ctrl-btn" onclick={stepForward} title="Next" disabled={!trajReady || frames.length <= 1}>
        <svg width="11" height="11" viewBox="0 0 11 11"><path d="M2 5.5l7-4v8z" fill="currentColor"/></svg>
      </button>
      <button class="ctrl-btn" onclick={() => { pause(); showFrame(frames.length - 1); }} title="Last frame" disabled={!trajReady || frames.length <= 1}>
        <svg width="11" height="11" viewBox="0 0 11 11"><line x1="10" y1="1" x2="10" y2="10" stroke="currentColor" stroke-width="1.5"/><path d="M8 5.5l-6-4v8z" fill="currentColor"/></svg>
      </button>

      <span class="frame-counter">
        {currentFrame + 1} / {frames.length}
      </span>

      <select class="speed-select" bind:value={speed} onchange={() => { if (playing) { pause(); play(); } }}>
        <option value={500}>0.5×</option>
        <option value={200}>1×</option>
        <option value={100}>2×</option>
        <option value={50}>4×</option>
      </select>
    </div>

    <input
      type="range"
      class="frame-slider"
      min="0"
      max={frames.length - 1}
      value={currentFrame}
      oninput={onSlider}
      disabled={frames.length <= 1}
    />
  </div>
</div>

<style>
  .md-overlay {
    background: rgba(13, 17, 23, 0.92);
    border: 1px solid rgba(48, 54, 61, 0.7);
    border-radius: 8px;
    backdrop-filter: blur(12px);
    display: flex;
    flex-direction: column;
    gap: 0;
    overflow: hidden;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.6);
  }

  .overlay-header {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 7px 10px;
    border-bottom: 1px solid rgba(48, 54, 61, 0.5);
    background: rgba(0, 0, 0, 0.2);
  }

  .overlay-title {
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.4px;
    color: var(--text-muted, #484f58);
    flex-shrink: 0;
  }

  .overlay-compound {
    font-size: 11px;
    font-family: 'SF Mono', monospace;
    color: var(--accent, #58a6ff);
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .overlay-actions {
    display: flex;
    gap: 4px;
    flex-shrink: 0;
  }

  .icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    background: none;
    border: 1px solid rgba(48, 54, 61, 0.5);
    border-radius: 3px;
    color: var(--text-muted, #484f58);
    cursor: pointer;
    transition: all 0.15s;
  }

  .icon-btn:hover { border-color: rgba(88, 166, 255, 0.4); color: var(--accent, #58a6ff); }
  .icon-btn.active { border-color: rgba(88, 166, 255, 0.5); color: var(--accent, #58a6ff); background: rgba(88, 166, 255, 0.08); }
  .close-btn:hover { border-color: rgba(248, 81, 73, 0.4); color: #f85149; }

  .energy-section {
    border-bottom: 1px solid rgba(48, 54, 61, 0.4);
    padding: 4px 0;
  }

  .controls {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 8px 10px;
  }

  .ctrl-row {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .ctrl-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 22px;
    background: rgba(48, 54, 61, 0.4);
    border: 1px solid rgba(48, 54, 61, 0.6);
    border-radius: 4px;
    color: var(--text-secondary, #8b949e);
    cursor: pointer;
    flex-shrink: 0;
    transition: all 0.12s;
  }

  .ctrl-btn:hover:not(:disabled) {
    background: rgba(88, 166, 255, 0.1);
    border-color: rgba(88, 166, 255, 0.3);
    color: var(--text-primary, #e6edf3);
  }

  .ctrl-btn:disabled { opacity: 0.3; cursor: default; }

  .ctrl-btn.play-btn {
    width: 30px;
    background: rgba(88, 166, 255, 0.1);
    border-color: rgba(88, 166, 255, 0.3);
    color: var(--accent, #58a6ff);
  }

  .ctrl-btn.play-btn:hover:not(:disabled) { background: rgba(88, 166, 255, 0.2); }

  .frame-counter {
    flex: 1;
    font-size: 11px;
    font-family: 'SF Mono', monospace;
    color: var(--text-secondary, #8b949e);
    text-align: center;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
  }

  .loading-dot {
    width: 5px;
    height: 5px;
    background: var(--accent, #58a6ff);
    border-radius: 50%;
    animation: pulse 0.8s ease-in-out infinite;
  }

  @keyframes pulse {
    0%, 100% { opacity: 0.3; }
    50% { opacity: 1; }
  }

  .speed-select {
    font-size: 10px;
    font-family: 'SF Mono', monospace;
    background: rgba(48, 54, 61, 0.4);
    border: 1px solid rgba(48, 54, 61, 0.6);
    border-radius: 3px;
    color: var(--text-secondary, #8b949e);
    padding: 2px 4px;
    cursor: pointer;
    flex-shrink: 0;
  }

  .frame-slider {
    width: 100%;
    height: 3px;
    appearance: none;
    background: rgba(48, 54, 61, 0.6);
    border-radius: 2px;
    outline: none;
    cursor: pointer;
  }

  .frame-slider::-webkit-slider-thumb {
    appearance: none;
    width: 12px;
    height: 12px;
    background: var(--accent, #58a6ff);
    border-radius: 50%;
    cursor: pointer;
  }

  .frame-slider:disabled { opacity: 0.3; cursor: default; }

  .traj-status {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 10px;
    color: var(--text-muted, #484f58);
    padding: 4px 10px;
    border-bottom: 1px solid rgba(48, 54, 61, 0.3);
  }

  .frame-error {
    font-size: 10px;
    color: #f85149;
    padding: 4px 10px;
    border-bottom: 1px solid rgba(248, 81, 73, 0.2);
    background: rgba(248, 81, 73, 0.06);
    word-break: break-word;
  }
</style>
