<script lang="ts">
  import type { AtomInfo } from '$lib/viewer';

  let { info }: { info: AtomInfo | null } = $props();

  const aminoAcids: Record<string, { full: string; props: string }> = {
    ALA: { full: 'Alanine', props: 'nonpolar, hydrophobic' },
    ARG: { full: 'Arginine', props: 'polar, positive charge' },
    ASN: { full: 'Asparagine', props: 'polar, uncharged' },
    ASP: { full: 'Aspartate', props: 'polar, negative charge' },
    CYS: { full: 'Cysteine', props: 'polar, thiol group' },
    GLN: { full: 'Glutamine', props: 'polar, uncharged' },
    GLU: { full: 'Glutamate', props: 'polar, negative charge' },
    GLY: { full: 'Glycine', props: 'nonpolar, smallest' },
    HIS: { full: 'Histidine', props: 'polar, aromatic, imidazole' },
    ILE: { full: 'Isoleucine', props: 'nonpolar, hydrophobic' },
    LEU: { full: 'Leucine', props: 'nonpolar, hydrophobic' },
    LYS: { full: 'Lysine', props: 'polar, positive charge' },
    MET: { full: 'Methionine', props: 'nonpolar, thioether' },
    PHE: { full: 'Phenylalanine', props: 'nonpolar, aromatic' },
    PRO: { full: 'Proline', props: 'nonpolar, cyclic' },
    SER: { full: 'Serine', props: 'polar, hydroxyl' },
    THR: { full: 'Threonine', props: 'polar, hydroxyl' },
    TRP: { full: 'Tryptophan', props: 'nonpolar, aromatic, indole' },
    TYR: { full: 'Tyrosine', props: 'polar, aromatic, hydroxyl' },
    VAL: { full: 'Valine', props: 'nonpolar, hydrophobic' },
  };

  let aaInfo = $derived(info ? aminoAcids[info.residueName.toUpperCase()] ?? null : null);
</script>

{#if info}
  <div class="selection-info">
    <div class="sel-row sel-main">
      <span class="sel-element">{info.element}</span>
      <span class="sel-atom">{info.atomName}</span>
      <span class="sel-sep">/</span>
      <span class="sel-residue">{info.residueName} {info.residueId}</span>
      <span class="sel-sep">/</span>
      <span class="sel-chain">Chain {info.chainId}</span>
    </div>
    <div class="sel-row sel-coords">
      ({info.x.toFixed(2)}, {info.y.toFixed(2)}, {info.z.toFixed(2)})
    </div>
    {#if aaInfo}
      <div class="sel-row sel-aa">
        {aaInfo.full} &mdash; {aaInfo.props}
      </div>
    {/if}
  </div>
{/if}

<style>
  .selection-info {
    background: var(--bg-surface);
    backdrop-filter: var(--panel-blur);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    padding: 8px 12px;
    box-shadow: var(--shadow-md);
    max-width: 360px;
  }

  .sel-row {
    font-size: 12px;
  }

  .sel-main {
    display: flex;
    align-items: center;
    gap: 4px;
    color: var(--text-primary);
    font-weight: 500;
  }

  .sel-element {
    font-weight: 700;
    color: var(--accent);
    font-family: var(--font-mono);
    font-size: 14px;
  }

  .sel-atom {
    font-family: var(--font-mono);
  }

  .sel-sep {
    color: var(--text-muted);
  }

  .sel-residue {
    color: var(--success);
    font-family: var(--font-mono);
  }

  .sel-chain {
    color: var(--text-secondary);
  }

  .sel-coords {
    color: var(--text-muted);
    font-family: var(--font-mono);
    font-size: 11px;
    margin-top: 2px;
  }

  .sel-aa {
    color: var(--text-secondary);
    font-size: 11px;
    margin-top: 2px;
    font-style: italic;
  }
</style>
