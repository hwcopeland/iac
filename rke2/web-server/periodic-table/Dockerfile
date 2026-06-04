# Vite/TS periodic table → static assets served by nginx.
# Build runs on the ARC cluster runner (never on the Mac); base images come
# from the private zot mirror. `npm run build` type-checks then emits /app/dist.
FROM zot.hwcopeland.net/library/node:22-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY . .
# Quality gate: type-check + unit tests must pass before the image is produced.
RUN npm test
RUN npm run build

FROM zot.hwcopeland.net/library/nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
