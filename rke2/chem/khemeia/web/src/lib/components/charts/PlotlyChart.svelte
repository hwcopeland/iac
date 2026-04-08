<script lang="ts">
  import { onMount } from 'svelte';

  let { data, layout = {}, config = {} }: { data: any[]; layout?: any; config?: any } = $props();
  let plotDiv = $state<HTMLDivElement>(undefined as unknown as HTMLDivElement);
  let Plotly: any = null;

  const darkLayout = {
    paper_bgcolor: 'rgba(0,0,0,0)',
    plot_bgcolor: 'rgba(0,0,0,0.15)',
    font: { color: '#8b949e', family: 'SF Mono, monospace', size: 11 },
    xaxis: { gridcolor: 'rgba(48,54,61,0.6)', zerolinecolor: 'rgba(48,54,61,0.6)' },
    yaxis: { gridcolor: 'rgba(48,54,61,0.6)', zerolinecolor: 'rgba(48,54,61,0.6)' },
    margin: { l: 60, r: 20, t: 30, b: 40 },
    autosize: true,
  };

  onMount(async () => {
    Plotly = (await import('plotly.js-dist-min')).default;
    render();
    return () => { if (plotDiv && Plotly) Plotly.purge(plotDiv); };
  });

  $effect(() => {
    if (Plotly && plotDiv && data) render();
  });

  function render() {
    Plotly.react(plotDiv, data, { ...darkLayout, ...layout }, { responsive: true, displayModeBar: false, ...config });
  }
</script>

<div bind:this={plotDiv} class="plotly-container"></div>

<style>
  .plotly-container { width: 100%; min-height: 250px; }
</style>
