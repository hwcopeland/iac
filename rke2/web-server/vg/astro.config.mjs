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
});
