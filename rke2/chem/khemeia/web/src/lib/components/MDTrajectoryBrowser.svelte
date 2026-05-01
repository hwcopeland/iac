<script lang="ts">
  import { listMDJobs, getMDResults, getMDTrajectory, getMDEnergy } from '$lib/api';
  import Panel from './Panel.svelte';

  let { onView }: {
    onView: (
      frames: string[],
      energy: { time: number[]; potential: number[]; temperature: number[] } | null,
      compoundId: string
    ) => void;
  } = $props();

  interface MDJobRow {
    name: string;
    status: string;
    dock_job_name: string;
    top_n: number;
    created_at: string;
  }

  interface MDResultRow {
    compound_id: string;
    dock_affinity_kcal_mol: number;
    duration_s: number;
    has_trajectory: boolean;
    has_energy: boolean;
  }

  let jobs = $state<MDJobRow[]>([]);
  let loadingJobs = $state(false);
  let selectedJob = $state<MDJobRow | null>(null);
  let results = $state<MDResultRow[]>([]);
  let loadingResults = $state(false);
  let viewerLoading = $state<string | null>(null);
  let open = $state(false);

  async function loadJobs() {
    loadingJobs = true;
    try {
      const res = await listMDJobs();
      jobs = res.jobs ?? [];
    } catch {} finally {
      loadingJobs = false;
    }
  }

  async function selectJob(job: MDJobRow) {
    selectedJob = job;
    results = [];
    loadingResults = true;
    try {
      const res = await getMDResults(job.name);
      results = res.results ?? [];
    } catch {} finally {
      loadingResults = false;
    }
  }

  async function viewCompound(r: MDResultRow) {
    if (!selectedJob) return;
    viewerLoading = r.compound_id;
    try {
      const [trajResult, energyResult] = await Promise.allSettled([
        r.has_trajectory
          ? getMDTrajectory(selectedJob.name, r.compound_id)
          : Promise.reject('no frames'),
        r.has_energy
          ? getMDEnergy(selectedJob.name, r.compound_id)
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
      }

      const energy = energyResult.status === 'fulfilled' ? energyResult.value as any : null;
      onView(frames, energy, r.compound_id);
    } finally {
      viewerLoading = null;
    }
  }

  function statusColor(s: string) {
    const p = s.toLowerCase();
    if (p === 'completed' || p === 'succeeded') return 'done';
    if (p === 'failed') return 'failed';
    if (p === 'running') return 'running';
    return 'pending';
  }
</script>

<Panel title="MD Trajectories">
  {#snippet children()}
    <div class="traj-browser">
      {#if !open}
        <button class="browse-btn" onclick={() => { open = true; loadJobs(); }}>
          Browse MD Jobs
        </button>
      {:else}
        <div class="browser-body">
          <!-- Job list -->
          <div class="section-label">
            MD Jobs
            <button class="refresh-btn" onclick={loadJobs} title="Refresh">↻</button>
          </div>

          {#if loadingJobs}
            <p class="muted-msg">Loading...</p>
          {:else if jobs.length === 0}
            <p class="muted-msg">No MD jobs found.</p>
          {:else}
            <div class="job-list">
              {#each jobs as job}
                <button
                  class="job-row"
                  class:selected={selectedJob?.name === job.name}
                  onclick={() => selectJob(job)}
                >
                  <span class="job-dot {statusColor(job.status)}"></span>
                  <span class="job-name">{job.name}</span>
                  <span class="job-meta">{job.top_n}×</span>
                </button>
              {/each}
            </div>
          {/if}

          <!-- Results for selected job -->
          {#if selectedJob}
            <div class="section-label" style="margin-top: 8px;">
              {selectedJob.name}
              {#if loadingResults}<span class="loading-inline">loading...</span>{/if}
            </div>

            {#if !loadingResults && results.length === 0}
              <p class="muted-msg">No completed compounds yet.</p>
            {:else if results.length > 0}
              <div class="result-list">
                {#each results as r}
                  {@const isLoading = viewerLoading === r.compound_id}
                  {@const hasData = r.has_trajectory || r.has_energy}
                  <div class="result-row">
                    <span class="result-id">{r.compound_id}</span>
                    <span class="result-aff">{r.dock_affinity_kcal_mol?.toFixed(2)}</span>
                    <div class="result-icons">
                      {#if r.has_trajectory}<span class="icon-pill traj" title="3D trajectory">3D</span>{/if}
                      {#if r.has_energy}<span class="icon-pill energy" title="Energy plot">E</span>{/if}
                    </div>
                    <button
                      class="view-btn"
                      onclick={() => viewCompound(r)}
                      disabled={!!viewerLoading || !hasData}
                      title={!hasData ? 'Post-processing pending' : 'View trajectory'}
                    >
                      {#if isLoading}
                        <span class="spin-dot"></span>
                      {:else}
                        View
                      {/if}
                    </button>
                  </div>
                {/each}
              </div>
            {/if}
          {/if}
        </div>
      {/if}
    </div>
  {/snippet}
</Panel>

<style>
  .traj-browser {
    display: flex;
    flex-direction: column;
  }

  .browse-btn {
    background: rgba(88, 166, 255, 0.1);
    border: 1px solid rgba(88, 166, 255, 0.3);
    color: var(--accent, #58a6ff);
    font-size: 12px;
    font-weight: 600;
    padding: 6px 12px;
    border-radius: 4px;
    cursor: pointer;
    width: 100%;
    transition: all 0.15s;
  }

  .browse-btn:hover { background: rgba(88, 166, 255, 0.2); }

  .browser-body {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .section-label {
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.4px;
    color: var(--text-muted, #484f58);
    display: flex;
    align-items: center;
    gap: 4px;
    margin-bottom: 4px;
  }

  .refresh-btn {
    margin-left: auto;
    background: none;
    border: none;
    color: var(--text-muted, #484f58);
    font-size: 13px;
    cursor: pointer;
    padding: 0;
    line-height: 1;
    transition: color 0.12s;
  }

  .refresh-btn:hover { color: var(--accent, #58a6ff); }

  .loading-inline {
    font-size: 10px;
    color: var(--text-muted, #484f58);
    font-weight: 400;
    text-transform: none;
    margin-left: 4px;
  }

  .muted-msg {
    font-size: 11px;
    color: var(--text-muted, #484f58);
    font-style: italic;
    margin: 0;
    padding: 2px 0;
  }

  .job-list {
    display: flex;
    flex-direction: column;
    gap: 1px;
    max-height: 160px;
    overflow-y: auto;
  }

  .job-row {
    display: flex;
    align-items: center;
    gap: 6px;
    background: none;
    border: 1px solid transparent;
    padding: 4px 6px;
    border-radius: 3px;
    cursor: pointer;
    text-align: left;
    width: 100%;
    transition: all 0.1s;
  }

  .job-row:hover { background: rgba(88, 166, 255, 0.06); }
  .job-row.selected {
    background: rgba(88, 166, 255, 0.1);
    border-color: rgba(88, 166, 255, 0.25);
  }

  .job-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .job-dot.done    { background: #3fb950; }
  .job-dot.running { background: #d29922; animation: pulse 1.4s ease-in-out infinite; }
  .job-dot.failed  { background: #f85149; }
  .job-dot.pending { background: rgba(48,54,61,0.8); }

  @keyframes pulse { 0%,100%{opacity:1}50%{opacity:0.4} }

  .job-name {
    flex: 1;
    font-size: 11px;
    font-family: 'SF Mono', monospace;
    color: var(--text-secondary, #8b949e);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .job-meta {
    font-size: 10px;
    color: var(--text-muted, #484f58);
    flex-shrink: 0;
  }

  .result-list {
    display: flex;
    flex-direction: column;
    gap: 1px;
    max-height: 240px;
    overflow-y: auto;
  }

  .result-row {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 3px 4px;
    border-radius: 3px;
  }

  .result-row:hover { background: rgba(88,166,255,0.04); }

  .result-id {
    flex: 1;
    font-size: 10px;
    font-family: 'SF Mono', monospace;
    color: var(--text-secondary, #8b949e);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .result-aff {
    font-size: 10px;
    color: #3fb950;
    flex-shrink: 0;
  }

  .result-icons {
    display: flex;
    gap: 2px;
    flex-shrink: 0;
  }

  .icon-pill {
    font-size: 8px;
    font-weight: 700;
    padding: 1px 4px;
    border-radius: 2px;
  }

  .icon-pill.traj   { background: rgba(88,166,255,0.15); color: var(--accent, #58a6ff); }
  .icon-pill.energy { background: rgba(210,153,34,0.15); color: #d29922; }

  .view-btn {
    font-size: 10px;
    font-weight: 600;
    background: rgba(88,166,255,0.1);
    border: 1px solid rgba(88,166,255,0.3);
    color: var(--accent, #58a6ff);
    padding: 2px 7px;
    border-radius: 3px;
    cursor: pointer;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    min-width: 36px;
    height: 18px;
    transition: all 0.12s;
  }

  .view-btn:hover:not(:disabled) { background: rgba(88,166,255,0.2); }
  .view-btn:disabled { opacity: 0.35; cursor: default; }

  .spin-dot {
    width: 5px;
    height: 5px;
    background: var(--accent, #58a6ff);
    border-radius: 50%;
    animation: pulse 0.8s ease-in-out infinite;
  }
</style>
