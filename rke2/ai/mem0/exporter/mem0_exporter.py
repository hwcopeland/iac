#!/usr/bin/env python3
"""mem0_exporter — Prometheus exporter for JARVIS unified memory (mem0/qdrant).

WHY this exists: qdrant's own /metrics only gives collection-wide totals
(jarvis_mem0 has N points). The dashboard needs to break memory down by
SCOPE (which user_id owns the fact: the "jarvis" world scope vs the "runs"
scope vs the "owner" scope) and by PROJECT/domain (a payload field, e.g.
worldsync domain "workload"/"db"). Those breakdowns are payload-derived and
qdrant won't compute them without a payload index we deliberately do NOT
create (this exporter is strictly READ-ONLY — it never mutates the collection).

HOW: every scrape we
  1. ask qdrant for the cheap authoritative total via /points/count, and
  2. scroll the whole collection (paged, vectors off) and tally points per
     scope/project/source/domain in Python.
The jarvis_mem0 collection is small (~10^3 points), so a full scroll per
scrape is inexpensive. If it ever grows large, raise the scrape interval or
add a payload index + facet path — but for now scroll is simplest & robust.

It also probes the mem0 REST server's /health so the dashboard can show
whether the memory engine itself is initialized.

Stdlib only — NO prometheus_client, NO custom image. Runs on a stock
python:3-slim with this file mounted from a ConfigMap. That keeps it
additive/safe (no in-cluster image build needed).

Exposed metrics (all gauges unless noted):
  jarvis_mem0_points{scope,project}      points per (scope, project) pair
  jarvis_mem0_points_by_scope{scope}     points per scope (user_id)
  jarvis_mem0_points_by_source{source}   points per source payload value
  jarvis_mem0_points_by_domain{domain}   points per domain payload value
  jarvis_mem0_points_total               total points seen during the scroll
  jarvis_mem0_collection_points          authoritative /points/count total
  jarvis_mem0_scrape_success             1 if the qdrant scrape fully succeeded
  jarvis_mem0_scrape_duration_seconds    wall time of the last collection
  jarvis_mem0_server_up                  1 if mem0 /health returned ok:true
  jarvis_mem0_exporter_up                always 1 (liveness of the exporter)

Config via env (sensible in-cluster defaults):
  QDRANT_URL       default http://qdrant.ai.svc.cluster.local:6333
  QDRANT_COLLECTION default jarvis_mem0
  MEM0_URL         default http://jarvis-mem0.ai.svc.cluster.local:8800
  SCOPE_KEY        payload key used as `scope` label   (default user_id)
  PROJECT_KEY      payload key used as `project` label (default domain)
  EMPTY_LABEL      value used when a payload key is absent (default "none")
  LISTEN_PORT      default 9114
  SCROLL_PAGE      page size for the qdrant scroll      (default 256)
  HTTP_TIMEOUT     seconds, default 15
  MAX_SERIES       cap on distinct (scope,project) pairs to emit (default 200)
"""
from __future__ import annotations

import json
import os
import time
import urllib.error
import urllib.request
from collections import Counter
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

QDRANT_URL = os.environ.get(
    "QDRANT_URL", "http://qdrant.ai.svc.cluster.local:6333"
).rstrip("/")
QDRANT_COLLECTION = os.environ.get("QDRANT_COLLECTION", "jarvis_mem0")
MEM0_URL = os.environ.get(
    "MEM0_URL", "http://jarvis-mem0.ai.svc.cluster.local:8800"
).rstrip("/")
SCOPE_KEY = os.environ.get("SCOPE_KEY", "user_id")
PROJECT_KEY = os.environ.get("PROJECT_KEY", "domain")
EMPTY_LABEL = os.environ.get("EMPTY_LABEL", "none")
LISTEN_PORT = int(os.environ.get("LISTEN_PORT", "9114"))
SCROLL_PAGE = int(os.environ.get("SCROLL_PAGE", "256"))
HTTP_TIMEOUT = float(os.environ.get("HTTP_TIMEOUT", "15"))
MAX_SERIES = int(os.environ.get("MAX_SERIES", "200"))


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------
def _post(url: str, body: dict) -> dict:
    req = urllib.request.Request(
        url,
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as r:
        return json.loads(r.read() or b"{}")


def _get(url: str) -> dict:
    req = urllib.request.Request(url, method="GET")
    with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as r:
        return json.loads(r.read() or b"{}")


def _esc(v: str) -> str:
    """Escape a Prometheus label value."""
    return str(v).replace("\\", "\\\\").replace('"', '\\"').replace("\n", " ")


def _norm(v) -> str:
    if v is None or v == "":
        return EMPTY_LABEL
    return str(v)


# ---------------------------------------------------------------------------
# collection
# ---------------------------------------------------------------------------
def collect() -> dict:
    """Return a dict of computed metric structures, plus success flag/duration."""
    started = time.monotonic()
    pair = Counter()      # (scope, project) -> count
    by_scope = Counter()  # scope -> count
    by_source = Counter()
    by_domain = Counter()
    total = 0
    success = 1
    collection_count = None

    # 1) cheap authoritative total
    try:
        c = _post(
            f"{QDRANT_URL}/collections/{QDRANT_COLLECTION}/points/count",
            {"exact": True},
        )
        collection_count = int(c.get("result", {}).get("count", 0))
    except Exception:
        collection_count = None  # surfaced as missing series, not a hard fail

    # 2) full scroll, tally by payload
    offset = None
    try:
        while True:
            body = {
                "limit": SCROLL_PAGE,
                "with_payload": True,
                "with_vector": False,
            }
            if offset is not None:
                body["offset"] = offset
            res = _post(
                f"{QDRANT_URL}/collections/{QDRANT_COLLECTION}/points/scroll",
                body,
            )
            result = res.get("result", {})
            for p in result.get("points", []):
                pl = p.get("payload", {}) or {}
                total += 1
                scope = _norm(pl.get(SCOPE_KEY))
                project = _norm(pl.get(PROJECT_KEY))
                pair[(scope, project)] += 1
                by_scope[scope] += 1
                if "source" in pl:
                    by_source[_norm(pl.get("source"))] += 1
                if "domain" in pl:
                    by_domain[_norm(pl.get("domain"))] += 1
            offset = result.get("next_page_offset")
            if offset is None:
                break
    except Exception:
        success = 0

    duration = time.monotonic() - started

    # 3) mem0 server health
    server_up = 0
    try:
        h = _get(f"{MEM0_URL}/health")
        server_up = 1 if h.get("ok") else 0
    except Exception:
        server_up = 0

    return {
        "pair": pair,
        "by_scope": by_scope,
        "by_source": by_source,
        "by_domain": by_domain,
        "total": total,
        "collection_count": collection_count,
        "success": success,
        "duration": duration,
        "server_up": server_up,
    }


def render(d: dict) -> str:
    out: list[str] = []

    def help_type(name: str, typ: str, helptext: str):
        out.append(f"# HELP {name} {helptext}")
        out.append(f"# TYPE {name} {typ}")

    help_type(
        "jarvis_mem0_points", "gauge",
        "JARVIS mem0 facts per (scope, project) in qdrant collection "
        f"{QDRANT_COLLECTION}.",
    )
    # Cap the (scope,project) cardinality, biggest series first.
    pairs = d["pair"].most_common(MAX_SERIES)
    for (scope, project), n in pairs:
        out.append(
            f'jarvis_mem0_points{{scope="{_esc(scope)}",'
            f'project="{_esc(project)}"}} {n}'
        )

    help_type(
        "jarvis_mem0_points_by_scope", "gauge",
        "JARVIS mem0 facts per scope (payload key "
        f"'{SCOPE_KEY}').",
    )
    for scope, n in d["by_scope"].most_common():
        out.append(f'jarvis_mem0_points_by_scope{{scope="{_esc(scope)}"}} {n}')

    help_type(
        "jarvis_mem0_points_by_source", "gauge",
        "JARVIS mem0 facts per 'source' payload value.",
    )
    for source, n in d["by_source"].most_common():
        out.append(
            f'jarvis_mem0_points_by_source{{source="{_esc(source)}"}} {n}'
        )

    help_type(
        "jarvis_mem0_points_by_domain", "gauge",
        "JARVIS mem0 facts per 'domain' payload value.",
    )
    for domain, n in d["by_domain"].most_common():
        out.append(
            f'jarvis_mem0_points_by_domain{{domain="{_esc(domain)}"}} {n}'
        )

    help_type(
        "jarvis_mem0_points_total", "gauge",
        "Total JARVIS mem0 facts tallied during the last scroll.",
    )
    out.append(f"jarvis_mem0_points_total {d['total']}")

    help_type(
        "jarvis_mem0_collection_points", "gauge",
        "Authoritative qdrant /points/count for the collection.",
    )
    if d["collection_count"] is not None:
        out.append(f"jarvis_mem0_collection_points {d['collection_count']}")

    help_type(
        "jarvis_mem0_scrape_success", "gauge",
        "1 if the last qdrant scroll completed fully, else 0.",
    )
    out.append(f"jarvis_mem0_scrape_success {d['success']}")

    help_type(
        "jarvis_mem0_scrape_duration_seconds", "gauge",
        "Wall time of the last qdrant collection.",
    )
    out.append(f"jarvis_mem0_scrape_duration_seconds {d['duration']:.4f}")

    help_type(
        "jarvis_mem0_server_up", "gauge",
        "1 if mem0 REST /health reported ok:true, else 0.",
    )
    out.append(f"jarvis_mem0_server_up {d['server_up']}")

    help_type(
        "jarvis_mem0_exporter_up", "gauge",
        "Liveness of the mem0 exporter itself (always 1 when scraped).",
    )
    out.append("jarvis_mem0_exporter_up 1")

    return "\n".join(out) + "\n"


# ---------------------------------------------------------------------------
# http server
# ---------------------------------------------------------------------------
class Handler(BaseHTTPRequestHandler):
    def do_GET(self):  # noqa: N802
        if self.path.rstrip("/") in ("/metrics", ""):
            try:
                payload = render(collect()).encode()
                code = 200
            except Exception as e:  # noqa: BLE001
                payload = (
                    "jarvis_mem0_exporter_up 1\n"
                    "jarvis_mem0_scrape_success 0\n"
                    f"# exporter error: {e}\n"
                ).encode()
                code = 200  # still 200 so the series above are scraped
            self.send_response(code)
            self.send_header(
                "Content-Type", "text/plain; version=0.0.4; charset=utf-8"
            )
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
        elif self.path.rstrip("/") in ("/health", "/healthz"):
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"ok\n")
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, *args):  # silence per-request logging
        return


def main():
    print(
        f"mem0_exporter listening on :{LISTEN_PORT}/metrics  "
        f"qdrant={QDRANT_URL} collection={QDRANT_COLLECTION} "
        f"scope_key={SCOPE_KEY} project_key={PROJECT_KEY}",
        flush=True,
    )
    ThreadingHTTPServer(("0.0.0.0", LISTEN_PORT), Handler).serve_forever()


if __name__ == "__main__":
    main()
