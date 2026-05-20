<script lang="ts">
  import type { RunRecord } from '$lib/api';
  import { formatRelative, formatTime, mapDisplayName, trackLabel } from '$lib/format';
  import PlayerCell from './PlayerCell.svelte';

  let { run, showMap = true }: { run: RunRecord; showMap?: boolean } = $props();
</script>

<tr>
  {#if showMap}
    <td><a href="/maps/{run.map_file}">{mapDisplayName(run.map_file)}</a></td>
  {/if}
  <td><span class="pill">{trackLabel(run.track, run.stage, run.run_type)}</span></td>
  <td><PlayerCell steamId={run.steam_id} name={run.player_name} avatar={run.avatar} /></td>
  <td class="mono">{formatTime(run.time)}</td>
  <td class="mono dim">{run.jumps}</td>
  <td class="dim">{formatRelative(run.date)}</td>
</tr>
