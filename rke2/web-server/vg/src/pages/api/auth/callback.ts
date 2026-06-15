import type { APIRoute } from 'astro';
import {
  readOAuthState,
  exchangeCode,
  verifyIdToken,
  createSessionToken,
  setSessionCookie,
  clearOAuthCookie,
  OAUTH_COOKIE,
} from '@/lib/auth';

export const prerender = false;

function redirectUri(site: URL | undefined, origin: string): string {
  if (process.env.OIDC_REDIRECT_URI) return process.env.OIDC_REDIRECT_URI;
  return new URL('/api/auth/callback', site ?? origin).toString();
}

export const GET: APIRoute = async ({ site, url, cookies, redirect }) => {
  const code = url.searchParams.get('code');
  const state = url.searchParams.get('state');
  const saved = await readOAuthState(cookies.get(OAUTH_COOKIE)?.value);
  clearOAuthCookie(cookies);

  if (url.searchParams.get('error')) {
    return redirect(`/admin/login?err=${encodeURIComponent(url.searchParams.get('error')!)}`);
  }
  if (!code || !state || !saved || saved.state !== state) {
    return redirect('/admin/login?err=bad_state');
  }

  try {
    const idToken = await exchangeCode(code, redirectUri(site, url.origin));
    const session = await verifyIdToken(idToken, saved.nonce);
    setSessionCookie(cookies, await createSessionToken(session));
    return redirect('/admin');
  } catch (e) {
    const code = (e as { code?: string }).code;
    if (code === 'NOT_AUTHORIZED') return redirect('/admin/login?err=forbidden');
    return redirect('/admin/login?err=auth_failed');
  }
};
