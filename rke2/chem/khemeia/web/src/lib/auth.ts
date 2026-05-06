import { setToken } from './api';

const AUTHORIZE_URL = 'https://auth.hwcopeland.net/application/o/authorize/';
const TOKEN_URL = 'https://auth.hwcopeland.net/application/o/token/';
const USERINFO_URL = 'https://auth.hwcopeland.net/application/o/userinfo/';
const CLIENT_ID = 'khemeia';
const SCOPES = 'openid email profile offline_access';

function getRedirectUri(): string {
  return `${window.location.origin}/auth/callback`;
}

const VERIFIER_KEY = 'khemeia_pkce_verifier';
const REFRESH_KEY = 'khemeia_refresh_token';

let accessToken: string | null = null;
let tokenExpiresAt = 0; // epoch ms
let user: UserInfo | null = null;
let refreshTimerId: ReturnType<typeof setTimeout> | null = null;

export interface UserInfo {
  sub: string;
  email?: string;
  email_verified?: boolean;
  name?: string;
  preferred_username?: string;
}

// --- PKCE helpers ---

function generateRandomString(length: number): string {
  const array = new Uint8Array(length);
  crypto.getRandomValues(array);
  return Array.from(array, (b) => b.toString(16).padStart(2, '0')).join('').slice(0, length);
}

async function sha256(plain: string): Promise<ArrayBuffer> {
  const encoder = new TextEncoder();
  return crypto.subtle.digest('SHA-256', encoder.encode(plain));
}

function base64UrlEncode(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

async function createCodeChallenge(verifier: string): Promise<string> {
  return base64UrlEncode(await sha256(verifier));
}

// --- Token refresh ---

function scheduleTokenRefresh(expiresIn?: number): void {
  if (refreshTimerId !== null) {
    clearTimeout(refreshTimerId);
    refreshTimerId = null;
  }
  const ttl = expiresIn ?? 3600;
  tokenExpiresAt = Date.now() + ttl * 1000;
  const delaySeconds = Math.max(30, ttl - 300);
  refreshTimerId = setTimeout(refreshToken, delaySeconds * 1000);
}

export async function refreshToken(): Promise<boolean> {
  const storedRefresh = localStorage.getItem(REFRESH_KEY);
  if (!storedRefresh) return false;

  try {
    const body = new URLSearchParams({
      grant_type: 'refresh_token',
      client_id: CLIENT_ID,
      refresh_token: storedRefresh,
    });

    const res = await fetch(TOKEN_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body,
    });

    if (!res.ok) {
      if (res.status === 400) {
        const errBody = await res.json().catch(() => ({}));
        if (errBody.error === 'invalid_grant') {
          // Refresh token is genuinely expired or revoked — must re-login
          logout();
        }
        // Any other 400: leave session intact
      }
      // 5xx or anything else: leave session intact, visibility handler will retry
      return false;
    }

    const data = await res.json();
    accessToken = data.access_token;
    if (data.refresh_token) localStorage.setItem(REFRESH_KEY, data.refresh_token);

    setToken(accessToken!);
    scheduleTokenRefresh(data.expires_in);
    await fetchUserInfo();
    return true;
  } catch {
    // Network error — don't destroy the session, retry in 60s
    if (refreshTimerId !== null) clearTimeout(refreshTimerId);
    refreshTimerId = setTimeout(refreshToken, 60_000);
    return false;
  }
}

// When the tab becomes visible, refresh if the access token is missing or near expiry.
// Handles the case where the laptop slept through a scheduled timer.
function handleVisibilityChange(): void {
  if (document.visibilityState !== 'visible') return;
  if (!localStorage.getItem(REFRESH_KEY)) return;
  if (!accessToken || Date.now() >= tokenExpiresAt - 300_000) {
    refreshToken();
  }
}

if (typeof document !== 'undefined') {
  document.addEventListener('visibilitychange', handleVisibilityChange);
}

// --- Public API ---

export async function login(): Promise<void> {
  const verifier = generateRandomString(64);
  const challenge = await createCodeChallenge(verifier);
  sessionStorage.setItem(VERIFIER_KEY, verifier);

  const params = new URLSearchParams({
    response_type: 'code',
    client_id: CLIENT_ID,
    redirect_uri: getRedirectUri(),
    scope: SCOPES,
    code_challenge: challenge,
    code_challenge_method: 'S256',
  });

  window.location.href = `${AUTHORIZE_URL}?${params.toString()}`;
}

export async function handleCallback(code: string): Promise<void> {
  const verifier = sessionStorage.getItem(VERIFIER_KEY);
  if (!verifier) throw new Error('Missing PKCE code_verifier in session storage');

  const body = new URLSearchParams({
    grant_type: 'authorization_code',
    client_id: CLIENT_ID,
    code,
    redirect_uri: getRedirectUri(),
    code_verifier: verifier,
  });

  const res = await fetch(TOKEN_URL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body,
  });

  if (!res.ok) {
    const err = await res.text();
    throw new Error(`Token exchange failed: ${res.status} ${err}`);
  }

  const data = await res.json();
  accessToken = data.access_token;
  if (data.refresh_token) localStorage.setItem(REFRESH_KEY, data.refresh_token);
  sessionStorage.removeItem(VERIFIER_KEY);
  setToken(accessToken!);
  scheduleTokenRefresh(data.expires_in);
  await fetchUserInfo();
}

async function fetchUserInfo(): Promise<void> {
  if (!accessToken) return;
  const res = await fetch(USERINFO_URL, {
    headers: { Authorization: `Bearer ${accessToken}` },
  });
  if (res.ok) user = await res.json();
}

export function logout(): void {
  accessToken = null;
  tokenExpiresAt = 0;
  user = null;
  if (refreshTimerId !== null) {
    clearTimeout(refreshTimerId);
    refreshTimerId = null;
  }
  localStorage.removeItem(REFRESH_KEY);
  sessionStorage.removeItem(VERIFIER_KEY);
  setToken('');
}

export function getUser(): UserInfo | null { return user; }
export function isAuthenticated(): boolean { return accessToken !== null; }

export async function restoreSession(): Promise<void> {
  await refreshToken();
}
