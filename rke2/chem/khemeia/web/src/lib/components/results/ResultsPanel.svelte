<script lang="ts">
  import Panel from '../Panel.svelte';
  import SummaryTab from './SummaryTab.svelte';
  import ChartsTab from './ChartsTab.svelte';
  import ThreeDTab from './ThreeDTab.svelte';
  import RawTab from './RawTab.svelte';

  let { job, pluginSlug }: { job: any; pluginSlug: string } = $props();

  function switchTo3D() {
    if (hasArtifacts) activeTab = '3d';
  }

  type TabId = 'summary' | 'charts' | '3d' | 'raw';
  let activeTab = $state<TabId>('summary');

  let hasChartData = $derived(
    Array.isArray(job?.output_data?.scf_energies) && job.output_data.scf_energies.length > 0 ||
    (Array.isArray(job?.output_data?.ir_frequencies) && Array.isArray(job?.output_data?.ir_intensities) && job.output_data.ir_frequencies.length > 0) ||
    (Array.isArray(job?.output_data?.raman_frequencies) && Array.isArray(job?.output_data?.raman_intensities) && job.output_data.raman_frequencies.length > 0)
  );

  let hasArtifacts = $derived(
    Array.isArray(job?.artifacts) && job.artifacts.length > 0
  );

  let tabs = $derived.by(() => {
    const t: { id: TabId; label: string }[] = [
      { id: 'summary', label: 'Summary' },
    ];
    if (hasChartData) {
      t.push({ id: 'charts', label: 'Charts' });
    }
    if (hasArtifacts) {
      t.push({ id: '3d', label: '3D' });
    }
    t.push({ id: 'raw', label: 'Raw' });
    return t;
  });

  // Reset to summary if the active tab is no longer visible
  $effect(() => {
    if (!tabs.find(t => t.id === activeTab)) {
      activeTab = 'summary';
    }
  });
</script>

<Panel title="Result">
  <div class="results-panel">
    <div class="result-tabs">
      {#each tabs as tab}
        <button
          class="result-tab"
          class:active={activeTab === tab.id}
          onclick={() => { activeTab = tab.id; }}
        >
          {tab.label}
        </button>
      {/each}
    </div>

    <div class="tab-content">
      {#if activeTab === 'summary'}
        <SummaryTab {job} {pluginSlug} onView3D={hasArtifacts ? switchTo3D : undefined} />
      {:else if activeTab === 'charts'}
        <ChartsTab {job} />
      {:else if activeTab === '3d'}
        <ThreeDTab {job} {pluginSlug} />
      {:else if activeTab === 'raw'}
        <RawTab {job} />
      {/if}
    </div>
  </div>
</Panel>

<style>
  .results-panel {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .result-tabs {
    display: flex;
    gap: 2px;
    overflow-x: auto;
  }

  .result-tab {
    background: none;
    border: none;
    color: var(--text-secondary, #8b949e);
    font-size: 12px;
    font-weight: 500;
    padding: 4px 10px;
    border-radius: 4px;
    cursor: pointer;
    transition: all 0.15s;
    white-space: nowrap;
  }

  .result-tab:hover {
    color: var(--text-primary, #e6edf3);
    background: rgba(88,166,255,0.1);
  }

  .result-tab.active {
    color: var(--accent, #58a6ff);
    background: rgba(88,166,255,0.1);
  }

  .tab-content {
    min-height: 0;
  }
</style>
