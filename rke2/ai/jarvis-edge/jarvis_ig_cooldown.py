"""jarvis_ig_cooldown — shared IG throttle backoff state.

Why this exists:
    instagrapi raises FeedbackRequired / PleaseWaitFewMinutes when the
    account hits IG's anti-spam limits. The original failure mode was
    that the comment responder caught the exception, logged it, and
    immediately retried on the next cycle — each retry refreshes IG's
    rate-limit clock and extends the soft-block. We need *every* IG
    API caller to honour a single process-wide cooldown.

Contract:
    >>> import jarvis_ig_cooldown as ig_cd
    >>> if ig_cd.is_cooling_down():
    ...     return  # skip work this cycle
    >>> try:
    ...     result = client.media_comments(media_pk, amount=8)
    >>> except FeedbackRequired as e:
    ...     ig_cd.record_throttle(e)
    ...     return None
    >>> # On a successful call, no action — record_clean() is optional.

Tunables (env):
    IG_COOLDOWN_PATH         persistence path (default /state/ig_cooldown.json)
    IG_COOLDOWN_BASE_S       first-hit cooldown in seconds (default 1800 = 30m)
    IG_COOLDOWN_MAX_S        cap for the exponential backoff (default 86400 = 24h)
    IG_COOLDOWN_DECAY_S      consecutive-hits counter decays to zero after this
                             many seconds without a hit (default 14400 = 4h)

Persisted state schema:
    {"until_ts": <epoch>, "consecutive_hits": <int>, "last_hit_ts": <epoch>,
     "last_reason": <str>}
"""
from __future__ import annotations

import json
import os
import threading
import time
from typing import Any

_lock = threading.Lock()


def _path() -> str:
    return os.environ.get("IG_COOLDOWN_PATH", "/state/ig_cooldown.json")


def _base_s() -> int:
    return int(os.environ.get("IG_COOLDOWN_BASE_S", "1800"))


def _max_s() -> int:
    return int(os.environ.get("IG_COOLDOWN_MAX_S", "86400"))


def _decay_s() -> int:
    return int(os.environ.get("IG_COOLDOWN_DECAY_S", "14400"))


def _load() -> dict:
    try:
        with open(_path()) as f:
            return json.load(f)
    except (OSError, ValueError):
        return {}


def _save(state: dict) -> None:
    p = _path()
    tmp = p + ".tmp"
    try:
        with open(tmp, "w") as f:
            json.dump(state, f)
        os.replace(tmp, p)
    except OSError as exc:
        print(f"ig cooldown: persist failed: {exc!r}")


def is_cooling_down() -> bool:
    """True when we should refuse to make IG API calls."""
    with _lock:
        s = _load()
        return float(s.get("until_ts", 0)) > time.time()


def cooldown_remaining_s() -> int:
    """Seconds until cooldown expires (0 if not cooling)."""
    with _lock:
        s = _load()
        rem = float(s.get("until_ts", 0)) - time.time()
        return max(0, int(rem))


def record_throttle(exc: BaseException) -> None:
    """Mark IG as throttled. Doubles consecutive_hits if a recent prior
    hit was recorded; otherwise resets to 1. The next-allowed timestamp
    grows as base * 2^(hits-1), capped at max."""
    reason = type(exc).__name__
    now = time.time()
    with _lock:
        s = _load()
        last = float(s.get("last_hit_ts", 0))
        hits = int(s.get("consecutive_hits", 0) or 0)
        # If the previous hit was long enough ago that consecutive_hits
        # should have decayed, start fresh.
        if last and (now - last) > _decay_s():
            hits = 0
        hits = min(hits + 1, 12)  # 2^12 is plenty even if max wasn't set
        delay = min(_max_s(), _base_s() * (2 ** (hits - 1)))
        until = now + delay
        s.update({
            "until_ts": until,
            "consecutive_hits": hits,
            "last_hit_ts": now,
            "last_reason": reason,
        })
        _save(s)
    print(f"ig cooldown: {reason} → sleeping {int(delay)}s "
          f"(hit #{hits}, until {time.strftime('%H:%M:%S', time.localtime(until))})")


def record_clean() -> None:
    """Optional — call after a confirmed-clean IG API response. Decays
    consecutive_hits so the next throttle starts from a smaller base."""
    with _lock:
        s = _load()
        if not s:
            return
        hits = int(s.get("consecutive_hits", 0) or 0)
        if hits <= 0:
            return
        s["consecutive_hits"] = hits - 1
        _save(s)


def status() -> dict:
    """For diagnostic logging / MCP tool."""
    with _lock:
        s = _load()
        return {
            "cooling": is_cooling_down(),
            "remaining_s": cooldown_remaining_s(),
            "consecutive_hits": int(s.get("consecutive_hits", 0) or 0),
            "last_reason": s.get("last_reason", ""),
            "last_hit_ts": s.get("last_hit_ts", 0),
            "until_ts": s.get("until_ts", 0),
        }
