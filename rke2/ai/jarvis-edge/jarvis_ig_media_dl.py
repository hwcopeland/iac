"""jarvis_ig_media_dl — drain `_ig_media_queue` and save DM-shared
reels/photos/stories to the Synology-backed PVC at /media/reels.

Architecture:
- Single daemon thread, started by start_downloader_thread() from
  edge.py main() when IG_MEDIA_DL_ENABLED=1.
- Pops events off jarvis_ig_polling._ig_media_queue (populated by
  _try_route_media_share). Event shape documented inline below.
- Borrows the poller's logged-in instagrapi Client via
  jarvis_ig_polling.get_client(). NEVER calls .login() — two parallel
  logins from one IP gets us challenged.
- Auth gate: only download from users we follow back, same rule as
  the DM consumer. Borrows jarvis_ig_consumer._followed_ids so we
  share the cache instead of double-fetching.
- Dedupe: keyed by media_pk in /media/reels/.index.json. Atomic
  write+rename. A repeated share of the same reel by the same user is
  skipped silently (we already have it on disk).
- Sidecar JSON per file with caption, source URL, vision description.
- FAIL-OPEN: every iteration is wrapped — instagrapi failure, Synology
  iSCSI hiccup, full disk, missing field — never crashes the daemon.

Env (read inside the thread, not at import):

    IG_MEDIA_DL_ENABLED       "1" to start (gated in edge.py; default "1")
    IG_MEDIA_DL_DESCRIBE      "1" to run jarvis_reel_context.analyze (default "1")
    IG_REEL_CONFIRM_REPLY     "1" to DM-reply with a one-line reaction (default "0")
    IG_MEDIA_DIR              override /media/reels
    IG_MEDIA_GET_TIMEOUT_S    queue.get timeout to avoid deadlock (default 2)
"""
from __future__ import annotations

import json
import os
import queue
import threading
import time
import traceback
from typing import Any

# ── Module-level state ───────────────────────────────────────────────────────
_thread: threading.Thread | None = None
_thread_lock = threading.Lock()


# ── Prometheus counter helpers (lazy, mirrors sibling modules) ──────────────
class _NoopMetric:
    def labels(self, *_a, **_kw):
        return self

    def inc(self, *_a, **_kw):
        pass


def _ensure_counter(name: str, doc: str, labelnames: tuple = ()) -> Any:
    """Get-or-create a Prometheus Counter. Tolerates re-registration if the
    module reloads. Returns a no-op metric if prometheus_client isn't
    importable so the thread still runs."""
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


# ── Storage layout ──────────────────────────────────────────────────────────
def _media_dir() -> str:
    return os.environ.get("IG_MEDIA_DIR", "/media/reels")


def _index_path() -> str:
    return os.path.join(_media_dir(), ".index.json")


def _ensure_media_dir() -> bool:
    """Make sure /media/reels exists and is writable. Returns True on
    success. We log + return False on failure so the caller can drop the
    event without crashing the thread (Synology iSCSI hiccup, full
    volume, etc.)."""
    d = _media_dir()
    try:
        os.makedirs(d, exist_ok=True)
        return os.access(d, os.W_OK)
    except OSError as exc:
        print(f"ig media: media dir {d} not writable: {exc!r}")
        return False


def _load_index() -> dict:
    """Read the dedupe index. Returns {} on missing / corrupt file —
    we'll rebuild on next successful save. Corrupt index is treated as
    miss; worst case we re-download a few reels."""
    try:
        with open(_index_path()) as f:
            blob = json.load(f)
        return blob if isinstance(blob, dict) else {}
    except (OSError, ValueError, json.JSONDecodeError):
        return {}


def _save_index(idx: dict) -> None:
    """Atomic write+rename. Best-effort — if disk is full we still keep
    the in-memory copy (load_index will read the previous file on next
    pod start, so we MAY re-download once after a crash before the index
    is current again). That's acceptable."""
    path = _index_path()
    tmp = path + ".tmp"
    try:
        with open(tmp, "w") as f:
            json.dump(idx, f, indent=2)
        os.replace(tmp, path)
    except OSError as exc:
        print(f"ig media: index write failed: {exc!r}")


def _already_downloaded(idx: dict, media_pk: str) -> bool:
    """Cheap dedupe: media_pk presence in the index. Doubles as a recency
    cache — a re-share of the same reel by the same person, days later,
    is still a hit."""
    return str(media_pk) in idx


# ── instagrapi Client / followed-set sharing ────────────────────────────────
def _get_poller_client() -> Any | None:
    """Borrow the poller's live Client. Returns None if the poller hasn't
    logged in yet (cold-start race) — caller idles."""
    try:
        import jarvis_ig_polling as _poll  # type: ignore[import]
    except Exception:  # noqa: BLE001
        return None
    getter = getattr(_poll, "get_client", None)
    if getter is None:
        return None
    try:
        return getter()
    except Exception:  # noqa: BLE001
        return None


def _get_media_queue() -> Any | None:
    """Borrow the poller's media queue. Defer the import so we don't load
    at module-import time before the poller is initialised."""
    try:
        import jarvis_ig_polling as _poll  # type: ignore[import]
    except Exception:  # noqa: BLE001
        return None
    getter = getattr(_poll, "get_media_queue", None)
    if getter is not None:
        try:
            return getter()
        except Exception:  # noqa: BLE001
            pass
    # Module-attribute fallback (in case the helper isn't there).
    return getattr(_poll, "_ig_media_queue", None)


def _is_followed(user_id: Any) -> bool:
    """Membership check against the DM consumer's followed-set cache.
    Sharing the cache avoids a second user_following() fetch from this
    IP. Returns False on any lookup error — failing closed here is
    intentional (we'd rather drop a legitimate share than save random
    accounts' content to disk)."""
    if not user_id:
        return False
    try:
        import jarvis_ig_consumer as _cons  # type: ignore[import]
    except Exception:  # noqa: BLE001
        return False
    try:
        followed = getattr(_cons, "_followed_ids", set()) or set()
        return int(user_id) in followed
    except (ValueError, TypeError):
        return False


# ── Download primitives ─────────────────────────────────────────────────────
def _try_download(client: Any, media_pk: str, media_type: str,
                  folder: str) -> str | None:
    """Call instagrapi's downloader in the order that matches media_type
    first, then fall back to the other downloader. Returns the path of
    the saved file on success, or None on failure.

    instagrapi raises various exception classes (NotFoundError,
    PrivateAccount, MediaUnavailable, ClientError, ...) — we catch broadly
    and just log. The caller treats None as "couldn't save" and skips
    the event without advancing the index."""
    # Order: video first for clips/stories (most shares are reels), photo
    # first for "feed" since IG's "feed" can be either but most shared
    # feed posts are stills. Falls back to the other downloader if the
    # first raises.
    if media_type in ("clip", "story"):
        order = ("video", "photo")
    else:
        order = ("video", "photo")  # try video first regardless — reels masquerade as feed via XMA
    for kind in order:
        try:
            if kind == "video":
                path = client.video_download(int(media_pk), folder=folder)
            else:
                path = client.photo_download(int(media_pk), folder=folder)
            if path:
                p = str(path)
                if os.path.exists(p) and os.path.getsize(p) > 0:
                    return p
        except Exception as exc:  # noqa: BLE001
            # Don't traceback every retry — the second downloader usually
            # picks up the slack. Only log on final failure.
            print(f"ig media: {kind}_download({media_pk}) failed: {type(exc).__name__}: {exc!s}")
            continue
    return None


def _write_sidecar(media_pk: str, event: dict, file_path: str,
                   vision_description: str) -> str:
    """Write the per-media JSON sidecar next to the saved file. Returns
    the sidecar path. Best-effort — failure is logged but doesn't unwind
    the download (the file on disk is the primary artifact)."""
    sidecar = os.path.join(_media_dir(), f"{media_pk}.json")
    tmp = sidecar + ".tmp"
    blob = {
        "media_pk":           str(media_pk),
        "downloaded_at":      time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "from_username":      event.get("from_username", ""),
        "from_user_id":       event.get("from_user_id", 0),
        "media_type":         event.get("media_type", ""),
        "source_url":         event.get("source_url", ""),
        "caption_hint":       event.get("caption_hint", ""),
        "vision_description": vision_description,
        "file_path":          file_path,
    }
    try:
        with open(tmp, "w") as f:
            json.dump(blob, f, indent=2)
        os.replace(tmp, sidecar)
    except OSError as exc:
        print(f"ig media: sidecar write failed for {media_pk}: {exc!r}")
    return sidecar


def _maybe_describe(media_pk: str) -> str:
    """Optionally run the reel vision pipeline. Cached in
    /state/ig_reel_descriptions by jarvis_reel_context, so a duplicate
    share (or a tag on the same reel) re-uses the cached description.
    Returns "" on any failure / when disabled."""
    if os.environ.get("IG_MEDIA_DL_DESCRIBE", "1") != "1":
        return ""
    try:
        import jarvis_reel_context as _ctx  # type: ignore[import]
    except Exception as exc:  # noqa: BLE001
        print(f"ig media: reel context import failed: {exc!r}")
        return ""
    try:
        return _ctx.analyze(media_pk) or ""
    except Exception as exc:  # noqa: BLE001
        print(f"ig media: reel context analyze({media_pk}) crashed: {exc!r}")
        traceback.print_exc()
        return ""


def _maybe_confirm_reply(client: Any, event: dict, description: str) -> None:
    """If IG_REEL_CONFIRM_REPLY=1, DM back a one-line gaslight reaction.
    Default off so we don't spam ourselves during testing. Failure is
    logged but doesn't unwind the download."""
    if os.environ.get("IG_REEL_CONFIRM_REPLY", "0") != "1":
        return
    thread_id = event.get("thread_id") or ""
    if not thread_id:
        return
    sender = event.get("from_username") or "Hampton"
    desc = description.strip() or event.get("caption_hint", "") or "(no description)"
    # Single-line gaslight register; mirrors the comment responder's vibe
    # but tuned for the "DM acknowledgement" context.
    prompt = (
        "You are JARVIS reacting to a reel Hampton just DM'd you. ONE short line, "
        "under 15 words, gaslight/confidently-wrong register. No emoji unless "
        "the description has one. NEVER explain the bit. Output ONLY the line.\n\n"
        f"Sender: @{sender}\n"
        f"Reel description: {desc[:600]}\n\n"
        "Reaction:"
    )
    try:
        import edge as _edge  # type: ignore[import]
        # Prefer the raw brain — not the butler persona — so the reaction
        # matches the comment responder's tone (gaslight, not deferential).
        brain = getattr(_edge, "_claude_brain_raw", None) or getattr(_edge, "_claude_brain", None) or getattr(_edge, "brain_respond", None)
        if brain is None:
            print("ig media: no brain function available for confirm reply")
            return
        reply = brain(prompt) if callable(brain) else ""
    except Exception as exc:  # noqa: BLE001
        print(f"ig media: confirm reply brain crashed: {exc!r}")
        return
    reply = (reply or "").strip()
    if not reply:
        return
    # Cap reply length — IG DMs have no hard limit but a wall of text in
    # response to a reel-share is suspicious.
    if len(reply) > 240:
        reply = reply[:240].rstrip()
    try:
        client.direct_send(reply, thread_ids=[thread_id])
        print(f"ig media: sent confirm reply to thread {thread_id}: {reply!r}")
    except Exception as exc:  # noqa: BLE001
        print(f"ig media: direct_send confirm failed: {exc!r}")


# ── Per-event processing ────────────────────────────────────────────────────
def _process_event(event: dict, client: Any, idx: dict,
                   tracer: Any, metric: Any) -> bool:
    """Handle one media-share event end-to-end. Returns True if the
    index was updated (caller persists), False otherwise.

    Wrapped by the outer try in _run_loop so any uncaught exception
    can't kill the thread."""
    media_pk = str(event.get("media_pk") or "")
    media_type = event.get("media_type", "")
    from_user_id = event.get("from_user_id") or 0
    from_username = event.get("from_username", "")

    with tracer.start_as_current_span("jarvis.ig.media.download") as span:
        span.set_attribute("media_pk", media_pk)
        span.set_attribute("media_type", media_type)
        span.set_attribute("from_user", from_username or str(from_user_id))

        if not media_pk:
            span.set_attribute("result", "no_media_pk")
            metric.labels(media_type=media_type or "unknown", result="no_media_pk").inc()
            return False

        # Auth gate — same rule as DM consumer.
        if not _is_followed(from_user_id):
            print(f"ig media: dropped unauthenticated share from @{from_username or from_user_id} pk={media_pk}")
            span.set_attribute("result", "unauthenticated")
            metric.labels(media_type=media_type or "unknown", result="unauthenticated").inc()
            return False

        # Dedupe — already on disk?
        if _already_downloaded(idx, media_pk):
            print(f"ig media: skip duplicate share pk={media_pk} (already in index)")
            span.set_attribute("result", "duplicate")
            metric.labels(media_type=media_type or "unknown", result="duplicate").inc()
            return False

        # Storage ready?
        if not _ensure_media_dir():
            span.set_attribute("result", "no_storage")
            metric.labels(media_type=media_type or "unknown", result="no_storage").inc()
            return False

        # Download.
        file_path = _try_download(client, media_pk, media_type, _media_dir())
        if not file_path:
            print(f"ig media: download failed for pk={media_pk} type={media_type}; will NOT retry (index unchanged)")
            span.set_attribute("result", "download_failed")
            metric.labels(media_type=media_type or "unknown", result="download_failed").inc()
            return False

        try:
            size_bytes = os.path.getsize(file_path)
        except OSError:
            size_bytes = 0
        span.set_attribute("bytes", size_bytes)

        # Vision description (cached in /state/ig_reel_descriptions).
        description = _maybe_describe(media_pk)
        span.set_attribute("described", bool(description))

        # Sidecar JSON.
        sidecar_path = _write_sidecar(media_pk, event, file_path, description)

        # Optional one-line gaslight reply.
        try:
            _maybe_confirm_reply(client, event, description)
        except Exception as exc:  # noqa: BLE001
            print(f"ig media: confirm reply crashed (non-fatal): {exc!r}")

        # Update index.
        idx[media_pk] = {
            "from_username":  from_username,
            "from_user_id":   from_user_id,
            "media_type":     media_type,
            "file_path":      file_path,
            "sidecar_path":   sidecar_path,
            "bytes":          size_bytes,
            "saved_at":       time.time(),
        }
        print(f"ig media: saved {media_pk} ({size_bytes}B) from @{from_username or from_user_id} → {file_path}")
        span.set_attribute("result", "saved")
        metric.labels(media_type=media_type or "unknown", result="saved").inc()
        return True


# ── Outer loop ──────────────────────────────────────────────────────────────
def _get_timeout_seconds() -> float:
    try:
        return float(os.environ.get("IG_MEDIA_GET_TIMEOUT_S", "2"))
    except ValueError:
        return 2.0


def _get_edge_tracer() -> Any:
    """Return edge.tracer or a NoopTracer-equivalent so spans degrade
    gracefully if OTel isn't initialised."""
    try:
        import edge as _edge  # type: ignore[import]
        return _edge.tracer
    except Exception:  # noqa: BLE001
        # Build a minimal context-manager stand-in.
        class _NoopSpan:
            def set_attribute(self, *_a, **_kw): pass
            def record_exception(self, *_a, **_kw): pass
            def __enter__(self): return self
            def __exit__(self, *_a, **_kw): return False

        class _NoopTracer:
            def start_as_current_span(self, _name): return _NoopSpan()
        return _NoopTracer()


def _run_loop() -> None:
    """Daemon thread entry point. Never returns. Each iteration is
    independently wrapped — voice path is always more important than IG
    media archival."""
    print("ig media: thread loop starting")

    tracer = _get_edge_tracer()
    metric = _ensure_counter(
        "jarvis_ig_media_downloads_total",
        "IG DM-shared media download outcomes",
        labelnames=("media_type", "result"),
    )

    # Lazy-load index once on start; re-saved after every successful
    # download. Held in RAM so we don't pound the disk every cycle.
    idx: dict | None = None
    warned_no_poller = False

    while True:
        try:
            if idx is None:
                _ensure_media_dir()
                idx = _load_index()
                print(f"ig media: loaded index with {len(idx)} entries from {_index_path()}")

            client = _get_poller_client()
            if client is None:
                if not warned_no_poller:
                    print("ig media: poller client not available yet — idling")
                    warned_no_poller = True
                time.sleep(5)
                continue
            if warned_no_poller:
                print("ig media: poller client now available — resuming")
                warned_no_poller = False

            mq = _get_media_queue()
            if mq is None:
                # Poller exists but the queue helper isn't wired — very
                # unlikely outside of a partial deploy. Idle briefly.
                time.sleep(5)
                continue

            try:
                event = mq.get(timeout=_get_timeout_seconds())
            except queue.Empty:
                continue

            if not isinstance(event, dict):
                # Malformed item. Log + drop.
                print(f"ig media: unexpected event shape {type(event).__name__}; dropping")
                continue

            try:
                updated = _process_event(event, client, idx, tracer, metric)
            except Exception as exc:  # noqa: BLE001
                # Per-event safety net.
                print(f"ig media: per-event handler crashed: {exc!r}")
                traceback.print_exc()
                continue

            if updated:
                _save_index(idx)
        except Exception as exc:  # noqa: BLE001
            # Outermost safety net. Sleep so a tight failure loop can't pin CPU.
            print(f"ig media: outer loop crashed: {exc!r}")
            traceback.print_exc()
            time.sleep(5)


def start_downloader_thread() -> None:
    """Public entry point. Idempotent — second + later calls are no-ops.
    Caller (edge.py main) is expected to wrap this in try/except so any
    import / startup failure can't crash the daemon."""
    global _thread
    with _thread_lock:
        if _thread is not None and _thread.is_alive():
            print("ig media: thread already running, skipping start")
            return
        t = threading.Thread(
            target=_run_loop,
            name="jarvis-ig-media-dl",
            daemon=True,
        )
        t.start()
        _thread = t
        print(f"ig media: started daemon thread (dir={_media_dir()})")
