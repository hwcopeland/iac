import { setToken } from './api';

// Authentik OIDC endpoints
const AUTHORIZE_URL = 'https://auth.hwcopeland.net/application/o/authorize/';
const TOKEN_URL = 'https://auth.hwcopeland.net/application/o/token/';
const USERINFO_URL = 'https://auth.hwcopeland.net/application/o/userinfo/';
const CLIENT_ID = 'khemeia';
const SCOPES = 'openid email profile';

// Derive redirect URI from the current origin
function getRedirectUri(): string {
  return `${window.location.origin}/auth/callback`;
}

// Session storage keys
const VERIFIER_KEY = 'khemeia_pkce_verifier';
const REFRESH_KEY = 'khemeia_refresh_token';

// In-memory state
let accessToken: string | null = null;
let user: UserInfo | null = null;

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
  for (const b of bytes) {
    binary += String.fromCharCode(b);
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

async function createCodeChallenge(verifier: string): Promise<string> {
  const hash = await sha256(verifier);
  return base64UrlEncode(hash);
}

// --- Public API ---

/**
 * Start the OIDC login flow.
 * Generates a PKCE code_verifier, stores it in sessionStorage,
 * and redirects the browser to the Authentik authorize endpoint.
 */
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

/**
 * Handle the authorization callback.
 * Extracts the code from the URL, exchanges it for tokens using the stored
 * code_verifier, and fetches user info.
 */
export async function handleCallback(code: string): Promise<void> {
  const verifier = sessionStorage.getItem(VERIFIER_KEY);
  if (!verifier) {
    throw new Error('Missing PKCE code_verifier in session storage');
  }

  // Exchange code for tokens
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

  // Store tokens
  accessToken = data.access_token;
  if (data.refresh_token) {
    sessionStorage.setItem(REFRESH_KEY, data.refresh_token);
  }

  // Clean up verifier -- no longer needed
  sessionStorage.removeItem(VERIFIER_KEY);

  // Wire token to the API client
  setToken(accessToken!);

  // Fetch user info
  await fetchUserInfo();
}

/**
 * Fetch user info from the OIDC userinfo endpoint using the current access token.
 */
async function fetchUserInfo(): Promise<void> {
  if (!accessToken) return;

  const res = await fetch(USERINFO_URL, {
    headers: { Authorization: `Bearer ${accessToken}` },
  });

  if (res.ok) {
    user = await res.json();
  }
}

/**
 * Log out: clear all auth state.
 */
export function logout(): void {
  accessToken = null;
  user = null;
  sessionStorage.removeItem(REFRESH_KEY);
  sessionStorage.removeItem(VERIFIER_KEY);
  setToken('');
}

/**
 * Get the cached user info, or null if not authenticated.
 */
export function getUser(): UserInfo | null {
  return user;
}

/**
 * Check whether the user has an active access token.
 */
export function isAuthenticated(): boolean {
  return accessToken !== null;
}

/**
 * Attempt to restore a session from sessionStorage.
 * Called once on app load. If a refresh_token exists, we attempt to use it
 * to get a new access_token. For now (no refresh flow), this is a no-op
 * stub that clears stale state.
 */
export async function restoreSession(): Promise<void> {
  const refreshToken = sessionStorage.getItem(REFRESH_KEY);
  if (!refreshToken) return;

  // Attempt token refresh using the stored refresh_token
  try {
    const body = new URLSearchParams({
      grant_type: 'refresh_token',
      client_id: CLIENT_ID,
      refresh_token: refreshToken,
    });

    const res = await fetch(TOKEN_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body,
    });

    if (!res.ok) {
      // Refresh failed -- clear stale state, user needs to log in again
      logout();
      return;
    }

    const data = await res.json();
    accessToken = data.access_token;
    if (data.refresh_token) {
      sessionStorage.setItem(REFRESH_KEY, data.refresh_token);
    }

    setToken(accessToken!);
    await fetchUserInfo();
  } catch {
    // Network error or other failure -- clear state
    logout();
  }
}
