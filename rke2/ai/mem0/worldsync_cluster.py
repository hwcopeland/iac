#!/usr/bin/env python3
"""worldsync_cluster — map the homelab CLUSTER, NODES, and WORKLOADS into the
JARVIS unified memory (mem0).

This is one of the "world sync" mappers from the JARVIS charter: live homelab
state is *synced into* mem0 so the unified memory stays both unified and fresh.
Its sibling worldsync_databases.py maps datastores; this one maps the compute
substrate JARVIS reasons over:

  - the cluster as a whole (node count, total CPU/RAM, GPU inventory),
  - each NODE (role, Ready status, CPU/RAM capacity, GPU capacity, OS), and
  - each Deployment / StatefulSet / DaemonSet (kind, namespace, replicas vs
    ready, primary image, where it actually runs, whether it asks for GPU).

It writes concise, self-contained facts that read like *understanding*, e.g.:
  "[worldsync:node:nixos-gpu] Node nixos-gpu: worker, Ready. 32 CPU, 93Gi RAM.
   GPU: 10x nvidia.com/gpu (RTX 3070, time-sliced). OS NixOS 25.11 ..."

  RE-RUNNABLE BY DESIGN. Run it on a schedule (CronJob) or by hand. It does an
  idempotent UPSERT into mem0: every fact is a single self-contained statement
  prefixed with a stable "[worldsync:<kind>:<key>]" tag. The stable tag makes a
  fact greppable and lets a future run supersede a stale one rather than pile up
  duplicates; mem0's own extraction layer also deduplicates equivalent facts.
  We never freeze live state as durable memory — re-running refreshes it.

DISCOVERY (read-only kubectl, all namespaces):
  - nodes (-o json): roles, conditions, capacity, nvidia.com/gpu, nodeInfo.
  - namespaces (-o json): inventory.
  - deployments / statefulsets / daemonsets (-A -o json): replicas, ready,
    images, GPU resource requests, nodeSelector.
  - pods (-A -o json): authoritative *actual* node placement for each workload's
    running pods (a workload can land somewhere its nodeSelector merely allows).

TARGET: mem0 REST service. POST /add {text, user_id}. Default user_id "jarvis"
(the homelab/world scope — distinct from the per-speaker "owner" scope used by
the brain's jarvis_mem0_mcp.py shim).

  NOTE (2026-06-16): the mem0 server is currently returning HTTP 400
  "Unsupported embedding provider: fastembed" on /add, so writes will fail until
  the server is fixed (see mem0/build/server.py _register_fastembed_provider and
  the Kaniko rebuild path). This mapper is correct and complete: the write
  payload matches the documented /add interface ({text, user_id}); it reports
  each failed write clearly and exits non-zero so a CronJob surfaces the
  breakage, and it will start persisting the moment the server embeds again. Use
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

# Stable tag prefix that marks every fact this mapper writes, so facts are
# greppable in the store and a re-run produces the *same* phrasing per key
# (idempotent upsert). Sub-namespaced by entity kind: cluster, node, workload.
TAG = "worldsync"

# Curated, human-known facts kubectl can't infer: what a node is FOR and any
# operational caveats. Keyed by node name. Unknown nodes still get a generic
# inventory line so discovery never silently drops a new node.
NODE_PURPOSE: dict[str, str] = {
    "nixos-gpu":
        "the single GPU box (RTX 3070, time-sliced 10x, ~8GB VRAM). Runs the AI "
        "GPU workloads (ollama, jarvis voice STT/TTS) AND khemeia/chem GPU jobs "
        "— GPU is shared and contended. NixOS, so pods need "
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

# Curated purpose for a few flagship workloads — the "what it's FOR" kubectl
# can't infer. Keyed by "<namespace>/<name>". Everything else is inventoried
# generically (purpose omitted), so discovery degrades gracefully as the
# cluster changes; add a line here to enrich a workload.
WORKLOAD_PURPOSE: dict[str, str] = {
    "ai/jarvis-stack":
        "the JARVIS voice brain pod: whisper STT + chatterbox TTS (both GPU) + "
        "jarvis-edge (CPU) — the front door for voice interaction",
    "ai/jarvis-mem0":
        "the mem0 unified-memory REST backend — the spine this very sync writes "
        "into",
    "ai/ollama":
        "local LLM inference (qwen models) on the RTX 3070; fallback brain",
    "ai/open-webui": "web chat UI over ollama (chat.hwcopeland.net)",
    "ai/homelab-bot": "Discord bot (Grok primary, ollama fallback)",
}


def _ki_to_gib(quantity: str | None) -> str | None:
    """Render a Kubernetes memory quantity (e.g. '97956740Ki') as 'NNGi'."""
    if not quantity:
        return None
    try:
        if quantity.endswith("Ki"):
            gib = int(quantity[:-2]) / (1024 * 1024)
        elif quantity.endswith("Mi"):
            gib = int(quantity[:-2]) / 1024
        elif quantity.endswith("Gi"):
            gib = float(quantity[:-2])
        else:
            return quantity  # unknown unit; pass through verbatim
        return f"{gib:.0f}Gi"
    except ValueError:
        return quantity


def _kubectl_json(args: list[str]) -> dict:
    """Run a read-only `kubectl get ... -o json` and parse it."""
    cmd = ["kubectl", *args, "-o", "json"]
    out = subprocess.run(cmd, capture_output=True, text=True, timeout=90)
    if out.returncode != 0:
        raise RuntimeError(f"kubectl failed: {' '.join(cmd)}\n{out.stderr.strip()}")
    return json.loads(out.stdout)


def _images_of(workload: dict) -> list[str]:
    spec = workload.get("spec", {}).get("template", {}).get("spec", {})
    imgs = [c.get("image", "") for c in spec.get("containers", [])]
    imgs += [c.get("image", "") for c in spec.get("initContainers", []) or []]
    return [i for i in imgs if i]


def _wants_gpu(workload: dict) -> bool:
    spec = workload.get("spec", {}).get("template", {}).get("spec", {})
    for c in spec.get("containers", []) + (spec.get("initContainers", []) or []):
        res = c.get("resources", {}) or {}
        for section in ("limits", "requests"):
            if "nvidia.com/gpu" in (res.get(section) or {}):
                return True
    return False


def discover_nodes() -> list[dict]:
    nodes = []
    for n in _kubectl_json(["get", "nodes"]).get("items", []):
        m, st = n["metadata"], n["status"]
        labels = m.get("labels", {})
        roles = sorted(
            k.split("/", 1)[1] for k in labels
            if k.startswith("node-role.kubernetes.io/")
        )
        conds = {c["type"]: c["status"] for c in st.get("conditions", [])}
        cap = st.get("capacity", {})
        ni = st.get("nodeInfo", {})
        nodes.append({
            "name": m["name"],
            "roles": roles or ["worker"],
            "ready": conds.get("Ready") == "True",
            "cpu": cap.get("cpu"),
            "mem": _ki_to_gib(cap.get("memory")),
            "gpu_count": cap.get("nvidia.com/gpu"),
            "gpu_model": labels.get("gpu"),
            "os": ni.get("osImage"),
            "kubelet": ni.get("kubeletVersion"),
        })
    nodes.sort(key=lambda d: d["name"])
    return nodes


def _pod_nodes_by_owner() -> dict[tuple[str, str], dict[str, int]]:
    """Map (namespace, controller-name) -> {nodeName: running_pod_count}.

    Resolves the *actual* placement of a workload's pods. We climb one level of
    ownerReferences (Pod -> ReplicaSet -> Deployment) so Deployment pods, whose
    direct owner is a hash-suffixed ReplicaSet, attribute to the Deployment;
    StatefulSet/DaemonSet pods own-ref the controller directly.
    """
    # Build ReplicaSet -> Deployment lookup so we can resolve the grandparent.
    rs_owner: dict[tuple[str, str], str] = {}
    for rs in _kubectl_json(["get", "rs", "-A"]).get("items", []):
        ns = rs["metadata"]["namespace"]
        for o in rs["metadata"].get("ownerReferences", []) or []:
            if o.get("kind") == "Deployment":
                rs_owner[(ns, rs["metadata"]["name"])] = o["name"]

    placement: dict[tuple[str, str], dict[str, int]] = {}
    for p in _kubectl_json(["get", "pods", "-A"]).get("items", []):
        if p["status"].get("phase") != "Running":
            continue
        node = p["spec"].get("nodeName")
        if not node:
            continue
        ns = p["metadata"]["namespace"]
        for o in p["metadata"].get("ownerReferences", []) or []:
            kind, name = o.get("kind"), o.get("name")
            if kind == "ReplicaSet":
                dep = rs_owner.get((ns, name))
                if dep:
                    key = (ns, dep)
                else:
                    continue
            elif kind in ("StatefulSet", "DaemonSet"):
                key = (ns, name)
            else:
                continue
            placement.setdefault(key, {}).setdefault(node, 0)
            placement[key][node] += 1
    return placement


def discover_workloads() -> list[dict]:
    placement = _pod_nodes_by_owner()
    found: list[dict] = []
    for kind_plural, kind in (
        ("deployments", "Deployment"),
        ("statefulsets", "StatefulSet"),
        ("daemonsets", "DaemonSet"),
    ):
        for w in _kubectl_json(["get", kind_plural, "-A"]).get("items", []):
            ns = w["metadata"]["namespace"]
            name = w["metadata"]["name"]
            spec = w.get("spec", {}) or {}
            status = w.get("status", {}) or {}
            if kind == "DaemonSet":
                desired = status.get("desiredNumberScheduled", 0)
                ready = status.get("numberReady", 0)
            else:
                desired = spec.get("replicas", 0)
                ready = status.get("readyReplicas", 0) or 0
            images = _images_of(w)
            nodes = placement.get((ns, name), {})
            node_str = (
                ", ".join(f"{n} (x{c})" if c > 1 else n
                          for n, c in sorted(nodes.items()))
                if nodes else None
            )
            sel = (spec.get("template", {}).get("spec", {})
                   .get("nodeSelector") or {})
            pinned = sel.get("kubernetes.io/hostname") or sel.get("gpu")
            found.append({
                "key": f"{ns}/{name}",
                "namespace": ns,
                "name": name,
                "kind": kind,
                "desired": desired,
                "ready": ready,
                "images": images,
                "wants_gpu": _wants_gpu(w),
                "nodes": node_str,
                "pinned": pinned,
            })
    found.sort(key=lambda d: d["key"])
    return found


def cluster_fact(nodes: list[dict], ns_names: list[str], n_workloads: int) -> str:
    ready = sum(1 for n in nodes if n["ready"])
    cp = sum(1 for n in nodes if "control-plane" in n["roles"])
    workers = len(nodes) - cp
    total_cpu = sum(int(n["cpu"]) for n in nodes if n["cpu"])
    gpu_nodes = [n for n in nodes if n["gpu_count"]]
    gpu_bits = []
    for n in gpu_nodes:
        model = f" {n['gpu_model']}" if n["gpu_model"] else ""
        gpu_bits.append(f"{n['name']} ({n['gpu_count']}x{model})")
    gpu_str = ("; GPU: " + ", ".join(gpu_bits)) if gpu_bits else "; no GPU nodes"
    return (
        f"[{TAG}:cluster:homelab] RKE2 Kubernetes cluster: {len(nodes)} nodes "
        f"({ready} Ready) = {cp} control-plane + {workers} workers, "
        f"{total_cpu} total CPU cores{gpu_str}. "
        f"{len(ns_names)} namespaces; {n_workloads} workloads "
        f"(Deployments/StatefulSets/DaemonSets). Namespaces: "
        f"{', '.join(ns_names)}."
    )


def node_fact(n: dict) -> str:
    roles = "+".join(n["roles"])
    status = "Ready" if n["ready"] else "NOT Ready"
    parts = [f"[{TAG}:node:{n['name']}] Node {n['name']}: {roles}, {status}."]
    cap = []
    if n["cpu"]:
        cap.append(f"{n['cpu']} CPU")
    if n["mem"]:
        cap.append(f"{n['mem']} RAM")
    if cap:
        parts.append("Capacity: " + ", ".join(cap) + ".")
    if n["gpu_count"]:
        model = f" ({n['gpu_model']})" if n["gpu_model"] else ""
        parts.append(f"GPU: {n['gpu_count']}x nvidia.com/gpu{model}.")
    if n["os"]:
        parts.append(f"OS {n['os']}, kubelet {n['kubelet']}.")
    purpose = NODE_PURPOSE.get(n["name"])
    if purpose:
        parts.append(f"Role: {purpose}.")
    return " ".join(parts)


def workload_fact(w: dict) -> str:
    if w["kind"] == "DaemonSet":
        health = f"{w['ready']}/{w['desired']} ready (DaemonSet)"
    else:
        health = f"{w['ready']}/{w['desired']} ready"
    parts = [
        f"[{TAG}:workload:{w['key']}] {w['kind']} '{w['name']}' in namespace "
        f"'{w['namespace']}': {health}."
    ]
    purpose = WORKLOAD_PURPOSE.get(w["key"])
    if purpose:
        parts.append(f"Purpose: {purpose}.")
    if w["images"]:
        if len(w["images"]) == 1:
            parts.append(f"Image: {w['images'][0]}.")
        else:
            parts.append(f"Images: {', '.join(w['images'])}.")
    if w["wants_gpu"]:
        parts.append("Requests GPU (nvidia.com/gpu).")
    if w["nodes"]:
        parts.append(f"Running on: {w['nodes']}.")
    elif w["desired"] == 0:
        parts.append("Currently scaled to 0 (not running).")
    elif w["pinned"]:
        parts.append(f"Pinned to: {w['pinned']} (no running pod).")
    return " ".join(parts)


def mem0_add(text: str) -> tuple[bool, str]:
    payload = json.dumps({"text": text, "user_id": USER_ID}).encode()
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


def build_facts() -> list[tuple[str, str]]:
    """Gather everything and return [(key, fact_text), ...] in write order."""
    nodes = discover_nodes()
    ns_names = sorted(
        n["metadata"]["name"]
        for n in _kubectl_json(["get", "ns"]).get("items", [])
    )
    workloads = discover_workloads()

    facts: list[tuple[str, str]] = [
        ("cluster:homelab", cluster_fact(nodes, ns_names, len(workloads))),
    ]
    facts += [(f"node:{n['name']}", node_fact(n)) for n in nodes]
    facts += [(f"workload:{w['key']}", workload_fact(w)) for w in workloads]
    return facts


def main() -> int:
    global USER_ID
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--dry-run", action="store_true",
                    help="print the facts that would be written; do not POST")
    ap.add_argument("--user-id", default=USER_ID,
                    help=f"mem0 partition key (default: {USER_ID})")
    ap.add_argument("--limit-workloads", type=int, default=0,
                    help="cap workload facts (0 = all); for quick smoke tests")
    args = ap.parse_args()
    USER_ID = args.user_id

    try:
        facts = build_facts()
    except Exception as exc:  # noqa: BLE001
        print(f"discovery failed: {exc}", file=sys.stderr)
        return 2

    if args.limit_workloads > 0:
        head = [f for f in facts if not f[0].startswith("workload:")]
        wls = [f for f in facts if f[0].startswith("workload:")]
        facts = head + wls[: args.limit_workloads]

    print(f"Built {len(facts)} fact(s) from cluster state.\n")

    if args.dry_run:
        for _key, text in facts:
            print("  " + text + "\n")
        print(f"[dry-run] would write {len(facts)} fact(s) to "
              f"{MEM0_URL}/add (user_id={USER_ID}).")
        return 0

    ok = failures = 0
    for key, text in facts:
        success, detail = mem0_add(text)
        if success:
            ok += 1
            print(f"  [ok]   {key}")
        else:
            failures += 1
            print(f"  [FAIL] {key}: {detail}")

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
