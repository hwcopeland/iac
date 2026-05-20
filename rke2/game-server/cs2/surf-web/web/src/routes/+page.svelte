<script lang="ts">
  import { api, type PlayerSummary, type RunRecord, type ServerInfo } from '$lib/api';
  import { rankClass } from '$lib/format';
  import PlayerCell from '$lib/components/PlayerCell.svelte';
  import RunRow from '$lib/components/RunRow.svelte';

  let players = $state<PlayerSummary[] | null>(null);
  let records = $state<RunRecord[] | null>(null);
  let server = $state<ServerInfo | null>(null);
  let error = $state<string | null>(null);

  $effect(() => {
    Promise.all([api.players({ limit: 25 }), api.records(15, 'all'), api.server()])
      .then(([p, r, s]) => { players = p; records = r; server = s; })
      .catch((e) => { error = String(e); });
  });
</script>

{#if error}
  <div class="card"><p class="dim">Failed to load: {error}</p></div>
{:else}
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
      <div class="card-header"><span class="card-title">Recent Records</span></div>
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
