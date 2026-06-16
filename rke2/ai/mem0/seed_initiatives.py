#!/usr/bin/env python3
"""seed_initiatives — seed JARVIS unified memory with PROJECT/INITIATIVE context.

This is the project/initiative seeding step from the JARVIS charter ("Map the
homelab into it — ... seed project/initiative context"). It writes one concise
fact per active initiative so that project-level questions resolve from the
unified memory, e.g. "what's the deal with the mem0 rollout?".

  DURABLE, not synced. Unlike the worldsync_* mappers (which sync live cluster
  state and are meant to run on a schedule), initiative context is durable
  human-curated knowledge — it changes when *we* change it, not when the cluster
  ticks. So this script is fine to run once. It is still SAFE TO RE-RUN: each
  fact is a single self-contained statement prefixed with a stable
  "[initiative:<key>]" tag, and mem0's extraction layer deduplicates equivalent
  facts, so a re-run refreshes rather than piles up duplicates. Edit a fact here
  and re-run to supersede the stale phrasing.

WHAT IS SEEDED: each entry below is name + goal + current status + key
components for one initiative, mined from rke2/ai/ (READMEs, docs/spec,
namespace-note.md, deployment comments) and the homelab repo (khemeia,
audiomuse, spotify-exporter) plus recent git history. Curated facts only — this
is the "what it's FOR / where it stands" knowledge that kube/Prometheus cannot
infer.

TARGET: mem0 REST service. POST /add {text, user_id}. Default user_id "jarvis"
(the homelab/world scope — the same scope the worldsync_* mappers write to, and
distinct from the per-speaker "owner" scope used by the brain's
jarvis_mem0_mcp.py shim).

  NOTE: requires the mem0 server's embedding fix (custom 'fastembed' provider +
  the EmbedderConfig validator patch + xAI top_p>0) in build/server.py. Until a
  rebuilt image carries that fix, /add returns HTTP 400 "Unsupported embedding
  provider: fastembed". This script reports each failed write clearly and exits
  non-zero so the breakage is obvious; it starts persisting the moment the
  server embeds. Use --dry-run to see exactly what it WOULD write.

Local/uncommitted like the rest of JARVIS. Python 3, stdlib only.
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request

MEM0_URL = os.environ.get(
    "MEM0_URL", "http://jarvis-mem0.ai.svc.cluster.local:8800"
).rstrip("/")
USER_ID = os.environ.get("SEED_USER_ID", "jarvis")
HTTP_TIMEOUT = float(os.environ.get("MEM0_HTTP_TIMEOUT", "60"))

# Stable tag prefix marking every fact this seeder writes, so a fact is greppable
# in the store and a re-run produces the *same* phrasing (idempotent upsert).
TAG = "initiative"


# One curated record per initiative: a stable key (used in the [initiative:<key>]
# tag) and a single, self-contained, spoken-friendly fact stating the
# initiative's NAME, GOAL, CURRENT STATUS, and KEY COMPONENTS. Keep each to a
# tight paragraph; mem0's extraction will distill atomic memories from it.
INITIATIVES: list[tuple[str, str]] = [
    (
        "jarvis",
        "JARVIS is Hampton's voice-first AI agent over his homelab and personal "
        "projects. Its goal is a single unified memory of everything the homelab "
        "knows (cluster, deployments, databases, metrics, projects) that IS "
        "JARVIS's understanding, plus tools to act on it; it proposes then acts "
        "on confirmation, stays grounded (never confabulates), and is proactive "
        "not chatty. The brain runs Anthropic Claude Sonnet (never Haiku, which "
        "confabulates tool calls). Key components: the jarvis-edge brain "
        "(claude CLI per turn), whisper STT, Chatterbox TTS, openWakeWord, Yeti "
        "mic and Sonos output, and the mem0 unified memory.",
    ),
    (
        "mem0-rollout",
        "The mem0 rollout is the JARVIS unified-memory initiative: a self-hosted "
        "mem0 REST service (Deployment jarvis-mem0 in the ai namespace, Service "
        "jarvis-mem0.ai.svc.cluster.local:8800) backed by a Qdrant vector store, "
        "with CPU-only fastembed embeddings (BAAI/bge-small-en-v1.5, 384-dim) and "
        "fact extraction by xAI Grok (grok-3-mini) reusing the existing "
        "XAI_API_KEY — no GPU, no new API key. The goal is the unified memory "
        "spine the whole charter depends on. Current status: deployed and now "
        "working after a fix in build/server.py (register a custom 'fastembed' "
        "embedder, patch mem0's EmbedderConfig provider-allowlist validator, and "
        "set top_p>0 for xAI), but the running pod still serves a STALE image "
        "without that fix, so an ARC image rebuild and rollout is required to "
        "make the fix durable. The MCP shim jarvis_mem0_mcp.py that exposes "
        "memory_add/memory_search to the brain exists but is not yet wired into "
        "edge.py — wiring it is the next step.",
    ),
    (
        "jarvis-voice-stack",
        "The JARVIS voice stack is the cluster-side deployment that gives JARVIS "
        "a voice in the room. It runs as the jarvis-stack pod on the nixos-gpu "
        "node with hostNetwork, containing jarvis-edge (brain + wake word + "
        "audio), whisper-stt (faster-whisper large-v3-turbo on GPU) and "
        "chatterbox-tts (zero-shot voice clone on GPU). Per turn: openWakeWord "
        "'hey jarvis' triggers capture, Whisper transcribes, the claude brain "
        "answers with MCP tools, Chatterbox synthesizes, and a Sonos Play:1 "
        "plays it back. Goal: low-latency present-in-the-room voice interaction. "
        "Status: deployed and working; known follow-ups are streaming TTS and a "
        "warm streaming brain to cut per-turn latency.",
    ),
    (
        "jarvis-identity",
        "JARVIS identity and trust is the initiative that gates who can authorize "
        "what, by speaker. Trust is identity-scoped: speaker-ID determines the "
        "memory scope ('owner' vs per-user) and what actions are allowed, and "
        "auth never routes through the brain (the locked/TRUSTED brain has no MCP "
        "config at all, so non-owners cannot reach owner tools or memory). Goal: "
        "owner-safe partitioning of memory and capability. Status: a "
        "deterministic cross-domain identity gate is implemented in jarvis-edge "
        "(jarvis_identity.py, jarvis_voice_id.py); the mem0 partition key "
        "(JARVIS_MEM_SCOPE / user_id) enforces per-speaker memory isolation.",
    ),
    (
        "khemeia",
        "Khemeia is Hampton's Kubernetes-orchestrated computational-chemistry "
        "platform for Structure-Based Drug Discovery (SBDD), running on the "
        "bare-metal RKE2 cluster (namespace chem). Goal: scientists work "
        "alongside an end-to-end docking-to-design workflow. Working prototype "
        "today: a Go API with a YAML-driven plugin system, a SvelteKit frontend, "
        "a Molstar 3D viewer (has known bugs), AutoDock Vina with parallel "
        "fan-out docking, a ProLIF interaction-fingerprint sidecar, a "
        "result-writer service, Authentik OIDC, and Flux GitOps; a "
        "khemeia-postgres control-plane DB holds DockJob/compute CRD state and "
        "results metadata, and ChEMBL provides reference bioactivity data. "
        "Status: docking prototype works; nine work packages (target intake, "
        "library prep, docking refinement, ADMET, generative SAR, selectivity "
        "FEP, UI, reporting, infrastructure) are specced but largely not started. "
        "JARVIS is meant to launch and track khemeia DockJobs on the GPU.",
    ),
    (
        "audiomuse",
        "AudioMuse-AI is the sonic-analysis initiative over Hampton's local FLAC "
        "archive (the ~1.7 TB / ~37,891-track Plex music library). Goal: organize "
        "the archive on its own terms — CLAP audio embeddings + Essentia DSP + "
        "mood/sonic clustering for collection-local similarity and playlist "
        "generation — filling the gap left by Spotify deprecating its "
        "audio-features endpoint. It is a standalone archive system (namespace "
        "audiomuse) with its OWN Postgres, Redis queue, a Flask web UI, and a "
        "CPU-only RQ worker on the forum nodes, reading the library via Navidrome; "
        "it is deliberately separate from the Spotify analytics DB. Status: "
        "scaffold only — manifests are written but nothing has been applied; "
        "needs Bitwarden secret items created before first kubectl apply.",
    ),
    (
        "spotify-analytics",
        "Spotify analytics (the 'spotifyranked' work) turns Hampton's Spotify "
        "listening into ranked metrics and dashboards. A standalone "
        "spotify-exporter (namespace monitor) polls the Spotify API and serves "
        "Prometheus metrics — top-artist and top-track ranks, artist popularity, "
        "rank-weighted genre scores, cumulative play counts, and live "
        "now-playing — over short/medium/long time ranges, with a provisioned "
        "Grafana 'Spotify' dashboard and a Spotify analytics Postgres warehouse. "
        "Goal: quantify and rank listening habits over time. Status: deployed and "
        "running, independent of JARVIS; it shares a single Spotify app / PKCE "
        "refresh token with JARVIS (Spotify rotates the refresh token, so they "
        "must share one authorization).",
    ),
    (
        "homelab-bot",
        "homelab-bot is a Discord bot for homelab communities (a test server, "
        "eventually r/homelab), built on Red-DiscordBot with a custom "
        "'labbotbrain' cog. Goal: answer homelab questions in Discord via an LLM "
        "with homelab-tuned tools. It calls xAI Grok (with an Ollama "
        "qwen3:8b fallback at ollama.ai.svc) using native function-calling tools "
        "web_search (DuckDuckGo) and stackexchange_search; access is gated by a "
        "Discord role allow-list (fail-closed to owner-only). It runs in the ai "
        "namespace and its homelab-bot-credentials Secret also supplies the "
        "shared XAI_API_KEY that the mem0 rollout reuses for fact extraction.",
    ),
    (
        "worldsync",
        "World-sync is the set of re-runnable mappers that keep the JARVIS "
        "unified memory fresh by syncing live homelab state into mem0 (user_id "
        "'jarvis'), as opposed to durable hand-curated facts. Goal: the memory is "
        "both unified and current, never frozen state masquerading as memory. "
        "Each mapper writes one concise fact per item with a stable tag and does "
        "an idempotent upsert, designed to run on a schedule (CronJob) or by "
        "hand. Status: worldsync_databases.py (maps the cluster's databases — "
        "engine, namespace/service, purpose, backing storage) exists and is "
        "complete; it and future mappers (cluster/nodes, deployments, metrics) "
        "depend on the mem0 embedding fix to actually persist.",
    ),
]


def to_fact(key: str, body: str) -> str:
    """Prefix the curated body with the stable [initiative:<key>] tag."""
    return f"[{TAG}:{key}] {body}"


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
    ap = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    ap.add_argument("--dry-run", action="store_true",
                    help="print the facts that would be written; do not POST")
    ap.add_argument("--user-id", default=USER_ID,
                    help=f"mem0 partition key (default: {USER_ID})")
    ap.add_argument("--only", metavar="KEY",
                    help="seed only the initiative with this key")
    args = ap.parse_args()
    USER_ID = args.user_id

    records = INITIATIVES
    if args.only:
        records = [(k, b) for (k, b) in INITIATIVES if k == args.only]
        if not records:
            keys = ", ".join(k for k, _ in INITIATIVES)
            print(f"no initiative with key '{args.only}'. known keys: {keys}",
                  file=sys.stderr)
            return 2

    print(f"Seeding {len(records)} initiative fact(s) into mem0 "
          f"(user_id={USER_ID}).\n")
    facts = [(k, to_fact(k, b)) for (k, b) in records]

    if args.dry_run:
        for k, f in facts:
            print(f"  [{k}]\n  {f}\n")
        print(f"[dry-run] would write {len(facts)} fact(s) to "
              f"{MEM0_URL}/add (user_id={USER_ID}).")
        return 0

    ok = 0
    failures = 0
    for k, f in facts:
        success, detail = mem0_add(f)
        if success:
            ok += 1
            # Show what mem0 extracted so a human can sanity-check the result.
            try:
                extracted = json.loads(detail).get("result", [])
                summary = "; ".join(
                    m.get("memory", "") for m in extracted if m.get("memory")
                ) or "(no new atomic facts — likely already stored)"
            except Exception:  # noqa: BLE001
                summary = detail[:200]
            print(f"  [ok]   {k}: {summary}")
        else:
            failures += 1
            print(f"  [FAIL] {k}: {detail}")

    print(f"\nwrote {ok}/{len(facts)} initiative fact(s) to mem0 "
          f"(user_id={USER_ID}); {failures} failed.")
    if failures:
        print("note: if failures are 'Unsupported embedding provider: fastembed', "
              "the mem0 server image is stale — rebuild it with the fix in "
              "build/server.py and re-run. This seeder is idempotent and safe to "
              "re-run.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
