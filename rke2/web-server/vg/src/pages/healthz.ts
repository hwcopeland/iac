import type { APIRoute } from 'astro';

export const prerender = false;

export const GET: APIRoute = () =>
  new Response('ok\n', { status: 200, headers: { 'content-type': 'text/plain' } });
