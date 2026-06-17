"""Stdio MCP server: launch + track cluster-side JARVIS agent runs.

This generalizes ``jarvis_delegate_mcp.py`` from "spawn ``claude -p`` as a
detached in-pod subprocess" to "create a TRACKED, INDEPENDENT k8s Job".
A heavy, long-running task no longer hogs the edge's voice loop, survives
the laptop closing, and is independently trackable via the k8s API.

Source-of-truth split (the key architectural decision):

  * k8s is the run REGISTRY + STATUS authority. Every run is a ``Job`` in
    namespace ``ai`` labeled ``jarvis-run=<id>`` / ``app=jarvis-runner``.
    The Job's ``.status`` (Active / Succeeded / Failed) IS the run state.
    ``list_runs`` / ``run_status`` read structured status from the API —
    exact, free, RBAC-gated, and auto-reaped by ``ttlSecondsAfterFinished``.
  * mem0 (user_id "runs") is the SECONDARY store for the human-readable
    result blurb only — what the Sonos announce loop narrates and what
    "what did that run find?" recall hits later. mem0 ``/add`` runs lossy
    LLM fact-extraction, so it is NOT used as the status table.

Three tools:

    launch_run(task, mode="read"|"apply") -> {run_id, status, message}
        Creates the runner Job. **OWNER-CONFIRMED TURNS ONLY** — this tool
        is in the OWNER warm-brain's MCP config only, and gate_and_respond
        gates the launch behind an explicit owner confirmation (Ship phase
        wires that). Python is the real gate; the description tells the
        model to PROPOSE first.
    list_runs(limit=10)  -> [{run_id, status, mode, age, task}]
    run_status(run_id)   -> {run_id, status, mode, started, finished, result?}

Talks to the in-cluster k8s API directly via urllib using the pod's
projected ServiceAccount token + CA bundle (no `kubernetes` python dep,
matching the thin-shim style of the other MCP servers). The Job is created
under the ``jarvis-runner`` SA (see rbac-runner.yaml) which can only
create/get/list Jobs in ns ``ai`` — the always-on voice pod's default
``jarvis-readonly`` SA stays read-only.

Local/uncommitted, like the rest of JARVIS.
"""
from __future__ import annotations

import json
import os
import random
import ssl
import string
import sys
import time
import traceback
import urllib.error
import urllib.request
from typing import Optional

# ── Config ──────────────────────────────────────────────────────────────
_NS = os.environ.get("JARVIS_RUNNER_NAMESPACE", "ai")
_RUNNER_IMAGE = os.environ.get(
    "JARVIS_RUNNER_IMAGE", "zot.hwcopeland.net/ai/jarvis-edge:latest")
_RUNNER_ENTRYPOINT = os.environ.get(
    "JARVIS_RUNNER_ENTRYPOINT", "runner/jarvis_runner_entrypoint.py")
_RUNNER_SA = os.environ.get("JARVIS_RUNNER_SA", "jarvis-runner")
_MEM0_URL = os.environ.get(
    "JARVIS_MEM0_URL", "http://jarvis-mem0.ai.svc.cluster.local:8800")
_MEM_SCOPE = os.environ.get("JARVIS_MEM_SCOPE", "runs")
_WORLD_SCOPE = os.environ.get("JARVIS_WORLD_SCOPE", "jarvis")
_DEFAULT_BUDGET = os.environ.get("JARVIS_RUNNER_BUDGET_USD", "3")

# In-cluster API access (projected SA token + CA bundle).
_SA_DIR = "/var/run/secrets/kubernetes.io/serviceaccount"
_K8S_HOST = os.environ.get("KUBERNETES_SERVICE_HOST", "kubernetes.default.svc")
_K8S_PORT = os.environ.get("KUBERNETES_SERVICE_PORT", "443")
_K8S_BASE = f"https://{_K8S_HOST}:{_K8S_PORT}"
_CA_PATH = os.path.join(_SA_DIR, "ca.crt")
# launch_run creates the runner Job using the edge pod's projected SA token.
# The edge runs as `jarvis-readonly`; an additive RoleBinding (see
# rbac-runner.yaml: jarvis-runner-edge) grants THAT SA only create/get/list/
# watch/delete jobs + read pods in ns ai — a narrow, intentional capability,
# NOT a broad widening. The runner Job PODS themselves still run as the
# dedicated `jarvis-runner` SA (set on the Job spec).
_TOKEN_PATH = os.path.join(_SA_DIR, "token")

_LABEL_APP = "jarvis-runner"
_TASK_ANNO_MAX = 4000          # k8s annotation values are bounded (~256KiB
_TASK_LABEL_VALUE_MAX = 1500   # total) — keep individual ones small.


# ── k8s API helpers (thin urllib shim, no kubernetes dep) ───────────────
def _sa_token() -> str:
    with open(_TOKEN_PATH, "r", encoding="utf-8") as fh:
        return fh.read().strip()


def _ssl_ctx() -> ssl.SSLContext:
    if os.path.exists(_CA_PATH):
        return ssl.create_default_context(cafile=_CA_PATH)
    # Fall back to system trust if the projected CA isn't present (e.g. a
    # standalone handshake test off-cluster). API calls will then fail
    # closed at request time, which is the safe direction.
    return ssl.create_default_context()


def _k8s_request(method: str, path: str, body: Optional[dict] = None,
                 timeout: float = 15.0) -> dict:
    """Call the in-cluster API. Returns parsed JSON. Raises on transport
    errors; surfaces API errors as the parsed error object (which has a
    ``status``/``message``) so callers can map them to friendly text."""
    url = _K8S_BASE + path
    data = json.dumps(body).encode("utf-8") if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", f"Bearer {_sa_token()}")
    req.add_header("Accept", "application/json")
    if data is not None:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=timeout,
                                    context=_ssl_ctx()) as resp:
            raw = resp.read().decode("utf-8")
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", "replace")
        try:
            return json.loads(raw)
        except json.JSONDecodeError:
            return {"kind": "Status", "status": "Failure",
                    "code": exc.code, "message": raw or str(exc)}


# ── mem0 (result blurb only) ────────────────────────────────────────────
def _mem0_search_run(run_id: str, timeout: float = 8.0) -> Optional[str]:
    """Pull the result blurb the runner entrypoint wrote for ``run_id`` from
    mem0's "runs" scope. Best-effort: returns None on any failure (k8s
    status remains authoritative)."""
    body = {"query": f"run {run_id} finished", "user_id": _MEM_SCOPE,
            "limit": 5}
    req = urllib.request.Request(
        _MEM0_URL.rstrip("/") + "/search",
        data=json.dumps(body).encode("utf-8"),
        method="POST", headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            payload = json.loads(resp.read().decode("utf-8"))
    except (urllib.error.URLError, json.JSONDecodeError, OSError):
        return None
    # mem0 /search returns {"results":[{"memory": "...", ...}, ...]} or a
    # bare list depending on version — handle both, prefer a hit mentioning
    # this run_id and "finished".
    results = payload.get("results", payload) if isinstance(payload, dict) \
        else payload
    if not isinstance(results, list):
        return None
    best = None
    for item in results:
        text = item.get("memory") if isinstance(item, dict) else str(item)
        if not text:
            continue
        if run_id in text and "finish" in text.lower():
            return text
        if best is None and run_id in text:
            best = text
    return best


# ── Job manifest construction ───────────────────────────────────────────
def _gen_run_id() -> str:
    rand4 = "".join(random.choices(string.ascii_lowercase + string.digits, k=4))
    return f"r{int(time.time())}{rand4}"


def _now_rfc3339() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def _truncate(text: str, limit: int) -> str:
    text = text or ""
    return text if len(text) <= limit else text[:limit - 1] + "…"


def _job_manifest(run_id: str, task: str, mode: str) -> dict:
    """The runner Job. Mirrors the jarvis-stack pod's creds-seeding +
    config-mount patterns, but on a FRESH emptyDir per Job (never the edge's
    live jarvis-state PVC subPath) so a run can't race the edge's OAuth
    token-refresh writes. No GPU request (claude is network-bound) so it
    can't starve whisper/chatterbox even though it co-locates on nixos-gpu.
    """
    name = f"jarvis-run-{run_id}"
    return {
        "apiVersion": "batch/v1",
        "kind": "Job",
        "metadata": {
            "name": name,
            "namespace": _NS,
            "labels": {
                "jarvis-run": run_id,
                "app": _LABEL_APP,
            },
            "annotations": {
                "jarvis.run/task": _truncate(task, _TASK_ANNO_MAX),
                "jarvis.run/mode": mode,
                "jarvis.run/requested-by": "owner",
                "jarvis.run/created-at": _now_rfc3339(),
            },
        },
        "spec": {
            "backoffLimit": 0,            # never silently retry a (maybe) mutating task
            "activeDeadlineSeconds": 3600,
            "ttlSecondsAfterFinished": 86400,   # 24h so announce loop can read post-run
            "template": {
                "metadata": {
                    "labels": {
                        "jarvis-run": run_id,
                        "app": _LABEL_APP,
                    },
                },
                "spec": {
                    "restartPolicy": "Never",
                    "serviceAccountName": _RUNNER_SA,
                    "nodeSelector": {"gpu": "rtx3070"},
                    "tolerations": [{
                        "key": "gpu", "operator": "Equal",
                        "value": "true", "effect": "NoSchedule",
                    }],
                    "imagePullSecrets": [{"name": "zot-pull-secret"}],
                    "securityContext": {
                        "runAsUser": 1000,
                        "runAsGroup": 100,
                    },
                    "initContainers": [{
                        # Seed claude OAuth creds into a FRESH emptyDir
                        # (claude-home) per Job, then run claude with HOME=/tmp
                        # so it resolves ~/.claude/.credentials.json there. The
                        # emptyDir (never the edge's live PVC mount) means the
                        # runner can self-refresh its copy without racing the
                        # edge's in-place OAuth-refresh writes.
                        #
                        # SOURCE PRIORITY — read the LIVE edge creds first, the
                        # Secret only as a last resort:
                        #   1. /live/.credentials.json — the edge's CURRENT
                        #      creds on the jarvis-state PVC (mounted read-only
                        #      here). This holds the LIVE, rotated refreshToken
                        #      the edge's claude self-refreshes with.
                        #   2. /src/claude_credentials.json — the Secret seed.
                        #      FALLBACK ONLY: OAuth refreshTokens ROTATE on use,
                        #      so the Secret's refreshToken was superseded the
                        #      first time the edge refreshed and is now DEAD —
                        #      seeding from it gives an expired accessToken AND a
                        #      dead refreshToken, so claude can't refresh and
                        #      401s ("Invalid authentication credentials"). The
                        #      live PVC copy is the only source with a working
                        #      refreshToken.
                        #
                        # CRITICAL: chown/chmod the /dst DIRECTORY (not just the
                        # file) to uid 1000. The seeded accessToken is usually
                        # already expired, so the runner's claude must refresh it
                        # on first call — an ATOMIC write (temp file + rename)
                        # INTO ~/.claude, which needs write permission on the
                        # directory. An emptyDir mount is created root-owned, so
                        # without this chown uid 1000 cannot write the refreshed
                        # creds and the run fails. The edge pod's init does the
                        # same `chown 1000:100 /dst` + `chmod 0700 /dst`.
                        "name": "claude-creds-init",
                        "image": "busybox:1.36",
                        "command": ["sh", "-c", (
                            "chown 1000:100 /dst && chmod 0700 /dst && "
                            "if [ -s /live/.credentials.json ]; then "
                            "  install -m 0600 /live/.credentials.json "
                            "/dst/.credentials.json && "
                            "  echo 'claude creds: seeded from LIVE edge PVC "
                            "(rotated refreshToken)'; "
                            "else "
                            "  install -m 0600 /src/claude_credentials.json "
                            "/dst/.credentials.json && "
                            "  echo 'claude creds: WARNING seeded from stale "
                            "Secret fallback (refreshToken likely dead -> may "
                            "401)'; "
                            "fi && "
                            "chown 1000:100 /dst/.credentials.json"
                        )],
                        "securityContext": {"runAsUser": 0, "runAsGroup": 0},
                        "volumeMounts": [
                            # Live edge creds on the jarvis-state PVC — READ
                            # ONLY. We only ever copy OUT of it into the
                            # emptyDir, so we never touch the edge's live file
                            # and can't race its refresh. RWO PVC co-mounts on
                            # the same node (nixos-gpu) where the edge already
                            # has it; the Job is pinned there via nodeSelector.
                            {"name": "jarvis-state-live", "mountPath": "/live",
                             "subPath": ".claude", "readOnly": True},
                            {"name": "claude-creds-src", "mountPath": "/src",
                             "readOnly": True},
                            {"name": "claude-home", "mountPath": "/dst"},
                        ],
                    }],
                    "containers": [{
                        "name": "runner",
                        "image": _RUNNER_IMAGE,
                        "command": ["python", "-u", _RUNNER_ENTRYPOINT],
                        "env": [
                            {"name": "RUNNER_RUN_ID", "value": run_id},
                            {"name": "RUNNER_TASK", "value": task},
                            {"name": "RUNNER_MODE", "value": mode},
                            {"name": "RUNNER_BUDGET_USD",
                             "value": str(_DEFAULT_BUDGET)},
                            {"name": "JARVIS_MEM_SCOPE", "value": _MEM_SCOPE},
                            {"name": "JARVIS_WORLD_SCOPE", "value": _WORLD_SCOPE},
                            {"name": "JARVIS_MEM0_URL", "value": _MEM0_URL},
                            {"name": "HOME", "value": "/tmp"},
                        ],
                        "resources": {
                            "requests": {"cpu": "250m", "memory": "512Mi"},
                            "limits": {"cpu": "2", "memory": "2Gi"},
                        },
                        "volumeMounts": [
                            {"name": "claude-home", "mountPath": "/tmp/.claude"},
                            {"name": "jarvis-config",
                             "mountPath": "/tmp/.openjarvis", "readOnly": True},
                        ],
                    }],
                    "volumes": [
                        {"name": "claude-home", "emptyDir": {}},
                        # Live edge creds (jarvis-state PVC). Mounted read-only
                        # in the init only — source for the LIVE rotated
                        # refreshToken. RWO PVC; safe because the Job is pinned
                        # to nixos-gpu (nodeSelector gpu=rtx3070) where the edge
                        # already mounts it, and RWO is per-node not per-pod.
                        {"name": "jarvis-state-live", "persistentVolumeClaim": {
                            "claimName": "jarvis-state", "readOnly": True}},
                        {"name": "claude-creds-src", "secret": {
                            "secretName": "jarvis-secrets",
                            "items": [{"key": "claude_credentials.json",
                                       "path": "claude_credentials.json"}],
                        }},
                        {"name": "jarvis-config", "secret": {
                            "secretName": "jarvis-secrets",
                        }},
                    ],
                },
            },
        },
    }


# ── Job status mapping ──────────────────────────────────────────────────
def _job_phase(job: dict) -> tuple[str, Optional[str]]:
    """Map a Job object to (status, reason). status in
    {Running, Succeeded, Failed, Pending}."""
    status = job.get("status") or {}
    conditions = status.get("conditions") or []
    for cond in conditions:
        if cond.get("type") == "Complete" and cond.get("status") == "True":
            return "Succeeded", cond.get("reason")
        if cond.get("type") == "Failed" and cond.get("status") == "True":
            return "Failed", cond.get("reason") or cond.get("message")
    if status.get("active"):
        return "Running", None
    if status.get("succeeded"):
        return "Succeeded", None
    if status.get("failed"):
        return "Failed", None
    return "Pending", None


def _age(job: dict) -> str:
    ts = (job.get("metadata") or {}).get("creationTimestamp")
    if not ts:
        return "?"
    try:
        created = time.mktime(time.strptime(ts, "%Y-%m-%dT%H:%M:%SZ"))
    except (ValueError, TypeError):
        return "?"
    secs = max(0, int(time.time() - created - time.timezone))
    if secs < 60:
        return f"{secs}s"
    if secs < 3600:
        return f"{secs // 60}m"
    if secs < 86400:
        return f"{secs // 3600}h"
    return f"{secs // 86400}d"


def _job_run_summary(job: dict) -> dict:
    meta = job.get("metadata") or {}
    anno = meta.get("annotations") or {}
    labels = meta.get("labels") or {}
    status, reason = _job_phase(job)
    out = {
        "run_id": labels.get("jarvis-run") or meta.get("name", ""),
        "status": status,
        "mode": anno.get("jarvis.run/mode", "read"),
        "age": _age(job),
        "task": anno.get("jarvis.run/task", ""),
    }
    if reason:
        out["reason"] = reason
    return out


# ── Tool implementations ────────────────────────────────────────────────
def launch_run(task: str, mode: str = "read") -> dict:
    """Create a cluster-side runner Job. OWNER-CONFIRMED turns only (the
    edge's gate enforces that; this tool exists only in the owner brain's
    MCP config). One launch_run call == one Job == one tracked run."""
    task = (task or "").strip()
    if not task:
        return {"status": "error", "message": "empty task"}
    mode = mode if mode in ("read", "apply") else "read"
    run_id = _gen_run_id()
    manifest = _job_manifest(run_id, task, mode)
    try:
        resp = _k8s_request(
            "POST", f"/apis/batch/v1/namespaces/{_NS}/jobs", body=manifest)
    except (urllib.error.URLError, OSError) as exc:
        return {"status": "error", "run_id": run_id,
                "message": f"k8s API unreachable: {exc}"}
    if resp.get("kind") == "Status" and resp.get("status") == "Failure":
        return {"status": "error", "run_id": run_id,
                "message": f"Job create failed: {resp.get('message')}"}
    posture = ("read-only investigation" if mode == "read"
               else "apply mode (writes via jarvis branch)")
    return {
        "status": "launched",
        "run_id": run_id,
        "mode": mode,
        "message": (f"Cluster run {run_id} launched ({posture}, "
                    f"${_DEFAULT_BUDGET} cap, 1h deadline). I'll announce the "
                    f"result on the speakers when it finishes."),
    }


def list_runs(limit: int = 10) -> list[dict]:
    """List recent runner Jobs (newest first) mapped to run summaries."""
    limit = max(1, min(int(limit or 10), 50))
    path = (f"/apis/batch/v1/namespaces/{_NS}/jobs"
            f"?labelSelector=app%3D{_LABEL_APP}")
    try:
        resp = _k8s_request("GET", path)
    except (urllib.error.URLError, OSError) as exc:
        return [{"status": "error", "message": f"k8s API unreachable: {exc}"}]
    if resp.get("kind") == "Status" and resp.get("status") == "Failure":
        return [{"status": "error", "message": resp.get("message", "list failed")}]
    items = resp.get("items") or []
    items.sort(
        key=lambda j: (j.get("metadata") or {}).get("creationTimestamp", ""),
        reverse=True)
    return [_job_run_summary(j) for j in items[:limit]]


def run_status(run_id: str) -> dict:
    """Status of a single run. Status is authoritative from k8s; if finished,
    pull the human-readable result blurb from mem0's "runs" scope (falling
    back to a Job result annotation if the entrypoint wrote one)."""
    run_id = (run_id or "").strip()
    if not run_id:
        return {"status": "error", "message": "empty run_id"}
    path = (f"/apis/batch/v1/namespaces/{_NS}/jobs"
            f"?labelSelector=jarvis-run%3D{run_id}")
    try:
        resp = _k8s_request("GET", path)
    except (urllib.error.URLError, OSError) as exc:
        return {"status": "error", "run_id": run_id,
                "message": f"k8s API unreachable: {exc}"}
    if resp.get("kind") == "Status" and resp.get("status") == "Failure":
        return {"status": "error", "run_id": run_id,
                "message": resp.get("message", "status read failed")}
    items = resp.get("items") or []
    if not items:
        return {"status": "not_found", "run_id": run_id,
                "message": (f"No run {run_id} (may have been reaped after its "
                            f"24h TTL).")}
    job = items[0]
    summary = _job_run_summary(job)
    meta = job.get("metadata") or {}
    jstatus = job.get("status") or {}
    out = {
        "run_id": run_id,
        "status": summary["status"],
        "mode": summary["mode"],
        "task": summary["task"],
        "started": jstatus.get("startTime"),
        "finished": jstatus.get("completionTime"),
    }
    if summary.get("reason"):
        out["reason"] = summary["reason"]
    if summary["status"] in ("Succeeded", "Failed"):
        # Result blurb: prefer mem0 (durable, narratable), fall back to a
        # Job annotation the entrypoint may have written.
        blurb = _mem0_search_run(run_id)
        if not blurb:
            blurb = (meta.get("annotations") or {}).get("jarvis.run/result")
        if blurb:
            out["result"] = blurb
    return out


# ── MCP server boilerplate (same shape as jarvis_delegate_mcp.py) ────────
_TOOLS = [
    {
        "name": "launch_run",
        "description": (
            "Launch a LONG-RUNNING task as a tracked, independent cluster-side "
            "k8s Job that survives your laptop closing. Use this (not "
            "`delegate`) for heavy/slow work the owner wants to fire-and-forget "
            "— the result is announced on the speakers when it finishes.\n\n"
            "**PROPOSE FIRST, then confirm.** Do NOT call launch_run on the "
            "owner's first request. First say: \"Sir, I'll launch a cluster run "
            "to <X> in <read|apply> mode. Confirm?\" — and only call launch_run "
            "AFTER the owner explicitly confirms on a following turn. (The edge "
            "also enforces this; only the owner can ever reach this tool.)\n\n"
            "Modes:\n"
            "  • read (default) — read-only investigation: kubectl get/describe/"
            "logs, git log/diff, curl, file reads. Use for 'go figure out why X "
            "is broken and tell me'.\n"
            "  • apply — the run may change files / kubectl apply / commit+push "
            "on the `jarvis` branch (never main). **Only select apply when the "
            "owner explicitly said make / apply / fix / deploy / commit.**\n\n"
            "Returns immediately with a run_id. Track with list_runs / "
            "run_status; the result is also spoken on Sonos on completion."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "task": {"type": "string",
                         "description": "Clear, self-contained task for the cluster run."},
                "mode": {"type": "string", "enum": ["read", "apply"],
                         "description": "read (default, read-only) | apply (may mutate via jarvis branch)."},
            },
            "required": ["task"],
            "additionalProperties": False,
        },
    },
    {
        "name": "list_runs",
        "description": (
            "List recent cluster runs (newest first) with their live status "
            "(Running / Succeeded / Failed / Pending), mode, age, and task. Use "
            "to answer 'what runs are going' or 'did that finish'."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "limit": {"type": "integer", "minimum": 1, "maximum": 50,
                          "description": "Max runs to return (default 10)."},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "run_status",
        "description": (
            "Get the status of one run by its run_id. Status is authoritative "
            "from the k8s Job; if the run has finished, `result` carries the "
            "human-readable result blurb (for 'what did that run find?')."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "run_id": {"type": "string",
                           "description": "The run_id returned by launch_run / list_runs."},
            },
            "required": ["run_id"],
            "additionalProperties": False,
        },
    },
]


def _text_result(obj) -> dict:
    return {"content": [{"type": "text", "text": json.dumps(obj)}]}


def _call(name: str, args: dict) -> dict:
    if name == "launch_run":
        return _text_result(launch_run(
            task=args.get("task", ""), mode=args.get("mode", "read")))
    if name == "list_runs":
        return _text_result(list_runs(limit=args.get("limit", 10)))
    if name == "run_status":
        return _text_result(run_status(run_id=args.get("run_id", "")))
    return {"content": [{"type": "text", "text": f"unknown tool: {name}"}],
            "isError": True}


def _handle(req: dict):
    method = req.get("method")
    rid = req.get("id")
    if method == "initialize":
        return {"jsonrpc": "2.0", "id": rid, "result": {
            "protocolVersion": "2025-11-25",
            "capabilities": {"tools": {"listChanged": False}},
            "serverInfo": {"name": "jarvis_runner", "version": "0.1.0"},
        }}
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
