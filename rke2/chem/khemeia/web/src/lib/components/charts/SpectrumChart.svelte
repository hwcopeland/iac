<script lang="ts">
  import PlotlyChart from './PlotlyChart.svelte';

  let { frequencies, intensities, type }: {
    frequencies: number[];
    intensities: number[];
    type: 'ir' | 'raman' | 'nmr';
  } = $props();

  /** Lorentzian line shape: L(x) = (gamma^2) / ((x - x0)^2 + gamma^2) */
  function lorentzian(x: number, x0: number, gamma: number): number {
    return (gamma * gamma) / ((x - x0) * (x - x0) + gamma * gamma);
  }

  /** Apply Lorentzian broadening to stick spectrum, producing a smooth curve. */
  function broaden(freqs: number[], ints: number[], fwhm: number, nPoints: number = 2000): { x: number[]; y: number[] } {
    if (freqs.length === 0) return { x: [], y: [] };

    const gamma = fwhm / 2;
    const minFreq = Math.min(...freqs);
    const maxFreq = Math.max(...freqs);
    const padding = fwhm * 5;
    const xMin = Math.max(0, minFreq - padding);
    const xMax = maxFreq + padding;
    const step = (xMax - xMin) / (nPoints - 1);

    const xOut: number[] = [];
    const yOut: number[] = [];

    for (let i = 0; i < nPoints; i++) {
      const x = xMin + i * step;
      let y = 0;
      for (let j = 0; j < freqs.length; j++) {
        y += ints[j] * lorentzian(x, freqs[j], gamma);
      }
      xOut.push(x);
      yOut.push(y);
    }

    return { x: xOut, y: yOut };
  }

  const axisLabels: Record<string, { x: string; y: string }> = {
    ir: { x: 'Wavenumber (cm\u207B\u00B9)', y: 'Transmittance' },
    raman: { x: 'Raman Shift (cm\u207B\u00B9)', y: 'Intensity' },
    nmr: { x: 'Chemical Shift (ppm)', y: 'Intensity' },
  };

  let chartData = $derived.by(() => {
    const fwhm = type === 'nmr' ? 1 : 10;
    const spectrum = broaden(frequencies, intensities, fwhm);

    // For IR, convert to transmittance (invert the absorption spectrum)
    let yValues = spectrum.y;
    if (type === 'ir' && yValues.length > 0) {
      const maxY = Math.max(...yValues);
      if (maxY > 0) {
        yValues = yValues.map(v => 1 - v / maxY);
      }
    }

    return [{
      x: spectrum.x,
      y: yValues,
      type: 'scatter' as const,
      mode: 'lines' as const,
      line: { color: '#58a6ff', width: 1.5 },
      fill: type === 'raman' ? 'tozeroy' as const : undefined,
      fillcolor: type === 'raman' ? 'rgba(88,166,255,0.1)' : undefined,
    }];
  });

  let chartLayout = $derived.by(() => {
    const labels = axisLabels[type] || axisLabels.ir;
    return {
      xaxis: {
        title: { text: labels.x, font: { size: 11 } },
        autorange: type === 'ir' ? 'reversed' as const : true as const,
      },
      yaxis: {
        title: { text: labels.y, font: { size: 11 } },
      },
    };
  });
</script>

<PlotlyChart data={chartData} layout={chartLayout} />
