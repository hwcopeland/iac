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
import queue as _queue
import threading
import time
import traceback
from typing import Any

# ── Module-level state (set by start_polling_thread) ──────────────────────
_thread: threading.Thread | None = None
_thread_lock = threading.Lock()

# ── DM media-share queue ───────────────────────────────────────────────────
# When a user (we follow back) DMs us a reel/post/story-share, the poller
# enqueues a media_share event onto this queue and jarvis_ig_media_dl
# drains it. Separate from _ig_event_queue (text DMs) and the comment
# responder's queue (post @mentions) so each consumer can be enabled
# independently without coupling. Bounded so an idle/wedged downloader
# can't starve the polling cycle — if full, we drop oldest with a log
# (mirrors the comment responder pattern).
_ig_media_queue: _queue.Queue = _queue.Queue(maxsize=200)


def get_media_queue() -> _queue.Queue:
    """Return the module-level media-share queue. Exposed so the
    downloader (jarvis_ig_media_dl) can drain it without importing
    private state. Module-attribute fallback (jarvis_ig_polling._ig_media_queue)
    still works for parity with the existing _ig_comment_queue contract."""
    return _ig_media_queue

# Live instagrapi Client cached for sibling modules (jarvis_ig_consumer)
# that need to send DMs. We expose this rather than letting consumers
# log in themselves — two parallel logins from one IP gets us challenged.
# Set inside the poll loop on every successful (re-)login; cleared on
# auth-class failures.
_client: Any = None


def get_client() -> Any:
    """Return the live instagrapi Client, or None if the poller hasn't
    logged in yet. Sibling modules (jarvis_ig_consumer.send_reply) use
    this to share the session — DO NOT call .login() on it; that's the
    poller's job."""
    return _client


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
    item_type = getattr(msg, "item_type", "") or ""
    # Surface which side-channel fields are populated so we can decide
    # how to extend non-text handling. Keep raw payload off the queue —
    # we only ship field names, not pydantic models.
    populated_extras = [
        name for name in (
            "media_share", "clip", "reel_share", "story_share",
            "xma_share", "voice_media", "link", "animated_media",
            "visual_media", "raven_media",
        )
        if getattr(msg, name, None) is not None
    ]
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
        "item_type": item_type,
        "populated_extras": populated_extras,
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


def _msg_as_dict(msg: Any) -> dict:
    """Convert an instagrapi DirectMessage pydantic model to a plain dict
    so we can poke at fields the model doesn't surface via attributes
    (e.g. nested generic_xma payloads with attribution metadata). Tolerant
    of both pydantic v1 (.dict()) and v2 (.model_dump()) plus the
    fallback where instagrapi already handed us a dict."""
    if isinstance(msg, dict):
        return msg
    for attr in ("model_dump", "dict"):
        dumper = getattr(msg, attr, None)
        if callable(dumper):
            try:
                out = dumper()
                if isinstance(out, dict):
                    return out
            except Exception:  # noqa: BLE001
                continue
    return {}


def _try_route_tagged_comment(msg: Any, thread: Any) -> bool:
    """If `msg` is a generic_xma DM item carrying a tagged-comment
    notification (IG delivers @mentions on posts as a special DM item
    with send_attribution=='tagged_comment'), parse the deep-link in
    cta_buttons[0].action_url to recover the media_pk + comment_id, then
    hand the event to the comment responder's queue. Returns True if we
    routed it (caller should `continue` and SKIP the normal DM enqueue),
    False otherwise.

    NOTE: this runs BEFORE the normal DM enqueue path on purpose — we
    never want a tagged_comment item to ALSO land in the DM queue. If
    routing here fails (queue full, parse error, comment responder
    module not importable), we still return True so we drop the item
    silently rather than confusing the DM consumer with a contentless
    generic_xma payload."""
    item = _msg_as_dict(msg)
    if item.get("item_type") != "generic_xma":
        return False
    if item.get("send_attribution") != "tagged_comment":
        return False
    xma_list = item.get("generic_xma") or []
    if not xma_list:
        # Still claim it — a tagged_comment with no payload is unusable
        # but shouldn't fall through to the DM queue either.
        print("ig polling: tagged_comment with empty generic_xma; dropping")
        return True
    xma = xma_list[0] if isinstance(xma_list[0], dict) else {}
    cta_buttons = xma.get("cta_buttons") or []
    cta = cta_buttons[0] if cta_buttons and isinstance(cta_buttons[0], dict) else {}
    action = cta.get("action_url") or ""
    if not action.startswith("instagram://comments?"):
        print(f"ig polling: tagged_comment with unexpected action_url {action!r}; dropping")
        return True
    import urllib.parse as _up
    try:
        qs = _up.parse_qs(_up.urlparse(action).query)
    except Exception as exc:  # noqa: BLE001
        print(f"ig polling: tagged_comment action_url parse failed ({exc!r}); dropping")
        return True
    media_id = (qs.get("media_id") or [None])[0]
    comment_id = (qs.get("comment_id") or [None])[0]
    if not media_id or not comment_id:
        print(f"ig polling: tagged_comment missing media_id/comment_id in {action!r}; dropping")
        return True
    # IG action_url media_id is "<pk>_<userid>" — instagrapi's
    # media_info(pk) expects the leading int before the underscore.
    media_pk = str(media_id).split("_", 1)[0]
    comment_event = {
        "source":             "mention_dm",
        "media_pk":           media_pk,
        "media_id":           media_id,
        "trigger_comment_id": str(comment_id),
        "trigger_text":       xma.get("subtitle_text") or "",
        "tagger_username":    xma.get("title_text") or "",
        "tagger_id":          str(item.get("user_id") or ""),
        "preview_url":        xma.get("preview_url") or "",
        "thread_id":          getattr(thread, "id", None) or item.get("thread_id"),
        "dm_item_id":         item.get("item_id"),
        "timestamp":          item.get("timestamp"),
    }
    try:
        import jarvis_ig_comment_responder as _resp  # type: ignore[import]
        _resp._ig_comment_queue.put_nowait(comment_event)
        print(
            f"ig polling: enqueued TAGGED_COMMENT from "
            f"@{comment_event['tagger_username']} on media "
            f"{comment_event['media_pk']}: {comment_event['trigger_text'][:80]!r}"
        )
    except Exception as exc:  # noqa: BLE001
        # Comment responder not loaded / queue full / etc. Log loudly
        # so Hampton sees it, but still return True — we don't want a
        # half-routed tagged_comment to also pollute the DM queue.
        print(f"ig polling: failed to enqueue tagged_comment ({exc!r}); dropping")
    return True


def _extract_media_share_fields(item: dict) -> dict | None:
    """Pull (media_pk, media_type, caption_hint, source_url, sender) out
    of a DM item that shares media. Returns None if the item isn't a
    recognised share type OR we can't extract a media_pk.
    Defensive — IG's payload shapes drift; if a key is missing we log
    and return None rather than crashing the poll cycle. The downloader
    will silently skip None events anyway, but we'd rather not enqueue
    them in the first place."""
    item_type = item.get("item_type") or ""
    media_pk: str = ""
    media_type: str = ""
    caption_hint: str = ""
    source_url: str = ""

    if item_type == "clip":
        # IG wraps reels in a double-nested {clip: {clip: {...}}}.
        # The outer .clip is a generic wrapper; the inner .clip holds
        # the actual media. Earlier IG dumps confirmed this shape.
        outer = item.get("clip") or {}
        inner = outer.get("clip") if isinstance(outer, dict) else None
        if not isinstance(inner, dict):
            return None
        media_pk = str(inner.get("pk") or inner.get("id") or "").split("_", 1)[0]
        media_type = "clip"
        cap = inner.get("caption") or {}
        if isinstance(cap, dict):
            caption_hint = (cap.get("text") or "")[:400]
        code = inner.get("code") or ""
        if code:
            source_url = f"https://www.instagram.com/reel/{code}/"

    elif item_type == "media_share":
        ms = item.get("media_share") or {}
        if not isinstance(ms, dict):
            return None
        media_pk = str(ms.get("pk") or ms.get("id") or "").split("_", 1)[0]
        # caption may be string-typed or nested {text: ...}
        cap = ms.get("caption")
        if isinstance(cap, dict):
            caption_hint = (cap.get("text") or "")[:400]
        elif isinstance(cap, str):
            caption_hint = cap[:400]
        media_type = "feed"
        code = ms.get("code") or ""
        if code:
            source_url = f"https://www.instagram.com/p/{code}/"

    elif item_type in ("xma_media_share", "xma_clip", "xma_reel_share",
                        "xma_story_share", "xma_profile"):
        # XMA = "extensible message attachment". IG ships shared media
        # through several XMA-flavoured item types depending on whether
        # the share is a feed post / reel / story / etc. They all share
        # the same payload shape: an array of XMA objects, each with a
        # cta_buttons[0].action_url deep-link or a preview_media_fbid.
        # We treat them uniformly here.
        xma_list = (item.get("xma_media_share") or item.get("xma_clip")
                    or item.get("xma_reel_share") or item.get("xma_story_share")
                    or item.get("xma_profile") or item.get("generic_xma") or [])
        if not isinstance(xma_list, list) or not xma_list:
            return None
        xma = xma_list[0] if isinstance(xma_list[0], dict) else {}
        # Try cta action URL first (mirrors tagged_comment path).
        cta_buttons = xma.get("cta_buttons") or []
        if cta_buttons and isinstance(cta_buttons[0], dict):
            action = cta_buttons[0].get("action_url") or ""
            # Examples: instagram://media?id=12345_67890
            # or instagram://reels?media_id=12345_67890
            if action:
                import urllib.parse as _up
                try:
                    parsed = _up.urlparse(action)
                    qs = _up.parse_qs(parsed.query)
                    mid = (qs.get("media_id") or qs.get("id") or [""])[0]
                    if mid:
                        media_pk = str(mid).split("_", 1)[0]
                except Exception:  # noqa: BLE001
                    pass
        # preview_media_fbid is a fallback in some XMA shapes.
        if not media_pk:
            pf = xma.get("preview_media_fbid")
            if pf:
                media_pk = str(pf).split("_", 1)[0]
        if not media_pk:
            return None
        media_type = "feed"  # XMA can wrap reels too; "feed" is the safe default
        caption_hint = (xma.get("subtitle_text") or xma.get("header_subtitle_text") or "")[:400]
        source_url = xma.get("preview_url") or ""

    elif item_type == "story_share":
        ss = item.get("story_share") or {}
        if not isinstance(ss, dict):
            return None
        media = ss.get("media") or {}
        if not isinstance(media, dict):
            return None
        media_pk = str(media.get("pk") or media.get("id") or "").split("_", 1)[0]
        media_type = "story"
        cap = media.get("caption")
        if isinstance(cap, dict):
            caption_hint = (cap.get("text") or "")[:400]

    else:
        return None

    if not media_pk:
        return None

    return {
        "media_pk":    media_pk,
        "media_type":  media_type,
        "caption_hint": caption_hint,
        "source_url":  source_url,
    }


def _try_route_media_share(msg: Any, thread: Any) -> bool:
    """If `msg` is a shared-media DM item (clip/media_share/xma_media_share/
    story_share), enqueue a media_share event onto _ig_media_queue and
    return True so the caller can SKIP the normal DM enqueue path.
    Returns False if the item isn't a media share — caller falls through
    to the regular DM enqueue logic.

    Accepts either an instagrapi pydantic DirectMessage OR a raw dict
    (from the raw-inbox scan), same pattern as _try_route_tagged_comment."""
    item = _msg_as_dict(msg)
    item_type = item.get("item_type") or ""
    if item_type not in ("clip", "media_share", "xma_media_share", "story_share"):
        return False

    fields = _extract_media_share_fields(item)
    if not fields:
        print(f"ig polling: media_share item_type={item_type} missing media_pk; dropping")
        # Still return True — we don't want a half-parsed share to also
        # land in the DM queue and confuse the text consumer.
        return True

    # Sender resolution: prefer thread.users[match] for username; fall
    # back to the user nested inside the share payload.
    from_user_id = int(item.get("user_id") or 0)
    from_username = ""
    try:
        for u in (getattr(thread, "users", None) or []) or (thread.get("users", []) if isinstance(thread, dict) else []):
            uid = getattr(u, "pk", None) if not isinstance(u, dict) else u.get("pk")
            if uid and int(uid) == from_user_id:
                from_username = (getattr(u, "username", None) if not isinstance(u, dict) else u.get("username")) or ""
                break
    except Exception:  # noqa: BLE001
        pass
    if not from_username:
        # Last-ditch: some share payloads embed the sender's username in
        # the nested user object on the share itself (e.g. clip.user.username
        # is the POSTER's, NOT sender — only use share-level user if no
        # thread match was possible).
        pass

    thread_id = (
        (thread.get("thread_id") if isinstance(thread, dict) else None)
        or (getattr(thread, "id", None))
        or item.get("thread_id")
        or ""
    )

    event = {
        "source":         "dm_media_share",
        "media_pk":       fields["media_pk"],
        "media_type":     fields["media_type"],
        "from_username":  from_username,
        "from_user_id":   from_user_id,
        "thread_id":      str(thread_id) if thread_id else "",
        "dm_item_id":     str(item.get("item_id") or ""),
        "timestamp_us":   int(item.get("timestamp") or 0),
        "caption_hint":   fields["caption_hint"],
        "source_url":     fields["source_url"],
    }
    try:
        _ig_media_queue.put_nowait(event)
        print(
            f"ig polling: enqueued DM_MEDIA_SHARE from "
            f"@{from_username or from_user_id} media_pk={fields['media_pk']} "
            f"type={item_type}"
        )
    except _queue.Full:
        # Drop oldest to keep latency low. The downloader is single-threaded
        # so a wedged Synology/iSCSI write could back this up; we still
        # claim the item so we don't re-enqueue on the next poll cycle.
        try:
            _ig_media_queue.get_nowait()
            _ig_media_queue.put_nowait(event)
            print("ig polling: media queue full — dropped oldest, enqueued new")
        except Exception:  # noqa: BLE001
            print("ig polling: media queue full + recovery failed; dropping event")
    except Exception as exc:  # noqa: BLE001
        print(f"ig polling: media_share enqueue failed ({exc!r}); dropping")
    return True


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

        # ── Raw inbox scan: route tagged_comment + media_share first ─────
        # `client.direct_threads()` parses messages into pydantic
        # DirectMessage models that DROP some fields (`send_attribution`,
        # nested xma_media_share payloads), so detection from those is
        # impossible. Fetch the raw inbox dict in parallel and route
        # mention-DMs + shared-media DMs from there. We DO still re-scan
        # in the direct_threads loop below for shapes the raw inbox
        # missed — both routes populate routed_item_ids for dedupe.
        routed_item_ids: set = set()
        try:
            raw = client.private_request(
                "direct_v2/inbox/?persistentBadging=true&limit=20"
            )
            for raw_thread in (raw.get("inbox", {}) or {}).get("threads", []) or []:
                for raw_item in raw_thread.get("items", []) or []:
                    ts_us = int(raw_item.get("timestamp") or 0)
                    if ts_us <= hwm_us:
                        continue
                    item_id = str(raw_item.get("item_id") or "")
                    # tagged_comment route (existing)
                    if raw_item.get("send_attribution") == "tagged_comment":
                        if _try_route_tagged_comment(raw_item, raw_thread):
                            routed_item_ids.add(item_id)
                            continue
                    # media_share route (new) — any of the four share shapes
                    if raw_item.get("item_type") in (
                        "clip", "media_share", "xma_media_share", "story_share",
                    ):
                        if _try_route_media_share(raw_item, raw_thread):
                            routed_item_ids.add(item_id)
                            continue
        except Exception as exc:  # noqa: BLE001
            print(f"ig polling: raw-inbox scan failed ({exc!r})")

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
                    # Intercept generic_xma 'tagged_comment' DM items
                    # BEFORE we hit the normal DM enqueue path. These are
                    # @mentions on posts that IG delivers via the DM
                    # inbox; they belong on the comment responder's
                    # queue, not the DM queue. Routed items still
                    # advance the hwm so we don't re-process on next
                    # poll cycle / pod restart.
                    if _try_route_tagged_comment(msg, thread):
                        msg_span.set_attribute("ig.tagged_comment_routed", True)
                        if ts_us > new_hwm:
                            new_hwm = ts_us
                        continue
                    # Skip messages already routed by the raw-inbox scan
                    # earlier in this cycle (pydantic models don't carry
                    # send_attribution; raw scan is the source of truth).
                    msg_id = str(getattr(msg, "id", "") or "")
                    if msg_id and msg_id in routed_item_ids:
                        msg_span.set_attribute("ig.routed_raw", True)
                        if ts_us > new_hwm:
                            new_hwm = ts_us
                        continue
                    # Route shared-media DMs (clip/media_share/xma_media_share/
                    # story_share) to the media downloader queue BEFORE
                    # falling through to the text-DM enqueue. Pydantic
                    # DirectMessage models DO surface item_type + the
                    # nested clip/media_share fields for most shapes, so
                    # this catches anything the raw-inbox scan missed.
                    if _try_route_media_share(msg, thread):
                        msg_span.set_attribute("ig.media_share_routed", True)
                        if ts_us > new_hwm:
                            new_hwm = ts_us
                        continue
                    event = _format_event(thread, msg)
                    try:
                        event_queue.put_nowait(event)
                        enqueued += 1
                        metric_messages.inc()
                        # Log every successful enqueue. The silent-on-success
                        # behaviour confused Hampton when he was watching the
                        # log to confirm DMs were landing — single log line
                        # per pickup makes the data path visible.
                        try:
                            sender = (event.get("from") or {}).get("username") or "unknown"
                            preview = (event.get("text") or "")[:60]
                            it = event.get("item_type") or "?"
                            extras = event.get("populated_extras") or []
                            extras_s = ",".join(extras) if extras else "-"
                            print(f"ig polling: enqueued msg from {sender} item_type={it} extras=[{extras_s}]: {preview!r}")
                        except Exception:  # noqa: BLE001
                            pass
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
        "FeedbackRequired",
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

    global _client
    try:
        import jarvis_ig_cooldown as _cd  # type: ignore[import]
    except Exception:  # noqa: BLE001
        _cd = None
    while True:
        try:
            if handles is None:
                handles = _get_edge_handles()
                metric_errors = handles["metric_errors"]
                metric_cycles = handles["metric_cycles"]
            if client is None:
                client = _build_client()
                # Publish the live client so sibling modules (consumer)
                # can borrow it for outbound DMs. Avoids two parallel
                # logins from one IP, which would trip a challenge.
                _client = client

            # Cooldown gate — when IG is throttling the account, sit out
            # the entire cycle rather than making any API calls.
            if _cd and _cd.is_cooling_down():
                rem = _cd.cooldown_remaining_s()
                print(f"ig polling: cooldown active ({rem}s remaining) — skip cycle")
                metric_cycles.labels(outcome="cooldown").inc()
                time.sleep(min(60, max(5, rem)))
                continue

            hwm_us = _poll_once(client, handles, hwm_us)
            _save_last_seen(hwm_us)
            metric_cycles.labels(outcome="ok").inc()
        except Exception as exc:  # noqa: BLE001
            # Catch EVERYTHING — voice path is more important than IG.
            reason = _classify_error(exc)
            if _cd and reason in ("FeedbackRequired", "PleaseWaitFewMinutes"):
                _cd.record_throttle(exc)
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
                _client = None
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
