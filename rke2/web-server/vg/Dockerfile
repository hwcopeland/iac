# Valley Growers CMS → Astro SSR (Node) server. Unlike the static blog, this app
# is dynamic (SQLite-backed admin), so it runs the Node adapter output directly
# rather than being served by nginx. Build runs on the ARC cluster runner; base
# images come from the private zot mirror.
FROM zot.hwcopeland.net/library/node:22-slim AS builder
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build && npm prune --omit=dev

FROM zot.hwcopeland.net/library/node:22-slim
ENV NODE_ENV=production \
    HOST=0.0.0.0 \
    PORT=4321 \
    DATA_DIR=/data
WORKDIR /app
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/package.json ./package.json
# SQLite db + uploaded images persist here (a PVC in the cluster).
RUN mkdir -p /data && chown -R node:node /data
USER node
EXPOSE 4321
CMD ["node", "./dist/server/entry.mjs"]
