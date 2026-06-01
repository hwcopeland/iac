# Astro blog → static HTML served by nginx.
# Build runs on the ARC cluster runner (never on the Mac); base images come
# from the private zot mirror. Output of `astro build` lands in /app/dist.
FROM zot.hwcopeland.net/library/node:22-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build

FROM zot.hwcopeland.net/library/nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
