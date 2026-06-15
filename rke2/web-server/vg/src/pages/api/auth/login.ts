import type { APIRoute } from 'astro';
import { buildAuthorizeUrl, createOAuthState, setOAuthCookie } from '@/lib/auth';

export const prerender = false;

function redirectUri(site: URL | undefined, origin: string): string {
  if (process.env.OIDC_REDIRECT_URI) return process.env.OIDC_REDIRECT_URI;
  return new URL('/api/auth/callback', site ?? origin).toString();
}

export const GET: APIRoute = async ({ site, url, cookies, redirect }) => {
  const { url: authUrl, state, nonce } = await buildAuthorizeUrl(redirectUri(site, url.origin));
  setOAuthCookie(cookies, await createOAuthState(state, nonce));
  return redirect(authUrl);
};
