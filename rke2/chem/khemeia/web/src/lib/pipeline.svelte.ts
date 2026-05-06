import {
  submitTargetPrep, getTargetPrep, getTargetReceptor, selectPocket,
  submitLibraryPrep, getLibraryPrep, getLibraryCompounds,
  submitDocking, getDockingV2Job, listDockingJobs, getDockingV2Summary, getDockingV2Results, getDockingPose,
  submitADMET, getADMETJob, getADMETResults,
  submitMD, getMDJob, listMDJobs, getMDResults, getMDTrajectory, getMDEnergy,
  AuthError,
} from '$lib/api';
import { loadFile, overlayStructure, clearOverlays, showPocketMarkers, showBindingBox, focusPocketCenter } from '$lib/viewer';

export type StageStatus = 'pending' | 'running' | 'succeeded' | 'failed';

export interface StageState {
  status: StageStatus;
  jobName: string | null;
  error: string;
  collapsed: boolean;
}

const PIPELINE_STORAGE_KEY = 'khemeia_pipeline_v1';
const WORKGROUPS_KEY = 'khemeia_workgroups_v1';
const ACTIVE_WG_KEY = 'khemeia_active_wg_v1';

export interface WorkgroupMeta {
  id: string;
  name: string;
  createdAt: number;
  stages: Record<string, { jobName: string | null; status: StageStatus }>;
  pdbId: string;
}

function defaultStages(): Record<string, StageState> {
  return {
    target:  { status: 'pending', jobName: null, error: '', collapsed: false },
    library: { status: 'pending', jobName: null, error: '', collapsed: true },
    docking: { status: 'pending', jobName: null, error: '', collapsed: true },
    md:      { status: 'pending', jobName: null, error: '', collapsed: true },
    admet:   { status: 'pending', jobName: null, error: '', collapsed: true },
  };
}

function readSavedStages(): Record<string, StageState> {
  const defaults = defaultStages();
  try {
    const raw = localStorage.getItem(PIPELINE_STORAGE_KEY);
    if (!raw) return defaults;
    const saved = JSON.parse(raw) as Record<string, { jobName: string | null; status: StageStatus }>;
    for (const [k, v] of Object.entries(saved)) {
      if (defaults[k] && v.jobName) {
        defaults[k] = { ...defaults[k], jobName: v.jobName, status: v.status,
          collapsed: v.status === 'pending' && k !== 'target' };
      }
    }
  } catch {}
  return defaults;
}

function saveStages(stages: Record<string, StageState>) {
  const snapshot: Record<string, { jobName: string | null; status: StageStatus }> = {};
  for (const [k, s] of Object.entries(stages)) {
    snapshot[k] = { jobName: s.jobName, status: s.status };
  }
  try { localStorage.setItem(PIPELINE_STORAGE_KEY, JSON.stringify(snapshot)); } catch {}
}

function stageSnapshot(stages: Record<string, StageState>): Record<string, { jobName: string | null; status: StageStatus }> {
  const snap: Record<string, { jobName: string | null; status: StageStatus }> = {};
  for (const [k, s] of Object.entries(stages)) snap[k] = { jobName: s.jobName, status: s.status };
  return snap;
}

function _initWorkgroupData(): { workgroups: WorkgroupMeta[]; activeId: string } {
  try {
    const wgsRaw = localStorage.getItem(WORKGROUPS_KEY);
    let wgs: WorkgroupMeta[] = wgsRaw ? JSON.parse(wgsRaw) : [];

    if (wgs.length === 0) {
      const stagesRaw = localStorage.getItem(PIPELINE_STORAGE_KEY);
      const legacyStages: Record<string, { jobName: string | null; status: StageStatus }> = stagesRaw ? JSON.parse(stagesRaw) : {};
      const wg: WorkgroupMeta = { id: crypto.randomUUID(), name: 'Workgroup 1', createdAt: Date.now(), stages: legacyStages, pdbId: '' };
      wgs = [wg];
      localStorage.setItem(WORKGROUPS_KEY, JSON.stringify(wgs));
      localStorage.setItem(ACTIVE_WG_KEY, wg.id);
      return { workgroups: wgs, activeId: wg.id };
    }

    const savedActive = localStorage.getItem(ACTIVE_WG_KEY);
    const activeId = savedActive && wgs.some(w => w.id === savedActive) ? savedActive : wgs[0].id;
    return { workgroups: wgs, activeId };
  } catch {
    const wg: WorkgroupMeta = { id: crypto.randomUUID(), name: 'Workgroup 1', createdAt: Date.now(), stages: {}, pdbId: '' };
    return { workgroups: [wg], activeId: wg.id };
  }
}

const _wgInit = _initWorkgroupData();

class PipelineStore {
  // Core stage state
  stages = $state<Record<string, StageState>>(readSavedStages());
  sessionExpired = $state(false);

  // Workgroups
  workgroups = $state<WorkgroupMeta[]>(_wgInit.workgroups);
  activeWorkgroupId = $state<string>(_wgInit.activeId);
  wgRenaming = $state(false);
  wgRenameValue = $state('');

  // Target Prep form
  pdbId = $state('');
  bindingSiteMode = $state<'native-ligand' | 'custom-box' | 'pocket-detection'>('native-ligand');
  nativeLigandId = $state('');
  targetSubmitting = $state(false);
  boxCenterX = $state(0); boxCenterY = $state(0); boxCenterZ = $state(0);
  boxSizeX = $state(20); boxSizeY = $state(20); boxSizeZ = $state(20);
  pockets = $state<any[] | null>(null);
  selectedPocketIdx = $state<number | null>(null);
  targetPrepResult = $state<any | null>(null);
  receptorLoading = $state(false);

  // Library Prep form
  libSource = $state<'smiles' | 'chembl'>('smiles');
  smilesText = $state('');
  chemblTarget = $state('');
  chemblMaxPhase = $state(0);
  chemblMwMin = $state(''); chemblMwMax = $state('');
  chemblLogpMin = $state(''); chemblLogpMax = $state('');
  chemblHbaMax = $state(''); chemblHbdMax = $state('');
  filterLipinski = $state(true); filterVeber = $state(true); filterPAINS = $state(true);
  libSubmitting = $state(false);
  libAttachName = $state('');
  libAttaching = $state(false);
  libraryStatus = $state<any | null>(null);
  libraryCompoundSample = $state<any[]>([]);

  // Docking form
  engVina = $state(false); engGnina = $state(false); engVinaGpu = $state(true);
  exhaustiveness = $state(8);
  dockSubmitting = $state(false);
  dockingSummary = $state<any | null>(null);
  dockResults = $state<any[]>([]);
  dockResultsPage = $state(1);
  dockResultsTotal = $state(0);
  dockResultsLoading = $state(false);
  loadingPoseId = $state<string | null>(null);

  // MD form
  mdForceField = $state<'amber99sb-ildn' | 'amber14sb' | 'charmm36m'>('amber99sb-ildn');
  mdLigandFF = $state<'gaff2' | 'gaff'>('gaff2');
  mdNSteps = $state(500000);
  mdTopN = $state(10);
  mdAffinityCutoff = $state(-7.0);
  mdUseRESP = $state(false);
  mdSubmitting = $state(false);
  mdJobStatus = $state<any | null>(null);
  mdResults = $state<any[]>([]);
  mdViewerLoading = $state<string | null>(null);
  mdViewerError = $state<string | null>(null);

  // ADMET form
  mpoProfile = $state<'oral' | 'cns' | 'oncology' | 'antimicrobial'>('oral');
  admetSubmitting = $state(false);
  admetResults = $state<any[]>([]);
  admetResultsTotal = $state(0);
  admetResultsLoading = $state(false);

  // Recent runs
  recentJobs = $state<any[]>([]);
  recentOpen = $state(false);

  // Internal poll bookkeeping (not reactive)
  pollTimers: Record<string, ReturnType<typeof setTimeout>> = {};
  pollFailures: Record<string, number> = {};
  initialized = false;

  readonly DOCK_PER_PAGE = 25;
  readonly EXAMPLE_SMILES = [
    'CC(=O)Oc1ccccc1C(=O)O',
    'CC(C)Cc1ccc(cc1)C(C)C(=O)O',
    'OC(=O)c1ccccc1O',
    'CC(=O)Nc1ccc(O)cc1',
    'CN1C=NC2=C1C(=O)N(C(=O)N2C)C',
    'c1ccc2c(c1)cc1ccc3cccc4ccc2c1c34',
    'CC12CCC3C(C1CCC2O)CCC4=CC(=O)CCC34C',
    'OC[C@H]1OC(O)[C@H](O)[C@@H](O)[C@@H]1O',
    'CC(C)NCC(O)c1ccc(O)c(CO)c1',
    'Clc1ccc(cc1)C(c1ccc(Cl)cc1)C(Cl)(Cl)Cl',
  ].join('\n');

  get smilesCount() {
    return this.smilesText.split('\n').map(s => s.trim()).filter(Boolean).length;
  }

  get mdEligibleCount() {
    if (!this.dockingSummary?.cutoff_counts) return null;
    const key = this.mdAffinityCutoff.toFixed(1);
    return this.dockingSummary.cutoff_counts[key] ?? null;
  }

  // --- Stage helpers ---

  updateStage(key: string, patch: Partial<StageState>) {
    this.stages[key] = { ...this.stages[key], ...patch };
    saveStages(this.stages);
    this._syncActiveWorkgroup();
  }

  // --- Workgroup management ---

  get activeWorkgroup(): WorkgroupMeta | undefined {
    return this.workgroups.find(w => w.id === this.activeWorkgroupId);
  }

  _syncActiveWorkgroup() {
    const snap = stageSnapshot(this.stages);
    this.workgroups = this.workgroups.map(w =>
      w.id === this.activeWorkgroupId ? { ...w, stages: snap, pdbId: this.pdbId } : w
    );
    try { localStorage.setItem(WORKGROUPS_KEY, JSON.stringify(this.workgroups)); } catch {}
  }

  newWorkgroup(name?: string) {
    this._syncActiveWorkgroup();
    const n = this.workgroups.length + 1;
    const wg: WorkgroupMeta = { id: crypto.randomUUID(), name: name ?? `Workgroup ${n}`, createdAt: Date.now(), stages: {}, pdbId: '' };
    this.workgroups = [...this.workgroups, wg];
    this.activeWorkgroupId = wg.id;
    try { localStorage.setItem(ACTIVE_WG_KEY, wg.id); localStorage.setItem(WORKGROUPS_KEY, JSON.stringify(this.workgroups)); } catch {}
    this._applyWorkgroupStages({});
    this.clearPolls();
    this._resetLoadedState();
    this.initialized = false;
  }

  switchWorkgroup(id: string) {
    if (id === this.activeWorkgroupId) return;
    this._syncActiveWorkgroup();
    const wg = this.workgroups.find(w => w.id === id);
    if (!wg) return;
    this.clearPolls();
    this._resetLoadedState();
    this.initialized = false;
    this.activeWorkgroupId = id;
    try { localStorage.setItem(ACTIVE_WG_KEY, id); } catch {}
    this._applyWorkgroupStages(wg.stages);
    if (wg.pdbId) this.pdbId = wg.pdbId;
    this.init();
  }

  renameWorkgroup(id: string, name: string) {
    this.workgroups = this.workgroups.map(w => w.id === id ? { ...w, name } : w);
    try { localStorage.setItem(WORKGROUPS_KEY, JSON.stringify(this.workgroups)); } catch {}
  }

  deleteWorkgroup(id: string) {
    const filtered = this.workgroups.filter(w => w.id !== id);
    if (filtered.length === 0) { this.newWorkgroup('Workgroup 1'); return; }
    this.workgroups = filtered;
    try { localStorage.setItem(WORKGROUPS_KEY, JSON.stringify(this.workgroups)); } catch {}
    if (this.activeWorkgroupId === id) this.switchWorkgroup(filtered[0].id);
  }

  startRename() {
    this.wgRenameValue = this.activeWorkgroup?.name ?? '';
    this.wgRenaming = true;
  }

  commitRename() {
    const name = this.wgRenameValue.trim();
    if (name) this.renameWorkgroup(this.activeWorkgroupId, name);
    this.wgRenaming = false;
  }

  cancelRename() { this.wgRenaming = false; }

  _applyWorkgroupStages(snap: Record<string, { jobName: string | null; status: StageStatus }>) {
    const fresh = defaultStages();
    for (const [k, v] of Object.entries(snap)) {
      if (fresh[k] && v.jobName) {
        fresh[k] = { ...fresh[k], jobName: v.jobName, status: v.status, collapsed: v.status === 'pending' && k !== 'target' };
      }
    }
    this.stages = fresh;
    saveStages(this.stages);
  }

  _resetLoadedState() {
    this.targetPrepResult = null;
    this.pockets = null;
    this.selectedPocketIdx = null;
    this.receptorLoading = false;
    this.libraryStatus = null;
    this.libraryCompoundSample = [];
    this.dockingSummary = null;
    this.dockResults = [];
    this.dockResultsTotal = 0;
    this.dockResultsPage = 1;
    this.mdJobStatus = null;
    this.mdResults = [];
    this.mdViewerLoading = null;
    this.mdViewerError = null;
    this.admetResults = [];
    this.admetResultsTotal = 0;
    this.recentJobs = [];
    this.recentOpen = false;
    this.sessionExpired = false;
  }

  canAdvance(key: string): boolean {
    return this.stages[key].status === 'succeeded';
  }

  handleAdvance(key: string) {
    const order = ['target', 'library', 'docking', 'md', 'admet'];
    const idx = order.indexOf(key);
    const next = idx >= 0 && idx < order.length - 1 ? order[idx + 1] : null;
    if (!next) return;
    this.updateStage(next, { collapsed: false });
    requestAnimationFrame(() => {
      document.getElementById(`stage-${next}`)?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    });
  }

  statusBadgeClass(s: StageStatus): string {
    if (s === 'succeeded') return 'completed';
    if (s === 'failed') return 'failed';
    if (s === 'running') return 'running';
    return 'pending';
  }

  // --- Polling ---

  startPoll(stageKey: string, pollFn: (name: string) => Promise<any>) {
    const name = this.stages[stageKey].jobName;
    if (!name) return;
    if (this.pollTimers[stageKey]) clearTimeout(this.pollTimers[stageKey]);
    const POLL_MAX_FAILURES = 3;

    const tick = async () => {
      if (this.sessionExpired) return;
      try {
        const res = await pollFn(name);
        this.pollFailures[stageKey] = 0;
        const phase = (res.phase || res.status || '').toLowerCase();
        if (stageKey === 'library') this.libraryStatus = res;
        if (stageKey === 'md') this.mdJobStatus = res;

        if (phase === 'completed' || phase === 'succeeded') {
          this.updateStage(stageKey, { status: 'succeeded', error: '' });
          if (stageKey === 'target') {
            this.targetPrepResult = res;
            this.loadTargetInViewer(this.stages.target.jobName!, res);
          }
          if (stageKey === 'library') this.loadLibrarySample(this.stages.library.jobName!);
          if (stageKey === 'docking' && this.stages.docking.jobName) {
            this.loadDockingSummary(this.stages.docking.jobName);
            this.loadDockResults(this.stages.docking.jobName, 1);
          }
          if (stageKey === 'md' && this.stages.md.jobName) this.loadMDResults(this.stages.md.jobName);
          if (stageKey === 'admet' && this.stages.admet.jobName) this.loadAdmetResults(this.stages.admet.jobName);
          return;
        }
        if (phase === 'failed') {
          this.updateStage(stageKey, { status: 'failed', error: res.error || res.error_output || 'Job failed' });
          return;
        }
        this.pollTimers[stageKey] = setTimeout(tick, 10_000);
      } catch (e: any) {
        if (e instanceof AuthError) { this.sessionExpired = true; return; }
        this.pollFailures[stageKey] = (this.pollFailures[stageKey] ?? 0) + 1;
        if (this.pollFailures[stageKey] < POLL_MAX_FAILURES) {
          this.pollTimers[stageKey] = setTimeout(tick, 15_000);
          return;
        }
        this.updateStage(stageKey, { status: 'failed', error: e.message || 'Poll failed' });
      }
    };
    tick();
  }

  clearPolls() {
    for (const t of Object.values(this.pollTimers)) clearTimeout(t);
    this.pollTimers = {};
  }

  // --- Data loaders ---

  async loadDockingSummary(name: string) {
    try { this.dockingSummary = await getDockingV2Summary(name); } catch {}
  }

  async loadDockResults(name: string, page: number = 1) {
    this.dockResultsLoading = true;
    try {
      const res = await getDockingV2Results(name, page, this.DOCK_PER_PAGE);
      this.dockResults = res.results ?? [];
      this.dockResultsTotal = res.total_results ?? 0;
      this.dockResultsPage = page;
    } catch {} finally { this.dockResultsLoading = false; }
  }

  async loadPoseInViewer(compoundId: string) {
    if (!this.stages.docking.jobName) return;
    this.loadingPoseId = compoundId;
    try {
      await clearOverlays();
      const pdbqt = await getDockingPose(this.stages.docking.jobName, compoundId);
      await overlayStructure(pdbqt, 'pdbqt');
    } catch (e) {
      console.error('Pose load failed:', e);
    } finally { this.loadingPoseId = null; }
  }

  async loadAdmetResults(name: string) {
    this.admetResultsLoading = true;
    try {
      const res = await getADMETResults(name, 1, 100);
      this.admetResults = res.results ?? [];
      this.admetResultsTotal = res.total ?? this.admetResults.length;
    } catch {} finally { this.admetResultsLoading = false; }
  }

  async loadLibrarySample(name: string) {
    try {
      const res = await getLibraryCompounds(name, 1, 8);
      this.libraryCompoundSample = res.compounds ?? [];
    } catch {}
  }

  async loadMDResults(name: string) {
    try {
      const res = await getMDResults(name);
      this.mdResults = res.results ?? [];
    } catch {}
  }

  async viewMDCompound(
    compound: any,
    onMDView: ((frames: string[], energy: any, compoundId: string) => void) | undefined
  ) {
    if (!this.stages.md.jobName || !onMDView) return;
    this.mdViewerLoading = compound.compound_id;
    this.mdViewerError = null;
    try {
      const [trajResult, energyResult] = await Promise.allSettled([
        compound.has_trajectory
          ? getMDTrajectory(this.stages.md.jobName, compound.compound_id)
          : Promise.reject('no frames'),
        compound.has_energy
          ? getMDEnergy(this.stages.md.jobName, compound.compound_id)
          : Promise.reject('no energy'),
      ]);
      const frames: string[] = [];
      if (trajResult.status === 'fulfilled' && trajResult.value) {
        const raw = trajResult.value as string;
        if (/^MODEL\s+\d+/m.test(raw)) {
          for (const block of raw.split(/ENDMDL\s*/)) {
            const t = block.trim();
            if (t) frames.push(t + '\nENDMDL\n');
          }
        } else {
          frames.push(raw);
        }
      } else if (trajResult.status === 'rejected' && compound.has_trajectory) {
        this.mdViewerError = `Failed to load trajectory: ${trajResult.reason}`;
      }
      const energy = energyResult.status === 'fulfilled' ? energyResult.value as any : null;
      if (frames.length === 0 && energy === null) {
        if (!this.mdViewerError) this.mdViewerError = 'No trajectory or energy data available yet.';
        return;
      }
      onMDView(frames, energy, compound.compound_id);
    } catch (err: any) {
      this.mdViewerError = err?.message ?? 'Failed to load trajectory.';
    } finally { this.mdViewerLoading = null; }
  }

  async loadTargetInViewer(name: string, prepResult: any) {
    if (this.receptorLoading) return;
    this.receptorLoading = true;
    try {
      const pdb = await getTargetReceptor(name);
      await loadFile(pdb, 'pdb');
      if (prepResult?.pockets?.length) {
        await showPocketMarkers(prepResult.pockets.map((p: any) => ({
          center: p.center, score: p.consensus_score ?? 0.5, rank: p.rank ?? 1,
        })));
      } else if (prepResult?.binding_site) {
        const bs = prepResult.binding_site;
        await showBindingBox(bs.center, bs.size);
      }
    } catch (e) {
      console.error('Target load failed:', e);
    } finally { this.receptorLoading = false; }
  }

  // --- Recent runs ---

  async loadRecentJobs() {
    this.recentOpen = !this.recentOpen;
    if (this.recentOpen && this.recentJobs.length === 0) {
      try {
        const res = await listDockingJobs();
        this.recentJobs = res.jobs ?? [];
      } catch {}
    }
  }

  async restorePipeline(job: any) {
    this.recentOpen = false;
    this.updateStage('target',  { jobName: job.receptor_ref, status: 'running', error: '', collapsed: false });
    this.updateStage('library', { jobName: job.library_ref,  status: 'running', error: '', collapsed: false });
    this.updateStage('docking', { jobName: job.name,         status: 'running', error: '', collapsed: false });

    const toStatus = (phase: string): StageStatus => {
      const p = (phase || '').toLowerCase();
      if (p === 'succeeded' || p === 'completed') return 'succeeded';
      if (p === 'failed') return 'failed';
      if (p === 'running') return 'running';
      return 'pending';
    };

    try {
      const [tRes, lRes, dRes] = await Promise.all([
        getTargetPrep(job.receptor_ref),
        getLibraryPrep(job.library_ref),
        getDockingV2Job(job.name),
      ]);
      const tStatus = toStatus(tRes.phase || tRes.status);
      this.updateStage('target',  { status: tStatus });
      this.updateStage('library', { status: toStatus(lRes.phase || lRes.status) });
      const dockStatus = toStatus(dRes.phase || dRes.status);
      this.updateStage('docking', { status: dockStatus });

      if (tStatus === 'succeeded') {
        this.targetPrepResult = tRes;
        this.loadTargetInViewer(job.receptor_ref, tRes);
      } else if (tStatus === 'running') this.startPoll('target', getTargetPrep);

      if (this.stages.library.status === 'succeeded') this.loadLibrarySample(job.library_ref);
      else if (this.stages.library.status === 'running') this.startPoll('library', getLibraryPrep);

      if (dockStatus === 'succeeded') {
        this.loadDockingSummary(job.name);
        this.loadDockResults(job.name, 1);
      } else if (dockStatus === 'running') this.startPoll('docking', getDockingV2Job);
    } catch {}

    try {
      const mdList = await listMDJobs(job.name);
      const latestMD = mdList.jobs?.[0];
      if (latestMD) {
        const mdStatus = toStatus(latestMD.status);
        this.updateStage('md', { jobName: latestMD.name, status: mdStatus, error: '', collapsed: false });
        if (mdStatus === 'running') this.startPoll('md', getMDJob);
        if (mdStatus === 'running' || mdStatus === 'succeeded') this.loadMDResults(latestMD.name);
      }
    } catch {}
  }

  clearPipeline() {
    this.clearPolls();
    this._resetLoadedState();
    this.stages = defaultStages();
    try { localStorage.removeItem(PIPELINE_STORAGE_KEY); } catch {}
    this._syncActiveWorkgroup();
  }

  // --- Submit handlers ---

  targetFormValid(): boolean {
    if (!this.pdbId) return false;
    if (this.bindingSiteMode === 'native-ligand' && !this.nativeLigandId.trim()) return false;
    return true;
  }

  async handleTargetSubmit() {
    if (!this.targetFormValid()) {
      this.updateStage('target', { error: 'Please fill in all required fields' });
      return;
    }
    this.targetSubmitting = true;
    this.updateStage('target', { error: '', status: 'running' });
    try {
      const params: any = { pdb_id: this.pdbId, binding_site_mode: this.bindingSiteMode };
      if (this.bindingSiteMode === 'native-ligand') params.native_ligand_id = this.nativeLigandId;
      if (this.bindingSiteMode === 'custom-box') {
        params.custom_box = {
          center: [this.boxCenterX, this.boxCenterY, this.boxCenterZ],
          size: [this.boxSizeX, this.boxSizeY, this.boxSizeZ],
        };
      }
      const res = await submitTargetPrep(params);
      this.updateStage('target', { jobName: res.name || res.job_name, status: 'running' });
      this.startPoll('target', getTargetPrep);
    } catch (e: any) {
      this.updateStage('target', { status: 'failed', error: e.message || 'Submission failed' });
    } finally { this.targetSubmitting = false; }
  }

  libFormValid(): boolean {
    if (this.libSource === 'smiles') return this.smilesCount > 0;
    if (this.libSource === 'chembl') {
      return this.chemblTarget.trim() !== '' || !!this.chemblMwMin || !!this.chemblMwMax ||
        !!this.chemblLogpMin || !!this.chemblLogpMax || !!this.chemblHbaMax || !!this.chemblHbdMax;
    }
    return true;
  }

  async handleLibraryAttach() {
    const name = this.libAttachName.trim();
    if (!name) return;
    this.libAttaching = true;
    try {
      const res = await getLibraryPrep(name);
      const phase = (res.phase || res.status || '').toLowerCase();
      const status: StageStatus =
        phase === 'completed' || phase === 'succeeded' ? 'succeeded' :
        phase === 'failed' ? 'failed' : 'running';
      this.updateStage('library', { jobName: name, status, error: '' });
      this.libraryStatus = res;
      if (status === 'running') this.startPoll('library', getLibraryPrep);
      if (status === 'succeeded') this.loadLibrarySample(name);
    } catch (e: any) {
      this.updateStage('library', { error: e.message || 'Job not found' });
    } finally { this.libAttaching = false; }
  }

  async handleLibrarySubmit() {
    this.libSubmitting = true;
    this.updateStage('library', { error: '', status: 'running' });
    try {
      const params: any = {
        source: this.libSource,
        filters: { lipinski: this.filterLipinski, veber: this.filterVeber, pains: this.filterPAINS },
        name: 'pipeline-lib-' + Date.now(),
      };
      if (this.libSource === 'smiles') {
        params.smiles_list = this.smilesText.split('\n').map((s: string) => s.trim()).filter(Boolean);
      } else {
        const chembl: Record<string, any> = {};
        if (this.chemblTarget.trim()) chembl.q = this.chemblTarget.trim();
        if (this.chemblMaxPhase > 0) chembl.max_phase = this.chemblMaxPhase;
        if (this.chemblMwMin) chembl.mw_min = parseFloat(this.chemblMwMin);
        if (this.chemblMwMax) chembl.mw_max = parseFloat(this.chemblMwMax);
        if (this.chemblLogpMin) chembl.logp_min = parseFloat(this.chemblLogpMin);
        if (this.chemblLogpMax) chembl.logp_max = parseFloat(this.chemblLogpMax);
        if (this.chemblHbaMax) chembl.hba_max = parseInt(this.chemblHbaMax);
        if (this.chemblHbdMax) chembl.hbd_max = parseInt(this.chemblHbdMax);
        params.chembl = chembl;
      }
      const res = await submitLibraryPrep(params);
      this.updateStage('library', { jobName: res.name || res.job_name, status: 'running' });
      this.startPoll('library', getLibraryPrep);
    } catch (e: any) {
      this.updateStage('library', { status: 'failed', error: e.message || 'Submission failed' });
    } finally { this.libSubmitting = false; }
  }

  async handleDockingSubmit() {
    this.dockSubmitting = true;
    this.updateStage('docking', { error: '', status: 'running' });
    try {
      const engines: string[] = [];
      if (this.engVina) engines.push('vina-1.2');
      if (this.engGnina) engines.push('gnina');
      if (this.engVinaGpu) engines.push('vina-gpu');
      const res = await submitDocking({
        receptor_ref: this.stages.target.jobName,
        library_ref: this.stages.library.jobName,
        engines,
        exhaustiveness: this.exhaustiveness,
      });
      this.updateStage('docking', { jobName: res.name || res.job_name, status: 'running' });
      this.startPoll('docking', getDockingV2Job);
    } catch (e: any) {
      this.updateStage('docking', { status: 'failed', error: e.message || 'Submission failed' });
    } finally { this.dockSubmitting = false; }
  }

  async handleMDSubmit() {
    this.mdSubmitting = true;
    this.updateStage('md', { error: '', status: 'running' });
    try {
      const res = await submitMD({
        dock_job_name: this.stages.docking.jobName,
        receptor_ref: this.stages.target.jobName,
        top_n: this.mdTopN,
        affinity_cutoff: this.mdAffinityCutoff,
        md_nsteps: this.mdNSteps,
        force_field: this.mdForceField,
        ligand_ff: this.mdLigandFF,
        use_resp: this.mdUseRESP,
      });
      this.updateStage('md', { jobName: res.name || res.job_name, status: 'running' });
      this.startPoll('md', getMDJob);
    } catch (e: any) {
      this.updateStage('md', { status: 'failed', error: e.message || 'Submission failed' });
    } finally { this.mdSubmitting = false; }
  }

  async handleADMETSubmit() {
    this.admetSubmitting = true;
    this.updateStage('admet', { error: '', status: 'running' });
    try {
      const res = await submitADMET({
        library_ref: this.stages.library.jobName,
        mpo_profile: this.mpoProfile,
      });
      this.updateStage('admet', { jobName: res.name || res.job_name, status: 'running' });
      this.startPoll('admet', getADMETJob);
    } catch (e: any) {
      this.updateStage('admet', { status: 'failed', error: e.message || 'Submission failed' });
    } finally { this.admetSubmitting = false; }
  }

  // Called once from onMount to reattach polls and reload results for persisted state
  async init() {
    if (this.initialized) return;
    this.initialized = true;

    const POLL_FNS: Record<string, (name: string) => Promise<any>> = {
      target: getTargetPrep, library: getLibraryPrep, docking: getDockingV2Job,
      md: getMDJob, admet: getADMETJob,
    };

    for (const [k, s] of Object.entries(this.stages)) {
      if (s.status === 'running' && s.jobName && POLL_FNS[k]) this.startPoll(k, POLL_FNS[k]);
    }
    if (this.stages.target.status === 'succeeded' && this.stages.target.jobName) {
      try {
        const res = await getTargetPrep(this.stages.target.jobName);
        this.targetPrepResult = res;
        this.loadTargetInViewer(this.stages.target.jobName, res);
      } catch {}
    }
    if (this.stages.library.status === 'succeeded' && this.stages.library.jobName)
      this.loadLibrarySample(this.stages.library.jobName);
    if (this.stages.docking.status === 'succeeded' && this.stages.docking.jobName) {
      this.loadDockingSummary(this.stages.docking.jobName);
      this.loadDockResults(this.stages.docking.jobName, 1);
    }
    if (this.stages.md.status === 'succeeded' && this.stages.md.jobName)
      this.loadMDResults(this.stages.md.jobName);
    if (this.stages.admet.status === 'succeeded' && this.stages.admet.jobName)
      this.loadAdmetResults(this.stages.admet.jobName);
  }
}

export const pipeline = new PipelineStore();
