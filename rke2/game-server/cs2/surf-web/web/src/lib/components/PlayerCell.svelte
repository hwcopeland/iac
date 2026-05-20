<script lang="ts">
  import { defaultAvatar } from '$lib/format';

  let { steamId, name, avatar = null }: {
    steamId: string;
    name: string;
    avatar?: string | null;
  } = $props();
</script>

<a href="/players/{steamId}" class="player-cell">
  <img class="avatar" src={avatar ?? defaultAvatar()} alt="" loading="lazy" />
  <span>{name}</span>
</a>
