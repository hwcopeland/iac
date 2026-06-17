"""Completion-announce loop for cluster-side agent runs (charter roadmap #2).

A voice-initiated long-running task is launched as an independent k8s Job
(see jarvis_runner_mcp.launch_run) labeled `app=jarvis-runner` and
`jarvis-run=<id>` in namespace `ai`. That Job survives the laptop closing.
THIS module is the other half: a daemon thread on the always-on edge pod
that periodically polls the k8s API for finished runner Jobs, pulls the
human-readable result blurb from mem0's "runs" scope, and SPEAKS it on
Sonos via the edge's existing `_stream_on_sonos` path:

    "Sir, the <task> run has finished: <short result>."

Source-of-truth split (matches the design):
  * k8s Job .status is the STATUS AUTHORITY (Succeeded / Failed / running).
    We read it via the pod's projected SA token — the edge's `jarvis-readonly`
    SA already permits get/list jobs, so this thread needs NO new RBAC.
  * mem0 user_id="runs" is the NARRATION store — the runner entrypoint wrote
    a "run <id> finished (ok|error): <blurb>" fact there. We read it for the
    spoken result. If mem0 is unreachable or has nothing, we fall back to the
    Job's `jarvis.run/result` annotation, then to a generic phrase.

Design constraints honored:
  * NON-BLOCKING w.r.t. the mic loop. Runs in its own daemon thread.
  * SPEAK ONCE per run. Announced run_ids are persisted to
    /state/announced_runs.json so a pod restart never re-announces a run nor
    drops a completion it hadn't spoken yet.
  * Never collide mid-sentence with a live owner turn. Before streaming we
    acquire a shared speaking lock (passed in from edge.main) so an
    announcement waits its turn exactly like serialized owner turns do today.
  * FAIL-OPEN. Any error in the loop is logged and swallowed — the voice
    loop must never go down because the announce thread hiccupped.

This module is import-safe (no work at import time) and has ZERO third-party
deps — stdlib only — so it cannot break the edge image's import graph.

edge.py integration (Ship phase — ONE call in main(), see start_announce_thread):

    import jarvis_run_announce
    jarvis_run_announce.start_announce_thread(
        sonos=sonos, host_ip=host_ip,
        http_port=EDGE_ADVERTISED_PORT,
        stream_fn=_stream_on_sonos, stash=stash,
        speak_lock=_speak_lock,          # a threading.Lock shared with mic loop
    )
"""
from __future__ import annotations

import json
import os
import ssl
import threading
import time
import urllib.parse
import urllib.request

# ── Config (all env-overridable; safe defaults) ──────────────────────────
RUN_ANNOUNCE_ENABLED = os.environ.get("RUN_ANNOUNCE_ENABLED", "1") == "1"
ANNOUNCE_POLL_S = float(os.environ.get("RUN_ANNOUNCE_POLL_S", "20"))
RUN_NAMESPACE = os.environ.get("JARVIS_RUN_NAMESPACE", "ai")
RUN_LABEL_APP = "app=jarvis-runner"
MEM0_URL = os.environ.get("MEM0_URL", "http://jarvis-mem0.ai.svc.cluster.local:8800")
MEM0_RUNS_SCOPE = os.environ.get("JARVIS_RUNS_SCOPE", "runs")
ANNOUNCED_PATH = os.environ.get("RUN_ANNOUNCED_PATH", "/state/announced_runs.json")
RESULT_MAX_CHARS = int(os.environ.get("RUN_ANNOUNCE_RESULT_CHARS", "320"))

# In-cluster SA token / CA bundle (same paths kubectl uses). Read lazily so
# import never fails off-cluster (e.g. during standalone unit tests).
_SA_DIR = "/var/run/secrets/kubernetes.io/serviceaccount"
_K8S_API = os.environ.get("KUBERNETES_API", "https://kubernetes.default.svc")


def _log(msg: str) -> None:
    print(f"run-announce: {msg}", flush=True)


# ── k8s API (read-only, via projected SA token) ──────────────────────────
def _read_sa_token() -> str:
    with open(os.path.join(_SA_DIR, "token"), "r") as f:
        return f.read().strip()


def _k8s_ssl_ctx() -> ssl.SSLContext:
    ca = os.path.join(_SA_DIR, "ca.crt")
    if os.path.exists(ca):
        return ssl.create_default_context(cafile=ca)
    # No CA on disk (off-cluster / test) — fall back to default context.
    return ssl.create_default_context()


def _list_runner_jobs() -> list[dict]:
    """GET finished+running runner Jobs in the ai namespace. Returns the
    raw .items list. Raises on transport/auth error (caller handles)."""
    token = _read_sa_token()
    path = (f"/apis/batch/v1/namespaces/{RUN_NAMESPACE}/jobs"
            f"?labelSelector={urllib.parse.quote(RUN_LABEL_APP)}")
    req = urllib.request.Request(
        _K8S_API + path,
        headers={"Authorization": f"Bearer {token}",
                 "Accept": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=10.0,
                                context=_k8s_ssl_ctx()) as r:
        body = json.loads(r.read().decode("utf-8"))
    return body.get("items", []) or []


# ── Job → run-state mapping ──────────────────────────────────────────────
def _job_phase(job: dict) -> str:
    """Map a Job object to one of: running | succeeded | failed | unknown.

    k8s Job status: .status.succeeded>=1 => Succeeded; a condition
    type=Failed (incl. DeadlineExceeded -> backoffLimit 0) => Failed;
    .status.active>=1 => Running. We treat DeadlineExceeded as failed."""
    status = job.get("status", {}) or {}
    if (status.get("succeeded") or 0) >= 1:
        return "succeeded"
    for cond in status.get("conditions", []) or []:
        if cond.get("type") == "Failed" and cond.get("status") == "True":
            return "failed"
    if (status.get("active") or 0) >= 1:
        return "running"
    # No active/succeeded/failed yet (just created, pod pending).
    return "unknown"


def _run_id_of(job: dict) -> str:
    labels = (job.get("metadata", {}) or {}).get("labels", {}) or {}
    rid = labels.get("jarvis-run")
    if rid:
        return rid
    # Fall back to the Job name (jarvis-run-<id>) if the label is missing.
    return (job.get("metadata", {}) or {}).get("name", "")


def _task_label_of(job: dict) -> str:
    """Short human task name for the spoken sentence. Prefer the
    jarvis.run/task annotation; trim to a clause."""
    ann = (job.get("metadata", {}) or {}).get("annotations", {}) or {}
    task = ann.get("jarvis.run/task", "") or ""
    task = task.strip()
    if not task:
        return "background"
    # Keep it short — first ~8 words, no trailing punctuation.
    words = task.split()
    short = " ".join(words[:8])
    return short.rstrip(".!?,;:")


def _result_annotation_of(job: dict) -> str:
    ann = (job.get("metadata", {}) or {}).get("annotations", {}) or {}
    return (ann.get("jarvis.run/result", "") or "").strip()


# ── mem0 narration lookup ────────────────────────────────────────────────
def _mem0_result_blurb(run_id: str) -> str:
    """Search the mem0 'runs' scope for this run's result fact. Returns ''
    on any failure — caller falls back to the Job annotation. Best-effort,
    never raises."""
    try:
        payload = json.dumps({
            "query": f"run {run_id} finished result",
            "user_id": MEM0_RUNS_SCOPE,
            "limit": 5,
        }).encode("utf-8")
        req = urllib.request.Request(
            MEM0_URL.rstrip("/") + "/search",
            data=payload,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib.request.urlopen(req, timeout=8.0) as r:
            body = json.loads(r.read().decode("utf-8"))
    except Exception as exc:
        _log(f"mem0 search failed for {run_id} (using fallback): {exc!r}")
        return ""
    # Response shape is {"results":[{"memory":...,"score":...}]} or a bare
    # list — handle both defensively.
    items = body.get("results", body) if isinstance(body, dict) else body
    if not isinstance(items, list):
        return ""
    # Prefer a fact that names this run_id and "finished"; else the top hit.
    best = ""
    for it in items:
        text = ""
        if isinstance(it, dict):
            text = it.get("memory") or it.get("text") or it.get("name") or ""
        elif isinstance(it, str):
            text = it
        text = (text or "").strip()
        if not text:
            continue
        if run_id in text and "finish" in text.lower():
            return text
        if not best:
            best = text
    return best


def _strip_run_prefix(blurb: str, run_id: str) -> str:
    """The runner writes 'run <id> finished (ok): <blurb>'. For narration we
    want just the <blurb>. Strip the bookkeeping prefix if present."""
    marker = "):"
    low = blurb.lower()
    if low.startswith("run ") and marker in blurb and run_id in blurb[:80]:
        return blurb.split(marker, 1)[1].strip()
    return blurb


def _compose_sentence(phase: str, task_label: str, result: str) -> str:
    """Build the spoken sentence. Succeeded vs failed phrasing."""
    result = (result or "").strip()
    if len(result) > RESULT_MAX_CHARS:
        result = result[:RESULT_MAX_CHARS].rstrip() + "…"
    if phase == "failed":
        if result:
            return f"Sir, the {task_label} run failed: {result}"
        return f"Sir, the {task_label} run failed."
    # succeeded
    if result:
        return f"Sir, the {task_label} run has finished: {result}"
    return f"Sir, the {task_label} run has finished."


# ── announced-set persistence ────────────────────────────────────────────
def _load_announced() -> set[str]:
    try:
        with open(ANNOUNCED_PATH, "r") as f:
            data = json.load(f)
        if isinstance(data, list):
            return set(str(x) for x in data)
    except FileNotFoundError:
        return set()
    except Exception as exc:
        _log(f"announced-set load failed (starting empty): {exc!r}")
    return set()


def _save_announced(announced: set[str]) -> None:
    try:
        tmp = ANNOUNCED_PATH + ".tmp"
        with open(tmp, "w") as f:
            json.dump(sorted(announced), f)
        os.replace(tmp, ANNOUNCED_PATH)
    except Exception as exc:
        _log(f"announced-set save failed (will retry next tick): {exc!r}")


# ── the loop ─────────────────────────────────────────────────────────────
def announce_once(sonos, host_ip, http_port, stream_fn, stash,
                  speak_lock, announced: set[str]) -> int:
    """One poll pass. Speaks any newly-finished, not-yet-announced runs.
    Mutates and persists `announced`. Returns the number of runs announced
    this pass. Never raises (fail-open)."""
    try:
        jobs = _list_runner_jobs()
    except Exception as exc:
        _log(f"k8s list failed (skipping this tick): {exc!r}")
        return 0

    spoken = 0
    for job in jobs:
        run_id = _run_id_of(job)
        if not run_id or run_id in announced:
            continue
        phase = _job_phase(job)
        if phase not in ("succeeded", "failed"):
            continue  # still running / pending — leave for a later tick.

        task_label = _task_label_of(job)
        blurb = _mem0_result_blurb(run_id)
        if not blurb:
            blurb = _result_annotation_of(job)
        blurb = _strip_run_prefix(blurb, run_id)
        sentence = _compose_sentence(phase, task_label, blurb)
        _log(f"announcing {run_id} ({phase}): {sentence!r}")

        # Use a synthetic turn number so _stream_on_sonos's per-turn WAV
        # stash keys don't collide with the mic loop's turn_n. Negative =
        # "announcement turn", monotonically distinct per run.
        synthetic_turn = -(abs(hash(run_id)) % 1_000_000) - 1

        # Serialize against live owner turns: wait for the shared speaking
        # lock so we never start mid-sentence over the mic loop. The mic
        # loop holds this lock for the duration of its own _stream_on_sonos.
        got_lock = True
        if speak_lock is not None:
            got_lock = speak_lock.acquire(timeout=120.0)
            if not got_lock:
                _log(f"speak lock busy >120s; deferring {run_id} to next tick")
                continue
        try:
            stream_fn(sonos, [sentence], host_ip, http_port,
                      synthetic_turn, stash)
        except Exception as exc:
            # Sonos hiccup — do NOT mark announced; retry next tick.
            _log(f"sonos stream failed for {run_id} (will retry): {exc!r}")
            continue
        finally:
            if speak_lock is not None and got_lock:
                speak_lock.release()

        # Only mark announced AFTER a successful stream — guarantees
        # speak-once without dropping a completion on a transient failure.
        announced.add(run_id)
        _save_announced(announced)
        spoken += 1
    return spoken


def _run_loop(sonos, host_ip, http_port, stream_fn, stash, speak_lock):
    announced = _load_announced()
    _log(f"loop started (poll={ANNOUNCE_POLL_S}s, ns={RUN_NAMESPACE}, "
         f"known-announced={len(announced)})")
    while True:
        try:
            announce_once(sonos, host_ip, http_port, stream_fn, stash,
                          speak_lock, announced)
        except Exception as exc:  # defensive — announce_once is already safe
            _log(f"unexpected loop error (continuing): {exc!r}")
        time.sleep(ANNOUNCE_POLL_S)


def start_announce_thread(sonos, host_ip, http_port, stream_fn, stash,
                          speak_lock=None):
    """Start the completion-announce daemon thread. FAIL-OPEN: any failure
    to start is logged and swallowed so the voice loop is never affected.

    Args:
      sonos:      the soco device handle from edge.main (may be None — then
                  _stream_on_sonos writes WAVs to /tmp instead of speaking).
      host_ip:    edge host IP Sonos fetches WAVs from (edge.main resolved).
      http_port:  EDGE_ADVERTISED_PORT (the URL port Sonos hits).
      stream_fn:  edge._stream_on_sonos (passed in to avoid an import cycle).
      stash:      edge's _AudioStash instance (shared WAV cache).
      speak_lock: a threading.Lock shared with the mic loop so announcements
                  never interleave with a live owner turn. If None, no
                  serialization (acceptable but not recommended).
    """
    if not RUN_ANNOUNCE_ENABLED:
        _log("disabled via RUN_ANNOUNCE_ENABLED=0")
        return None
    try:
        t = threading.Thread(
            target=_run_loop,
            args=(sonos, host_ip, http_port, stream_fn, stash, speak_lock),
            name="run-announce",
            daemon=True,
        )
        t.start()
        _log("thread started")
        return t
    except Exception as exc:
        _log(f"failed to start (voice loop unaffected): {exc!r}")
        return None
