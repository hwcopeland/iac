<script lang="ts">
  import { onMount } from 'svelte';
  import { getSVG } from '$lib/rdkit';
  import type { PocketResidue } from '$lib/api';

  let { smiles, residues, onResidueClick }:
    { smiles: string; residues: PocketResidue[]; onResidueClick?: (r: PocketResidue) => void } = $props();

  let ligandSvg = $state('');
  let loading = $state(true);

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
    if (smiles) {
      const svg = await getSVG(smiles, 250, 200);
      if (svg) {
        // Strip white background from RDKit SVG
        ligandSvg = svg
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
    <div class="net-loading">Loading...</div>
  {:else}
    <svg viewBox="0 0 500 350" class="net-svg">
      <!-- Background -->
      <rect width="500" height="350" fill="#0d1117" rx="8" />

      <!-- Interaction lines -->
      {#each residuePositions as rp}
        <line
          x1={CX} y1={CY} x2={rp.x} y2={rp.y}
          stroke={rp.color}
          stroke-width="2"
          stroke-dasharray="6 4"
          opacity="0.5"
        />
      {/each}

      <!-- Ligand 2D depiction -->
      <foreignObject x={CX - 125} y={CY - 100} width="250" height="200">
        <div class="ligand-svg-wrap">
          {#if ligandSvg}
            {@html ligandSvg}
          {:else}
            <div class="ligand-placeholder">Ligand</div>
          {/if}
        </div>
      </foreignObject>

      <!-- Residue nodes -->
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

      <!-- Legend -->
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
