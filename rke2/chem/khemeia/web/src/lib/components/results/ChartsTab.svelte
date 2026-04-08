<script lang="ts">
  import EnergyConvergenceChart from '../charts/EnergyConvergenceChart.svelte';
  import SpectrumChart from '../charts/SpectrumChart.svelte';

  let { job }: { job: any } = $props();

  let hasScfEnergies = $derived(
    Array.isArray(job?.output_data?.scf_energies) && job.output_data.scf_energies.length > 0
  );

  let hasIrSpectrum = $derived(
    Array.isArray(job?.output_data?.ir_frequencies) &&
    Array.isArray(job?.output_data?.ir_intensities) &&
    job.output_data.ir_frequencies.length > 0
  );

  let hasRamanSpectrum = $derived(
    Array.isArray(job?.output_data?.raman_frequencies) &&
    Array.isArray(job?.output_data?.raman_intensities) &&
    job.output_data.raman_frequencies.length > 0
  );

  let hasAnyChart = $derived(hasScfEnergies || hasIrSpectrum || hasRamanSpectrum);
</script>

<div class="charts-tab">
  {#if !hasAnyChart}
    <p class="no-data">No chart data available for this job.</p>
  {:else}
    {#if hasScfEnergies}
      <div class="chart-section">
        <EnergyConvergenceChart energies={job.output_data.scf_energies} />
      </div>
    {/if}

    {#if hasIrSpectrum}
      <div class="chart-section">
        <SpectrumChart
          frequencies={job.output_data.ir_frequencies}
          intensities={job.output_data.ir_intensities}
          type="ir"
        />
      </div>
    {/if}

    {#if hasRamanSpectrum}
      <div class="chart-section">
        <SpectrumChart
          frequencies={job.output_data.raman_frequencies}
          intensities={job.output_data.raman_intensities}
          type="raman"
        />
      </div>
    {/if}
  {/if}
</div>

<style>
  .charts-tab {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .chart-section {
    border: 1px solid rgba(48,54,61,0.4);
    border-radius: 6px;
    overflow: hidden;
    background: rgba(0,0,0,0.1);
  }

  .no-data {
    font-size: 12px;
    color: var(--text-muted, #484f58);
    text-align: center;
    padding: 24px 0;
  }
</style>
