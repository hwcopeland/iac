<script lang="ts">
  import type { AtomInfo } from '$lib/viewer';

  let { hoverInfo }: { hoverInfo: AtomInfo | null } = $props();
</script>

<footer class="status-bar">
  {#if hoverInfo?.distance != null}
    <span class="status-interaction">Interaction</span>
    <span class="status-sep">&middot;</span>
    <span class="status-residue">{hoverInfo.residueName} {hoverInfo.residueId}.{hoverInfo.chainId}</span>
    <span class="status-sep">&mdash;</span>
    <span class="status-partner">{hoverInfo.partnerResidue ?? 'ligand'}</span>
    <span class="status-sep">&middot;</span>
    <span class="status-distance">{hoverInfo.distance.toFixed(2)} A</span>
  {:else if hoverInfo}
    <span class="status-element">{hoverInfo.element}</span>
    <span class="status-atom">{hoverInfo.atomName}</span>
    <span class="status-sep">&middot;</span>
    <span class="status-residue">{hoverInfo.residueName} {hoverInfo.residueId}</span>
    <span class="status-sep">&middot;</span>
    <span class="status-chain">Chain {hoverInfo.chainId}</span>
    <span class="status-sep">&middot;</span>
    <span class="status-coords">({hoverInfo.x.toFixed(1)}, {hoverInfo.y.toFixed(1)}, {hoverInfo.z.toFixed(1)})</span>
  {:else}
    <span class="status-idle">Hover over atoms to see details</span>
  {/if}
</footer>

<style>
  .status-bar {
    display: flex;
    align-items: center;
    gap: 6px;
    height: 24px;
    padding: 0 12px;
    background: var(--bg-surface);
    backdrop-filter: var(--panel-blur);
    border-top: 1px solid var(--border-default);
    font-size: 11px;
    font-family: var(--font-mono);
    flex-shrink: 0;
    z-index: 100;
  }

  .status-element {
    color: var(--accent);
    font-weight: 700;
  }

  .status-atom {
    color: var(--text-primary);
  }

  .status-sep {
    color: var(--text-muted);
  }

  .status-residue {
    color: var(--success);
  }

  .status-chain {
    color: var(--text-secondary);
  }

  .status-coords {
    color: var(--text-muted);
  }

  .status-interaction {
    color: #d29922;
    font-weight: 700;
  }

  .status-partner {
    color: var(--accent);
  }

  .status-distance {
    color: var(--text-primary);
    font-weight: 600;
  }

  .status-idle {
    color: var(--text-muted);
    font-family: var(--font-sans);
  }
</style>
