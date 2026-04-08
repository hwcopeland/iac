<script lang="ts">
  let { job, pluginSlug }: { job: any; pluginSlug: string } = $props();

  let artifacts = $derived(
    Array.isArray(job?.artifacts) ? job.artifacts : []
  );
</script>

<div class="three-d-tab">
  <div class="placeholder">
    <p class="placeholder-title">3D Visualization</p>
    <p class="placeholder-text">Interactive 3D molecular visualization coming soon.</p>

    {#if artifacts.length > 0}
      <div class="artifacts-list">
        <p class="artifacts-label">Available Artifacts</p>
        {#each artifacts as artifact}
          <div class="artifact-item">
            <span class="artifact-name">{artifact.name || artifact.filename || artifact}</span>
            {#if artifact.type}
              <span class="artifact-type">{artifact.type}</span>
            {/if}
          </div>
        {/each}
      </div>
    {:else}
      <p class="no-artifacts">No artifacts available for this job.</p>
    {/if}
  </div>
</div>

<style>
  .three-d-tab {
    display: flex;
    flex-direction: column;
  }

  .placeholder {
    text-align: center;
    padding: 24px 12px;
  }

  .placeholder-title {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-primary, #e6edf3);
    margin-bottom: 4px;
  }

  .placeholder-text {
    font-size: 12px;
    color: var(--text-muted, #484f58);
    margin-bottom: 16px;
  }

  .artifacts-list {
    text-align: left;
    border: 1px solid rgba(48,54,61,0.4);
    border-radius: 6px;
    overflow: hidden;
    margin-top: 8px;
  }

  .artifacts-label {
    font-size: 10px;
    font-weight: 600;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.3px;
    padding: 6px 10px;
    background: rgba(0,0,0,0.3);
  }

  .artifact-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 5px 10px;
    border-top: 1px solid rgba(48,54,61,0.3);
  }

  .artifact-name {
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    color: var(--text-primary, #e6edf3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .artifact-type {
    font-size: 10px;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    flex-shrink: 0;
    margin-left: 8px;
  }

  .no-artifacts {
    font-size: 11px;
    color: var(--text-muted, #484f58);
  }
</style>
