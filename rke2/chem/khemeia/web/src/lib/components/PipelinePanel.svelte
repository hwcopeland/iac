<script lang="ts">
  import Panel from './Panel.svelte';
  import {
    submitTargetPrep, getTargetPrep, getTargetPockets, selectPocket,
    submitLibraryPrep, getLibraryPrep,
    submitDocking, getDockingV2Job, listDockingJobs, getDockingV2Summary,
    submitADMET, getADMETJob,
    submitMD, getMDJob,
    advanceStage,
    AuthError,
  } from '$lib/api';
  import { login } from '$lib/auth';
  import { loadFile, showPocketMarkers, clearPocketMarkers, focusPocketCenter } from '$lib/viewer';

  type StageStatus = 'pending' | 'running' | 'succeeded' | 'failed';

  interface StageState {
    status: StageStatus;
    jobName: string | null;
    error: string;
    collapsed: boolean;
  }

  // --- Stage state ---
  let stages = $state<Record<string, StageState>>({
    target:  { status: 'pending', jobName: null, error: '', collapsed: false },
    library: { status: 'pending', jobName: null, error: '', collapsed: true },
    docking: { status: 'pending', jobName: null, error: '', collapsed: true },
    md:      { status: 'pending', jobName: null, error: '', collapsed: true },
    admet:   { status: 'pending', jobName: null, error: '', collapsed: true },
  });

  // --- Target Prep form ---
  let pdbId = $state('');
  let bindingSiteMode = $state<'native-ligand' | 'custom-box' | 'pocket-detection'>('native-ligand');
  let nativeLigandId = $state('');
  let targetSubmitting = $state(false);

  // --- Library Prep form ---
  let libSource = $state<'smiles' | 'chembl'>('smiles');
  let smilesText = $state('');
  let chemblTarget = $state('');
  let chemblMaxPhase = $state(0);
  let chemblMwMin = $state('');
  let chemblMwMax = $state('');
  let chemblLogpMin = $state('');
  let chemblLogpMax = $state('');
  let chemblHbaMax = $state('');
  let chemblHbdMax = $state('');
  let filterLipinski = $state(true);
  let filterVeber = $state(true);
  let filterPAINS = $state(true);
  let libSubmitting = $state(false);

  // Example drug molecules for SMILES input
  const EXAMPLE_SMILES = [
    'CC(=O)Oc1ccccc1C(=O)O',           // Aspirin
    'CC(C)Cc1ccc(cc1)C(C)C(=O)O',      // Ibuprofen
    'OC(=O)c1ccccc1O',                   // Salicylic acid
    'CC(=O)Nc1ccc(O)cc1',               // Acetaminophen
    'CN1C=NC2=C1C(=O)N(C(=O)N2C)C',    // Caffeine
    'c1ccc2c(c1)cc1ccc3cccc4ccc2c1c34', // Pyrene
    'CC12CCC3C(C1CCC2O)CCC4=CC(=O)CCC34C', // Testosterone
    'OC[C@H]1OC(O)[C@H](O)[C@@H](O)[C@@H]1O', // Glucose
    'CC(C)NCC(O)c1ccc(O)c(CO)c1',       // Salbutamol
    'Clc1ccc(cc1)C(c1ccc(Cl)cc1)C(Cl)(Cl)Cl', // DDT (historical ref)
  ].join('\n');

  // Computed: count of valid (non-empty) SMILES lines
  let smilesCount = $derived(
    smilesText.split('\n').map(s => s.trim()).filter(Boolean).length
  );

  // --- Docking form ---
  let engVina = $state(true);
  let engGnina = $state(false);
  let engVinaGpu = $state(false);
  let exhaustiveness = $state(8);
  let dockSubmitting = $state(false);

  // --- MD Simulation form ---
  let mdForceField = $state<'amber99sb-ildn' | 'amber14sb' | 'charmm36m'>('amber99sb-ildn');
  let mdLigandFF = $state<'gaff2' | 'gaff'>('gaff2');
  let mdNSteps = $state(500000);
  let mdTopN = $state(10);
  let mdAffinityCutoff = $state(-7.0);
  let mdUseRESP = $state(false);
  let mdSubmitting = $state(false);

  let mdEligibleCount = $derived.by(() => {
    if (!dockingSummary?.cutoff_counts) return null;
    const key = mdAffinityCutoff.toFixed(1);
    return dockingSummary.cutoff_counts[key] ?? null;
  });

  // --- ADMET form ---
  let mpoProfile = $state<'oral' | 'cns' | 'oncology' | 'antimicrobial'>('oral');
  let admetSubmitting = $state(false);

  // --- Custom box inputs ---
  let boxCenterX = $state(0);
  let boxCenterY = $state(0);
  let boxCenterZ = $state(0);
  let boxSizeX = $state(20);
  let boxSizeY = $state(20);
  let boxSizeZ = $state(20);

  // --- Pocket detection results ---
  let pockets = $state<any[] | null>(null);
  let selectedPocketIdx = $state<number | null>(null);

  // --- Target prep result (for displaying binding site info) ---
  let targetPrepResult = $state<any | null>(null);

  // --- Library prep result (for displaying resolved compound count) ---
  let libraryStatus = $state<any | null>(null);

  // --- MD job status (for per-compound progress display) ---
  let mdJobStatus = $state<any | null>(null);

  // --- Docking summary (loaded when docking succeeds) ---
  let dockingSummary = $state<any | null>(null);

  async function loadDockingSummary(name: string) {
    try {
      dockingSummary = await getDockingV2Summary(name);
    } catch {}
  }

  // --- Session state ---
  let sessionExpired = $state(false);

  // --- Recent runs ---
  let recentJobs = $state<any[]>([]);
  let recentOpen = $state(false);

  async function loadRecentJobs() {
    recentOpen = !recentOpen;
    if (recentOpen && recentJobs.length === 0) {
      try {
        const res = await listDockingJobs();
        recentJobs = res.jobs ?? [];
      } catch {}
    }
  }

  async function restorePipeline(job: any) {
    recentOpen = false;
    // Set all three stage job names immediately
    updateStage('target',  { jobName: job.receptor_ref, status: 'running', error: '', collapsed: false });
    updateStage('library', { jobName: job.library_ref,  status: 'running', error: '', collapsed: false });
    updateStage('docking', { jobName: job.name,          status: 'running', error: '', collapsed: false });
    // Fetch current status for each and update
    try {
      const [tRes, lRes, dRes] = await Promise.all([
        getTargetPrep(job.receptor_ref),
        getLibraryPrep(job.library_ref),
        getDockingV2Job(job.name),
      ]);
      const toStatus = (phase: string): StageStatus => {
        const p = (phase || '').toLowerCase();
        if (p === 'succeeded' || p === 'completed') return 'succeeded';
        if (p === 'failed') return 'failed';
        if (p === 'running') return 'running';
        return 'pending';
      };
      updateStage('target',  { status: toStatus(tRes.phase || tRes.status) });
      updateStage('library', { status: toStatus(lRes.phase || lRes.status) });
      const dockStatus = toStatus(dRes.phase || dRes.status);
      updateStage('docking', { status: dockStatus });
      if (dockStatus === 'succeeded') loadDockingSummary(job.name);
      // Re-poll anything still running
      if (stages.target.status  === 'running') startPoll('target',  getTargetPrep);
      if (stages.library.status === 'running') startPoll('library', getLibraryPrep);
      if (stages.docking.status === 'running') startPoll('docking', getDockingV2Job);
    } catch {}
  }

  function clearPipeline() {
    for (const k of ['target', 'library', 'docking', 'md', 'admet']) {
      for (const t of Object.values(pollTimers)) clearTimeout(t);
      stages[k] = { status: 'pending', jobName: null, error: '', collapsed: k === 'target' ? false : true };
    }
    try { localStorage.removeItem(PIPELINE_STORAGE_KEY); } catch {}
  }

  // --- Polling ---
  let pollTimers = $state<Record<string, ReturnType<typeof setTimeout>>>({});
  const pollFailures: Record<string, number> = {};
  const POLL_MAX_FAILURES = 3;

  function updateStage(key: string, patch: Partial<StageState>) {
    stages[key] = { ...stages[key], ...patch };
  }

  function stageOrder(key: string): number {
    return { target: 0, library: 1, docking: 2, md: 3, admet: 4 }[key] ?? 99;
  }

  function canAdvance(key: string): boolean {
    return stages[key].status === 'succeeded';
  }

  function nextStageKey(key: string): string | null {
    const order = ['target', 'library', 'docking', 'md', 'admet'];
    const idx = order.indexOf(key);
    return idx >= 0 && idx < order.length - 1 ? order[idx + 1] : null;
  }

  function handleAdvance(key: string) {
    const next = nextStageKey(key);
    if (!next) return;
    updateStage(next, { collapsed: false });
    // Scroll into view after a tick
    requestAnimationFrame(() => {
      const el = document.getElementById(`stage-${next}`);
      el?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    });
  }

  // --- Poll a stage job status ---
  function startPoll(stageKey: string, pollFn: (name: string) => Promise<any>) {
    const name = stages[stageKey].jobName;
    if (!name) return;
    // Clear any existing timer
    if (pollTimers[stageKey]) clearTimeout(pollTimers[stageKey]);

    const tick = async () => {
      if (sessionExpired) return;
      try {
        const res = await pollFn(name);
        pollFailures[stageKey] = 0;
        const phase = (res.phase || res.status || '').toLowerCase();

        // Stash stage-specific status for progress display
        if (stageKey === 'library') libraryStatus = res;
        if (stageKey === 'md') mdJobStatus = res;

        if (phase === 'completed' || phase === 'succeeded') {
          updateStage(stageKey, { status: 'succeeded', error: '' });
          if (stageKey === 'docking' && stages.docking.jobName) {
            loadDockingSummary(stages.docking.jobName);
          }
          // Stash poll result so we can display binding site info
          if (stageKey === 'target') {
            targetPrepResult = res;
            if (res?.pockets?.length && res?.receptor_pdb_url) {
              try {
                const pdbRes = await fetch(res.receptor_pdb_url);
                if (pdbRes.ok) {
                  const pdbText = await pdbRes.text();
                  await loadFile(pdbText, 'pdb');
                }
                await showPocketMarkers(res.pockets.map((p: any) => ({
                  center: p.center,
                  score: p.consensus_score ?? 0,
                  rank: p.rank ?? 0,
                })));
              } catch (e) {
                console.warn('Failed to load receptor/pockets into viewer:', e);
              }
            }
          }
          return;
        }
        if (phase === 'failed') {
          updateStage(stageKey, { status: 'failed', error: res.error || res.error_output || 'Job failed' });
          return;
        }
        // Still running — poll again
        pollTimers[stageKey] = setTimeout(tick, 10_000);
      } catch (e: any) {
        if (e instanceof AuthError) {
          sessionExpired = true;
          // Job is still running server-side — don't mark as failed
          return;
        }
        // Tolerate transient errors (DNS blips, 502s) — only fail after POLL_MAX_FAILURES consecutive errors
        pollFailures[stageKey] = (pollFailures[stageKey] ?? 0) + 1;
        if (pollFailures[stageKey] < POLL_MAX_FAILURES) {
          pollTimers[stageKey] = setTimeout(tick, 15_000);
          return;
        }
        updateStage(stageKey, { status: 'failed', error: e.message || 'Poll failed' });
      }
    };
    tick();
  }

  // --- Submit handlers ---

  function targetFormValid(): boolean {
    if (!pdbId) return false;
    if (bindingSiteMode === 'native-ligand' && !nativeLigandId.trim()) return false;
    return true;
  }

  async function handleTargetSubmit() {
    if (!targetFormValid()) {
      updateStage('target', { error: 'Please fill in all required fields' });
      return;
    }
    targetSubmitting = true;
    updateStage('target', { error: '', status: 'running' });
    try {
      const params: any = {
        pdb_id: pdbId,
        binding_site_mode: bindingSiteMode,
      };
      if (bindingSiteMode === 'native-ligand') {
        params.native_ligand_id = nativeLigandId;
      }
      if (bindingSiteMode === 'custom-box') {
        params.custom_box = {
          center: [boxCenterX, boxCenterY, boxCenterZ],
          size: [boxSizeX, boxSizeY, boxSizeZ],
        };
      }
      const res = await submitTargetPrep(params);
      updateStage('target', { jobName: res.name || res.job_name, status: 'running' });
      startPoll('target', getTargetPrep);
    } catch (e: any) {
      updateStage('target', { status: 'failed', error: e.message || 'Submission failed' });
    } finally {
      targetSubmitting = false;
    }
  }

  function libFormValid(): boolean {
    if (libSource === 'smiles') return smilesCount > 0;
    if (libSource === 'chembl') {
      return chemblTarget.trim() !== '' ||
        !!chemblMwMin || !!chemblMwMax ||
        !!chemblLogpMin || !!chemblLogpMax ||
        !!chemblHbaMax || !!chemblHbdMax;
    }
    return true;
  }

  async function handleLibrarySubmit() {
    libSubmitting = true;
    updateStage('library', { error: '', status: 'running' });
    try {
      const params: any = {
        source: libSource,
        filters: {
          lipinski: filterLipinski,
          veber: filterVeber,
          pains: filterPAINS,
        },
      };
      params.name = 'pipeline-lib-' + Date.now();
      if (libSource === 'smiles') {
        params.smiles_list = smilesText.split('\n').map((s: string) => s.trim()).filter(Boolean);
      } else {
        const chembl: Record<string, any> = {};
        if (chemblTarget.trim()) chembl.q = chemblTarget.trim();
        if (chemblMaxPhase > 0) chembl.max_phase = chemblMaxPhase;
        if (chemblMwMin) chembl.mw_min = parseFloat(chemblMwMin);
        if (chemblMwMax) chembl.mw_max = parseFloat(chemblMwMax);
        if (chemblLogpMin) chembl.logp_min = parseFloat(chemblLogpMin);
        if (chemblLogpMax) chembl.logp_max = parseFloat(chemblLogpMax);
        if (chemblHbaMax) chembl.hba_max = parseInt(chemblHbaMax);
        if (chemblHbdMax) chembl.hbd_max = parseInt(chemblHbdMax);
        params.chembl = chembl;
      }
      const res = await submitLibraryPrep(params);
      updateStage('library', { jobName: res.name || res.job_name, status: 'running' });
      startPoll('library', getLibraryPrep);
    } catch (e: any) {
      updateStage('library', { status: 'failed', error: e.message || 'Submission failed' });
    } finally {
      libSubmitting = false;
    }
  }

  async function handleDockingSubmit() {
    dockSubmitting = true;
    updateStage('docking', { error: '', status: 'running' });
    try {
      const engines: string[] = [];
      if (engVina) engines.push('vina-1.2');
      if (engGnina) engines.push('gnina');
      if (engVinaGpu) engines.push('vina-gpu');
      const params: any = {
        receptor_ref: stages.target.jobName,
        library_ref: stages.library.jobName,
        engines,
        exhaustiveness,
      };
      const res = await submitDocking(params);
      updateStage('docking', { jobName: res.name || res.job_name, status: 'running' });
      startPoll('docking', getDockingV2Job);
    } catch (e: any) {
      updateStage('docking', { status: 'failed', error: e.message || 'Submission failed' });
    } finally {
      dockSubmitting = false;
    }
  }

  async function handleMDSubmit() {
    mdSubmitting = true;
    updateStage('md', { error: '', status: 'running' });
    try {
      const params: any = {
        dock_job_name: stages.docking.jobName,
        receptor_ref: stages.target.jobName,
        top_n: mdTopN,
        affinity_cutoff: mdAffinityCutoff,
        md_nsteps: mdNSteps,
        force_field: mdForceField,
        ligand_ff: mdLigandFF,
        use_resp: mdUseRESP,
      };
      const res = await submitMD(params);
      updateStage('md', { jobName: res.name || res.job_name, status: 'running' });
      startPoll('md', getMDJob);
    } catch (e: any) {
      updateStage('md', { status: 'failed', error: e.message || 'Submission failed' });
    } finally {
      mdSubmitting = false;
    }
  }

  async function handleADMETSubmit() {
    admetSubmitting = true;
    updateStage('admet', { error: '', status: 'running' });
    try {
      const params: any = {
        library_ref: stages.library.jobName,
        mpo_profile: mpoProfile,
      };
      const res = await submitADMET(params);
      updateStage('admet', { jobName: res.name || res.job_name, status: 'running' });
      startPoll('admet', getADMETJob);
    } catch (e: any) {
      updateStage('admet', { status: 'failed', error: e.message || 'Submission failed' });
    } finally {
      admetSubmitting = false;
    }
  }

  const PIPELINE_STORAGE_KEY = 'khemeia_pipeline_v1';

  const POLL_FNS: Record<string, (name: string) => Promise<any>> = {
    target:  getTargetPrep,
    library: getLibraryPrep,
    docking: getDockingV2Job,
    md:      getMDJob,
    admet:   getADMETJob,
  };

  // Persist jobNames + statuses to localStorage whenever stages change
  $effect(() => {
    const snapshot: Record<string, { jobName: string | null; status: StageStatus }> = {};
    for (const [k, s] of Object.entries(stages)) {
      snapshot[k] = { jobName: s.jobName, status: s.status };
    }
    try { localStorage.setItem(PIPELINE_STORAGE_KEY, JSON.stringify(snapshot)); } catch {}
  });

  // Restore from localStorage on mount and re-poll running jobs
  $effect(() => {
    try {
      const raw = localStorage.getItem(PIPELINE_STORAGE_KEY);
      if (!raw) return;
      const snapshot: Record<string, { jobName: string | null; status: StageStatus }> = JSON.parse(raw);
      for (const [k, v] of Object.entries(snapshot)) {
        if (!v.jobName) continue;
        updateStage(k, { jobName: v.jobName, status: v.status, collapsed: v.status === 'pending' });
        // Re-attach poll for stages that were running
        if (v.status === 'running' && POLL_FNS[k]) {
          startPoll(k, POLL_FNS[k]);
        }
      }
    } catch {}
  });

  // Cleanup poll timers on destroy
  $effect(() => {
    return () => {
      for (const t of Object.values(pollTimers)) clearTimeout(t);
    };
  });

  const stageLabels: Record<string, string> = {
    target:  '1. Target Preparation',
    library: '2. Library Preparation',
    docking: '3. Molecular Docking',
    md:      '4. MD Simulation',
    admet:   '5. ADMET Prediction',
  };

  function statusBadgeClass(s: StageStatus): string {
    if (s === 'succeeded') return 'completed';
    if (s === 'failed') return 'failed';
    if (s === 'running') return 'running';
    return 'pending';
  }
</script>

<div class="pipeline-panels">
  <div class="pipeline-header">
    <span class="pipeline-title">SBDD Pipeline</span>
    <span class="pipeline-subtitle">Structure-Based Drug Discovery</span>
    <div class="pipeline-actions">
      <button class="action-btn" onclick={loadRecentJobs}>
        {recentOpen ? 'Hide runs' : 'Load previous'}
      </button>
      <button class="action-btn danger" onclick={clearPipeline}>New</button>
    </div>
  </div>

  {#if recentOpen}
    <div class="recent-runs">
      {#if recentJobs.length === 0}
        <span class="recent-empty">No previous runs found.</span>
      {:else}
        {#each recentJobs as job}
          <button class="recent-row" onclick={() => restorePipeline(job)}>
            <span class="recent-name">{job.name}</span>
            <span class="recent-badge {job.status.toLowerCase()}">{job.status}</span>
            <span class="recent-meta">{new Date(job.created_at).toLocaleString()}</span>
          </button>
        {/each}
      {/if}
    </div>
  {/if}

  <!-- Stepper indicator -->
  <div class="stepper">
    {#each ['target', 'library', 'docking', 'md', 'admet'] as key, i}
      <div class="step" class:active={!stages[key].collapsed} class:done={stages[key].status === 'succeeded'}>
        <div class="step-dot {statusBadgeClass(stages[key].status)}">
          {#if stages[key].status === 'succeeded'}
            <span class="check-mark">&#10003;</span>
          {:else}
            {i + 1}
          {/if}
        </div>
        {#if i < 4}
          <div class="step-line" class:done={stages[key].status === 'succeeded'}></div>
        {/if}
      </div>
    {/each}
  </div>

  <!-- Session expired banner -->
  {#if sessionExpired}
    <div class="session-banner">
      <span class="session-msg">Session expired — your jobs are still running on the server.</span>
      <button class="session-login-btn" onclick={() => login()}>Log in again</button>
    </div>
  {/if}

  <!-- Stage 1: Target Prep -->
  <div id="stage-target">
    <Panel title={stageLabels.target} collapsed={stages.target.collapsed}>
      {#snippet children()}
        <div class="stage-header" role="button" tabindex="0"
          onclick={() => updateStage('target', { collapsed: !stages.target.collapsed })}
          onkeydown={(e) => { if (e.key === 'Enter') updateStage('target', { collapsed: !stages.target.collapsed }); }}>
          <span class="stage-label">{stageLabels.target}</span>
          <span class="status-badge {statusBadgeClass(stages.target.status)}">{stages.target.status}</span>
        </div>

        {#if stages.target.status === 'pending' || stages.target.status === 'failed'}
          <form class="stage-form" onsubmit={(e) => { e.preventDefault(); handleTargetSubmit(); }}>
            <div class="form-field">
              <label class="form-label" for="pp-pdb">PDB ID <span class="required">*</span></label>
              <input id="pp-pdb" type="text" class="form-input" placeholder="e.g. 1AKE" bind:value={pdbId} required />
            </div>

            <div class="form-field">
              <label class="form-label" for="pp-mode">Binding Site Mode</label>
              <select id="pp-mode" class="form-select" bind:value={bindingSiteMode}>
                <option value="native-ligand">Native Ligand</option>
                <option value="custom-box">Custom Box</option>
                <option value="pocket-detection">Pocket Detection (P2Rank)</option>
              </select>
            </div>

            {#if bindingSiteMode === 'native-ligand'}
              <div class="form-field">
                <label class="form-label" for="pp-lig">Native Ligand ID <span class="required">*</span></label>
                <input id="pp-lig" type="text" class="form-input" placeholder="e.g. ATP" bind:value={nativeLigandId} required />
              </div>
              {#if !nativeLigandId.trim()}
                <p class="validation-msg">Native Ligand ID is required for this mode.</p>
              {/if}
            {/if}

            {#if bindingSiteMode === 'custom-box'}
              <div class="box-grid">
                <div class="box-group">
                  <span class="box-group-label">Center (x, y, z)</span>
                  <div class="box-inputs">
                    <input type="number" step="0.1" class="form-input box-input" placeholder="x" bind:value={boxCenterX} />
                    <input type="number" step="0.1" class="form-input box-input" placeholder="y" bind:value={boxCenterY} />
                    <input type="number" step="0.1" class="form-input box-input" placeholder="z" bind:value={boxCenterZ} />
                  </div>
                </div>
                <div class="box-group">
                  <span class="box-group-label">Size (x, y, z)</span>
                  <div class="box-inputs">
                    <input type="number" step="0.1" min="1" class="form-input box-input" placeholder="x" bind:value={boxSizeX} />
                    <input type="number" step="0.1" min="1" class="form-input box-input" placeholder="y" bind:value={boxSizeY} />
                    <input type="number" step="0.1" min="1" class="form-input box-input" placeholder="z" bind:value={boxSizeZ} />
                  </div>
                </div>
              </div>
            {/if}

            <button type="submit" class="submit-btn" disabled={targetSubmitting || !targetFormValid()}>
              {targetSubmitting ? 'Submitting...' : 'Prepare Target'}
            </button>
          </form>
        {/if}

        {#if stages.target.status === 'running'}
          <div class="running-indicator">
            <span class="pulse-dot"></span>
            <span class="running-text">Preparing target {stages.target.jobName ?? ''}...</span>
          </div>
        {/if}

        {#if stages.target.error}
          <p class="error-msg">{stages.target.error}</p>
        {/if}

        {#if canAdvance('target')}
          {#if targetPrepResult?.binding_site}
            <div class="result-info">
              <span class="result-label">Binding Site</span>
              <div class="result-detail">
                <span class="result-field">Center: ({targetPrepResult.binding_site.center.map((v: number) => v.toFixed(1)).join(', ')})</span>
                <span class="result-field">Size: ({targetPrepResult.binding_site.size.map((v: number) => v.toFixed(1)).join(', ')})</span>
              </div>
            </div>
          {/if}
          {#if targetPrepResult?.pockets?.length}
            <div class="result-info">
              <span class="result-label">Detected Pockets ({targetPrepResult.pockets.length})</span>
              {#each targetPrepResult.pockets.slice(0, 5) as pocket, i}
                <div class="pocket-row" class:selected={selectedPocketIdx === i}>
                  <button class="pocket-focus-btn" type="button"
                    onclick={() => { selectedPocketIdx = i; focusPocketCenter(pocket.center); }}
                    title="Focus camera on this pocket">
                    <span class="pocket-rank">#{pocket.rank}</span>
                    <span class="pocket-score">consensus: {pocket.consensus_score?.toFixed(2) ?? '?'}</span>
                    <span class="pocket-center">({pocket.center.map((v: number) => v.toFixed(1)).join(', ')})</span>
                  </button>
                  <button class="pocket-select-btn" type="button"
                    disabled={selectedPocketIdx === i && targetPrepResult.selected_pocket === i}
                    onclick={async () => {
                      if (!stages.target.jobName) return;
                      selectedPocketIdx = i;
                      try {
                        await selectPocket(stages.target.jobName, i);
                        targetPrepResult = { ...targetPrepResult, selected_pocket: i };
                      } catch (e) {
                        updateStage('target', { error: (e as Error).message || 'Pocket selection failed' });
                      }
                    }}>
                    {targetPrepResult.selected_pocket === i ? 'Selected' : 'Select'}
                  </button>
                </div>
              {/each}
            </div>
          {/if}
          <div class="advance-row">
            <span class="done-label">Target ready: {stages.target.jobName}</span>
            <button class="advance-btn" onclick={() => handleAdvance('target')}>Next: Library Prep</button>
          </div>
        {/if}
      {/snippet}
    </Panel>
  </div>

  <!-- Stage 2: Library Prep -->
  <div id="stage-library">
    <Panel title={stageLabels.library} collapsed={stages.library.collapsed}>
      {#snippet children()}
        <div class="stage-header" role="button" tabindex="0"
          onclick={() => updateStage('library', { collapsed: !stages.library.collapsed })}
          onkeydown={(e) => { if (e.key === 'Enter') updateStage('library', { collapsed: !stages.library.collapsed }); }}>
          <span class="stage-label">{stageLabels.library}</span>
          <span class="status-badge {statusBadgeClass(stages.library.status)}">{stages.library.status}</span>
        </div>

        {#if stages.library.status === 'pending' || stages.library.status === 'failed'}
          <form class="stage-form" onsubmit={(e) => { e.preventDefault(); handleLibrarySubmit(); }}>
            <div class="form-field">
              <label class="form-label" for="lp-source">Source</label>
              <select id="lp-source" class="form-select" bind:value={libSource}>
                <option value="smiles">SMILES Input</option>
                <option value="chembl">ChEMBL Filter</option>
              </select>
            </div>

            {#if libSource === 'smiles'}
              <div class="form-field">
                <label class="form-label" for="lp-smiles">Enter SMILES (one per line)</label>
                <textarea id="lp-smiles" class="form-textarea" rows="6" placeholder="CC(=O)Oc1ccccc1C(=O)O&#10;c1ccccc1" bind:value={smilesText}></textarea>
                <div class="smiles-meta">
                  <span class="compound-count">{smilesCount} compound{smilesCount !== 1 ? 's' : ''}</span>
                  <button type="button" class="load-example-btn" onclick={() => { smilesText = EXAMPLE_SMILES; }}>
                    Load example
                  </button>
                </div>
              </div>
            {:else}
              <div class="form-field">
                <label class="form-label" for="lp-target">ChEMBL Target ID</label>
                <input id="lp-target" type="text" class="form-input" placeholder="e.g. CHEMBL25" bind:value={chemblTarget} />
              </div>
              <div class="form-field">
                <label class="form-label" for="lp-phase">Min Clinical Phase</label>
                <select id="lp-phase" class="form-select" bind:value={chemblMaxPhase}>
                  <option value={0}>Any</option>
                  <option value={1}>Phase I+</option>
                  <option value={2}>Phase II+</option>
                  <option value={3}>Phase III+</option>
                  <option value={4}>Approved</option>
                </select>
              </div>
              <div class="form-field">
                <label class="form-label">MW Range</label>
                <div class="range-row">
                  <input type="number" step="1" class="form-input" placeholder="Min" bind:value={chemblMwMin} />
                  <span class="range-sep">-</span>
                  <input type="number" step="1" class="form-input" placeholder="Max" bind:value={chemblMwMax} />
                </div>
              </div>
              <div class="form-field">
                <label class="form-label">LogP Range</label>
                <div class="range-row">
                  <input type="number" step="0.1" class="form-input" placeholder="Min" bind:value={chemblLogpMin} />
                  <span class="range-sep">-</span>
                  <input type="number" step="0.1" class="form-input" placeholder="Max" bind:value={chemblLogpMax} />
                </div>
              </div>
              <div class="form-field">
                <label class="form-label" for="lp-hba">HBA Max</label>
                <input id="lp-hba" type="number" step="1" min="0" class="form-input" placeholder="e.g. 10" bind:value={chemblHbaMax} />
              </div>
              <div class="form-field">
                <label class="form-label" for="lp-hbd">HBD Max</label>
                <input id="lp-hbd" type="number" step="1" min="0" class="form-input" placeholder="e.g. 5" bind:value={chemblHbdMax} />
              </div>
            {/if}

            <div class="filter-toggles">
              <label class="toggle-label">
                <input type="checkbox" bind:checked={filterLipinski} />
                <span>Lipinski</span>
              </label>
              <label class="toggle-label">
                <input type="checkbox" bind:checked={filterVeber} />
                <span>Veber</span>
              </label>
              <label class="toggle-label">
                <input type="checkbox" bind:checked={filterPAINS} />
                <span>PAINS</span>
              </label>
            </div>

            <button type="submit" class="submit-btn" disabled={libSubmitting || !libFormValid()}>
              {libSubmitting ? 'Submitting...' : 'Prepare Library'}
            </button>
          </form>
        {/if}

        {#if stages.library.status === 'running'}
          <div class="running-indicator">
            <span class="pulse-dot"></span>
            <span class="running-text">
              {#if libraryStatus?.compound_count > 0}
                {libraryStatus.compound_count} compounds found — standardizing &amp; filtering...
              {:else}
                Resolving compounds from source...
              {/if}
            </span>
          </div>
        {/if}

        {#if stages.library.error}
          <p class="error-msg">{stages.library.error}</p>
        {/if}

        {#if canAdvance('library')}
          <div class="advance-row">
            <span class="done-label">Library ready: {stages.library.jobName}</span>
            <button class="advance-btn" onclick={() => handleAdvance('library')}>Next: Docking</button>
          </div>
        {/if}
      {/snippet}
    </Panel>
  </div>

  <!-- Stage 3: Docking -->
  <div id="stage-docking">
    <Panel title={stageLabels.docking} collapsed={stages.docking.collapsed}>
      {#snippet children()}
        <div class="stage-header" role="button" tabindex="0"
          onclick={() => updateStage('docking', { collapsed: !stages.docking.collapsed })}
          onkeydown={(e) => { if (e.key === 'Enter') updateStage('docking', { collapsed: !stages.docking.collapsed }); }}>
          <span class="stage-label">{stageLabels.docking}</span>
          <span class="status-badge {statusBadgeClass(stages.docking.status)}">{stages.docking.status}</span>
        </div>

        {#if (stages.docking.status === 'pending' || stages.docking.status === 'failed')}
          {#if !stages.target.jobName || !stages.library.jobName}
            <p class="prereq-msg">Complete Target Prep and Library Prep first.</p>
          {:else}
            <form class="stage-form" onsubmit={(e) => { e.preventDefault(); handleDockingSubmit(); }}>
              <div class="form-field">
                <label class="form-label">Docking Engines</label>
                <div class="engine-checks">
                  <label class="toggle-label">
                    <input type="checkbox" bind:checked={engVina} />
                    <span>Vina 1.2</span>
                  </label>
                  <label class="toggle-label">
                    <input type="checkbox" bind:checked={engGnina} />
                    <span>GNINA</span>
                  </label>
                  <label class="toggle-label">
                    <input type="checkbox" bind:checked={engVinaGpu} />
                    <span>Vina GPU</span>
                  </label>
                </div>
              </div>

              <div class="form-field">
                <label class="form-label" for="dk-exh">
                  Exhaustiveness: {exhaustiveness}
                </label>
                <input id="dk-exh" type="range" min="1" max="32" step="1" bind:value={exhaustiveness} class="form-slider" />
                <div class="slider-ticks">
                  <span>1</span><span>8</span><span>16</span><span>32</span>
                </div>
              </div>

              <div class="ref-info">
                <span class="ref-chip">Target: {stages.target.jobName}</span>
                <span class="ref-chip">Library: {stages.library.jobName}</span>
              </div>

              <button type="submit" class="submit-btn" disabled={dockSubmitting || (!engVina && !engGnina && !engVinaGpu)}>
                {dockSubmitting ? 'Submitting...' : 'Start Docking'}
              </button>
            </form>
          {/if}
        {/if}

        {#if stages.docking.status === 'running'}
          <div class="running-indicator">
            <span class="pulse-dot"></span>
            <span class="running-text">Docking in progress {stages.docking.jobName ?? ''}...</span>
          </div>
        {/if}

        {#if stages.docking.error}
          <p class="error-msg">{stages.docking.error}</p>
        {/if}

        {#if canAdvance('docking')}
          {#if dockingSummary}
            <div class="dock-summary">
              <div class="dock-summary-stats">
                <span class="dock-stat"><strong>{dockingSummary.unique_compounds.toLocaleString()}</strong> compounds</span>
                <span class="dock-stat">best <strong>{dockingSummary.best_affinity?.toFixed(1)}</strong> kcal/mol</span>
              </div>
              <div class="cutoff-table">
                {#each Object.entries(dockingSummary.cutoff_counts).sort(([a], [b]) => parseFloat(a) - parseFloat(b)) as [cutoff, count]}
                  <div class="cutoff-row">
                    <span class="cutoff-val">≤ {cutoff}</span>
                    <span class="cutoff-bar-wrap">
                      <span class="cutoff-bar" style="width:{Math.min(100, (count as number) / dockingSummary.unique_compounds * 100 * 8)}%"></span>
                    </span>
                    <span class="cutoff-count">{(count as number).toLocaleString()}</span>
                  </div>
                {/each}
              </div>
              <div class="top-hits-label">Top hits</div>
              <div class="top-hits-list">
                {#each dockingSummary.top_hits.slice(0, 10) as hit}
                  <div class="hit-row">
                    <span class="hit-id">{hit.compound_id}</span>
                    <span class="hit-aff">{hit.affinity_kcal_mol.toFixed(1)}</span>
                  </div>
                {/each}
              </div>
            </div>
          {/if}
          <div class="advance-row">
            <span class="done-label">Docking complete: {stages.docking.jobName}</span>
            <button class="advance-btn" onclick={() => handleAdvance('docking')}>Next: MD Simulation</button>
          </div>
        {/if}
      {/snippet}
    </Panel>
  </div>

  <!-- Stage 4: MD Simulation -->
  <div id="stage-md">
    <Panel title={stageLabels.md} collapsed={stages.md.collapsed}>
      {#snippet children()}
        <div class="stage-header" role="button" tabindex="0"
          onclick={() => updateStage('md', { collapsed: !stages.md.collapsed })}
          onkeydown={(e) => { if (e.key === 'Enter') updateStage('md', { collapsed: !stages.md.collapsed }); }}>
          <span class="stage-label">{stageLabels.md}</span>
          <span class="status-badge {statusBadgeClass(stages.md.status)}">{stages.md.status}</span>
        </div>

        {#if (stages.md.status === 'pending' || stages.md.status === 'failed')}
          {#if !stages.docking.jobName}
            <p class="prereq-msg">Complete Docking first.</p>
          {:else}
            <form class="stage-form" onsubmit={(e) => { e.preventDefault(); handleMDSubmit(); }}>
              <div class="form-field">
                <label class="form-label" for="md-ff">Protein Force Field</label>
                <select id="md-ff" class="form-select" bind:value={mdForceField}>
                  <option value="amber99sb-ildn">AMBER99SB-ILDN</option>
                  <option value="amber14sb">AMBER14SB</option>
                  <option value="charmm36m">CHARMM36m</option>
                </select>
              </div>

              <div class="form-field">
                <label class="form-label" for="md-lff">Ligand Force Field</label>
                <select id="md-lff" class="form-select" bind:value={mdLigandFF}>
                  <option value="gaff2">GAFF2</option>
                  <option value="gaff">GAFF</option>
                </select>
              </div>

              <div class="form-field">
                <label class="form-label" for="md-steps">
                  MD Steps: {mdNSteps.toLocaleString()} ({(mdNSteps * 0.002).toFixed(0)} ps)
                </label>
                <input id="md-steps" type="range" min="50000" max="2000000" step="50000"
                  bind:value={mdNSteps} class="form-slider" />
                <div class="slider-ticks">
                  <span>100 ps</span><span>1 ns</span><span>2 ns</span><span>4 ns</span>
                </div>
              </div>

              <div class="form-field">
                <label class="form-label" for="md-cutoff">
                  Affinity cutoff: ≤ {mdAffinityCutoff.toFixed(1)} kcal/mol
                  {#if mdEligibleCount !== null}
                    <span class="eligible-count">({mdEligibleCount.toLocaleString()} eligible)</span>
                  {/if}
                </label>
                <input id="md-cutoff" type="range" min="-9" max="-5" step="0.5"
                  bind:value={mdAffinityCutoff} class="form-slider" />
                <div class="slider-ticks">
                  <span>-9</span><span>-7.5</span><span>-6</span><span>-5</span>
                </div>
              </div>

              <div class="form-field">
                <label class="form-label" for="md-topn">Top-N from eligible: {mdTopN}</label>
                <input id="md-topn" type="range" min="1" max="20" step="1"
                  bind:value={mdTopN} class="form-slider" />
                <div class="slider-ticks">
                  <span>1</span><span>5</span><span>10</span><span>20</span>
                </div>
              </div>

              <div class="filter-toggles">
                <label class="toggle-label">
                  <input type="checkbox" bind:checked={mdUseRESP} />
                  <span>RESP Charges (HF/6-31G*)</span>
                </label>
              </div>

              <div class="ref-info">
                <span class="ref-chip">Docking: {stages.docking.jobName}</span>
                <span class="ref-chip">Target: {stages.target.jobName}</span>
              </div>

              <button type="submit" class="submit-btn" disabled={mdSubmitting}>
                {mdSubmitting ? 'Submitting...' : 'Run MD Simulation'}
              </button>
            </form>
          {/if}
        {/if}

        {#if stages.md.status === 'running'}
          <div class="running-indicator">
            <span class="pulse-dot"></span>
            <span class="running-text">
              {#if mdJobStatus?.compounds?.length > 0}
                MD: {mdJobStatus.completed ?? 0}/{mdJobStatus.compounds.length} compounds
              {:else}
                MD simulation {stages.md.jobName ?? ''}...
              {/if}
            </span>
          </div>
          {#if mdJobStatus?.compounds?.length > 0}
            <div class="md-compound-list">
              {#each mdJobStatus.compounds as c}
                <div class="md-compound-row">
                  <span class="md-compound-status-dot" class:done={c.status === 'Completed'} class:running={c.status === 'Running'} class:failed={c.status === 'Failed'}></span>
                  <span class="md-compound-id">{c.compound_id}</span>
                  <span class="md-compound-aff">{c.dock_affinity_kcal_mol?.toFixed(1)}</span>
                  <span class="md-compound-status">{c.status}</span>
                  {#if c.duration_s}
                    <span class="md-compound-dur">{c.duration_s}s</span>
                  {/if}
                </div>
              {/each}
            </div>
          {/if}
        {/if}

        {#if stages.md.error}
          <p class="error-msg">{stages.md.error}</p>
        {/if}

        {#if canAdvance('md')}
          <div class="advance-row">
            <span class="done-label">MD complete: {stages.md.jobName}</span>
            <button class="advance-btn" onclick={() => handleAdvance('md')}>Next: ADMET</button>
          </div>
        {/if}
      {/snippet}
    </Panel>
  </div>

  <!-- Stage 5: ADMET -->
  <div id="stage-admet">
    <Panel title={stageLabels.admet} collapsed={stages.admet.collapsed}>
      {#snippet children()}
        <div class="stage-header" role="button" tabindex="0"
          onclick={() => updateStage('admet', { collapsed: !stages.admet.collapsed })}
          onkeydown={(e) => { if (e.key === 'Enter') updateStage('admet', { collapsed: !stages.admet.collapsed }); }}>
          <span class="stage-label">{stageLabels.admet}</span>
          <span class="status-badge {statusBadgeClass(stages.admet.status)}">{stages.admet.status}</span>
        </div>

        {#if (stages.admet.status === 'pending' || stages.admet.status === 'failed')}
          {#if !stages.md.jobName}
            <p class="prereq-msg">Complete MD Simulation first.</p>
          {:else}
            <form class="stage-form" onsubmit={(e) => { e.preventDefault(); handleADMETSubmit(); }}>
              <div class="form-field">
                <label class="form-label" for="ad-mpo">MPO Profile</label>
                <select id="ad-mpo" class="form-select" bind:value={mpoProfile}>
                  <option value="oral">Oral Drug</option>
                  <option value="cns">CNS Penetrant</option>
                  <option value="oncology">Oncology</option>
                  <option value="antimicrobial">Antimicrobial</option>
                </select>
              </div>

              <div class="ref-info">
                <span class="ref-chip">Library: {stages.library.jobName}</span>
              </div>

              <button type="submit" class="submit-btn" disabled={admetSubmitting}>
                {admetSubmitting ? 'Submitting...' : 'Run ADMET'}
              </button>
            </form>
          {/if}
        {/if}

        {#if stages.admet.status === 'running'}
          <div class="running-indicator">
            <span class="pulse-dot"></span>
            <span class="running-text">ADMET prediction {stages.admet.jobName ?? ''}...</span>
          </div>
        {/if}

        {#if stages.admet.error}
          <p class="error-msg">{stages.admet.error}</p>
        {/if}

        {#if stages.admet.status === 'succeeded'}
          <div class="advance-row">
            <span class="done-label">ADMET complete: {stages.admet.jobName}</span>
            <span class="pipeline-done">Pipeline finished. View results in Analysis tab.</span>
          </div>
        {/if}
      {/snippet}
    </Panel>
  </div>
</div>

<style>
  .pipeline-panels {
    display: flex;
    flex-direction: column;
  }

  .pipeline-header {
    display: flex;
    flex-direction: column;
    padding: 8px 0 12px;
  }

  .pipeline-title {
    font-size: 13px;
    font-weight: 700;
    color: var(--text-primary, #e6edf3);
    letter-spacing: 0.3px;
  }

  .pipeline-subtitle {
    font-size: 11px;
    color: var(--text-muted, #484f58);
    margin-top: 2px;
  }

  /* Stepper */
  .stepper {
    display: flex;
    align-items: center;
    padding: 0 8px 12px;
    gap: 0;
  }

  .step {
    display: flex;
    align-items: center;
    flex: 1;
  }

  .step:last-child {
    flex: 0;
  }

  .step-dot {
    width: 24px;
    height: 24px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 11px;
    font-weight: 700;
    flex-shrink: 0;
    transition: all 0.2s;
  }

  .step-dot.pending {
    background: rgba(48, 54, 61, 0.6);
    color: var(--text-muted, #484f58);
    border: 1px solid rgba(48, 54, 61, 0.8);
  }

  .step-dot.running {
    background: rgba(210, 153, 34, 0.2);
    color: #d29922;
    border: 1px solid rgba(210, 153, 34, 0.5);
  }

  .step-dot.completed {
    background: rgba(63, 185, 80, 0.2);
    color: #3fb950;
    border: 1px solid rgba(63, 185, 80, 0.5);
  }

  .step-dot.failed {
    background: rgba(248, 81, 73, 0.2);
    color: #f85149;
    border: 1px solid rgba(248, 81, 73, 0.5);
  }

  .check-mark {
    font-size: 13px;
    line-height: 1;
  }

  .step-line {
    flex: 1;
    height: 2px;
    background: rgba(48, 54, 61, 0.6);
    margin: 0 4px;
    transition: background 0.2s;
  }

  .step-line.done {
    background: rgba(63, 185, 80, 0.5);
  }

  /* Stage header */
  .stage-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
    cursor: pointer;
  }

  .stage-label {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-primary, #e6edf3);
  }

  .status-badge {
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    padding: 1px 6px;
    border-radius: 3px;
    white-space: nowrap;
  }

  .status-badge.pending { background: rgba(88, 166, 255, 0.1); color: var(--accent, #58a6ff); }
  .status-badge.running { background: rgba(210, 153, 34, 0.15); color: #d29922; }
  .status-badge.completed { background: rgba(63, 185, 80, 0.15); color: #3fb950; }
  .status-badge.failed { background: rgba(248, 81, 73, 0.15); color: #f85149; }

  /* Forms */
  .stage-form {
    display: flex;
    flex-direction: column;
    gap: 10px;
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

  .form-input, .form-select, .form-textarea {
    background: rgba(0, 0, 0, 0.3);
    border: 1px solid rgba(48, 54, 61, 0.6);
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

  .form-slider {
    width: 100%;
    accent-color: var(--accent, #58a6ff);
  }

  .slider-ticks {
    display: flex;
    justify-content: space-between;
    font-size: 9px;
    color: var(--text-muted, #484f58);
    padding: 0 2px;
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

  /* Filter toggles */
  .filter-toggles, .engine-checks {
    display: flex;
    gap: 12px;
    flex-wrap: wrap;
  }

  .toggle-label {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 12px;
    color: var(--text-secondary, #8b949e);
    cursor: pointer;
  }

  .toggle-label input[type="checkbox"] {
    accent-color: var(--accent, #58a6ff);
  }

  /* Running indicator */
  .running-indicator {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 0;
  }

  .pulse-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #d29922;
    animation: pulse 1.5s ease-in-out infinite;
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; transform: scale(1); }
    50% { opacity: 0.5; transform: scale(0.8); }
  }

  .running-text {
    font-size: 12px;
    color: #d29922;
  }

  /* Error */
  .error-msg {
    color: var(--danger, #f85149);
    font-size: 12px;
    margin-top: 4px;
  }

  /* Prerequisite message */
  .prereq-msg {
    color: var(--text-muted, #484f58);
    font-size: 12px;
    font-style: italic;
    padding: 8px 0;
  }

  /* Advance row */
  .advance-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 8px 0;
    flex-wrap: wrap;
  }

  .done-label {
    font-size: 11px;
    color: #3fb950;
    font-family: 'SF Mono', monospace;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 160px;
  }

  .advance-btn {
    background: rgba(63, 185, 80, 0.15);
    border: 1px solid rgba(63, 185, 80, 0.4);
    color: #3fb950;
    font-size: 11px;
    font-weight: 600;
    padding: 4px 12px;
    border-radius: 4px;
    cursor: pointer;
    white-space: nowrap;
    transition: all 0.15s;
  }

  .advance-btn:hover {
    background: rgba(63, 185, 80, 0.25);
  }

  .pipeline-done {
    font-size: 11px;
    color: var(--text-secondary, #8b949e);
  }

  /* Reference chips */
  .ref-info {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }

  .ref-chip {
    font-size: 10px;
    font-family: 'SF Mono', monospace;
    color: var(--text-muted, #484f58);
    background: rgba(0, 0, 0, 0.2);
    border: 1px solid rgba(48, 54, 61, 0.4);
    border-radius: 3px;
    padding: 2px 6px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 140px;
  }

  /* Validation */
  .validation-msg {
    color: var(--danger, #f85149);
    font-size: 11px;
    margin: 0;
  }

  /* Custom box inputs */
  .box-grid {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .box-group {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .box-group-label {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-secondary, #8b949e);
    text-transform: uppercase;
    letter-spacing: 0.3px;
  }

  .box-inputs {
    display: flex;
    gap: 6px;
  }

  .box-input {
    flex: 1;
    min-width: 0;
  }

  /* Result info (binding site, pockets) */
  .result-info {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 8px 0;
    border-top: 1px solid rgba(48, 54, 61, 0.4);
  }

  .result-label {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-secondary, #8b949e);
    text-transform: uppercase;
    letter-spacing: 0.3px;
  }

  .result-detail {
    display: flex;
    gap: 12px;
    flex-wrap: wrap;
  }

  .result-field {
    font-size: 11px;
    font-family: 'SF Mono', monospace;
    color: var(--text-primary, #e6edf3);
  }

  .pocket-row {
    display: flex;
    gap: 8px;
    align-items: center;
    font-size: 11px;
    font-family: 'SF Mono', monospace;
    color: var(--text-secondary, #8b949e);
    padding: 2px 4px;
    border-radius: 3px;
  }

  .pocket-row.selected {
    background: rgba(63, 185, 80, 0.1);
    border: 1px solid rgba(63, 185, 80, 0.3);
  }

  .pocket-rank {
    font-weight: 700;
    color: var(--text-primary, #e6edf3);
    min-width: 20px;
  }

  .pocket-score {
    color: var(--accent, #58a6ff);
  }

  .pocket-center {
    color: var(--text-muted, #484f58);
  }

  .pocket-focus-btn {
    display: flex;
    align-items: center;
    gap: 8px;
    background: none;
    border: none;
    padding: 2px 4px;
    cursor: pointer;
    font-family: 'SF Mono', monospace;
    font-size: 11px;
    color: inherit;
    flex: 1;
    min-width: 0;
    text-align: left;
    border-radius: 3px;
    transition: background 0.15s;
  }

  .pocket-focus-btn:hover {
    background: rgba(88, 166, 255, 0.1);
  }

  .pocket-select-btn {
    background: rgba(88, 166, 255, 0.15);
    border: 1px solid rgba(88, 166, 255, 0.3);
    color: var(--accent, #58a6ff);
    font-size: 10px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 3px;
    cursor: pointer;
    white-space: nowrap;
    transition: all 0.15s;
    flex-shrink: 0;
  }

  .pocket-select-btn:hover:not(:disabled) {
    background: rgba(88, 166, 255, 0.25);
  }

  .pocket-select-btn:disabled {
    opacity: 0.5;
    cursor: default;
    background: rgba(63, 185, 80, 0.15);
    border-color: rgba(63, 185, 80, 0.3);
    color: #3fb950;
  }

  .smiles-meta {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-top: 2px;
  }

  .compound-count {
    font-size: 11px;
    color: var(--text-secondary, #8b949e);
    font-family: 'SF Mono', monospace;
    padding: 2px 0;
  }

  .load-example-btn {
    background: rgba(88, 166, 255, 0.1);
    border: 1px solid rgba(88, 166, 255, 0.3);
    color: var(--accent, #58a6ff);
    font-size: 10px;
    font-weight: 600;
    padding: 3px 8px;
    border-radius: 3px;
    cursor: pointer;
    white-space: nowrap;
    transition: all 0.15s;
  }

  .load-example-btn:hover {
    background: rgba(88, 166, 255, 0.2);
  }

  .range-row {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .range-row .form-input {
    flex: 1;
    min-width: 0;
  }

  .range-sep {
    color: var(--text-muted, #484f58);
    font-size: 11px;
    flex-shrink: 0;
  }

  /* Session-expired banner */
  .session-banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    background: rgba(210, 153, 34, 0.12);
    border: 1px solid rgba(210, 153, 34, 0.4);
    border-radius: 6px;
    padding: 8px 12px;
    margin-bottom: 8px;
    flex-wrap: wrap;
  }

  .session-msg {
    font-size: 12px;
    color: #d29922;
  }

  .session-login-btn {
    background: rgba(210, 153, 34, 0.2);
    border: 1px solid rgba(210, 153, 34, 0.5);
    color: #d29922;
    font-size: 11px;
    font-weight: 600;
    padding: 4px 10px;
    border-radius: 4px;
    cursor: pointer;
    white-space: nowrap;
    transition: all 0.15s;
  }

  .session-login-btn:hover {
    background: rgba(210, 153, 34, 0.3);
  }

  /* Pipeline header actions */
  .pipeline-actions {
    display: flex;
    gap: 6px;
    margin-top: 6px;
  }

  .action-btn {
    background: rgba(88, 166, 255, 0.1);
    border: 1px solid rgba(88, 166, 255, 0.3);
    color: var(--accent, #58a6ff);
    font-size: 10px;
    font-weight: 600;
    padding: 3px 8px;
    border-radius: 3px;
    cursor: pointer;
    white-space: nowrap;
    transition: all 0.15s;
  }

  .action-btn:hover { background: rgba(88, 166, 255, 0.2); }

  .action-btn.danger {
    background: rgba(248, 81, 73, 0.1);
    border-color: rgba(248, 81, 73, 0.3);
    color: #f85149;
  }

  .action-btn.danger:hover { background: rgba(248, 81, 73, 0.2); }

  /* Recent runs dropdown */
  .recent-runs {
    display: flex;
    flex-direction: column;
    gap: 3px;
    margin-bottom: 10px;
    background: rgba(0, 0, 0, 0.2);
    border: 1px solid rgba(48, 54, 61, 0.6);
    border-radius: 4px;
    padding: 6px;
    max-height: 200px;
    overflow-y: auto;
  }

  .recent-empty {
    font-size: 11px;
    color: var(--text-muted, #484f58);
    padding: 4px;
  }

  .recent-row {
    display: flex;
    align-items: center;
    gap: 8px;
    background: none;
    border: none;
    padding: 4px 6px;
    border-radius: 3px;
    cursor: pointer;
    text-align: left;
    width: 100%;
    transition: background 0.1s;
  }

  .recent-row:hover { background: rgba(88, 166, 255, 0.08); }

  .recent-name {
    font-size: 11px;
    font-family: 'SF Mono', monospace;
    color: var(--text-primary, #e6edf3);
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .recent-badge {
    font-size: 9px;
    font-weight: 700;
    text-transform: uppercase;
    padding: 1px 5px;
    border-radius: 2px;
    flex-shrink: 0;
  }

  .recent-badge.completed { background: rgba(63, 185, 80, 0.15); color: #3fb950; }
  .recent-badge.failed    { background: rgba(248, 81, 73, 0.15); color: #f85149; }
  .recent-badge.running   { background: rgba(210, 153, 34, 0.15); color: #d29922; }
  .recent-badge.pending   { background: rgba(88, 166, 255, 0.1); color: var(--accent, #58a6ff); }

  .recent-meta {
    font-size: 10px;
    color: var(--text-muted, #484f58);
    flex-shrink: 0;
    white-space: nowrap;
  }

  /* Docking summary */
  .dock-summary {
    padding: 8px 0;
    border-top: 1px solid rgba(48, 54, 61, 0.4);
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .dock-summary-stats {
    display: flex;
    gap: 16px;
  }

  .dock-stat {
    font-size: 12px;
    color: var(--text-secondary, #8b949e);
  }

  .dock-stat strong {
    color: var(--text-primary, #e6edf3);
  }

  .cutoff-table {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .cutoff-row {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 11px;
    font-family: 'SF Mono', monospace;
  }

  .cutoff-val {
    color: var(--text-muted, #484f58);
    width: 42px;
    flex-shrink: 0;
    text-align: right;
  }

  .cutoff-bar-wrap {
    flex: 1;
    height: 6px;
    background: rgba(48, 54, 61, 0.4);
    border-radius: 3px;
    overflow: hidden;
  }

  .cutoff-bar {
    display: block;
    height: 100%;
    background: var(--accent, #58a6ff);
    border-radius: 3px;
    opacity: 0.6;
  }

  .cutoff-count {
    color: var(--text-secondary, #8b949e);
    width: 46px;
    text-align: right;
    flex-shrink: 0;
  }

  .top-hits-label {
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.3px;
    color: var(--text-muted, #484f58);
    margin-top: 2px;
  }

  .top-hits-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    max-height: 140px;
    overflow-y: auto;
  }

  .hit-row {
    display: flex;
    justify-content: space-between;
    font-size: 11px;
    font-family: 'SF Mono', monospace;
    padding: 1px 4px;
    border-radius: 2px;
  }

  .hit-row:hover { background: rgba(88, 166, 255, 0.06); }

  .hit-id {
    color: var(--text-secondary, #8b949e);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .hit-aff {
    color: #3fb950;
    flex-shrink: 0;
    margin-left: 8px;
  }

  /* MD per-compound progress */
  .md-compound-list {
    display: flex;
    flex-direction: column;
    gap: 3px;
    margin-top: 4px;
    max-height: 200px;
    overflow-y: auto;
  }

  .md-compound-row {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 11px;
    font-family: 'SF Mono', monospace;
    padding: 2px 4px;
    border-radius: 2px;
  }

  .md-compound-status-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex-shrink: 0;
    background: rgba(48, 54, 61, 0.8);
  }

  .md-compound-status-dot.done    { background: #3fb950; }
  .md-compound-status-dot.running { background: #d29922; animation: pulse 1.5s ease-in-out infinite; }
  .md-compound-status-dot.failed  { background: #f85149; }

  .md-compound-id {
    flex: 1;
    color: var(--text-secondary, #8b949e);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .md-compound-aff {
    color: #3fb950;
    flex-shrink: 0;
  }

  .md-compound-status {
    color: var(--text-muted, #484f58);
    flex-shrink: 0;
    width: 60px;
    text-align: right;
  }

  .md-compound-dur {
    color: var(--text-muted, #484f58);
    flex-shrink: 0;
  }

  /* Eligible count inline label */
  .eligible-count {
    color: var(--accent, #58a6ff);
    font-size: 10px;
    font-weight: 400;
    text-transform: none;
    margin-left: 4px;
  }
</style>
