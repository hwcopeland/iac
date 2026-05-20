<script lang="ts">
  import { page } from '$app/state';
  import { api, type MapDetail } from '$lib/api';
  import { formatRelative, formatTime, mapDisplayName, rankClass } from '$lib/format';
  import PlayerCell from '$lib/components/PlayerCell.svelte';

  let detail = $state<MapDetail | null>(null);
  let error = $state<string | null>(null);

  $effect(() => {
    const f = page.params.file;
    if (!f) return;
    detail = null;
    api.map(f).then((d) => { detail = d; }).catch((e) => { error = String(e); });
  });
</script>

{#if error}
  <div class="card"><p class="dim">Failed to load: {error}</p></div>
{:else if !detail}
  <div class="card"><div class="skeleton" style="height: 6rem;"></div></div>
{:else}
  {@const m = detail.map}
  <section class="card">
    <div style="display: flex; align-items: center; gap: 1rem; flex-wrap: wrap;">
      <h1>{mapDisplayName(m.file)}</h1>
      <span class="tier tier-{m.tier}">Tier {m.tier}</span>
      <span class="dim mono">{m.file}</span>
    </div>
    <div class="stats-row" style="margin-top: 1.25rem;">
      <div class="stat"><span class="label">Stages</span><span class="value">{m.stages || 0}</span></div>
      <div class="stat"><span class="label">Bonuses</span><span class="value">{detail.bonuses.length}</span></div>
      <div class="stat"><span class="label">Completions</span><span class="value">{m.completions}</span></div>
      <div class="stat">
        <span class="label">Map Record</span>
        <span class="value">{m.record_time ? formatTime(m.record_time) : '—'}</span>
      </div>
    </div>
  </section>

  <section class="card" style="margin-top: 1.5rem;">
    <div class="card-header"><span class="card-title">Main Track — Top 50</span></div>
    <table class="lb">
      <thead>
        <tr><th>#</th><th>Player</th><th>Time</th><th>Jumps</th><th>Strafes</th><th>Sync</th><th>Set</th></tr>
      </thead>
      <tbody>
        {#each detail.main_top as r, i}
          <tr>
            <td class={rankClass(i + 1)}>{i + 1}</td>
            <td><PlayerCell steamId={r.steam_id} name={r.player_name} avatar={r.avatar} /></td>
            <td class="mono">{formatTime(r.time)}</td>
            <td class="mono dim">{r.jumps}</td>
            <td class="mono dim">{r.strafes}</td>
            <td class="mono dim">{r.sync.toFixed(1)}</td>
            <td class="dim">{formatRelative(r.date)}</td>
          </tr>
        {/each}
        {#if detail.main_top.length === 0}
          <tr><td colspan="7" class="dim" style="padding: 1rem; text-align: center;">No runs recorded yet.</td></tr>
        {/if}
      </tbody>
    </table>
  </section>

  {#if detail.stages.length > 0}
    <section class="card" style="margin-top: 1.5rem;">
      <div class="card-header"><span class="card-title">Stages</span></div>
      <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 1.25rem;">
        {#each detail.stages as st}
          <div>
            <h3 style="margin-bottom: 0.5rem;">Stage {st.stage}</h3>
            <table class="lb">
              <tbody>
                {#each st.times.slice(0, 5) as r, i}
                  <tr>
                    <td class={rankClass(i + 1)}>{i + 1}</td>
                    <td><PlayerCell steamId={r.steam_id} name={r.player_name} avatar={r.avatar} /></td>
                    <td class="mono">{formatTime(r.time)}</td>
                  </tr>
                {/each}
                {#if st.times.length === 0}
                  <tr><td class="dim">—</td></tr>
                {/if}
              </tbody>
            </table>
          </div>
        {/each}
      </div>
    </section>
  {/if}

  {#if detail.bonuses.length > 0}
    <section class="card" style="margin-top: 1.5rem;">
      <div class="card-header"><span class="card-title">Bonuses</span></div>
      <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 1.25rem;">
        {#each detail.bonuses as b}
          <div>
            <h3 style="margin-bottom: 0.5rem;">Bonus {b.track}</h3>
            <table class="lb">
              <tbody>
                {#each b.times.slice(0, 5) as r, i}
                  <tr>
                    <td class={rankClass(i + 1)}>{i + 1}</td>
                    <td><PlayerCell steamId={r.steam_id} name={r.player_name} avatar={r.avatar} /></td>
                    <td class="mono">{formatTime(r.time)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/each}
      </div>
    </section>
  {/if}
{/if}
