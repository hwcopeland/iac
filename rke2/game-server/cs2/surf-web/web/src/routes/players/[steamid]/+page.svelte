<script lang="ts">
  import { page } from '$app/state';
  import { api, type PlayerProfile } from '$lib/api';
  import { defaultAvatar, formatRelative, formatTime, mapDisplayName, trackLabel } from '$lib/format';

  let profile = $state<PlayerProfile | null>(null);
  let error = $state<string | null>(null);

  $effect(() => {
    const sid = page.params.steamid;
    if (!sid) return;
    profile = null;
    api.player(sid)
      .then((p) => { profile = p; })
      .catch((e) => { error = String(e); });
  });
</script>

{#if error}
  <div class="card"><p class="dim">Failed to load: {error}</p></div>
{:else if !profile}
  <div class="card"><div class="skeleton" style="height: 6rem;"></div></div>
{:else}
  {@const pl = profile.player}
  <section class="card" style="display: flex; gap: 1.5rem; align-items: center;">
    <img src={pl.avatar ?? defaultAvatar()} alt="" style="width: 96px; height: 96px; border-radius: 12px;" />
    <div style="flex: 1;">
      <h1>{pl.name}</h1>
      <p class="dim mono" style="font-size: 0.85rem;">SteamID64: {pl.steam_id}</p>
      {#if pl.profile_url}<p><a href={pl.profile_url} target="_blank" rel="noopener">Steam profile ↗</a></p>{/if}
    </div>
    <div class="stats-row" style="margin: 0; grid-template-columns: repeat(3, minmax(0, 1fr)); flex: 1;">
      <div class="stat"><span class="label">Points</span><span class="value">{pl.points.toLocaleString()}</span></div>
      <div class="stat"><span class="label">Maps</span><span class="value">{pl.map_completions}</span></div>
      <div class="stat"><span class="label">Records</span><span class="value">{pl.record_count}</span></div>
    </div>
  </section>

  <div class="grid-2" style="margin-top: 1.5rem;">
    <section class="card">
      <div class="card-header"><span class="card-title">Personal Bests</span></div>
      <table class="lb">
        <thead><tr><th>Map</th><th>Track</th><th>Time</th><th>Set</th></tr></thead>
        <tbody>
          {#each profile.personal_bests as r}
            <tr>
              <td><a href="/maps/{r.map_file}">{mapDisplayName(r.map_file)}</a></td>
              <td><span class="pill">{trackLabel(r.track, r.stage, r.run_type)}</span></td>
              <td class="mono">{formatTime(r.time)}</td>
              <td class="dim">{formatRelative(r.date)}</td>
            </tr>
          {/each}
          {#if profile.personal_bests.length === 0}
            <tr><td colspan="4" class="dim" style="padding: 1rem; text-align: center;">No PBs yet.</td></tr>
          {/if}
        </tbody>
      </table>
    </section>

    <section class="card">
      <div class="card-header"><span class="card-title">Recent Runs</span></div>
      <table class="lb">
        <thead><tr><th>Map</th><th>Track</th><th>Time</th><th>When</th></tr></thead>
        <tbody>
          {#each profile.recent_runs as r}
            <tr>
              <td><a href="/maps/{r.map_file}">{mapDisplayName(r.map_file)}</a></td>
              <td><span class="pill">{trackLabel(r.track, r.stage, r.run_type)}</span></td>
              <td class="mono">{formatTime(r.time)}</td>
              <td class="dim">{formatRelative(r.date)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </section>
  </div>
{/if}
