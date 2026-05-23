# MinIO on Synology

`compose.yaml` runs MinIO on the Synology NAS at **10.41.0.200:9000** (S3
API) and **10.41.0.200:9001** (web console). Data persists on the NAS
under `/volume1/minio`.

## Why this lives outside the cluster

MinIO is the object store for Loki chunks (and potentially other future
tenants). Running it on the NAS rather than in-cluster:

- Keeps log storage durable across cluster rebuilds.
- Uses the NAS's RAID and snapshot story instead of Longhorn replication.
- Avoids stacking chunk cache → Longhorn → CSI on the same path.

## Setup on the Synology

1. Install Container Manager / Docker package via Synology Package Center.
2. SSH to the Synology, drop `compose.yaml` somewhere persistent (e.g.
   `/volume1/docker/minio/`).
3. Set `MINIO_ROOT_USER` and `MINIO_ROOT_PASSWORD` in a `.env` file next to
   the compose, or export them before `docker compose up -d`. The same
   credentials must be stored in Bitwarden as the item referenced by
   `rke2/monitor/loki/external-secret-minio.yaml`.
4. Create the `loki-chunks` bucket via the web UI at `:9001`.

## Consumers

- **Loki** — chunks + ruler. See `rke2/monitor/loki/loki-values.yaml` and
  `rke2/monitor/loki/external-secret-minio.yaml`. Cluster reaches MinIO at
  `http://10.41.0.200:9000` (insecure path-style S3).

## Operational notes

- The compose file uses `restart: unless-stopped` — survives Synology
  reboot.
- Healthcheck hits `/minio/health/live` every 30s.
- Upgrades: bump the image tag in `compose.yaml`, then on the NAS:
  `docker compose pull && docker compose up -d`.
