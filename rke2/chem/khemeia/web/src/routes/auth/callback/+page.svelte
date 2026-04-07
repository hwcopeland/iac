<script lang="ts">
  import { onMount } from 'svelte';
  import { handleCallback } from '$lib/auth';
  import { goto } from '$app/navigation';

  let error = $state<string | null>(null);
  let processing = $state(true);

  onMount(async () => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get('code');
    const errorParam = params.get('error');
    const errorDescription = params.get('error_description');

    if (errorParam) {
      error = errorDescription || errorParam;
      processing = false;
      return;
    }

    if (!code) {
      error = 'No authorization code received';
      processing = false;
      return;
    }

    try {
      await handleCallback(code);
      goto('/', { replaceState: true });
    } catch (e) {
      error = e instanceof Error ? e.message : 'Authentication failed';
      processing = false;
    }
  });
</script>

<div class="callback-page">
  {#if processing}
    <div class="status">
      <div class="spinner"></div>
      <p>Signing in...</p>
    </div>
  {:else if error}
    <div class="status error">
      <p class="error-title">Authentication failed</p>
      <p class="error-detail">{error}</p>
      <a href="/" class="back-link">Back to Khemeia</a>
    </div>
  {/if}
</div>

<style>
  .callback-page {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100vh;
    background: var(--bg-base);
  }

  .status {
    text-align: center;
    color: var(--text-secondary);
    font-family: var(--font-sans);
  }

  .spinner {
    width: 24px;
    height: 24px;
    border: 2px solid var(--border-default);
    border-top-color: var(--accent);
    border-radius: 50%;
    margin: 0 auto 12px;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .error .error-title {
    color: var(--danger);
    font-weight: 600;
    margin-bottom: 4px;
  }

  .error .error-detail {
    color: var(--text-muted);
    font-size: 13px;
    margin-bottom: 16px;
  }

  .back-link {
    color: var(--accent);
    text-decoration: none;
    font-size: 13px;
  }

  .back-link:hover {
    color: var(--accent-hover);
  }
</style>
