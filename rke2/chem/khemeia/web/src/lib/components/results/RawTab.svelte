<script lang="ts">
  let { job }: { job: any } = $props();
</script>

<div class="raw-tab">
  {#if job.output_data}
    <div class="raw-section">
      <p class="section-label">Output Data</p>
      <pre class="raw-pre">{JSON.stringify(job.output_data, null, 2)}</pre>
    </div>
  {:else}
    <p class="no-data">No output data.</p>
  {/if}

  {#if job.error_output}
    <div class="error-section">
      <p class="error-label">Error Output</p>
      <pre class="error-pre">{job.error_output}</pre>
    </div>
  {/if}

  {#if job.input_data}
    <details class="input-details">
      <summary>Input Data</summary>
      <pre class="raw-pre">{JSON.stringify(job.input_data, null, 2)}</pre>
    </details>
  {/if}
</div>

<style>
  .raw-tab {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .raw-section {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .section-label {
    font-size: 10px;
    font-weight: 600;
    color: var(--text-muted, #484f58);
    text-transform: uppercase;
    letter-spacing: 0.3px;
  }

  .raw-pre {
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    color: var(--text-secondary, #8b949e);
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 300px;
    overflow-y: auto;
    margin: 0;
    background: rgba(0,0,0,0.2);
    padding: 10px;
    border-radius: 6px;
    border: 1px solid rgba(48,54,61,0.4);
  }

  .no-data {
    font-size: 12px;
    color: var(--text-muted, #484f58);
    text-align: center;
    padding: 12px 0;
  }

  .error-section {
    background: rgba(248,81,73,0.05);
    border: 1px solid rgba(248,81,73,0.2);
    border-radius: 6px;
    padding: 10px;
  }

  .error-label {
    font-size: 10px;
    font-weight: 600;
    color: var(--danger, #f85149);
    text-transform: uppercase;
    margin-bottom: 6px;
  }

  .error-pre {
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    color: var(--text-secondary, #8b949e);
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 200px;
    overflow-y: auto;
    margin: 0;
  }

  .input-details {
    margin-top: 2px;
  }

  .input-details summary {
    font-size: 11px;
    color: var(--text-secondary, #8b949e);
    cursor: pointer;
    user-select: none;
    font-weight: 500;
  }

  .input-details .raw-pre {
    margin-top: 6px;
  }
</style>
