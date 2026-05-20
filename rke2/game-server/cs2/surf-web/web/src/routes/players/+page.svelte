<script lang="ts">
  import { api, type PlayerSort, type PlayerSummary, type SortOrder } from '$lib/api';
  import { rankClass } from '$lib/format';
  import PlayerCell from '$lib/components/PlayerCell.svelte';
  import SortHeader from '$lib/components/SortHeader.svelte';

  let players = $state<PlayerSummary[] | null>(null);
  let offset = $state(0);
  let search = $state('');
  let sort = $state<PlayerSort>('points');
  let order = $state<SortOrder>('desc');
  let error = $state<string | null>(null);
  const limit = 50;

  let searchTimer: ReturnType<typeof setTimeout> | null = null;

  function reload() {
    players = null;
    api.players({ limit, offset, search: search || undefined, sort, order })
      .then((p) => { players = p; })
      .catch((e) => { error = String(e); });
  }

  function onSearch(v: string) {
    search = v;
    offset = 0;
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = setTimeout(reload, 250);
  }

  function onSort(key: string) {
    if (sort === key) {
      order = order === 'desc' ? 'asc' : 'desc';
    } else {
      sort = key as PlayerSort;
      order = key === 'name' ? 'asc' : 'desc';
    }
    offset = 0;
    reload();
  }

  $effect(() => { reload(); });
</script>

<section class="card">
  <div class="card-header">
    <span class="card-title">Players</span>
    <input
      class="search"
      type="search"
      placeholder="Search by name…"
      value={search}
      oninput={(e) => onSearch((e.currentTarget as HTMLInputElement).value)}
    />
  </div>
  <table class="lb">
    <thead>
      <tr>
        <th>#</th>
        <th><SortHeader label="Player" sortKey="name" activeSort={sort} {order} {onSort} /></th>
        <th><SortHeader label="Points" sortKey="points" activeSort={sort} {order} {onSort} /></th>
        <th><SortHeader label="Maps" sortKey="completions" activeSort={sort} {order} {onSort} /></th>
      </tr>
    </thead>
    <tbody>
      {#if players && players.length === 0}
        <tr><td colspan="4" class="dim" style="padding: 2rem; text-align: center;">No players match.</td></tr>
      {:else if players}
        {#each players as p}
          <tr>
            <td class={rankClass(p.rank ?? 0)}>{p.rank}</td>
            <td><PlayerCell steamId={p.steam_id} name={p.name} avatar={p.avatar} /></td>
            <td class="mono">{p.points.toLocaleString()}</td>
            <td class="mono dim">{p.map_completions}</td>
          </tr>
        {/each}
      {:else}
        {#each Array(10) as _}
          <tr><td colspan="4"><div class="skeleton"></div></td></tr>
        {/each}
      {/if}
    </tbody>
  </table>

  <div style="display: flex; justify-content: space-between; margin-top: 1rem;">
    <button class="search" style="cursor: pointer; max-width: 100px;"
      disabled={offset === 0}
      onclick={() => { offset = Math.max(0, offset - limit); reload(); }}>← Prev</button>
    <span class="dim mono" style="align-self: center;">offset {offset}</span>
    <button class="search" style="cursor: pointer; max-width: 100px;"
      disabled={!players || players.length < limit}
      onclick={() => { offset += limit; reload(); }}>Next →</button>
  </div>
  {#if error}<p class="dim">{error}</p>{/if}
</section>
