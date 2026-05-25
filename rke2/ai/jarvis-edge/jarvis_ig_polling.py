"""jarvis_ig_polling — Instagram DM poller using instagrapi.

Polls the @hmlbjarvis IG inbox every ~30s and pushes new direct
messages onto the same in-memory _ig_event_queue that the Meta webhook
handler in edge.py feeds. The webhook path is the "proper" ingress but
requires Meta Business Verification, which we are not doing yet — this
private-mobile-API poller is the pragmatic workaround.

Contract (only the daemon thread is exposed):

    import jarvis_ig_polling
    jarvis_ig_polling.start_polling_thread()

Spawned from edge.py:main() when IG_POLLING_ENABLED=1.

Env (read at thread start, NOT at import time):

    IG_USERNAME           IG account login (string)
    IG_PASSWORD           IG account password (string)
    IG_POLL_INTERVAL_S    poll cadence in seconds (default 30)
    IG_POLL_THREAD_AMOUNT how many threads to fetch per poll (default 20)
    IG_SESSION_PATH       session settings file (default /state/ig_session.json)
    IG_LAST_SEEN_PATH     high-water-mark file (default /state/ig_last_seen.json)

Hard guarantees:
- Never raises into the daemon thread's outer loop. ANY exception
  (login challenge, 2FA, rate limit, network, JSON parse, etc.) is
  caught + logged + the thread sleeps `IG_BACKOFF_S` (default 5min)
  before retrying. JARVIS voice path must keep working even if IG
  is completely broken.
- Session settings are persisted on first successful login so
  subsequent pod restarts skip the expensive (and challenge-prone)
  fresh login flow.
- High-water-mark is a single integer (max message timestamp_us seen).
  Messages with timestamp_us <= hwm are skipped. On first run with no
  hwm file we seed from `now` so we don't replay the entire DM history.

ChallengeRequired handling: instagrapi raises ChallengeRequired when
Instagram demands an email/SMS code (very common on first login from a
new IP). Fully automating this is messy (requires email/SMS access).
We log loudly, persist what state we can, and back off — Hampton
resolves manually:

    kubectl exec -it -n ai deploy/jarvis-stack -c jarvis-edge -- python3 -c "
    from instagrapi import Client
    c = Client(); c.load_settings('/state/ig_session.json')
    c.challenge_resolve(c.last_json)  # follow prompts
    c.dump_settings('/state/ig_session.json')
    "

The poller picks up the resolved session on the next backoff retry.
"""
from __future__ import annotations

import json
import os
import threading
import time
import traceback
from typing import Any

# ── Module-level state (set by start_polling_thread) ──────────────────────
_thread: threading.Thread | None = None
_thread_lock = threading.Lock()

# Default backoff after ANY error (login challenge, rate limit, etc.).
# 5 minutes is a balance between "recover quickly when transient" and
# "don't hammer IG with retries when they're rate-limiting us".
_DEFAULT_BACKOFF_S = 300


# ── Lazy imports from edge.py ──────────────────────────────────────────────
# We can't `from edge import _ig_event_queue, tracer, ...` at module load
# time — edge.py imports us inside main() AFTER its module body runs, so
# at our import time edge.py is still partially initialised. Defer all
# edge-module access to inside the thread loop where main() has finished.
def _get_edge_handles() -> dict[str, Any]:
    """Return {queue, tracer, metric_*} from edge.py. Called once at
    thread start. Returns an empty dict if edge.py isn't importable
    (shouldn't happen in production — we're spawned from edge.main())."""
    import edge as _edge  # type: ignore[import]

    return {
        "queue": _edge._ig_event_queue,
        "tracer": _edge.tracer,
        "metric_messages": _ensure_counter(
            "jarvis_ig_polling_messages_total",
            "IG DM messages picked up by the poller and enqueued",
        ),
        "metric_cycles": _ensure_counter(
            "jarvis_ig_polling_cycles_total",
            "Completed IG polling cycles",
            labelnames=("outcome",),
        ),
        "metric_errors": _ensure_counter(
            "jarvis_ig_polling_errors_total",
            "IG polling errors by reason",
            labelnames=("reason",),
        ),
    }


# ── Prometheus counter helpers ─────────────────────────────────────────────
# The polling counters are NEW (not pre-declared in edge.py), so we lazily
# construct them here. If prometheus_client isn't importable we fall back
# to a no-op (mirrors edge.py's pattern) so the thread still runs.
class _NoopMetric:
    def labels(self, *_a, **_kw):
        return self

    def inc(self, *_a, **_kw):
        pass


def _ensure_counter(name: str, doc: str, labelnames: tuple = ()) -> Any:
    """Get-or-create a Prometheus Counter. Safe under repeated calls —
    if the metric already exists in the default REGISTRY (e.g. from a
    previous module reload), return the existing one instead of raising
    ValueError."""
    try:
        from prometheus_client import Counter, REGISTRY
    except Exception:  # noqa: BLE001
        return _NoopMetric()
    # Fast path: try to construct.
    try:
        return Counter(name, doc, labelnames=labelnames)
    except ValueError:
        # Already registered — dig it out of REGISTRY's collectors.
        for collector in list(REGISTRY._collector_to_names.keys()):  # type: ignore[attr-defined]
            if getattr(collector, "_name", None) == name:
                return collector
        return _NoopMetric()


# ── Session + high-water-mark persistence ──────────────────────────────────
def _session_path() -> str:
    return os.environ.get("IG_SESSION_PATH", "/state/ig_session.json")


def _last_seen_path() -> str:
    return os.environ.get("IG_LAST_SEEN_PATH", "/state/ig_last_seen.json")


def _load_last_seen() -> int:
    """Return the last-seen message timestamp (microseconds since epoch
    — instagrapi's native unit). Returns 0 if the file is missing or
    corrupt; the FIRST successful poll seeds the hwm from `now` so we
    don't replay the entire DM history."""
    path = _last_seen_path()
    try:
        with open(path) as f:
            blob = json.load(f)
        ts = int(blob.get("timestamp_us", 0))
        return ts
    except (OSError, ValueError, json.JSONDecodeError):
        return 0


def _save_last_seen(ts_us: int) -> None:
    """Persist the high-water mark atomically (write+rename) so a crash
    mid-write can't leave a half-truncated JSON file."""
    path = _last_seen_path()
    tmp = path + ".tmp"
    try:
        os.makedirs(os.path.dirname(path), exist_ok=True)
    except OSError:
        pass
    try:
        with open(tmp, "w") as f:
            json.dump({"timestamp_us": int(ts_us)}, f)
        os.replace(tmp, path)
    except OSError as exc:
        print(f"ig polling: failed to persist last_seen: {exc!r}")


# ── instagrapi client setup ────────────────────────────────────────────────
def _build_client() -> Any:
    """Construct + log in an instagrapi Client. Uses persisted session
    settings when available. Raises any instagrapi exception unchanged —
    the outer loop handles them. NEVER caches the client across login
    failures, so a wedged session doesn't poison subsequent retries."""
    from instagrapi import Client  # type: ignore[import]

    username = os.environ.get("IG_USERNAME", "").strip()
    password = os.environ.get("IG_PASSWORD", "").strip()
    if not username or not password:
        raise RuntimeError("IG_USERNAME / IG_PASSWORD not set")

    session_path = _session_path()
    client = Client()
    settings_loaded = False
    if os.path.exists(session_path):
        try:
            client.load_settings(session_path)
            settings_loaded = True
            print(f"ig polling: loaded session settings from {session_path}")
        except Exception as exc:  # noqa: BLE001
            # Corrupt settings → fall through to fresh login. Don't
            # delete the file — Hampton may want to inspect it.
            print(f"ig polling: load_settings failed ({exc!r}); falling back to fresh login")

    # login() is a no-op when settings already cover a valid session, but
    # instagrapi's docs still recommend calling it so the client picks up
    # username/password for any in-session re-auth.
    client.login(username, password)

    if not settings_loaded:
        # First successful login — persist settings for future restarts.
        try:
            os.makedirs(os.path.dirname(session_path), exist_ok=True)
        except OSError:
            pass
        try:
            client.dump_settings(session_path)
            print(f"ig polling: persisted session settings to {session_path}")
        except Exception as exc:  # noqa: BLE001
            print(f"ig polling: dump_settings failed ({exc!r}); will re-login on restart")
    return client


# ── Per-poll work ──────────────────────────────────────────────────────────
def _format_event(thread: Any, msg: Any) -> dict:
    """Build the event dict that downstream consumers read off the
    queue. Shape mirrors the webhook handler's `payload` so consumers
    can't tell the source apart — `source: polling` distinguishes if
    needed. instagrapi returns DirectMessage / DirectThread / UserShort
    pydantic models; we pull the few fields the consumer needs.

    `from` (sender) is best-effort: thread.users[*] gives username for
    each user_id; we look the sender up there. Falls back to id-only if
    the user isn't in users (rare — happens when sender == self)."""
    sender_id = getattr(msg, "user_id", None)
    sender_username: str | None = None
    try:
        for u in getattr(thread, "users", []) or []:
            if getattr(u, "pk", None) == sender_id:
                sender_username = getattr(u, "username", None)
                break
    except Exception:  # noqa: BLE001
        pass
    text = getattr(msg, "text", "") or ""
    timestamp = getattr(msg, "timestamp", None)
    if timestamp is not None and hasattr(timestamp, "isoformat"):
        ts_iso = timestamp.isoformat()
    else:
        ts_iso = str(timestamp) if timestamp is not None else ""
    return {
        "source": "polling",
        "type": "messages",
        "thread_id": getattr(thread, "id", None),
        "message_id": getattr(msg, "id", None),
        "from": {
            "id": str(sender_id) if sender_id is not None else "",
            "username": sender_username or "",
        },
        "text": text,
        "timestamp": ts_iso,
    }


def _message_ts_us(msg: Any) -> int:
    """Return the message timestamp in MICROSECONDS since the epoch.
    instagrapi returns datetime for `timestamp`; we convert to µs so the
    hwm comparison stays in pure-int territory. Returns 0 if the message
    has no parseable timestamp (will be treated as "old" and skipped)."""
    ts = getattr(msg, "timestamp", None)
    if ts is None:
        return 0
    try:
        # datetime → epoch seconds (float) → µs (int).
        return int(ts.timestamp() * 1_000_000)
    except Exception:  # noqa: BLE001
        return 0


def _poll_once(client: Any, handles: dict, hwm_us: int) -> int:
    """Run ONE polling cycle. Returns the new hwm (max ts seen across
    all messages this cycle, or the input hwm if nothing newer was
    found). Raises any instagrapi/network exception upward so the outer
    loop can categorise + back off."""
    amount = int(os.environ.get("IG_POLL_THREAD_AMOUNT", "20"))
    tracer = handles["tracer"]
    metric_messages = handles["metric_messages"]
    event_queue = handles["queue"]

    with tracer.start_as_current_span("jarvis.ig.poll_cycle") as cycle_span:
        cycle_span.set_attribute("ig.thread_amount", amount)
        cycle_span.set_attribute("ig.hwm_us", hwm_us)
        threads = client.direct_threads(amount=amount)
        cycle_span.set_attribute("ig.thread_count", len(threads) if threads else 0)

        new_hwm = hwm_us
        enqueued = 0
        scanned = 0
        for thread in threads or []:
            for msg in getattr(thread, "messages", []) or []:
                scanned += 1
                ts_us = _message_ts_us(msg)
                if ts_us <= hwm_us:
                    continue  # already seen
                with tracer.start_as_current_span("jarvis.ig.poll_message") as msg_span:
                    msg_span.set_attribute("ig.thread_id", str(getattr(thread, "id", "")))
                    msg_span.set_attribute("ig.message_id", str(getattr(msg, "id", "")))
                    msg_span.set_attribute("ig.message_ts_us", ts_us)
                    event = _format_event(thread, msg)
                    try:
                        event_queue.put_nowait(event)
                        enqueued += 1
                        metric_messages.inc()
                        if ts_us > new_hwm:
                            new_hwm = ts_us
                    except Exception as exc:  # noqa: BLE001
                        # queue.Full is the expected case here — the
                        # consumer is wedged. Log + drop (mirrors the
                        # webhook handler's behaviour) and DON'T
                        # advance the hwm, so we retry on next poll.
                        msg_span.set_attribute("error", True)
                        msg_span.record_exception(exc)
                        print(f"ig polling: enqueue failed ({exc!r}); will retry on next poll")
        cycle_span.set_attribute("ig.messages_scanned", scanned)
        cycle_span.set_attribute("ig.messages_enqueued", enqueued)
        return new_hwm


# ── Outer loop ─────────────────────────────────────────────────────────────
def _classify_error(exc: BaseException) -> str:
    """Best-effort reason label for the errors counter. Bounded
    cardinality — anything we don't recognise lumps into "other"."""
    name = type(exc).__name__
    # Common instagrapi exception class names (we don't import the
    # exception module up here — only inside the running thread — so
    # match by class name).
    known = {
        "ChallengeRequired",
        "LoginRequired",
        "PleaseWaitFewMinutes",
        "RateLimitError",
        "ClientThrottledError",
        "ClientConnectionError",
        "ClientForbiddenError",
        "ClientJSONDecodeError",
        "ClientError",
        "BadPassword",
        "TwoFactorRequired",
        "RuntimeError",
        "TimeoutError",
    }
    return name if name in known else "other"


def _backoff_seconds() -> int:
    try:
        return int(os.environ.get("IG_BACKOFF_S", str(_DEFAULT_BACKOFF_S)))
    except ValueError:
        return _DEFAULT_BACKOFF_S


def _poll_interval_seconds() -> int:
    try:
        return int(os.environ.get("IG_POLL_INTERVAL_S", "30"))
    except ValueError:
        return 30


def _run_loop() -> None:
    """Daemon thread entry point. Never returns — only stops when the
    process exits. Top-level try/except wraps EVERY iteration so a
    single bad poll can't kill the thread."""
    print("ig polling: thread loop starting")
    handles: dict | None = None
    client: Any = None
    metric_errors: Any = _NoopMetric()
    metric_cycles: Any = _NoopMetric()

    # Seed hwm: if no file exists, start from "now" so we don't replay
    # the full DM backlog on first run. (Set this BEFORE first login so
    # a login failure doesn't reset the hwm.)
    hwm_us = _load_last_seen()
    if hwm_us == 0:
        hwm_us = int(time.time() * 1_000_000)
        _save_last_seen(hwm_us)
        print(f"ig polling: seeded hwm = {hwm_us} (now) — will only pick up DMs after this moment")

    while True:
        try:
            if handles is None:
                handles = _get_edge_handles()
                metric_errors = handles["metric_errors"]
                metric_cycles = handles["metric_cycles"]
            if client is None:
                client = _build_client()

            hwm_us = _poll_once(client, handles, hwm_us)
            _save_last_seen(hwm_us)
            metric_cycles.labels(outcome="ok").inc()
        except Exception as exc:  # noqa: BLE001
            # Catch EVERYTHING — voice path is more important than IG.
            reason = _classify_error(exc)
            metric_errors.labels(reason=reason).inc()
            metric_cycles.labels(outcome="error").inc()
            print(f"ig polling: cycle failed ({reason}): {exc!r}")
            # Print the traceback for first-time challenges so Hampton
            # can see what instagrapi wanted (verify URL / 2FA prompt).
            traceback.print_exc()
            # Drop the client on auth-class failures so the next loop
            # iteration re-logs-in fresh (picks up any manually-resolved
            # challenge state via load_settings).
            if reason in ("ChallengeRequired", "LoginRequired",
                          "BadPassword", "TwoFactorRequired"):
                client = None
                print(f"ig polling: dropped client; will re-login after backoff "
                      f"({reason} — may need manual resolution via "
                      f"`kubectl exec` + `c.challenge_resolve(c.last_json)`)")
            backoff = _backoff_seconds()
            print(f"ig polling: sleeping {backoff}s before retry")
            time.sleep(backoff)
            continue

        # Normal cadence between cycles.
        time.sleep(max(1, _poll_interval_seconds()))


def start_polling_thread() -> None:
    """Public entry point. Idempotent — second + later calls are no-ops.
    Caller (edge.py main) is expected to wrap this in try/except so an
    import failure of instagrapi doesn't crash the daemon."""
    global _thread
    with _thread_lock:
        if _thread is not None and _thread.is_alive():
            print("ig polling: thread already running, skipping start")
            return
        t = threading.Thread(
            target=_run_loop,
            name="jarvis-ig-polling",
            daemon=True,
        )
        t.start()
        _thread = t
        interval = _poll_interval_seconds()
        print(f"ig polling: started daemon thread (interval={interval}s, "
              f"session={_session_path()}, hwm={_last_seen_path()})")
