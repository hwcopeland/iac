<script lang="ts">
  import Panel from './Panel.svelte';
  import ResultsPanel from './results/ResultsPanel.svelte';
  import LigandSearch from './LigandSearch.svelte';
  import { getPlugins, submitJob, getJobs, getJob, getLigandDatabases } from '$lib/api';
  import type { Plugin, PluginInputField } from '$lib/api';
  import { getCurrentStructureText } from '$lib/viewer';
  import { isAuthenticated } from '$lib/auth';
  import { pipeline } from '$lib/pipeline.svelte.ts';

  let {
    onMDView = undefined,
  }: {
    onMDView?: (frames: string[], energy: any, compoundId: string) => void;
  } = $props();

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

  <!-- Stage 3: Molecular Docking -->
  <div id="stage-docking">
    <Panel title="Molecular Docking">
      {#snippet children()}
        <div class="stage-header" role="button" tabindex="0"
          onclick={() => pipeline.updateStage('docking', { collapsed: !pipeline.stages.docking.collapsed })}
          onkeydown={(e) => { if (e.key === 'Enter') pipeline.updateStage('docking', { collapsed: !pipeline.stages.docking.collapsed }); }}>
          <span class="stage-label">3. Molecular Docking</span>
          <span class="status-badge {pipeline.statusBadgeClass(pipeline.stages.docking.status)}">{pipeline.stages.docking.status}</span>
        </div>

        {#if pipeline.stages.docking.status === 'pending' || pipeline.stages.docking.status === 'failed'}
          {#if !pipeline.stages.target.jobName || !pipeline.stages.library.jobName}
            <p class="prereq-msg">Complete Target Prep and Library Prep in the Explore tab first.</p>
          {:else}
            <form class="stage-form" onsubmit={(e) => { e.preventDefault(); pipeline.handleDockingSubmit(); }}>
              <div class="form-field">
                <label class="form-label">Engines</label>
                <div class="engine-checks">
                  <label class="toggle-label"><input type="checkbox" bind:checked={pipeline.engVina} /><span>Vina 1.2</span></label>
                  <label class="toggle-label"><input type="checkbox" bind:checked={pipeline.engGnina} /><span>GNINA</span></label>
                  <label class="toggle-label"><input type="checkbox" bind:checked={pipeline.engVinaGpu} /><span>Vina GPU</span></label>
                </div>
              </div>
              <div class="form-field">
                <label class="form-label" for="cp-exh">Exhaustiveness: {pipeline.exhaustiveness}</label>
                <input id="cp-exh" type="range" min="1" max="32" step="1" bind:value={pipeline.exhaustiveness} class="form-slider" />
                <div class="slider-ticks"><span>1</span><span>8</span><span>16</span><span>32</span></div>
              </div>
              <div class="ref-info">
                <span class="ref-chip">Target: {pipeline.stages.target.jobName}</span>
                <span class="ref-chip">Library: {pipeline.stages.library.jobName}</span>
              </div>
              <button type="submit" class="submit-btn" disabled={pipeline.dockSubmitting || (!pipeline.engVina && !pipeline.engGnina && !pipeline.engVinaGpu)}>
                {pipeline.dockSubmitting ? 'Submitting...' : 'Start Docking'}
              </button>
            </form>
          {/if}
        {/if}

        {#if pipeline.stages.docking.status === 'running'}
          <div class="running-indicator">
            <span class="pulse-dot"></span>
            <span class="running-text">Docking {pipeline.stages.docking.jobName ?? ''}...</span>
          </div>
        {/if}
        {#if pipeline.stages.docking.error}<p class="error-msg">{pipeline.stages.docking.error}</p>{/if}

        {#if pipeline.canAdvance('docking')}
          {#if pipeline.dockingSummary}
            <div class="dock-summary">
              <div class="dock-summary-stats">
                <span class="dock-stat"><strong>{pipeline.dockingSummary.unique_compounds.toLocaleString()}</strong> compounds</span>
                <span class="dock-stat">best <strong>{pipeline.dockingSummary.best_affinity?.toFixed(1)}</strong> kcal/mol</span>
              </div>
              <div class="cutoff-table">
                {#each Object.entries(pipeline.dockingSummary.cutoff_counts).sort(([a], [b]) => parseFloat(a) - parseFloat(b)) as [cutoff, count]}
                  <div class="cutoff-row">
                    <span class="cutoff-val">≤ {cutoff}</span>
                    <span class="cutoff-bar-wrap">
                      <span class="cutoff-bar" style="width:{Math.min(100, (count as number) / pipeline.dockingSummary.unique_compounds * 100 * 8)}%"></span>
                    </span>
                    <span class="cutoff-count">{(count as number).toLocaleString()}</span>
                  </div>
                {/each}
              </div>
            </div>
          {/if}
          <div class="pose-browser">
            <div class="pose-browser-header">
              <span class="pose-browser-title">Results ({pipeline.dockResultsTotal.toLocaleString()})</span>
              {#if pipeline.dockResultsLoading}<span class="loading-inline">loading...</span>{/if}
            </div>
            {#if pipeline.dockResults.length > 0}
              <div class="pose-list">
                {#each pipeline.dockResults as hit}
                  <div class="pose-row">
                    <div class="pose-row-main">
                      <span class="pose-rank">#{hit.consensus_rank}</span>
                      <span class="pose-id" title={hit.compound_id}>{hit.compound_id?.slice(0, 20)}</span>
                      <span class="pose-score">{hit.per_engine?.[0]?.raw_score?.toFixed(1) ?? '?'}</span>
                    </div>
                    <div class="pose-row-actions">
                      <button class="pose-action-btn" disabled={pipeline.loadingPoseId === hit.compound_id}
                        onclick={() => pipeline.loadPoseInViewer(hit.compound_id)}>
                        {pipeline.loadingPoseId === hit.compound_id ? '...' : '3D'}
                      </button>
                      {#if hit.smiles}
                        <button class="pose-action-btn"
                          onclick={() => {
                            document.dispatchEvent(new CustomEvent('khemeia:dock-network', {
                              bubbles: true,
                              detail: { smiles: hit.smiles, jobName: pipeline.stages.docking.jobName, compoundId: hit.compound_id }
                            }));
                          }}>Net</button>
                      {/if}
                    </div>
                  </div>
                {/each}
              </div>
              <div class="pose-pagination">
                <button class="page-btn" disabled={pipeline.dockResultsPage <= 1}
                  onclick={() => pipeline.loadDockResults(pipeline.stages.docking.jobName!, pipeline.dockResultsPage - 1)}>‹</button>
                <span class="page-info">{pipeline.dockResultsPage} / {Math.ceil(pipeline.dockResultsTotal / pipeline.DOCK_PER_PAGE) || 1}</span>
                <button class="page-btn" disabled={pipeline.dockResultsPage * pipeline.DOCK_PER_PAGE >= pipeline.dockResultsTotal}
                  onclick={() => pipeline.loadDockResults(pipeline.stages.docking.jobName!, pipeline.dockResultsPage + 1)}>›</button>
              </div>
            {:else if !pipeline.dockResultsLoading}
              <span class="empty-msg">No results loaded.</span>
            {/if}
          </div>
        {/if}
      {/snippet}
    </Panel>
  </div>

  <!-- Stage 4: MD Simulation -->
  <div id="stage-md">
    <Panel title="MD Simulation">
      {#snippet children()}
        <div class="stage-header" role="button" tabindex="0"
          onclick={() => pipeline.updateStage('md', { collapsed: !pipeline.stages.md.collapsed })}
          onkeydown={(e) => { if (e.key === 'Enter') pipeline.updateStage('md', { collapsed: !pipeline.stages.md.collapsed }); }}>
          <span class="stage-label">4. MD Simulation</span>
          <span class="status-badge {pipeline.statusBadgeClass(pipeline.stages.md.status)}">{pipeline.stages.md.status}</span>
        </div>

        {#if pipeline.stages.md.status === 'pending' || pipeline.stages.md.status === 'failed'}
          {#if !pipeline.stages.docking.jobName}
            <p class="prereq-msg">Complete Docking first.</p>
          {:else}
            <form class="stage-form" onsubmit={(e) => { e.preventDefault(); pipeline.handleMDSubmit(); }}>
              <div class="form-field">
                <label class="form-label" for="cp-ff">Protein Force Field</label>
                <select id="cp-ff" class="form-select" bind:value={pipeline.mdForceField}>
                  <option value="amber99sb-ildn">AMBER99SB-ILDN</option>
                  <option value="amber14sb">AMBER14SB</option>
                  <option value="charmm36m">CHARMM36m</option>
                </select>
              </div>
              <div class="form-field">
                <label class="form-label" for="cp-lff">Ligand Force Field</label>
                <select id="cp-lff" class="form-select" bind:value={pipeline.mdLigandFF}>
                  <option value="gaff2">GAFF2</option>
                  <option value="gaff">GAFF</option>
                </select>
              </div>
              <div class="form-field">
                <label class="form-label" for="cp-steps">MD Steps: {pipeline.mdNSteps.toLocaleString()} ({(pipeline.mdNSteps * 0.002).toFixed(0)} ps)</label>
                <input id="cp-steps" type="range" min="50000" max="2000000" step="50000" bind:value={pipeline.mdNSteps} class="form-slider" />
                <div class="slider-ticks"><span>100 ps</span><span>1 ns</span><span>2 ns</span><span>4 ns</span></div>
              </div>
              <div class="form-field">
                <label class="form-label" for="cp-cutoff">
                  Affinity cutoff: ≤ {pipeline.mdAffinityCutoff.toFixed(1)} kcal/mol
                  {#if pipeline.mdEligibleCount !== null}<span class="eligible-count">({pipeline.mdEligibleCount.toLocaleString()} eligible)</span>{/if}
                </label>
                <input id="cp-cutoff" type="range" min="-9" max="-5" step="0.5" bind:value={pipeline.mdAffinityCutoff} class="form-slider" />
                <div class="slider-ticks"><span>-9</span><span>-7.5</span><span>-6</span><span>-5</span></div>
              </div>
              <div class="form-field">
                <label class="form-label" for="cp-topn">Top-N: {pipeline.mdTopN}</label>
                <input id="cp-topn" type="range" min="1" max="20" step="1" bind:value={pipeline.mdTopN} class="form-slider" />
                <div class="slider-ticks"><span>1</span><span>5</span><span>10</span><span>20</span></div>
              </div>
              <div class="filter-toggles">
                <label class="toggle-label"><input type="checkbox" bind:checked={pipeline.mdUseRESP} /><span>RESP Charges (HF/6-31G*)</span></label>
              </div>
              <div class="ref-info">
                <span class="ref-chip">Docking: {pipeline.stages.docking.jobName}</span>
                <span class="ref-chip">Target: {pipeline.stages.target.jobName}</span>
              </div>
              <button type="submit" class="submit-btn" disabled={pipeline.mdSubmitting}>
                {pipeline.mdSubmitting ? 'Submitting...' : 'Run MD Simulation'}
              </button>
            </form>
          {/if}
        {/if}

        {#if pipeline.stages.md.status === 'running'}
          <div class="running-indicator">
            <span class="pulse-dot"></span>
            <span class="running-text">
              {#if pipeline.mdJobStatus?.compounds?.length > 0}
                MD: {pipeline.mdJobStatus.completed ?? 0}/{pipeline.mdJobStatus.compounds.length} compounds
              {:else}
                MD simulation {pipeline.stages.md.jobName ?? ''}...
              {/if}
            </span>
          </div>
          {#if pipeline.mdJobStatus?.compounds?.length > 0}
            <div class="md-compound-list">
              {#each pipeline.mdJobStatus.compounds as c}
                <div class="md-compound-row">
                  <span class="md-compound-status-dot" class:done={c.status === 'Completed'} class:running={c.status === 'Running'} class:failed={c.status === 'Failed'}></span>
                  <span class="md-compound-id">{c.compound_id}</span>
                  <span class="md-compound-aff">{c.dock_affinity_kcal_mol?.toFixed(1)}</span>
                  <span class="md-compound-status">{c.status}</span>
                  {#if c.duration_s}<span class="md-compound-dur">{c.duration_s}s</span>{/if}
                </div>
              {/each}
            </div>
          {/if}
          {#if pipeline.mdJobStatus?.progress}
            {@const p = pipeline.mdJobStatus.progress}
            {@const phaseLabel: Record<string,string> = { energy_minimization: 'Energy Minimization', nvt_equilibration: 'NVT Equilibration', npt_equilibration: 'NPT Equilibration', production_md: 'Production MD' }}
            {@const pct = p.total_steps > 0 ? Math.round(p.step / p.total_steps * 100) : null}
            <div class="md-phase-progress">
              <span class="md-phase-label">{phaseLabel[p.phase] ?? p.phase}</span>
              {#if pct !== null}
                <div class="md-step-bar-wrap"><div class="md-step-bar" style="width:{pct}%"></div></div>
                <span class="md-step-pct">{pct}%</span>
                <span class="md-step-detail">{(p.step as number).toLocaleString()} / {(p.total_steps as number).toLocaleString()} steps</span>
              {/if}
            </div>
          {/if}
        {/if}

        {#if pipeline.stages.md.error}<p class="error-msg">{pipeline.stages.md.error}</p>{/if}

        {#if pipeline.stages.md.jobName && (pipeline.stages.md.status === 'succeeded' || pipeline.stages.md.status === 'running')}
          <div class="md-results-section">
            <div class="md-results-label">
              {pipeline.stages.md.status === 'running' ? 'Completed so far' : 'MD Results'}
              {#if pipeline.mdResults.length > 0}<span class="md-results-count">{pipeline.mdResults.length} compound{pipeline.mdResults.length !== 1 ? 's' : ''}</span>{/if}
              <button class="md-refresh-btn" onclick={() => pipeline.loadMDResults(pipeline.stages.md.jobName!)}>↻</button>
            </div>
            {#if pipeline.mdResults.length === 0}
              <p class="md-no-results">No completed compounds yet.</p>
            {:else}
              <div class="md-results-list">
                {#each pipeline.mdResults as r}
                  {@const isLoading = pipeline.mdViewerLoading === r.compound_id}
                  <div class="md-result-row">
                    <span class="md-result-id">{r.compound_id}</span>
                    <span class="md-result-aff">{r.dock_affinity_kcal_mol?.toFixed(2)}</span>
                    <span class="md-result-dur">{r.duration_s ? Math.round(r.duration_s / 60) + 'm' : ''}</span>
                    <button class="md-result-view-btn"
                      onclick={() => pipeline.viewMDCompound(r, onMDView)}
                      disabled={!!pipeline.mdViewerLoading || (!r.has_trajectory && !r.has_energy)}
                      title={!r.has_trajectory && !r.has_energy ? 'Post-processing pending' : 'View trajectory in 3D'}>
                      {#if isLoading}<span class="md-loading-dot"></span>{:else}View{/if}
                    </button>
                  </div>
                {/each}
              </div>
            {/if}
            {#if pipeline.mdViewerError}<p class="md-viewer-error">{pipeline.mdViewerError}</p>{/if}
          </div>
        {/if}

      {/snippet}
    </Panel>
  </div>

  <!-- Stage 5: ADMET -->
  <div id="stage-admet">
    <Panel title="ADMET Prediction">
      {#snippet children()}
        <div class="stage-header" role="button" tabindex="0"
          onclick={() => pipeline.updateStage('admet', { collapsed: !pipeline.stages.admet.collapsed })}
          onkeydown={(e) => { if (e.key === 'Enter') pipeline.updateStage('admet', { collapsed: !pipeline.stages.admet.collapsed }); }}>
          <span class="stage-label">5. ADMET Prediction</span>
          <span class="status-badge {pipeline.statusBadgeClass(pipeline.stages.admet.status)}">{pipeline.stages.admet.status}</span>
        </div>

        {#if pipeline.stages.admet.status === 'pending' || pipeline.stages.admet.status === 'failed'}
          {#if !pipeline.stages.library.jobName}
            <p class="prereq-msg">Complete Library Prep in the Explore tab first.</p>
          {:else}
            <form class="stage-form" onsubmit={(e) => { e.preventDefault(); pipeline.handleADMETSubmit(); }}>
              <div class="form-field">
                <label class="form-label" for="cp-mpo">MPO Profile</label>
                <select id="cp-mpo" class="form-select" bind:value={pipeline.mpoProfile}>
                  <option value="oral">Oral Drug</option>
                  <option value="cns">CNS Penetrant</option>
                  <option value="oncology">Oncology</option>
                  <option value="antimicrobial">Antimicrobial</option>
                </select>
              </div>
              <div class="ref-info">
                <span class="ref-chip">Library: {pipeline.stages.library.jobName}</span>
              </div>
              <button type="submit" class="submit-btn" disabled={pipeline.admetSubmitting}>
                {pipeline.admetSubmitting ? 'Submitting...' : 'Run ADMET'}
              </button>
            </form>
          {/if}
        {/if}

        {#if pipeline.stages.admet.status === 'running'}
          <div class="running-indicator">
            <span class="pulse-dot"></span>
            <span class="running-text">ADMET {pipeline.stages.admet.jobName ?? ''}...</span>
          </div>
        {/if}
        {#if pipeline.stages.admet.error}<p class="error-msg">{pipeline.stages.admet.error}</p>{/if}

        {#if pipeline.stages.admet.status === 'succeeded'}
          <div class="admet-results">
            <div class="admet-results-header">
              <span class="admet-results-title">Results {pipeline.admetResultsTotal > 0 ? `(${pipeline.admetResultsTotal})` : ''}</span>
              {#if pipeline.admetResultsLoading}
                <span class="loading-inline">loading...</span>
              {:else}
                <button class="refresh-btn" onclick={() => pipeline.loadAdmetResults(pipeline.stages.admet.jobName!)}>↻</button>
              {/if}
            </div>
            {#if pipeline.admetResults.length > 0}
              <div class="admet-list">
                {#each pipeline.admetResults.slice(0, 50) as r}
                  {@const mpo = r.mpo_score ?? 0}
                  {@const mpoColor = mpo >= 0.7 ? '#3fb950' : mpo >= 0.4 ? '#d29922' : '#f85149'}
                  <div class="admet-row">
                    <span class="admet-id" title={r.compound_id}>{r.compound_id?.slice(0, 18)}</span>
                    <span class="admet-mpo" style="color:{mpoColor}">{mpo.toFixed(2)}</span>
                    {#if r.mpo_profile}
                      {@const prof = typeof r.mpo_profile === 'string' ? JSON.parse(r.mpo_profile) : r.mpo_profile}
                      <span class="admet-flags">
                        {#if prof.hia !== undefined}<span class="admet-pill" class:good={prof.hia > 0.5}>HIA</span>{/if}
                        {#if prof.bbb !== undefined}<span class="admet-pill" class:good={prof.bbb > 0.5}>BBB</span>{/if}
                        {#if prof.cyp_inhibition !== undefined}<span class="admet-pill" class:bad={prof.cyp_inhibition > 0.5}>CYP</span>{/if}
                      </span>
                    {/if}
                  </div>
                {/each}
              </div>
            {:else if !pipeline.admetResultsLoading}
              <span class="empty-msg">No ADMET predictions found.</span>
            {/if}
          </div>
          <div class="advance-row">
            <span class="pipeline-done">Pipeline complete.</span>
          </div>
        {/if}
      {/snippet}
    </Panel>
  </div>

  <div class="section-divider">
    <span class="section-label">QC Calculations</span>
  </div>

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

  .section-divider {
    display: flex;
    align-items: center;
    padding: 12px 0 4px;
  }
  .section-label { font-size: 11px; font-weight: 700; color: var(--text-muted, #484f58); text-transform: uppercase; letter-spacing: 0.5px; }

  /* Pipeline stage styles */
  .stage-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; cursor: pointer; }
  .stage-label { font-size: 12px; font-weight: 600; color: var(--text-primary, #e6edf3); }
  .status-badge { font-size: 10px; font-weight: 600; text-transform: uppercase; padding: 1px 6px; border-radius: 3px; white-space: nowrap; }
  .status-badge.pending { background: rgba(88,166,255,0.1); color: var(--accent, #58a6ff); }
  .status-badge.running { background: rgba(210,153,34,0.15); color: #d29922; }
  .status-badge.completed { background: rgba(63,185,80,0.15); color: #3fb950; }
  .status-badge.failed { background: rgba(248,81,73,0.15); color: #f85149; }

  .stage-form { display: flex; flex-direction: column; gap: 10px; }
  .form-slider { width: 100%; accent-color: var(--accent, #58a6ff); }
  .slider-ticks { display: flex; justify-content: space-between; font-size: 9px; color: var(--text-muted, #484f58); padding: 0 2px; }
  .filter-toggles, .engine-checks { display: flex; gap: 12px; flex-wrap: wrap; }
  .toggle-label { display: flex; align-items: center; gap: 4px; font-size: 12px; color: var(--text-secondary, #8b949e); cursor: pointer; }
  .toggle-label input[type="checkbox"] { accent-color: var(--accent, #58a6ff); }
  .eligible-count { font-size: 10px; color: var(--text-muted, #484f58); margin-left: 4px; }

  .submit-btn { background: var(--accent, #58a6ff); border: none; color: #000; font-size: 13px; font-weight: 600; padding: 8px 12px; border-radius: 6px; cursor: pointer; width: 100%; }
  .submit-btn:hover:not(:disabled) { opacity: 0.9; }
  .submit-btn:disabled { opacity: 0.4; cursor: not-allowed; }

  .ref-info { display: flex; gap: 6px; flex-wrap: wrap; }
  .ref-chip { font-size: 10px; font-family: 'SF Mono', monospace; color: var(--text-muted, #484f58); background: rgba(0,0,0,0.2); border: 1px solid rgba(48,54,61,0.4); border-radius: 3px; padding: 2px 6px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 140px; }

  .prereq-msg { color: var(--text-muted, #484f58); font-size: 12px; font-style: italic; padding: 8px 0; }
  .error-msg { color: #f85149; font-size: 12px; margin-top: 4px; }

  .running-indicator { display: flex; align-items: center; gap: 8px; padding: 8px 0; }
  .pulse-dot { width: 8px; height: 8px; border-radius: 50%; background: #d29922; animation: pulse 1.5s ease-in-out infinite; flex-shrink: 0; }
  @keyframes pulse { 0%, 100% { opacity: 1; transform: scale(1); } 50% { opacity: 0.5; transform: scale(0.8); } }
  .running-text { font-size: 12px; color: #d29922; }

  .done-label { font-size: 11px; color: #3fb950; font-family: 'SF Mono', monospace; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 160px; }
  .advance-row { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 8px 0; flex-wrap: wrap; }
  .advance-btn { background: rgba(63,185,80,0.15); border: 1px solid rgba(63,185,80,0.4); color: #3fb950; font-size: 11px; font-weight: 600; padding: 4px 12px; border-radius: 4px; cursor: pointer; white-space: nowrap; }
  .advance-btn:hover { background: rgba(63,185,80,0.25); }
  .pipeline-done { font-size: 11px; color: var(--text-secondary, #8b949e); }

  /* Docking results */
  .dock-summary { margin: 8px 0; }
  .dock-summary-stats { display: flex; gap: 12px; margin-bottom: 6px; }
  .dock-stat { font-size: 12px; color: var(--text-secondary, #8b949e); }
  .cutoff-table { display: flex; flex-direction: column; gap: 2px; }
  .cutoff-row { display: flex; align-items: center; gap: 6px; font-size: 11px; }
  .cutoff-val { font-family: 'SF Mono', monospace; color: var(--text-muted, #484f58); width: 36px; flex-shrink: 0; }
  .cutoff-bar-wrap { flex: 1; height: 4px; background: rgba(48,54,61,0.4); border-radius: 2px; overflow: hidden; }
  .cutoff-bar { height: 100%; background: var(--accent, #58a6ff); border-radius: 2px; min-width: 2px; }
  .cutoff-count { font-family: 'SF Mono', monospace; color: var(--text-secondary, #8b949e); width: 40px; text-align: right; flex-shrink: 0; }

  .pose-browser { margin: 8px 0; }
  .pose-browser-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 4px; }
  .pose-browser-title { font-size: 11px; font-weight: 600; color: var(--text-secondary, #8b949e); text-transform: uppercase; letter-spacing: 0.3px; }
  .loading-inline { font-size: 10px; color: var(--text-muted, #484f58); }
  .pose-list { display: flex; flex-direction: column; gap: 2px; }
  .pose-row { display: flex; align-items: center; gap: 4px; padding: 3px 4px; border-radius: 3px; border: 1px solid rgba(48,54,61,0.3); }
  .pose-row:hover { border-color: rgba(88,166,255,0.3); }
  .pose-row-main { display: flex; align-items: center; gap: 6px; flex: 1; min-width: 0; }
  .pose-rank { font-size: 10px; color: var(--text-muted, #484f58); width: 24px; flex-shrink: 0; }
  .pose-id { font-size: 11px; font-family: 'SF Mono', monospace; color: var(--text-secondary, #8b949e); flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pose-score { font-size: 11px; font-weight: 600; color: var(--accent, #58a6ff); white-space: nowrap; }
  .pose-row-actions { display: flex; gap: 3px; }
  .pose-action-btn { background: rgba(88,166,255,0.1); border: 1px solid rgba(88,166,255,0.3); color: var(--accent, #58a6ff); font-size: 10px; padding: 1px 6px; border-radius: 2px; cursor: pointer; }
  .pose-action-btn:disabled { opacity: 0.4; cursor: not-allowed; }
  .pose-pagination { display: flex; align-items: center; justify-content: center; gap: 10px; padding: 6px 0; }
  .page-btn { background: none; border: 1px solid rgba(48,54,61,0.5); color: var(--text-secondary, #8b949e); font-size: 14px; width: 24px; height: 24px; border-radius: 3px; cursor: pointer; display: flex; align-items: center; justify-content: center; }
  .page-btn:disabled { opacity: 0.3; cursor: not-allowed; }
  .page-info { font-size: 11px; color: var(--text-muted, #484f58); }
  .empty-msg { font-size: 11px; color: var(--text-muted, #484f58); }

  /* MD results */
  .md-compound-list { display: flex; flex-direction: column; gap: 2px; margin: 4px 0; }
  .md-compound-row { display: flex; align-items: center; gap: 6px; font-size: 11px; font-family: 'SF Mono', monospace; padding: 2px 0; }
  .md-compound-status-dot { width: 6px; height: 6px; border-radius: 50%; background: rgba(48,54,61,0.5); flex-shrink: 0; }
  .md-compound-status-dot.done { background: #3fb950; }
  .md-compound-status-dot.running { background: #d29922; }
  .md-compound-status-dot.failed { background: #f85149; }
  .md-compound-id { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-secondary, #8b949e); }
  .md-compound-aff { color: var(--accent, #58a6ff); width: 36px; text-align: right; flex-shrink: 0; }
  .md-compound-status { color: var(--text-muted, #484f58); }
  .md-compound-dur { color: var(--text-muted, #484f58); }

  .md-phase-progress { display: flex; align-items: center; gap: 6px; padding: 4px 0; flex-wrap: wrap; }
  .md-phase-label { font-size: 11px; color: var(--text-secondary, #8b949e); }
  .md-step-bar-wrap { flex: 1; height: 4px; background: rgba(48,54,61,0.4); border-radius: 2px; overflow: hidden; min-width: 60px; }
  .md-step-bar { height: 100%; background: #d29922; border-radius: 2px; transition: width 0.3s; }
  .md-step-pct { font-size: 11px; color: #d29922; white-space: nowrap; }
  .md-step-detail { font-size: 10px; color: var(--text-muted, #484f58); white-space: nowrap; }

  .md-results-section { margin: 8px 0; }
  .md-results-label { display: flex; align-items: center; gap: 6px; font-size: 11px; font-weight: 600; color: var(--text-secondary, #8b949e); text-transform: uppercase; letter-spacing: 0.3px; margin-bottom: 4px; }
  .md-results-count { font-size: 10px; color: var(--text-muted, #484f58); font-weight: 400; text-transform: none; }
  .md-refresh-btn { background: none; border: none; color: var(--text-muted, #484f58); font-size: 13px; cursor: pointer; padding: 0; line-height: 1; }
  .md-refresh-btn:hover { color: var(--accent, #58a6ff); }
  .md-no-results { font-size: 12px; color: var(--text-muted, #484f58); margin: 0; }
  .md-results-list { display: flex; flex-direction: column; gap: 2px; }
  .md-result-row { display: flex; align-items: center; gap: 6px; padding: 3px 4px; border-radius: 3px; border: 1px solid rgba(48,54,61,0.3); }
  .md-result-id { font-size: 11px; font-family: 'SF Mono', monospace; color: var(--text-secondary, #8b949e); flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .md-result-aff { font-size: 11px; color: var(--accent, #58a6ff); white-space: nowrap; }
  .md-result-dur { font-size: 10px; color: var(--text-muted, #484f58); white-space: nowrap; }
  .md-result-view-btn { background: rgba(88,166,255,0.1); border: 1px solid rgba(88,166,255,0.3); color: var(--accent, #58a6ff); font-size: 10px; padding: 2px 8px; border-radius: 3px; cursor: pointer; white-space: nowrap; }
  .md-result-view-btn:disabled { opacity: 0.3; cursor: not-allowed; }
  .md-loading-dot { display: inline-block; width: 6px; height: 6px; border-radius: 50%; background: var(--accent, #58a6ff); animation: pulse 1s ease-in-out infinite; }
  .md-viewer-error { color: #f85149; font-size: 11px; margin-top: 4px; }

  /* ADMET results */
  .admet-results { margin: 8px 0; }
  .admet-results-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 4px; }
  .admet-results-title { font-size: 11px; font-weight: 600; color: var(--text-secondary, #8b949e); text-transform: uppercase; letter-spacing: 0.3px; }
  .refresh-btn { background: none; border: none; color: var(--text-muted, #484f58); font-size: 13px; cursor: pointer; padding: 0; }
  .refresh-btn:hover { color: var(--accent, #58a6ff); }
  .admet-list { display: flex; flex-direction: column; gap: 2px; }
  .admet-row { display: flex; align-items: center; gap: 6px; padding: 3px 4px; border-radius: 3px; border: 1px solid rgba(48,54,61,0.3); }
  .admet-id { font-size: 11px; font-family: 'SF Mono', monospace; color: var(--text-secondary, #8b949e); flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .admet-mpo { font-size: 11px; font-weight: 600; white-space: nowrap; }
  .admet-flags { display: flex; gap: 3px; }
  .admet-pill { font-size: 9px; padding: 1px 4px; border-radius: 2px; background: rgba(248,81,73,0.15); color: #f85149; font-weight: 600; }
  .admet-pill.good { background: rgba(63,185,80,0.15); color: #3fb950; }
  .admet-pill.bad { background: rgba(248,81,73,0.15); color: #f85149; }

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
