<script lang="ts" module>
  type ToastType = 'info' | 'success' | 'error' | 'warning';
  type ToastItem = { id: number; message: string; type: ToastType };

  let toasts = $state<ToastItem[]>([]);
  let nextId = 0;

  export function toast(message: string, type: ToastType = 'info', duration = 4000) {
    const id = nextId++;
    toasts.push({ id, message, type });
    setTimeout(() => {
      toasts = toasts.filter((t) => t.id !== id);
    }, duration);
  }
</script>

<script lang="ts">
  function dismiss(id: number) {
    toasts = toasts.filter((t) => t.id !== id);
  }
</script>

{#if toasts.length > 0}
  <div class="toast-container">
    {#each toasts as t (t.id)}
      <div class="toast toast-{t.type}">
        <span class="toast-msg">{t.message}</span>
        <button class="toast-close" onclick={() => dismiss(t.id)}>x</button>
      </div>
    {/each}
  </div>
{/if}

<style>
  .toast-container {
    position: fixed;
    top: 60px;
    right: 16px;
    z-index: 900;
    display: flex;
    flex-direction: column;
    gap: 6px;
    max-width: 360px;
  }

  .toast {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 10px 14px;
    border-radius: var(--radius-md);
    background: var(--bg-surface);
    backdrop-filter: var(--panel-blur);
    border: 1px solid var(--border-default);
    box-shadow: var(--shadow-md);
    font-size: 13px;
    color: var(--text-primary);
    animation: toast-in 200ms ease;
  }

  .toast-info {
    border-left: 3px solid var(--accent);
  }

  .toast-success {
    border-left: 3px solid var(--success);
  }

  .toast-error {
    border-left: 3px solid var(--danger);
  }

  .toast-warning {
    border-left: 3px solid var(--warning);
  }

  .toast-msg {
    flex: 1;
  }

  .toast-close {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 12px;
    padding: 2px 4px;
    font-family: var(--font-sans);
  }

  .toast-close:hover {
    color: var(--text-primary);
  }

  @keyframes toast-in {
    from {
      opacity: 0;
      transform: translateX(20px);
    }
    to {
      opacity: 1;
      transform: translateX(0);
    }
  }
</style>
