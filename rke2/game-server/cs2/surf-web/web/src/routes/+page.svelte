<script lang="ts">
  import { onDestroy } from 'svelte';
  import { api, type LiveServer, type PlayerSummary, type RunRecord, type ServerInfo } from '$lib/api';
  import { rankClass } from '$lib/format';
  import PlayerCell from '$lib/components/PlayerCell.svelte';
  import RunRow from '$lib/components/RunRow.svelte';

  let players = $state<PlayerSummary[] | null>(null);
  let records = $state<RunRecord[] | null>(null);
  let server = $state<ServerInfo | null>(null);
  let live = $state<LiveServer | null>(null);
  let error = $state<string | null>(null);

  function refreshLive() {
    api.live().then((l) => { live = l; }).catch(() => {});
  }

  let liveTimer: ReturnType<typeof setInterval> | undefined;

  $effect(() => {
    Promise.all([api.players({ limit: 25 }), api.records(15, 'main'), api.server()])
      .then(([p, r, s]) => { players = p; records = r; server = s; })
      .catch((e) => { error = String(e); });
    refreshLive();
    liveTimer = setInterval(refreshLive, 15000);
  });

  onDestroy(() => { if (liveTimer) clearInterval(liveTimer); });
</script>

{#if error}
  <div class="card"><p class="dim">Failed to load: {error}</p></div>
{:else}
  <section class="card live-card" class:live-off={!live?.online}>
    <div class="live-left">
      <span class="live-dot" aria-hidden="true"></span>
      <div>
        <div class="live-status">{live?.online ? 'Server online' : live ? 'Server offline' : 'Checking…'}</div>
        <div class="live-name dim">{live?.info?.name ?? '—'}</div>
      </div>
    </div>
    <div class="live-right">
      <div class="live-stat">
        <span class="label">Map</span>
        <span class="value mono">{live?.info?.map ?? '—'}</span>
      </div>
      <div class="live-stat">
        <span class="label">Players</span>
        <span class="value mono">
          {live?.info ? `${live.info.players}/${live.info.max_players}` : '—'}
          {#if live?.info?.bots}<span class="dim"> (+{live.info.bots} bot{live.info.bots > 1 ? 's' : ''})</span>{/if}
        </span>
      </div>
      <a class="live-join" href="steam://connect/surf.hwcopeland.net:27015">Connect</a>
    </div>
  </section>

  <div class="stats-row">
    <div class="card stat">
      <span class="label">Players</span>
      <span class="value">{server?.counts.players ?? '—'}</span>
    </div>
    <div class="card stat">
      <span class="label">Total Runs</span>
      <span class="value">{server?.counts.runs ?? '—'}</span>
    </div>
    <div class="card stat">
      <span class="label">Maps</span>
      <span class="value">{server?.counts.maps ?? '—'}</span>
    </div>
  </div>

  <div class="grid-2">
    <section class="card">
      <div class="card-header">
        <span class="card-title">Top Players</span>
        <a href="/players" class="dim">all →</a>
      </div>
      <table class="lb">
        <thead>
          <tr><th>#</th><th>Player</th><th>Points</th><th>Maps</th></tr>
        </thead>
        <tbody>
          {#if players}
            {#each players as p}
              <tr>
                <td class={rankClass(p.rank ?? 0)}>{p.rank}</td>
                <td><PlayerCell steamId={p.steam_id} name={p.name} avatar={p.avatar} /></td>
                <td class="mono">{p.points.toLocaleString()}</td>
                <td class="mono dim">{p.map_completions}</td>
              </tr>
            {/each}
          {:else}
            {#each Array(8) as _}
              <tr><td colspan="4"><div class="skeleton"></div></td></tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </section>

    <section class="card">
      <div class="card-header"><span class="card-title">Recent Map Records</span></div>
      <table class="lb">
        <thead>
          <tr><th>Map</th><th>Track</th><th>Player</th><th>Time</th><th>Jmp</th><th>When</th></tr>
        </thead>
        <tbody>
          {#if records}
            {#each records as r}<RunRow run={r} />{/each}
          {:else}
            {#each Array(8) as _}
              <tr><td colspan="6"><div class="skeleton"></div></td></tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </section>
  </div>
{/if}
