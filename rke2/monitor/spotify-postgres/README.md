# spotify-postgres

Dedicated Postgres 16 data layer for the Spotify analytics pipeline, in the
`monitor` namespace. The `spotify-exporter` writes here; Grafana reads via a
read-only role.

## Components

| File | What |
|---|---|
| `secret.yaml` | DB credentials (`spotify-postgres`) + Garage S3 creds for backups (`spotify-pgbackup-garage`). Direct Secrets, same precedent as `loki-garage-secret.yaml`. |
| `postgres.yaml` | `postgres:16` Deployment (Recreate, 1 replica), Longhorn RWO 10Gi PVC tagged into the `b2-backup` group, ClusterIP Service on 5432. |
| `migration.yaml` | Idempotent DDL in a ConfigMap + a one-shot Job that applies it and creates the `spotify_app` / `grafana_ro` roles. Migration 001 = base schema; 002 = MusicBrainz genre enrichment (`music_ids`, `track_genres`, `artist_genres_enriched`, `track_genre_effective` fine sub-genre view). |
| `cronjob-enrich.yaml` | Hourly MusicBrainz genre enrichment (reuses the `spotify-exporter` image, runs `enrich.py`). **No API key / signup** — MusicBrainz needs only a descriptive User-Agent + 1 req/s. Fills fine sub-genres for tracks Spotify leaves `untagged` (old/catalog music). |
| `cronjob-backup.yaml` | Weekly `pg_dump` (custom format) → Garage bucket `spotify-pgbackup`, Sundays 05:00. |
| `grafana-datasource.yaml` | Provisioned Grafana Postgres datasource (`uid: spotify-postgres`) via the datasource sidecar, connecting as `grafana_ro`. |

## Connection details

- Service: `spotify-postgres.monitor.svc.cluster.local:5432`
- Database: `spotify`
- Roles:
  - `postgres` — superuser (container bootstrap only)
  - `spotify_app` — read/write, used by the exporter
  - `grafana_ro` — SELECT-only, used by the Grafana datasource

## Deploy

```sh
kubectl apply -f secret.yaml
kubectl apply -f postgres.yaml
kubectl -n monitor rollout status deploy/spotify-postgres
kubectl apply -f migration.yaml
kubectl -n monitor wait --for=condition=complete job/spotify-postgres-migrate-002 --timeout=180s
kubectl apply -f cronjob-backup.yaml
kubectl apply -f cronjob-enrich.yaml
kubectl apply -f grafana-datasource.yaml
```

The `build-spotify-exporter` workflow's deploy job applies `migration.yaml` and
`cronjob-enrich.yaml` automatically on a push to `main` that touches them (and
kicks one bootstrap enrichment run), so no manual step is needed for the genre
pipeline. MusicBrainz enrichment requires **no API key** — it runs immediately.

Adding a new migration: append a guarded block to a new `00N_*.sql` key, bump
the `schema_migrations` version, and rename the Job (`-migrate-00N`) so `apply`
doesn't hit the immutable-Job error.

## Backups

1. **Longhorn snapshots** — daily 03:00, whole-cluster `default` group (local, CoW).
2. **Longhorn B2 backup** — daily 04:00, the PVC is in the curated `b2-backup` group.
3. **pg_dump → Garage** — weekly logical dump (transactionally consistent;
   the only restore source that's guaranteed-consistent for a live DB).

Restore a logical dump:
```sh
aws --endpoint-url http://garage.garage-system.svc.cluster.local:3900 \
    s3 cp s3://spotify-pgbackup/dumps/<file>.dump ./
pg_restore -h spotify-postgres -U postgres -d spotify --clean <file>.dump
```

## Credential rotation

DB passwords are generated at deploy and live in `secret.yaml` (not Bitwarden).
To rotate: `ALTER ROLE <role> WITH PASSWORD '<new>'`, base64 into `secret.yaml`
(and the inline value in `grafana-datasource.yaml` for `grafana_ro`), re-apply,
restart consumers. Garage key: `garage key rotate spotify-pgbackup-app`.
