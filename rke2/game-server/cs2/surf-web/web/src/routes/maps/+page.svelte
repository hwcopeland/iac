<script lang="ts">
  import { api, type MapSummary } from '$lib/api';
  import { formatTime, mapDisplayName } from '$lib/format';

  let maps = $state<MapSummary[] | null>(null);
  let tierFilter = $state<number | null>(null);
  let search = $state('');
  let error = $state<string | null>(null);

  $effect(() => {
    api.maps().then((m) => { maps = m; }).catch((e) => { error = String(e); });
  });

  let filtered = $derived.by(() => {
    if (!maps) return null;
    return maps.filter((m) => {
      if (tierFilter !== null && m.tier !== tierFilter) return false;
      if (search && !m.file.toLowerCase().includes(search.toLowerCase())) return false;
      return true;
    });
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
      onclick={() => (tierFilter = null)}
      class:active={tierFilter === null}>All</button>
    {#each tiers as t}
      <button class="tier tier-{t}" style="cursor: pointer; border: none;"
        onclick={() => (tierFilter = t)}>T{t}</button>
    {/each}
  </div>

  <table class="lb">
    <thead>
      <tr><th>Map</th><th>Tier</th><th>Stages</th><th>Completions</th><th>WR</th><th>Holder</th></tr>
    </thead>
    <tbody>
      {#if filtered}
        {#each filtered as m}
          <tr>
            <td><a href="/maps/{m.file}">{mapDisplayName(m.file)}</a></td>
            <td><span class="tier tier-{m.tier}">T{m.tier}</span></td>
            <td class="mono dim">{m.stages || '—'}</td>
            <td class="mono dim">{m.completions}</td>
            <td class="mono">{m.wr_time ? formatTime(m.wr_time) : '—'}</td>
            <td class="dim">{m.wr_holder ?? '—'}</td>
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
