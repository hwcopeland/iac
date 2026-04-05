<script lang="ts">
  import Panel from './Panel.svelte';
  import { getPlugins, submitJob, getJobs } from '$lib/api';
  import type { Plugin, PluginInputField } from '$lib/api';

  let plugins = $state<Plugin[]>([]);
  let pluginsLoading = $state(true);
  let pluginsError = $state('');
  let activePlugin = $state<string | null>(null);

  // Form data per plugin slug
  let formData = $state<Record<string, Record<string, any>>>({});
  let submitting = $state<Record<string, boolean>>({});
  let submitErrors = $state<Record<string, string>>({});
  let jobs = $state<Record<string, any[]>>({});
  let jobsLoading = $state<Record<string, boolean>>({});
  let selectedJob = $state<Record<string, any> | null>(null);

  $effect(() => {
    loadPlugins();
  });

  async function loadPlugins() {
    pluginsLoading = true;
    pluginsError = '';
    try {
      const res = await getPlugins();
      plugins = res.plugins;
      if (plugins.length > 0) {
        activePlugin = plugins[0].slug;
        for (const p of plugins) {
          initFormData(p);
        }
      }
    } catch (e: any) {
      pluginsError = e.message || 'Failed to load plugins';
    } finally {
      pluginsLoading = false;
    }
  }

  function initFormData(plugin: Plugin) {
    const data: Record<string, any> = {};
    for (const field of plugin.input) {
      data[field.name] = field.default ?? '';
    }
    formData[plugin.slug] = data;
  }

  function getFieldValue(slug: string, name: string): any {
    return formData[slug]?.[name] ?? '';
  }

  function setFieldValue(slug: string, name: string, value: any) {
    if (!formData[slug]) formData[slug] = {};
    formData[slug][name] = value;
  }

  async function handleSubmit(plugin: Plugin) {
    submitting[plugin.slug] = true;
    submitErrors[plugin.slug] = '';
    try {
      await submitJob(plugin.slug, formData[plugin.slug] || {});
      await loadJobs(plugin);
    } catch (e: any) {
      submitErrors[plugin.slug] = e.message || 'Submission failed';
    } finally {
      submitting[plugin.slug] = false;
    }
  }

  async function loadJobs(plugin: Plugin) {
    jobsLoading[plugin.slug] = true;
    try {
      const res = await getJobs(plugin.slug);
      jobs[plugin.slug] = Array.isArray(res) ? res : res.jobs || [];
    } catch {
      jobs[plugin.slug] = [];
    } finally {
      jobsLoading[plugin.slug] = false;
    }
  }

  function inputType(field: PluginInputField): string {
    if (field.type === 'int' || field.type === 'float') return 'number';
    return 'text';
  }
</script>

<div class="calc-panels">
  {#if pluginsLoading}
    <div class="loading">Loading plugins...</div>
  {:else if pluginsError}
    <div class="error-box">
      <p class="error-title">Failed to load plugins</p>
      <p class="error-detail">{pluginsError}</p>
      <button class="btn btn-small" onclick={loadPlugins}>Retry</button>
    </div>
  {:else if plugins.length === 0}
    <div class="empty">No computation plugins available.</div>
  {:else}
    <div class="plugin-tabs">
      {#each plugins as plugin}
        <button
          class="plugin-tab"
          class:active={activePlugin === plugin.slug}
          onclick={() => (activePlugin = plugin.slug)}
        >
          {plugin.name}
        </button>
      {/each}
    </div>

    {#each plugins as plugin}
      {#if activePlugin === plugin.slug}
        <Panel title="{plugin.name} Input">
          <form
            class="plugin-form"
            onsubmit={(e) => { e.preventDefault(); handleSubmit(plugin); }}
          >
            {#each plugin.input as field}
              <div class="form-field">
                <label class="form-label" for="{plugin.slug}-{field.name}">
                  {field.name.replace(/_/g, ' ')}
                  {#if field.required}
                    <span class="required">*</span>
                  {/if}
                </label>
                {#if field.description}
                  <p class="form-desc">{field.description}</p>
                {/if}

                {#if field.type === 'text'}
                  <textarea
                    id="{plugin.slug}-{field.name}"
                    class="form-textarea"
                    rows="4"
                    value={getFieldValue(plugin.slug, field.name)}
                    oninput={(e) => setFieldValue(plugin.slug, field.name, (e.target as HTMLTextAreaElement).value)}
                    required={field.required}
                  ></textarea>
                {:else if field.enum}
                  <select
                    id="{plugin.slug}-{field.name}"
                    class="form-select"
                    value={getFieldValue(plugin.slug, field.name)}
                    onchange={(e) => setFieldValue(plugin.slug, field.name, (e.target as HTMLSelectElement).value)}
                  >
                    {#each field.enum as opt}
                      <option value={opt}>{opt}</option>
                    {/each}
                  </select>
                {:else if field.type === 'int' || field.type === 'float'}
                  <input
                    id="{plugin.slug}-{field.name}"
                    type="number"
                    class="form-input"
                    value={getFieldValue(plugin.slug, field.name)}
                    oninput={(e) => setFieldValue(plugin.slug, field.name, Number((e.target as HTMLInputElement).value))}
                    max={field.max}
                    required={field.required}
                  />
                  {#if field.max}
                    <span class="field-hint">max: {field.max}</span>
                  {/if}
                {:else}
                  <input
                    id="{plugin.slug}-{field.name}"
                    type="text"
                    class="form-input"
                    value={getFieldValue(plugin.slug, field.name)}
                    oninput={(e) => setFieldValue(plugin.slug, field.name, (e.target as HTMLInputElement).value)}
                    required={field.required}
                  />
                {/if}
              </div>
            {/each}

            <button
              type="submit"
              class="btn btn-accent btn-full"
              disabled={submitting[plugin.slug]}
            >
              {submitting[plugin.slug] ? 'Submitting...' : 'Submit Job'}
            </button>

            {#if submitErrors[plugin.slug]}
              <p class="error-msg">{submitErrors[plugin.slug]}</p>
            {/if}
          </form>
        </Panel>

        <Panel title="Recent Jobs" collapsed={true}>
          {#if jobsLoading[plugin.slug]}
            <p class="loading-small">Loading jobs...</p>
          {:else if (jobs[plugin.slug]?.length ?? 0) === 0}
            <p class="empty-small">No jobs yet.</p>
          {:else}
            <ul class="job-list">
              {#each jobs[plugin.slug] as job}
                <li>
                  <button
                    class="job-item"
                    onclick={() => (selectedJob = job)}
                  >
                    <span class="job-id">{job.id || 'job'}</span>
                    <span class="job-status" class:running={job.status === 'running'} class:completed={job.status === 'completed'} class:failed={job.status === 'failed'}>
                      {job.status || 'unknown'}
                    </span>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </Panel>

        {#if selectedJob}
          <Panel title="Job Output">
            <pre class="job-output">{JSON.stringify(selectedJob, null, 2)}</pre>
          </Panel>
        {/if}
      {/if}
    {/each}
  {/if}
</div>

<style>
  .calc-panels {
    display: flex;
    flex-direction: column;
  }

  .loading {
    color: var(--text-secondary);
    font-size: 13px;
    padding: 16px;
    text-align: center;
  }

  .loading-small {
    color: var(--text-muted);
    font-size: 12px;
  }

  .empty {
    color: var(--text-muted);
    font-size: 13px;
    padding: 16px;
    text-align: center;
  }

  .empty-small {
    color: var(--text-muted);
    font-size: 12px;
  }

  .error-box {
    padding: 16px;
    text-align: center;
  }

  .error-title {
    color: var(--danger);
    font-weight: 600;
    font-size: 13px;
    margin-bottom: 4px;
  }

  .error-detail {
    color: var(--text-secondary);
    font-size: 12px;
    margin-bottom: 8px;
  }

  .error-msg {
    color: var(--danger);
    font-size: 12px;
    margin-top: 8px;
  }

  .plugin-tabs {
    display: flex;
    gap: 2px;
    padding: 0 0 8px;
    overflow-x: auto;
  }

  .plugin-tab {
    background: none;
    border: none;
    color: var(--text-secondary);
    font-size: 12px;
    font-weight: 500;
    padding: 4px 10px;
    border-radius: var(--radius-sm);
    cursor: pointer;
    transition: all var(--transition-fast);
    white-space: nowrap;
    font-family: var(--font-sans);
  }

  .plugin-tab:hover {
    color: var(--text-primary);
    background: var(--accent-subtle);
  }

  .plugin-tab.active {
    color: var(--accent);
    background: var(--accent-subtle);
  }

  .plugin-form {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .form-field {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .form-label {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.3px;
  }

  .required {
    color: var(--danger);
  }

  .form-desc {
    font-size: 11px;
    color: var(--text-muted);
  }

  .form-input,
  .form-select,
  .form-textarea {
    background: var(--bg-input);
    border: 1px solid var(--border-default);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: 12px;
    padding: 6px 8px;
    border-radius: var(--radius-sm);
    transition: border-color var(--transition-fast);
    width: 100%;
  }

  .form-input:focus,
  .form-select:focus,
  .form-textarea:focus {
    border-color: var(--border-focus);
    outline: none;
  }

  .form-select {
    cursor: pointer;
  }

  .form-textarea {
    resize: vertical;
    min-height: 60px;
  }

  .field-hint {
    font-size: 10px;
    color: var(--text-muted);
  }

  .btn {
    background: var(--accent-subtle);
    border: 1px solid transparent;
    color: var(--accent);
    font-size: 12px;
    font-weight: 500;
    padding: 6px 12px;
    border-radius: var(--radius-sm);
    cursor: pointer;
    transition: all var(--transition-fast);
    font-family: var(--font-sans);
  }

  .btn:hover:not(:disabled) {
    background: rgba(88, 166, 255, 0.25);
  }

  .btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .btn-accent {
    background: var(--accent);
    color: var(--bg-base);
    font-weight: 600;
  }

  .btn-accent:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .btn-full {
    width: 100%;
    padding: 8px 12px;
  }

  .btn-small {
    font-size: 11px;
    padding: 4px 10px;
  }

  .job-list {
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .job-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    width: 100%;
    padding: 6px 8px;
    background: var(--bg-input);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    cursor: pointer;
    transition: border-color var(--transition-fast);
    font-family: var(--font-sans);
    color: var(--text-primary);
    font-size: 12px;
  }

  .job-item:hover {
    border-color: var(--accent);
  }

  .job-id {
    font-family: var(--font-mono);
    font-size: 11px;
  }

  .job-status {
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    padding: 1px 6px;
    border-radius: 3px;
    background: var(--accent-subtle);
    color: var(--accent);
  }

  .job-status.running {
    background: rgba(210, 153, 34, 0.15);
    color: var(--warning);
  }

  .job-status.completed {
    background: rgba(63, 185, 80, 0.15);
    color: var(--success);
  }

  .job-status.failed {
    background: rgba(248, 81, 73, 0.15);
    color: var(--danger);
  }

  .job-output {
    background: var(--bg-input);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    padding: 8px;
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-secondary);
    overflow-x: auto;
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 300px;
    overflow-y: auto;
  }
</style>
