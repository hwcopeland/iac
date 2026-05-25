"""Read-only kubectl MCP for JARVIS — brain can inspect the cluster.

Bound to k8s ServiceAccount `jarvis-readonly` (subjects: view ClusterRole
+ jarvis-node-reader), which excludes secrets. The pod's mounted SA
token at /var/run/secrets/kubernetes.io/serviceaccount/ is what kubectl
auto-picks-up in-cluster, so no extra config needed.

Tools (all read-only, all explicitly NS-scoped where relevant):

    kube_get_pods(namespace?)        — list pods (default: all namespaces)
    kube_logs(name, namespace, container?, tail_lines?)
    kube_describe(kind, name, namespace?)
    kube_nodes()                     — node list + status
    kube_events(namespace?)          — recent warnings/events
    kube_top_pods(namespace?)        — `kubectl top pods` (needs metrics-server)
    kube_top_nodes()                 — `kubectl top nodes`
    kube_get(kind, name?, namespace?, label?)
                                     — generic `get` for any kind RBAC permits
"""
from __future__ import annotations

import json
import os
import shlex
import subprocess
import sys
import traceback


_TOOLS = [
    {
        "name": "kube_get_pods",
        "description": (
            "List pods in the cluster. Pass `namespace` for a specific NS, "
            "or omit for all namespaces. Returns a compact table-like JSON: "
            "{namespace, name, status, restarts, age, node, ready}."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {"namespace": {"type": "string"}},
            "additionalProperties": False,
        },
    },
    {
        "name": "kube_logs",
        "description": (
            "Tail logs from a pod. Pass `name` and `namespace`; optional "
            "`container` (defaults to first), optional `tail_lines` (default 80, "
            "max 500). Use this to investigate failures the user mentions."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "name": {"type": "string"},
                "namespace": {"type": "string"},
                "container": {"type": "string"},
                "tail_lines": {"type": "integer", "minimum": 1, "maximum": 500},
            },
            "required": ["name", "namespace"],
            "additionalProperties": False,
        },
    },
    {
        "name": "kube_describe",
        "description": (
            "`kubectl describe <kind> <name>` style detail dump. Pass `kind` "
            "(pod, node, service, deployment, ...), `name`, and `namespace` "
            "(required for namespaced kinds, optional for nodes/etc)."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "kind": {"type": "string"},
                "name": {"type": "string"},
                "namespace": {"type": "string"},
            },
            "required": ["kind", "name"],
            "additionalProperties": False,
        },
    },
    {
        "name": "kube_nodes",
        "description": "List cluster nodes with status, role, age, and version.",
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
    {
        "name": "kube_events",
        "description": (
            "Recent cluster events (last ~10 minutes worth, sorted by lastTimestamp). "
            "Pass `namespace` for a specific NS, omit for all."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {"namespace": {"type": "string"}},
            "additionalProperties": False,
        },
    },
    {
        "name": "kube_top_pods",
        "description": "Pod CPU + memory usage. Optionally per namespace.",
        "inputSchema": {
            "type": "object",
            "properties": {"namespace": {"type": "string"}},
            "additionalProperties": False,
        },
    },
    {
        "name": "kube_top_nodes",
        "description": "Per-node CPU + memory usage.",
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
    {
        "name": "kube_get",
        "description": (
            "Generic read for any resource kind RBAC permits. Pass `kind` "
            "(pods, services, deployments, configmaps, ...), optional `name`, "
            "optional `namespace`, optional `label` selector ('app=foo'). "
            "Returns YAML for a single named resource or summary table for lists."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "kind": {"type": "string"},
                "name": {"type": "string"},
                "namespace": {"type": "string"},
                "label": {"type": "string"},
            },
            "required": ["kind"],
            "additionalProperties": False,
        },
    },
]


def _kubectl(args: list[str], timeout: float = 15.0) -> str:
    """Run kubectl, return merged stdout+stderr capped at 12 KB."""
    try:
        r = subprocess.run(["kubectl", *args], capture_output=True,
                           text=True, timeout=timeout)
        out = (r.stdout or "") + (r.stderr or "")
        if len(out) > 12000:
            out = out[:12000] + "\n…(truncated)"
        return out
    except subprocess.TimeoutExpired:
        return f"kubectl timed out after {timeout}s"
    except Exception as exc:  # noqa: BLE001
        return f"kubectl error: {exc}"


def _call(name: str, args: dict) -> dict:
    if name == "kube_get_pods":
        ns = args.get("namespace")
        if ns:
            out = _kubectl(["-n", ns, "get", "pods", "-o", "wide"])
        else:
            out = _kubectl(["get", "pods", "-A", "-o", "wide"])
        return {"content": [{"type": "text", "text": out}]}
    if name == "kube_logs":
        cmd = ["-n", args["namespace"], "logs", args["name"],
               "--tail", str(min(int(args.get("tail_lines", 80)), 500))]
        if args.get("container"):
            cmd += ["-c", args["container"]]
        out = _kubectl(cmd)
        return {"content": [{"type": "text", "text": out}]}
    if name == "kube_describe":
        cmd = ["describe", args["kind"], args["name"]]
        if args.get("namespace"):
            cmd = ["-n", args["namespace"]] + cmd
        out = _kubectl(cmd)
        return {"content": [{"type": "text", "text": out}]}
    if name == "kube_nodes":
        out = _kubectl(["get", "nodes", "-o", "wide"])
        return {"content": [{"type": "text", "text": out}]}
    if name == "kube_events":
        ns = args.get("namespace")
        if ns:
            cmd = ["-n", ns, "get", "events", "--sort-by=.lastTimestamp"]
        else:
            cmd = ["get", "events", "-A", "--sort-by=.lastTimestamp"]
        out = _kubectl(cmd)
        return {"content": [{"type": "text", "text": out[-6000:]}]}
    if name == "kube_top_pods":
        ns = args.get("namespace")
        cmd = ["top", "pods"] + (["-n", ns] if ns else ["-A"])
        out = _kubectl(cmd, timeout=20.0)
        return {"content": [{"type": "text", "text": out}]}
    if name == "kube_top_nodes":
        out = _kubectl(["top", "nodes"], timeout=20.0)
        return {"content": [{"type": "text", "text": out}]}
    if name == "kube_get":
        cmd = ["get", args["kind"]]
        if args.get("name"):
            cmd.append(args["name"])
            cmd += ["-o", "yaml"]
        elif args.get("label"):
            cmd += ["-l", args["label"], "-o", "wide"]
        else:
            cmd += ["-o", "wide"]
        if args.get("namespace"):
            cmd = ["-n", args["namespace"]] + cmd
        elif not args.get("name") or args.get("namespace") == "":
            # list across all namespaces if not pinpointing a single named
            # resource without an NS — safer default for the user.
            cmd = ["-A"] + cmd if "-A" not in cmd else cmd
        out = _kubectl(cmd)
        return {"content": [{"type": "text", "text": out}]}
    return {"content": [{"type": "text", "text": f"unknown tool: {name}"}], "isError": True}


def _handle(req: dict) -> dict | None:
    method = req.get("method")
    rid = req.get("id")
    if method == "initialize":
        return {
            "jsonrpc": "2.0", "id": rid,
            "result": {
                "protocolVersion": "2025-11-25",
                "capabilities": {"tools": {"listChanged": False}},
                "serverInfo": {"name": "jarvis_kube", "version": "0.1.0"},
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
                    "result": _call(params.get("name", ""), params.get("arguments") or {})}
        except Exception as exc:  # noqa: BLE001
            return {"jsonrpc": "2.0", "id": rid,
                    "error": {"code": -32603, "message": f"{type(exc).__name__}: {exc}",
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
