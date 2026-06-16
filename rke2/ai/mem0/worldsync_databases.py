#!/usr/bin/env python3
"""worldsync_databases — map the homelab's DATABASES into JARVIS unified memory.

This is one of the "world sync" mappers from the JARVIS charter: live homelab
state is *synced into* mem0 so the unified memory stays both unified and fresh.
This mapper writes one concise fact per database datastore: what kind of DB it
is, which namespace/service it lives in, what it's FOR, and what backs its
storage. Inventory + purpose, NOT a dump of the rows inside.

  RE-RUNNABLE BY DESIGN. Run it on a schedule (CronJob) or by hand. It does an
  idempotent UPSERT into mem0: every fact is a single self-contained statement
  prefixed with a stable "[worldsync:db:<key>]" tag. mem0's own extraction layer
  deduplicates equivalent facts, and the stable tag makes a fact greppable and
  lets a future run supersede a stale one rather than pile up duplicates. We
  never freeze live state as durable memory — re-running refreshes it.

DISCOVERY (read-only kubectl, all namespaces):
  - StatefulSets + Deployments whose image/name looks like a database engine
    (postgres, mysql/mariadb, redis/valkey, mongo, qdrant/chroma/weaviate,
    clickhouse, etc.), cross-referenced against...
  - PVCs (backing storage: size + StorageClass), and
  - Services (how the DB is reached: ClusterIP/DNS + port).
  Purpose for each known datastore is curated in PURPOSE below (the part kube
  can't tell you); unknown datastores are still inventoried generically so the
  memory degrades gracefully as the cluster changes.

TARGET: mem0 REST service. POST /add {text, user_id}. Default user_id "jarvis"
(the homelab/world scope — distinct from the per-speaker "owner" scope used by
the brain's jarvis_mem0_mcp.py shim).

  NOTE (2026-06-16): the mem0 server is currently returning HTTP 400
  "Unsupported embedding provider: fastembed" on /add, so writes will fail until
  the server is fixed. This mapper is correct and complete; it reports each
  failed write clearly and exits non-zero so a CronJob surfaces the breakage,
  and it will start persisting the moment the server embeds again. Use
  --dry-run to see exactly what it WOULD write without touching the server.

Local/uncommitted like the rest of JARVIS. Python 3, stdlib only.
"""
from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import urllib.error
import urllib.request

MEM0_URL = os.environ.get(
    "MEM0_URL", "http://jarvis-mem0.ai.svc.cluster.local:8800"
).rstrip("/")
USER_ID = os.environ.get("WORLDSYNC_USER_ID", "jarvis")
HTTP_TIMEOUT = float(os.environ.get("MEM0_HTTP_TIMEOUT", "30"))

# Stable tag that marks every fact this mapper writes, so a fact is greppable in
# the store and a re-run produces the *same* phrasing (idempotent upsert).
TAG = "worldsync:db"

# Substrings that flag a workload/image as a database engine. Lowercased match.
DB_IMAGE_HINTS = (
    "postgres", "postgresql", "mysql", "mariadb", "percona",
    "redis", "valkey", "keydb", "memcached",
    "mongo", "couchdb", "cassandra", "scylla",
    "qdrant", "chroma", "weaviate", "milvus", "pgvector",
    "clickhouse", "influx", "timescale", "cockroach", "neo4j",
    "elasticsearch", "opensearch", "etcd",
    "garage", "minio",  # S3-compatible object stores (primary datastores)
)

# Pure in-memory caches: real datastores but they hold no durable data of their
# own, so we don't try to attach a PVC to them and we label them as caches.
EPHEMERAL_HINTS = ("memcached",)

# Curated purpose for each datastore — the "what it's FOR" that kubectl can't
# infer. Keyed by "<namespace>/<workload-name>". Anything not listed still gets
# inventoried with a generic purpose line, so discovery never silently drops a
# new DB; add a line here to enrich it.
PURPOSE: dict[str, str] = {
    "ai/qdrant":
        "vector store backing the JARVIS mem0 unified memory (embeddings + "
        "memory payloads, collection 'jarvis_mem0')",
    "authentik/authentik-postgresql":
        "primary database for Authentik SSO/identity provider (users, "
        "applications, flows, sessions)",
    "audiomuse/postgres-deployment":
        "AudioMuse music-analysis app database",
    "audiomuse/redis-deployment":
        "AudioMuse cache / task broker (Redis)",
    "chem/chembl-mysql":
        "ChEMBL bioactivity reference database for khemeia cheminformatics "
        "(scaled to 0 when idle)",
    "chem/khemeia-postgres":
        "khemeia control-plane database — DockJob/compute CRD state, results "
        "metadata for the docking pipeline",
    "game-server/cs2-mysql":
        "Counter-Strike 2 modded game-server database (player/match state)",
    "monitor/spotify-postgres":
        "Spotify analytics warehouse (play_events, tracks, artists, library "
        "history) powering the Grafana Spotify dashboards",
    "garage-system/garage":
        "Garage S3-compatible object store — cluster-wide buckets incl. "
        "backups (e.g. spotify-pgbackup); not a relational DB but a primary "
        "datastore",
    "chem/garage":
        "Garage S3-compatible object store scoped to the chem/khemeia namespace "
        "(docking inputs/outputs and artifacts)",
    "monitor/loki-chunks-cache":
        "Memcached chunk cache for Grafana Loki log storage (in-memory query "
        "acceleration, not durable)",
    "monitor/loki-results-cache":
        "Memcached query-results cache for Grafana Loki (in-memory query "
        "acceleration, not durable)",
}

# PVC-only datastores: storage that clearly backs a database but whose workload
# isn't currently running (scaled down / residual). Still worth inventorying so
# the memory knows the data exists. Keyed by "<namespace>/<pvc-name>".
PVC_ONLY_PURPOSE: dict[str, str] = {
    "authentik/redis-data-authentik-redis-master-0":
        "Authentik Redis cache storage (PVC present; redis workload not "
        "currently running)",
}

# SQLite-backed (and other embedded/file-based) datastores. These are real
# durable datastores but the DB engine is *embedded in the application image*,
# so image/name hints can't detect them — we curate them explicitly. Keyed by
# "<namespace>/<workload-name>". 'engine' is how to label it, 'purpose' is the
# what-it's-FOR. The mapper resolves the workload's running state, Service, and
# backing PVC live (same as auto-discovered DBs); only the engine+purpose are
# curated here. Add a line when a new app keeps durable state in its own PVC.
SQLITE_SERVICES: dict[str, dict[str, str]] = {
    "external-secrets/vault-warden": {
        "engine": "SQLite (embedded)",
        "purpose": "Vaultwarden password manager database (vaults, items, "
                   "users) — SQLite file on its data PVC",
    },
    "plex-system/navidrome": {
        "engine": "SQLite (embedded)",
        "purpose": "Navidrome music server database (library index, play "
                   "counts, playlists) — SQLite file on its data PVC",
        # Navidrome also mounts the 25Ti plex-media library volume; that's the
        # music files, NOT the DB. Scope reporting to the DB's own PVC.
        "db_pvc": "navidrome-data",
    },
    "ai/open-webui": {
        "engine": "SQLite (embedded)",
        "purpose": "Open WebUI database (chats, prompts, users, settings for "
                   "the LLM chat front-end) — SQLite file on its data PVC",
    },
    "plex-system/prowlarr": {
        "engine": "SQLite (embedded)",
        "purpose": "Prowlarr indexer-manager database (indexers, app sync, "
                   "history) — SQLite file on its config PVC",
    },
    "monitor/loki": {
        "engine": "Loki (log datastore)",
        "purpose": "Grafana Loki log store — ingests/queries cluster logs; "
                   "chunks live in object storage (Garage), accelerated by the "
                   "loki-*-cache memcacheds",
    },
}


def _kubectl_json(args: list[str]) -> dict:
    """Run a read-only kubectl get ... -o json and parse it."""
    cmd = ["kubectl", *args, "-o", "json"]
    out = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
    if out.returncode != 0:
        raise RuntimeError(f"kubectl failed: {' '.join(cmd)}\n{out.stderr.strip()}")
    return json.loads(out.stdout)


def _images_of(workload: dict) -> list[str]:
    spec = workload.get("spec", {}).get("template", {}).get("spec", {})
    imgs = [c.get("image", "") for c in spec.get("containers", [])]
    imgs += [c.get("image", "") for c in spec.get("initContainers", []) or []]
    return imgs


def _looks_like_db(name: str, images: list[str]) -> str | None:
    """Return the matched engine hint if this workload looks like a DB, else None."""
    hay = " ".join([name.lower(), *[i.lower() for i in images]])
    for hint in DB_IMAGE_HINTS:
        if hint in hay:
            return hint
    return None


def _engine_label(hint: str) -> str:
    norm = {
        "postgresql": "PostgreSQL", "postgres": "PostgreSQL", "pgvector": "PostgreSQL",
        "mysql": "MySQL", "mariadb": "MariaDB", "percona": "MySQL",
        "redis": "Redis", "valkey": "Valkey", "keydb": "KeyDB",
        "memcached": "Memcached", "mongo": "MongoDB",
        "qdrant": "Qdrant (vector)", "chroma": "Chroma (vector)",
        "weaviate": "Weaviate (vector)", "milvus": "Milvus (vector)",
        "clickhouse": "ClickHouse", "influx": "InfluxDB", "timescale": "TimescaleDB",
        "cockroach": "CockroachDB", "neo4j": "Neo4j",
        "elasticsearch": "Elasticsearch", "opensearch": "OpenSearch",
        "etcd": "etcd", "cassandra": "Cassandra", "scylla": "ScyllaDB",
        "couchdb": "CouchDB",
        "garage": "Garage (S3 object store)", "minio": "MinIO (S3 object store)",
    }
    return norm.get(hint, hint)


def _resolve_workload(
    kind: str,
    w: dict,
    svc_by_ns: dict[str, list[dict]],
    pvc_by_ns: dict[str, list[dict]],
    is_ephemeral: bool,
) -> dict:
    """Resolve a workload's live state, Service endpoint, and backing PVC(s).

    Shared by the auto-detected-DB pass and the curated SQLite-services pass so
    both report running state, DNS, and storage identically.
    """
    ns = w["metadata"]["namespace"]
    name = w["metadata"]["name"]

    desired = (w.get("spec", {}) or {}).get("replicas", 0)
    ready = (w.get("status", {}) or {}).get("readyReplicas", 0) or 0
    running = (desired or 0) > 0 and ready > 0

    # Best-effort match a Service in the same ns whose selector targets this
    # workload's pod labels (fall back to name-substring match).
    pod_labels = (
        w.get("spec", {}).get("template", {}).get("metadata", {}).get("labels", {})
        or {}
    )
    svc_match = None
    selector_matches = []
    for s in svc_by_ns.get(ns, []):
        sel = (s.get("spec", {}) or {}).get("selector") or {}
        if sel and all(pod_labels.get(k) == v for k, v in sel.items()):
            selector_matches.append(s)
    if selector_matches:
        # Multiple Services can select the same pods (helm subcharts share
        # labels). Prefer a non-headless Service whose name relates to the
        # workload; else the first non-headless; else the first.
        def _rank(s: dict) -> tuple:
            sn = s["metadata"]["name"]
            cip = (s.get("spec", {}) or {}).get("clusterIP")
            headless = cip in (None, "None")
            name_rel = sn == name or sn.startswith(name) or name.startswith(sn)
            return (0 if name_rel else 1, 1 if headless else 0)
        svc_match = sorted(selector_matches, key=_rank)[0]
    if svc_match is None:
        # Fallback: a Service named exactly after the workload, or after it
        # with a conventional suffix. Skip headless services (clusterIP None)
        # which are for peer discovery, not a stable endpoint.
        for s in svc_by_ns.get(ns, []):
            sn = s["metadata"]["name"]
            clusterip = (s.get("spec", {}) or {}).get("clusterIP")
            if clusterip in (None, "None"):
                continue
            if sn == name or sn.startswith(f"{name}-") or sn == name.replace(
                "-deployment", "-service"
            ):
                svc_match = s
                break

    svc_dns, svc_port = None, None
    if svc_match:
        svc_dns = f"{svc_match['metadata']['name']}.{ns}.svc.cluster.local"
        ports = (svc_match.get("spec", {}) or {}).get("ports") or []
        if ports:
            svc_port = ports[0].get("port")

    # PVC backing this workload — resolved from the workload's OWN volume
    # references (authoritative), not name guessing:
    #   Deployment: spec.template.spec.volumes[].persistentVolumeClaim.claimName
    #   StatefulSet: each volumeClaimTemplate 'X' yields PVCs 'X-<sts>-<ordinal>'.
    # Pure in-memory caches declare no PVC and get none attached.
    claim_names: set[str] = set()
    if not is_ephemeral:
        tspec = w.get("spec", {}).get("template", {}).get("spec", {})
        for vol in tspec.get("volumes", []) or []:
            cn = (vol.get("persistentVolumeClaim") or {}).get("claimName")
            if cn:
                claim_names.add(cn)
        if kind == "statefulset":
            for vct in w.get("spec", {}).get("volumeClaimTemplates", []) or []:
                base = vct["metadata"]["name"]
                # Match any existing PVC of the form "<base>-<name>-<n>".
                for p in pvc_by_ns.get(ns, []):
                    pn = p["metadata"]["name"]
                    if pn.startswith(f"{base}-{name}-"):
                        claim_names.add(pn)

    pvc_descs: list[str] = []
    pvc_index = {p["metadata"]["name"]: p for p in pvc_by_ns.get(ns, [])}
    for cn in sorted(claim_names):
        p = pvc_index.get(cn)
        if not p:
            continue
        cap = (p.get("status", {}) or {}).get("capacity", {}).get("storage")
        sc = (p.get("spec", {}) or {}).get("storageClassName")
        pvc_descs.append(f"{cn} ({cap}, StorageClass {sc})")
    pvc_info = "; ".join(pvc_descs) if pvc_descs else None
    if is_ephemeral and pvc_info is None:
        pvc_info = "in-memory cache (no persistent volume)"

    return {
        "namespace": ns, "name": name, "kind": kind,
        "running": running, "desired": desired, "ready": ready,
        "service_dns": svc_dns, "service_port": svc_port,
        "pvc": pvc_info,
    }


def discover() -> list[dict]:
    """Return a list of discovered datastore dicts (namespace, name, kind, ...)."""
    workloads: list[tuple[str, dict]] = []
    for kind in ("statefulsets", "deployments"):
        for item in _kubectl_json(["get", kind, "-A"]).get("items", []):
            workloads.append((kind[:-1], item))  # 'statefulset' / 'deployment'

    services = _kubectl_json(["get", "svc", "-A"]).get("items", [])
    pvcs = _kubectl_json(["get", "pvc", "-A"]).get("items", [])

    # Index services by namespace for port/selector lookup.
    svc_by_ns: dict[str, list[dict]] = {}
    for s in services:
        svc_by_ns.setdefault(s["metadata"]["namespace"], []).append(s)

    # Index PVCs by namespace.
    pvc_by_ns: dict[str, list[dict]] = {}
    for p in pvcs:
        pvc_by_ns.setdefault(p["metadata"]["namespace"], []).append(p)

    # Index workloads by "<ns>/<name>" for the curated-SQLite second pass.
    workload_index: dict[str, tuple[str, dict]] = {
        f"{w['metadata']['namespace']}/{w['metadata']['name']}": (kind, w)
        for kind, w in workloads
    }

    found: list[dict] = []
    seen_keys: set[str] = set()

    # Pass 1: auto-detect DB engines by workload name / container image.
    for kind, w in workloads:
        ns = w["metadata"]["namespace"]
        name = w["metadata"]["name"]
        images = _images_of(w)
        hint = _looks_like_db(name, images)
        if not hint:
            continue
        key = f"{ns}/{name}"
        if key in seen_keys:
            continue
        seen_keys.add(key)

        is_ephemeral = hint in EPHEMERAL_HINTS
        rec = _resolve_workload(kind, w, svc_by_ns, pvc_by_ns, is_ephemeral)
        rec["key"] = key
        rec["engine"] = _engine_label(hint)
        rec["image"] = images[0] if images else "?"
        found.append(rec)

    # Pass 2: curated SQLite / embedded-DB / log-store services. The DB engine
    # is embedded in the app image, so image hints can't find these — we look up
    # the named workload and resolve its live state/Service/PVC the same way.
    for key, meta in SQLITE_SERVICES.items():
        if key in seen_keys:
            continue
        entry = workload_index.get(key)
        if entry is None:
            continue  # workload not present (renamed/removed) — degrade quietly
        seen_keys.add(key)
        kind, w = entry
        rec = _resolve_workload(kind, w, svc_by_ns, pvc_by_ns, is_ephemeral=False)
        rec["key"] = key
        rec["engine"] = meta["engine"]
        rec["image"] = (_images_of(w) or ["?"])[0]
        rec["curated_purpose"] = meta["purpose"]
        # For embedded-SQLite apps that also mount bulk/media volumes, narrow the
        # reported backing storage to the PVC that actually holds the DB file.
        db_pvc = meta.get("db_pvc")
        if db_pvc and rec.get("pvc"):
            kept = [seg for seg in rec["pvc"].split("; ")
                    if seg.startswith(f"{db_pvc} ")]
            rec["pvc"] = "; ".join(kept) if kept else rec["pvc"]
        found.append(rec)

    # PVC-only datastores (workload not running but data exists).
    for p in pvcs:
        ns = p["metadata"]["namespace"]
        pn = p["metadata"]["name"]
        pvc_key = f"{ns}/{pn}"
        if pvc_key not in PVC_ONLY_PURPOSE:
            continue
        cap = (p.get("status", {}) or {}).get("capacity", {}).get("storage")
        sc = (p.get("spec", {}) or {}).get("storageClassName")
        found.append({
            "key": pvc_key, "namespace": ns, "name": pn, "kind": "pvc-only",
            "engine": "Redis" if "redis" in pn.lower() else "datastore",
            "image": None, "running": False, "desired": 0, "ready": 0,
            "service_dns": None, "service_port": None,
            "pvc": f"{pn} ({cap}, StorageClass {sc})",
        })

    found.sort(key=lambda d: d["key"])
    return found


def to_fact(d: dict) -> str:
    """Render one discovered datastore as a single concise memory fact."""
    if d["kind"] == "pvc-only":
        purpose = PVC_ONLY_PURPOSE.get(d["key"], "datastore (workload not running)")
        return (
            f"[{TAG}:{d['key']}] {d['engine']} storage in namespace "
            f"'{d['namespace']}': {purpose}. Backing storage: {d['pvc']}."
        )

    purpose = d.get("curated_purpose") or PURPOSE.get(
        d["key"],
        f"{d['engine']} database (purpose not yet curated — inventoried "
        f"automatically)",
    )
    parts = [
        f"[{TAG}:{d['key']}] {d['engine']} database '{d['name']}' "
        f"({d['kind']}) in namespace '{d['namespace']}'.",
        f"Purpose: {purpose}.",
    ]
    if d["service_dns"]:
        port = f":{d['service_port']}" if d["service_port"] else ""
        parts.append(f"Reached at {d['service_dns']}{port}.")
    if d["pvc"]:
        parts.append(f"Backing storage: {d['pvc']}.")
    parts.append(
        "Currently running." if d["running"]
        else f"Currently scaled down (desired replicas={d['desired']})."
    )
    return " ".join(parts)


def mem0_add(text: str) -> tuple[bool, str]:
    payload = json.dumps({"text": text, "user_id": USER_ID}).encode()
    req = urllib.request.Request(
        f"{MEM0_URL}/add", data=payload,
        headers={"Content-Type": "application/json"}, method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as r:
            body = r.read().decode() or "{}"
        return True, body
    except urllib.error.HTTPError as exc:
        return False, f"HTTP {exc.code}: {exc.read().decode(errors='replace')[:300]}"
    except urllib.error.URLError as exc:
        return False, f"unreachable: {exc}"


def main() -> int:
    global USER_ID
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--dry-run", action="store_true",
                    help="print the facts that would be written; do not POST")
    ap.add_argument("--user-id", default=USER_ID,
                    help=f"mem0 partition key (default: {USER_ID})")
    args = ap.parse_args()
    USER_ID = args.user_id

    try:
        stores = discover()
    except Exception as exc:  # noqa: BLE001
        print(f"discovery failed: {exc}", file=sys.stderr)
        return 2

    print(f"Discovered {len(stores)} datastore(s) across the cluster.\n")
    facts = [to_fact(d) for d in stores]

    if args.dry_run:
        for f in facts:
            print("  " + f + "\n")
        print(f"[dry-run] would write {len(facts)} fact(s) to "
              f"{MEM0_URL}/add (user_id={USER_ID}).")
        return 0

    ok = 0
    failures = 0
    for d, f in zip(stores, facts):
        success, detail = mem0_add(f)
        if success:
            ok += 1
            print(f"  [ok]   {d['key']}")
        else:
            failures += 1
            print(f"  [FAIL] {d['key']}: {detail}")

    print(f"\nwrote {ok}/{len(facts)} fact(s) to mem0 (user_id={USER_ID}); "
          f"{failures} failed.")
    if failures:
        print("note: if failures are 'Unsupported embedding provider: fastembed', "
              "the mem0 server is broken — fix the embedding config and re-run. "
              "This mapper is idempotent and safe to re-run.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
