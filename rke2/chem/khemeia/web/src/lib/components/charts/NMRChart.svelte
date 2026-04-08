<script lang="ts">
  import PlotlyChart from './PlotlyChart.svelte';

  let { shifts, labels }: {
    shifts: number[];
    labels: string[];
  } = $props();

  let chartData = $derived.by(() => {
    // Build display labels: use provided labels or fall back to atom index
    const displayLabels = shifts.map((_, i) =>
      labels[i] || `Atom ${i + 1}`
    );

    return [{
      x: shifts,
      y: displayLabels,
      type: 'bar' as const,
      orientation: 'h' as const,
      marker: {
        color: '#58a6ff',
        opacity: 0.85,
      },
      hovertemplate: '%{y}: %{x:.2f} ppm<extra></extra>',
    }];
  });

  let chartLayout = $derived.by(() => {
    const barCount = shifts.length;
    // Scale height: minimum 250px, ~28px per bar, capped at 600px
    const dynamicHeight = Math.min(600, Math.max(250, barCount * 28 + 80));

    return {
      title: { text: 'NMR Chemical Shifts', font: { size: 13, color: '#e6edf3' } },
      xaxis: {
        title: { text: 'Chemical Shift (ppm)', font: { size: 11 } },
      },
      yaxis: {
        automargin: true,
        tickfont: { size: 10 },
      },
      height: dynamicHeight,
      bargap: 0.15,
    };
  });
</script>

<PlotlyChart data={chartData} layout={chartLayout} />
