<script lang="ts">
  import { onMount } from 'svelte';
  import { getSVG } from '$lib/rdkit';
  import type { PocketResidue } from '$lib/api';

  let { smiles, residues, onResidueClick }:
    { smiles: string; residues: PocketResidue[]; onResidueClick?: (r: PocketResidue) => void } = $props();

  let svgContainer = $state<HTMLDivElement>(undefined as unknown as HTMLDivElement);
  let ligandSvg = $state('');
  let loading = $state(true);

  const IX_COLORS: Record<string, string> = {
    hbond: '#58a6ff',
    hydrophobic: '#8b949e',
    ionic: '#d29922',
    dipole: '#bb33bb',
    contact: '#484f58',
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
      const svg = await getSVG(smiles, 160, 120);
      if (svg) ligandSvg = svg;
    }
    loading = false;
  });

  // Layout: ligand in center, residues in a circle around it
  const CX = 175;
  const CY = 110;
  const RADIUS = 90;

  let residuePositions = $derived(
    residues.slice(0, 12).map((r, i) => {
      const angle = (i / Math.min(residues.length, 12)) * 2 * Math.PI - Math.PI / 2;
      return {
        ...r,
        x: CX + RADIUS * Math.cos(angle),
        y: CY + RADIUS * Math.sin(angle),
        color: IX_COLORS[primaryInteraction(r)] || '#484f58',
        ix: primaryInteraction(r),
      };
    })
  );
</script>

<div class="interaction-network">
  {#if loading}
    <div class="net-loading">Loading...</div>
  {:else}
    <svg viewBox="0 0 350 220" class="net-svg">
      <!-- Interaction lines -->
      {#each residuePositions as rp}
        <line
          x1={CX} y1={CY} x2={rp.x} y2={rp.y}
          stroke={rp.color}
          stroke-width="1.5"
          stroke-dasharray="4 3"
          opacity="0.6"
        />
      {/each}

      <!-- Ligand 2D depiction -->
      <foreignObject x={CX - 80} y={CY - 60} width="160" height="120">
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
          <circle r="20" fill="rgba(13,17,23,0.9)" stroke={rp.color} stroke-width="1.5" />
          <text text-anchor="middle" dy="-4" fill={rp.color} font-size="8" font-weight="700">
            {rp.res_name}
          </text>
          <text text-anchor="middle" dy="7" fill="#8b949e" font-size="7" font-family="SF Mono, monospace">
            {rp.chain_id}{rp.res_id}
          </text>
          <text text-anchor="middle" dy="16" fill={rp.color} font-size="6" opacity="0.7">
            {rp.min_distance.toFixed(1)}A
          </text>
        </g>
      {/each}

      <!-- Legend -->
      {#each Object.entries(IX_COLORS).filter(([k]) => residuePositions.some(r => r.ix === k)) as [type, color], i}
        <g transform="translate(5,{8 + i * 12})">
          <line x1="0" y1="0" x2="12" y2="0" stroke={color} stroke-width="2" stroke-dasharray="3 2" />
          <text x="16" y="3" fill={color} font-size="7">{IX_LABELS[type]}</text>
        </g>
      {/each}
    </svg>
  {/if}
</div>

<style>
  .interaction-network {
    border: 1px solid rgba(48,54,61,0.4);
    border-radius: 6px;
    background: rgba(0,0,0,0.2);
    overflow: hidden;
  }

  .net-loading {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100px;
    color: var(--text-muted, #484f58);
    font-size: 11px;
  }

  .net-svg {
    width: 100%;
    height: auto;
    display: block;
  }

  .ligand-svg-wrap {
    width: 160px;
    height: 120px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .ligand-svg-wrap :global(svg) {
    width: 100%;
    height: 100%;
  }

  .ligand-placeholder {
    color: var(--text-muted, #484f58);
    font-size: 10px;
  }

  .res-node {
    cursor: pointer;
  }

  .res-node:hover circle {
    fill: rgba(88,166,255,0.1);
  }
</style>
