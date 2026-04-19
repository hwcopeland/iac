<script lang="ts">
  import { onMount } from 'svelte';
  import { getInteractionMap } from '$lib/api';
  import { getSVG } from '$lib/rdkit';
  import type { PocketResidue } from '$lib/api';

  let { smiles, residues, jobName, compoundId, onResidueClick }:
    {
      smiles: string;
      residues: PocketResidue[];
      jobName?: string;
      compoundId?: string;
      onResidueClick?: (r: PocketResidue) => void;
    } = $props();

  let prolifSvg = $state('');
  let fallbackSvg = $state('');
  let loading = $state(true);
  let useProLIF = $state(false);

  const IX_COLORS: Record<string, string> = {
    hbond: '#58a6ff',
    hydrophobic: '#8b949e',
    ionic: '#d29922',
    dipole: '#bb33bb',
    contact: '#555555',
  };

  const IX_LABELS: Record<string, string> = {
    hbond: 'H-bond',
    hydrophobic: 'Hydrophobic',
    ionic: 'Ionic',
    dipole: 'Dipole',
    contact: 'Contact',
  };

  function primaryInteraction(r: PocketResidue): string {
    for (const t of ['hbond', 'ionic', 'dipole', 'hydrophobic']) {
      if (r.interactions.includes(t)) return t;
    }
    return 'contact';
  }

  onMount(async () => {
    // Try ProLIF first
    if (jobName && compoundId) {
      try {
        const res = await getInteractionMap(jobName, compoundId);
        if (res?.svg) {
          prolifSvg = res.svg;
          useProLIF = true;
          loading = false;
          return;
        }
      } catch {
        // ProLIF not available, fall back
      }
    }

    // Fallback: RDKit WASM ligand SVG + custom diagram
    if (smiles) {
      const svg = await getSVG(smiles, 250, 200);
      if (svg) {
        fallbackSvg = svg
          .replace(/fill:\s*#FFFFFF/gi, 'fill:transparent')
          .replace(/fill="#FFFFFF"/gi, 'fill="transparent"')
          .replace(/fill="white"/gi, 'fill="transparent"')
          .replace(/<rect[^>]*style='[^']*fill:\s*#FFFFFF[^']*'[^>]*\/>/gi, '');
      }
    }
    loading = false;
  });

  const CX = 250;
  const CY = 175;
  const RADIUS = 140;

  let residuePositions = $derived(
    residues.slice(0, 16).map((r, i) => {
      const n = Math.min(residues.length, 16);
      const angle = (i / n) * 2 * Math.PI - Math.PI / 2;
      return {
        ...r,
        x: CX + RADIUS * Math.cos(angle),
        y: CY + RADIUS * Math.sin(angle),
        color: IX_COLORS[primaryInteraction(r)] || '#555',
        ix: primaryInteraction(r),
      };
    })
  );
</script>

<div class="interaction-network">
  {#if loading}
    <div class="net-loading">Loading interaction map...</div>
  {:else if useProLIF && prolifSvg}
    <div class="prolif-svg-wrap">
      {@html prolifSvg}
    </div>
  {:else}
    <svg viewBox="0 0 500 350" class="net-svg">
      <rect width="500" height="350" fill="#0d1117" rx="8" />

      {#each residuePositions as rp, i}
        {@const spread = 40}
        {@const angle = (i / Math.max(residuePositions.length, 1)) * 2 * Math.PI}
        {@const lx = CX + spread * Math.cos(angle)}
        {@const ly = CY + spread * Math.sin(angle)}
        <line
          x1={lx} y1={ly} x2={rp.x} y2={rp.y}
          stroke={rp.color}
          stroke-width="2"
          stroke-dasharray="6 4"
          opacity="0.5"
        />
      {/each}

      <foreignObject x={CX - 125} y={CY - 100} width="250" height="200">
        <div class="ligand-svg-wrap">
          {#if fallbackSvg}
            {@html fallbackSvg}
          {:else}
            <div class="ligand-placeholder">Ligand</div>
          {/if}
        </div>
      </foreignObject>

      {#each residuePositions as rp}
        <g
          class="res-node"
          transform="translate({rp.x},{rp.y})"
          onclick={() => onResidueClick?.(rp)}
          role="button"
          tabindex="0"
        >
          <circle r="26" fill="rgba(13,17,23,0.95)" stroke={rp.color} stroke-width="2" />
          <text text-anchor="middle" dy="-6" fill={rp.color} font-size="11" font-weight="700" font-family="system-ui, sans-serif">
            {rp.res_name}
          </text>
          <text text-anchor="middle" dy="8" fill="#c9d1d9" font-size="10" font-family="SF Mono, monospace">
            {rp.chain_id}{rp.res_id}
          </text>
          <text text-anchor="middle" dy="20" fill={rp.color} font-size="9" opacity="0.8">
            {rp.min_distance.toFixed(1)}A
          </text>
        </g>
      {/each}

      {#each Object.entries(IX_COLORS).filter(([k]) => residuePositions.some(r => r.ix === k)) as [type, color], i}
        <g transform="translate(10,{14 + i * 16})">
          <line x1="0" y1="0" x2="16" y2="0" stroke={color} stroke-width="2.5" stroke-dasharray="4 3" />
          <text x="22" y="4" fill={color} font-size="10" font-family="system-ui, sans-serif">{IX_LABELS[type]}</text>
        </g>
      {/each}
    </svg>
  {/if}
</div>

<style>
  .interaction-network {
    border: 1px solid rgba(48,54,61,0.6);
    border-radius: 8px;
    background: #0d1117;
    overflow: hidden;
    box-shadow: 0 4px 24px rgba(0,0,0,0.5);
  }

  .net-loading {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 150px;
    color: var(--text-muted, #484f58);
    font-size: 12px;
  }

  .prolif-svg-wrap {
    width: 100%;
    padding: 8px;
  }

  .prolif-svg-wrap :global(svg) {
    width: 100%;
    height: auto;
    display: block;
  }

  .net-svg {
    width: 100%;
    height: auto;
    display: block;
  }

  .ligand-svg-wrap {
    width: 250px;
    height: 200px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .ligand-svg-wrap :global(svg) {
    width: 100%;
    height: 100%;
    background: transparent !important;
  }

  .ligand-svg-wrap :global(rect) {
    fill: transparent !important;
  }

  .ligand-placeholder {
    color: var(--text-muted, #484f58);
    font-size: 12px;
  }

  .res-node {
    cursor: pointer;
  }

  .res-node:hover circle {
    fill: rgba(88,166,255,0.15);
  }
</style>
