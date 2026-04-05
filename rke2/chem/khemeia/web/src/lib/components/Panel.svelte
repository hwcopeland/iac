<script lang="ts">
  import type { Snippet } from 'svelte';

  let { title, collapsed = $bindable(false), children }: {
    title: string;
    collapsed?: boolean;
    children: Snippet;
  } = $props();
</script>

<section class="panel" class:collapsed>
  <button class="panel-header" onclick={() => (collapsed = !collapsed)}>
    <span class="panel-title">{title}</span>
    <span class="panel-chevron">{collapsed ? '\u25B6' : '\u25BC'}</span>
  </button>
  {#if !collapsed}
    <div class="panel-body">
      {@render children()}
    </div>
  {/if}
</section>

<style>
  .panel {
    background: var(--bg-surface);
    backdrop-filter: var(--panel-blur);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-lg);
    overflow: hidden;
    margin-bottom: 8px;
    box-shadow: var(--shadow-md);
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    padding: 10px 14px;
    background: none;
    border: none;
    color: var(--text-primary);
    font-size: 12px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    cursor: pointer;
    transition: background var(--transition-fast);
    font-family: var(--font-sans);
  }

  .panel-header:hover {
    background: rgba(255, 255, 255, 0.03);
  }

  .panel-chevron {
    font-size: 9px;
    color: var(--text-muted);
  }

  .panel-body {
    padding: 0 14px 14px;
  }
</style>
