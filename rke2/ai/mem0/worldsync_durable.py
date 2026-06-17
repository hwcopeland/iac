#!/usr/bin/env python3
"""worldsync_durable — the nightly ANTI-DRIFT sync for JARVIS unified memory.

This is the redesigned, DURABLE + RECONCILING world-sync mapper. It replaces the
old per-domain mappers (worldsync_cluster / worldsync_databases / worldsync_metrics)
whose facts drifted because they captured VOLATILE readiness ("3/3 ready",
"running on node X", "scaled to 0", "CPU is 82%"). Per the JARVIS charter's
architecture rule:

    mem0 holds DURABLE knowledge only — what a service/project IS, its purpose,
    stable config, and status. LIVE/VOLATILE state (pod readiness, replica
    counts, what's on which node, CPU/GPU%, firing alerts) is NEVER written to
    mem0 — the brain answers those live via cluster_overview / kube tools.
    Never freeze live state as durable memory; it goes stale the instant
    anything changes.

So this sync captures ONLY stable structure:

  CLUSTER  — node count by role, total CPU cores, GPU inventory (model/count),
             namespace inventory. (Counts of nodes/namespaces are topology, not
             second-to-second readiness.)
  NODE     — name, role, capacity (CPU/RAM/GPU model), OS, and curated PURPOSE
             (what the node is FOR). NOT its Ready condition.
  WORKLOAD — kind, namespace, primary image, whether it is GPU-bound, where it
             is PINNED (nodeSelector — a deliberate, durable config), and curated
             PURPOSE. NOT ready/desired replica counts, NOT actual pod placement,
             NOT "scaled to 0".
  DATABASE — engine, namespace, purpose, stable Service DNS endpoint, backing
             PVC (size + StorageClass — durable storage facts). NOT "currently
             running / scaled down".

It does NOT touch metrics/health at all — those are inherently "now" and belong
to live tools, not memory.

RECONCILING (the anti-drift core)
---------------------------------
Every fact is written through mem0 /add with a `metadata` payload that mem0
stores FLAT in the qdrant point:

    {"source": "worldsync", "domain": "<cluster|node|workload|db>",
     "wskey": "<stable-entity-key>", "worldsync_run": "<this-run-id>"}

After a run successfully writes every current fact, it RECONCILES: it deletes,
directly from qdrant, every point tagged source=="worldsync" whose
worldsync_run != this run's id — i.e. facts for entities that were NOT re-seen
this run because the underlying resource was removed/renamed. That keeps the
memory from accumulating ghosts of deleted services. Reconciliation is SKIPPED
if any write failed this run (a partial run must not delete still-valid facts).

IDEMPOTENT: re-running rewrites the same facts (mem0 dedupes equivalent extracted
facts by hash) and re-stamps them with the new run id; reconciliation then prunes
only what truly vanished. Safe to run on a schedule or by hand any number of
times.

DISCOVERY transport
-------------------
Reads the cluster via the in-cluster Kubernetes API (https://kubernetes.default)
using the mounted ServiceAccount token — no kubectl binary required, so this runs
on a stock python image. Outside a pod (e.g. a Mac with a kubeconfig) it falls
back to shelling out to `kubectl ... -o json` so --dry-run works locally.

TARGETS (in-cluster ClusterIP DNS):
  mem0 REST : http://jarvis-mem0.ai.svc.cluster.local:8800  (POST /add)
  qdrant    : http://qdrant.ai.svc.cluster.local:6333       (scroll + delete)
  user_id   : "jarvis" (the shared WORLD scope every speaker's brain can read)

Python 3, stdlib only. Local/uncommitted like the rest of JARVIS.
"""
from __future__ import annotations

import argparse
import json
import os
import ssl
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone

# ── Config (sensible in-cluster defaults; all overridable by env) ────────────
MEM0_URL = os.environ.get(
    "MEM0_URL", "http://jarvis-mem0.ai.svc.cluster.local:8800"
).rstrip("/")
QDRANT_URL = os.environ.get(
    "QDRANT_URL", "http://qdrant.ai.svc.cluster.local:6333"
).rstrip("/")
QDRANT_COLLECTION = os.environ.get("QDRANT_COLLECTION", "jarvis_mem0")
USER_ID = os.environ.get("WORLDSYNC_USER_ID", "jarvis")
HTTP_TIMEOUT = float(os.environ.get("MEM0_HTTP_TIMEOUT", "120"))

# Marks every point this sync owns. Reconciliation only ever touches points
# carrying this source value, so it can never delete owner memories or seeds.
SOURCE = "worldsync"

# In-cluster Kubernetes API access (ServiceAccount token + CA).
K8S_API = os.environ.get("KUBERNETES_API", "https://kubernetes.default.svc")
_SA = "/var/run/secrets/kubernetes.io/serviceaccount"


# ── Curated "what it's FOR" — the durable knowledge kube can't infer ─────────
# Keyed by node name. Unknown nodes still get a generic inventory line.
NODE_PURPOSE: dict[str, str] = {
    "nixos-gpu":
        "the single GPU box (RTX 3070, time-sliced 10x, ~8GB VRAM). Runs the AI "
        "GPU workloads (ollama, jarvis voice STT/TTS) AND khemeia/chem GPU jobs "
        "- GPU is shared and contended. NixOS, so pods need "
        "/run/opengl-driver + /nix/store host mounts; host->pod routing is "
        "broken (Cilium) so GPU pods use hostNetwork",
    "microedge":
        "general worker; hosts the CPU-side JARVIS pieces (mem0 unified-memory "
        "backend, qdrant, open-webui) that intentionally avoid the GPU node",
    "microedge2":
        "general worker; pinned home for the Klipper 3D-printer stack and Home "
        "Assistant (needs the printer's USB serial + camera devices)",
    "forum01": "general worker (high-RAM)",
    "forum02": "general worker (high-RAM)",
    "ctlpln1": "control-plane + etcd member",
    "ctlpln2": "control-plane + etcd member",
    "ctlpln3": "control-plane + etcd member",
}

# Keyed by "<namespace>/<name>". Everything else is inventoried generically.
WORKLOAD_PURPOSE: dict[str, str] = {
    "ai/jarvis-stack":
        "the JARVIS voice brain pod: whisper STT + chatterbox TTS (both GPU) + "
        "jarvis-edge (CPU) - the front door for voice interaction",
    "ai/jarvis-mem0":
        "the mem0 unified-memory REST backend - the spine this very sync writes "
        "into",
    "ai/ollama":
        "local LLM inference (qwen models) on the RTX 3070; fallback brain",
    "ai/open-webui": "web chat UI over ollama (chat.hwcopeland.net)",
    "ai/homelab-bot": "Discord bot (Grok primary, ollama fallback)",
}

# Database purpose, keyed by "<namespace>/<workload-name>".
DB_PURPOSE: dict[str, str] = {
    "ai/qdrant":
        "vector store backing the JARVIS mem0 unified memory (embeddings + "
        "memory payloads, collection 'jarvis_mem0')",
    "authentik/authentik-postgresql":
        "primary database for Authentik SSO/identity provider (users, "
        "applications, flows, sessions)",
    "audiomuse/postgres-deployment": "AudioMuse music-analysis app database",
    "audiomuse/redis-deployment": "AudioMuse cache / task broker (Redis)",
    "chem/chembl-mysql":
        "ChEMBL bioactivity reference database for khemeia cheminformatics",
    "chem/khemeia-postgres":
        "khemeia control-plane database - DockJob/compute CRD state, results "
        "metadata for the docking pipeline",
    "game-server/cs2-mysql":
        "Counter-Strike 2 modded game-server database (player/match state)",
    "monitor/spotify-postgres":
        "Spotify analytics warehouse (play_events, tracks, artists, library "
        "history) powering the Grafana Spotify dashboards",
    "garage-system/garage":
        "Garage S3-compatible object store - cluster-wide buckets incl. "
        "backups (e.g. spotify-pgbackup); a primary datastore, not relational",
    "chem/garage":
        "Garage S3-compatible object store scoped to the chem/khemeia namespace "
        "(docking inputs/outputs and artifacts)",
}

# Embedded/file-based datastores image hints can't detect; curated explicitly.
# Keyed by "<ns>/<workload>". Resolved for Service/PVC live like auto-detected.
SQLITE_SERVICES: dict[str, dict[str, str]] = {
    "external-secrets/vault-warden": {
        "engine": "SQLite (embedded)",
        "purpose": "Vaultwarden password manager database (vaults, items, "
                   "users) - SQLite file on its data PVC",
    },
    "plex-system/navidrome": {
        "engine": "SQLite (embedded)",
        "purpose": "Navidrome music server database (library index, play "
                   "counts, playlists) - SQLite file on its data PVC",
        "db_pvc": "navidrome-data",
    },
    "ai/open-webui": {
        "engine": "SQLite (embedded)",
        "purpose": "Open WebUI database (chats, prompts, users, settings for "
                   "the LLM chat front-end) - SQLite file on its data PVC",
    },
    "plex-system/prowlarr": {
        "engine": "SQLite (embedded)",
        "purpose": "Prowlarr indexer-manager database (indexers, app sync, "
                   "history) - SQLite file on its config PVC",
    },
}

# Image/name substrings that flag a workload as a database engine.
DB_IMAGE_HINTS = (
    "postgres", "postgresql", "mysql", "mariadb", "percona",
    "redis", "valkey", "keydb", "mongo", "couchdb", "cassandra", "scylla",
    "qdrant", "chroma", "weaviate", "milvus", "pgvector",
    "clickhouse", "influx", "timescale", "cockroach", "neo4j",
    "elasticsearch", "opensearch", "garage", "minio",
)
# Memcached is a pure in-memory cache (no durable data of its OWN) — we do not
# treat it as a durable datastore worth a memory fact.
_ENGINE_LABEL = {
    "postgresql": "PostgreSQL", "postgres": "PostgreSQL", "pgvector": "PostgreSQL",
    "mysql": "MySQL", "mariadb": "MariaDB", "percona": "MySQL",
    "redis": "Redis", "valkey": "Valkey", "keydb": "KeyDB", "mongo": "MongoDB",
    "qdrant": "Qdrant (vector)", "chroma": "Chroma (vector)",
    "weaviate": "Weaviate (vector)", "milvus": "Milvus (vector)",
    "clickhouse": "ClickHouse", "influx": "InfluxDB", "timescale": "TimescaleDB",
    "cockroach": "CockroachDB", "neo4j": "Neo4j",
    "elasticsearch": "Elasticsearch", "opensearch": "OpenSearch",
    "cassandra": "Cassandra", "scylla": "ScyllaDB", "couchdb": "CouchDB",
    "garage": "Garage (S3 object store)", "minio": "MinIO (S3 object store)",
}


# ── Kubernetes read transport (in-cluster API, kubectl fallback) ─────────────
def _in_cluster() -> bool:
    return os.path.exists(f"{_SA}/token")


def _k8s_get(path: str) -> dict:
    """GET a list resource from the in-cluster API; fall back to kubectl off-cluster.

    `path` is the API path, e.g. "/api/v1/nodes" or
    "/apis/apps/v1/deployments". Returns the parsed JSON (a List object).
    """
    if _in_cluster():
        with open(f"{_SA}/token", encoding="utf-8") as fh:
            token = fh.read().strip()
        ctx = ssl.create_default_context(cafile=f"{_SA}/ca.crt")
        req = urllib.request.Request(
            f"{K8S_API}{path}", headers={"Authorization": f"Bearer {token}"}
        )
        with urllib.request.urlopen(req, context=ctx, timeout=HTTP_TIMEOUT) as r:
            return json.loads(r.read() or b"{}")
    # Off-cluster: translate the API path to a kubectl invocation for --dry-run.
    mapping = {
        "/api/v1/nodes": ["get", "nodes"],
        "/api/v1/namespaces": ["get", "ns"],
        "/api/v1/services": ["get", "svc", "-A"],
        "/api/v1/persistentvolumeclaims": ["get", "pvc", "-A"],
        "/apis/apps/v1/deployments": ["get", "deployments", "-A"],
        "/apis/apps/v1/statefulsets": ["get", "statefulsets", "-A"],
        "/apis/apps/v1/daemonsets": ["get", "daemonsets", "-A"],
    }
    args = mapping.get(path)
    if not args:
        raise RuntimeError(f"no kubectl fallback for API path {path}")
    out = subprocess.run(
        ["kubectl", *args, "-o", "json"], capture_output=True, text=True, timeout=90
    )
    if out.returncode != 0:
        raise RuntimeError(f"kubectl failed: {args}\n{out.stderr.strip()}")
    return json.loads(out.stdout)


# ── Small helpers ────────────────────────────────────────────────────────────
def _ki_to_gib(q: str | None) -> str | None:
    if not q:
        return None
    try:
        if q.endswith("Ki"):
            return f"{int(q[:-2]) / (1024 * 1024):.0f}Gi"
        if q.endswith("Mi"):
            return f"{int(q[:-2]) / 1024:.0f}Gi"
        if q.endswith("Gi"):
            return f"{float(q[:-2]):.0f}Gi"
        return q
    except ValueError:
        return q


def _images_of(w: dict) -> list[str]:
    spec = w.get("spec", {}).get("template", {}).get("spec", {})
    imgs = [c.get("image", "") for c in spec.get("containers", [])]
    imgs += [c.get("image", "") for c in spec.get("initContainers", []) or []]
    return [i for i in imgs if i]


def _wants_gpu(w: dict) -> bool:
    spec = w.get("spec", {}).get("template", {}).get("spec", {})
    for c in spec.get("containers", []) + (spec.get("initContainers", []) or []):
        res = c.get("resources", {}) or {}
        for section in ("limits", "requests"):
            if "nvidia.com/gpu" in (res.get(section) or {}):
                return True
    return False


# ── Discovery → durable structure (NO readiness/placement/counts) ────────────
def discover_nodes() -> list[dict]:
    nodes = []
    for n in _k8s_get("/api/v1/nodes").get("items", []):
        m, st = n["metadata"], n["status"]
        labels = m.get("labels", {})
        roles = sorted(
            k.split("/", 1)[1] for k in labels
            if k.startswith("node-role.kubernetes.io/")
        )
        cap = st.get("capacity", {})
        ni = st.get("nodeInfo", {})
        nodes.append({
            "name": m["name"],
            "roles": roles or ["worker"],
            "cpu": cap.get("cpu"),
            "mem": _ki_to_gib(cap.get("memory")),
            "gpu_count": cap.get("nvidia.com/gpu"),
            "gpu_model": labels.get("gpu"),
            "os": ni.get("osImage"),
        })
    nodes.sort(key=lambda d: d["name"])
    return nodes


def discover_workloads() -> list[dict]:
    found: list[dict] = []
    for path, kind in (
        ("/apis/apps/v1/deployments", "Deployment"),
        ("/apis/apps/v1/statefulsets", "StatefulSet"),
        ("/apis/apps/v1/daemonsets", "DaemonSet"),
    ):
        for w in _k8s_get(path).get("items", []):
            ns = w["metadata"]["namespace"]
            name = w["metadata"]["name"]
            spec = w.get("spec", {}) or {}
            sel = (spec.get("template", {}).get("spec", {}).get("nodeSelector") or {})
            pinned = sel.get("kubernetes.io/hostname") or sel.get("gpu")
            found.append({
                "key": f"{ns}/{name}", "namespace": ns, "name": name,
                "kind": kind, "images": _images_of(w),
                "wants_gpu": _wants_gpu(w), "pinned": pinned,
            })
    found.sort(key=lambda d: d["key"])
    return found


def _svc_pvc_indexes():
    svc_by_ns: dict[str, list[dict]] = {}
    for s in _k8s_get("/api/v1/services").get("items", []):
        svc_by_ns.setdefault(s["metadata"]["namespace"], []).append(s)
    pvc_by_ns: dict[str, list[dict]] = {}
    for p in _k8s_get("/api/v1/persistentvolumeclaims").get("items", []):
        pvc_by_ns.setdefault(p["metadata"]["namespace"], []).append(p)
    return svc_by_ns, pvc_by_ns


def _resolve_db_endpoint(w: dict, kind: str, svc_by_ns, pvc_by_ns,
                         db_pvc: str | None) -> dict:
    """Resolve a datastore's DURABLE Service DNS + backing PVC (no run state)."""
    ns, name = w["metadata"]["namespace"], w["metadata"]["name"]
    pod_labels = (
        w.get("spec", {}).get("template", {}).get("metadata", {}).get("labels", {})
        or {}
    )
    svc_match = None
    matches = []
    for s in svc_by_ns.get(ns, []):
        sel = (s.get("spec", {}) or {}).get("selector") or {}
        if sel and all(pod_labels.get(k) == v for k, v in sel.items()):
            matches.append(s)
    if matches:
        def _rank(s):
            sn = s["metadata"]["name"]
            headless = (s.get("spec", {}) or {}).get("clusterIP") in (None, "None")
            rel = sn == name or sn.startswith(name) or name.startswith(sn)
            return (0 if rel else 1, 1 if headless else 0)
        svc_match = sorted(matches, key=_rank)[0]
    if svc_match is None:
        for s in svc_by_ns.get(ns, []):
            sn = s["metadata"]["name"]
            if (s.get("spec", {}) or {}).get("clusterIP") in (None, "None"):
                continue
            if sn == name or sn.startswith(f"{name}-") or \
                    sn == name.replace("-deployment", "-service"):
                svc_match = s
                break
    svc_dns = svc_port = None
    if svc_match:
        svc_dns = f"{svc_match['metadata']['name']}.{ns}.svc.cluster.local"
        ports = (svc_match.get("spec", {}) or {}).get("ports") or []
        if ports:
            svc_port = ports[0].get("port")

    claim_names: set[str] = set()
    tspec = w.get("spec", {}).get("template", {}).get("spec", {})
    for vol in tspec.get("volumes", []) or []:
        cn = (vol.get("persistentVolumeClaim") or {}).get("claimName")
        if cn:
            claim_names.add(cn)
    if kind == "StatefulSet":
        for vct in w.get("spec", {}).get("volumeClaimTemplates", []) or []:
            base = vct["metadata"]["name"]
            for p in pvc_by_ns.get(ns, []):
                if p["metadata"]["name"].startswith(f"{base}-{name}-"):
                    claim_names.add(p["metadata"]["name"])
    pvc_index = {p["metadata"]["name"]: p for p in pvc_by_ns.get(ns, [])}
    descs = []
    for cn in sorted(claim_names):
        if db_pvc and cn != db_pvc:
            continue
        p = pvc_index.get(cn)
        if not p:
            continue
        cap = (p.get("status", {}) or {}).get("capacity", {}).get("storage")
        sc = (p.get("spec", {}) or {}).get("storageClassName")
        descs.append(f"{cn} ({cap}, StorageClass {sc})")
    return {
        "service_dns": svc_dns, "service_port": svc_port,
        "pvc": "; ".join(descs) if descs else None,
    }


def discover_databases() -> list[dict]:
    workloads: list[tuple[str, dict]] = []
    for path, kind in (
        ("/apis/apps/v1/statefulsets", "StatefulSet"),
        ("/apis/apps/v1/deployments", "Deployment"),
    ):
        for w in _k8s_get(path).get("items", []):
            workloads.append((kind, w))
    svc_by_ns, pvc_by_ns = _svc_pvc_indexes()
    by_key = {f"{w['metadata']['namespace']}/{w['metadata']['name']}": (k, w)
              for k, w in workloads}

    found: list[dict] = []
    seen: set[str] = set()
    for kind, w in workloads:
        ns, name = w["metadata"]["namespace"], w["metadata"]["name"]
        key = f"{ns}/{name}"
        hay = " ".join([name.lower(), *[i.lower() for i in _images_of(w)]])
        hint = next((h for h in DB_IMAGE_HINTS if h in hay), None)
        if not hint or key in seen:
            continue
        seen.add(key)
        ep = _resolve_db_endpoint(w, kind, svc_by_ns, pvc_by_ns, None)
        found.append({
            "key": key, "namespace": ns, "name": name,
            "engine": _ENGINE_LABEL.get(hint, hint),
            "purpose": DB_PURPOSE.get(key), **ep,
        })
    for key, meta in SQLITE_SERVICES.items():
        if key in seen or key not in by_key:
            continue
        seen.add(key)
        kind, w = by_key[key]
        ep = _resolve_db_endpoint(w, kind, svc_by_ns, pvc_by_ns, meta.get("db_pvc"))
        found.append({
            "key": key, "namespace": w["metadata"]["namespace"],
            "name": w["metadata"]["name"], "engine": meta["engine"],
            "purpose": meta["purpose"], **ep,
        })
    found.sort(key=lambda d: d["key"])
    return found


# ── Fact rendering (durable, present-tense structural statements) ─────────────
def cluster_fact(nodes, ns_names, n_workloads) -> str:
    cp = sum(1 for n in nodes if "control-plane" in n["roles"])
    workers = len(nodes) - cp
    total_cpu = sum(int(n["cpu"]) for n in nodes if n["cpu"])
    gpu_bits = [
        f"{n['name']} ({n['gpu_count']}x{(' ' + n['gpu_model']) if n['gpu_model'] else ''})"
        for n in nodes if n["gpu_count"]
    ]
    gpu_str = ("; GPU: " + ", ".join(gpu_bits)) if gpu_bits else "; no GPU nodes"
    return (
        f"The homelab is an RKE2 Kubernetes cluster of {len(nodes)} nodes "
        f"({cp} control-plane + {workers} workers) with {total_cpu} total CPU "
        f"cores{gpu_str}. It runs {len(ns_names)} namespaces and {n_workloads} "
        f"workloads (Deployments/StatefulSets/DaemonSets). Namespaces: "
        f"{', '.join(ns_names)}."
    )


def node_fact(n: dict) -> str:
    parts = [f"Cluster node {n['name']} is a {'+'.join(n['roles'])} node."]
    cap = []
    if n["cpu"]:
        cap.append(f"{n['cpu']} CPU cores")
    if n["mem"]:
        cap.append(f"{n['mem']} RAM")
    if cap:
        parts.append("It has " + " and ".join(cap) + ".")
    if n["gpu_count"]:
        model = f" ({n['gpu_model']})" if n["gpu_model"] else ""
        parts.append(f"It provides {n['gpu_count']}x nvidia.com/gpu{model}.")
    if n["os"]:
        parts.append(f"OS is {n['os']}.")
    if NODE_PURPOSE.get(n["name"]):
        parts.append(f"Its role: {NODE_PURPOSE[n['name']]}.")
    return " ".join(parts)


def workload_fact(w: dict) -> str:
    parts = [
        f"{w['kind']} '{w['name']}' is a workload in the '{w['namespace']}' "
        f"namespace."
    ]
    if WORKLOAD_PURPOSE.get(w["key"]):
        parts.append(f"Its purpose: {WORKLOAD_PURPOSE[w['key']]}.")
    if w["images"]:
        parts.append(
            f"Image: {w['images'][0]}." if len(w["images"]) == 1
            else f"Images: {', '.join(w['images'])}."
        )
    if w["wants_gpu"]:
        parts.append("It is GPU-bound (requests nvidia.com/gpu).")
    if w["pinned"]:
        parts.append(f"It is pinned to node/selector '{w['pinned']}'.")
    return " ".join(parts)


def db_fact(d: dict) -> str:
    purpose = d.get("purpose") or (
        f"{d['engine']} datastore (purpose not yet curated)")
    parts = [
        f"{d['engine']} database '{d['name']}' lives in the '{d['namespace']}' "
        f"namespace.",
        f"Its purpose: {purpose}.",
    ]
    if d.get("service_dns"):
        port = f":{d['service_port']}" if d.get("service_port") else ""
        parts.append(f"It is reached at {d['service_dns']}{port}.")
    if d.get("pvc"):
        parts.append(f"Backing storage: {d['pvc']}.")
    return " ".join(parts)


def build_facts() -> list[dict]:
    """Return [{domain, wskey, text}, ...] of durable facts in write order."""
    nodes = discover_nodes()
    ns_names = sorted(
        n["metadata"]["name"] for n in _k8s_get("/api/v1/namespaces").get("items", [])
    )
    workloads = discover_workloads()
    dbs = discover_databases()

    facts = [{
        "domain": "cluster", "wskey": "cluster:homelab",
        "text": cluster_fact(nodes, ns_names, len(workloads)),
    }]
    facts += [{"domain": "node", "wskey": f"node:{n['name']}",
               "text": node_fact(n)} for n in nodes]
    facts += [{"domain": "workload", "wskey": f"workload:{w['key']}",
               "text": workload_fact(w)} for w in workloads]
    facts += [{"domain": "db", "wskey": f"db:{d['key']}",
               "text": db_fact(d)} for d in dbs]
    return facts


# ── mem0 write + qdrant reconcile ────────────────────────────────────────────
def mem0_add(text: str, metadata: dict) -> tuple[bool, str]:
    payload = json.dumps(
        {"text": text, "user_id": USER_ID, "metadata": metadata}
    ).encode()
    req = urllib.request.Request(
        f"{MEM0_URL}/add", data=payload,
        headers={"Content-Type": "application/json"}, method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as r:
            return True, (r.read().decode() or "{}")
    except urllib.error.HTTPError as exc:
        return False, f"HTTP {exc.code}: {exc.read().decode(errors='replace')[:300]}"
    except urllib.error.URLError as exc:
        return False, f"unreachable: {exc}"


def _qdrant_post(path: str, body: dict) -> dict:
    req = urllib.request.Request(
        f"{QDRANT_URL}{path}", data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json"}, method="POST",
    )
    with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as r:
        return json.loads(r.read() or b"{}")


def reconcile(run_id: str) -> int:
    """Delete every worldsync point whose run id != this run (vanished resources).

    Returns the number of stale points removed. Counts first (for the report),
    then deletes by the same filter in one operation.
    """
    flt = {
        "must": [{"key": "source", "match": {"value": SOURCE}}],
        "must_not": [{"key": "worldsync_run", "match": {"value": run_id}}],
    }
    # Count stale points so the run reports what it pruned.
    stale = 0
    next_page = None
    while True:
        body = {"limit": 256, "with_payload": False, "with_vector": False,
                "filter": flt}
        if next_page is not None:
            body["offset"] = next_page
        res = _qdrant_post(
            f"/collections/{QDRANT_COLLECTION}/points/scroll", body
        ).get("result", {})
        pts = res.get("points", [])
        stale += len(pts)
        next_page = res.get("next_page_offset")
        if not next_page:
            break
    if stale:
        _qdrant_post(
            f"/collections/{QDRANT_COLLECTION}/points/delete?wait=true",
            {"filter": flt},
        )
    return stale


def main() -> int:
    global USER_ID
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--dry-run", action="store_true",
                    help="print durable facts that would be written; no writes, "
                         "no reconcile")
    ap.add_argument("--user-id", default=USER_ID,
                    help=f"mem0 partition key (default: {USER_ID})")
    ap.add_argument("--no-reconcile", action="store_true",
                    help="write facts but skip the qdrant prune of vanished ones")
    args = ap.parse_args()
    USER_ID = args.user_id

    run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")

    try:
        facts = build_facts()
    except Exception as exc:  # noqa: BLE001
        print(f"discovery failed: {exc}", file=sys.stderr)
        return 2

    print(f"worldsync_durable run_id={run_id} — built {len(facts)} durable "
          f"fact(s) (user_id={USER_ID}).\n")

    if args.dry_run:
        for f in facts:
            print(f"  [{f['wskey']}]\n  {f['text']}\n")
        print(f"[dry-run] would write {len(facts)} fact(s) to {MEM0_URL}/add "
              f"and reconcile stale worldsync points in qdrant.")
        return 0

    ok = failures = 0
    for f in facts:
        meta = {"source": SOURCE, "domain": f["domain"],
                "wskey": f["wskey"], "worldsync_run": run_id}
        success, detail = mem0_add(f["text"], meta)
        if success:
            ok += 1
            print(f"  [ok]   {f['wskey']}")
        else:
            failures += 1
            print(f"  [FAIL] {f['wskey']}: {detail}")

    print(f"\nwrote {ok}/{len(facts)} durable fact(s) to mem0; {failures} failed.")

    # RECONCILE only on a fully successful write — a partial run must never
    # delete facts that might still be valid.
    if failures:
        print("writes failed — SKIPPING reconcile (won't prune on a partial "
              "run). Fix mem0 and re-run.", file=sys.stderr)
        return 1
    if args.no_reconcile:
        print("reconcile skipped (--no-reconcile).")
        return 0
    try:
        pruned = reconcile(run_id)
        print(f"reconcile: pruned {pruned} stale worldsync point(s) "
              f"(resources no longer present).")
    except Exception as exc:  # noqa: BLE001
        print(f"reconcile failed (facts were written OK): {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
