<script lang="ts">
  import { downloadArtifact } from '$lib/api';
  import type { ArtifactSummary } from '$lib/api';
  import { loadCubeFile, overlayStructure, isReady } from '$lib/viewer';
  import TrajectoryPlayer from './TrajectoryPlayer.svelte';

  let { job, pluginSlug }: { job: any; pluginSlug: string } = $props();

  let loading = $state('');
  let errorMsg = $state('');

  let artifacts: ArtifactSummary[] = $derived(
    Array.isArray(job?.artifacts) ? job.artifacts : []
  );

  let dockingResults = $derived(
    Array.isArray(job?.docking_results) ? job.docking_results : []
  );

  // Classify artifacts by file extension
  function getArtifactType(filename: string): 'cube' | 'pose' | 'other' {
    const lower = filename.toLowerCase();
    if (lower.endsWith('.cube')) return 'cube';
    if (lower.endsWith('.pdbqt') || lower.endsWith('.pdb') || lower.endsWith('.sdf') || lower.endsWith('.mol') || lower.endsWith('.mol2')) return 'pose';
    return 'other';
  }

  function getFormatFromFilename(filename: string): string {
    const ext = filename.split('.').pop()?.toLowerCase() || '';
    return ext;
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }

  let cubeArtifacts = $derived(artifacts.filter(a => getArtifactType(a.filename) === 'cube'));
  let poseArtifacts = $derived(artifacts.filter(a => getArtifactType(a.filename) === 'pose'));
  let otherArtifacts = $derived(artifacts.filter(a => getArtifactType(a.filename) === 'other'));

  // ─── Trajectory Support ───

  /**
   * Extract trajectory frames from job data.
   * Source 1: optimization_trajectory in output_data (future: reduce:blocks plugin mode)
   * Source 2: multiple XYZ artifacts with inline content
   */
  let trajectoryFrames = $derived.by(() => {
    const traj = job?.output_data?.optimization_trajectory;
    if (Array.isArray(traj) && traj.length > 1) {
      return traj as string[];
    }
    // Multiple XYZ artifacts with embedded content
    const xyzArtifacts = artifacts.filter(
      (a: ArtifactSummary) => a.filename.toLowerCase().endsWith('.xyz')
    );
    if (xyzArtifacts.length > 1 && xyzArtifacts.every((a: any) => a.content)) {
      return xyzArtifacts.map((a: any) => a.content as string);
    }
    return [];
  });

  let hasTrajectory = $derived(trajectoryFrames.length > 1);

  let trajectoryFormat = $derived.by(() => {
    if (job?.output_data?.optimization_trajectory) return 'xyz';
    return 'xyz';
  });

  // ─── Vibration Mode Detection ───

  let hasFrequencyData = $derived(
    Array.isArray(job?.output_data?.ir_frequencies) && job.output_data.ir_frequencies.length > 0
  );

  let moldenArtifact = $derived(
    artifacts.find((a: ArtifactSummary) => a.filename.toLowerCase().endsWith('.molden'))
  );

  async function renderCube(artifact: ArtifactSummary) {
    if (!isReady()) {
      errorMsg = 'Viewer not initialized. Load a structure first.';
      return;
    }
    loading = artifact.filename;
    errorMsg = '';
    try {
      const data = await downloadArtifact(pluginSlug, job.name, artifact.filename);
      const text = new TextDecoder().decode(data);
      await loadCubeFile(text);
    } catch (e: any) {
      errorMsg = `Failed to render ${artifact.filename}: ${e.message || e}`;
      console.error('Failed to render cube:', e);
    } finally {
      loading = '';
    }
  }

  async function loadPose(artifact: ArtifactSummary) {
    if (!isReady()) {
      errorMsg = 'Viewer not initialized. Load a structure first.';
      return;
    }
    loading = artifact.filename;
    errorMsg = '';
    try {
      const data = await downloadArtifact(pluginSlug, job.name, artifact.filename);
      const text = new TextDecoder().decode(data);
      const fmt = getFormatFromFilename(artifact.filename);
      await overlayStructure(text, fmt);
    } catch (e: any) {
      errorMsg = `Failed to load ${artifact.filename}: ${e.message || e}`;
      console.error('Failed to load pose:', e);
    } finally {
      loading = '';
    }
  }
</script>

<div class="three-d-tab">
  {#if errorMsg}
    <div class="error-banner">
      <span class="error-text">{errorMsg}</span>
      <button class="error-dismiss" onclick={() => { errorMsg = ''; }}>x</button>
    </div>
  {/if}

  {#if hasTrajectory}
    <TrajectoryPlayer
      frames={trajectoryFrames}
      format={trajectoryFormat}
    />
  {/if}

  {#if hasFrequencyData}
    <div class="vibration-note">
      <span class="note-icon">~</span>
      <div class="note-content">
        <p class="note-title">Vibration Modes</p>
        <p class="note-text">
          Vibration mode animation requires normal mode displacement vectors.
          {#if moldenArtifact}
            A Molden file is available in artifacts for external visualization.
          {:else}
            Export a Molden file from your calculation for visualization in Avogadro or Gabedit.
          {/if}
        </p>
      </div>
    </div>
  {/if}

  {#if cubeArtifacts.length > 0}
    <div class="artifact-group">
      <p class="group-label">Volumetric Data</p>
      {#each cubeArtifacts as artifact}
        <div class="artifact-row">
          <div class="artifact-info">
            <span class="artifact-name">{artifact.filename}</span>
            <span class="artifact-size">{formatSize(artifact.size_bytes)}</span>
          </div>
          <button
            class="action-btn"
            disabled={loading === artifact.filename}
            onclick={() => renderCube(artifact)}
          >
            {loading === artifact.filename ? 'Loading...' : 'Render'}
          </button>
        </div>
      {/each}
    </div>
  {/if}

  {#if poseArtifacts.length > 0}
    <div class="artifact-group">
      <p class="group-label">Docked Poses</p>
      {#each poseArtifacts as artifact}
        <div class="artifact-row">
          <div class="artifact-info">
            <span class="artifact-name">{artifact.filename}</span>
            <span class="artifact-size">{formatSize(artifact.size_bytes)}</span>
          </div>
          <button
            class="action-btn"
            disabled={loading === artifact.filename}
            onclick={() => loadPose(artifact)}
          >
            {loading === artifact.filename ? 'Loading...' : 'Overlay'}
          </button>
        </div>
      {/each}
    </div>
  {/if}

  {#if dockingResults.length > 0 && poseArtifacts.length === 0}
    <div class="artifact-group">
      <p class="group-label">Docking Results</p>
      <p class="hint-text">
        {dockingResults.length} compounds docked. Pose files are saved for top hits
        (below threshold).
      </p>
    </div>
  {/if}

  {#if otherArtifacts.length > 0}
    <div class="artifact-group">
      <p class="group-label">Other Artifacts</p>
      {#each otherArtifacts as artifact}
        <div class="artifact-row">
          <div class="artifact-info">
            <span class="artifact-name">{artifact.filename}</span>
            <span class="artifact-size">{formatSize(artifact.size_bytes)}</span>
          </div>
          <span class="artifact-type">{artifact.content_type.split('/').pop()}</span>
        </div>
      {/each}
    </div>
  {/if}

  {#if artifacts.length === 0 && dockingResults.length === 0}
    <p class="no-artifacts">No 3D artifacts available for this job.</p>
  {/if}
</div>

<style>
  .three-d-tab {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .error-banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    background: rgba(248,81,73,0.1);
    border: 1px solid rgba(248,81,73,0.3);
    border-radius: 6px;
    padding: 6px 10px;
  }

  .error-text {
    font-size: 11px;
    color: #f85149;
  }

  .error-dismiss {
    background: none;
    border: none;
    color: #f85149;
    cursor: pointer;
    font-size: 12px;
    padding: 0 4px;
    opacity: 0.7;
  }

  .error-dismiss:hover {
    opacity: 1;
  }

  .artifact-group {
    border: 1px solid rgba(48,54,61,0.4);
    border-radius: 6px;
    overflow: hidden;
  }

  .group-label {
    font-size: 10px;
    font-weight: 600;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.3px;
    padding: 6px 10px;
    background: rgba(0,0,0,0.3);
    margin: 0;
  }

  .artifact-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 5px 10px;
    border-top: 1px solid rgba(48,54,61,0.3);
  }

  .artifact-info {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    flex: 1;
  }

  .artifact-name {
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    color: var(--text-primary, #e6edf3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .artifact-size {
    font-size: 10px;
    color: var(--text-muted, #484f58);
    flex-shrink: 0;
  }

  .artifact-type {
    font-size: 10px;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    flex-shrink: 0;
  }

  .action-btn {
    font-size: 10px;
    font-weight: 600;
    color: var(--accent, #58a6ff);
    background: rgba(88,166,255,0.08);
    border: 1px solid rgba(88,166,255,0.2);
    border-radius: 4px;
    padding: 3px 10px;
    cursor: pointer;
    transition: all 0.15s;
    flex-shrink: 0;
    margin-left: 8px;
  }

  .action-btn:hover:not(:disabled) {
    background: rgba(88,166,255,0.15);
    border-color: rgba(88,166,255,0.4);
  }

  .action-btn:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .hint-text {
    font-size: 11px;
    color: var(--text-muted, #484f58);
    padding: 6px 10px;
    margin: 0;
    border-top: 1px solid rgba(48,54,61,0.3);
  }

  .no-artifacts {
    font-size: 11px;
    color: var(--text-muted, #484f58);
    text-align: center;
    padding: 24px 12px;
  }

  .vibration-note {
    display: flex;
    gap: 8px;
    padding: 8px 10px;
    border: 1px solid rgba(210,153,34,0.2);
    border-radius: 6px;
    background: rgba(210,153,34,0.05);
  }

  .note-icon {
    font-family: 'SF Mono', monospace;
    font-size: 14px;
    font-weight: 700;
    color: #d29922;
    flex-shrink: 0;
    line-height: 1.2;
  }

  .note-content {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .note-title {
    font-size: 11px;
    font-weight: 600;
    color: #d29922;
    margin: 0;
  }

  .note-text {
    font-size: 11px;
    color: var(--text-secondary, #8b949e);
    margin: 0;
    line-height: 1.4;
  }
</style>
