import type { APIRoute } from 'astro';
import { createReadStream, existsSync, statSync } from 'node:fs';
import { join, normalize, extname } from 'node:path';
import { UPLOAD_DIR } from '@/db';

export const prerender = false;

const TYPES: Record<string, string> = {
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.png': 'image/png',
  '.webp': 'image/webp',
  '.gif': 'image/gif',
  '.avif': 'image/avif',
  '.svg': 'image/svg+xml',
};

// Serves user-uploaded images from the data volume. Guards against path
// traversal by resolving inside UPLOAD_DIR and rejecting anything that escapes.
export const GET: APIRoute = ({ params }) => {
  const rel = normalize(params.path ?? '').replace(/^(\.\.(\/|\\|$))+/, '');
  const full = join(UPLOAD_DIR, rel);
  if (!full.startsWith(UPLOAD_DIR) || !existsSync(full) || !statSync(full).isFile()) {
    return new Response('Not found', { status: 404 });
  }
  const stream = createReadStream(full) as unknown as ReadableStream;
  return new Response(stream, {
    headers: {
      'content-type': TYPES[extname(full).toLowerCase()] ?? 'application/octet-stream',
      'cache-control': 'public, max-age=86400',
    },
  });
};
