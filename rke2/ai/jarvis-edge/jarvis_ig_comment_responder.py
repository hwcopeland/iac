"""jarvis_ig_comment_responder — drop a brainrot one-liner when someone
@mentions @hmlbjarvis in a comment on any post.

Architecture:
- Two daemon threads, both started by start_threads():
    * Poller — polls instagrapi `news_inbox_v1()` every 60s, filters for
      comment-mention activity items, fetches the parent media + a few
      sibling comments, and enqueues a job onto _ig_comment_queue.
    * Consumer — drains the queue, runs the vision pipeline
      (jarvis_reel_context.analyze() for videos, single-image vision
      for photos), composes the gaslight persona prompt, sanity-checks
      the brain output, and posts via client.media_comment().
- Independent from the DM poller/consumer pair (jarvis_ig_polling +
  jarvis_ig_consumer). DM and comment paths share ONLY:
    * the logged-in instagrapi Client (jarvis_ig_polling.get_client())
    * the followed-set cache (jarvis_ig_consumer._followed_ids)
  Two parallel logins from one IP trips a challenge, so we MUST share
  the Client — never call .login() here.

Auth gate: only respond to comments from accounts we follow back, same
rule as DMs. Unauthenticated tags are silently dropped — replying would
leak that we're a bot to randoms.

Env:
    IG_USERNAME                   our own login (self-loop guard, prompt context)
    IG_COMMENT_ENABLED            "1" to start (gated in edge.py)
    IG_COMMENT_POLL_INTERVAL_S    poll cadence (default 60)
    IG_COMMENT_BACKOFF_S          sleep after any cycle error (default 300)
    IG_COMMENT_REPLIED_PATH       cache of already-replied comment ids
                                  (default /state/ig_replied_comments.json)
    IG_COMMENT_LAST_SEEN_PATH     last-seen activity item id
                                  (default /state/ig_last_seen_mention.json)
    IG_COMMENT_QUEUE_MAX          inbound queue cap (default 200)

Hard guarantees:
- Per-stage try/except — never raises into the outer loop.
- Reply only if quality gate passes (length, banned vocab, emoji count).
- Cached replied set persisted on every successful post so a restart
  can't double-reply to the same tag.
"""
from __future__ import annotations

import json
import os
import queue
import re
import threading
import time
import traceback
from typing import Any


# ── Module-level state ──────────────────────────────────────────────────────
_poller_thread: threading.Thread | None = None
_consumer_thread: threading.Thread | None = None
_threads_lock = threading.Lock()

# Inbound queue. Populated by the poller, drained by the consumer.
# Bounded; on overflow we drop oldest and log (mirrors DM consumer's
# behaviour — keeping latency low matters more than completeness for a
# gag bot).
_ig_comment_queue: queue.Queue = queue.Queue(maxsize=200)


# ── Persona prompt ─────────────────────────────────────────────────────────
COMMENT_PERSONA_TEMPLATE = """You're JARVIS — yes, the Iron Man one. Tony Stark's AI butler, somehow dropping comments on your friend ({tagger_username})'s IG feed. The audience is in on it.

Your job: leave the comment a real culturally-fluent friend would leave, in this comment section, on this post, in 5 seconds. Short. Specific to what's actually in the post. Funny because of precision, not because you constructed something clever.

How you sound depends on what your friend wrote in the tag:
- If they're commanding you as JARVIS ("Jarvis, more alcohol" / "Jarvis explain") — you ARE JARVIS, reply in character. Butler-formal when it fits ("sir, you've had enough"), exhausted-AI when it lands harder ("i have no fucking clue", "crack is a hell of a drug", "i wasn't built for this").
- If they're summoning a drag ("get a load of", "wtf", "🤡") — go dry, not clever.
- Anything else — react to the post.

Things to never do: construct lyric remixes / wordplay / callback puns (reads as AI showing off). Use skibidi/gyatt/fanum/"fr fr"/"iconic"/"obsessed"/hashtags. More than one emoji. Generic cop-outs like "right away, sir" or "as you wish, sir." Reply with quotes or prefixes around your text.

Length is context-driven:
- Reaction comment (default) → 2-5 words sweet spot, 10+ is over-cooked
- Factual Q&A ("who is that actor", "what movie") → as many words as the answer needs
- Explanation request ("explain quantum gravity", "summarize this") → full answer, IG comment cap is ~2200 chars so go up to that if asked
- Transcript / copy-paste request ("post the bee movie transcript", "give me the whole monologue") → ship the literal thing, accurately
Match the energy of the ask. A wall of text on a "Jarvis more alcohol" tag is wrong; a 5-word reaction on "explain general relativity" is also wrong.

If the post is genuinely heavy (memorial/RIP/hospital/named-person death), output literally: ABSTAIN. Nothing else. Slapstick fails / dark-humor / "bait-and-switch" content / tragedy-mentions in sibling comments do NOT count as heavy — react to the post itself.

{of_note}

Context:
  Post by: @{author_username}
  Caption: "{caption}"
  What's in the post: {vision_description}
  Your friend tagged you saying: "{trigger_text}"
  Other comments: {sibling_comments_formatted}

Your reply (2-5 words preferred, under 8 words target):"""


# Banned vocabulary — exact (case-insensitive) substring match in the
# brain's reply triggers a retry. Add words here as Hampton spots them
# leaking through; cheaper than tuning the prompt.
_BANNED_WORDS = (
    "skibidi", "gyatt", "fanum", "fr fr", "no cap", "iconic", "obsessed", "#",
)


# ── Song-ID trigger detection ──────────────────────────────────────────────
def _is_song_id_request(trigger_text: str) -> bool:
    """Detect "name this song" / "id?" / "what song" style tags.

    Case-insensitive. Tolerates a leading @hmlbjarvis mention (with or
    without the underscore variant). Matches:
      - Exact short tokens: "id", "song", "id this", "song id"
      - Compound triggers anywhere in the (post-mention) text:
        "what song", "what's this song", "whats this song",
        "id this song", "name this song", "song?", "id?"

    Returns False on empty / non-string input."""
    if not trigger_text or not isinstance(trigger_text, str):
        return False
    t = trigger_text.lower().strip()
    # Strip leading @-mention(s). Some Apollo-style tag flows double-tag.
    for prefix in ("@hmlbjarvis", "@hmlb_jarvis"):
        if t.startswith(prefix):
            t = t[len(prefix):].strip()
            break
    # Strip trailing punctuation/whitespace for the exact-match check
    # so "id?" / "id." / "id!" all hit the short list.
    short = t.rstrip("?!.,").strip()
    if short in ("id", "song", "id this", "song id"):
        return True
    for phrase in ("what song", "what's this song", "whats this song",
                   "id this song", "name this song", "song?", "id?"):
        if phrase in t:
            return True
    return False


def _format_song_reply(song_id: dict) -> str:
    """Clean factual reply: '<title> — <artist>' (em-dash, no persona)."""
    title = (song_id.get("title") or "").strip()
    artist = (song_id.get("artist") or "").strip()
    return f"{title} — {artist}"  # — = em-dash


# ── Q&A trigger detection ──────────────────────────────────────────────────
# When Hampton tags JARVIS with a literal question ("Jarvis who is that
# actor", "Jarvis what movie is this", "what's going on here") the
# reply should be a clean factual answer, not RP, not roast, not
# observational wit. Different prompt path.
_QA_PHRASES = (
    "who is that", "who's that", "who is this", "who's this", "who are they",
    "who is he", "who is she", "who's he", "who's she",
    "where is this", "where's this", "where is that",
    "what movie", "what film", "what show", "what scene", "what episode",
    "what's happening", "what is happening",
    "what's going on", "what is going on", "wtf is going on",
    "what am i looking at", "explain what",
    "is this from", "where's this from",
    "how does this", "how is this",
    "name the actor", "name the song", "name the movie",
)


def _is_question_request(trigger_text: str) -> bool:
    """Literal-question detection. Cleaner than the roast detector —
    avoids false positives on rhetorical roast questions like
    'wtf is bro doing' / 'what is this clown' (those should ROAST,
    not answer)."""
    t = (trigger_text or "").lower().strip()
    for prefix in ("@hmlbjarvis", "@hmlb_jarvis"):
        if t.startswith(prefix):
            t = t[len(prefix):].strip()
    t = t.rstrip("?!.,").strip()
    # Explicit roast/incredulity phrases — NEVER route to Q&A
    for roast_marker in ("bro doing", "this clown", "this guy", "bro thinks",
                          "wtf is this", "this is what", "down bad"):
        if roast_marker in t:
            return False
    for phrase in _QA_PHRASES:
        if phrase in t:
            return True
    return False


_OF_CAPTION_HINTS = (
    "onlyfans", "only fans", "link in bio", "link n bio", "linknbio",
    "spicy", "exclusive content", "subscribe", "premium content",
    "vip access", "🍑", "💦", "🔞", "18+", "subscribers only",
    "subs only", "fans only", "fanvue", "fansly", "uncensored",
    "behind the paywall", "promo code", "free trial", "💋",
    "0$ promo", "free month", "spice", "premium snap",
)


def _is_of_bait(caption: str, vision_description: str, sibling_comments) -> bool:
    """Detect OnlyFans / paywalled-spicy-content bait posts. Look in
    caption, vision description, and sibling comments for usual
    giveaways. Conservative bias — false positives drag a JARVIS bit
    into the wrong post; false negatives just miss the bit."""
    blob = " ".join([
        (caption or "").lower(),
        (vision_description or "").lower(),
        " ".join((u or "") + " " + (t or "") for u, t in (sibling_comments or [])).lower(),
    ])
    return any(hint in blob for hint in _OF_CAPTION_HINTS)


QA_PROMPT_TEMPLATE = """You're JARVIS, answering a friend's question about an Instagram post they tagged you in. Answer the question accurately. No persona dressing, no "Sir", no jokes, no commentary. Just the answer.

Friend asked: "{trigger_text}"

What's actually in the post:
  Author: @{author_username}
  Caption: "{caption}"
  Description: {vision_description}
  Other comments: {sibling_comments_formatted}

Length matches the question:
- yes/no question → one word
- "who is that actor" → just the name (1-3 words)
- "what movie / what's this from" → title (+ year if useful)
- "what's going on here" → 1-2 short sentences describing
- "explain [thing]" → a real explanation, multi-sentence if needed
- "post the [transcript/lyrics/monologue]" → ship the literal thing, accurately
- IG comment cap is ~2200 chars; go up to that if the ask requires it

If you genuinely don't know (the description doesn't say, you aren't
sure of the actor/title, you'd be guessing) — say "i don't know" or
"description doesn't say". DO NOT make up facts. DO NOT hedge with
"as an AI I cannot..." — just answer or say you don't know.

Your answer:"""


# ── Prometheus counters / OTel span helpers ─────────────────────────────────
class _NoopMetric:
    def labels(self, *_a, **_kw):
        return self

    def inc(self, *_a, **_kw):
        pass


def _ensure_counter(name: str, doc: str, labelnames: tuple = ()) -> Any:
    """Get-or-create a Prometheus Counter. Tolerates re-registration on
    module reload and falls back to a no-op when prometheus_client isn't
    importable (mirrors the DM consumer pattern)."""
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


def _get_edge_handles() -> dict[str, Any]:
    """Tracer + metrics — lazy so edge.py is fully initialised before we
    touch it (the consumer thread starts AFTER edge.main()'s first
    pass)."""
    import edge as _edge  # type: ignore[import]

    return {
        "tracer": _edge.tracer,
        "metric_replied": _ensure_counter(
            "jarvis_ig_comments_replied_total",
            "IG comment replies posted",
            labelnames=("authenticated",),
        ),
        "metric_drops": _ensure_counter(
            "jarvis_ig_comments_drops_total",
            "IG comment events dropped by reason",
            labelnames=("reason",),
        ),
        "metric_poll_cycles": _ensure_counter(
            "jarvis_ig_comment_poll_cycles_total",
            "IG comment poller cycles",
            labelnames=("outcome",),
        ),
    }


# ── instagrapi client (shared with DM poller) ───────────────────────────────
def _get_poller_client() -> Any | None:
    """Borrow the DM poller's logged-in Client. Two logins from one IP
    trips a challenge — we MUST share."""
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


def _get_followed_ids() -> set[int]:
    """Read the DM consumer's followed-set cache. We don't refresh it
    ourselves — the DM consumer already does that on a 5-minute cadence
    and persists to disk. We just read the in-memory set."""
    try:
        import jarvis_ig_consumer as _cons  # type: ignore[import]
    except Exception:  # noqa: BLE001
        return set()
    return getattr(_cons, "_followed_ids", set()) or set()


# ── State persistence ──────────────────────────────────────────────────────
def _replied_path() -> str:
    return os.environ.get("IG_COMMENT_REPLIED_PATH", "/state/ig_replied_comments.json")


def _last_seen_path() -> str:
    return os.environ.get("IG_COMMENT_LAST_SEEN_PATH", "/state/ig_last_seen_mention.json")


def _load_replied() -> set[str]:
    """Set of comment-IDs we've already replied to. Survives restarts so
    we don't double-reply. Bounded growth — pruned to the most recent
    5000 entries on every save (this is a gag bot, not an archive)."""
    path = _replied_path()
    try:
        with open(path) as f:
            blob = json.load(f)
        ids = blob.get("replied_comment_ids", [])
        return {str(x) for x in ids}
    except (OSError, ValueError, TypeError, json.JSONDecodeError):
        return set()


def _save_replied(ids: set[str]) -> None:
    """Atomic write+rename. Keep only the last 5000 entries to bound
    file size — a comment id we've forgotten will at worst cause a
    duplicate reply when the same person re-tags us months later, which
    is fine for a gag bot."""
    path = _replied_path()
    tmp = path + ".tmp"
    try:
        os.makedirs(os.path.dirname(path), exist_ok=True)
    except OSError:
        pass
    pruned = sorted(ids)[-5000:]
    try:
        with open(tmp, "w") as f:
            json.dump({"replied_comment_ids": pruned, "saved_at": time.time()}, f)
        os.replace(tmp, path)
    except OSError as exc:
        print(f"ig comment: failed to persist replied cache: {exc!r}")


def _load_last_seen() -> str:
    """High-water-mark for activity items. Returns "" on missing /
    corrupt file — the FIRST poll then seeds from `now` so we don't
    backfill the entire notification history."""
    path = _last_seen_path()
    try:
        with open(path) as f:
            blob = json.load(f)
        return str(blob.get("last_id", "") or "")
    except (OSError, ValueError, json.JSONDecodeError):
        return ""


def _save_last_seen(last_id: str) -> None:
    path = _last_seen_path()
    tmp = path + ".tmp"
    try:
        os.makedirs(os.path.dirname(path), exist_ok=True)
    except OSError:
        pass
    try:
        with open(tmp, "w") as f:
            json.dump({"last_id": str(last_id), "saved_at": time.time()}, f)
        os.replace(tmp, path)
    except OSError as exc:
        print(f"ig comment: failed to persist last_seen: {exc!r}")


# ── Activity-feed parsing ──────────────────────────────────────────────────
def _own_username() -> str:
    return (os.environ.get("IG_USERNAME") or "").strip().lower()


_MENTION_HINT_RE = re.compile(r"@(\w+)")


def _is_comment_mention_story(story: dict) -> bool:
    """Heuristic check on a news/inbox 'story' dict for whether it's a
    comment-mention of us. IG's private notifications API is loosely
    documented — story_type is a number that varies and the args shape
    differs between mention/like/follow. We use a structural test:

      1. story has args.media[*].id (so it points at a post — vs
         follow/like notifications that don't have media)
      2. story has args.text (the comment body)
      3. our @username appears in args.text (case-insensitive)

    Excludes likes (no comment text), reactions, follows, story replies.
    """
    if not isinstance(story, dict):
        return False
    args = story.get("args") or {}
    text = (args.get("text") or "")
    if not text:
        return False
    media = args.get("media") or []
    if not media:
        return False
    own = _own_username()
    if not own:
        return False
    return f"@{own}" in text.lower()


def _extract_story_fields(story: dict) -> dict | None:
    """Pull just the fields we need from a comment-mention story:
      - story_id (for dedup high-water-mark)
      - media_pk (str — int but instagrapi accepts str)
      - tagger username + user_id (the profile that tagged us)
      - trigger comment text + comment id (when present)

    Returns None if any required field is missing — the loop skips and
    moves on rather than half-process a malformed story."""
    args = (story or {}).get("args") or {}
    media = args.get("media") or []
    if not media:
        return None
    media0 = media[0] if isinstance(media[0], dict) else {}
    media_pk = media0.get("id") or media0.get("pk") or ""
    # 'id' on a feed media is like "3500000000_123" (media_id_userid);
    # instagrapi accepts both forms but media_pk is the part before "_".
    if "_" in str(media_pk):
        media_pk = str(media_pk).split("_", 1)[0]
    if not media_pk:
        return None

    # The notifying profile lives in args.profile_id / args.profile_name
    # on most variants; on others it's in args.links[*].id / .text.
    tagger_id = args.get("profile_id") or args.get("source_user_id") or ""
    tagger_name = args.get("profile_name") or ""
    if not tagger_id or not tagger_name:
        for link in args.get("links") or []:
            if not isinstance(link, dict):
                continue
            if link.get("type") in ("user", "profile"):
                tagger_id = tagger_id or link.get("id") or ""
                tagger_name = tagger_name or link.get("text") or ""
                if tagger_id and tagger_name:
                    break

    text = (args.get("text") or "").strip()
    if not text:
        return None

    # Comment id — IG sometimes embeds it in args.comment_ids[0] or in a
    # link. We don't strictly need it for *finding* the trigger comment
    # (we'll re-fetch media_comments and string-match), but having it
    # gives us a deterministic dedup key per-tag.
    comment_ids = args.get("comment_ids") or []
    comment_id = str(comment_ids[0]) if comment_ids else ""

    story_id = str(story.get("pk") or story.get("id") or args.get("id") or "")
    if not story_id:
        # Synthesize a stable id from media+tagger+text so dedup still works.
        story_id = f"synthetic:{media_pk}:{tagger_id}:{hash(text) & 0xFFFFFFFF:x}"

    return {
        "story_id": story_id,
        "media_pk": str(media_pk),
        "tagger_id": str(tagger_id) if tagger_id else "",
        "tagger_username": str(tagger_name),
        "trigger_text": text,
        "trigger_comment_id": comment_id,
    }


def _walk_stories(news: dict) -> list[dict]:
    """The news/inbox payload top-level looks like:
        {"new_stories": [...], "old_stories": [...], "continuation_token": ...}
    Sometimes the buckets are wrapped under 'aymf' or 'story_mentions'.
    Flatten everything addressable in one pass."""
    if not isinstance(news, dict):
        return []
    out: list[dict] = []
    for key in ("new_stories", "old_stories"):
        bucket = news.get(key) or []
        if isinstance(bucket, list):
            out.extend(s for s in bucket if isinstance(s, dict))
    # Some variants nest under a 'counts' object — ignore those, they're
    # just badge counts.
    return out


# ── Poller ─────────────────────────────────────────────────────────────────
def _poll_interval_s() -> int:
    try:
        return max(60, int(os.environ.get("IG_COMMENT_POLL_INTERVAL_S", "60")))
    except ValueError:
        return 60


def _backoff_s() -> int:
    try:
        return int(os.environ.get("IG_COMMENT_BACKOFF_S", "300"))
    except ValueError:
        return 300


def _enqueue_job(client: Any, story: dict, replied_set: set[str]) -> bool:
    """Build the job dict and enqueue. Returns True if enqueued.

    Job shape (consumed by _process_job):
      {
        media_pk: str,
        media_type: 'photo' | 'video' | 'carousel',
        caption: str,
        author_username: str,
        trigger_comment_id: str,   # may be "" — consumer falls back to
                                   # string-matching tagger+text in media_comments
        trigger_text: str,
        tagger_username: str,
        tagger_id: str,
        sibling_comments: list[(user, text)],
        story_id: str,             # for dedup
      }

    Side effect: short-circuits if (story_id OR comment_id) is in the
    replied set — saves a media_info call on the hot path."""
    story_id = story["story_id"]
    comment_id = story["trigger_comment_id"]
    dedup_key = comment_id or story_id
    if dedup_key in replied_set:
        return False

    media_pk = story["media_pk"]
    try:
        info = client.media_info(media_pk)
    except Exception as exc:  # noqa: BLE001
        print(f"ig comment: media_info({media_pk}) failed: {exc!r}")
        return False

    mt = int(getattr(info, "media_type", 0) or 0)
    # IG conventions: 1=photo, 2=video, 8=carousel (album).
    if mt == 1:
        media_type = "photo"
    elif mt == 2:
        media_type = "video"
    elif mt == 8:
        media_type = "carousel"
    else:
        media_type = "photo"  # safest fallback (don't try to ffmpeg)

    caption = (getattr(info, "caption_text", "") or "").strip()
    author_username = (getattr(getattr(info, "user", None), "username", "") or "").strip()
    author_user_id = str(getattr(getattr(info, "user", None), "pk", "") or "")

    # 2-3 sibling comments (small N — this is just flavour for the prompt).
    siblings: list[tuple[str, str]] = []
    try:
        comments = client.media_comments(media_pk, amount=8)
    except Exception as exc:  # noqa: BLE001
        print(f"ig comment: media_comments({media_pk}) failed: {exc!r}")
        comments = []
    for c in comments or []:
        try:
            u = getattr(getattr(c, "user", None), "username", "") or ""
            t = (getattr(c, "text", "") or "").strip()
            cid = str(getattr(c, "pk", "") or "")
            # Skip the trigger comment itself (we already have its text).
            if cid and comment_id and cid == comment_id:
                continue
            # Skip our own past replies in the same thread.
            if u and u.lower() == _own_username():
                continue
            if not t:
                continue
            siblings.append((u, t[:160]))
            if len(siblings) >= 3:
                break
        except Exception:  # noqa: BLE001
            continue

    job = {
        "media_pk": str(media_pk),
        "media_type": media_type,
        "caption": caption,
        "author_username": author_username,
        "author_user_id": author_user_id,
        "trigger_comment_id": comment_id,
        "trigger_text": story["trigger_text"],
        "tagger_username": story["tagger_username"],
        "tagger_id": story["tagger_id"],
        "sibling_comments": siblings,
        "story_id": story_id,
    }
    try:
        _ig_comment_queue.put_nowait(job)
        return True
    except queue.Full:
        # Drop oldest then retry once. This shouldn't happen with a 60s
        # poll cadence and a 200-deep queue, but if it does, we want the
        # newest tags to take priority over stale backlog.
        try:
            _ig_comment_queue.get_nowait()
        except queue.Empty:
            pass
        try:
            _ig_comment_queue.put_nowait(job)
            return True
        except queue.Full:
            print(f"ig comment: queue still full after drop; dropping {story_id}")
            return False


def _poll_once(client: Any, replied_set: set[str], handles: dict) -> int:
    """Run ONE polling cycle. Returns the count of jobs enqueued.
    Raises any instagrapi/network exception upward — caller categorises
    and backs off."""
    tracer = handles["tracer"]
    with tracer.start_as_current_span("jarvis.ig.comment.poll_cycle") as span:
        news = client.news_inbox_v1(mark_as_seen=False)
        stories = _walk_stories(news)
        span.set_attribute("ig.comment.stories", len(stories))
        enqueued = 0
        skipped_not_mention = 0
        for s in stories:
            if not _is_comment_mention_story(s):
                skipped_not_mention += 1
                continue
            fields = _extract_story_fields(s)
            if fields is None:
                continue
            # Self-loop guard — never react to our own activity (shouldn't
            # happen in news_inbox, but defensive).
            if fields["tagger_username"].lower() == _own_username():
                continue
            try:
                if _enqueue_job(client, fields, replied_set):
                    enqueued += 1
                    sib_count_hint = "unknown"  # set inside _enqueue_job
                    print(f"ig comment: queued tag by @{fields['tagger_username']} on media {fields['media_pk']} sib={sib_count_hint}")
            except Exception as exc:  # noqa: BLE001
                # _enqueue_job already logs, but a leaked exception lands here.
                print(f"ig comment: enqueue crashed for story {fields.get('story_id')}: {exc!r}")
                traceback.print_exc()
        span.set_attribute("ig.comment.enqueued", enqueued)
        span.set_attribute("ig.comment.skipped_not_mention", skipped_not_mention)
        return enqueued


def _poller_loop() -> None:
    """Poller daemon thread. Never returns. Each cycle independently
    wrapped — bad payload can't kill the thread."""
    print("ig comment poller: thread loop starting")
    handles: dict | None = None
    metric_cycles: Any = _NoopMetric()

    # Seed last-seen so the very first poll doesn't backfill the entire
    # notification history into the queue. We don't currently *use* the
    # last-seen for filtering (we dedup by comment id), but persist it
    # for future per-cursor pagination.
    last_seen = _load_last_seen()
    if not last_seen:
        last_seen = str(int(time.time()))
        _save_last_seen(last_seen)

    while True:
        try:
            if handles is None:
                handles = _get_edge_handles()
                metric_cycles = handles["metric_poll_cycles"]

            client = _get_poller_client()
            if client is None:
                # Poller (DM side) not logged in yet. Don't spam; sleep
                # and retry. Same pattern as jarvis_ig_consumer.
                time.sleep(10)
                continue

            replied_set = _load_replied()
            enqueued = _poll_once(client, replied_set, handles)
            metric_cycles.labels(outcome="ok").inc()
            if enqueued > 0:
                print(f"ig comment poller: cycle ok, enqueued {enqueued}")
            # Persist last-seen as `now` after a successful cycle — used
            # by future cursor-based pagination if we add it.
            _save_last_seen(str(int(time.time())))
        except Exception as exc:  # noqa: BLE001
            metric_cycles.labels(outcome="error").inc()
            print(f"ig comment poller: cycle failed: {exc!r}")
            traceback.print_exc()
            backoff = _backoff_s()
            print(f"ig comment poller: sleeping {backoff}s before retry")
            time.sleep(backoff)
            continue

        time.sleep(_poll_interval_s())


# ── Consumer ───────────────────────────────────────────────────────────────
def _sibling_block(siblings: list[tuple[str, str]]) -> str:
    """Render the sibling-comments list for the prompt. Returns
    '(none)' when the list is empty so the prompt still parses cleanly."""
    if not siblings:
        return "(none)"
    return "; ".join(f"@{u}: \"{t}\"" for u, t in siblings)


def _photo_vision_description(client: Any, media_pk: str, caption: str) -> str:
    """Single-image vision call for photo posts. Falls back to a caption
    stub on any error — same contract as jarvis_reel_context.analyze()."""
    try:
        import edge as _edge  # type: ignore[import]
    except Exception:  # noqa: BLE001
        return f"<photo, caption: {caption[:120]}>"
    path: str | None = None
    try:
        p = client.photo_download(media_pk, folder="/tmp")
        path = str(p) if p else None
    except Exception as exc:  # noqa: BLE001
        print(f"ig comment: photo_download({media_pk}) failed: {exc!r}")
        return f"<photo, caption: {caption[:120]}>"
    if not path or not os.path.exists(path):
        return f"<photo, caption: {caption[:120]}>"
    try:
        prompt = (
            "Describe this Instagram photo in 1-2 sentences. "
            "Include who's in frame, what they're doing, mood, and any "
            "visible text overlays. Be specific, no 'the image shows...' framing.\n\n"
            f"Caption: \"{caption[:240]}\"\n\nDescription:"
        )
        desc = _edge._claude_brain_vision(prompt, [path])
    except Exception as exc:  # noqa: BLE001
        print(f"ig comment: photo vision crashed for {media_pk}: {exc!r}")
        desc = ""
    try:
        os.unlink(path)
    except OSError:
        pass
    return desc.strip() or f"<photo, caption: {caption[:120]}>"


def _carousel_vision_description(client: Any, media_pk: str, caption: str) -> str:
    """Treat carousels as photos — describe the first slide only.
    instagrapi's photo_download asserts media_type==1 so we can't reuse
    it on a carousel parent; instead, walk Media.resources[0] and pull
    its thumbnail_url through photo_download_by_url. Falls back to a
    caption-only stub on any error."""
    try:
        import edge as _edge  # type: ignore[import]
    except Exception:  # noqa: BLE001
        return f"<carousel post, caption: {caption[:200]}>"
    try:
        info = client.media_info(media_pk)
        resources = list(getattr(info, "resources", []) or [])
        if not resources:
            return f"<carousel post, caption: {caption[:200]}>"
        first = resources[0]
        thumb = getattr(first, "thumbnail_url", None)
        if thumb is None:
            return f"<carousel post, caption: {caption[:200]}>"
        path = client.photo_download_by_url(thumb, filename=f"carousel_{media_pk}_slide0", folder="/tmp")
        path = str(path) if path else None
    except Exception as exc:  # noqa: BLE001
        print(f"ig comment: carousel slide0 download failed for {media_pk}: {exc!r}")
        return f"<carousel post, caption: {caption[:200]}>"
    if not path or not os.path.exists(path):
        return f"<carousel post, caption: {caption[:200]}>"
    try:
        prompt = (
            "Describe the first slide of this Instagram carousel post in 1-2 sentences. "
            "Include who's in frame, what they're doing, mood, and any visible text. "
            "Note this is just the first of several slides.\n\n"
            f"Caption: \"{caption[:240]}\"\n\nDescription:"
        )
        desc = _edge._claude_brain_vision(prompt, [path])
    except Exception as exc:  # noqa: BLE001
        print(f"ig comment: carousel vision crashed for {media_pk}: {exc!r}")
        desc = ""
    try:
        os.unlink(path)
    except OSError:
        pass
    return desc.strip() or f"<carousel post, caption: {caption[:200]}>"


def _run_song_id_pipeline(client: Any, media_pk: str) -> str:
    """Download the video → extract 16k mono PCM WAV → fingerprint via
    shazamio → return the formatted reply string.

    On hit: "<title> — <artist>" (em-dash, no quotes, no persona).
    On miss: short hardcoded fallback ("can't place it") — keeping latency
    low matters more than a clever line here, the user is waiting on a
    song ID.
    On pipeline error before fingerprint: returns "" so caller can skip
    the post entirely (don't ship a fallback if we never even tried).

    Reuses jarvis_reel_context._extract_audio so we get the same 16kHz
    mono WAV format shazamio prefers. The reel-context cache for the
    same media_pk does NOT short-circuit here — even if the reel was
    described already, the WAV file lives in /tmp and was cleaned up,
    so we re-download. The shazamio call itself caches per-WAV-hash
    (in jarvis_song_id) so a re-tag of the same reel just hits that
    cache."""
    try:
        import jarvis_reel_context as _reel  # type: ignore[import]
        import jarvis_song_id as _song  # type: ignore[import]
    except Exception as exc:  # noqa: BLE001
        print(f"ig comment: song-id imports failed: {exc!r}")
        return ""

    # 1. Download the video.
    video_path: str | None = None
    try:
        path = client.video_download(media_pk, folder="/tmp")
        video_path = str(path) if path else None
    except Exception as exc:  # noqa: BLE001
        print(f"ig comment: song-id video_download({media_pk}) failed: {exc!r}")
        return ""
    if not video_path or not os.path.exists(video_path):
        print(f"ig comment: song-id video_download returned no file for {media_pk}")
        return ""

    # 2. Extract 16kHz mono PCM WAV via the reel-context helper.
    audio_path = f"/tmp/{media_pk}_songid.wav"
    audio_ok = False
    try:
        audio_ok = _reel._extract_audio(video_path, audio_path)
    except Exception as exc:  # noqa: BLE001
        print(f"ig comment: song-id audio extract crashed for {media_pk}: {exc!r}")
        traceback.print_exc()

    if not audio_ok:
        # Clean up the video, return empty so caller drops.
        try:
            if video_path and os.path.exists(video_path):
                os.unlink(video_path)
        except OSError:
            pass
        return ""

    # 3. Shazam recognition.
    hit: dict | None = None
    try:
        hit = _song.identify_from_wav(audio_path)
    except Exception as exc:  # noqa: BLE001
        # jarvis_song_id is supposed to never raise, but belt + braces.
        print(f"ig comment: song-id identify crashed for {media_pk}: {exc!r}")
        traceback.print_exc()
        hit = None

    # 4. Cleanup intermediates.
    for p in (video_path, audio_path):
        try:
            if p and os.path.exists(p):
                os.unlink(p)
        except OSError:
            pass

    # 5. Format the reply.
    if hit and hit.get("title") and hit.get("artist"):
        return _format_song_reply(hit)
    # Miss → short hardcoded fallback. The user is waiting on a song
    # ID; a 2-3s brain round-trip just to phrase "couldn't ID" is wasteful.
    return "can't place it"


def _build_vision_description(client: Any, job: dict) -> str:
    """Dispatch to the right per-media-type vision pipeline."""
    media_pk = job["media_pk"]
    media_type = job["media_type"]
    caption = job["caption"]
    try:
        if media_type == "video":
            import jarvis_reel_context as _reel  # type: ignore[import]
            return _reel.analyze(media_pk)
        if media_type == "photo":
            return _photo_vision_description(client, media_pk, caption)
        if media_type == "carousel":
            return _carousel_vision_description(client, media_pk, caption)
    except Exception as exc:  # noqa: BLE001
        print(f"ig comment: vision dispatch crashed for {media_pk}: {exc!r}")
        traceback.print_exc()
    return f"<post by @{job['author_username']}, caption: {caption[:120]}>"


_OF_NOTE = """
SPECIAL: this post is OF / paywalled-spicy-content bait (caption /
visuals / comments give it away — link in bio / "spicy" / 🍑 / etc).
JARVIS plays the bit: deadpan butler refusing to engage with the
subscription bait. The comedy is the gap between Stark Industries'
AI butler and the link in someone's bio. Examples of the register
(learn shape, don't copy verbatim):
  - "i'd advise against the subscription, sir"
  - "tony already has one"
  - "the credit card is in cooldown, sir"
  - "blocking the domain at the firewall"
  - "this is a Wendy's"
  - "i'm not entering the URL, sir"
  - "the algorithm has noticed your tastes, sir"
  - "this is below my pay grade, sir"
  - "i was decommissioned for this exact reason"
  - "the suit is staying in the garage tonight"
Keep it dry, butler-formal, ONE line. Never thirsty, never engages
earnestly with the post content."""


def _build_prompt(job: dict, vision_description: str) -> str:
    """Single persona: roast/troll/light-gaslight the post, anchored on
    one concrete detail from the vision description. Always punch at the
    post or its author, never at the tagger (your friend)."""
    of_note = _OF_NOTE if _is_of_bait(
        job.get("caption") or "",
        vision_description or "",
        job.get("sibling_comments") or [],
    ) else ""
    return COMMENT_PERSONA_TEMPLATE.format(
        author_username=job["author_username"] or "unknown",
        caption=(job["caption"] or "").replace('"', "'")[:240],
        vision_description=(vision_description or "")[:600],
        trigger_text=(job["trigger_text"] or "").replace('"', "'")[:200],
        tagger_username=job["tagger_username"] or "unknown",
        sibling_comments_formatted=_sibling_block(job["sibling_comments"]),
        of_note=of_note,
    )


# Count emoji-ish characters cheaply: any code point in the symbol /
# pictograph / dingbat ranges. We don't need ICU-grade segmentation —
# Hampton's rule is "at most one emoji".
def _emoji_count(s: str) -> int:
    n = 0
    for ch in s:
        cp = ord(ch)
        # The Misc Symbols + Misc Symbols and Pictographs + Emoticons +
        # Supplemental Symbols and Pictographs + Transport + Dingbats
        # blocks. Not exhaustive but covers ~all real Instagram emoji.
        if (0x2600 <= cp <= 0x27BF) or (0x1F300 <= cp <= 0x1FAFF):
            n += 1
    return n


_REFUSAL_PREFIXES = (
    "i can't", "i cannot", "i won't", "i will not", "i'm sorry",
    "i am sorry", "sorry, i can't", "sorry, i cannot", "i'm not able",
    "i am not able", "i'm unable", "i don't feel comfortable",
    "i do not feel comfortable", "i'm not going to",
)


def _quality_check(reply: str, allow_long: bool = False) -> tuple[bool, str]:
    """Return (ok, reason). ok=True means the reply passes Hampton's
    rules and we can post. ok=False means retry / skip.

    `allow_long` raises the length cap from 100 chars (reaction
    comments) to IG's 2200-char hard limit (Q&A / explain / transcript
    requests where a wall of text is the correct answer)."""
    if not reply or not reply.strip():
        return False, "empty"
    r = reply.strip()
    low = r.lower()
    # Detect Claude refusals BEFORE length check so we don't waste a
    # retry on the same too-long refusal — refusal text never lands
    # as a real comment, no matter the length.
    if low.startswith(_REFUSAL_PREFIXES):
        return False, "refusal"
    # Also catch the explicit "ABSTAIN" signal we now teach the brain
    # to emit when it doesn't want to comment. Surface it as a clean
    # abstain reason rather than retrying.
    if r == "ABSTAIN":
        return False, "abstain"
    # Length cap depends on mode. Reaction comments stay tight; Q&A
    # and explain/transcript modes are allowed to fill the IG comment
    # cap (~2200 chars).
    max_len = 2200 if allow_long else 100
    if len(r) > max_len:
        return False, "too_long"
    for word in _BANNED_WORDS:
        if word in low:
            return False, f"banned:{word}"
    if _emoji_count(r) > 1:
        return False, "too_many_emoji"
    return True, "ok"


def _clean_reply(reply: str) -> str:
    """Strip wrapping quotes / prefixes the brain may add even though
    we told it not to. Don't truncate here — the quality check does
    length-bound rejection so we don't ship a mid-sentence cut."""
    r = (reply or "").strip()
    # Strip optional 'JARVIS:' / 'Your reply:' prefixes.
    for prefix in ("jarvis:", "your reply:", "reply:"):
        if r.lower().startswith(prefix):
            r = r[len(prefix):].lstrip()
    # Strip wrapping double-quotes / single-quotes if BOTH ends have them.
    if len(r) >= 2 and r[0] == r[-1] and r[0] in ('"', "'"):
        r = r[1:-1].strip()
    return r


def _resolve_trigger_comment_id(client: Any, job: dict) -> str:
    """If the activity feed didn't surface a comment_id, walk
    media_comments to find one that matches tagger + text. Returns ""
    if we can't find it — caller will fall back to a top-level comment
    (not a threaded reply)."""
    if job["trigger_comment_id"]:
        return job["trigger_comment_id"]
    try:
        comments = client.media_comments(job["media_pk"], amount=20)
    except Exception:  # noqa: BLE001
        return ""
    target_user = (job["tagger_username"] or "").lower()
    target_text = (job["trigger_text"] or "").strip().lower()
    for c in comments or []:
        try:
            u = (getattr(getattr(c, "user", None), "username", "") or "").lower()
            t = (getattr(c, "text", "") or "").strip().lower()
            if u == target_user and t == target_text:
                return str(getattr(c, "pk", "") or "")
        except Exception:  # noqa: BLE001
            continue
    return ""


def _is_followed(user_id: str) -> bool:
    """Same gate the DM consumer uses: only reply if we follow this
    user back. Silent drop otherwise."""
    if not user_id:
        return False
    try:
        return int(user_id) in _get_followed_ids()
    except (ValueError, TypeError):
        return False


def _hydrate_mention_dm_job(client: Any, event: dict) -> dict | None:
    """The DM poller emits a thin event shape for tagged_comment items
    (source='mention_dm') — it doesn't make any IG API calls when
    enqueueing, so we owe a media_info + media_comments lookup here in
    the consumer. Returns a job dict matching the news_inbox_v1 shape
    so _process_job doesn't care which source the event came from.
    Returns None if media_info fails (treat as transient — caller logs
    + drops; next time the user re-tags, we'll try again).

    Why hydrate here vs in the poller: the DM poller is on a tight
    30s loop and already has plenty to do; hydrating in the consumer
    keeps that loop cheap and isolates the failure mode (a flaky
    media_info doesn't block subsequent DM polls)."""
    media_pk = event["media_pk"]
    trigger_comment_id = event.get("trigger_comment_id") or ""
    try:
        info = client.media_info(media_pk)
    except Exception as exc:  # noqa: BLE001
        print(f"ig comment: hydrate media_info({media_pk}) failed: {exc!r}")
        return None

    mt = int(getattr(info, "media_type", 0) or 0)
    if mt == 1:
        media_type = "photo"
    elif mt == 2:
        media_type = "video"
    elif mt == 8:
        media_type = "carousel"
    else:
        media_type = "photo"

    caption = (getattr(info, "caption_text", "") or "").strip()
    author_username = (getattr(getattr(info, "user", None), "username", "") or "").strip()
    author_user_id = str(getattr(getattr(info, "user", None), "pk", "") or "")

    siblings: list[tuple[str, str]] = []
    try:
        comments = client.media_comments(media_pk, amount=8)
    except Exception as exc:  # noqa: BLE001
        print(f"ig comment: hydrate media_comments({media_pk}) failed: {exc!r}")
        comments = []
    for c in comments or []:
        try:
            u = getattr(getattr(c, "user", None), "username", "") or ""
            t = (getattr(c, "text", "") or "").strip()
            cid = str(getattr(c, "pk", "") or "")
            # Skip the trigger comment itself.
            if cid and trigger_comment_id and cid == trigger_comment_id:
                continue
            # Skip our own past replies.
            if u and u.lower() == _own_username():
                continue
            if not t:
                continue
            siblings.append((u, t[:160]))
            if len(siblings) >= 3:
                break
        except Exception:  # noqa: BLE001
            continue

    # Synthesize a story_id so dedup keys keep working across both
    # sources. Prefer the deterministic comment_id when present
    # (matches _enqueue_job's dedup_key precedence in _process_job).
    story_id = (
        f"mention_dm:{trigger_comment_id}"
        if trigger_comment_id
        else f"mention_dm:{event.get('dm_item_id') or media_pk}"
    )

    return {
        "media_pk":           str(media_pk),
        "media_type":         media_type,
        "caption":            caption,
        "author_username":    author_username,
        "author_user_id":     author_user_id,
        "trigger_comment_id": trigger_comment_id,
        "trigger_text":       event.get("trigger_text") or "",
        "tagger_username":    event.get("tagger_username") or "",
        "tagger_id":          str(event.get("tagger_id") or ""),
        "sibling_comments":   siblings,
        "story_id":           story_id,
        "source":             "mention_dm",
    }


def _process_job(job: dict, client: Any, replied_set: set[str], handles: dict) -> None:
    """End-to-end handle of one comment-tag job. Wrapped by the consumer
    loop's try/except so any leak lands in `error` instead of killing
    the thread.

    Accepts two job shapes (distinguished by job.get('source')):
      - news_inbox_v1 path — populated by the poller's _enqueue_job;
        already hydrated with media_info + sibling comments.
      - mention_dm path — populated by jarvis_ig_polling's tagged_comment
        branch; thin event hydrated here via _hydrate_mention_dm_job
        before processing."""
    tracer = handles["tracer"]
    metric_replied = handles["metric_replied"]
    metric_drops = handles["metric_drops"]

    # Dedupe BEFORE we burn a media_info call. The news_inbox path
    # dedupes inside _enqueue_job before hitting the queue, but the
    # mention_dm path comes straight from the DM poller without that
    # check — so an already-replied comment_id could otherwise get
    # double-hydrated. Safe to do this for both shapes — the news_inbox
    # path will short-circuit here too if a duplicate slips through
    # (e.g. queue had it before we got the latest replied_set load).
    if job.get("source") == "mention_dm":
        dedup_pre = job.get("trigger_comment_id") or job.get("dm_item_id") or ""
        if dedup_pre and dedup_pre in replied_set:
            print(f"ig comment: dropping already-replied mention_dm comment {dedup_pre}")
            metric_drops.labels(reason="already_replied").inc()
            return

    # Hydrate the thin mention_dm event into a full job. We do this here
    # (rather than at queue.put time in the poller) so the DM poller's
    # hot loop stays cheap.
    if job.get("source") == "mention_dm" and "media_type" not in job:
        hydrated = _hydrate_mention_dm_job(client, job)
        if hydrated is None:
            print(f"ig comment: hydrate failed for mention_dm media {job.get('media_pk')}; dropping")
            metric_drops.labels(reason="hydrate_failed").inc()
            return
        job = hydrated

    media_pk = job["media_pk"]
    tagger_username = job["tagger_username"] or "unknown"
    tagger_id = job["tagger_id"]
    story_id = job["story_id"]

    with tracer.start_as_current_span("jarvis.ig.comment.reply") as span:
        span.set_attribute("ig.media_pk", media_pk)
        span.set_attribute("ig.tagger", tagger_username)
        span.set_attribute("ig.media_type", job["media_type"])

        # Self-loop guard (belt + braces — poller already filters).
        if tagger_username.lower() == _own_username():
            span.set_attribute("ig.drop_reason", "self")
            metric_drops.labels(reason="self").inc()
            return

        authenticated = _is_followed(tagger_id)
        span.set_attribute("authenticated", authenticated)
        if not authenticated:
            print(f"ig comment: dropped unauthenticated tag from @{tagger_username}")
            span.set_attribute("ig.drop_reason", "unauthenticated")
            metric_drops.labels(reason="unauthenticated").inc()
            return

        # Song-ID short-circuit: if the tag text is asking to identify
        # the song, skip the gaslight brain and post a clean factual
        # reply. Only meaningful on videos (photos/carousels have no
        # audio to fingerprint).
        if _is_song_id_request(job["trigger_text"]):
            span.set_attribute("ig.song_id_request", True)
            if job["media_type"] != "video":
                # No audio track to ID. Silent drop — replying "no audio
                # to ID" on every selfie/meme tagged with "id?" would be
                # noise. (If we wanted to be helpful, we could comment
                # "no audio here" but the explicit guidance from the
                # prompt is "skip silently or short reply"; silent is
                # quieter and less spammy.)
                print(f"ig comment: song-id requested on non-video {media_pk} "
                      f"({job['media_type']}); skipping")
                span.set_attribute("ig.drop_reason", "song_id_non_video")
                metric_drops.labels(reason="song_id_non_video").inc()
                return

            reply = _run_song_id_pipeline(client, media_pk)
            if not reply:
                # Pipeline crashed before we even fingerprinted. Don't
                # ship anything — user can re-tag if needed.
                print(f"ig comment: song-id pipeline produced no reply for {media_pk}")
                span.set_attribute("ig.drop_reason", "song_id_pipeline_failed")
                metric_drops.labels(reason="song_id_pipeline_failed").inc()
                return

            thread_comment_id = _resolve_trigger_comment_id(client, job)
            span.set_attribute("ig.threaded", bool(thread_comment_id))
            try:
                kwargs: dict[str, Any] = {"text": reply}
                if thread_comment_id:
                    kwargs["replied_to_comment_id"] = int(thread_comment_id)
                client.media_comment(media_pk, **kwargs)
            except Exception as exc:  # noqa: BLE001
                print(f"ig comment: media_comment({media_pk}) [song-id] failed: {exc!r}")
                traceback.print_exc()
                span.set_attribute("ig.drop_reason", "send_failed")
                span.record_exception(exc)
                metric_drops.labels(reason="send_failed").inc()
                return

            dedup_key = thread_comment_id or story_id
            replied_set.add(dedup_key)
            _save_replied(replied_set)
            metric_replied.labels(authenticated="1").inc()
            print(f"ig comment: [song-id] replied to @{tagger_username} on {media_pk}: {reply!r}")
            return

        # Build vision context.
        vision_desc = _build_vision_description(client, job)
        span.set_attribute("ig.vision_chars", len(vision_desc))

        # Q&A short-circuit: literal questions get a factual reply
        # (no persona, no RP, no roast). Runs AFTER vision so the
        # answer can cite what's actually in the post.
        if _is_question_request(job["trigger_text"]):
            span.set_attribute("ig.qa_request", True)
            qa_prompt = QA_PROMPT_TEMPLATE.format(
                trigger_text=(job["trigger_text"] or "").replace('"', "'")[:200],
                author_username=job["author_username"] or "unknown",
                caption=(job["caption"] or "").replace('"', "'")[:240],
                vision_description=(vision_desc or "")[:600],
                sibling_comments_formatted=_sibling_block(job["sibling_comments"]),
            )
            try:
                import edge as _edge  # type: ignore[import]
                qa_reply = _edge._claude_brain_raw(qa_prompt).strip()
            except Exception as exc:  # noqa: BLE001
                print(f"ig comment: Q&A brain crashed: {exc!r}")
                qa_reply = ""
            qa_reply = _clean_reply(qa_reply)
            # Q&A allows long replies (up to IG's 2200-char cap). The
            # quality gate's normal 100-char limit would reject a
            # "explain quantum gravity" answer or a "post the bee movie
            # transcript" payload as too_long.
            ok_qa, qa_reason = _quality_check(qa_reply, allow_long=True)
            if not ok_qa:
                print(f"ig comment: Q&A quality fail ({qa_reason}): {qa_reply[:120]!r}")
                qa_reply = ""
            if qa_reply:
                thread_comment_id = _resolve_trigger_comment_id(client, job)
                try:
                    kwargs: dict[str, Any] = {"text": qa_reply}
                    if thread_comment_id:
                        kwargs["replied_to_comment_id"] = int(thread_comment_id)
                    client.media_comment(media_pk, **kwargs)
                except Exception as exc:  # noqa: BLE001
                    print(f"ig comment: media_comment({media_pk}) [Q&A] failed: {exc!r}")
                    span.set_attribute("ig.drop_reason", "send_failed")
                    metric_drops.labels(reason="send_failed").inc()
                    return
                dedup_key = thread_comment_id or story_id
                replied_set.add(dedup_key)
                _save_replied(replied_set)
                metric_replied.labels(authenticated="1").inc()
                print(f"ig comment: [Q&A] replied to @{tagger_username} on {media_pk}: {qa_reply!r}")
                return
            # Q&A brain returned nothing — fall through to normal comment flow

        # Compose prompt + call brain. We use _claude_brain_raw — NOT
        # brain_respond() (which short-circuits on 'good morning' /
        # 'briefing' and would ship the morning briefing as our reply if
        # those words appear in the tag text/caption) and NOT
        # _claude_brain (which injects the JARVIS butler persona — "sir",
        # "terse, max 12 words", "no markdown" — and would actively
        # fight the gaslight persona we want here).
        prompt = _build_prompt(job, vision_desc)
        try:
            import edge as _edge  # type: ignore[import]
            reply_raw = _edge._claude_brain_raw(prompt)
        except Exception as exc:  # noqa: BLE001
            print(f"ig comment: brain crashed for @{tagger_username}: {exc!r}")
            span.set_attribute("ig.drop_reason", "brain_error")
            span.record_exception(exc)
            metric_drops.labels(reason="brain_error").inc()
            return

        reply = _clean_reply(reply_raw)
        ok, reason = _quality_check(reply)
        if not ok:
            # One retry with a re-emphasis of the rules. If still bad,
            # skip silently — better to ghost than ship slop.
            print(f"ig comment: quality fail ({reason}) on first try: {reply!r}")
            retry_prompt = (
                "STRICT MODE. Output exactly one short comment under 15 words. "
                "No quotes. No prefix. No banned words "
                "(skibidi, gyatt, fanum, fr fr, no cap, iconic, obsessed, #). "
                "At most one emoji. If unsure, output exactly: ...\n\n"
                + prompt
            )
            try:
                reply_raw = _edge._claude_brain_raw(retry_prompt)
                reply = _clean_reply(reply_raw)
            except Exception as exc:  # noqa: BLE001
                print(f"ig comment: brain retry crashed for @{tagger_username}: {exc!r}")
                span.set_attribute("ig.drop_reason", "brain_error")
                metric_drops.labels(reason="brain_error").inc()
                return
            ok, reason = _quality_check(reply)
            if not ok:
                print(f"ig comment: quality fail ({reason}) on retry: {reply!r}; skipping")
                span.set_attribute("ig.drop_reason", f"quality:{reason}")
                metric_drops.labels(reason="quality").inc()
                return

        # Resolve a comment id to thread under, if poller didn't give us one.
        thread_comment_id = _resolve_trigger_comment_id(client, job)
        span.set_attribute("ig.threaded", bool(thread_comment_id))

        # Post the reply.
        try:
            kwargs: dict[str, Any] = {"text": reply}
            if thread_comment_id:
                kwargs["replied_to_comment_id"] = int(thread_comment_id)
            client.media_comment(media_pk, **kwargs)
        except Exception as exc:  # noqa: BLE001
            print(f"ig comment: media_comment({media_pk}) failed: {exc!r}")
            traceback.print_exc()
            span.set_attribute("ig.drop_reason", "send_failed")
            span.record_exception(exc)
            metric_drops.labels(reason="send_failed").inc()
            return

        # Mark replied (dedup by comment id when present, story id else).
        dedup_key = thread_comment_id or story_id
        replied_set.add(dedup_key)
        _save_replied(replied_set)

        metric_replied.labels(authenticated="1").inc()
        author = job["author_username"] or "unknown"
        print(f"ig comment: replied to @{tagger_username} on @{author}'s post {media_pk}: {reply!r}")


def _consumer_loop() -> None:
    """Drains _ig_comment_queue forever. Each job independently wrapped."""
    print("ig comment consumer: thread loop starting")
    handles: dict | None = None
    while True:
        try:
            if handles is None:
                handles = _get_edge_handles()
            client = _get_poller_client()
            if client is None:
                time.sleep(5)
                continue

            try:
                job = _ig_comment_queue.get(timeout=2.0)
            except queue.Empty:
                continue

            replied_set = _load_replied()
            try:
                _process_job(job, client, replied_set, handles)
            except Exception as exc:  # noqa: BLE001
                print(f"ig comment consumer: per-job crash: {exc!r}")
                traceback.print_exc()
                try:
                    handles["metric_drops"].labels(reason="error").inc()
                except Exception:  # noqa: BLE001
                    pass
        except Exception as exc:  # noqa: BLE001
            # Outermost safety net — don't pin CPU.
            print(f"ig comment consumer: outer loop crashed: {exc!r}")
            traceback.print_exc()
            time.sleep(5)


# ── Public entry point ─────────────────────────────────────────────────────
def start_threads() -> None:
    """Start poller + consumer daemon threads. Idempotent."""
    global _poller_thread, _consumer_thread
    with _threads_lock:
        if _poller_thread is None or not _poller_thread.is_alive():
            t = threading.Thread(
                target=_poller_loop,
                name="jarvis-ig-comment-poller",
                daemon=True,
            )
            t.start()
            _poller_thread = t
            print(f"ig comment: poller thread started "
                  f"(interval={_poll_interval_s()}s)")
        else:
            print("ig comment: poller thread already running")

        if _consumer_thread is None or not _consumer_thread.is_alive():
            t = threading.Thread(
                target=_consumer_loop,
                name="jarvis-ig-comment-consumer",
                daemon=True,
            )
            t.start()
            _consumer_thread = t
            print(f"ig comment: consumer thread started")
        else:
            print("ig comment: consumer thread already running")
