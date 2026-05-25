"""jarvis_song_id — Shazam-based song recognition for IG reel audio.

Public API:

    import jarvis_song_id
    hit = jarvis_song_id.identify_from_wav("/tmp/3500000000.wav")
    if hit:
        print(hit["title"], "-", hit["artist"])
    else:
        print("unrecognised / instrumental / no audio")

Contract:
- Input: path to an audio file (WAV is what jarvis_reel_context produces,
  but shazamio accepts mp3/m4a/etc. too — it fingerprints raw audio).
- Output on hit: {"title", "artist", "album" (or ""), "apple_music_url"
  (or ""), "raw" (the full shazamio response)}.
- Output on miss / error / network blip: None.
- NEVER raises. Caller can treat None as "couldn't identify".

shazamio is async-only (built on aiohttp). We wrap one-shot recognition
in `asyncio.run` for sync callers — the reel-context pipeline is sync,
and the comment responder runs in its own thread, so each call gets a
fresh event loop and we don't entangle with any existing async runtime.

Caching:
- Results (hit OR miss) are cached to /state/ig_song_id/<sha256-prefix>.json
  keyed by the file content hash, so re-runs of the same reel audio are
  free (no Shazam round-trip). Misses are cached too — re-fingerprinting
  the same unrecognised audio gets you the same miss.

Observability:
- OTel span: jarvis.ig.song_id
- Prom counter: jarvis_ig_song_id_total{result} where result ∈
  {hit, miss, error, cache_hit, cache_miss}.

Env:
    SONG_ID_CACHE_DIR    override /state/ig_song_id
    SONG_ID_TIMEOUT_S    per-call timeout for the asyncio.run wrapper (default 25)
"""
from __future__ import annotations

import asyncio
import hashlib
import json
import os
import time
import traceback
from typing import Any


# ── Prometheus / OTel helpers (mirrors comment-responder pattern) ───────────
class _NoopMetric:
    def labels(self, *_a, **_kw):
        return self

    def inc(self, *_a, **_kw):
        pass


class _NoopSpan:
    def set_attribute(self, *_a, **_kw):
        pass

    def record_exception(self, *_a, **_kw):
        pass

    def __enter__(self):
        return self

    def __exit__(self, *_a):
        return False


class _NoopTracer:
    def start_as_current_span(self, _name):
        return _NoopSpan()


def _ensure_counter(name: str, doc: str, labelnames: tuple = ()) -> Any:
    """Get-or-create a Prometheus Counter. Tolerates re-registration and
    falls back to a no-op when prometheus_client isn't importable."""
    try:
        from prometheus_client import Counter, REGISTRY
    except Exception:  # noqa: BLE001
        return _NoopMetric()
    try:
        return Counter(name, doc, labelnames=labelnames)
    except ValueError:
        for collector in list(REGISTRY._collector_to_names.keys()):  # type: ignore[attr-defined]
            if getattr(collector, "_name", None) == name:
                return collector
        return _NoopMetric()


def _get_tracer() -> Any:
    """Lazy import of edge.tracer — edge.py is fully initialised by the
    time identify_from_wav() runs (called from a consumer thread)."""
    try:
        import edge as _edge  # type: ignore[import]
        return getattr(_edge, "tracer", None) or _NoopTracer()
    except Exception:  # noqa: BLE001
        return _NoopTracer()


# Lazy-init metric singleton (created on first call so the module import
# is side-effect-free if prometheus_client isn't around).
_METRIC: Any = None


def _metric() -> Any:
    global _METRIC
    if _METRIC is None:
        _METRIC = _ensure_counter(
            "jarvis_ig_song_id_total",
            "Shazamio song-identification outcomes",
            labelnames=("result",),
        )
    return _METRIC


# ── Cache ──────────────────────────────────────────────────────────────────
def _cache_dir() -> str:
    return os.environ.get("SONG_ID_CACHE_DIR", "/state/ig_song_id")


def _file_hash(path: str) -> str:
    """sha256 hex prefix (16 chars) of the file contents. Sufficient
    collision resistance for our scale — IG reels in our cache are tens
    to low hundreds, not millions."""
    h = hashlib.sha256()
    with open(path, "rb") as f:
        while True:
            chunk = f.read(1024 * 1024)
            if not chunk:
                break
            h.update(chunk)
    return h.hexdigest()[:16]


def _cache_path(file_hash: str) -> str:
    return os.path.join(_cache_dir(), f"{file_hash}.json")


def _load_cached(file_hash: str) -> dict | None:
    """Return cached entry. The entry shape is:
        {"hit": bool, "result": dict|None, "cached_at": float}
    Returns None on miss / corrupt cache."""
    path = _cache_path(file_hash)
    try:
        with open(path) as f:
            blob = json.load(f)
        if not isinstance(blob, dict):
            return None
        return blob
    except (OSError, ValueError, json.JSONDecodeError):
        return None


def _save_cached(file_hash: str, result: dict | None) -> None:
    """Persist hit OR miss atomically. Best-effort — cache failure
    doesn't change the returned value."""
    path = _cache_path(file_hash)
    tmp = path + ".tmp"
    try:
        os.makedirs(os.path.dirname(path), exist_ok=True)
    except OSError:
        pass
    try:
        with open(tmp, "w") as f:
            json.dump({
                "hit": result is not None,
                "result": result,
                "cached_at": time.time(),
            }, f)
        os.replace(tmp, path)
    except OSError as exc:
        print(f"song_id: cache write failed for {file_hash}: {exc!r}")


# ── shazamio call (async-wrapped) ──────────────────────────────────────────
async def _shazam_recognize(wav_path: str) -> dict | None:
    """Run shazamio against the file. Returns the parsed result dict on
    hit, None on miss. Raises on import / network errors — caller
    converts to None."""
    from shazamio import Shazam  # type: ignore[import]

    s = Shazam()
    out = await s.recognize(wav_path)
    # shazamio response shape on hit: {"matches": [...], "track": {...}}.
    # On miss: {"matches": []} (no "track" key, or "track" is falsy).
    if not isinstance(out, dict):
        return None
    track = out.get("track")
    if not track or not isinstance(track, dict):
        return None
    title = (track.get("title") or "").strip()
    artist = (track.get("subtitle") or "").strip()  # 'subtitle' is the artist field in Shazam payload
    if not title or not artist:
        return None

    # Album: nested under sections[0].metadata[*] where text key == "Album".
    album = ""
    try:
        sections = track.get("sections") or []
        for sec in sections:
            if not isinstance(sec, dict):
                continue
            for md in sec.get("metadata") or []:
                if not isinstance(md, dict):
                    continue
                if (md.get("title") or "").strip().lower() == "album":
                    album = (md.get("text") or "").strip()
                    if album:
                        break
            if album:
                break
    except Exception:  # noqa: BLE001
        album = ""

    # Apple Music URL: track.hub.actions[*].uri where uri starts with
    # 'https://music.apple.com'. Falls back to track.url if present.
    apple_music_url = ""
    try:
        hub = track.get("hub") or {}
        for action in hub.get("actions") or []:
            if not isinstance(action, dict):
                continue
            uri = (action.get("uri") or "").strip()
            if uri.startswith("https://music.apple.com"):
                apple_music_url = uri
                break
        if not apple_music_url:
            apple_music_url = (track.get("url") or "").strip()
    except Exception:  # noqa: BLE001
        apple_music_url = ""

    return {
        "title": title,
        "artist": artist,
        "album": album,
        "apple_music_url": apple_music_url,
        "raw": track,  # keep the full track for debugging / future enrichment
    }


def _run_async(coro, timeout_s: float):
    """Run an awaitable to completion on a fresh event loop with a
    timeout. Each call gets its own loop so we don't entangle with any
    existing async runtime in the caller (the comment responder runs in
    its own thread; the reel context runs sync from the consumer
    thread)."""
    loop = asyncio.new_event_loop()
    try:
        return loop.run_until_complete(asyncio.wait_for(coro, timeout=timeout_s))
    finally:
        try:
            loop.close()
        except Exception:  # noqa: BLE001
            pass


# ── Public entry point ─────────────────────────────────────────────────────
def identify_from_wav(wav_path: str) -> dict | None:
    """Best-effort Shazam recognition. NEVER raises.

    Returns:
        {"title", "artist", "album", "apple_music_url", "raw"} on hit
        None on miss / error / file-missing / shazamio import failure
    """
    tracer = _get_tracer()
    metric = _metric()

    with tracer.start_as_current_span("jarvis.ig.song_id") as span:
        # File sanity check first — saves an asyncio setup cost on a
        # path that's obviously going to fail.
        if not wav_path or not isinstance(wav_path, str):
            span.set_attribute("song_id.result", "error")
            span.set_attribute("song_id.error", "bad_path_arg")
            metric.labels(result="error").inc()
            return None
        if not os.path.exists(wav_path):
            print(f"song_id: file not found: {wav_path}")
            span.set_attribute("song_id.result", "error")
            span.set_attribute("song_id.error", "file_not_found")
            metric.labels(result="error").inc()
            return None

        try:
            size = os.path.getsize(wav_path)
        except OSError:
            size = 0
        span.set_attribute("song_id.audio_bytes", size)
        if size < 1024:
            # < 1KB of audio — nothing to fingerprint.
            print(f"song_id: audio too small ({size}B): {wav_path}")
            span.set_attribute("song_id.result", "error")
            span.set_attribute("song_id.error", "audio_too_small")
            metric.labels(result="error").inc()
            return None

        # Cache lookup. Both hits and misses are cached.
        try:
            fh = _file_hash(wav_path)
        except OSError as exc:
            print(f"song_id: hash failed for {wav_path}: {exc!r}")
            span.set_attribute("song_id.result", "error")
            span.set_attribute("song_id.error", "hash_failed")
            metric.labels(result="error").inc()
            return None
        span.set_attribute("song_id.file_hash", fh)

        cached = _load_cached(fh)
        if cached is not None:
            metric.labels(result="cache_hit").inc()
            span.set_attribute("song_id.cache", "hit")
            if cached.get("hit"):
                metric.labels(result="hit").inc()
                span.set_attribute("song_id.result", "hit")
                return cached.get("result")
            else:
                metric.labels(result="miss").inc()
                span.set_attribute("song_id.result", "miss")
                return None
        metric.labels(result="cache_miss").inc()
        span.set_attribute("song_id.cache", "miss")

        # Live recognition.
        try:
            timeout_s = float(os.environ.get("SONG_ID_TIMEOUT_S", "25"))
        except ValueError:
            timeout_s = 25.0

        t0 = time.time()
        try:
            result = _run_async(_shazam_recognize(wav_path), timeout_s=timeout_s)
        except asyncio.TimeoutError:
            print(f"song_id: shazam recognition timed out after {timeout_s:.1f}s")
            span.set_attribute("song_id.result", "error")
            span.set_attribute("song_id.error", "timeout")
            metric.labels(result="error").inc()
            return None
        except ImportError as exc:
            print(f"song_id: shazamio not installed: {exc!r}")
            span.set_attribute("song_id.result", "error")
            span.set_attribute("song_id.error", "import_error")
            metric.labels(result="error").inc()
            return None
        except Exception as exc:  # noqa: BLE001
            print(f"song_id: shazamio crashed for {wav_path}: {exc!r}")
            traceback.print_exc()
            span.set_attribute("song_id.result", "error")
            span.set_attribute("song_id.error", "exception")
            span.record_exception(exc)
            metric.labels(result="error").inc()
            return None

        elapsed = time.time() - t0
        span.set_attribute("song_id.elapsed_s", round(elapsed, 3))

        # Persist hit OR miss to the cache.
        _save_cached(fh, result)

        if result is None:
            metric.labels(result="miss").inc()
            span.set_attribute("song_id.result", "miss")
            print(f"song_id: no match for {wav_path} (elapsed={elapsed:.2f}s)")
            return None

        metric.labels(result="hit").inc()
        span.set_attribute("song_id.result", "hit")
        span.set_attribute("song_id.title", result["title"])
        span.set_attribute("song_id.artist", result["artist"])
        print(f"song_id: hit '{result['title']}' by {result['artist']} "
              f"({elapsed:.2f}s) for {wav_path}")
        return result
