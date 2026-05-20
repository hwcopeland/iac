<script lang="ts">
  let {
    label,
    sortKey,
    activeSort,
    order,
    onSort,
    align = 'left',
  }: {
    label: string;
    sortKey: string;
    activeSort: string | null;
    order: 'asc' | 'desc';
    onSort: (key: string) => void;
    align?: 'left' | 'right';
  } = $props();

  let isActive = $derived(activeSort === sortKey);
  let arrow = $derived(isActive ? (order === 'asc' ? '↑' : '↓') : '');
</script>

<button
  type="button"
  class="sort-btn"
  class:active={isActive}
  style:text-align={align}
  onclick={() => onSort(sortKey)}
>
  {label}{#if arrow}&nbsp;<span class="arrow">{arrow}</span>{/if}
</button>

<style>
  .sort-btn {
    background: none;
    border: none;
    padding: 0;
    color: inherit;
    font: inherit;
    cursor: pointer;
    text-transform: inherit;
    letter-spacing: inherit;
  }
  .sort-btn:hover { color: var(--text); }
  .sort-btn.active { color: var(--accent); }
  .arrow { font-weight: 700; }
</style>
