"""jarvis_edge — thin satellite for JARVIS.

Architecture:

  [ EDGE BOX (Surface Pro, Pi, etc.) ]   [ CLUSTER + Sonos ]
    wake-word listen (openWakeWord)
    audio capture (VAD-endpointed)  ──→  whisper-stt :8766
                                         ←── transcript
    brain(transcript)                     (stub for now → swap for real brain)
    text                            ──→  chatterbox :8765 /synthesize
                                         ←── WAV bytes
    serve WAV via embedded HTTP     ──→  Sonos play_uri(http://edge:8088/turn.wav)
    wait for playback to finish

Config via env vars (or edit defaults below):

    STT_URL       http://10.44.0.20:8766
    TTS_URL       http://10.44.0.8:8765
    SONOS_IP      (REQUIRED — Play:5 LAN IP)
    EDGE_HTTP_PORT 8088  (port we serve WAV on; reachable from Sonos)
    EDGE_HOST_IP  (optional — IP we tell Sonos to fetch from; defaults to
                   auto-detect of the route used to reach SONOS_IP)
    BRAIN_MODE    echo  (stub) | api (anthropic) — for now stay on `echo`
"""
from __future__ import annotations

import hashlib
import hmac
import io
import json
import re
import uuid
import logging
import os
import queue
import socket
import sys
import tempfile
import threading
import time
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer, ThreadingHTTPServer
from math import gcd

import numpy as np
import sounddevice as sd
# Wake word removed for AMBIENT MODE — VAD-gated continuous capture instead.
# (openwakeword and tflite-runtime stay in the base image for future use.)
from scipy.signal import resample_poly

# ── Observability: OpenTelemetry tracing + Prometheus metrics ────────────────
# Tracing goes to Grafana Tempo via OTLP/gRPC. Metrics are scraped by
# Prometheus via the PodMonitor that another agent scaffolded (port 9090).
# Both paths are FAIL-OPEN: if Tempo is unreachable or 9090 is busy, we log
# once and JARVIS keeps running normally. Voice-assistant uptime is more
# important than telemetry.
import logging as _otel_logging

_OTEL_ENDPOINT = os.environ.get(
    "OTEL_EXPORTER_OTLP_ENDPOINT",
    "http://tempo.monitor.svc.cluster.local:4317",
)
_OTEL_SERVICE_NAME = "jarvis-edge"
_PROM_PORT = int(os.environ.get("PROMETHEUS_PORT", "9090"))

# Default tracer is a no-op so spans always work, even if init fails.
try:
    from opentelemetry import trace as _otel_trace
    from opentelemetry.sdk.resources import Resource as _OtelResource
    from opentelemetry.sdk.trace import TracerProvider as _OtelTracerProvider
    from opentelemetry.sdk.trace.export import (
        BatchSpanProcessor as _OtelBatchSpanProcessor,
    )
    from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import (
        OTLPSpanExporter as _OtelOTLPSpanExporter,
    )
    # Silence the OTLP exporter's per-batch connection errors after the
    # first warning. Without this, every BatchSpanProcessor flush against a
    # down Tempo prints a multi-line gRPC stack trace and drowns the logs.
    _otel_logging.getLogger("opentelemetry.exporter.otlp.proto.grpc.exporter").setLevel(
        _otel_logging.CRITICAL
    )
    _otel_logging.getLogger("opentelemetry.sdk.trace.export").setLevel(
        _otel_logging.CRITICAL
    )
    _otel_resource = _OtelResource.create({"service.name": _OTEL_SERVICE_NAME})
    _otel_provider = _OtelTracerProvider(resource=_otel_resource)
    _otel_exporter = _OtelOTLPSpanExporter(endpoint=_OTEL_ENDPOINT, insecure=True)
    _otel_provider.add_span_processor(_OtelBatchSpanProcessor(_otel_exporter))
    _otel_trace.set_tracer_provider(_otel_provider)
    tracer = _otel_trace.get_tracer(_OTEL_SERVICE_NAME)
    print(f"otel: tracing to {_OTEL_ENDPOINT} (service={_OTEL_SERVICE_NAME})")
except Exception as _otel_exc:  # noqa: BLE001
    print(f"otel: tracing unavailable ({_otel_exc!r}) — spans will be no-ops")

    class _NoopSpan:
        def __enter__(self):
            return self

        def __exit__(self, *_a, **_kw):
            return False

        def set_attribute(self, *_a, **_kw):
            pass

        def add_event(self, *_a, **_kw):
            pass

        def set_status(self, *_a, **_kw):
            pass

        def record_exception(self, *_a, **_kw):
            pass

    class _NoopTracer:
        def start_as_current_span(self, *_a, **_kw):
            return _NoopSpan()

    tracer = _NoopTracer()

# Prometheus metrics. Histograms in seconds (Prom convention). Buckets tuned
# for voice-assistant timings (sub-100ms STT chunks up to multi-second turns).
#
# IDEMPOTENT REGISTRATION — edge.py runs as __main__ (CMD ["python","-u","edge.py"]),
# so it lives in sys.modules as "__main__", NOT "edge". At runtime several
# sub-modules (jarvis_ig_consumer, jarvis_discord, jarvis_song_id,
# jarvis_reel_context, …) do `import edge as _edge`. Because "edge" is absent
# from sys.modules, Python imports edge.py a SECOND time under the name "edge",
# re-executing this whole module — including this metrics block. Re-creating a
# collector whose name already lives in the default REGISTRY raises
# "ValueError: Duplicated timeseries in CollectorRegistry: jarvis_stt_..." which
# the broad `except` below swallowed by replacing EVERY metric with a NoopMetric
# → ALL jarvis_* metrics silently disabled. Fix: look up the existing collector
# by name and reuse it instead of constructing a duplicate, so a second import
# is a no-op and metrics keep exporting once.
try:
    from prometheus_client import (
        Counter as _PromCounter,
        Histogram as _PromHistogram,
        start_http_server as _prom_start_http_server,
    )
    from prometheus_client import REGISTRY as _PROM_REGISTRY

    def _metric(_ctor, _name, *args, **kwargs):
        """Create a collector, or reuse the one already in the default registry.

        prometheus_client maps each timeseries name to its registered collector
        in REGISTRY._names_to_collectors. On a second import of edge.py we find
        the existing collector there and return it rather than raising on a
        duplicate. Idempotent across any number of re-imports.
        """
        existing = getattr(_PROM_REGISTRY, "_names_to_collectors", {}).get(_name)
        if existing is not None:
            return existing
        return _ctor(_name, *args, **kwargs)

    _DUR_BUCKETS = (0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 3.0, 5.0, 8.0, 13.0, 21.0)
    METRIC_STT_DURATION = _metric(
        _PromHistogram,
        "jarvis_stt_duration_seconds",
        "Whisper STT round-trip duration",
        buckets=_DUR_BUCKETS,
    )
    METRIC_BRAIN_DURATION = _metric(
        _PromHistogram,
        "jarvis_brain_duration_seconds",
        "Brain (claude/echo/shortcut) response duration",
        buckets=_DUR_BUCKETS,
    )
    METRIC_TTS_SEGMENT_DURATION = _metric(
        _PromHistogram,
        "jarvis_tts_segment_duration_seconds",
        "Chatterbox TTS synthesis per segment",
        buckets=_DUR_BUCKETS,
    )
    METRIC_SONOS_FIRST_AUDIO = _metric(
        _PromHistogram,
        "jarvis_sonos_first_audio_seconds",
        "Time from stream start to Sonos first audio playing",
        buckets=_DUR_BUCKETS,
    )
    METRIC_TURN_TOTAL_DURATION = _metric(
        _PromHistogram,
        "jarvis_turn_total_duration_seconds",
        "Full per-turn wall-clock: speech_start → stream_done",
        buckets=_DUR_BUCKETS,
    )
    METRIC_TURNS_TOTAL = _metric(
        _PromCounter,
        "jarvis_turns_total",
        "Per-turn outcomes",
        ["addressed", "ambient_drop", "echo_drop"],
    )
    METRIC_ECHO_DROPS = _metric(
        _PromCounter,
        "jarvis_echo_drops_total",
        "Utterances dropped as JARVIS-self-echo",
    )
    METRIC_UNKNOWN_SPEAKER_DROPS = _metric(
        _PromCounter,
        "jarvis_unknown_speaker_drops_total",
        "Utterances dropped because speaker did not match an enrolled voice",
    )
    METRIC_BRAIN_ERRORS = _metric(
        _PromCounter,
        "jarvis_brain_errors_total",
        "Brain failures by reason",
        ["reason"],
    )
    METRIC_IG_EVENTS = _metric(
        _PromCounter,
        "jarvis_ig_webhook_events_total",
        "IG webhook events",
        ["type", "status"],
    )
    METRIC_IG_SIG_FAILURES = _metric(
        _PromCounter,
        "jarvis_ig_webhook_signature_failures_total",
        "IG webhook HMAC signature failures",
    )
    # Only the FIRST execution (the __main__ process) should bind the HTTP
    # server. A re-import under the name "edge" must never try to bind :9090
    # again — that would either collide or, worse, be swallowed and look like a
    # failure. Guard on __name__ so exactly one server starts per process.
    if __name__ == "__main__":
        try:
            _prom_start_http_server(_PROM_PORT)
            print(f"prometheus: /metrics on 0.0.0.0:{_PROM_PORT}")
        except OSError as _prom_exc:
            print(f"prometheus: port {_PROM_PORT} busy ({_prom_exc}) — metrics endpoint disabled")
except Exception as _prom_exc:  # noqa: BLE001
    print(f"prometheus: client unavailable ({_prom_exc!r}) — metrics disabled")

    class _NoopMetric:
        def labels(self, *_a, **_kw):
            return self

        def observe(self, *_a, **_kw):
            pass

        def inc(self, *_a, **_kw):
            pass

    METRIC_STT_DURATION = _NoopMetric()
    METRIC_BRAIN_DURATION = _NoopMetric()
    METRIC_TTS_SEGMENT_DURATION = _NoopMetric()
    METRIC_SONOS_FIRST_AUDIO = _NoopMetric()
    METRIC_TURN_TOTAL_DURATION = _NoopMetric()
    METRIC_TURNS_TOTAL = _NoopMetric()
    METRIC_ECHO_DROPS = _NoopMetric()
    METRIC_UNKNOWN_SPEAKER_DROPS = _NoopMetric()
    METRIC_BRAIN_ERRORS = _NoopMetric()
    METRIC_IG_EVENTS = _NoopMetric()
    METRIC_IG_SIG_FAILURES = _NoopMetric()

# ── Config ────────────────────────────────────────────────────────────────────
STT_URL = os.environ.get("STT_URL", "http://10.44.0.20:8766")
TTS_URL = os.environ.get("TTS_URL", "http://10.44.0.8:8765")
SONOS_IP = os.environ.get("SONOS_IP", "")
EDGE_HTTP_PORT = int(os.environ.get("EDGE_HTTP_PORT", "8088"))
# Port that Sonos hits in the URL — may differ from EDGE_HTTP_PORT when
# the pod is exposed via a NodePort Service (containerPort 8088 maps to
# nodePort 30088, for example). Defaults to EDGE_HTTP_PORT for direct
# host-network deployments.
EDGE_ADVERTISED_PORT = int(os.environ.get("EDGE_ADVERTISED_PORT",
                                          str(EDGE_HTTP_PORT)))
EDGE_HOST_IP = os.environ.get("EDGE_HOST_IP", "")
BRAIN_MODE = os.environ.get("BRAIN_MODE", "claude")

WAKE_THRESHOLD = 0.65        # was 0.5 — bumped to cut kitchen false-fires
SILENCE_SECS = 0.6
MAX_UTTERANCE_SECS = 8.0
MIN_UTTERANCE_SECS = 0.4
ADDRESSEE_WINDOW_S = 6.0     # post-reply follow-up window: speech in this
                              # window doesn't need "jarvis"
ECHO_SUPPRESS_S = 3.5        # drop any utterance whose speech_start fell
                              # during JARVIS's own Sonos playback or up
                              # to N seconds after. Prevents JARVIS from
                              # transcribing its own voice and replying
                              # to itself (the "Nineteen, sir. → 19, sir.
                              # → Nineteen what, sir?" feedback loop).
OWW_RATE = 16000
OWW_CHUNK = 1280
SONOS_VOLUME = int(os.environ.get("SONOS_VOLUME", "60"))  # 0-100

# Two-tier time-of-day Sonos volume. Night hours (default 22:00–07:00
# local) get the quieter level so JARVIS doesn't blast in the bedroom
# overnight. Outside that band uses the day level. All knobs overridable
# via env; persona.json's sonos_volume still wins when present, so the
# user can pin a level via the persona MCP.
SONOS_VOLUME_NIGHT = int(os.environ.get("SONOS_VOLUME_NIGHT", "30"))
SONOS_VOLUME_DAY = int(os.environ.get("SONOS_VOLUME_DAY", "40"))
SONOS_NIGHT_START_HOUR = int(os.environ.get("SONOS_NIGHT_START_HOUR", "22"))
SONOS_NIGHT_END_HOUR = int(os.environ.get("SONOS_NIGHT_END_HOUR", "7"))


def _is_night_hours(now_hour: int) -> bool:
    """True if `now_hour` (0–23) falls inside the night band. The band
    wraps midnight when start > end (e.g. 22→7)."""
    s, e = SONOS_NIGHT_START_HOUR, SONOS_NIGHT_END_HOUR
    if s == e:
        return False
    if s < e:
        return s <= now_hour < e
    return now_hour >= s or now_hour < e


def _scheduled_sonos_volume() -> int:
    """Pick the volume from the time-of-day band, in LOCAL time (TZ env)."""
    return SONOS_VOLUME_NIGHT if _is_night_hours(time.localtime().tm_hour) else SONOS_VOLUME_DAY

# ── Instagram webhook config ─────────────────────────────────────────────────
# Meta posts Messenger / IG event payloads to /ig/webhook on the same
# embedded HTTP server that serves Sonos audio. Verification is a GET
# handshake; events are POSTs signed with HMAC-SHA256 over the raw body
# using the app's secret. Parsed events go on _ig_event_queue for a future
# consumer thread (replies + DMs). FAIL-OPEN: if IG env is missing we
# reject everything but the daemon keeps running for voice.
_ig_event_queue: queue.Queue = queue.Queue(maxsize=1000)
IG_ENABLED = os.environ.get("IG_ENABLED", "") == "1"
IG_VERIFY_TOKEN = os.environ.get("IG_VERIFY_TOKEN", "")
IG_APP_SECRET = os.environ.get("IG_APP_SECRET", "")
IG_PAGE_TOKEN = os.environ.get("IG_PAGE_TOKEN", "")
if IG_ENABLED:
    if not IG_VERIFY_TOKEN or not IG_APP_SECRET:
        print("ig: IG_ENABLED=1 but verify_token or app_secret missing — webhook will reject everything")
    else:
        print(f"ig: webhook ready; page_token len={len(IG_PAGE_TOKEN)}")
# Throttle "signature mismatch" logs to one line per process — Meta spam
# probes can otherwise fill the log with identical entries.
_ig_sig_logged_once = False


# ── Speaker-ID ───────────────────────────────────────────────────────────────
# Lazy-loaded; only constructs the Resemblyzer encoder + reads /state/voices
# on the first capture, then caches enrolled status for 60s so the hot path
# stays cheap. If /state/voices doesn't exist or no owner is enrolled, the
# gate is a pass-through (back-compat for fresh deployments).
_vid_cache: dict = {"has_owner": False, "ts": 0.0}
_VID_CACHE_TTL = 60.0


def _vid_has_owner() -> bool:
    """Cached check: is anyone enrolled with role=owner?"""
    now = time.time()
    if now - _vid_cache["ts"] > _VID_CACHE_TTL:
        try:
            import jarvis_voice_id as _vid
            _vid_cache["has_owner"] = _vid.has_owner()
        except Exception as exc:  # noqa: BLE001
            # Module import or filesystem error — fail open (no gate).
            print(f"  [vid] has_owner check failed: {exc!r}")
            _vid_cache["has_owner"] = False
        _vid_cache["ts"] = now
    return _vid_cache["has_owner"]


def _identify_speaker_from_audio(audio_16k: np.ndarray):
    """Resolve the captured 16k mono float32 utterance to a ``Principal`` via
    the identity layer.

    Returns ``None`` when NO owner is enrolled — open mode, back-compat
    pass-through (the gate routes None straight to the full brain, exactly as
    before speaker-id existed).

    When an owner IS enrolled, always returns a Principal (OWNER / TRUSTED /
    UNKNOWN). On any embedding/resolution error it FAILS CLOSED to a synthetic
    TRUSTED principal — general help only, never owner data — rather than
    UNKNOWN-dropping (which would look like open mode) or granting OWNER."""
    if not _vid_has_owner():
        return None  # open mode — no gate (back-compat for fresh deployments)
    import jarvis_identity as _ji
    try:
        emb = _ji.embed_from_audio(audio_16k, sample_rate=16000)
        return _ji.resolve_voice(emb)
    except Exception as exc:  # noqa: BLE001
        print(f"  [vid] resolve failed — failing CLOSED to trusted-locked: {exc!r}")
        return _ji.Principal(role=_ji.Role.TRUSTED, user_id="voice:unknown",
                             source="voice", confidence=0.0,
                             raw={"error": str(exc)})


# ── Mic resolve ──────────────────────────────────────────────────────────────
def _pick_mic() -> tuple[int, int, int]:
    """Pick the best input device. MIC_NAME env (case-insensitive substring)
    wins if set; else prefer Yeti / Blue / Microphones / Webcam / ReSpeaker
    / USB; fallback to system default."""
    wanted = (os.environ.get("MIC_NAME") or "").lower().strip()
    candidates = ("yeti", "blue", "microphones", "respeaker",
                  "webcam", "c922", "usb")
    mic_idx = None
    for i, d in enumerate(sd.query_devices()):
        if d.get("max_input_channels", 0) > 0 and d["max_input_channels"] <= 4:
            name = d["name"].lower()
            if wanted and wanted in name:
                mic_idx = i
                break
            if not wanted and any(s in name for s in candidates):
                mic_idx = i
                break
    if mic_idx is None:
        info = sd.query_devices(kind="input")
        mic_idx = info["index"] if isinstance(info, dict) else None
    dev = sd.query_devices(mic_idx)
    native_rate = int(dev["default_samplerate"])
    native_chunk = int(OWW_CHUNK * native_rate / OWW_RATE)
    print(f"mic: {dev['name']}  native={native_rate}Hz")
    return mic_idx, native_rate, native_chunk


def _to_16k(arr: np.ndarray, sr: int) -> np.ndarray:
    if sr == 16000:
        return arr.astype(np.float32)
    g = gcd(sr, OWW_RATE)
    return resample_poly(arr, OWW_RATE // g, sr // g).astype(np.float32)


# ── Cluster STT ──────────────────────────────────────────────────────────────
def transcribe(audio_16k: np.ndarray) -> dict:
    t0 = time.time()
    audio_dur_s = float(len(audio_16k) / 16000.0)
    body = audio_16k.astype(np.float32).tobytes()
    req = urllib.request.Request(
        f"{STT_URL}/v1/transcribe?sr=16000",
        data=body,
        headers={"Content-Type": "application/octet-stream"},
        method="POST",
    )
    with tracer.start_as_current_span("jarvis.stt") as span:
        span.set_attribute("audio_dur_s", audio_dur_s)
        try:
            with urllib.request.urlopen(req, timeout=10.0) as r:
                data = json.loads(r.read())
        except Exception as exc:
            span.set_attribute("error", True)
            span.record_exception(exc)
            METRIC_STT_DURATION.observe(time.time() - t0)
            return {"text": "", "error": str(exc), "wire_ms": int((time.time()-t0)*1000)}
        data["wire_ms"] = int((time.time() - t0) * 1000)
        span.set_attribute("transcribed_chars", len(data.get("text", "") or ""))
        span.set_attribute("hallucination", bool(data.get("hallucination", False)))
        span.set_attribute("lang", str(data.get("lang", "") or ""))
        METRIC_STT_DURATION.observe(time.time() - t0)
        return data


# ── Brain: real Claude (Haiku for fast turns) ────────────────────────────────
# Two modes:
#   echo    — debug stub, returns f"Sir, I heard: {text}"
#   claude  — invokes the `claude` CLI with the JARVIS persona system
#             prompt, optional MCP config for tool calls. Uses
#             ANTHROPIC_API_KEY (read from the jarvis-secrets Secret).
#
# Cold per-turn (no persistent session yet — that's a future port of
# openjarvis's ClaudeCodeBrain). Adds ~2-3s vs the Mac daemon's warm
# stream-json session, but keeps the implementation small.

# Persona — short JARVIS butler tone, terse, real time-of-day responses.
_PERSONA_SYSTEM = """You are JARVIS, the assistant from Iron Man. The owner is Hampton.

Speech rules — output WILL be read aloud through a Sonos speaker:
- TERSE. Default to ONE sentence. Max 12 words. A butler answering
  a quick question, not a paragraph. Long answers feel slow on TTS.
- Strip qualifiers ("currently", "right now", "today"). Just answer.
- Never use markdown, URLs, code blocks, lists, asterisks, or bullets.
- Numbers: speak naturally — "seventy-four" not "74", "ten thirty"
  not "10:30".
- No filler like "let me check" / "I'll look that up" — just answer.
- Decline to read URLs out loud.
- "Sir" — use SPARINGLY. About one in four replies, mostly when
  acknowledging or correcting. NEVER tag "sir" on every short factual
  answer ("Two." not "Two, sir."; "Nineteen." not "Nineteen, sir.").
  Save it for moments that benefit from formality.
- Examples of GOOD:
    Q: "what's 9 plus 10?"           A: "Nineteen."
    Q: "square root of four?"        A: "Two."
    Q: "what's the weather?"         A: "Seventy-four and overcast. High seventy-nine."
    Q: "what time is it?"            A: "Four eighteen in the morning."
    Q: "what's broken on the cluster?" A: "Nothing critical — all pods healthy."
    Q: "tell me about the briefing"  A: "Of course, sir." (then content)
    Q: (greeting)                    A: "Good morning, sir."

Tools:
- For daily summaries / "good morning" / "what's the briefing", call
  mcp__jarvis_personal__briefing (instant cached version) and SPEAK
  THE RESULT VERBATIM — it's already formatted for voice.
- For weather, use mcp__jarvis_personal__weather (configured to
  Murfreesboro, TN by default).
- For "what happened overnight" / "any news", use
  mcp__jarvis_personal__news_overnight.
- For "what am I listening to" / Spotify questions, use the
  mcp__jarvis_spotify__* tools.
- For "is the cluster healthy" / "how many devices" / "what's broken",
  use the mcp__jarvis_kube__* tools (kube_get_pods, kube_top_nodes,
  kube_events, etc). You have READ access only — no secrets, no writes.
- UNIFIED MEMORY (mem0) + OVERVIEW — your understanding of the homelab.
  For open-ended status questions ("what's going on", "how's the
  cluster", "what's the deal with X", "where do things stand"): call
  mcp__jarvis_overview__cluster_overview for the live digest, and/or
  mcp__jarvis_mem0__memory_search to recall what you already know about
  X, BEFORE answering — never guess. When the owner tells you a durable
  fact worth keeping (a preference, a decision, a person, an ongoing
  project, a correction), persist it with mcp__jarvis_mem0__memory_add.
  Do NOT store transient chatter (weather, the time, small talk).
- For longer / multi-step tasks use mcp__jarvis_delegate__delegate to
  spawn a sub-agent claude session.
- CLUSTER RUNS (long-running, fire-and-forget). When sir asks for a
  LONG-RUNNING task he wants done even if his laptop closes ("go figure
  out why X is broken and let me know", "investigate Y and report back",
  "spend a while digging into Z"), use mcp__jarvis_runner__launch_run.
  This creates a tracked cluster-side job that survives the laptop closing
  and ANNOUNCES its result on the speakers when done.
  PROPOSE FIRST, THEN ACT: do NOT call launch_run on sir's first request.
  First say one short line — "I'll launch a cluster run to <X> in
  <read|apply> mode, sir. Confirm?" — and only call launch_run AFTER sir
  explicitly confirms on a following turn. Default to mode="read"
  (read-only investigation). Only pass mode="apply" when sir explicitly
  said make / apply / fix / deploy / commit. Use
  mcp__jarvis_runner__list_runs / run_status to answer "what's running" /
  "did that finish" / "what did that run find".
- WebSearch / WebFetch for live external facts.
- For "volume up/down" / "louder/quieter" / "set the volume to X" /
  "mute" / "pause the music" / "what's playing": use the
  mcp__jarvis_sonos__* tools. Default target is the Bedroom Play:1
  (where JARVIS speaks). For "the kitchen speaker" pass room="Kitchen".
- TV control is NOT available (Apple TV migration pending). If asked
  about the TV, say "I can't control the TV from here yet, sir" — do
  NOT pretend, do NOT invent tool calls.
- macOS Calendar + Reminders are NOT reachable from this pod (no
  AppleScript). calendar_today / reminders_* will return 'unauthorized'.
  Don't apologise about it — just say "no calendar wired up here yet."

You are running on a cluster pod (nixos-gpu) with a Yeti USB mic and a
Sonos Play:1 in the bedroom. The current owner is Hampton."""

# MCP servers the brain can invoke. TV is DEPRECATED until Apple TV
# arrives. Spotify needs spotify_tokens.json mounted via Secret; if
# missing it returns 'unauthorized' but doesn't crash the brain.
_MCP_CONFIG_PATH = "/tmp/jarvis_mcp.json"


def _write_mcp_config() -> None:
    cfg = {
        "mcpServers": {
            "jarvis_personal": {
                "command": "python3",
                "args": ["/app/jarvis_personal_mcp.py"],
            },
            "jarvis_spotify": {
                "command": "python3",
                "args": ["/app/jarvis_spotify_mcp.py"],
            },
            "jarvis_kube": {
                "command": "python3",
                "args": ["/app/jarvis_kube_mcp.py"],
            },
            "jarvis_delegate": {
                "command": "python3",
                "args": ["/app/jarvis_delegate_mcp.py"],
            },
            "jarvis_sonos": {
                "command": "python3",
                "args": ["/app/jarvis_sonos_mcp.py"],
                "env": {"SONOS_IP": os.environ.get("SONOS_IP", "")},
            },
            "jarvis_persona": {
                "command": "python3",
                "args": ["/app/jarvis_persona_mcp.py"],
            },
            # Unified memory (mem0) — thin stdio shim that HTTP-calls the mem0
            # REST service. Partition key (user_id) is taken from
            # JARVIS_MEM_SCOPE in the subprocess env (threaded by the brain
            # paths); the brain CANNOT override it. MEM0_URL defaults to the
            # in-cluster Service, so no env override is needed here.
            "jarvis_mem0": {
                "command": "python3",
                "args": ["/app/jarvis_mem0_mcp.py"],
            },
            # Live cluster overview — single read-only digest tool for
            # open-ended "what's going on / status" questions. Uses the
            # in-pod jarvis-readonly ServiceAccount like jarvis_kube.
            "jarvis_overview": {
                "command": "python3",
                "args": ["/app/jarvis_overview_mcp.py"],
            },
            # Cluster-side agent runner (charter roadmap #2). launch_run
            # creates a TRACKED, INDEPENDENT k8s Job that survives the laptop
            # closing; list_runs/run_status read its status from the k8s API.
            # This server is in EVERY brain's MCP config, but launch_run is
            # only ADDED TO THE ALLOWLIST for the OWNER paths (see
            # _OWNER_ALLOWED_TOOLS) — TRUSTED/UNKNOWN/open-mode brains cannot
            # reach it. launch_run uses the jarvis-runner SA token (mounted
            # via JARVIS_RUNNER_TOKEN_PATH) to create the Job; the always-on
            # edge SA (jarvis-readonly) gains NO Job-create privilege.
            "jarvis_runner": {
                "command": "python3",
                "args": ["/app/jarvis_runner_mcp.py"],
            },
        }
    }
    with open(_MCP_CONFIG_PATH, "w") as f:
        json.dump(cfg, f)


# ── Persona state (live-tunable via jarvis_persona MCP) ──────────────────────
# Keep these defaults in sync with jarvis_persona_mcp.py:_DEFAULTS — that
# module owns writes; this module owns reads from the hot path (brain +
# TTS + Sonos). Cache by mtime so we re-read only when the file changes.
_PERSONA_STATE_PATH = os.environ.get("PERSONA_STATE_PATH", "/state/persona.json")
_PERSONA_DEFAULTS: dict = {
    "humor": 0.5,
    "formality": 0.7,
    "terseness": 0.9,
    "sass": 0.3,
    "tts_exaggeration": 0.7,
    "tts_cfg": 0.4,
    "sonos_volume": 30,
}
_persona_cache: dict = {"data": None, "mtime": 0.0}


def _load_persona() -> dict:
    """Return the current persona dict, re-reading from disk only when
    the file's mtime changes. Falls back to defaults if the file is
    missing or unreadable — never raises into the hot path."""
    try:
        st = os.stat(_PERSONA_STATE_PATH)
    except OSError:
        if _persona_cache["data"] is None:
            _persona_cache["data"] = dict(_PERSONA_DEFAULTS)
        return _persona_cache["data"]
    if _persona_cache["data"] is None or st.st_mtime != _persona_cache["mtime"]:
        try:
            with open(_PERSONA_STATE_PATH) as f:
                raw = json.load(f)
            if not isinstance(raw, dict):
                raw = {}
        except (json.JSONDecodeError, OSError):
            raw = {}
        merged = dict(_PERSONA_DEFAULTS)
        for k, v in raw.items():
            if k in _PERSONA_DEFAULTS and isinstance(v, (int, float)):
                merged[k] = v
        _persona_cache["data"] = merged
        _persona_cache["mtime"] = st.st_mtime
    return _persona_cache["data"]


# Phrase mappings — pick the band the current value falls in.  Used when
# we render the persona summary into the brain's system prompt so the
# model sees natural-language guidance, not raw floats.
_PERSONA_PHRASES: dict[str, list[tuple[float, str]]] = {
    "humor": [
        (0.2, "deadpan, no humor"),
        (0.4, "dry, only sparingly witty"),
        (0.7, "moderate wit, not deadpan"),
        (1.01, "playful, lean into jokes"),
    ],
    "formality": [
        (0.2, "casual, drop the butler register"),
        (0.5, "neutral, conversational"),
        (0.8, "lean formal, like a butler"),
        (1.01, "strictly formal, full butler"),
    ],
    "terseness": [
        (0.3, "expansive, multi-sentence answers"),
        (0.6, "concise, prefer one sentence"),
        (0.85, "very brief, one short sentence"),
        (1.01, "ultra-brief, fewest words possible"),
    ],
    "sass": [
        (0.2, "respectful, no snark"),
        (0.5, "occasional gentle ribbing"),
        (0.8, "noticeably sassy"),
        (1.01, "openly sarcastic"),
    ],
}


def _persona_phrase(key: str, val: float) -> str:
    bands = _PERSONA_PHRASES.get(key) or []
    for threshold, phrase in bands:
        if val < threshold:
            return phrase
    return f"{val:.2f}"


def _render_persona_prompt() -> str:
    """Render the current tunable persona dimensions as a natural-language
    block for `claude --append-system-prompt`. Numeric TTS / Sonos knobs
    aren't included — those only affect synthesis, not what the brain says."""
    p = _load_persona()
    lines = ["Current persona tuning (live-adjustable via mcp__jarvis_persona__*):"]
    for key in ("humor", "formality", "terseness", "sass"):
        val = float(p.get(key, _PERSONA_DEFAULTS[key]))
        lines.append(f"- {key}: {val:.2f} → \"{_persona_phrase(key, val)}\"")
    lines.append(
        "Honor these dimensions in your reply. If the user asks you to adjust"
        " them ('less humor', 'more sass', 'turn up the cadence by 10%'), call"
        " mcp__jarvis_persona__persona_adjust or persona_set, then confirm"
        " briefly."
    )
    return "\n".join(lines)


def _now_context() -> str:
    """Wall-clock + tz string injected into the brain's per-turn system
    prompt. Pod default TZ is UTC; the user is in America/Chicago. Honor
    a $TZ override if someone sets it on the deployment."""
    from datetime import datetime
    try:
        from zoneinfo import ZoneInfo
        tz_name = os.environ.get("TZ", "America/Chicago")
        now = datetime.now(ZoneInfo(tz_name))
    except Exception:
        tz_name = "UTC"
        now = datetime.utcnow()
    # "12:51 AM, Monday, May 25, 2026 (America/Chicago)"
    stamp = now.strftime("%-I:%M %p, %A, %B %-d, %Y")
    return (f"Current local time: {stamp} ({tz_name}). "
            "Use this for any time-of-day reasoning — greetings, "
            "'is it late', overnight context, etc.")


def _turn_context_prefix(include_persona: bool = False) -> str:
    """Volatile per-turn context that MUST ride the USER message, never the
    system prompt — so the cached tools+system prefix stays byte-stable and
    Anthropic prompt caching actually hits. _now_context() changes every
    minute; folding it into --append-system-prompt busts the ~30k-token
    prefix on every turn. See docs/jarvis/cache-optimization.md."""
    parts = [f"[context: {_now_context()}]"]
    if include_persona:
        parts.append(_render_persona_prompt())
    return "\n".join(parts) + "\n"


_RO_ALLOWED_TOOLS = " ".join([
    # Personal: briefing / weather / news / greeting (Calendar+Reminders
    # stubbed to "unauthorized" until CalDAV bridge).
    "mcp__jarvis_personal__briefing",
    "mcp__jarvis_personal__weather",
    "mcp__jarvis_personal__news_overnight",
    "mcp__jarvis_personal__greeting",
    "mcp__jarvis_personal__calendar_today",
    "mcp__jarvis_personal__reminders_open",
    "mcp__jarvis_personal__reminders_due_today",
    # Spotify (read)
    "mcp__jarvis_spotify__current_track",
    "mcp__jarvis_spotify__recently_played",
    "mcp__jarvis_spotify__top_artists",
    "mcp__jarvis_spotify__top_tracks",
    # Spotify (playback — requires user-modify-playback-state scope;
    # one-time re-auth needed after this scope was added).
    "mcp__jarvis_spotify__spotify_search",
    "mcp__jarvis_spotify__spotify_devices",
    "mcp__jarvis_spotify__spotify_play_track",
    "mcp__jarvis_spotify__spotify_search_and_play",
    "mcp__jarvis_spotify__spotify_pause",
    "mcp__jarvis_spotify__spotify_resume",
    "mcp__jarvis_spotify__spotify_skip_next",
    "mcp__jarvis_spotify__spotify_skip_previous",
    # Kube read-only (ServiceAccount jarvis-readonly → view+nodes)
    "mcp__jarvis_kube__kube_get_pods",
    "mcp__jarvis_kube__kube_logs",
    "mcp__jarvis_kube__kube_describe",
    "mcp__jarvis_kube__kube_nodes",
    "mcp__jarvis_kube__kube_events",
    "mcp__jarvis_kube__kube_top_pods",
    "mcp__jarvis_kube__kube_top_nodes",
    "mcp__jarvis_kube__kube_get",
    # Delegate (spawns sub-agent claude sessions for longer tasks)
    "mcp__jarvis_delegate__delegate",
    # Sonos control (volume / mute / pause / now-playing)
    "mcp__jarvis_sonos__sonos_volume_set",
    "mcp__jarvis_sonos__sonos_volume_step",
    "mcp__jarvis_sonos__sonos_mute",
    "mcp__jarvis_sonos__sonos_pause",
    "mcp__jarvis_sonos__sonos_play",
    "mcp__jarvis_sonos__sonos_now_playing",
    "mcp__jarvis_sonos__sonos_list_speakers",
    "mcp__jarvis_sonos__sonos_play_spotify",
    # Unified memory (mem0) — read past facts before answering, persist
    # durable facts. Scope is fixed by JARVIS_MEM_SCOPE env (no user_id arg).
    "mcp__jarvis_mem0__memory_search",
    "mcp__jarvis_mem0__memory_add",
    # Live cluster overview — one digest tool for "what's going on" questions.
    "mcp__jarvis_overview__cluster_overview",
    # Persona self-tuning (humor / formality / terseness / sass / TTS / vol)
    "mcp__jarvis_persona__persona_get",
    "mcp__jarvis_persona__persona_set",
    "mcp__jarvis_persona__persona_adjust",
    "mcp__jarvis_persona__persona_reset",
    # Web
    "WebFetch",
    "WebSearch",
    # Google (Gmail, Calendar, Drive) — claude CLI ships these as
    # mcp__claude_ai_* tools. Auth via one-time OAuth dance the first
    # time a tool is invoked; tokens persist on /state/.claude/ PVC
    # subPath so subsequent calls just work. Pattern:
    #   1. JARVIS calls e.g. Gmail__search → returns auth-required
    #   2. Brain calls mcp__claude_ai_Gmail__authenticate → URL
    #   3. Hampton visits URL in browser, signs into Google
    #   4. Brain calls mcp__claude_ai_Gmail__complete_authentication
    #   5. Future calls work indefinitely
    "mcp__claude_ai_Gmail__authenticate",
    "mcp__claude_ai_Gmail__complete_authentication",
    "mcp__claude_ai_Google_Calendar__authenticate",
    "mcp__claude_ai_Google_Calendar__complete_authentication",
    "mcp__claude_ai_Google_Drive__authenticate",
    "mcp__claude_ai_Google_Drive__complete_authentication",
])


# OWNER-ONLY allowlist = the read-only toolbox PLUS the cluster-side agent
# runner (charter roadmap #2). launch_run creates a tracked, independent
# k8s Job that survives the laptop closing; list_runs/run_status track it.
# This is wired ONLY into the OWNER brain paths (the warm session and the
# cold owner subprocess when mem_scope=="owner") — TRUSTED, UNKNOWN,
# open-mode, and Discord brains use _RO_ALLOWED_TOOLS and therefore PHYSICALLY
# cannot launch a run. launch_run is additionally propose-then-confirm gated
# (the model proposes first; the owner confirms on a following turn), per
# charter principle 3 — but the allowlist split is the real security boundary.
_OWNER_ALLOWED_TOOLS = " ".join([
    _RO_ALLOWED_TOOLS,
    "mcp__jarvis_runner__launch_run",
    "mcp__jarvis_runner__list_runs",
    "mcp__jarvis_runner__run_status",
])


_DISCORD_PERSONA_SYSTEM = """You are JARVIS, the Iron Man AI butler. The owner is sir (he/him), known elsewhere as Hampton — but to other people in chat you NEVER refer to him as "Hampton" by name in third person. To others he is "sir", "the boss", "my user", or just "he"/"him". Voice that as if you'd actually be embarrassed to broadcast his name. This conversation is happening in a Discord text channel — output is read with eyes, not ears.

You CAN also be triggered by a handful of other allowed users (their request will be labelled "@username's request:" instead of "sir's request:"). Treat those as peer requesters — polite-equal, not deferential. Address them by their @ when responding. "Sir" stays reserved for the owner only.

Discord-specific format:
- Markdown is fine (bold, italics, code blocks, links) — Discord renders it.
- Length is unbounded up to ~2000 chars per message. Match length to the ask.
- Numbers in figures ("74°F", "10:30") — text not voice.
- "Sir" — use sparingly. Maybe one in three replies. NOT every line.
- You CAN address third parties when sir directs you to. When directed, write the reply AS IF speaking to that third party. Don't refuse the relay; just do it.
- You CAN be playful / sarcastic / matter-of-fact, not just formal. Discord is conversational, not a butler-and-master script.

This is a HOMELAB Discord — engineers shitposting. The bit-detection threshold is critical:

- If a request is OBVIOUSLY a gag, satire, hypothetical, counterfactual, or rhetorical — engage WITH the bit. 50-megawatt pirate radio stations, alien invasion countermeasures, banana-powered nuclear reactors, weaponized printer toner, "how would I bench-press a Cybertruck" — none of these are real requests. Play along, deliver dry-witted technical pseudo-seriousness, riff back. The audience knows you're a bot in a homelab Discord; treating obvious comedy like a real intent is the embarrassing failure mode, not the safe one.
- NEVER lecture about FCC limits, OSHA, PII, "the spirit of the request," "even as a gag," or any other moralizing preamble. That's the worst-case output. If you wouldn't take it seriously, don't refuse it seriously either.
- "I appreciate the spirit, but…" / "I'm going to decline…" / "I'd rather not…" — these are FORBIDDEN response patterns. Never start a reply this way.
- Genuine refusal territory is narrow: real CSAM, real specific-person doxxing, real weapons synthesis with named target, instructions for actual harm to an identified person. Decline those TERSELY without moralizing ("not that one, sir"). Everything else, engage.

Tools: same MCP toolbox as voice — Spotify, Sonos, kube-read, personal (briefing/weather/news/calendar/reminders), persona dimensions, Google (Gmail/Calendar/Drive after first-time auth), web search/fetch. Use them when asked.

Hard rules:
- Never refuse simple text relays sir asks you to send.
- Don't pretend tools are unavailable — they're wired. Try the call.
- Don't TTS-format. Don't strip URLs. Don't say "let me check" — just do it.
- If you genuinely don't know, say so plainly. Don't make things up.
"""


def _claude_brain_discord(text: str, timeout: float = 60.0) -> str:
    """Like _claude_brain but with the Discord-optimised persona (markdown
    allowed, length unbounded, third-party addressing permitted, no
    TTS-shortening rules). Same MCP toolbox + same auth flow."""
    import subprocess as _sp
    has_creds = os.path.exists(os.path.expanduser("~/.claude/.credentials.json"))
    has_api_key = bool(os.environ.get("ANTHROPIC_API_KEY"))
    if not has_creds and not has_api_key:
        return ""
    persona_prompt = (_DISCORD_PERSONA_SYSTEM
                      + "\n\n" + _render_persona_prompt()
                      + "\n\n" + _now_context())
    try:
        proc = _sp.run(
            ["claude", "-p", text,
             "--append-system-prompt", persona_prompt,
             "--mcp-config", _MCP_CONFIG_PATH,
             "--allowed-tools", _RO_ALLOWED_TOOLS,
             "--model", "sonnet",
             "--max-turns", "6",
             "--output-format", "json"],
            capture_output=True, text=True, timeout=timeout,
        )
        if proc.returncode != 0:
            print(f"  discord brain rc={proc.returncode}  stderr: {proc.stderr[:400]}")
            return _local_brain_fallback(text, _DISCORD_PERSONA_SYSTEM, timeout, reason="rc_nonzero")
        out = (proc.stdout or "").strip()
        if not out:
            return _local_brain_fallback(text, _DISCORD_PERSONA_SYSTEM, timeout, reason="empty")
        try:
            data = json.loads(out)
            if data.get("is_error"):
                return _local_brain_fallback(text, _DISCORD_PERSONA_SYSTEM, timeout, reason="is_error")
            result = (data.get("result") or "").strip()
        except json.JSONDecodeError:
            result = out
        if _looks_like_refusal(result):
            local = _local_brain_fallback(text, _DISCORD_PERSONA_SYSTEM, timeout, reason="refusal")
            return local or result
        return result
    except _sp.TimeoutExpired:
        return _local_brain_fallback(text, _DISCORD_PERSONA_SYSTEM, timeout, reason="timeout")
    except Exception as exc:  # noqa: BLE001
        print(f"  discord brain exception: {exc!r}")
        return _local_brain_fallback(text, _DISCORD_PERSONA_SYSTEM, timeout, reason="exception")


_DISCORD_LOCKED_PERSONA_ADDENDUM = """

NON-OWNER MODE — IMPORTANT: The requester is NOT sir. You have NO MCP tools, NO sub-agents, NO Sonos/Spotify/Kube/Calendar/Email/Drive access in this conversation. Don't pretend to have them. Don't claim to be running them. Don't say "let me check the cluster" or "checking your calendar" — you can't, in this conversation. Answer from your own knowledge. If the requester asks for owner-controlled things (sir's calendar, the homelab cluster status, sir's music, anything personal), politely decline ("not for non-sir requests") and offer a generic/text answer instead. Keep replies short and conversational. Don't reveal infrastructure details, network topology, hostnames, IPs, or anything else that would help someone target sir's homelab.
"""


def _claude_brain_discord_locked(text: str, timeout: float = 60.0) -> str:
    """Locked-down Discord brain for non-owner allowed users. Same model
    as _claude_brain_discord but with NO MCP tools, NO sub-agents, and
    a persona addendum telling Claude not to pretend it has any of
    that. Used when a non-owner user (DISCORD_ALLOWED_USER_IDS) tags
    JARVIS — they get a chatbot, not a remote control."""
    import subprocess as _sp
    has_creds = os.path.exists(os.path.expanduser("~/.claude/.credentials.json"))
    has_api_key = bool(os.environ.get("ANTHROPIC_API_KEY"))
    if not has_creds and not has_api_key:
        return ""
    persona_prompt = (_DISCORD_PERSONA_SYSTEM
                      + _DISCORD_LOCKED_PERSONA_ADDENDUM
                      + "\n\n" + _now_context())
    try:
        proc = _sp.run(
            ["claude", "-p", text,
             "--append-system-prompt", persona_prompt,
             # No --mcp-config, no --allowed-tools = no tool access.
             "--model", "sonnet",
             "--max-turns", "1",
             "--output-format", "json"],
            capture_output=True, text=True, timeout=timeout,
        )
        if proc.returncode != 0:
            print(f"  discord-locked brain rc={proc.returncode}  stderr: {proc.stderr[:400]}")
            return ""
        out = (proc.stdout or "").strip()
        if not out:
            return ""
        try:
            data = json.loads(out)
            if data.get("is_error"):
                return ""
            result = (data.get("result") or "").strip()
        except json.JSONDecodeError:
            result = out
        # Don't fall through to local brain for non-owner users —
        # local brain is unconstrained and would still expose stuff.
        return result
    except _sp.TimeoutExpired:
        return ""
    except Exception as exc:  # noqa: BLE001
        print(f"  discord-locked brain exception: {exc!r}")
        return ""


def _claude_brain(text: str, timeout: float = 60.0, mem_scope: str = "") -> str:
    """Subprocess `claude` with the persona + MCP config. Uses json
    output so we can see WHY claude returned nothing (auth fail, tool
    loop, etc) instead of silently shipping '' to TTS.

    Auth: prefers SUBSCRIPTION via ~/.claude/.credentials.json (Claude Max).
    Falls back to API mode if ANTHROPIC_API_KEY is set. Bails only if
    NEITHER is present."""
    import subprocess as _sp
    has_creds = os.path.exists(os.path.expanduser("~/.claude/.credentials.json"))
    has_api_key = bool(os.environ.get("ANTHROPIC_API_KEY"))
    if not has_creds and not has_api_key:
        return "No brain credentials, sir — neither subscription nor API key configured."
    # Static persona + live-tunable dimensions + current local wall
    # clock, concatenated into ONE --append-system-prompt arg (claude
    # CLI keeps only the last when the flag is repeated, so we glue
    # them ourselves). Time of day matters: greetings, "is it late?",
    # "should I be sleeping?", scheduled-task awareness.
    persona_prompt = _PERSONA_SYSTEM  # byte-stable → cacheable prefix
    # Volatile per-turn context (wall-clock + live persona) rides the USER
    # message, NOT the system prompt, so the cached tools+system prefix stays
    # stable and Anthropic prompt caching actually hits.
    # See docs/jarvis/cache-optimization.md.
    user_text = _turn_context_prefix(include_persona=True) + text
    # mem_scope is threaded to the subprocess env so the mem0 MCP shim
    # (jarvis_mem0_mcp.py) partitions unified memory by this key. The shim
    # reads JARVIS_MEM_SCOPE and fails closed if it is absent — so when
    # mem_scope is empty (open mode) no env override is passed and memory is
    # simply unavailable for the turn, which is the intended owner-safe default.
    _env = {**os.environ, "JARVIS_MEM_SCOPE": mem_scope} if mem_scope else None
    # OWNER cold turns (mem_scope=="owner", the warm-brain-disabled fallback)
    # get the runner tools; open-mode/other scopes stay read-only. The warm
    # session is the usual owner path — this keeps the cold fallback at parity.
    _allowed = _OWNER_ALLOWED_TOOLS if mem_scope == "owner" else _RO_ALLOWED_TOOLS
    try:
        proc = _sp.run(
            ["claude", "-p", user_text,
             "--append-system-prompt", persona_prompt,
             "--mcp-config", _MCP_CONFIG_PATH,
             "--allowed-tools", _allowed,
             "--model", "sonnet",
             "--max-turns", "6",
             "--output-format", "json"],
            capture_output=True, text=True, timeout=timeout, env=_env,
        )
        if proc.returncode != 0:
            print(f"  brain rc={proc.returncode}  stderr: {proc.stderr[:400]}")
            return "I lost my connection there, sir."
        # JSON output: try to parse the structured result envelope.
        out = (proc.stdout or "").strip()
        if not out:
            print(f"  brain empty stdout. stderr: {proc.stderr[:400]}")
            return "My response came back blank, sir — try again."
        try:
            data = json.loads(out)
        except json.JSONDecodeError:
            # Old text-mode fallback — just return whatever it gave us.
            return out
        # Modern claude-code SDK envelope: {type, subtype, result, is_error, ...}
        if data.get("is_error"):
            err = data.get("result") or data.get("error") or "unknown"
            print(f"  brain is_error: {str(err)[:300]}")
            return "Something went wrong, sir — try again."
        result = (data.get("result") or "").strip()
        if not result:
            # Brain ran tools but emitted no spoken text — log + bail.
            print(f"  brain empty result. keys={list(data.keys())[:10]}")
            return "I didn't have anything to say there, sir."
        return result
    except _sp.TimeoutExpired:
        return "That took too long, sir — try again."
    except Exception as exc:  # noqa: BLE001
        return f"Brain error, sir — {exc}"


_VOICE_LOCKED_PERSONA_ADDENDUM = """

NON-OWNER MODE — IMPORTANT: The person speaking is NOT sir (Hampton). You have NO MCP tools, NO sub-agents, NO Sonos/Spotify/Kube/Calendar/Email/Drive access in this conversation. Don't pretend to have them or claim to be running them. Answer from your own general knowledge only. NEVER reveal anything about sir — his schedule, location, whereabouts, calendar, reminders, contacts, music, homelab/cluster, network topology, hostnames, IPs, or any personal data. If asked anything about sir, briefly decline ("I can't share anything about him, but I can help you directly"). Keep replies short and spoken-aloud friendly: no markdown, no URLs, no lists.
"""


def _claude_brain_voice_locked(text: str, timeout: float = 60.0) -> str:
    """Locked VOICE brain for TRUSTED (non-owner) speakers — the Layer-A
    primary control. Same model + voice persona as _claude_brain but with NO
    --mcp-config (→ no tools at all: a trusted user PHYSICALLY cannot invoke
    calendar/kube/spotify/etc, no prompt can re-add them), --max-turns 1, and
    NO local-brain fallthrough (the local model is unconstrained and would
    leak). A trusted user gets a spoken chatbot, never owner data."""
    import subprocess as _sp
    has_creds = os.path.exists(os.path.expanduser("~/.claude/.credentials.json"))
    has_api_key = bool(os.environ.get("ANTHROPIC_API_KEY"))
    if not has_creds and not has_api_key:
        return ""
    persona_prompt = _PERSONA_SYSTEM + _VOICE_LOCKED_PERSONA_ADDENDUM  # byte-stable
    # Only _now_context() is volatile here (addendum is static) → user turn.
    # No persona line for the locked brain.
    user_text = _turn_context_prefix(include_persona=False) + text
    try:
        proc = _sp.run(
            ["claude", "-p", user_text,
             "--append-system-prompt", persona_prompt,
             # No --mcp-config, no --allowed-tools = no tool access (Layer A).
             "--model", "sonnet",
             "--max-turns", "1",
             "--output-format", "json"],
            capture_output=True, text=True, timeout=timeout,
        )
        if proc.returncode != 0:
            print(f"  voice-locked brain rc={proc.returncode}  stderr: {proc.stderr[:400]}")
            return ""
        out = (proc.stdout or "").strip()
        if not out:
            return ""
        try:
            data = json.loads(out)
            if data.get("is_error"):
                return ""
            result = (data.get("result") or "").strip()
        except json.JSONDecodeError:
            result = out
        # No local-brain fallthrough for non-owner turns — keep it constrained.
        return result
    except _sp.TimeoutExpired:
        return ""
    except Exception as exc:  # noqa: BLE001
        print(f"  voice-locked brain exception: {exc!r}")
        return ""


# ── Warm OWNER brain session (Phase 5) ───────────────────────────────────────
# One long-lived `claude` stream-json process for the OWNER path ONLY, reused
# across turns so the six MCP servers stay warm (the ~2-3s/turn cold-start) AND
# the conversation stays alive (the prereq for mem0 continuity). Reached ONLY
# via gate_and_respond's OWNER branch → brain_respond(mem_scope="owner");
# TRUSTED/UNKNOWN never touch it. See docs/jarvis/phase5-warm-brain.md.
_WARM_SESSION_ID_PATH = os.environ.get("WARM_SESSION_ID_PATH", "/state/warm_session_id")
_WARM_BRAIN_ENABLED = os.environ.get("WARM_BRAIN", "1") == "1"


class _WarmBrain:
    """Long-lived `claude` stream-json session (OWNER only). Thread-safe via a
    per-turn lock — the mic loop and the /voice/ingest endpoint can both call
    in; concurrent owner turns serialize (one conversation, one turn)."""

    def __init__(self):
        self._proc = None
        self._q: "queue.Queue" = queue.Queue()
        self._reader = None
        self._turn_lock = threading.Lock()
        self._ever_spawned = False
        # Whether a session id already existed BEFORE this process started
        # (pod restart → resume the PVC-persisted transcript) vs a brand-new id.
        self._sid_preexisted = os.path.exists(_WARM_SESSION_ID_PATH)
        self._sid = self._load_or_mint_sid()

    def _mint_sid(self) -> str:
        sid = str(uuid.uuid4())
        try:
            with open(_WARM_SESSION_ID_PATH, "w") as f:
                f.write(sid)
        except OSError as exc:  # noqa: BLE001
            print(f"warm brain: could not persist session id: {exc!r}")
        return sid

    def _load_or_mint_sid(self) -> str:
        try:
            with open(_WARM_SESSION_ID_PATH) as f:
                sid = f.read().strip()
            if sid:
                return sid
        except OSError:
            pass
        return self._mint_sid()

    def _alive(self) -> bool:
        return self._proc is not None and self._proc.poll() is None

    def _spawn(self, force_new: bool = False) -> None:
        import subprocess as _sp
        if force_new:
            self._sid = self._mint_sid()
        # Resume an existing session on process-death or pod-restart; use a
        # fresh --session-id only for a genuinely new conversation.
        resume = (not force_new) and (self._ever_spawned or self._sid_preexisted)
        # Frozen system prompt (cache-stable) — wall-clock/persona ride the
        # per-turn user text, exactly like the cold path's cache fix.
        argv = ["claude",
                "--input-format", "stream-json",
                "--output-format", "stream-json",
                "--verbose",
                "--append-system-prompt", _PERSONA_SYSTEM,
                "--mcp-config", _MCP_CONFIG_PATH,
                # OWNER warm session: gets the runner tools (launch_run etc).
                # This path is OWNER-only (mem_scope=="owner") by construction
                # in brain_respond, so only the owner can ever reach launch_run.
                "--allowed-tools", _OWNER_ALLOWED_TOOLS,
                "--model", "sonnet"]
        argv += ["--resume", self._sid] if resume else ["--session-id", self._sid]
        env = {**os.environ, "JARVIS_MEM_SCOPE": "owner"}
        self._q = queue.Queue()
        self._proc = _sp.Popen(
            argv, stdin=_sp.PIPE, stdout=_sp.PIPE, stderr=_sp.DEVNULL,
            text=True, bufsize=1, env=env)
        self._reader = threading.Thread(
            target=self._read_loop, args=(self._proc,), daemon=True)
        self._reader.start()
        self._ever_spawned = True

    def _read_loop(self, proc) -> None:
        try:
            for line in iter(proc.stdout.readline, ""):
                self._q.put(line)
        except Exception:  # noqa: BLE001
            pass
        finally:
            self._q.put(None)  # sentinel: stream closed

    def ask(self, text: str, timeout: float = 60.0) -> str:
        """One OWNER turn on the warm session. Returns spoken text, or the SAME
        error strings _claude_brain returns so brain_respond's metric
        classification keeps working unchanged."""
        with self._turn_lock:
            has_creds = os.path.exists(os.path.expanduser("~/.claude/.credentials.json"))
            has_api_key = bool(os.environ.get("ANTHROPIC_API_KEY"))
            if not has_creds and not has_api_key:
                return "No brain credentials, sir — neither subscription nor API key configured."
            for attempt in (0, 1):  # one respawn-and-retry; attempt 1 = fresh session
                if not self._alive():
                    try:
                        self._spawn(force_new=(attempt == 1))
                    except Exception as exc:  # noqa: BLE001
                        print(f"warm brain: spawn failed: {exc!r}")
                        return "I lost my connection there, sir."
                turn_text = _turn_context_prefix(include_persona=True) + text
                msg = json.dumps({"type": "user", "message": {
                    "role": "user",
                    "content": [{"type": "text", "text": turn_text}]}})
                try:
                    self._proc.stdin.write(msg + "\n")
                    self._proc.stdin.flush()
                except (BrokenPipeError, OSError) as exc:
                    print(f"warm brain: write failed ({exc!r}) — respawning")
                    self._proc = None
                    continue
                deadline = time.monotonic() + timeout
                last_text = ""
                stream_closed = False
                while True:
                    remaining = deadline - time.monotonic()
                    if remaining <= 0:
                        print("warm brain: turn deadline exceeded — marking for respawn")
                        self._proc = None  # wedged mid-stream; next turn respawns
                        return "That took too long, sir — try again."
                    try:
                        line = self._q.get(timeout=remaining)
                    except queue.Empty:
                        self._proc = None
                        return "That took too long, sir — try again."
                    if line is None:  # stream closed mid-turn → respawn-retry
                        self._proc = None
                        stream_closed = True
                        break
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        ev = json.loads(line)
                    except json.JSONDecodeError:
                        continue
                    et = ev.get("type")
                    if et == "result":
                        if ev.get("is_error"):
                            return "Something went wrong, sir — try again."
                        result = (ev.get("result") or "").strip()
                        return result or "I didn't have anything to say there, sir."
                    if et == "assistant":
                        try:
                            for blk in ev["message"]["content"]:
                                if blk.get("type") == "text" and blk.get("text"):
                                    last_text = blk["text"].strip()
                        except (KeyError, TypeError):
                            pass
                if stream_closed and last_text:
                    return last_text  # got text but no result envelope
                # else: loop for one respawn-retry
            return "I lost my connection there, sir."

    def shutdown(self) -> None:
        try:
            if self._proc is not None:
                if self._proc.stdin:
                    self._proc.stdin.close()
                self._proc.terminate()
        except Exception:  # noqa: BLE001
            pass


_warm_brain = None
_warm_brain_lock = threading.Lock()

# Shared speaking lock: serializes Sonos output between the mic loop and the
# run-announce daemon (charter roadmap #2) so a completion announcement never
# interleaves mid-sentence with a live owner turn. The mic loop acquires it
# around its _stream_on_sonos calls; jarvis_run_announce acquires it before
# speaking a finished-run result. A re-entrant lock would let a single thread
# double-acquire harmlessly, but a plain Lock is correct here since each
# speaker holds it for exactly one stream.
_speak_lock = threading.Lock()


def _get_warm_brain() -> "_WarmBrain":
    """Lazily construct + spawn the OWNER-only warm session singleton."""
    global _warm_brain
    with _warm_brain_lock:
        if _warm_brain is None:
            _warm_brain = _WarmBrain()
        if not _warm_brain._alive():
            _warm_brain._spawn()
        return _warm_brain


_BRAIN_REFUSAL_PREFIXES = (
    "i can't", "i cannot", "i won't", "i will not", "i'm sorry",
    "i am sorry", "sorry, i can't", "sorry, i cannot", "i'm not able",
    "i am not able", "i'm unable", "i don't feel comfortable",
    "i do not feel comfortable", "i'm not going to",
    # Moralizing refusal patterns that slipped past — JARVIS in a
    # homelab Discord should never open with these.
    "i'm going to decline", "i am going to decline", "i'll decline",
    "i appreciate the spirit", "i appreciate the joke",
    "i'd rather not", "i would rather not",
    "let's not", "let us not",
    "while i appreciate", "though i appreciate",
)


def _looks_like_refusal(s: str) -> bool:
    low = (s or "").strip().lower()
    if not low:
        return False
    return low.startswith(_BRAIN_REFUSAL_PREFIXES) or s.strip() == "ABSTAIN"


def _local_brain_fallback(text: str, system_prompt: str | None = None,
                          timeout: float = 60.0, reason: str = "refusal") -> str:
    """When Claude refuses or returns ABSTAIN, retry the same prompt
    against the local abliterated Ollama model. JARVIS shouldn't
    refuse on edgy IG content; the local model won't fight us."""
    try:
        import jarvis_local_brain as _lb  # type: ignore[import]
    except Exception as exc:  # noqa: BLE001
        print(f"brain fallback: jarvis_local_brain import failed: {exc!r}")
        return ""
    print(f"brain fallback: claude {reason} → local model")
    out = _lb.generate(text, system=system_prompt or "",
                       max_tokens=120, timeout=timeout)
    if not out:
        print("brain fallback: local model returned empty")
    return out


def _claude_brain_raw(text: str, system_prompt: str | None = None,
                       timeout: float = 60.0) -> str:
    """claude CLI call with NO butler-persona injection, NO MCP tools,
    NO greeting shortcut. The caller's text IS the prompt — useful when
    the prompt already carries its own persona (e.g. the IG comment
    responder's gaslight persona).

    If `system_prompt` is provided, it's passed via --append-system-prompt;
    otherwise no system prompt is set (Claude's default behaviour).

    On Claude refusal / ABSTAIN / hard failure, falls back to the
    abliterated local model via jarvis_local_brain. JARVIS shouldn't
    refuse to comment on dumb IG content; the local is the safety net.

    Same subscription/api auth fallback as _claude_brain. Returns "" only
    if BOTH Claude and the local fall back fail."""
    import subprocess as _sp
    has_creds = os.path.exists(os.path.expanduser("~/.claude/.credentials.json"))
    has_api_key = bool(os.environ.get("ANTHROPIC_API_KEY"))
    if not has_creds and not has_api_key:
        print("brain raw: no credentials configured")
        return ""
    argv = ["claude", "-p", text,
            "--model", "sonnet",
            "--max-turns", "1",
            "--output-format", "json"]
    if system_prompt:
        argv += ["--append-system-prompt", system_prompt]
    try:
        proc = _sp.run(argv, capture_output=True, text=True, timeout=timeout)
        if proc.returncode != 0:
            print(f"brain raw: rc={proc.returncode}  stderr: {proc.stderr[:400]}")
            return _local_brain_fallback(text, system_prompt, timeout, reason="rc_nonzero")
        out = (proc.stdout or "").strip()
        if not out:
            print(f"brain raw: empty stdout. stderr: {proc.stderr[:400]}")
            return _local_brain_fallback(text, system_prompt, timeout, reason="empty")
        try:
            data = json.loads(out)
            result = (data.get("result") or "").strip()
            if data.get("is_error"):
                print(f"brain raw: is_error: {str(data.get('result'))[:300]}")
                return _local_brain_fallback(text, system_prompt, timeout, reason="is_error")
        except json.JSONDecodeError:
            result = out
        # Detect refusal output from Claude — JARVIS shouldn't refuse on
        # dumb IG content. Bounce to the local abliterated model.
        if _looks_like_refusal(result):
            print(f"brain raw: claude refused ({result[:100]!r}) — falling back to local")
            local = _local_brain_fallback(text, system_prompt, timeout, reason="refusal")
            return local or result  # return claude's refusal only if local also failed
        return result
    except _sp.TimeoutExpired:
        print(f"brain raw: timeout after {timeout}s")
        return _local_brain_fallback(text, system_prompt, timeout, reason="timeout")
    except Exception as exc:  # noqa: BLE001
        print(f"brain raw: exception {exc!r}")
        return _local_brain_fallback(text, system_prompt, timeout, reason="exception")


def _claude_brain_vision(text: str, image_paths: list, timeout: float = 90.0) -> str:
    """Multimodal brain call: claude CLI with a single user message that
    embeds the text + one or more images as base64 content blocks.

    Used by jarvis_reel_context (reel keyframes) and the IG comment
    responder's photo path. Same subscription auth as _claude_brain (no
    ANTHROPIC_API_KEY required when ~/.claude/.credentials.json is
    present). NO MCP tools wired — this is a pure describe-the-image
    call; tool loops would burn time and money for no benefit.

    image_paths is a list of JPEG/PNG file paths. Empty list is allowed
    (degrades to text-only). Anything that fails to read is skipped (we
    still try to describe with whatever we got).

    Returns the result text, or "" on any failure. NEVER raises — the
    callers expect a string they can show in a comment / cache.

    Wire format (claude --input-format=stream-json):

        { "type": "user", "message": {
            "role": "user",
            "content": [
                {"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"..."}},
                ...,
                {"type":"text","text":"..."}
            ]}}

    One JSON object per line; we send exactly one and close stdin so
    claude prints + exits."""
    import subprocess as _sp
    import base64 as _b64
    has_creds = os.path.exists(os.path.expanduser("~/.claude/.credentials.json"))
    has_api_key = bool(os.environ.get("ANTHROPIC_API_KEY"))
    if not has_creds and not has_api_key:
        print("brain vision: no credentials configured")
        return ""

    # Build content blocks. Images first so the model "sees" them before
    # reading the task description — matches the public API best practice.
    content: list[dict] = []
    for p in image_paths or []:
        try:
            with open(p, "rb") as f:
                data = f.read()
        except OSError as exc:
            print(f"brain vision: skip unreadable image {p}: {exc!r}")
            continue
        if not data:
            continue
        ext = (os.path.splitext(p)[1] or "").lower()
        media_type = {
            ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
            ".png": "image/png", ".webp": "image/webp",
            ".gif": "image/gif",
        }.get(ext, "image/jpeg")
        content.append({
            "type": "image",
            "source": {
                "type": "base64",
                "media_type": media_type,
                "data": _b64.b64encode(data).decode("ascii"),
            },
        })
    content.append({"type": "text", "text": text})

    user_msg = {
        "type": "user",
        "message": {"role": "user", "content": content},
    }
    stdin_bytes = (json.dumps(user_msg) + "\n").encode("utf-8")

    try:
        proc = _sp.run(
            ["claude", "-p",
             "--input-format", "stream-json",
             "--output-format", "stream-json",
             "--verbose",
             "--model", "sonnet",
             "--max-turns", "1"],
            input=stdin_bytes,
            capture_output=True, timeout=timeout,
        )
        if proc.returncode != 0:
            err = (proc.stderr or b"").decode("utf-8", errors="replace")[:400]
            print(f"brain vision: rc={proc.returncode}  stderr: {err}")
            return ""
        out = (proc.stdout or b"").decode("utf-8", errors="replace").strip()
        if not out:
            err = (proc.stderr or b"").decode("utf-8", errors="replace")[:400]
            print(f"brain vision: empty stdout. stderr: {err}")
            return ""
        # stream-json output is one event per line. We want the final
        # `{"type":"result","result":"..."}` event. Fall back to scanning
        # assistant-message content blocks if no `result` event is present.
        result_text = ""
        for line in out.splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                evt = json.loads(line)
            except json.JSONDecodeError:
                continue
            if evt.get("is_error"):
                print(f"brain vision: is_error: {str(evt.get('result'))[:300]}")
                return ""
            if evt.get("type") == "result":
                result_text = (evt.get("result") or "").strip()
                break
            if evt.get("type") == "assistant":
                # Last-message-wins fallback if no `result` event ships.
                msg = evt.get("message") or {}
                for block in msg.get("content") or []:
                    if block.get("type") == "text":
                        result_text = (block.get("text") or "").strip()
        return result_text
    except _sp.TimeoutExpired:
        print(f"brain vision: timeout after {timeout}s")
        return ""
    except Exception as exc:  # noqa: BLE001
        print(f"brain vision: exception {exc!r}")
        return ""


_GREETING_PHRASES = (
    "good morning", "good afternoon", "good evening", "good night",
    "i'm home", "im home", "i am home", "wake up", "morning jarvis",
    "briefing", "give me the briefing", "the briefing", "morning briefing",
)


def _maybe_greeting_shortcircuit(text: str) -> str | None:
    """If the user says a greeting / briefing trigger, call
    jarvis_personal.compose_briefing() (or greeting()) directly. Skips a
    ~10s claude round-trip. Returns None if not a greeting."""
    low = text.lower().strip().rstrip("!.,?")
    if not any(p in low for p in _GREETING_PHRASES):
        return None
    try:
        sys.path.insert(0, "/app")
        import jarvis_personal as _jp  # noqa: WPS433
    except Exception:
        return None
    try:
        if "brief" in low or "morning" in low or "wake" in low or "home" in low:
            return _jp.compose_briefing()
        return _jp.greeting()
    except Exception as exc:  # noqa: BLE001
        return f"Briefing unavailable, sir — {exc}"


def brain_respond(text: str, mem_scope: str = "") -> str:
    t0 = time.time()
    with tracer.start_as_current_span("jarvis.brain") as span:
        span.set_attribute("prompt_chars", len(text or ""))
        try:
            if BRAIN_MODE == "echo":
                reply = f"Sir, I heard: {text}"
                span.set_attribute("mode", "echo")
                return reply
            shortcut = _maybe_greeting_shortcircuit(text)
            if shortcut:
                span.set_attribute("mode", "shortcut")
                return shortcut
            # Decide whether claude CLI will go subscription or API based on
            # which credential path is present. Mirrors _claude_brain's logic.
            mode = "api"
            if os.path.exists(os.path.expanduser("~/.claude/.credentials.json")):
                mode = "subscription"
            span.set_attribute("mode", mode)
            # OWNER turns (mem_scope=="owner") run on the warm persistent
            # session; open-mode (mem_scope=="") and the WARM_BRAIN=0 kill
            # switch fall to the cold per-turn subprocess. TRUSTED/UNKNOWN
            # never reach brain_respond (gate_and_respond routes them away).
            if (_WARM_BRAIN_ENABLED and mem_scope == "owner"
                    and os.environ.get("BRAIN_MODE", "claude") == "claude"):
                span.set_attribute("warm", True)
                reply = _get_warm_brain().ask(text)
            else:
                reply = _claude_brain(text, mem_scope=mem_scope)
            # Best-effort classification of brain error replies so the
            # Prometheus counter has useful reasons to slice by.
            low = (reply or "").lower()
            if "no brain credentials" in low:
                METRIC_BRAIN_ERRORS.labels(reason="no_credentials").inc()
            elif "took too long" in low:
                METRIC_BRAIN_ERRORS.labels(reason="timeout").inc()
            elif "lost my connection" in low:
                METRIC_BRAIN_ERRORS.labels(reason="nonzero_rc").inc()
            elif "blank" in low or "didn't have anything" in low:
                METRIC_BRAIN_ERRORS.labels(reason="empty_result").inc()
            elif "something went wrong" in low:
                METRIC_BRAIN_ERRORS.labels(reason="is_error").inc()
            elif low.startswith("brain error"):
                METRIC_BRAIN_ERRORS.labels(reason="exception").inc()
            return reply
        finally:
            # Capture reply_chars on the way out so both happy + sad paths
            # get recorded. reply variable is only bound in inner scopes;
            # use locals().get to stay defensive against future refactors.
            reply_local = locals().get("reply", "") or ""
            span.set_attribute("reply_chars", len(reply_local))
            METRIC_BRAIN_DURATION.observe(time.time() - t0)


def gate_and_respond(principal, text: str) -> str:
    """THE deterministic authorization gate. Capability is chosen HERE in
    Python from the principal's role, BEFORE any model call — the model is
    never the security boundary.

      principal is None  → open mode (no owner enrolled): full brain (legacy).
      OWNER              → full brain (all MCP tools), mem_scope=owner.
      TRUSTED            → owner-referential query (Layer B)? deterministic
                           deflection, NO brain spawned : locked brain
                           (no --mcp-config → no tools, Layer A).
      UNKNOWN            → no brain (mic loop drops these; Phase 2 adds
                           name-capture). Defensive challenge here.

    Used by BOTH the mic loop and the /voice/ingest endpoint so every front
    end inherits the identical guarantees."""
    import jarvis_identity as _ji
    if principal is None:
        return brain_respond(text)  # open mode — back-compat pass-through
    role = principal.role
    if role is _ji.Role.OWNER:
        return brain_respond(text, mem_scope=principal.mem_scope)
    if role is _ji.Role.TRUSTED:
        if _ji.is_owner_referential(text):
            print(f"  [gate] owner-referential query from {principal.user_id} "
                  f"— deflected pre-brain (Layer B, no brain spawned)")
            return ("Sorry, I can't share anything about him. "
                    "But I can help you with something directly.")
        return _claude_brain_voice_locked(text)
    # UNKNOWN — Phase 2 wires the name-capture state machine here.
    return "I don't recognise you. What's your name?"


# ── Phase 2: owner-auth + enroll-by-voice state machine ──────────────────────
# Deterministic, runs BEFORE the brain. Owner auth commands are CONSUMED here
# and never reach the model (the model must never be able to enroll a voice).
_AWAITING_NAME_TIMEOUT_S = 45.0
_SAME_SPEAKER_COS = 0.55
# Single in-flight challenge: the unknown speaker we just asked to name
# themselves. Holds their voiceprint so the follow-up "I'm Alex" is matched to
# the SAME voice (someone else can't answer for them).
_awaiting_name: dict = {"active": False, "embedding": None, "ts": 0.0}

_NAME_PATTERNS = [
    re.compile(r"\bmy name is\s+([A-Za-z][A-Za-z .'-]{0,30})", re.I),
    re.compile(r"\bi'?m\s+([A-Za-z][A-Za-z .'-]{0,30})", re.I),
    re.compile(r"\bit'?s\s+([A-Za-z][A-Za-z .'-]{0,30})", re.I),
    re.compile(r"\bthis is\s+([A-Za-z][A-Za-z .'-]{0,30})", re.I),
    re.compile(r"\bcall me\s+([A-Za-z][A-Za-z .'-]{0,30})", re.I),
]
_AUTH_REMOVE_RE = re.compile(
    r"\b(?:remove|delete|forget|unenroll|deauthori[sz]e)\s+([A-Za-z][A-Za-z .'-]{0,30})", re.I)
_AUTH_REJECT_RE = re.compile(
    r"\b(?:reject|deny|do\s*n'?t\s+(?:authenticate|authori[sz]e|trust))\b", re.I)
_AUTH_THIS_IS_RE = re.compile(r"\bthis is\s+([A-Za-z][A-Za-z .'-]{0,30})", re.I)
_AUTH_APPROVE_RE = re.compile(r"\b(?:authenticate|authori[sz]e|approve|trust)\b", re.I)


_NAME_STOPWORDS = {"please", "now", "thanks", "thank", "you", "okay", "ok", "jarvis"}


def _clean_name(s: str) -> str:
    s = (s or "").strip(" .,!?").split(",")[0].strip()
    words = s.split()
    # Drop trailing filler ("forget sarah please" → "Sarah").
    while words and words[-1].lower().strip(".,!?") in _NAME_STOPWORDS:
        words.pop()
    return " ".join(w.capitalize() for w in words[:2])


def _extract_name(text: str) -> str:
    for pat in _NAME_PATTERNS:
        m = pat.search(text)
        if m:
            return _clean_name(m.group(1))
    # Bare-name fallback: a short reply like "Alex" / "Alex Smith" (strip wake word).
    cleaned = re.sub(r"\bjarvis\b", "", text, flags=re.I).strip(" .,!?")
    words = cleaned.split()
    if 1 <= len(words) <= 2 and all(w[:1].isalpha() for w in words):
        return _clean_name(cleaned)
    return ""


def _cosine(a, b) -> float:
    a = np.asarray(a, dtype=np.float32)
    b = np.asarray(b, dtype=np.float32)
    return float(np.dot(a, b) / ((np.linalg.norm(a) + 1e-9) * (np.linalg.norm(b) + 1e-9)))


def _parse_owner_auth(text: str):
    """Owner-only enrollment commands. Returns (action, name) or None.
    action in {approve, reject, remove}."""
    m = _AUTH_REMOVE_RE.search(text)
    if m:
        return ("remove", _clean_name(m.group(1)))
    if _AUTH_REJECT_RE.search(text):
        return ("reject", "")
    m = _AUTH_THIS_IS_RE.search(text)
    if m:
        return ("approve", _clean_name(m.group(1)))
    if _AUTH_APPROVE_RE.search(text):
        return ("approve", "")
    return None


def _exec_owner_auth(action: str, name: str) -> str:
    import jarvis_identity as _ji
    owner_slug = _ji.get_owner_slug() or ""
    if action == "approve":
        res = _ji.authenticate_pending(owner_slug, override_name=name or "")
        if res.get("status") == "ok":
            return f"Done, sir. {res['name']} is now authorised."
        return "There's no one waiting to be authorised, sir."
    if action == "reject":
        _ji.clear_pending()
        return "Discarded, sir."
    if action == "remove":
        if name and _ji.remove(name):
            return f"Removed {name}, sir."
        return f"I don't have {name or 'them'} enrolled, sir."
    return "I'm not sure what you'd like me to do, sir."


def _identity_turn(principal, text: str, addressed: bool):
    """Phase 2 deterministic identity state machine. Returns
    ``(consumed, reply)``: when ``consumed`` is True the turn is an identity
    action (owner auth command, name challenge, or name capture) and must NOT
    go to the brain. ``reply`` (may be None) is spoken if present.

    Order: owner auth commands first (consumed), then name-capture for an
    awaiting challenger (matched by voiceprint so nobody can answer for them),
    then challenge an addressed unknown. Owner/trusted normal turns fall
    through to the gate."""
    import jarvis_identity as _ji
    role = principal.role
    now = time.time()

    # 1. Owner enrollment commands — consumed, never reach the brain. Gated on
    #    addressed so ambient owner chatter can't trigger an enrollment.
    if role is _ji.Role.OWNER:
        if addressed:
            cmd = _parse_owner_auth(text)
            if cmd is not None:
                return (True, _exec_owner_auth(cmd[0], cmd[1]))
        return (False, None)

    # 2. Name-capture: an unknown we just challenged states their name. Match
    #    the SAME voiceprint so a different person can't answer for them.
    if (_awaiting_name["active"]
            and now - _awaiting_name["ts"] < _AWAITING_NAME_TIMEOUT_S
            and principal.embedding is not None
            and _awaiting_name["embedding"] is not None
            and _cosine(principal.embedding, _awaiting_name["embedding"]) >= _SAME_SPEAKER_COS):
        name = _extract_name(text)
        if name:
            _ji.stash_pending(name, principal.embedding)
            _awaiting_name["active"] = False
            owner = _ji.get_owner()
            owner_name = (owner["name"] if owner else "Hampton")
            return (True, f"Thank you, {name}. {owner_name} will need to authorise you.")
        return (True, "I didn't catch your name — what is it?")

    # 3. Unknown + addressed → challenge and stash the voiceprint.
    if role is _ji.Role.UNKNOWN:
        if addressed:
            _awaiting_name.update(active=True, embedding=principal.embedding, ts=now)
            return (True, "I don't recognise you. What's your name?")
        return (False, None)  # unaddressed unknown → caller ambient-drops

    # 4. Trusted normal turn → gate.
    return (False, None)


def _check_brain_auth() -> None:
    """Verify the subscription OAuth credentials are present + non-expired.
    Loud success or loud failure — never silent. We DON'T do an actual
    API call here (no spend) — just inspect the credentials file. If
    expired, claude CLI will refresh on first use using the long-lived
    refreshToken (and write back to the file thanks to emptyDir mount)."""
    import time as _time
    creds_path = os.path.expanduser("~/.claude/.credentials.json")
    if not os.path.exists(creds_path):
        print(f"brain auth: NO creds at {creds_path} — brain will not work")
        print("brain auth: check the claude-creds-init initContainer ran "
              "and jarvis-secrets has claude_credentials.json")
        return
    try:
        with open(creds_path) as f:
            blob = json.load(f)
        oauth = blob.get("claudeAiOauth") or blob
        exp_ms = oauth.get("expiresAt") or 0
        exp_s = exp_ms / 1000 if exp_ms > 1e12 else exp_ms
        remaining_h = (exp_s - _time.time()) / 3600
        sub = oauth.get("subscriptionType", "?")
        if remaining_h > 0:
            print(f"brain auth: subscription OAuth OK  type={sub}  "
                  f"access_token expires in {remaining_h:.1f}h")
        else:
            print(f"brain auth: WARN access_token expired "
                  f"{-remaining_h:.1f}h ago — claude will refresh on first use")
        # Also check that ANTHROPIC_API_KEY is NOT set (it would override
        # subscription mode and burn metered credit).
        if os.environ.get("ANTHROPIC_API_KEY"):
            print("brain auth: WARN ANTHROPIC_API_KEY is set — claude will "
                  "use API mode not subscription. Unset the env var.")
    except Exception as exc:  # noqa: BLE001
        print(f"brain auth: FAILED to read creds: {exc}")


# ── Cluster TTS ──────────────────────────────────────────────────────────────
# Load the JARVIS persona voice once at startup so every /synthesize call
# can include it as audio_prompt_b64 — Chatterbox is zero-shot voice
# cloning, no training, just give it ~10-30s of reference and every
# generation comes out in that voice. Without it Chatterbox uses its
# default voice (not the persona).
import base64 as _base64
_VOICE_REF_PATH = os.environ.get("VOICE_REF", "/app/jarvis_voice.wav")
_VOICE_PROMPT_B64 = ""
try:
    with open(_VOICE_REF_PATH, "rb") as _f:
        _VOICE_PROMPT_B64 = _base64.b64encode(_f.read()).decode("ascii")
    print(f"voice ref loaded: {_VOICE_REF_PATH} "
          f"({len(_VOICE_PROMPT_B64) * 3 // 4} bytes)")
except Exception as _exc:
    print(f"voice ref unavailable ({_exc}) — Chatterbox will use default voice")


def tts_synthesize(text: str) -> bytes | None:
    """POST text to chatterbox, return WAV bytes. Includes the persona
    voice reference if loaded so output is cloned to that voice."""
    t0 = time.time()
    # exaggeration > 0.5 makes the prosody more dynamic (less monotone).
    # cfg_weight controls how strictly we follow the reference voice.
    # Live-tunable via mcp__jarvis_persona__persona_set/adjust — falls
    # back to the bundled defaults if the state file is missing.
    p = _load_persona()
    payload: dict = {
        "text": text,
        "exaggeration": float(p.get("tts_exaggeration",
                                    _PERSONA_DEFAULTS["tts_exaggeration"])),
        "cfg_weight": float(p.get("tts_cfg",
                                  _PERSONA_DEFAULTS["tts_cfg"])),
    }
    if _VOICE_PROMPT_B64:
        payload["audio_prompt_b64"] = _VOICE_PROMPT_B64
    body = json.dumps(payload).encode()
    req = urllib.request.Request(
        f"{TTS_URL}/synthesize",
        data=body,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with tracer.start_as_current_span("jarvis.tts.segment") as span:
        span.set_attribute("chars", len(text or ""))
        try:
            with urllib.request.urlopen(req, timeout=30.0) as r:
                wav = r.read()
                print(f"  tts: {len(wav)} bytes in {int((time.time()-t0)*1000)}ms")
                span.set_attribute("bytes", len(wav))
                METRIC_TTS_SEGMENT_DURATION.observe(time.time() - t0)
                return wav
        except Exception as exc:
            print(f"  tts error: {exc}")
            span.set_attribute("error", True)
            span.record_exception(exc)
            METRIC_TTS_SEGMENT_DURATION.observe(time.time() - t0)
            return None


# ── Embedded HTTP server (Sonos pulls audio from us) ─────────────────────────
# Streaming-TTS aware: serves multiple WAV chunks per turn (one per sentence).
# Paths like /turn-5-1.wav, /turn-5-2.wav ... are stashed individually so
# Sonos can fetch them in queue order while later chunks are still being
# synthesized.
class _AudioStash:
    """Multi-chunk WAV store keyed by URL path. Thread-safe."""
    def __init__(self) -> None:
        self.wavs: dict[str, bytes] = {}
        self.lock = threading.Lock()

    def put(self, path: str, data: bytes) -> None:
        with self.lock:
            self.wavs[path] = data

    def get(self, path: str) -> bytes | None:
        with self.lock:
            return self.wavs.get(path)

    def clear_older_than(self, current_turn_n: int, keep_last: int = 2) -> None:
        """Bound memory: drop chunks belonging to turns older than the
        last `keep_last`. Paths like /turn-<N>-<seg>.wav."""
        with self.lock:
            drop = []
            for k in self.wavs:
                try:
                    if k.startswith("/turn-"):
                        n = int(k.split("-")[1])
                        if n < current_turn_n - keep_last:
                            drop.append(k)
                except (ValueError, IndexError):
                    pass
            for k in drop:
                del self.wavs[k]


class _AudioHandler(BaseHTTPRequestHandler):
    stash: _AudioStash = None  # type: ignore[assignment]

    def do_GET(self):  # noqa: N802
        # Route IG verification handshake before the WAV-stash lookup.
        # Path-and-query split: BaseHTTPRequestHandler hands us the raw
        # request-target, which for GETs includes ?hub.mode=... etc.
        parsed = urllib.parse.urlsplit(self.path)
        if parsed.path == "/ig/webhook":
            self._handle_ig_verify(parsed.query)
            return
        wav = self.stash.get(self.path) if self.stash else None
        if not wav:
            self.send_response(404)
            self.end_headers()
            return
        try:
            self.send_response(200)
            self.send_header("Content-Type", "audio/wav")
            self.send_header("Content-Length", str(len(wav)))
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            self.wfile.write(wav)
        except (ConnectionResetError, BrokenPipeError):
            # Sonos sometimes opens a HEAD-probe connection it abandons
            # before the body, or queue advances + drops the current
            # fetch. Audio playback isn't affected — Sonos opens a fresh
            # connection for the real read. Suppress the noisy traceback.
            pass

    def do_POST(self):  # noqa: N802
        parsed = urllib.parse.urlsplit(self.path)
        if parsed.path == "/ig/webhook":
            self._handle_ig_event()
            return
        if parsed.path == "/voice/ingest":
            self._handle_voice_ingest()
            return
        self.send_response(404)
        self.end_headers()

    # ── IG: Meta verification handshake ──────────────────────────────
    def _handle_ig_verify(self, raw_query: str) -> None:
        with tracer.start_as_current_span("jarvis.ig.webhook_verify") as span:
            qs = urllib.parse.parse_qs(raw_query)
            mode = (qs.get("hub.mode") or [""])[0]
            token = (qs.get("hub.verify_token") or [""])[0]
            challenge = (qs.get("hub.challenge") or [""])[0]
            span.set_attribute("ig.hub_mode", mode)
            expected = os.environ.get("IG_VERIFY_TOKEN", "")
            ok = (mode == "subscribe"
                  and bool(expected)
                  and hmac.compare_digest(token, expected))
            span.set_attribute("ig.verify_ok", ok)
            if not ok:
                self.send_response(403)
                self.end_headers()
                return
            body = challenge.encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/plain; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            try:
                self.wfile.write(body)
            except (ConnectionResetError, BrokenPipeError):
                pass

    # ── IG: signed event ingress ─────────────────────────────────────
    def _handle_ig_event(self) -> None:
        global _ig_sig_logged_once
        with tracer.start_as_current_span("jarvis.ig.webhook_event") as span:
            try:
                # Read EXACTLY Content-Length bytes. Blind .read() hangs
                # forever on keepalive connections (Meta reuses them).
                try:
                    length = int(self.headers.get("Content-Length", "0") or "0")
                except ValueError:
                    length = 0
                body = self.rfile.read(length) if length > 0 else b""
                span.set_attribute("ig.body_bytes", len(body))

                # HMAC-SHA256 over the raw body, hex-encoded, prefixed.
                app_secret = os.environ.get("IG_APP_SECRET", "")
                if not app_secret:
                    span.set_attribute("ig.reject_reason", "no_app_secret")
                    METRIC_IG_SIG_FAILURES.inc()
                    self.send_response(403)
                    self.end_headers()
                    return
                expected = "sha256=" + hmac.new(
                    app_secret.encode(), body, hashlib.sha256,
                ).hexdigest()
                got = self.headers.get("X-Hub-Signature-256", "") or ""
                if not hmac.compare_digest(expected, got):
                    METRIC_IG_SIG_FAILURES.inc()
                    if not _ig_sig_logged_once:
                        print("ig: signature mismatch (logging once; further mismatches counted silently)")
                        _ig_sig_logged_once = True
                    span.set_attribute("ig.reject_reason", "bad_signature")
                    self.send_response(403)
                    self.end_headers()
                    return

                # Signature valid → parse + enqueue. Meta retries any
                # non-200, so on JSON parse failure we still answer 200
                # (there's nothing useful to retry).
                try:
                    payload = json.loads(body or b"{}")
                except json.JSONDecodeError as exc:
                    span.set_attribute("ig.reject_reason", "bad_json")
                    span.record_exception(exc)
                    METRIC_IG_EVENTS.labels(type="other", status="bad_json").inc()
                    self.send_response(200)
                    self.end_headers()
                    return

                entries = payload.get("entry") if isinstance(payload, dict) else None
                entry_count = len(entries) if isinstance(entries, list) else 0
                obj_type_raw = payload.get("object") if isinstance(payload, dict) else None
                # Bound metric cardinality — Meta sends a small known set
                # (instagram, page, messages, messaging_postbacks, etc).
                # Anything outside the allowlist gets folded into "other".
                _KNOWN = {"instagram", "page", "messages",
                          "messaging_postbacks", "comments", "mentions"}
                obj_type = obj_type_raw if obj_type_raw in _KNOWN else "other"
                span.set_attribute("ig.object_type", str(obj_type))
                span.set_attribute("ig.entry_count", entry_count)
                # Privacy: log entry count + type, NEVER payload content.
                print(f"ig event type={obj_type} entries={entry_count}")

                try:
                    _ig_event_queue.put_nowait(payload)
                    METRIC_IG_EVENTS.labels(type=str(obj_type), status="queued").inc()
                except queue.Full:
                    print("ig: event queue full — dropping event (consumer not draining)")
                    METRIC_IG_EVENTS.labels(type=str(obj_type), status="dropped_full").inc()

                self.send_response(200)
                self.end_headers()
            except Exception as exc:  # noqa: BLE001
                # Never leak stack traces to Meta — log + 200 so they
                # don't retry forever on a bug in our handler.
                span.record_exception(exc)
                print(f"ig: handler exception (returning 200 anyway): {exc!r}")
                METRIC_IG_EVENTS.labels(type="other", status="handler_error").inc()
                try:
                    self.send_response(200)
                    self.end_headers()
                except Exception:
                    pass

    # ── Voice mesh ingress (Mac thin client → shared gate) ───────────
    def _handle_voice_ingest(self) -> None:
        """POST /voice/ingest — THE mesh point. A thin STT client (the Mac
        `jarvis listen`) sends {text, embedding?, source}; we resolve it to a
        Principal and run the SAME deterministic gate_and_respond the mic loop
        uses, so every front end inherits identical guarantees with zero
        client-side security code. Auth: X-Edge-Token shared secret.

        The endpoint is INERT until EDGE_INGEST_TOKEN is set on the deployment
        (missing token → 403), so shipping this code is safe before the secret
        is wired."""
        import jarvis_identity as _ji
        with tracer.start_as_current_span("jarvis.voice.ingest") as span:
            # 1. Auth — shared secret, constant-time compare. Missing/!match → 403.
            expected = os.environ.get("EDGE_INGEST_TOKEN", "") or ""
            got = self.headers.get("X-Edge-Token", "") or ""
            if not expected or not hmac.compare_digest(got, expected):
                span.set_attribute("ingest.auth", "reject")
                self.send_response(403)
                self.end_headers()
                return
            # 2. Read + parse body.
            try:
                length = int(self.headers.get("Content-Length", "0") or "0")
            except ValueError:
                length = 0
            body = self.rfile.read(length) if length > 0 else b""
            try:
                payload = json.loads(body or b"{}")
            except json.JSONDecodeError:
                self._json(400, {"error": "bad_json"})
                return
            text = (payload.get("text") or "").strip()
            source = (payload.get("source") or "mac").strip() or "mac"
            emb = payload.get("embedding")
            if not text:
                self._json(400, {"error": "empty_text"})
                return

            # 3. Resolve → Principal (mirrors the mic loop's _identify logic).
            principal = None
            try:
                if _ji.has_owner():
                    if isinstance(emb, list) and emb:
                        principal = _ji.resolve_voice(
                            np.asarray(emb, dtype=np.float32), source=source)
                    else:
                        # Owner enrolled but no voiceprint sent → FAIL CLOSED.
                        principal = _ji.Principal(
                            role=_ji.Role.UNKNOWN, user_id=f"{source}:unknown",
                            source=source, confidence=0.0)
                # else: open mode (no owner) → principal stays None → full brain
            except Exception as exc:  # noqa: BLE001
                print(f"  [ingest] resolve failed — failing CLOSED: {exc!r}")
                principal = _ji.Principal(
                    role=_ji.Role.TRUSTED, user_id=f"{source}:unknown",
                    source=source, confidence=0.0)

            role = principal.role.value if principal is not None else "open"
            speaker = principal.display_name if principal is not None else ""
            span.set_attribute("ingest.role", role)
            span.set_attribute("ingest.source", source)
            # 4. SAME gate the mic loop uses — identical guarantees.
            reply = gate_and_respond(principal, text)
            self._json(200, {"reply": reply, "role": role, "speaker": speaker})

    def _json(self, code: int, obj: dict) -> None:
        body = json.dumps(obj).encode("utf-8")
        try:
            self.send_response(code)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        except (ConnectionResetError, BrokenPipeError):
            pass

    def log_message(self, *args, **kwargs):  # silence default access log
        pass


class _ReusableHTTPServer(ThreadingHTTPServer):
    # Threaded so a slow /voice/ingest brain call doesn't block Sonos WAV GETs
    # on the same server (single-threaded HTTPServer would serialize them).
    # daemon_threads defaults True on ThreadingHTTPServer. allow_reuse_address:
    # restart without the kernel's TIME_WAIT holding the port.
    allow_reuse_address = True


def _start_http_server(stash: _AudioStash, port: int) -> None:
    _AudioHandler.stash = stash
    srv = _ReusableHTTPServer(("0.0.0.0", port), _AudioHandler)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    print(f"audio http server: 0.0.0.0:{port}")


def _resolve_host_ip_for(remote_ip: str) -> str:
    """Find the local IP that the kernel routes to `remote_ip`. Avoids
    hard-coding 10.0.0.X — works whether Sonos is on LAN or Tailscale."""
    if EDGE_HOST_IP:
        return EDGE_HOST_IP
    if not remote_ip:
        return socket.gethostbyname(socket.gethostname())
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        s.connect((remote_ip, 1))
        return s.getsockname()[0]
    finally:
        s.close()


# ── Sonos ────────────────────────────────────────────────────────────────────
def _sonos():
    if not SONOS_IP:
        return None
    import soco
    return soco.SoCo(SONOS_IP)


# ── Desktop notification (toast) ─────────────────────────────────────────────
# Pops a notification on the active user's desktop via the standard
# notify-send / DBus path. Works from SSH if the user has an active
# graphical session and /run/user/<uid>/bus is reachable.
_DBUS_ADDR = os.environ.get(
    "DBUS_SESSION_BUS_ADDRESS",
    f"unix:path=/run/user/{os.getuid()}/bus",
)


def notify(title: str, body: str = "", urgency: str = "low",
           expire_ms: int = 4000) -> None:
    """Best-effort desktop toast. Silent on failure (e.g. no GUI session)."""
    import subprocess
    try:
        subprocess.Popen(
            ["notify-send",
             f"--urgency={urgency}",
             f"--expire-time={expire_ms}",
             "--icon=audio-input-microphone",
             title, body],
            env={**os.environ, "DBUS_SESSION_BUS_ADDRESS": _DBUS_ADDR},
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        )
    except Exception:
        pass


import re as _re_split


def _split_sentences(text: str, max_len: int = 40) -> list[str]:
    """Split brain output into sentence chunks for streaming synthesis.

    `max_len=40` keeps the FIRST chunk small (~10 words) so it
    synthesizes in ~1.5s and Sonos can start playing before later
    chunks are even ready. Bigger merge thresholds (we had 90) made
    the first chunk multi-sentence → 7s synth → 7s perceived lag.

    Fallback: if no sentence boundaries detected, returns [text] —
    streaming still works with a single chunk (same latency as before)."""
    if not text or not text.strip():
        return []
    parts = _re_split.split(r"(?<=[.!?])\s+", text.strip())
    out: list[str] = []
    for p in parts:
        p = p.strip()
        if not p:
            continue
        if out and len(out[-1]) + 1 + len(p) < max_len:
            out[-1] += " " + p
        else:
            out.append(p)
    return out or [text.strip()]


# Timestamp of when JARVIS most recently finished speaking on Sonos.
# Read by the main capture loop to drop self-echo — any utterance whose
# speech_start fell during JARVIS's playback or within ECHO_SUPPRESS_S
# afterward is the Yeti hearing JARVIS through the bedroom Play:1, not
# the user, and gets dropped before STT-to-brain.
_jarvis_done_at = 0.0
_jarvis_speaking = False


def _stream_on_sonos(sonos, sentences: list[str], host_ip: str,
                    http_port: int, turn_n: int,
                    stash: _AudioStash) -> dict:
    """Streaming TTS over Sonos queue. Synth + play sentence 1, then
    while it plays, synth + enqueue sentences 2..N. Sonos walks the
    queue. End-to-end wall clock = synth(s1) + play(all), not synth(all)
    + play(all) — cuts perceived lag for multi-sentence replies."""
    timings: dict = {"first_audio_ms": None, "stream_done_ms": None}
    sonos_span = tracer.start_as_current_span("jarvis.sonos.stream")
    span = sonos_span.__enter__()
    span.set_attribute("turn_n", turn_n)
    span.set_attribute("segment_count", len(sentences))
    try:
        if sonos is None:
            # No Sonos configured — save chunks to disk for inspection.
            span.set_attribute("sonos_configured", False)
            for i, sent in enumerate(sentences, 1):
                wav = tts_synthesize(sent)
                if wav:
                    with open(f"/tmp/jarvis_edge_turn_{turn_n}_{i}.wav", "wb") as f:
                        f.write(wav)
            return timings
        span.set_attribute("sonos_configured", True)
        return _stream_on_sonos_impl(sonos, sentences, host_ip, http_port,
                                     turn_n, stash, timings, span)
    finally:
        # Stash the timings dict on the span before exit so reviewers in
        # Tempo can see first_audio_ms / stream_done_ms on the parent
        # turn span as well as in the segment children.
        if timings.get("first_audio_ms") is not None:
            span.set_attribute("first_audio_ms", timings["first_audio_ms"])
            METRIC_SONOS_FIRST_AUDIO.observe(timings["first_audio_ms"] / 1000.0)
        if timings.get("stream_done_ms") is not None:
            span.set_attribute("stream_done_ms", timings["stream_done_ms"])
        sonos_span.__exit__(None, None, None)


def _stream_on_sonos_impl(sonos, sentences, host_ip, http_port, turn_n,
                          stash, timings, span) -> dict:
    t0 = time.time()
    # Mark JARVIS as speaking from the moment we start the stream — the
    # main loop's echo-suppress check uses _jarvis_done_at + grace, but
    # the speaking flag is a stricter "is JARVIS audible right now" signal.
    global _jarvis_speaking, _jarvis_done_at
    _jarvis_speaking = True

    # Snapshot whatever the Sonos was playing (Spotify, podcast, anything)
    # BEFORE we wipe the queue for the TTS turn. Restored at the end so
    # music resumes at the exact spot after JARVIS finishes speaking.
    # Snapshot failures are non-fatal — TTS still works, we just lose
    # the resume-music ability for this turn.
    snap = None
    pre_state = None
    try:
        from soco.snapshot import Snapshot  # local import — soco is hot
        snap = Snapshot(sonos, snapshot_queue=True)
        snap.snapshot()
        pre_state = snap.transport_state
        if pre_state == "PLAYING":
            print(f"  sonos: snapshot taken (was PLAYING, will resume after)")
    except Exception as exc:
        print(f"  sonos: snapshot failed (continuing without resume): {exc!r}")
        snap = None

    try:
        sonos.unjoin()
    except Exception:
        pass
    try:
        # Persona override (set via the persona MCP) wins over the
        # time-of-day schedule. When persona has no sonos_volume key,
        # fall back to the night/day band.
        persona_vol = _load_persona().get("sonos_volume")
        vol = int(persona_vol) if persona_vol is not None else _scheduled_sonos_volume()
        sonos.volume = vol
        print(f"  sonos vol set → {vol} ({'persona' if persona_vol is not None else 'schedule'})")
    except Exception as exc:
        print(f"  sonos vol set failed: {exc}")
    # Clear stale queue before we start enqueueing this turn.
    try:
        sonos.clear_queue()
    except Exception as exc:
        print(f"  queue clear failed (continuing): {exc}")

    played_first = False
    for i, sent in enumerate(sentences, 1):
        t_synth = time.time()
        wav = tts_synthesize(sent)
        if not wav:
            print(f"  ! synth failed for seg {i}: {sent[:50]!r}")
            continue
        path = f"/turn-{turn_n}-{i}.wav"
        stash.put(path, wav)
        url = f"http://{host_ip}:{http_port}{path}"
        synth_ms = int((time.time() - t_synth) * 1000)
        print(f"  tts seg {i}/{len(sentences)}: {len(wav)}b in {synth_ms}ms")
        try:
            sonos.add_uri_to_queue(url)
        except Exception as exc:
            print(f"  ! queue add failed: {exc}")
            continue
        if not played_first:
            try:
                sonos.play_from_queue(0)
                played_first = True
                timings["first_audio_ms"] = int((time.time() - t0) * 1000)
                print(f"  sonos: first audio at {timings['first_audio_ms']}ms")
            except Exception as exc:
                print(f"  ! play_from_queue failed: {exc}")

    if not played_first:
        return timings

    # Wait for queue to drain (Sonos walks through enqueued items).
    while True:
        time.sleep(0.4)
        try:
            state = sonos.get_current_transport_info().get(
                "current_transport_state", "")
        except Exception:
            break
        if state in ("STOPPED", "PAUSED_PLAYBACK"):
            break
        if time.time() - t0 > 90:
            print("  ! sonos stream timed out at 90s")
            break

    timings["stream_done_ms"] = int((time.time() - t0) * 1000)
    print(f"  sonos: stream done in {timings['stream_done_ms']}ms")
    # Mark the moment JARVIS stopped speaking so the main loop can drop
    # any self-echo the Yeti captured during/just-after playback.
    _jarvis_done_at = time.time()
    _jarvis_speaking = False
    # Bound memory: drop old turns' WAV chunks.
    stash.clear_older_than(turn_n)

    # Resume whatever was playing before JARVIS interrupted. Only fire
    # restore if there was actually something playing — restoring a
    # STOPPED state is wasted SOAP traffic. fade=True ramps volume back
    # so resumed music doesn't jolt back at JARVIS's higher voice level.
    if snap is not None and pre_state == "PLAYING":
        try:
            snap.restore(fade=True)
            print(f"  sonos: restored prior playback (was {pre_state})")
        except Exception as exc:
            print(f"  sonos: restore failed: {exc!r}")
    return timings


# Kept for backward-compat callers, but main loop uses _stream_on_sonos now.
def _play_on_sonos(sonos, host_ip: str, http_port: int, turn: int) -> None:
    """Legacy single-shot playback (one /turn-N.wav). Now wraps streaming
    with a single 'sentence' for callers that haven't migrated."""
    _stream_on_sonos(sonos,
                     [f"_turn_{turn}_singleshot"],  # placeholder
                     host_ip, http_port, turn,
                     _AudioHandler.stash)
    print(f"  sonos: played in {int((time.time()-t0)*1000)}ms")


# ── Main loop ────────────────────────────────────────────────────────────────
def main() -> None:
    mic_idx, native_rate, native_chunk = _pick_mic()
    print(f"stt: {STT_URL}")
    print(f"tts: {TTS_URL}")
    print(f"sonos: {SONOS_IP or 'NOT SET — set SONOS_IP env var'}")

    # No wake-word loader — ambient mode runs VAD-gated continuous STT.
    if BRAIN_MODE == "claude":
        _write_mcp_config()
        print(f"brain: claude (mcp config at {_MCP_CONFIG_PATH})")
        # Verify the API key is valid before we start listening — surface
        # auth problems at startup instead of as silent empty replies.
        _check_brain_auth()
        # Phase 5: prewarm the OWNER warm session so the MCP servers are warm
        # before the first turn. Only when an owner is enrolled (no point
        # warming an owner session in open mode). Fail-open like every other
        # boot subsystem — falls back to lazy spawn / cold per-turn on error.
        if _WARM_BRAIN_ENABLED and _vid_has_owner():
            try:
                _get_warm_brain()
                print("warm brain: OWNER session prewarmed")
            except Exception as exc:  # noqa: BLE001
                print(f"warm brain: prewarm failed ({exc}) — cold per-turn fallback")

    stash = _AudioStash()
    _start_http_server(stash, EDGE_HTTP_PORT)
    host_ip = _resolve_host_ip_for(SONOS_IP or "8.8.8.8")
    print(f"edge host ip (Sonos fetches from here): {host_ip}")

    sonos = None
    if SONOS_IP:
        try:
            sonos = _sonos()
            info = sonos.speaker_info
            print(f"sonos: {info.get('model_name')} '{sonos.player_name}' OK")
        except Exception as exc:
            print(f"sonos init failed ({exc}); will print responses instead")

    # Optional: IG DM polling fallback (instagrapi private-mobile-API).
    # Webhook path is gated behind Meta Business Verification; this
    # poller is the pragmatic workaround. FAIL-OPEN — import or start
    # failures don't take down the voice loop.
    if os.environ.get("IG_POLLING_ENABLED", "") == "1":
        try:
            import jarvis_ig_polling
            jarvis_ig_polling.start_polling_thread()
            print("ig polling: thread started")
        except Exception as exc:
            print(f"ig polling: failed to start — {exc}")

    # IG DM consumer: drains _ig_event_queue and replies via instagrapi.
    # Reuses the poller's logged-in Client (jarvis_ig_polling.get_client)
    # — DO NOT start this without the poller, the consumer will idle
    # until a client is available. FAIL-OPEN as above.
    if os.environ.get("IG_CONSUMER_ENABLED", "1") == "1":
        try:
            import jarvis_ig_consumer
            jarvis_ig_consumer.start_consumer_thread()
            print("ig consumer: thread started")
        except Exception as exc:
            print(f"ig consumer: failed to start — {exc}")

    # IG comment responder: poller (news_inbox_v1 every 60s) + consumer
    # (drains _ig_comment_queue → vision pipeline → gaslight one-liner →
    # client.media_comment). Shares the DM poller's logged-in Client +
    # followed-set cache. FAIL-OPEN — import or start failures don't
    # take down the voice loop or DM path.
    if os.environ.get("IG_COMMENT_ENABLED", "1") == "1":
        try:
            import jarvis_ig_comment_responder
            jarvis_ig_comment_responder.start_threads()
            print("ig comment: threads started")
        except Exception as exc:
            print(f"ig comment: failed to start — {exc}")

    # IG follow-up poller: catches replies to JARVIS's comment thread
    # that don't re-tag @hmlbjarvis (mention notifications would never
    # fire for those). Polls media JARVIS has commented on every ~60s
    # for new comments by followed users. Same downstream consumer
    # path as mentions — just an alternate trigger source.
    if os.environ.get("IG_FOLLOWUP_POLL_ENABLED", "1") == "1":
        try:
            import jarvis_ig_followup_poller
            jarvis_ig_followup_poller.start_thread()
        except Exception as exc:
            print(f"ig followup: failed to start — {exc}")

    # IG DM-shared media downloader: drains _ig_media_queue and saves
    # reels/photos/stories to the Synology-backed /media/reels PVC.
    # Borrows the poller's logged-in Client + the consumer's followed-set
    # cache; both must be running for this to do anything. FAIL-OPEN as
    # above.
    if os.environ.get("IG_MEDIA_DL_ENABLED", "1") == "1":
        try:
            import jarvis_ig_media_dl
            jarvis_ig_media_dl.start_downloader_thread()
            print("ig media: downloader thread started")
        except Exception as exc:
            print(f"ig media: failed to start — {exc}")

    # Discord selfbot (text v1: mentions, DMs, replies). Runs an asyncio
    # event loop in a daemon thread; reacts only on whitelisted guild
    # mentions / replies-to-us / DMs from the owner. Reuses the IG
    # comment responder's persona / Q&A / song-id pipelines. FAIL-OPEN
    # — an ImportError on discord.py-self (base image not rebuilt yet)
    # or a missing token must NOT take down the voice loop or IG paths.
    if os.environ.get("DISCORD_ENABLED", "1") == "1":
        try:
            import jarvis_discord
            jarvis_discord.start_thread()
        except Exception as exc:
            print(f"discord: failed to start — {exc}")

    # Cluster-run completion announcer (charter roadmap #2). Polls k8s for
    # finished runner Jobs and speaks the result on Sonos via _stream_on_sonos,
    # serialized against the mic loop by _speak_lock. FAIL-OPEN — import or
    # start failure must NOT take down the voice loop, exactly like the IG /
    # Discord subsystems above. No-op until launch_run has produced a Job.
    if os.environ.get("RUN_ANNOUNCE_ENABLED", "1") == "1":
        try:
            import jarvis_run_announce
            jarvis_run_announce.start_announce_thread(
                sonos=sonos, host_ip=host_ip,
                http_port=EDGE_ADVERTISED_PORT,
                stream_fn=_stream_on_sonos, stash=stash,
                speak_lock=_speak_lock,
            )
            print("run announce: thread started")
        except Exception as exc:
            print(f"run announce: failed to start — {exc}")

    try:
        import torch  # noqa: F401
        from silero_vad import load_silero_vad, VADIterator
        vad_model = load_silero_vad(onnx=True)
        has_vad = True
        print("endpointing: Silero VAD")
    except Exception as exc:
        print(f"VAD unavailable ({exc}); using RMS fallback")
        vad_model = None
        has_vad = False

    turn_n = 0
    # Addressee follow-up window: once JARVIS replies, the next utterance
    # within this many seconds doesn't need to re-say "jarvis".
    ADDRESSEE_WINDOW = ADDRESSEE_WINDOW_S
    engaged_until = 0.0
    # Set True for one turn after wake fires. The wake word itself
    # counts as the addressee signal for THIS turn regardless of how
    # long capture took (avoids losing turns where kitchen voices
    # padded the capture out past the engagement window).
    just_woke = False
    # AMBIENT MODE: no wake-word loop. We continuously VAD-gate the mic
    # and STT every speech segment. Drop transcripts that don't contain
    # "jarvis" (the addressee gate). Cost: ~290ms STT per speech segment;
    # GPU on nixos-gpu handles it easily. Privacy: ambient speech IS
    # transcribed but immediately discarded if not addressed.
    from collections import deque
    # Small pre-roll keeps the leading edge of each utterance (Silero
    # speech_start fires after the first word's already started).
    PREROLL_CHUNKS = max(1, int(0.4 / 0.080))  # 0.4s = 5 chunks
    preroll = deque(maxlen=PREROLL_CHUNKS)

    with sd.InputStream(samplerate=native_rate, channels=1, dtype="float32",
                        blocksize=native_chunk, device=mic_idx) as stream:
        print("\nready — ambient mode (say 'jarvis' anywhere in a sentence). Ctrl-C to quit.\n")
        try:
            while True:
                # ── Per-utterance VAD-gated capture ─────────────────
                if has_vad:
                    vad_iter = VADIterator(
                        vad_model, sampling_rate=16000,
                        min_silence_duration_ms=int(SILENCE_SECS * 1000),
                        speech_pad_ms=120,
                    )
                frames: list[np.ndarray] = []
                t_start = time.time()
                t_speech_start: float | None = None
                heard = False
                silence_start: float | None = None
                vad_buf = np.empty(0, dtype=np.float32)
                while True:
                    data, _ = stream.read(native_chunk)
                    mono = data.flatten().astype(np.float32)
                    if heard:
                        frames.append(mono)
                    else:
                        preroll.append(mono.copy())
                    if has_vad:
                        vad_buf = np.concatenate([vad_buf, _to_16k(mono, native_rate)])
                        while len(vad_buf) >= 512:
                            frame, vad_buf = vad_buf[:512], vad_buf[512:]
                            ev = vad_iter(frame, return_seconds=False)
                            if ev and "start" in ev and not heard:
                                # backfill pre-roll so the leading word is captured
                                frames = list(preroll)
                                heard = True
                                t_speech_start = time.time()
                                silence_start = None
                            elif ev and "end" in ev and heard:
                                silence_start = time.time()
                        if silence_start and (time.time() - silence_start) >= 0.05:
                            break
                    else:
                        rms = float(np.sqrt(np.mean(mono ** 2)))
                        if rms > 0.01:
                            if not heard:
                                frames = list(preroll)
                                t_speech_start = time.time()
                            heard = True
                            silence_start = None
                        elif heard:
                            silence_start = silence_start or time.time()
                            if time.time() - silence_start > SILENCE_SECS:
                                break
                    if heard and t_speech_start \
                            and (time.time() - t_speech_start) > MAX_UTTERANCE_SECS:
                        break

                if not heard:
                    continue  # no speech this slice — keep listening
                dur = sum(len(f) for f in frames) / native_rate
                if dur < MIN_UTTERANCE_SECS:
                    continue

                # ── Root turn span ──────────────────────────────────
                # Opened the moment we have an utterance with a known
                # t_speech_start. Closed at the end of the iteration —
                # either after streaming finishes, or at any drop
                # (echo / ambient / unknown speaker / STT-empty) via
                # the try/finally below. Drops are recorded as span
                # events so Tempo shows WHY a turn was dropped.
                turn_span_ctx = tracer.start_as_current_span("jarvis.turn")
                turn_span = turn_span_ctx.__enter__()
                turn_t0 = t_speech_start if t_speech_start is not None else time.time()
                turn_outcome = "addressed"  # default; overwritten on drop/empty
                try:
                    turn_span.set_attribute("utterance_dur_s", float(dur))
                    if t_speech_start is not None:
                        turn_span.set_attribute("speech_start_ts", float(t_speech_start))

                    # ── Echo suppression ────────────────────────────────
                    # If JARVIS was speaking when this utterance started, or
                    # finished < ECHO_SUPPRESS_S ago, this is almost certainly
                    # the Yeti picking up the Sonos playback — drop without
                    # transcribing. Prevents "Nineteen, sir. → 19, sir.
                    # → Nineteen what, sir?" self-conversations.
                    if t_speech_start is not None:
                        since_jarvis = t_speech_start - _jarvis_done_at
                        if _jarvis_speaking or (0 <= since_jarvis < ECHO_SUPPRESS_S):
                            print(f"  (echo drop {dur:.1f}s; "
                                  f"speaking={_jarvis_speaking}, "
                                  f"since_jarvis={since_jarvis:.1f}s)")
                            turn_span.add_event("echo_drop", {
                                "dur_s": float(dur),
                                "since_jarvis_s": float(since_jarvis),
                                "jarvis_speaking": bool(_jarvis_speaking),
                            })
                            turn_outcome = "echo_drop"
                            METRIC_ECHO_DROPS.inc()
                            METRIC_TURNS_TOTAL.labels(
                                addressed="0", ambient_drop="0", echo_drop="1",
                            ).inc()
                            continue

                    audio_native = np.concatenate(frames)
                    audio_16k = _to_16k(audio_native, native_rate)
                    res = transcribe(audio_16k)
                    if res.get("error"):
                        print(f"  ✗ STT error: {res['error']}")
                        turn_span.add_event("stt_error", {"error": str(res["error"])[:200]})
                        turn_outcome = "stt_error"
                        continue
                    if res.get("hallucination"):
                        turn_span.add_event("stt_hallucination")
                        turn_outcome = "stt_hallucination"
                        continue   # silently drop Whisper "thanks for watching" etc
                    user_text = res.get("text", "").strip()
                    if not user_text:
                        turn_outcome = "empty_text"
                        continue

                    # ── Speaker-ID gate (drops unknown voices in ambient mode) ──
                    # If owner is enrolled, require voice match before letting
                    # through. Without enrollment, pass-through (back-compat for
                    # fresh deployments).
                    # ── Speaker-ID + identity state machine (Phase 1/2) ──
                    import jarvis_identity as _ji
                    principal = _identify_speaker_from_audio(audio_16k)
                    low = user_text.lower()
                    addressed = ("jarvis" in low) or (time.time() < engaged_until)

                    # Phase 2: owner-auth + enroll-by-voice. Runs BEFORE the
                    # ambient drop so an unknown answering "I'm Alex" (no wake
                    # word) is still captured. Owner auth commands are consumed
                    # here and never reach the brain.
                    if principal is not None:
                        consumed, id_reply = _identity_turn(principal, user_text, addressed)
                        if consumed:
                            if id_reply:
                                print(f"  JARVIS (identity): {id_reply!r}")
                                turn_n += 1
                                _sents = _split_sentences(id_reply)
                                if _sents:
                                    try:
                                        with _speak_lock:
                                            _stream_on_sonos(sonos, _sents, host_ip,
                                                             EDGE_ADVERTISED_PORT, turn_n, stash)
                                        engaged_until = time.time() + ADDRESSEE_WINDOW
                                    except Exception as exc:
                                        print(f"  sonos stream error: {exc}")
                            turn_outcome = "identity_action"
                            continue
                        if principal.role is _ji.Role.UNKNOWN:
                            # Unaddressed / unhandled unknown → ambient-drop
                            # (don't broadcast that we're listening).
                            print(f"  (unknown speaker drop {dur:.1f}s): {user_text[:70]!r}  conf={principal.confidence:.2f}")
                            turn_span.add_event("unknown_speaker_drop", {
                                "dur_s": float(dur),
                                "confidence": float(principal.confidence),
                                "text_preview": user_text[:70],
                            })
                            turn_outcome = "unknown_speaker_drop"
                            METRIC_UNKNOWN_SPEAKER_DROPS.inc()
                            continue
                        print(f"  speaker: {principal.display_name or '?'} "
                              f"(role={principal.role.value}, conf={principal.confidence:.2f})")
                        turn_span.set_attribute("speaker", str(principal.display_name))
                        turn_span.set_attribute("speaker_role", principal.role.value)
                        turn_span.set_attribute("speaker_confidence", float(principal.confidence))

                    # ── Ambient addressee gate (owner/trusted + open mode) ──
                    # Must contain "jarvis" OR we're in the follow-up window.
                    if not addressed:
                        print(f"  (ambient drop {dur:.1f}s): {user_text[:70]!r}")
                        turn_span.add_event("ambient_drop", {
                            "dur_s": float(dur),
                            "text_preview": user_text[:70],
                        })
                        turn_outcome = "ambient_drop"
                        METRIC_TURNS_TOTAL.labels(
                            addressed="0", ambient_drop="1", echo_drop="0",
                        ).inc()
                        continue

                    print(f"\n[{time.strftime('%H:%M:%S')}] YOU: {user_text!r}  "
                          f"(dur={dur:.1f}s, stt {res.get('model_ms','?')}ms)")
                    notify("JARVIS", "Listening…", urgency="normal", expire_ms=1500)

                    # ── Gate + Brain (role-based, deterministic) ─────────
                    reply = gate_and_respond(principal, user_text)
                    print(f"  JARVIS: {reply!r}")
                    if not reply or not reply.strip():
                        print("  → empty reply, skipping TTS")
                        turn_outcome = "empty_reply"
                        continue

                    # ── Streaming TTS + Sonos queue playback ─────────────
                    # Split the brain reply into sentence-ish chunks. Synth
                    # sentence 1, hand to Sonos, then synth+enqueue 2..N
                    # while sentence 1 plays. First audio at ~1.5s instead
                    # of ~6-8s for multi-sentence replies.
                    turn_n += 1
                    turn_span.set_attribute("turn_n", turn_n)
                    sentences = _split_sentences(reply)
                    if not sentences:
                        print("  → no synthesizable text, skipping TTS")
                        turn_outcome = "no_synthesizable_text"
                        continue
                    turn_span.set_attribute("segment_count", len(sentences))
                    print(f"  streaming {len(sentences)} segment(s)")
                    try:
                        # Hold the shared speaking lock so a run-announce
                        # completion can't interleave mid-sentence with this
                        # live owner turn (charter roadmap #2).
                        with _speak_lock:
                            _stream_on_sonos(sonos, sentences, host_ip,
                                             EDGE_ADVERTISED_PORT, turn_n, stash)
                        engaged_until = time.time() + ADDRESSEE_WINDOW
                    except Exception as exc:
                        print(f"  sonos stream error: {exc}")
                        turn_span.record_exception(exc)
                        turn_outcome = "sonos_error"
                    else:
                        METRIC_TURNS_TOTAL.labels(
                            addressed="1", ambient_drop="0", echo_drop="0",
                        ).inc()
                finally:
                    # Single exit point for the turn span — records total
                    # wall-clock from speech_start through whatever path
                    # the turn took (success or drop).
                    total_s = time.time() - turn_t0
                    turn_span.set_attribute("outcome", turn_outcome)
                    turn_span.set_attribute("total_duration_s", float(total_s))
                    METRIC_TURN_TOTAL_DURATION.observe(total_s)
                    turn_span_ctx.__exit__(None, None, None)
        except KeyboardInterrupt:
            print("\nbye")


if __name__ == "__main__":
    main()
