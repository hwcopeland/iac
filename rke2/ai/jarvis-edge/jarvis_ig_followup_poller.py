"""jarvis_ig_followup_poller — catch follow-up comments on posts
JARVIS has already replied on, even when they don't re-mention
@hmlbjarvis.

Why this exists: IG only sends mention-notifications via the DM
inbox's `tagged_comment` events when the user explicitly @s us.
A follow-up reply to JARVIS's comment thread that doesn't re-tag
@hmlbjarvis is invisible to the comment poller.

Solution: maintain a small set of "media we recently commented on,"
poll their comments every ~60s, and enqueue any new comments by
followed accounts onto the same _ig_comment_queue the mention path
feeds. The downstream consumer (gaslight prompt / RP / Q&A / OF
protocol) treats them identically.

Contract:
    import jarvis_ig_followup_poller
    jarvis_ig_followup_poller.start_thread()
    jarvis_ig_followup_poller.track(media_pk)   # called by the
                                                # comment consumer
                                                # after each reply

Env:
    IG_FOLLOWUP_POLL_ENABLED        "1" to start (default)
    IG_FOLLOWUP_INTERVAL_S          per-cycle sleep (default 600)
    IG_FOLLOWUP_TTL_S               drop media after this many seconds
                                    of inactivity (default 7d)
    IG_FOLLOWUP_TRACKED_PATH        on-disk tracked-media list
                                    (default /state/ig_tracked_media.json)
"""
from __future__ import annotations

import json
import os
import threading
import time
import traceback
from typing import Any

_thread: threading.Thread | None = None
_thread_lock = threading.Lock()

# In-memory mirror of the tracked-media file: {media_pk: last_polled_us}
_tracked: dict[str, int] = {}
_tracked_lock = threading.Lock()


def _tracked_path() -> str:
    return os.environ.get(
        "IG_FOLLOWUP_TRACKED_PATH", "/state/ig_tracked_media.json"
    )


def _interval_s() -> int:
    return int(os.environ.get("IG_FOLLOWUP_INTERVAL_S", "600"))


def _ttl_s() -> int:
    # 7 days default — long enough that follow-ups land, short enough
    # that the poll set stays tiny.
    return int(os.environ.get("IG_FOLLOWUP_TTL_S", str(7 * 24 * 3600)))


def _load_tracked() -> dict[str, int]:
    try:
        with open(_tracked_path()) as f:
            d = json.load(f)
        return {str(k): int(v) for k, v in (d.get("tracked") or {}).items()}
    except (OSError, ValueError, json.JSONDecodeError):
        return {}


def _save_tracked(snap: dict[str, int]) -> None:
    path = _tracked_path()
    tmp = path + ".tmp"
    try:
        with open(tmp, "w") as f:
            json.dump({"tracked": snap, "saved_at": time.time()}, f)
        os.replace(tmp, path)
    except OSError as exc:
        print(f"ig followup: persist failed: {exc!r}")


def track(media_pk: str) -> None:
    """Public: register a media as something to follow-up-poll. Idempotent.
    Called by the comment consumer after each successful reply."""
    pk = str(media_pk)
    now_us = int(time.time() * 1_000_000)
    with _tracked_lock:
        _tracked[pk] = now_us
        _save_tracked(dict(_tracked))


def _prune_expired() -> None:
    """Drop media we haven't seen activity on in IG_FOLLOWUP_TTL_S."""
    ttl_us = _ttl_s() * 1_000_000
    cutoff = int(time.time() * 1_000_000) - ttl_us
    with _tracked_lock:
        before = len(_tracked)
        for pk in list(_tracked.keys()):
            if _tracked[pk] < cutoff:
                del _tracked[pk]
        if len(_tracked) != before:
            _save_tracked(dict(_tracked))
            print(f"ig followup: pruned {before - len(_tracked)} stale media")


def _get_handles() -> dict[str, Any] | None:
    """Pull live handles from sibling modules. Returns None if either
    the poller (no client) or the comment responder (no queue) isn't
    ready yet — caller should sleep + retry."""
    try:
        import jarvis_ig_polling as _polling
        import jarvis_ig_comment_responder as _resp
    except Exception as exc:  # noqa: BLE001
        print(f"ig followup: sibling import failed: {exc!r}")
        return None
    client = _polling.get_client()
    if client is None:
        return None
    # The consumer's followed-set + replied-set are the source of truth
    # for auth + dedupe. Read them by name; tolerate missing.
    try:
        from jarvis_ig_consumer import _followed_ids
    except Exception:  # noqa: BLE001
        _followed_ids = set()
    return {
        "client": client,
        "queue": _resp._ig_comment_queue,
        "followed_ids": _followed_ids,
        # replied_set lives inside the consumer thread's closure; we
        # can't reach it directly. The downstream consumer dedupes
        # again before posting (per-job check in _process_job), so a
        # duplicate enqueue here is harmless — just an extra cycle of
        # work that drops at the consumer.
    }


_OWN_USERNAME_CACHE: str | None = None


def _own_username() -> str:
    """Cache @hmlbjarvis's own username so we can skip self-comments."""
    global _OWN_USERNAME_CACHE
    if _OWN_USERNAME_CACHE is None:
        _OWN_USERNAME_CACHE = (os.environ.get("IG_USERNAME") or "").lower().strip()
    return _OWN_USERNAME_CACHE


def _poll_one(client: Any, media_pk: str, followed_ids: set, queue) -> int:
    """Scan one media's comments for new follow-ups by followed users.
    Returns count of new follow-ups enqueued."""
    try:
        comments = client.media_comments(media_pk, amount=8) or []
    except Exception as exc:  # noqa: BLE001
        print(f"ig followup: media_comments({media_pk}) failed: {exc!r}")
        return 0

    own = _own_username()
    enqueued = 0
    last_seen = _tracked.get(str(media_pk), 0)
    newest_seen = last_seen

    for c in comments:
        try:
            user = (getattr(getattr(c, "user", None), "username", "") or "").lower()
            user_id = str(getattr(getattr(c, "user", None), "pk", "") or "")
            text = (getattr(c, "text", "") or "").strip()
            cid = str(getattr(c, "pk", "") or "")
            ts = getattr(c, "created_at_utc", None)
            ts_us = int(ts.timestamp() * 1_000_000) if ts else 0
        except Exception:  # noqa: BLE001
            continue

        if not cid or not text:
            continue
        if user == own:
            continue                            # our own replies
        if int(user_id or 0) not in followed_ids:
            continue                            # not a followed user
        if ts_us <= last_seen:
            continue                            # already seen
        if ts_us > newest_seen:
            newest_seen = ts_us

        event = {
            "source": "mention_dm",            # reuse same hydration path
            "media_pk": str(media_pk),
            "media_id": str(media_pk),
            "trigger_comment_id": cid,
            "trigger_text": text,
            "tagger_username": user,
            "tagger_id": user_id,
            "preview_url": "",
            "thread_id": "",
            "dm_item_id": f"followup:{cid}",
            "timestamp": ts_us,
        }
        try:
            queue.put_nowait(event)
            enqueued += 1
            print(f"ig followup: enqueued from @{user} on {media_pk}: {text[:80]!r}")
        except Exception as exc:  # noqa: BLE001
            print(f"ig followup: enqueue failed: {exc!r}")

    # Bump last-seen even if we found nothing (proves we polled).
    if newest_seen > last_seen:
        with _tracked_lock:
            _tracked[str(media_pk)] = newest_seen
            _save_tracked(dict(_tracked))
    return enqueued


def _run_loop() -> None:
    global _tracked
    with _tracked_lock:
        _tracked = _load_tracked()
        print(f"ig followup: thread loop starting "
              f"(tracking {len(_tracked)} media, interval={_interval_s()}s)")

    while True:
        time.sleep(_interval_s())
        handles = _get_handles()
        if not handles:
            continue
        _prune_expired()

        # Snapshot the media list so we don't hold the lock during HTTP.
        with _tracked_lock:
            media_list = list(_tracked.keys())
        if not media_list:
            continue

        try:
            total = 0
            for pk in media_list:
                total += _poll_one(
                    handles["client"], pk,
                    handles["followed_ids"], handles["queue"],
                )
            if total:
                print(f"ig followup: cycle enqueued {total} follow-up(s) "
                      f"across {len(media_list)} media")
        except Exception as exc:  # noqa: BLE001
            print(f"ig followup: cycle crashed: {exc!r}")
            traceback.print_exc()


def start_thread() -> None:
    global _thread
    with _thread_lock:
        if _thread and _thread.is_alive():
            print("ig followup: thread already running, skipping start")
            return
        _thread = threading.Thread(
            target=_run_loop, name="ig-followup", daemon=True,
        )
        _thread.start()
        print(f"ig followup: started daemon thread "
              f"(interval={_interval_s()}s, ttl={_ttl_s()}s)")
