#!/usr/bin/env python3
"""worldsync_metrics — map cluster HEALTH/METRICS signal into JARVIS unified memory.

This is a RE-RUNNABLE SNAPSHOT sync, not a history recorder. Each run queries a
small, curated set of current-state signals from Prometheus and writes concise
"current state" facts into mem0 (user_id "jarvis") via POST /add. Re-run it (cron
or on demand) to keep JARVIS's understanding of the homelab fresh.

  *** EXPLICITLY NOT FROZEN HISTORY ***
  Every fact is phrased as a present-tense snapshot ("Node X CPU is N%") and
  carries a [worldsync:metrics] tag plus an "as of <UTC>" timestamp so a later
  re-run supersedes the previous reading instead of stacking duplicates. mem0's
  extraction/embedding layer collapses semantically-equal facts (idempotent
  upsert by design); this script does NOT attempt to delete prior rows itself.

WHY snapshot-not-history: the charter's ONE idea is that mem0 IS JARVIS's live
understanding of the homelab. Health/metrics are inherently "now" facts — a
CPU reading from last week is noise, not memory. So we only ever assert the
current value, tagged so it overwrites cleanly.

Curated signal set (intentionally SMALL — high signal, low chatter):
  1. Per-node CPU busy %, memory used %, root-fs used % (only notable values).
  2. GPU utilization + memory (DCGM), per GPU.
  3. Pods not Ready / crashlooping (restarts in the last hour).
  4. Firing Prometheus alerts (excluding the always-on Watchdog).
  5. A one-line cluster summary (node count, healthy/total pods).

Data sources (verified live 2026-06-16):
  - Prometheus: kube-prometheus-stack-prometheus.monitor.svc.cluster.local:9090
  - mem0 REST:  jarvis-mem0.ai.svc.cluster.local:8800  (POST /add {text,user_id})

NOTE: as of writing, the mem0 server is broken (returns HTTP 400
"Unsupported embedding provider: fastembed"). This script handles /add failures
gracefully: it logs the per-fact result and exits non-zero if NONE landed, so it
is safe to wire into a CronJob now and starts working the moment mem0 is fixed.

Run modes:
  python3 worldsync_metrics.py            # query + write to mem0
  python3 worldsync_metrics.py --dry-run  # query + print facts, no writes
  python3 worldsync_metrics.py --print    # alias for --dry-run

Config via env (all optional, sensible in-cluster defaults):
  PROM_URL   default http://kube-prometheus-stack-prometheus.monitor.svc.cluster.local:9090
  MEM0_URL   default http://jarvis-mem0.ai.svc.cluster.local:8800
  MEM0_USER  default jarvis
  HTTP_TIMEOUT seconds, default 15

Local/uncommitted like the rest of JARVIS.
"""
from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone

PROM_URL = os.environ.get(
    "PROM_URL",
    "http://kube-prometheus-stack-prometheus.monitor.svc.cluster.local:9090",
).rstrip("/")
MEM0_URL = os.environ.get(
    "MEM0_URL", "http://jarvis-mem0.ai.svc.cluster.local:8800"
).rstrip("/")
MEM0_USER = os.environ.get("MEM0_USER", "jarvis")
HTTP_TIMEOUT = float(os.environ.get("HTTP_TIMEOUT", "15"))

TAG = "[worldsync:metrics]"

# Thresholds for "notable" — below these we don't bother JARVIS with a fact,
# keeping the memory high-signal. (A node sitting at 8% CPU is not worth a fact.)
CPU_NOTABLE = 70.0     # %
MEM_NOTABLE = 80.0     # %
DISK_NOTABLE = 75.0    # %
GPU_NOTABLE = 5.0      # % util — GPUs are rare/expensive, lower bar


def _now_utc() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")


# ---------------------------------------------------------------------------
# Prometheus
# ---------------------------------------------------------------------------
def prom_query(expr: str) -> list[dict]:
    """Run an instant query, return the raw result vector (list of metric dicts)."""
    data = urllib.parse.urlencode({"query": expr}).encode()
    req = urllib.request.Request(
        f"{PROM_URL}/api/v1/query", data=data,
        headers={"Content-Type": "application/x-www-form-urlencoded"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as r:
        out = json.loads(r.read() or b"{}")
    if out.get("status") != "success":
        raise RuntimeError(f"prometheus query failed: {expr!r}: {out}")
    return out.get("data", {}).get("result", [])


def _val(m: dict) -> float:
    return float(m["value"][1])


# ---------------------------------------------------------------------------
# Signal collectors -> list[str] of concise current-state facts
# ---------------------------------------------------------------------------
def collect_node_pressure() -> list[str]:
    facts: list[str] = []
    ts = _now_utc()

    cpu = prom_query(
        '(1 - avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m]))) '
        '* 100 * on(instance) group_left(nodename) node_uname_info'
    )
    for m in cpu:
        v = _val(m)
        if v >= CPU_NOTABLE:
            node = m["metric"].get("nodename", m["metric"].get("instance", "?"))
            facts.append(
                f"{TAG} Node {node} CPU usage is high at {v:.0f}% as of {ts}."
            )

    mem = prom_query(
        "(1 - node_memory_MemAvailable_bytes/node_memory_MemTotal_bytes)*100 "
        "* on(instance) group_left(nodename) node_uname_info"
    )
    for m in mem:
        v = _val(m)
        if v >= MEM_NOTABLE:
            node = m["metric"].get("nodename", m["metric"].get("instance", "?"))
            facts.append(
                f"{TAG} Node {node} memory usage is high at {v:.0f}% as of {ts}."
            )

    disk = prom_query(
        '(1 - node_filesystem_avail_bytes{mountpoint="/"}'
        '/node_filesystem_size_bytes{mountpoint="/"})*100 '
        '* on(instance) group_left(nodename) node_uname_info'
    )
    for m in disk:
        v = _val(m)
        if v >= DISK_NOTABLE:
            node = m["metric"].get("nodename", m["metric"].get("instance", "?"))
            facts.append(
                f"{TAG} Node {node} root disk is {v:.0f}% full as of {ts}."
            )
    return facts


def collect_gpu() -> list[str]:
    facts: list[str] = []
    ts = _now_utc()
    try:
        util = prom_query("DCGM_FI_DEV_GPU_UTIL")
    except Exception:
        return facts  # DCGM may be absent; not an error
    # mem used in MiB (DCGM_FI_DEV_FB_USED is MiB)
    mem_used = {}
    try:
        for m in prom_query("DCGM_FI_DEV_FB_USED"):
            key = (m["metric"].get("Hostname"), m["metric"].get("gpu"))
            mem_used[key] = _val(m)
    except Exception:
        mem_used = {}
    for m in util:
        v = _val(m)
        if v < GPU_NOTABLE:
            continue
        md = m["metric"]
        model = md.get("modelName", "GPU")
        host = md.get("Hostname", "?")
        gpu = md.get("gpu", "0")
        fb = mem_used.get((host, gpu))
        fbtxt = f", {fb:.0f} MiB VRAM used" if fb is not None else ""
        facts.append(
            f"{TAG} GPU {gpu} ({model}) is at {v:.0f}% utilization"
            f"{fbtxt} as of {ts}."
        )
    return facts


def collect_unhealthy_pods() -> list[str]:
    facts: list[str] = []
    ts = _now_utc()

    # Pods stuck in a non-Running/Succeeded phase right now.
    try:
        bad = prom_query(
            'kube_pod_status_phase{phase!~"Running|Succeeded"} == 1'
        )
    except Exception:
        bad = []
    notready = []
    for m in bad:
        md = m["metric"]
        notready.append(f"{md.get('namespace','?')}/{md.get('pod','?')} ({md.get('phase','?')})")
    if notready:
        joined = ", ".join(sorted(set(notready))[:15])
        facts.append(f"{TAG} Pods not Running as of {ts}: {joined}.")

    # Containers that restarted in the last hour (crashloop signal).
    try:
        restarts = prom_query(
            "increase(kube_pod_container_status_restarts_total[1h]) > 0"
        )
    except Exception:
        restarts = []
    crash = []
    for m in restarts:
        md = m["metric"]
        n = _val(m)
        crash.append(
            f"{md.get('namespace','?')}/{md.get('pod','?')} ({n:.0f}x)"
        )
    if crash:
        joined = ", ".join(sorted(set(crash))[:15])
        facts.append(
            f"{TAG} Containers restarting in the last hour as of {ts}: {joined}."
        )
    return facts


def collect_alerts() -> list[str]:
    facts: list[str] = []
    ts = _now_utc()
    try:
        # Exclude Watchdog (always-on heartbeat alert, not a real problem).
        firing = prom_query(
            'ALERTS{alertstate="firing",alertname!="Watchdog"}'
        )
    except Exception:
        firing = []
    items = []
    for m in firing:
        md = m["metric"]
        name = md.get("alertname", "?")
        sev = md.get("severity", "")
        ns = md.get("namespace", "")
        loc = f" in {ns}" if ns else ""
        sevtxt = f" [{sev}]" if sev else ""
        items.append(f"{name}{sevtxt}{loc}")
    if items:
        # Dedup while preserving a stable order; cap length.
        seen, dedup = set(), []
        for it in items:
            if it not in seen:
                seen.add(it)
                dedup.append(it)
        joined = "; ".join(dedup[:20])
        facts.append(f"{TAG} Prometheus alerts firing as of {ts}: {joined}.")
    else:
        facts.append(f"{TAG} No Prometheus alerts firing (besides Watchdog) as of {ts}.")
    return facts


def collect_cluster_summary() -> list[str]:
    ts = _now_utc()
    try:
        nodes = prom_query("count(node_uname_info)")
        n_nodes = int(_val(nodes[0])) if nodes else 0
    except Exception:
        n_nodes = 0
    try:
        running = prom_query('count(kube_pod_status_phase{phase="Running"} == 1)')
        n_run = int(_val(running[0])) if running else 0
    except Exception:
        n_run = 0
    return [
        f"{TAG} Cluster has {n_nodes} nodes with {n_run} pods Running "
        f"as of {ts}."
    ]


def collect_all_facts() -> list[str]:
    facts: list[str] = []
    facts += collect_cluster_summary()
    facts += collect_node_pressure()
    facts += collect_gpu()
    facts += collect_unhealthy_pods()
    facts += collect_alerts()
    return facts


# ---------------------------------------------------------------------------
# mem0 write
# ---------------------------------------------------------------------------
def mem0_add(text: str) -> tuple[bool, str]:
    payload = json.dumps({"text": text, "user_id": MEM0_USER}).encode()
    req = urllib.request.Request(
        f"{MEM0_URL}/add", data=payload,
        headers={"Content-Type": "application/json"}, method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as r:
            body = (r.read() or b"").decode()
        return True, body[:200]
    except urllib.error.HTTPError as e:
        return False, f"HTTP {e.code}: {(e.read() or b'').decode()[:200]}"
    except urllib.error.URLError as e:
        return False, f"unreachable: {e}"


def main() -> int:
    dry = any(a in ("--dry-run", "--print", "-n") for a in sys.argv[1:])

    try:
        facts = collect_all_facts()
    except Exception as e:  # noqa: BLE001
        print(f"FATAL: could not query Prometheus at {PROM_URL}: {e}",
              file=sys.stderr)
        return 2

    if not facts:
        print("no facts produced (nothing notable)", file=sys.stderr)
        return 0

    print(f"# worldsync_metrics — {len(facts)} fact(s), user_id={MEM0_USER}")
    print(f"#   PROM_URL={PROM_URL}")
    print(f"#   MEM0_URL={MEM0_URL}{'  (DRY RUN, not writing)' if dry else ''}")
    for f in facts:
        print(f"  - {f}")

    if dry:
        return 0

    ok = 0
    for f in facts:
        success, msg = mem0_add(f)
        status = "OK" if success else "FAIL"
        print(f"[{status}] {f[:80]}... -> {msg}")
        if success:
            ok += 1

    print(f"# wrote {ok}/{len(facts)} fact(s) to mem0")
    # If mem0 is broken (current known state), nothing lands — surface that as
    # a non-zero exit so a CronJob marks the run as failed and stays loud.
    return 0 if ok > 0 else 1


if __name__ == "__main__":
    sys.exit(main())
