<script lang="ts">
  import PlotlyChart from './PlotlyChart.svelte';

  let { energies, unit = 'Ha' }: { energies: string[]; unit?: string } = $props();

  let chartData = $derived.by(() => {
    const values = energies.map(Number).filter(v => !isNaN(v));
    const iterations = values.map((_, i) => i + 1);
    return [{
      x: iterations,
      y: values,
      type: 'scatter' as const,
      mode: 'lines+markers' as const,
      line: { color: '#58a6ff', width: 2 },
      marker: { color: '#58a6ff', size: 4 },
    }];
  });

  let chartLayout = $derived({
    title: { text: 'SCF Energy Convergence', font: { size: 13, color: '#e6edf3' } },
    xaxis: { title: { text: 'Iteration', font: { size: 11 } } },
    yaxis: { title: { text: `Energy (${unit})`, font: { size: 11 } } },
  });
</script>

<PlotlyChart data={chartData} layout={chartLayout} />
