# surf.hwcopeland.net — CS2 surf leaderboard

Read-only web leaderboard for the CS2 surf server. Reads the `source2surf`
MySQL database that the in-pod **Source2Surf.Timer** plugin writes to.

## Components

| Path | What it is |
|---|---|
| `api/`  | FastAPI service (`surf-api`) — read-only SQL → JSON, Steam avatar enrichment. |
| `web/`  | SvelteKit static site (`surf-web`) — served by nginx, proxies `/api/*` to `surf-api`. |
| `k8s/`  | Deployments, Services, DNSRecord + HTTPRoute for `surf.hwcopeland.net`. |
| `../../../../.github/workflows/build-surf-web.yml` | GHA pipeline: builds both images on push, restarts deployments. |

## Secrets (already provisioned)

Two `Opaque` Secrets in the `game-server` namespace:

- **`surf-web-mysql`** — keys: `host`, `port`, `database`, `username`, `password`.
  Bound to a dedicated read-only MySQL user: `surfweb_ro`, scoped
  `GRANT SELECT ON source2surf.*`.
- **`surf-web-steam`** — key: `api_key`. Copied from `cs2-secret.API_KEY`.

To rotate the read-only user:

```sh
kubectl exec -n game-server cs2-mysql-... -- mysql -uroot -p \
  -e "ALTER USER 'surfweb_ro'@'%' IDENTIFIED BY 'NEWPW';"
kubectl -n game-server create secret generic surf-web-mysql \
  --from-literal=host=10.43.43.43 \
  --from-literal=port=3306 \
  --from-literal=database=source2surf \
  --from-literal=username=surfweb_ro \
  --from-literal=password='NEWPW' \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n game-server rollout restart deployment/surf-api
```

## Local dev

```sh
# API (needs the mysql secret values exported as env)
cd api
export SURFWEB_MYSQL_HOST=10.43.43.43
export SURFWEB_MYSQL_PASSWORD=...
export SURFWEB_STEAM_API_KEY=...
pip install -e .
uvicorn app.main:app --reload --port 8080

# Web (proxies /api → http://localhost:8080)
cd web
npm install
npm run dev
```

## Endpoints

- `GET /api/server` — totals (players, runs, maps)
- `GET /api/players?limit=&offset=&search=`
- `GET /api/players/{steamid64}`
- `GET /api/maps`
- `GET /api/maps/{map_file}`
- `GET /api/records/recent?limit=`
- `GET /api/records/wr?limit=`
- `GET /healthz`

## Notes

- `nginx` proxies `/api/*` so the browser only ever talks to one origin —
  no CORS preflight in production. The FastAPI `cors_origins` setting is
  defensive: it only matters if someone hits `surf-api` directly.
- The MySQL user has `SELECT` only — even a full SQLi against `surf-api`
  cannot tamper with timer data.
- Steam avatars are looked up in batches of 100 and cached in-memory for
  24 h; first cold load after pod restart will be slower.
