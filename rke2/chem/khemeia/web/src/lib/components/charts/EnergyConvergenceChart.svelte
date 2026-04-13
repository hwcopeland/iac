<script lang="ts">
  import PlotlyChart from './PlotlyChart.svelte';

  let { energies, unit = 'Ha' }: { energies: string[]; unit?: string } = $props();

  let numericValues = $derived(energies.map(Number).filter(v => !isNaN(v)));

  let chartData = $derived.by(() => {
    const iterations = numericValues.map((_, i) => i + 1);
    return [{
      x: iterations,
      y: numericValues,
      type: 'scatter' as const,
      mode: 'lines+markers' as const,
      line: { color: '#58a6ff', width: 2 },
      marker: { color: '#58a6ff', size: 4 },
    }];
  });

  /**
   * Detect whether the SCF energies have converged by checking if the last
   * few iterations are within a tight relative tolerance of each other.
   * Returns the converged value or null if not converged / too few points.
   */
  let convergedEnergy = $derived.by(() => {
    const vals = numericValues;
    if (vals.length < 3) return null;
    // Check last 3 values for convergence (relative change < 1e-6)
    const tail = vals.slice(-3);
    const last = tail[tail.length - 1];
    if (last === 0) return null;
    const allClose = tail.every(v => Math.abs((v - last) / last) < 1e-6);
    return allClose ? last : null;
  });

  let chartLayout = $derived.by(() => {
    const base: any = {
      title: { text: 'SCF Energy Convergence', font: { size: 13, color: '#e6edf3' } },
      xaxis: { title: { text: 'Iteration', font: { size: 11 } } },
      yaxis: { title: { text: `Energy (${unit})`, font: { size: 11 } } },
    };
    if (convergedEnergy != null) {
      base.shapes = [{
        type: 'line',
        x0: 0,
        x1: 1,
        xref: 'paper',
        y0: convergedEnergy,
        y1: convergedEnergy,
        yref: 'y',
        line: { color: '#3fb950', width: 1, dash: 'dash' },
      }];
      base.annotations = [{
        x: 1,
        xref: 'paper',
        xanchor: 'right',
        y: convergedEnergy,
        yref: 'y',
        text: `${convergedEnergy.toFixed(6)} ${unit}`,
        showarrow: false,
        font: { size: 9, color: '#3fb950', family: 'SF Mono, monospace' },
        bgcolor: 'rgba(0,0,0,0.5)',
        borderpad: 2,
      }];
    }
    return base;
  });
</script>

<PlotlyChart data={chartData} layout={chartLayout} />
