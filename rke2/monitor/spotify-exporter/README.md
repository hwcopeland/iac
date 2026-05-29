# spotify-exporter

A standalone Spotify → Prometheus exporter for the `monitor` namespace, with a
provisioned Grafana dashboard. **Independent of JARVIS** — it reuses none of the
openjarvis runtime; only the OAuth concept (PKCE refresh token) is shared.

## What it exports

Polled on a background loop and served at `:9112/metrics`:

| Metric | Meaning |
|---|---|
| `spotify_up` | 1 if the last poll cycle succeeded |
| `spotify_now_playing` / `_progress_ratio` / `_info{track,artist,album}` | live playback |
| `spotify_top_artist_rank{range,artist,genre}` | top artists, 1 = most played |
| `spotify_artist_popularity{range,artist}` | Spotify popularity 0–100 |
| `spotify_top_track_rank{range,track,artist}` | top tracks |
| `spotify_genre_score{range,genre}` | rank-weighted genre share (%) |
| `spotify_plays_total{artist}` | plays observed via recently-played (incl. `artist="__all__"`) |
| `spotify_scrape_errors_total` | exporter health |

`range` ∈ `short` (~4w) / `medium` (~6mo) / `long` (years).

## How it's wired

- **Image**: `zot.hwcopeland.net/monitor/spotify-exporter`, built on the
  `arc-chem` runner by `.github/workflows/build-spotify-exporter.yml`.
- **Deploy**: the workflow's `deploy` job (`kubectl apply` + rollout) — not Flux,
  since the `monitor-extended` Flux Kustomization is currently broken.
- **Scrape**: `PodMonitor` labeled `release: kube-prometheus-stack`.
- **Dashboard**: `dashboard.yaml` ConfigMap, picked up by the Grafana sidecar
  into the "Spotify" folder (datasource uid `prometheus`).
- **State**: a 64Mi `longhorn` PVC at `/state` persists cumulative play counts
  across restarts.

## Credentials — shared token, one Spotify app

JARVIS and this exporter share a **single Spotify app** (one PKCE
authorization). Spotify rotates the refresh token on every refresh and revokes
the old one, so two independent refreshers would continually break each other.
To avoid that, the token lives in **one** place that both read and write:

> **Source of truth: Secret `monitor/spotify-token`** (key `tokens.json`).
> Both sides follow the same protocol — read latest → use the access token if
> still valid → only refresh and write back when expired → on `invalid_grant`,
> re-read the peer's freshly-written token. Refreshes happen ~once/hour, so the
> only conflict window is two refreshes within a few seconds, which self-heals
> on the next read.
>
> - The **exporter** reads/patches the Secret via its ServiceAccount + Role
>   (`tokenstore.py`, in-cluster API).
> - **JARVIS** reads/patches the same Secret via `kubectl`
>   (`jarvis_spotify.py`), with a local-file fallback when the cluster is
>   unreachable.

`tokenstore.py` and `jarvis_spotify.py` implement the identical protocol.

### Bootstrap (Bitwarden → ExternalSecret)

The bootstrap creds come from **Bitwarden**, not GitHub secrets.
`external-secret.yaml` (ClusterSecretStore `bitwarden-login`) pulls them into
the `spotify-exporter` Secret:

| Secret key | Bitwarden field | Value |
|---|---|---|
| `client_id` | `login.username` | Spotify PKCE client_id |
| `refresh_token` | `login.password` | cold-start refresh token |

Bitwarden item: **`spotify-exporter`** (`8e8f830d-f56a-4e8a-9285-245fdc880a3b`).
The store's webhook only exposes login fields (`$.data.login.<property>`), hence
the username/password mapping.

The live `spotify-token` Secret is created on first write by whichever side runs
first — typically JARVIS, which writes it whenever it refreshes (incl. on
`jarvis_spotify.py auth`). The exporter only falls back to the Bitwarden
bootstrap `refresh_token` if `spotify-token` is empty.

> If the shared token is ever revoked (e.g. you remove the app's access in
> Spotify), re-run the JARVIS auth flow once:
> `python3 ~/openjarvis/jarvis_spotify.py auth` — the new token is written to
> both the local file and `monitor/spotify-token`, and the exporter picks it up
> on its next poll.
