<script lang="ts">
  import '../app.css';
  import { page } from '$app/state';

  let { children } = $props();

  const links = [
    { href: '/', label: 'Home' },
    { href: '/players', label: 'Players' },
    { href: '/maps', label: 'Maps' },
  ];

  function isActive(href: string): boolean {
    if (href === '/') return page.url.pathname === '/';
    return page.url.pathname.startsWith(href);
  }
</script>

<nav class="nav">
  <div class="nav-inner">
    <a href="/" class="nav-brand">surf<span class="accent">.hwcopeland.net</span></a>
    {#each links as l}
      <a href={l.href} class:active={isActive(l.href)}>{l.label}</a>
    {/each}
  </div>
</nav>

<main class="container-page">
  {@render children()}
</main>
