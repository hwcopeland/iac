# cloudflare-exporter

Exposes Cloudflare **edge analytics** as Prometheus metrics, scraped by the
kube-prometheus-stack. Powers per-domain (per-zone) dashboards: request volume,
status-code breakdown, country/geo geomap, cached-vs-uncached, bandwidth, and
Cloudflare's own edge-computed threat aggregates.

- **Data source:** Cloudflare **GraphQL Analytics API** (works on Free/Pro).
  We do **not** use Logpush (Enterprise-only).
- **Image:** `ghcr.io/lablabs/cloudflare_exporter:0.2.3` — the maintained
  community fork. Default listen `:8080`, metrics at `/metrics`.
- **Plan note:** firewall/threat metrics work on Pro but are **sampled**.

## Files

| File | What |
|---|---|
| `cloudflare-exporter.yaml` | Deployment + Service + ServiceMonitor |
| `external-secret.yaml` | ESO ExternalSecret pulling the CF API token from Bitwarden into the `cloudflare-exporter` Secret (key `api-token`) |

## Manual steps (token — user does this)

1. **Create the Cloudflare API token** (Cloudflare dash → My Profile → API
   Tokens → Create Token → custom). Scopes:
   - `Account · Account Analytics : Read`
   - `Zone · Analytics : Read`
   - `Zone · Firewall Services : Read`  *(zone rule names for firewall events)*
   - `Account · Account Rulesets : Read` *(account rule names for firewall events)*

   Account Resources → Include → your account.
   Zone Resources → Include → All zones (or pin the three live zones:
   `hwcopeland.net`, `khemeia.net`, `flmanbiosci.net`).
2. **Store it in Bitwarden** as a login item named `cloudflare-exporter`, token
   pasted into the **password** field (the `bitwarden-login` store only exposes
   login fields). Get the UUID: `bw get item cloudflare-exporter | jq -r .id`.
3. **Wire the UUID:** replace `REPLACE-WITH-BW-ITEM-UUID` in
   `external-secret.yaml` with that UUID.

### Zone list

All three zones exist as `cloudflare-operator.io/v1 Zone` CRs in
`rke2/kube-system/` (`cf-account.yaml`, `khemeia-zone.yaml`,
`flmanbiosci-zone.yaml`). With `CF_ZONES` **unset**, the exporter scrapes every
zone on the account automatically — no per-zone wiring needed. To pin, set
`CF_ZONES` to the comma-separated zone IDs in the Deployment env.

## Apply + verify

```bash
# diff / apply (mutation — get confirmation first)
kubectl apply --dry-run=server -f rke2/monitor/cloudflare-exporter/
kubectl apply -f rke2/monitor/cloudflare-exporter/

# secret materialized from Bitwarden?
kubectl get externalsecret -n monitor cloudflare-exporter
kubectl get secret -n monitor cloudflare-exporter -o jsonpath='{.data.api-token}' | wc -c

# pod healthy + metrics flowing
kubectl rollout status -n monitor deploy/cloudflare-exporter
kubectl exec -n monitor deploy/cloudflare-exporter -- wget -qO- http://localhost:8080/metrics | grep -c '^cloudflare_'

# Prometheus picked up the target
kubectl exec -n monitor prometheus-kube-prometheus-stack-prometheus-0 -c prometheus \
  -- wget -qO- 'http://localhost:9090/api/v1/targets?state=active' \
  | jq '.data.activeTargets[] | select(.labels.job=="cloudflare-exporter") | {instance,health}'
```

## Exposed metrics (for dashboard authors)

Zone-level (label `zone` = domain name):
- `cloudflare_zone_requests_total`, `cloudflare_zone_requests_cached`,
  `cloudflare_zone_requests_ssl_encrypted`, `cloudflare_zone_pageviews_total`,
  `cloudflare_zone_uniques_total`
- `cloudflare_zone_requests_status` (label `status`) — status-code breakdown
- `cloudflare_zone_requests_country` / `cloudflare_zone_bandwidth_country`
  (label `country`) — geomap source
- `cloudflare_zone_requests_status_country_host`,
  `cloudflare_zone_requests_origin_status_country_host` (+ `_p50_ms/_p95_ms/_p99_ms`)
- `cloudflare_zone_requests_content_type` / `cloudflare_zone_bandwidth_content_type`
- `cloudflare_zone_bandwidth_total`, `_cached`, `_ssl_encrypted`
- `cloudflare_zone_threats_total`, `cloudflare_zone_threats_country` — CF threat aggregates
- `cloudflare_zone_firewall_events_count` (labels for action/source/rule) — firewall/bot events
- `cloudflare_zone_requests_browser_map_page_views_count`
- Colocation: `cloudflare_zone_colocation_requests_total`, `_visits`,
  `_edge_response_bytes`
- Load balancer pools: `cloudflare_zone_pool_health_status`, `_requests_total`
- `cloudflare_logpush_failed_jobs_zone_count` (no-op for us — not on Logpush)

Worker metrics (account-level) also exposed if any Workers exist.
