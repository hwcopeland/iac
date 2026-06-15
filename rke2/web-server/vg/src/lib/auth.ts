import { SignJWT, jwtVerify, createRemoteJWKSet } from 'jose';
import type { AstroCookies } from 'astro';

export const SESSION_COOKIE = 'vg_session';
export const OAUTH_COOKIE = 'vg_oauth';
const ALG = 'HS256';

// Group a user must belong to in order to reach the admin CMS.
export const ADMIN_GROUP = process.env.ADMIN_GROUP ?? 'VG-Admin';

function env(name: string): string {
  const v = process.env[name];
  if (!v) throw new Error(`${name} is not set`);
  return v;
}

function secret(): Uint8Array {
  return new TextEncoder().encode(env('SESSION_SECRET'));
}

// ---------------------------------------------------------------------------
// OIDC discovery (Authentik). Cached for the life of the process.
// ---------------------------------------------------------------------------
interface OidcConfig {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  jwks_uri: string;
  end_session_endpoint?: string;
}
let discoveryPromise: Promise<OidcConfig> | null = null;
let jwks: ReturnType<typeof createRemoteJWKSet> | null = null;

export function getDiscovery(): Promise<OidcConfig> {
  if (!discoveryPromise) {
    const issuer = env('OIDC_ISSUER').replace(/\/$/, '');
    const url = `${issuer}/.well-known/openid-configuration`;
    discoveryPromise = fetch(url).then((r) => {
      if (!r.ok) throw new Error(`OIDC discovery failed: ${r.status}`);
      return r.json() as Promise<OidcConfig>;
    });
  }
  return discoveryPromise;
}

async function getJwks(jwksUri: string) {
  if (!jwks) jwks = createRemoteJWKSet(new URL(jwksUri));
  return jwks;
}

// ---------------------------------------------------------------------------
// Authorization-code flow helpers
// ---------------------------------------------------------------------------
export interface AdminSession {
  email: string;
  name: string;
  groups: string[];
}

export async function buildAuthorizeUrl(redirectUri: string) {
  const cfg = await getDiscovery();
  const state = crypto.randomUUID();
  const nonce = crypto.randomUUID();
  const params = new URLSearchParams({
    response_type: 'code',
    client_id: env('OIDC_CLIENT_ID'),
    redirect_uri: redirectUri,
    scope: process.env.OIDC_SCOPES ?? 'openid profile email groups',
    state,
    nonce,
  });
  return { url: `${cfg.authorization_endpoint}?${params}`, state, nonce };
}

export async function exchangeCode(code: string, redirectUri: string): Promise<string> {
  const cfg = await getDiscovery();
  const body = new URLSearchParams({
    grant_type: 'authorization_code',
    code,
    redirect_uri: redirectUri,
    client_id: env('OIDC_CLIENT_ID'),
    client_secret: env('OIDC_CLIENT_SECRET'),
  });
  const res = await fetch(cfg.token_endpoint, {
    method: 'POST',
    headers: { 'content-type': 'application/x-www-form-urlencoded' },
    body,
  });
  if (!res.ok) throw new Error(`Token exchange failed: ${res.status}`);
  const data = (await res.json()) as { id_token?: string };
  if (!data.id_token) throw new Error('No id_token in token response');
  return data.id_token;
}

// Verifies the ID token signature/claims and returns the admin session, or
// throws if the token is invalid or the user is not in the admin group.
export async function verifyIdToken(idToken: string, expectedNonce: string): Promise<AdminSession> {
  const cfg = await getDiscovery();
  const keys = await getJwks(cfg.jwks_uri);
  const { payload } = await jwtVerify(idToken, keys, {
    issuer: cfg.issuer,
    audience: env('OIDC_CLIENT_ID'),
  });
  if (payload.nonce !== expectedNonce) throw new Error('Nonce mismatch');

  const groups = Array.isArray(payload.groups) ? (payload.groups as string[]) : [];
  if (!groups.includes(ADMIN_GROUP)) {
    const err = new Error('not-authorized') as Error & { code?: string };
    err.code = 'NOT_AUTHORIZED';
    throw err;
  }
  return {
    email: String(payload.email ?? payload.preferred_username ?? ''),
    name: String(payload.name ?? payload.preferred_username ?? payload.email ?? 'Admin'),
    groups,
  };
}

// ---------------------------------------------------------------------------
// Session cookie (our own short-lived signed JWT, not the IdP token)
// ---------------------------------------------------------------------------
export async function createSessionToken(session: AdminSession): Promise<string> {
  return new SignJWT({ role: 'admin', ...session })
    .setProtectedHeader({ alg: ALG })
    .setIssuedAt()
    .setExpirationTime('8h')
    .sign(secret());
}

export async function getSession(token: string | undefined): Promise<AdminSession | null> {
  if (!token) return null;
  try {
    const { payload } = await jwtVerify(token, secret());
    if (payload.role !== 'admin') return null;
    return {
      email: String(payload.email ?? ''),
      name: String(payload.name ?? 'Admin'),
      groups: Array.isArray(payload.groups) ? (payload.groups as string[]) : [],
    };
  } catch {
    return null;
  }
}

export async function isValidSession(token: string | undefined): Promise<boolean> {
  return (await getSession(token)) !== null;
}

// Short-lived signed cookie carrying the OAuth state+nonce across the redirect.
export async function createOAuthState(state: string, nonce: string): Promise<string> {
  return new SignJWT({ state, nonce })
    .setProtectedHeader({ alg: ALG })
    .setExpirationTime('10m')
    .sign(secret());
}

export async function readOAuthState(token: string | undefined) {
  if (!token) return null;
  try {
    const { payload } = await jwtVerify(token, secret());
    return { state: String(payload.state), nonce: String(payload.nonce) };
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// Cookie setters
// ---------------------------------------------------------------------------
const baseCookie = {
  httpOnly: true,
  sameSite: 'lax' as const,
  secure: import.meta.env.PROD,
  path: '/',
};

export function setSessionCookie(cookies: AstroCookies, token: string) {
  cookies.set(SESSION_COOKIE, token, { ...baseCookie, maxAge: 60 * 60 * 8 });
}
export function clearSessionCookie(cookies: AstroCookies) {
  cookies.delete(SESSION_COOKIE, { path: '/' });
}
export function setOAuthCookie(cookies: AstroCookies, token: string) {
  cookies.set(OAUTH_COOKIE, token, { ...baseCookie, maxAge: 600 });
}
export function clearOAuthCookie(cookies: AstroCookies) {
  cookies.delete(OAUTH_COOKIE, { path: '/' });
}
