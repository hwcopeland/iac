<script lang="ts">
  import { api, type MapSummary, type SortOrder } from '$lib/api';
  import { formatTime, mapDisplayName } from '$lib/format';
  import SortHeader from '$lib/components/SortHeader.svelte';

  type MapSort = 'name' | 'tier' | 'completions' | 'record';

  let maps = $state<MapSummary[] | null>(null);
  let tierFilter = $state<number | null>(null);
  let search = $state('');
  let sort = $state<MapSort>('tier');
  let order = $state<SortOrder>('asc');
  let error = $state<string | null>(null);

  $effect(() => {
    api.maps().then((m) => { maps = m; }).catch((e) => { error = String(e); });
  });

  function onSort(key: string) {
    if (sort === key) {
      order = order === 'desc' ? 'asc' : 'desc';
    } else {
      sort = key as MapSort;
      order = key === 'name' ? 'asc' : 'desc';
    }
  }

  function compare(a: MapSummary, b: MapSummary): number {
    let v = 0;
    switch (sort) {
      case 'name': v = a.file.localeCompare(b.file); break;
      case 'tier': v = a.tier - b.tier; break;
      case 'completions': v = a.completions - b.completions; break;
      case 'record':
        // null record times sink to the bottom regardless of order
        if (a.record_time == null && b.record_time == null) v = 0;
        else if (a.record_time == null) return 1;
        else if (b.record_time == null) return -1;
        else v = a.record_time - b.record_time;
        break;
    }
    return order === 'asc' ? v : -v;
  }

  let filtered = $derived.by(() => {
    if (!maps) return null;
    return maps
      .filter((m) => {
        if (tierFilter !== null && m.tier !== tierFilter) return false;
        if (search && !m.file.toLowerCase().includes(search.toLowerCase())) return false;
        return true;
      })
      .sort(compare);
  });

  let tiers = $derived.by(() => {
    if (!maps) return [];
    return [...new Set(maps.map((m) => m.tier))].sort();
  });
</script>

<section class="card">
  <div class="card-header">
    <span class="card-title">Maps</span>
    <input
      class="search"
      type="search"
      placeholder="Search maps…"
      value={search}
      oninput={(e) => (search = (e.currentTarget as HTMLInputElement).value)}
    />
  </div>

  <div style="margin-bottom: 1rem; display: flex; gap: 0.5rem; flex-wrap: wrap;">
    <button class="pill" style="cursor: pointer; border: none;"
      onclick={() => (tierFilter = null)}>All</button>
    {#each tiers as t}
      <button class="tier tier-{t}" style="cursor: pointer; border: none;"
        onclick={() => (tierFilter = t)}>T{t}</button>
    {/each}
  </div>

  <table class="lb">
    <thead>
      <tr>
        <th><SortHeader label="Map" sortKey="name" activeSort={sort} {order} {onSort} /></th>
        <th><SortHeader label="Tier" sortKey="tier" activeSort={sort} {order} {onSort} /></th>
        <th>Stages</th>
        <th><SortHeader label="Completions" sortKey="completions" activeSort={sort} {order} {onSort} /></th>
        <th><SortHeader label="Record" sortKey="record" activeSort={sort} {order} {onSort} /></th>
        <th>Holder</th>
      </tr>
    </thead>
    <tbody>
      {#if filtered}
        {#each filtered as m}
          <tr>
            <td><a href="/maps/{m.file}">{mapDisplayName(m.file)}</a></td>
            <td><span class="tier tier-{m.tier}">T{m.tier}</span></td>
            <td class="mono dim">{m.stages || '—'}</td>
            <td class="mono dim">{m.completions}</td>
            <td class="mono">{m.record_time ? formatTime(m.record_time) : '—'}</td>
            <td class="dim">{m.record_holder ?? '—'}</td>
          </tr>
        {/each}
      {:else}
        {#each Array(10) as _}
          <tr><td colspan="6"><div class="skeleton"></div></td></tr>
        {/each}
      {/if}
    </tbody>
  </table>
  {#if error}<p class="dim">{error}</p>{/if}
</section>
