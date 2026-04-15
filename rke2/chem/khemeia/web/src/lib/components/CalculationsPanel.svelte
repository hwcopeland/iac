<script lang="ts">
  import Panel from './Panel.svelte';
  import ResultsPanel from './results/ResultsPanel.svelte';
  import LigandSearch from './LigandSearch.svelte';
  import { getPlugins, submitJob, getJobs, getJob, getLigandDatabases } from '$lib/api';
  import type { Plugin, PluginInputField } from '$lib/api';
  import { getCurrentStructureText } from '$lib/viewer';
  import { isAuthenticated } from '$lib/auth';

  let plugins = $state<Plugin[]>([]);
  let pluginsLoading = $state(true);
  let pluginsError = $state('');
  let ligandDBs = $state<string[]>([]);
  let activePlugin = $state<string | null>(null);

  let formData = $state<Record<string, Record<string, any>>>({});
  let submitting = $state<Record<string, boolean>>({});
  let submitErrors = $state<Record<string, string>>({});
  let jobs = $state<Record<string, any[]>>({});
  let jobsLoading = $state<Record<string, boolean>>({});
  let selectedJob = $state<Record<string, any> | null>(null);
  let pollingJobs = $state<Set<string>>(new Set());
  let showLigandSearch = $state(false);

  $effect(() => {
    if (isAuthenticated()) {
      loadPlugins();
      loadLigandDBs();
    } else {
      pluginsLoading = false;
    }
  });

  async function loadLigandDBs() {
    try {
      const res = await getLigandDatabases();
      ligandDBs = res.databases.map(d => d.name);
    } catch { ligandDBs = []; }
  }

  async function handleChemblImported(dbName: string) {
    await loadLigandDBs();
    // Auto-select the newly imported database in any active plugin form
    if (activePlugin) {
      setFieldValue(activePlugin, 'ligand_db', dbName);
    }
    showLigandSearch = false;
  }

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

  // Auto-fill structure data if available
  function autoFillStructure(slug: string) {
    const pdb = getCurrentStructureText();
    if (!pdb) return;
    const data = formData[slug];
    if (!data) return;
    // Look for fields that might accept structure data
    for (const key of Object.keys(data)) {
      const lower = key.toLowerCase();
      if ((lower.includes('input_file') || lower.includes('structure') || lower.includes('pdb'))
          && !data[key]) {
        data[key] = pdb;
      }
    }
  }

  async function handleSubmit(plugin: Plugin) {
    autoFillStructure(plugin.slug);
    submitting[plugin.slug] = true;
    submitErrors[plugin.slug] = '';
    try {
      const result = await submitJob(plugin.slug, formData[plugin.slug] || {});
      // Start polling this job
      if (result.name) {
        pollJob(plugin.slug, result.name);
      }
      await loadJobs(plugin);
    } catch (e: any) {
      submitErrors[plugin.slug] = e.message || 'Submission failed';
    } finally {
      submitting[plugin.slug] = false;
    }
  }

  async function pollJob(slug: string, jobName: string) {
    const key = `${slug}/${jobName}`;
    pollingJobs.add(key);
    pollingJobs = new Set(pollingJobs);

    const poll = async () => {
      try {
        const detail = await getJob(slug, jobName);
        // Update in job list
        if (jobs[slug]) {
          const idx = jobs[slug].findIndex((j: any) => j.name === jobName);
          if (idx >= 0) jobs[slug][idx] = { ...jobs[slug][idx], ...detail };
        }
        // Update selected job if viewing this one
        if (selectedJob?.name === jobName) {
          selectedJob = detail;
        }
        // Stop polling if terminal
        if (detail.status === 'Completed' || detail.status === 'Failed') {
          pollingJobs.delete(key);
          pollingJobs = new Set(pollingJobs);
          await loadJobs(plugins.find(p => p.slug === slug)!);
          return;
        }
        // Continue polling
        setTimeout(poll, 3000);
      } catch {
        pollingJobs.delete(key);
        pollingJobs = new Set(pollingJobs);
      }
    };
    poll();
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

  async function viewJob(slug: string, job: any) {
    try {
      selectedJob = await getJob(slug, job.name);
    } catch {
      selectedJob = job;
    }
  }

  function formatDate(d: string): string {
    if (!d) return '';
    const date = new Date(d);
    return date.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  }

  function statusClass(status: string): string {
    const s = status?.toLowerCase();
    if (s === 'completed') return 'completed';
    if (s === 'failed') return 'failed';
    if (s === 'running') return 'running';
    return 'pending';
  }
</script>

<div class="calc-panels">
  {#if pluginsLoading}
    <div class="loading">Loading plugins...</div>
  {:else if pluginsError}
    <div class="error-box">
      <p class="error-title">Failed to load plugins</p>
      <p class="error-detail">{pluginsError}</p>
      <button class="retry-btn" onclick={loadPlugins}>Retry</button>
    </div>
  {:else if plugins.length === 0}
    <div class="empty">No computation plugins available.</div>
  {:else}
    {#if plugins.length > 1}
      <div class="plugin-tabs">
        {#each plugins as plugin}
          <button
            class="plugin-tab"
            class:active={activePlugin === plugin.slug}
            onclick={() => { activePlugin = plugin.slug; loadJobs(plugin); }}
          >
            {plugin.name}
          </button>
        {/each}
      </div>
    {/if}

    {#each plugins as plugin}
      {#if activePlugin === plugin.slug}
        <Panel title="Input">
          <form
            class="plugin-form"
            onsubmit={(e) => { e.preventDefault(); handleSubmit(plugin); }}
          >
            {#each plugin.input as field}
              <div class="form-field">
                <label class="form-label" for="{plugin.slug}-{field.name}">
                  {field.name.replace(/_/g, ' ')}
                  {#if field.required}<span class="required">*</span>{/if}
                </label>
                {#if field.description}
                  <p class="form-desc">{field.description}</p>
                {/if}

                {#if field.name === 'ligand_db'}
                  <div class="ligand-db-row">
                    <select
                      id="{plugin.slug}-{field.name}"
                      class="form-select"
                      value={getFieldValue(plugin.slug, field.name)}
                      onchange={(e) => setFieldValue(plugin.slug, field.name, (e.target as HTMLSelectElement).value)}
                      required={field.required}
                    >
                      <option value="">{ligandDBs.length ? 'Select ligand database...' : 'No databases — import from ChEMBL'}</option>
                      {#each ligandDBs as db}
                        <option value={db}>{db}</option>
                      {/each}
                    </select>
                    <button
                      type="button"
                      class="chembl-btn"
                      onclick={() => (showLigandSearch = !showLigandSearch)}
                    >
                      {showLigandSearch ? 'Close' : 'Browse ChEMBL'}
                    </button>
                  </div>
                {:else if field.type === 'text'}
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
              class="submit-btn"
              disabled={submitting[plugin.slug]}
            >
              {submitting[plugin.slug] ? 'Submitting...' : 'Submit Job'}
            </button>

            {#if submitErrors[plugin.slug]}
              <p class="error-msg">{submitErrors[plugin.slug]}</p>
            {/if}
          </form>
        </Panel>

        {#if showLigandSearch}
          <LigandSearch onImported={handleChemblImported} />
        {/if}

        <Panel title="Jobs" collapsed={false}>
          <div class="jobs-header">
            <button class="refresh-btn" onclick={() => loadJobs(plugin)} disabled={jobsLoading[plugin.slug]}>
              {jobsLoading[plugin.slug] ? '...' : 'Refresh'}
            </button>
          </div>
          {#if jobsLoading[plugin.slug] && !jobs[plugin.slug]?.length}
            <p class="loading-small">Loading...</p>
          {:else if (jobs[plugin.slug]?.length ?? 0) === 0}
            <p class="empty-small">No jobs yet.</p>
          {:else}
            <div class="job-list">
              {#each jobs[plugin.slug] as job}
                <button
                  class="job-item"
                  class:selected={selectedJob?.name === job.name}
                  onclick={() => viewJob(plugin.slug, job)}
                >
                  <div class="job-info">
                    <span class="job-name">{job.name}</span>
                    <span class="job-date">{formatDate(job.created_at)}</span>
                  </div>
                  <span class="job-status {statusClass(job.status)}">
                    {#if pollingJobs.has(`${plugin.slug}/${job.name}`)}
                      {job.status}...
                    {:else}
                      {job.status}
                    {/if}
                  </span>
                </button>
              {/each}
            </div>
          {/if}
        </Panel>

        {#if selectedJob}
          <ResultsPanel job={selectedJob} pluginSlug={activePlugin} />
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

  .loading, .empty {
    color: var(--text-muted, #484f58);
    font-size: 13px;
    padding: 16px;
    text-align: center;
  }

  .loading-small, .empty-small {
    color: var(--text-muted, #484f58);
    font-size: 12px;
  }

  .error-box {
    padding: 16px;
    text-align: center;
  }

  .error-title {
    color: var(--danger, #f85149);
    font-weight: 600;
    font-size: 13px;
    margin-bottom: 4px;
  }

  .error-detail {
    color: var(--text-secondary, #8b949e);
    font-size: 12px;
    margin-bottom: 8px;
  }

  .error-msg {
    color: var(--danger, #f85149);
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
    color: var(--text-secondary, #8b949e);
    font-size: 12px;
    font-weight: 500;
    padding: 4px 10px;
    border-radius: 4px;
    cursor: pointer;
    transition: all 0.15s;
    white-space: nowrap;
  }

  .plugin-tab:hover { color: var(--text-primary, #e6edf3); background: rgba(88,166,255,0.1); }
  .plugin-tab.active { color: var(--accent, #58a6ff); background: rgba(88,166,255,0.1); }

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
    color: var(--text-secondary, #8b949e);
    text-transform: uppercase;
    letter-spacing: 0.3px;
  }

  .required { color: var(--danger, #f85149); }

  .form-desc {
    font-size: 11px;
    color: var(--text-muted, #484f58);
  }

  .form-input, .form-select, .form-textarea {
    background: rgba(0,0,0,0.3);
    border: 1px solid rgba(48,54,61,0.6);
    color: var(--text-primary, #e6edf3);
    font-family: 'SF Mono', monospace;
    font-size: 12px;
    padding: 6px 8px;
    border-radius: 4px;
    outline: none;
    transition: border-color 0.15s;
    width: 100%;
  }

  .form-input:focus, .form-select:focus, .form-textarea:focus {
    border-color: var(--accent, #58a6ff);
  }

  .form-textarea {
    resize: vertical;
    min-height: 60px;
  }

  .field-hint {
    font-size: 10px;
    color: var(--text-muted, #484f58);
  }

  .submit-btn {
    background: var(--accent, #58a6ff);
    border: none;
    color: #000;
    font-size: 13px;
    font-weight: 600;
    padding: 8px 12px;
    border-radius: 6px;
    cursor: pointer;
    width: 100%;
  }

  .submit-btn:hover:not(:disabled) { opacity: 0.9; }
  .submit-btn:disabled { opacity: 0.4; cursor: not-allowed; }

  .retry-btn {
    background: rgba(88,166,255,0.1);
    border: 1px solid rgba(88,166,255,0.3);
    color: var(--accent, #58a6ff);
    font-size: 11px;
    padding: 4px 10px;
    border-radius: 4px;
    cursor: pointer;
  }

  .jobs-header {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 6px;
  }

  .refresh-btn {
    background: none;
    border: 1px solid rgba(48,54,61,0.6);
    color: var(--text-secondary, #8b949e);
    font-size: 10px;
    padding: 2px 8px;
    border-radius: 4px;
    cursor: pointer;
  }

  .refresh-btn:hover { color: var(--text-primary, #e6edf3); }

  .job-list {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .job-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    width: 100%;
    padding: 6px 8px;
    background: rgba(0,0,0,0.15);
    border: 1px solid transparent;
    border-radius: 4px;
    cursor: pointer;
    transition: all 0.15s;
    text-align: left;
  }

  .job-item:hover { background: rgba(255,255,255,0.05); border-color: rgba(48,54,61,0.6); }
  .job-item.selected { border-color: var(--accent, #58a6ff); background: rgba(88,166,255,0.05); }

  .job-info {
    display: flex;
    flex-direction: column;
    gap: 1px;
    overflow: hidden;
  }

  .job-name {
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    color: var(--text-primary, #e6edf3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .job-date {
    font-size: 10px;
    color: var(--text-muted, #484f58);
  }

  .job-status {
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    padding: 1px 6px;
    border-radius: 3px;
    white-space: nowrap;
    flex-shrink: 0;
  }

  .job-status.pending { background: rgba(88,166,255,0.1); color: var(--accent, #58a6ff); }
  .job-status.running { background: rgba(210,153,34,0.15); color: #d29922; }
  .job-status.completed { background: rgba(63,185,80,0.15); color: #3fb950; }
  .job-status.failed { background: rgba(248,81,73,0.15); color: #f85149; }

  .ligand-db-row {
    display: flex;
    gap: 6px;
    align-items: center;
  }

  .ligand-db-row .form-select {
    flex: 1;
  }

  .chembl-btn {
    background: rgba(88,166,255,0.1);
    border: 1px solid rgba(88,166,255,0.3);
    color: var(--accent, #58a6ff);
    font-size: 11px;
    font-weight: 500;
    padding: 5px 10px;
    border-radius: 4px;
    cursor: pointer;
    white-space: nowrap;
    transition: all 0.15s;
  }

  .chembl-btn:hover {
    background: rgba(88,166,255,0.2);
  }

</style>
