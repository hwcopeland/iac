<script lang="ts">
  import { loadFile } from '$lib/viewer';

  let { frames, format = 'xyz', onFrameChange }: {
    frames: string[];
    format?: string;
    onFrameChange?: (index: number) => void;
  } = $props();

  let currentFrame = $state(0);
  let playing = $state(false);
  let speed = $state(200); // ms between frames
  let timeoutId = $state<ReturnType<typeof setTimeout> | null>(null);

  /** Load the frame at the given index into the viewer */
  async function showFrame(index: number) {
    if (index < 0 || index >= frames.length) return;
    currentFrame = index;
    onFrameChange?.(index);
    try {
      await loadFile(frames[index], format, true);
    } catch {
      // Frame load failed — keep UI in sync regardless
    }
  }

  function play() {
    playing = true;
    tick();
  }

  function tick() {
    if (!playing) return;
    const next = (currentFrame + 1) % frames.length;
    showFrame(next);
    timeoutId = setTimeout(tick, speed);
  }

  function pause() {
    playing = false;
    if (timeoutId !== null) {
      clearTimeout(timeoutId);
      timeoutId = null;
    }
  }

  function togglePlayPause() {
    if (playing) {
      pause();
    } else {
      play();
    }
  }

  function stepForward() {
    pause();
    showFrame((currentFrame + 1) % frames.length);
  }

  function stepBack() {
    pause();
    showFrame((currentFrame - 1 + frames.length) % frames.length);
  }

  function jumpToFirst() {
    pause();
    showFrame(0);
  }

  function jumpToLast() {
    pause();
    showFrame(frames.length - 1);
  }

  function onSliderInput(e: Event) {
    pause();
    const target = e.target as HTMLInputElement;
    showFrame(parseInt(target.value, 10));
  }
</script>

<div class="trajectory-player">
  <div class="player-label">
    <span class="label-text">Trajectory</span>
    <span class="frame-counter">{currentFrame + 1} / {frames.length}</span>
  </div>

  <div class="player-controls">
    <button class="ctrl-btn" onclick={jumpToFirst} title="First frame" disabled={frames.length <= 1}>
      <svg width="12" height="12" viewBox="0 0 12 12"><path d="M2 2v8M3 6l4-4v8z" fill="currentColor"/></svg>
    </button>
    <button class="ctrl-btn" onclick={stepBack} title="Previous frame" disabled={frames.length <= 1}>
      <svg width="12" height="12" viewBox="0 0 12 12"><path d="M3 6l6-4v8z" fill="currentColor"/></svg>
    </button>
    <button class="ctrl-btn play-btn" onclick={togglePlayPause} title={playing ? 'Pause' : 'Play'} disabled={frames.length <= 1}>
      {#if playing}
        <svg width="12" height="12" viewBox="0 0 12 12"><rect x="2" y="2" width="3" height="8" fill="currentColor"/><rect x="7" y="2" width="3" height="8" fill="currentColor"/></svg>
      {:else}
        <svg width="12" height="12" viewBox="0 0 12 12"><path d="M3 2l7 4-7 4z" fill="currentColor"/></svg>
      {/if}
    </button>
    <button class="ctrl-btn" onclick={stepForward} title="Next frame" disabled={frames.length <= 1}>
      <svg width="12" height="12" viewBox="0 0 12 12"><path d="M9 6l-6-4v8z" fill="currentColor"/></svg>
    </button>
    <button class="ctrl-btn" onclick={jumpToLast} title="Last frame" disabled={frames.length <= 1}>
      <svg width="12" height="12" viewBox="0 0 12 12"><path d="M10 2v8M9 6l-4-4v8z" fill="currentColor"/></svg>
    </button>
  </div>

  <input
    type="range"
    class="frame-slider"
    min="0"
    max={frames.length - 1}
    value={currentFrame}
    oninput={onSliderInput}
    disabled={frames.length <= 1}
  />

  <div class="speed-control">
    <span class="speed-label">Speed</span>
    <select class="speed-select" bind:value={speed} onchange={() => { if (playing) { pause(); play(); } }}>
      <option value={500}>0.5x</option>
      <option value={200}>1x</option>
      <option value={100}>2x</option>
      <option value={50}>4x</option>
    </select>
  </div>
</div>

<style>
  .trajectory-player {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 8px 10px;
    border: 1px solid rgba(48,54,61,0.4);
    border-radius: 6px;
    background: rgba(0,0,0,0.2);
  }

  .player-label {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .label-text {
    font-size: 10px;
    font-weight: 600;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.3px;
  }

  .frame-counter {
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    color: var(--text-secondary, #8b949e);
  }

  .player-controls {
    display: flex;
    justify-content: center;
    gap: 4px;
  }

  .ctrl-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 24px;
    background: rgba(48,54,61,0.4);
    border: 1px solid rgba(48,54,61,0.6);
    border-radius: 4px;
    color: var(--text-secondary, #8b949e);
    cursor: pointer;
    transition: all 0.15s;
  }

  .ctrl-btn:hover:not(:disabled) {
    background: rgba(88,166,255,0.1);
    color: var(--text-primary, #e6edf3);
    border-color: rgba(88,166,255,0.3);
  }

  .ctrl-btn:disabled {
    opacity: 0.3;
    cursor: default;
  }

  .ctrl-btn.play-btn {
    width: 32px;
    background: rgba(88,166,255,0.1);
    border-color: rgba(88,166,255,0.3);
    color: var(--accent, #58a6ff);
  }

  .ctrl-btn.play-btn:hover:not(:disabled) {
    background: rgba(88,166,255,0.2);
  }

  .frame-slider {
    width: 100%;
    height: 4px;
    appearance: none;
    background: rgba(48,54,61,0.6);
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

  .frame-slider:disabled {
    opacity: 0.3;
    cursor: default;
  }

  .speed-control {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 6px;
  }

  .speed-label {
    font-size: 10px;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.3px;
  }

  .speed-select {
    font-size: 10px;
    font-family: 'SF Mono', monospace;
    background: rgba(48,54,61,0.4);
    border: 1px solid rgba(48,54,61,0.6);
    border-radius: 3px;
    color: var(--text-secondary, #8b949e);
    padding: 2px 4px;
    cursor: pointer;
  }

  .speed-select:hover {
    border-color: rgba(88,166,255,0.3);
  }
</style>
