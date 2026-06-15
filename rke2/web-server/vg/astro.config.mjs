// @ts-check
import { defineConfig } from 'astro/config';
import node from '@astrojs/node';

// Valley Growers is a DB-backed CMS, so it runs as a Node server (SSR) rather
// than a static export like the blog. The Node adapter emits
// dist/server/entry.mjs, which the container runs directly.
export default defineConfig({
  site: 'https://vg.hwcopeland.net',
  output: 'server',
  adapter: node({ mode: 'standalone' }),
  server: { host: true, port: 4321 },
  devToolbar: { enabled: false },
  // TLS is terminated at the gateway, so the Node server sees http:// while the
  // browser sends an https Origin — Astro's default origin check would reject
  // every form POST ("Cross-site POST forbidden"). The admin is protected by
  // Authentik SSO + httpOnly SameSite cookies, so we disable the built-in check.
  security: { checkOrigin: false },
});
