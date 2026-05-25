"""jarvis_ig_consumer — drain `_ig_event_queue` and reply via instagrapi.

Counterpart to jarvis_ig_polling.py. The poller fills the queue with
inbound DMs; this module drains it, runs each text through the brain,
and ships the reply back into the same IG thread. Voice ambient loop
in shape: text in → brain → text out, no TTS.

Architecture rules:
- We DO NOT log in to instagrapi here. We borrow the cached `Client`
  the poller built (jarvis_ig_polling.get_client()). Two logins from
  one IP would speed-run a challenge. If the poller isn't running yet
  we sit idle and warn.
- Authentication: only reply to users we follow back. Anything else is
  silently dropped — we don't want to leak that we're a bot to randoms.
- Follower set is cached in /state/ig_followed.json so a restart doesn't
  cause a brief window of dropped legitimate DMs while we re-fetch.
- Brain prompt gets an IG-DM persona prefix that overrides the voice
  rules (1-3 sentences, no "sir" tag, casual register).
- FAIL-SAFE: every iteration is wrapped — instagrapi misbehaviour, a
  brain timeout, anything — never crashes the daemon. Voice path is
  always more important than IG.

Env (read inside the thread, not at import):

    IG_USERNAME              who we are (skip self-loop messages)
    IG_CONSUMER_ENABLED      "1" to start, anything else = no-op (gated in edge.py)
    IG_FOLLOWED_PATH         follower cache (default /state/ig_followed.json)
    IG_FOLLOWED_REFRESH_S    cadence for refreshing the followed set (default 300)
    IG_CONSUMER_GET_TIMEOUT_S  queue.get timeout to avoid deadlock (default 2)
    IG_CONSUMER_HISTORY_AMOUNT  prior messages pulled per reply (default 6)
    IG_CONSUMER_REPLY_MAX_CHARS  truncate runaway brain replies (default 900)
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

# Set of int IG user IDs we follow. Module-level cache; refreshed on a
# cadence inside the consumer loop. Persisted to disk so a restart
# doesn't drop legitimate DMs while we re-fetch.
_followed_ids: set[int] = set()
_followed_last_refresh: float = 0.0


# ── Persona prefix for IG DM context ────────────────────────────────────────
# IG DM replies are not read aloud — strip the TTS-tailored voice rules
# and replace with chat-tailored ones. Hampton can tune this; keep
# changes here and only here.
IG_DM_PERSONA_PREFIX = (
    "You are JARVIS replying to an authenticated Instagram DM (text chat, "
    "not voice). Reply concisely as in an Instagram DM: 1-3 sentences, no "
    "'sir' tag, casual register, no emojis unless the prior turn used them. "
    "Do not output the literal prefix 'JARVIS:' — the message body is your "
    "reply. The conversation history (most recent last) is provided after "
    "this preamble.\n\n"
)


# ── Prometheus counters (lazy, mirrors jarvis_ig_polling pattern) ──────────
class _NoopMetric:
    def labels(self, *_a, **_kw):
        return self

    def inc(self, *_a, **_kw):
        pass


def _ensure_counter(name: str, doc: str, labelnames: tuple = ()) -> Any:
    """Get-or-create a Prometheus Counter. Tolerates re-registration if
    the module is reloaded. Returns a no-op metric if prometheus_client
    isn't importable."""
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


# ── Edge handles (queue + tracer, lazy-imported) ────────────────────────────
def _get_edge_handles() -> dict[str, Any]:
    """Return {queue, tracer, metric_*} from edge.py. Deferred to thread
    start because edge.py imports us from inside main(), so at our
    import time edge.py is still partially initialised."""
    import edge as _edge  # type: ignore[import]

    return {
        "queue": _edge._ig_event_queue,
        "tracer": _edge.tracer,
        "metric_replies": _ensure_counter(
            "jarvis_ig_consumer_replies_total",
            "IG DM replies sent",
            labelnames=("authenticated",),
        ),
        "metric_drops": _ensure_counter(
            "jarvis_ig_consumer_drops_total",
            "IG DM events dropped by reason",
            labelnames=("reason",),
        ),
    }


def _get_poller_client() -> Any | None:
    """Return the live instagrapi Client built by jarvis_ig_polling, or
    None if the poller hasn't logged in yet. We DO NOT log in ourselves
    — two parallel logins from one IP gets us challenged."""
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


# ── Follower cache ──────────────────────────────────────────────────────────
def _followed_path() -> str:
    return os.environ.get("IG_FOLLOWED_PATH", "/state/ig_followed.json")


def _load_followed_from_disk() -> set[int]:
    """Read the persisted followed-set so a restart doesn't briefly drop
    legitimate DMs while we re-fetch. Returns empty set on missing /
    corrupt file — the consumer will rebuild from instagrapi on next
    refresh."""
    path = _followed_path()
    try:
        with open(path) as f:
            blob = json.load(f)
        ids = blob.get("followed_ids", [])
        return {int(x) for x in ids}
    except (OSError, ValueError, TypeError, json.JSONDecodeError):
        return set()


def _save_followed_to_disk(ids: set[int]) -> None:
    """Atomic write+rename so a crash mid-write can't truncate the file."""
    path = _followed_path()
    tmp = path + ".tmp"
    try:
        os.makedirs(os.path.dirname(path), exist_ok=True)
    except OSError:
        pass
    try:
        with open(tmp, "w") as f:
            json.dump({"followed_ids": sorted(ids), "saved_at": time.time()}, f)
        os.replace(tmp, path)
    except OSError as exc:
        print(f"ig consumer: failed to persist followed cache: {exc!r}")


def _refresh_followed(client: Any) -> bool:
    """Fetch followers from instagrapi and update the module-level cache.

    Returns True on success, False on any error (rate limit, network,
    auth). Caller decides whether to fall back to the cached file.

    `client.user_following(client.user_id, amount=0)` returns a dict
    `{user_id_str: UserShort}` of every account WE follow. amount=0 means
    no limit (return all)."""
    global _followed_ids, _followed_last_refresh
    if client is None:
        return False
    try:
        own_id = getattr(client, "user_id", None)
        if own_id is None:
            return False
        following = client.user_following(own_id, amount=0)
        # instagrapi returns a dict keyed by str user_id. Normalise to int.
        if isinstance(following, dict):
            ids = {int(k) for k in following.keys()}
        else:
            # Defensive: some instagrapi versions return list[UserShort].
            ids = {int(getattr(u, "pk", 0)) for u in following if getattr(u, "pk", None)}
            ids.discard(0)
        _followed_ids = ids
        _followed_last_refresh = time.time()
        _save_followed_to_disk(ids)
        print(f"ig consumer: followed set refreshed ({len(ids)} accounts)")
        return True
    except Exception as exc:  # noqa: BLE001
        print(f"ig consumer: refresh followed failed ({exc!r}); using cached file")
        traceback.print_exc()
        return False


def _is_followed(user_id: Any) -> bool:
    """Best-effort membership check. user_id may arrive as str (poller's
    _format_event stringifies it) or int. Empty / unparseable id → False."""
    if not user_id:
        return False
    try:
        return int(user_id) in _followed_ids
    except (ValueError, TypeError):
        return False


# ── Event extraction ────────────────────────────────────────────────────────
def _extract_event(payload: Any) -> dict | None:
    """The queue carries two shapes:

    - Polling: a single event dict already in our normalised format
      (_format_event in jarvis_ig_polling.py). `source: polling`.
    - Webhook: the raw Meta payload — `payload.entry[*].messaging[*]`
      with a different shape. Source: webhook.

    For now we only handle the polling shape — webhook consumption can be
    added later when Meta Business Verification clears. Webhook events
    are dropped with a debug log.

    Returns the event dict, or None if we can't / shouldn't process it."""
    if not isinstance(payload, dict):
        return None
    if payload.get("source") == "polling" and payload.get("type") == "messages":
        return payload
    # Webhook payload — not handled yet. Log + drop. Don't spam if Meta
    # is sending verification probes.
    if "entry" in payload:
        return None
    return None


def _format_history(thread_id: str, client: Any, amount: int) -> str:
    """Pull last `amount` messages from the same thread and render them
    as alternating Them:/JARVIS: lines (most recent last). Returns "" on
    any error — the brain still gets the user text via the primary
    prompt, just without context."""
    if not thread_id or client is None:
        return ""
    try:
        msgs = client.direct_messages(thread_id, amount=amount)
    except Exception as exc:  # noqa: BLE001
        print(f"ig consumer: direct_messages({thread_id}) failed ({exc!r}); no history")
        return ""
    if not msgs:
        return ""
    own_id = getattr(client, "user_id", None)
    # instagrapi returns newest-first; reverse so oldest is at the top
    # of the rendered block.
    lines: list[str] = []
    for m in reversed(msgs):
        text = (getattr(m, "text", "") or "").strip()
        if not text:
            continue
        sender = getattr(m, "user_id", None)
        is_us = (sender is not None and own_id is not None
                 and str(sender) == str(own_id))
        prefix = "JARVIS" if is_us else "Them"
        lines.append(f"{prefix}: {text}")
    return "\n".join(lines)


def _build_brain_prompt(event: dict, history_block: str) -> str:
    """Combine the IG-DM persona prefix, the prior conversation context,
    and the current inbound message into one prompt for the brain."""
    sender = (event.get("from") or {}).get("username", "") or "unknown"
    user_text = event.get("text", "") or ""
    parts = [IG_DM_PERSONA_PREFIX.rstrip()]
    if history_block:
        parts.append("Recent conversation:")
        parts.append(history_block)
        parts.append("")  # blank line before the new turn
    parts.append(f"Them ({sender}): {user_text}")
    parts.append("JARVIS:")  # cue the model that it's our turn
    return "\n".join(parts)


def _clean_reply(reply: str) -> str:
    """Strip any leading 'JARVIS:' the brain might echo back from the
    cue line, and bound to a sensible DM length. Empty after cleanup →
    empty string (caller short-circuits)."""
    if not reply:
        return ""
    out = reply.strip()
    # Strip a leading 'JARVIS:' (with optional whitespace / trailing space).
    low = out.lower()
    if low.startswith("jarvis:"):
        out = out[len("jarvis:"):].lstrip()
    # Some shortcut paths prepend "Of course, sir." style intros — leave
    # those alone, the persona prefix already steered the model away.
    try:
        cap = int(os.environ.get("IG_CONSUMER_REPLY_MAX_CHARS", "900"))
    except ValueError:
        cap = 900
    if cap > 0 and len(out) > cap:
        out = out[:cap].rstrip() + " …"
    return out


# ── Per-event processing ────────────────────────────────────────────────────
def _process_event(payload: Any, client: Any, handles: dict) -> None:
    """Handle one event end-to-end. Wrapped by the outer try in _run_loop
    so any uncaught exception lands in `error` instead of killing the
    thread."""
    tracer = handles["tracer"]
    metric_replies = handles["metric_replies"]
    metric_drops = handles["metric_drops"]

    with tracer.start_as_current_span("jarvis.ig.consumer.process") as span:
        event = _extract_event(payload)
        if event is None:
            span.set_attribute("ig.dropped", True)
            span.set_attribute("ig.drop_reason", "unsupported_payload")
            metric_drops.labels(reason="unsupported_payload").inc()
            return

        sender = event.get("from") or {}
        sender_username = (sender.get("username") or "").strip()
        sender_id = sender.get("id")
        text = (event.get("text") or "").strip()
        thread_id = event.get("thread_id") or ""

        span.set_attribute("from_user", sender_username or "unknown")

        # Self-loop guard: if the message is from us (the poller picked
        # up an echo of our own outbound reply), drop.
        own_username = (os.environ.get("IG_USERNAME") or "").strip().lower()
        if own_username and sender_username.lower() == own_username:
            print(f"ig consumer: skip self-loop from {sender_username}")
            span.set_attribute("ig.dropped", True)
            span.set_attribute("ig.drop_reason", "self")
            metric_drops.labels(reason="self").inc()
            return

        # No-text guard. We only handle text DMs for now; voice memos,
        # photos, reels, etc. all come through with empty .text.
        if not text:
            print(f"ig consumer: skip non-text from {sender_username or 'unknown'}")
            span.set_attribute("ig.dropped", True)
            span.set_attribute("ig.drop_reason", "no_text")
            metric_drops.labels(reason="no_text").inc()
            return

        # Authentication: only reply if we follow this user back. Silent
        # drop for everyone else (we don't want to leak that we're a bot).
        authenticated = _is_followed(sender_id)
        span.set_attribute("authenticated", authenticated)
        if not authenticated:
            print(f"ig consumer: dropped unauthenticated DM from {sender_username or sender_id}")
            span.set_attribute("ig.dropped", True)
            span.set_attribute("ig.drop_reason", "unauthenticated")
            metric_drops.labels(reason="unauthenticated").inc()
            return

        # Build context: last N messages from this thread.
        try:
            history_amount = int(os.environ.get("IG_CONSUMER_HISTORY_AMOUNT", "6"))
        except ValueError:
            history_amount = 6
        history_block = _format_history(thread_id, client, history_amount) if thread_id else ""

        prompt = _build_brain_prompt(event, history_block)

        # Brain call. Lazy import — edge.py is fully initialised by the
        # time we're in the loop, but we still defer to keep the consumer
        # importable even if edge.py changes shape.
        try:
            import edge as _edge  # type: ignore[import]
            reply = _edge.brain_respond(prompt)
        except Exception as exc:  # noqa: BLE001
            print(f"ig consumer: brain failed for {sender_username}: {exc!r}")
            span.set_attribute("ig.dropped", True)
            span.set_attribute("ig.drop_reason", "error")
            span.record_exception(exc)
            metric_drops.labels(reason="error").inc()
            return

        reply = _clean_reply(reply)
        if not reply:
            print(f"ig consumer: empty brain reply for {sender_username}; dropping")
            span.set_attribute("ig.dropped", True)
            span.set_attribute("ig.drop_reason", "empty_reply")
            metric_drops.labels(reason="error").inc()
            return

        span.set_attribute("reply_chars", len(reply))

        # Deliver. direct_send accepts thread_ids OR user_ids; we have
        # the thread id, prefer that — it routes back to the same DM.
        if not thread_id:
            print(f"ig consumer: no thread_id for {sender_username}; cannot reply")
            span.set_attribute("ig.dropped", True)
            span.set_attribute("ig.drop_reason", "no_thread_id")
            metric_drops.labels(reason="error").inc()
            return
        try:
            client.direct_send(reply, thread_ids=[thread_id])
        except Exception as exc:  # noqa: BLE001
            print(f"ig consumer: direct_send failed for {sender_username}: {exc!r}")
            span.set_attribute("ig.dropped", True)
            span.set_attribute("ig.drop_reason", "send_failed")
            span.record_exception(exc)
            metric_drops.labels(reason="error").inc()
            return

        metric_replies.labels(authenticated="1").inc()
        print(f"ig consumer: replied to {sender_username} ({len(reply)} chars)")


# ── Outer loop ──────────────────────────────────────────────────────────────
def _refresh_interval_seconds() -> int:
    try:
        return int(os.environ.get("IG_FOLLOWED_REFRESH_S", "300"))
    except ValueError:
        return 300


def _get_timeout_seconds() -> float:
    try:
        return float(os.environ.get("IG_CONSUMER_GET_TIMEOUT_S", "2"))
    except ValueError:
        return 2.0


def _maybe_refresh_followed(client: Any) -> None:
    """Refresh the followed set on cadence. First call: try a refresh,
    fall back to disk cache if it fails."""
    global _followed_ids
    interval = _refresh_interval_seconds()
    now = time.time()
    if (now - _followed_last_refresh) < interval:
        return
    ok = _refresh_followed(client)
    if not ok and not _followed_ids:
        # First-time refresh failed (rate limit on cold start) — use the
        # persisted cache so we don't silently drop legitimate DMs.
        cached = _load_followed_from_disk()
        if cached:
            _followed_ids = cached
            print(f"ig consumer: using cached followed set from disk ({len(cached)} accounts)")


def _run_loop() -> None:
    """Daemon thread entry point. Never returns. Each iteration is
    independently wrapped so a single bad event can't kill the thread."""
    global _followed_ids
    print("ig consumer: thread loop starting")

    # Seed the followed cache from disk so we have SOMETHING to gate with
    # while the poller comes up / before the first refresh succeeds.
    _followed_ids = _load_followed_from_disk()
    if _followed_ids:
        print(f"ig consumer: seeded followed cache from disk ({len(_followed_ids)} accounts)")

    handles: dict | None = None
    warned_no_poller = False

    while True:
        try:
            if handles is None:
                handles = _get_edge_handles()

            client = _get_poller_client()
            if client is None:
                if not warned_no_poller:
                    print("ig consumer: poller client not available yet — idling")
                    warned_no_poller = True
                time.sleep(5)
                continue
            if warned_no_poller:
                print("ig consumer: poller client now available — resuming")
                warned_no_poller = False

            _maybe_refresh_followed(client)

            try:
                payload = handles["queue"].get(timeout=_get_timeout_seconds())
            except queue.Empty:
                # No work this tick — loop back to refresh + retry.
                continue

            try:
                _process_event(payload, client, handles)
            except Exception as exc:  # noqa: BLE001
                # Per-event safety net. Any leaked exception lands here
                # without killing the thread.
                print(f"ig consumer: per-event handler crashed: {exc!r}")
                traceback.print_exc()
                try:
                    handles["metric_drops"].labels(reason="error").inc()
                except Exception:  # noqa: BLE001
                    pass
        except Exception as exc:  # noqa: BLE001
            # Outermost safety net. Sleep a bit so a tight failure loop
            # can't pin the CPU.
            print(f"ig consumer: outer loop crashed: {exc!r}")
            traceback.print_exc()
            time.sleep(5)


def start_consumer_thread() -> None:
    """Public entry point. Idempotent — second + later calls are no-ops.
    Caller (edge.py main) is expected to wrap this in try/except so any
    import / startup failure can't crash the daemon."""
    global _thread
    with _thread_lock:
        if _thread is not None and _thread.is_alive():
            print("ig consumer: thread already running, skipping start")
            return
        t = threading.Thread(
            target=_run_loop,
            name="jarvis-ig-consumer",
            daemon=True,
        )
        t.start()
        _thread = t
        print(f"ig consumer: started daemon thread "
              f"(followed_path={_followed_path()}, "
              f"refresh_s={_refresh_interval_seconds()})")
