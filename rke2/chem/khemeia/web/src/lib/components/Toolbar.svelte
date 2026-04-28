<script lang="ts">
  import { isAuthenticated, getUser, login, logout } from '$lib/auth';
  import type { UserInfo } from '$lib/auth';

  type Tab = 'explorer' | 'analysis' | 'calculations' | 'pipeline';

  let { activeTab = $bindable('explorer'), onCommandPalette, authReady = false }: {
    activeTab: Tab;
    onCommandPalette?: () => void;
    authReady?: boolean;
  } = $props();

  const tabs: { id: Tab; label: string }[] = [
    { id: 'explorer', label: 'Explorer' },
    { id: 'analysis', label: 'Analysis' },
    { id: 'calculations', label: 'Calculations' },
    { id: 'pipeline', label: 'Pipeline' },
  ];

  const isMac = typeof navigator !== 'undefined' && navigator.platform?.includes('Mac');
  const modKey = isMac ? '\u2318' : 'Ctrl';

  function getUserDisplay(u: UserInfo): string {
    return u.name || u.preferred_username || u.email || u.sub;
  }
</script>

<header class="toolbar">
  <div class="toolbar-left">
    <span class="logo">khemeia</span>
  </div>

  <nav class="toolbar-tabs">
    {#each tabs as tab}
      <button
        class="tab-btn"
        class:active={activeTab === tab.id}
        onclick={() => (activeTab = tab.id)}
      >
        {tab.label}
      </button>
    {/each}
  </nav>

  <div class="toolbar-right">
    <button class="cmd-hint" onclick={() => onCommandPalette?.()}>
      {modKey}+K
    </button>

    {#if authReady}
      {#if isAuthenticated()}
        {@const u = getUser()}
        {#if u}
          <span class="user-display">{getUserDisplay(u)}</span>
        {/if}
        <button class="auth-btn" onclick={() => { logout(); window.location.reload(); }}>
          Sign Out
        </button>
      {:else}
        <button class="auth-btn auth-btn-primary" onclick={() => login()}>
          Sign In
        </button>
      {/if}
    {/if}
  </div>
</header>

<style>
  .toolbar {
    display: flex;
    align-items: center;
    height: 48px;
    padding: 0 16px;
    background: var(--bg-surface);
    backdrop-filter: var(--panel-blur);
    border-bottom: 1px solid var(--border-default);
    position: relative;
    z-index: 100;
    flex-shrink: 0;
  }

  .toolbar-left {
    flex: 0 0 auto;
  }

  .logo-img {
    width: 24px;
    height: 24px;
    margin-right: 6px;
  }

  .logo {
    font-family: var(--font-mono);
    font-size: 16px;
    font-weight: 600;
    color: var(--accent);
    letter-spacing: 0.5px;
  }

  .toolbar-tabs {
    display: flex;
    gap: 2px;
    margin-left: 32px;
  }

  .tab-btn {
    background: none;
    border: none;
    color: var(--text-secondary);
    font-size: 13px;
    font-weight: 500;
    padding: 6px 14px;
    border-radius: var(--radius-md);
    cursor: pointer;
    transition: all var(--transition-fast);
    font-family: var(--font-sans);
  }

  .tab-btn:hover {
    color: var(--text-primary);
    background: var(--accent-subtle);
  }

  .tab-btn.active {
    color: var(--accent);
    background: var(--accent-subtle);
  }

  .toolbar-right {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: 0;
  }

  .cmd-hint {
    background: var(--bg-input);
    border: 1px solid var(--border-default);
    color: var(--text-muted);
    font-family: var(--font-mono);
    font-size: 11px;
    padding: 3px 8px;
    border-radius: var(--radius-sm);
    cursor: pointer;
    transition: all var(--transition-fast);
  }

  .cmd-hint:hover {
    color: var(--text-secondary);
    border-color: var(--text-muted);
  }

  .user-display {
    color: var(--text-secondary);
    font-size: 13px;
    margin-left: 12px;
  }

  .auth-btn {
    background: none;
    border: 1px solid var(--border-default);
    color: var(--text-secondary);
    font-family: var(--font-sans);
    font-size: 12px;
    padding: 4px 12px;
    border-radius: var(--radius-sm);
    cursor: pointer;
    margin-left: 8px;
    transition: all var(--transition-fast);
  }

  .auth-btn:hover {
    color: var(--text-primary);
    border-color: var(--text-muted);
  }

  .auth-btn-primary {
    background: var(--accent-subtle);
    border-color: var(--accent);
    color: var(--accent);
  }

  .auth-btn-primary:hover {
    background: var(--accent);
    color: var(--bg-base);
  }
</style>
