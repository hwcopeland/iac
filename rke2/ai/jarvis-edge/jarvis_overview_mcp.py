"""Read-only cluster-overview MCP for JARVIS — one tool, one live digest.

The brain calls this for open-ended "what's going on / what's the deal with X"
questions. It returns a compact, *spoken-friendly* snapshot of the whole
homelab cluster: nodes (incl. the GPU node), per-namespace workload health,
anything that is genuinely not Ready, and GPU allocation on nixos-gpu.

Read-only. Bound to the in-pod ServiceAccount `jarvis-readonly` (view +
node-reader, no secrets) the same way jarvis_kube_mcp.py is — kubectl
auto-picks-up the mounted SA token in-cluster, so no extra config is needed.

This is a *live sync*, never frozen state: every call re-queries kubectl, so
the digest is current the moment it's spoken.

Tool:
    cluster_overview()  — no args; returns the digest as plain text.

Design notes / gotchas baked in:
  * "Completed"/"Succeeded" pods are finished Jobs (CronJobs, helm installs,
    backups), NOT problems — they are filtered out of the not-Ready list.
  * The GPU is time-sliced 10x on nixos-gpu (9 allocatable). We report how many
    nvidia.com/gpu *requests* are outstanding, which is the only GPU-pressure
    signal available read-only via kubectl without Prometheus/DCGM scraping.
  * Output is kept short and grouped so it reads well aloud.
"""
from __future__ import annotations

import json
import subprocess
import sys
import traceback

# Allocatable GPU slices on the time-sliced RTX 3070 (nixos-gpu). Informational
# fallback only; the live value is read from the node when available.
_GPU_NODE = "nixos-gpu"

# Pod phases that mean "finished, on purpose" — these are not health problems.
_DONE_PHASES = {"Succeeded"}

_TOOLS = [
    {
        "name": "cluster_overview",
        "description": (
            "Live, spoken-friendly digest of the whole homelab cluster: nodes "
            "(including the GPU node and its status), a per-namespace count of "
            "healthy vs unhealthy workloads, an explicit list of anything that "
            "is NOT Ready right now (finished Jobs are excluded), and GPU "
            "allocation on the shared RTX 3070. Call this for open-ended "
            "'what's going on', 'how's the cluster', or 'what's the deal with X' "
            "questions before drilling in with the kube_* tools. No arguments."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {},
            "additionalProperties": False,
        },
    },
]


def _kubectl_json(args: list[str], timeout: float = 20.0) -> dict | None:
    """Run a kubectl read returning JSON; None on any failure."""
    try:
        r = subprocess.run(["kubectl", *args, "-o", "json"],
                           capture_output=True, text=True, timeout=timeout)
        if r.returncode != 0 or not r.stdout.strip():
            return None
        return json.loads(r.stdout)
    except (subprocess.TimeoutExpired, json.JSONDecodeError, Exception):  # noqa: BLE001
        return None


def _node_digest() -> tuple[list[str], list[str]]:
    """Return (ready_node_names, not_ready_lines)."""
    data = _kubectl_json(["get", "nodes"])
    ready: list[str] = []
    bad: list[str] = []
    if not data:
        return ready, ["could not read nodes"]
    for n in data.get("items", []):
        name = n["metadata"]["name"]
        conds = {c["type"]: c["status"] for c in n["status"].get("conditions", [])}
        is_ready = conds.get("Ready") == "True"
        # Cordoned / unschedulable is worth flagging even if Ready.
        unsched = n["spec"].get("unschedulable", False)
        if is_ready and not unsched:
            ready.append(name)
        else:
            state = "NotReady" if not is_ready else "cordoned"
            bad.append(f"{name} ({state})")
    return ready, bad


def _gpu_digest() -> str:
    """One-line GPU allocation summary for the shared RTX 3070."""
    node = _kubectl_json(["get", "node", _GPU_NODE])
    if not node:
        return f"GPU node {_GPU_NODE}: not found or unreadable."
    conds = {c["type"]: c["status"]
             for c in node["status"].get("conditions", [])}
    node_state = "Ready" if conds.get("Ready") == "True" else "NotReady"
    alloc = node["status"].get("allocatable", {}).get("nvidia.com/gpu", "?")

    pods = _kubectl_json(["get", "pods", "-A"])
    requested = 0
    users: list[str] = []
    if pods:
        for p in pods.get("items", []):
            if p["status"].get("phase") not in ("Running", "Pending"):
                continue
            n = 0
            for c in p["spec"].get("containers", []):
                res = c.get("resources", {})
                v = (res.get("requests", {}) or {}).get("nvidia.com/gpu") \
                    or (res.get("limits", {}) or {}).get("nvidia.com/gpu")
                if v:
                    try:
                        n += int(v)
                    except (TypeError, ValueError):
                        pass
            if n:
                requested += n
                users.append(f"{p['metadata']['namespace']}/"
                             f"{p['metadata']['name'].split('-')[0]}")
    who = f" ({', '.join(sorted(set(users)))})" if users else ""
    return (f"GPU ({_GPU_NODE}, {node_state}): {requested} of {alloc} "
            f"time-sliced slices requested{who}.")


def _workload_digest() -> tuple[dict[str, list[int]], list[str]]:
    """Return (per-ns [healthy, unhealthy] counts, not_ready_lines).

    Healthy  = Running with all containers ready, OR finished Job (Succeeded).
    Unhealthy = anything else (Pending, CrashLoop, not all containers ready).
    """
    pods = _kubectl_json(["get", "pods", "-A"])
    by_ns: dict[str, list[int]] = {}
    bad: list[str] = []
    if not pods:
        return by_ns, ["could not read pods"]
    for p in pods.get("items", []):
        ns = p["metadata"]["namespace"]
        name = p["metadata"]["name"]
        phase = p["status"].get("phase", "Unknown")
        by_ns.setdefault(ns, [0, 0])

        if phase in _DONE_PHASES:
            by_ns[ns][0] += 1  # finished Job — healthy/done
            continue

        cs = p["status"].get("containerStatuses") or []
        total = len(cs)
        ready = sum(1 for c in cs if c.get("ready"))
        restarts = sum(c.get("restartCount", 0) for c in cs)
        # Surface a waiting-reason (CrashLoopBackOff, ImagePullBackOff...) if any.
        reason = ""
        for c in cs:
            w = (c.get("state") or {}).get("waiting") or {}
            if w.get("reason"):
                reason = w["reason"]
                break

        healthy = phase == "Running" and (total == 0 or ready == total) and not reason
        if healthy:
            by_ns[ns][0] += 1
        else:
            by_ns[ns][1] += 1
            tag = reason or phase
            extra = f", {restarts} restarts" if restarts else ""
            bad.append(f"{ns}/{name}: {tag} ({ready}/{total} ready{extra})")
    return by_ns, bad


def _overview() -> str:
    ready_nodes, bad_nodes = _node_digest()
    by_ns, bad_pods = _workload_digest()
    gpu_line = _gpu_digest()

    lines: list[str] = []

    # --- Nodes ---
    nodes_msg = f"{len(ready_nodes)} nodes Ready"
    if bad_nodes:
        nodes_msg += f"; PROBLEM nodes: {', '.join(bad_nodes)}"
    lines.append(nodes_msg + ".")

    # --- GPU ---
    lines.append(gpu_line)

    # --- Workloads: only call out namespaces that have something unhealthy,
    #     plus a one-line healthy total, to keep it short for speech. ---
    total_healthy = sum(v[0] for v in by_ns.values())
    total_unhealthy = sum(v[1] for v in by_ns.values())
    unhealthy_ns = {ns: v[1] for ns, v in by_ns.items() if v[1]}
    if total_unhealthy == 0:
        lines.append(f"All {total_healthy} workloads across "
                     f"{len(by_ns)} namespaces are healthy.")
    else:
        ns_summary = ", ".join(f"{ns} ({n})"
                               for ns, n in sorted(unhealthy_ns.items()))
        lines.append(f"{total_healthy} workloads healthy, "
                     f"{total_unhealthy} NOT healthy in: {ns_summary}.")

    # --- Explicit not-Ready list (cap to keep it spoken-friendly) ---
    if bad_pods:
        shown = bad_pods[:8]
        lines.append("Not Ready:")
        lines.extend(f"  - {p}" for p in shown)
        if len(bad_pods) > len(shown):
            lines.append(f"  - …and {len(bad_pods) - len(shown)} more.")
    else:
        lines.append("Nothing is in a failed or pending state.")

    return "\n".join(lines)


def _call(name: str, args: dict) -> dict:
    if name == "cluster_overview":
        return {"content": [{"type": "text", "text": _overview()}]}
    return {"content": [{"type": "text", "text": f"unknown tool: {name}"}],
            "isError": True}


def _handle(req: dict) -> dict | None:
    method = req.get("method")
    rid = req.get("id")
    if method == "initialize":
        return {
            "jsonrpc": "2.0", "id": rid,
            "result": {
                "protocolVersion": "2025-11-25",
                "capabilities": {"tools": {"listChanged": False}},
                "serverInfo": {"name": "jarvis_overview", "version": "0.1.0"},
            },
        }
    if method == "notifications/initialized":
        return None
    if method == "tools/list":
        return {"jsonrpc": "2.0", "id": rid, "result": {"tools": _TOOLS}}
    if method == "tools/call":
        params = req.get("params") or {}
        try:
            return {"jsonrpc": "2.0", "id": rid,
                    "result": _call(params.get("name", ""),
                                    params.get("arguments") or {})}
        except Exception as exc:  # noqa: BLE001
            return {"jsonrpc": "2.0", "id": rid,
                    "error": {"code": -32603,
                              "message": f"{type(exc).__name__}: {exc}",
                              "data": traceback.format_exc()}}
    if rid is None:
        return None
    return {"jsonrpc": "2.0", "id": rid,
            "error": {"code": -32601, "message": f"method not found: {method}"}}


def main() -> None:
    out = sys.stdout
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except json.JSONDecodeError:
            continue
        resp = _handle(req)
        if resp is not None:
            out.write(json.dumps(resp) + "\n")
            out.flush()


if __name__ == "__main__":
    main()
