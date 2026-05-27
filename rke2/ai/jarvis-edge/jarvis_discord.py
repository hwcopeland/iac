"""jarvis_discord — text-only Discord selfbot transport (v1).

Spawned by edge.py main() when DISCORD_ENABLED=1, this module runs a
discord.py-self Client inside a dedicated asyncio loop on a daemon
thread. It reacts only when:

  - the message is a DM from DISCORD_OWNER_USER_ID, OR
  - the message is in a guild whose id is in DISCORD_WHITELIST_SERVER_IDS
    AND we are mentioned (`msg.mentions` contains us), OR
  - the message is a reply (`msg.reference`) to a message we previously
    sent in that same whitelisted guild.

Everything else is silently dropped. We never scrape, never enumerate
members, never join or leave guilds, never DM anyone unprompted. The
status is set to "invisible" on connect so we don't broadcast presence.

Persona reuse: this module is a thin transport layer. The persona /
prompt construction / quality gate / Q&A path / song-ID path are all
shared with the IG comment responder via direct function imports from
`jarvis_ig_comment_responder`. The brain call is the same
`edge._claude_brain_raw` used everywhere else, which already has the
local-Ollama fallback wired in. This guarantees Discord and IG never
drift apart in voice / refusal-handling / quality rules.

Architecture (sketch):

  start_thread()              ← public, called once from edge.py
    └─ threading.Thread(target=_thread_main, daemon=True)
         └─ asyncio.new_event_loop().run_until_complete(_async_main())
              └─ _async_main()  — connect/reconnect with backoff
                   └─ JarvisClient (discord.Client subclass)
                        ├─ on_ready  — set status invisible, log
                        └─ on_message — gate → route → reply

Trigger-routing decision tree (in _on_message_safe):

  IF author.id == our_id              → skip (self-loop)
  ELIF not _is_allowed_source(msg)    → skip silently
  ELIF not _is_triggered(msg)         → skip silently
  ELSE → _route_and_reply(msg)
         which dispatches to:
           - song-id path (audio/video attachment + song-id trigger text)
           - Q&A path (literal-question detection)
           - comment-persona path (default; the gaslight roast prompt)

Hard guarantees (fail-open everywhere):
  - Any exception inside on_message is caught and printed; the client
    keeps running.
  - LoginFailure / RateLimited bubble up to _async_main, which sleeps 5
    minutes and reconnects rather than hammering Discord.
  - An ImportError on discord.py-self (e.g. base image not rebuilt yet)
    is caught by edge.py's try/except around start_thread() — the voice
    loop and IG paths keep running.

Env vars (all read on thread start):
  DISCORD_ENABLED                "1" to start (gated in edge.py)
  DISCORD_USER_TOKEN             selfbot user token
  DISCORD_USER_ID                our own numeric id (self-loop guard,
                                  mention check, replies-to-us check)
  DISCORD_OWNER_USER_ID          Hampton's id; always allowed to DM
  DISCORD_WHITELIST_SERVER_IDS   comma-separated guild ids (only servers
                                  where we react to mentions/replies)
  DISCORD_RECONNECT_BACKOFF_S    sleep after LoginFailure/RateLimited
                                  (default 300)
  DISCORD_OWN_MESSAGES_CACHE     in-memory ringbuffer size for "replies-
                                  to-us" detection (default 500)
"""
from __future__ import annotations

import asyncio
import collections
import os
import threading
import time
import traceback
from typing import Any


# ── Module-level state ──────────────────────────────────────────────────────
_thread: threading.Thread | None = None
_thread_lock = threading.Lock()


def _env_int(name: str, default: int) -> int:
    try:
        return int(os.environ.get(name, str(default)) or default)
    except (TypeError, ValueError):
        return default


def _own_id() -> int:
    """Numeric Discord id of @hmlbjarvis. Returns 0 if unset/malformed —
    callers MUST treat 0 as "selfbot disabled" because the self-loop
    guard depends on a valid id."""
    raw = os.environ.get("DISCORD_USER_ID", "").strip()
    try:
        return int(raw)
    except (TypeError, ValueError):
        return 0


def _owner_id() -> int:
    """Hampton's main account. Used to allow DMs from the owner. Returns
    0 if unset/malformed, in which case DMs from anyone are dropped."""
    raw = os.environ.get("DISCORD_OWNER_USER_ID", "").strip()
    try:
        return int(raw)
    except (TypeError, ValueError):
        return 0


def _whitelist_server_ids() -> set[int]:
    """Comma-separated guild ids. Parsing is forgiving — bad entries are
    skipped, not raised. Empty set means "no guilds whitelisted" so
    every guild message is dropped (DMs from owner still work)."""
    raw = os.environ.get("DISCORD_WHITELIST_SERVER_IDS", "")
    out: set[int] = set()
    for tok in raw.split(","):
        tok = tok.strip()
        if not tok:
            continue
        try:
            out.add(int(tok))
        except ValueError:
            print(f"discord: skipping malformed guild id {tok!r}")
    return out


def _backoff_s() -> int:
    return max(30, _env_int("DISCORD_RECONNECT_BACKOFF_S", 300))


# ── Ringbuffer of messages WE sent ──────────────────────────────────────────
# Discord's `msg.reference.message_id` is what we check against to detect
# "someone replied to JARVIS". We can't query Discord for our own
# message history without scraping (against ToS + the spec), so we track
# the ids of messages WE post during this process's lifetime. Capacity
# is a soft cap — old entries roll off, which means a reply to a very
# old JARVIS message won't trigger; that's acceptable (a re-mention
# always works).
_own_message_ids: collections.deque[int] = collections.deque(
    maxlen=_env_int("DISCORD_OWN_MESSAGES_CACHE", 500),
)
_own_message_ids_lock = threading.Lock()


def _remember_own_message(message_id: int) -> None:
    """Idempotent — duplicates are harmless because we check membership,
    not count. Skips zero/falsy ids."""
    if not message_id:
        return
    with _own_message_ids_lock:
        _own_message_ids.append(int(message_id))


# ── Per-channel rolling conversation history ───────────────────────────────
# Keyed by channel_id. Each value is a deque of (timestamp, author_name,
# text) entries from ANY non-bot author in that channel — the owner,
# everyone else, even self. The reply-chain walker only helps when the
# user explicitly threads a Discord reply; rolling history fills the
# gap when context is scattered across multiple senders' messages
# (e.g. "@jarvis translate this" where the thing-to-translate was
# posted by a different user moments ago).
_channel_history: dict[int, collections.deque] = {}
_channel_history_lock = threading.Lock()


def _channel_id(msg: Any) -> int | None:
    channel = getattr(msg, "channel", None)
    cid = int(getattr(channel, "id", 0) or 0)
    return cid or None


def _history_append(msg: Any, author_name: str, text: str) -> None:
    """Append one turn to the rolling history for this channel."""
    cid = _channel_id(msg)
    if cid is None or not text:
        return
    cap = max(2, _env_int("DISCORD_HISTORY_TURNS", 12))
    with _channel_history_lock:
        dq = _channel_history.get(cid)
        if dq is None or dq.maxlen != cap:
            dq = collections.deque(maxlen=cap)
            _channel_history[cid] = dq
        dq.append((time.time(), author_name, text))


def _history_recent(
    msg: Any, exclude_text: str = ""
) -> list[tuple[str, str]]:
    """Return rolling history for this channel as (author_name, text)
    tuples, oldest → newest, TTL-filtered. `exclude_text` lets the
    caller skip the trigger message itself if it's been recorded."""
    cid = _channel_id(msg)
    if cid is None:
        return []
    ttl = max(60, _env_int("DISCORD_HISTORY_TTL_S", 3600))
    cap_chars = max(60, _env_int("DISCORD_HISTORY_TEXT_CAP", 240))
    cutoff = time.time() - ttl
    out: list[tuple[str, str]] = []
    with _channel_history_lock:
        dq = _channel_history.get(cid)
        if not dq:
            return []
        for ts, name, txt in dq:
            if ts < cutoff:
                continue
            if not txt:
                continue
            if exclude_text and txt == exclude_text:
                continue
            t = txt if len(txt) <= cap_chars else txt[:cap_chars].rstrip() + "…"
            out.append((name, t))
    return out


async def _fetch_channel_backfill(msg: Any, limit: int | None = None
                                   ) -> list[tuple[str, str]]:
    """Pull the last N messages from the Discord channel via the API.
    Used as a one-shot context grab when the rolling buffer is thin
    (e.g. right after pod start, or the trigger arrives before the
    channel-history accumulator has caught up). Oldest → newest order.
    Skips the trigger message itself and bot/self messages."""
    if limit is None:
        limit = max(2, _env_int("DISCORD_BACKFILL_LIMIT", 12))
    channel = getattr(msg, "channel", None)
    if channel is None or not hasattr(channel, "history"):
        return []
    own = _own_id()
    trigger_id = int(getattr(msg, "id", 0) or 0)
    cap_chars = max(60, _env_int("DISCORD_HISTORY_TEXT_CAP", 240))
    out: list[tuple[str, str]] = []
    try:
        # discord.py: channel.history() is an async iterator; newest first
        async for m in channel.history(limit=limit + 1):
            mid = int(getattr(m, "id", 0) or 0)
            if mid == trigger_id:
                continue
            author = getattr(m, "author", None)
            aid = int(getattr(author, "id", 0) or 0)
            if aid == own:
                continue
            if getattr(author, "bot", False):
                continue
            text = (getattr(m, "content", "") or "").strip()
            if not text:
                continue
            name = str(getattr(author, "name", "") or "?")
            if len(text) > cap_chars:
                text = text[:cap_chars].rstrip() + "…"
            out.append((name, text))
    except Exception as exc:  # noqa: BLE001
        print(f"discord: channel.history fetch failed: {exc!r}")
        return []
    out.reverse()  # oldest → newest
    return out


def _merge_context(
    *sources: list[tuple[str, str]],
) -> list[tuple[str, str]]:
    """Combine multiple context sources into one list for the brain's
    sibling_comments slot. Dedupes (name, text) pairs. Pass sources in
    oldest → newest order; the merge preserves that order."""
    seen: set[tuple[str, str]] = set()
    out: list[tuple[str, str]] = []
    for src in sources:
        for entry in src:
            if entry in seen:
                continue
            seen.add(entry)
            out.append(entry)
    return out


async def _collect_reply_chain(
    msg: Any, max_depth: int | None = None
) -> list[tuple[str, str]]:
    """Walk msg.reference upward, returning (author_name, content) tuples
    ordered oldest → newest (so the immediate parent of `msg` is last).

    Each text is trimmed to keep the prompt bounded. Stops on the first
    parent we can't resolve (deleted, no perms, network error).

    Depth defaults to DISCORD_REPLY_CHAIN_DEPTH (4)."""
    if max_depth is None:
        max_depth = max(0, _env_int("DISCORD_REPLY_CHAIN_DEPTH", 4))
    if max_depth <= 0:
        return []
    text_cap = max(40, _env_int("DISCORD_REPLY_CHAIN_TEXT_CAP", 300))

    chain: list[tuple[str, str]] = []
    cursor = msg
    for _ in range(max_depth):
        ref = getattr(cursor, "reference", None)
        if ref is None:
            break
        ref_id = getattr(ref, "message_id", None)
        if not ref_id:
            break
        parent = getattr(ref, "resolved", None)
        if parent is None:
            channel = getattr(cursor, "channel", None)
            if channel is None or not hasattr(channel, "fetch_message"):
                break
            try:
                parent = await channel.fetch_message(int(ref_id))
            except Exception as exc:  # noqa: BLE001
                print(f"discord: fetch_message({ref_id}) failed: {exc!r}")
                break
        if parent is None:
            break
        p_author = getattr(parent, "author", None)
        p_name = str(getattr(p_author, "name", "") or "?")
        p_text = (getattr(parent, "content", "") or "").strip()
        if len(p_text) > text_cap:
            p_text = p_text[:text_cap].rstrip() + "…"
        if p_text:
            chain.append((p_name, p_text))
        cursor = parent
    chain.reverse()
    return chain


def _is_reply_to_us(msg: Any) -> bool:
    """True if msg.reference points at a message we sent THIS process
    lifetime. Returns False for replies older than our cache window or
    replies in threads we never participated in.

    discord.py exposes `msg.reference` as a MessageReference object
    with .message_id (int) when the message is a reply. None otherwise."""
    ref = getattr(msg, "reference", None)
    if ref is None:
        return False
    ref_id = getattr(ref, "message_id", None)
    if not ref_id:
        return False
    with _own_message_ids_lock:
        return int(ref_id) in _own_message_ids


# ── Gate logic ──────────────────────────────────────────────────────────────
def _should_capture_context(msg: Any) -> bool:
    """Looser gate than _is_allowed_source — controls whether we RECORD
    a message into the rolling channel history for later context. We
    capture ANY non-bot author in a whitelisted guild (so the owner can
    later ask JARVIS about something someone else said), plus DMs from
    the owner. We never record DMs from non-owners (privacy + scope)."""
    guild = getattr(msg, "guild", None)
    if guild is None:
        # DM — only the owner.
        owner = _owner_id()
        if not owner:
            return False
        author_id = int(getattr(getattr(msg, "author", None), "id", 0) or 0)
        return author_id == owner
    # Guild message — whitelist check, any author.
    guild_id = int(getattr(guild, "id", 0) or 0)
    return guild_id != 0 and guild_id in _whitelist_server_ids()


def _is_allowed_source(msg: Any) -> bool:
    """Decide whether the message comes from a source we're allowed to
    react in.

    DMs: only allowed from DISCORD_OWNER_USER_ID. Anyone else's DM is
    silently dropped — we don't want to leak that the selfbot exists
    by replying to random DM-spammers, and "Hampton's main" is the only
    account we have a relationship with on Discord at this stage.

    Guild messages: only allowed in guilds whose id is in
    DISCORD_WHITELIST_SERVER_IDS. Trigger detection (mention vs reply)
    is layered on top in _is_triggered().

    Empty whitelist or unset owner id collapses to "drop everything" —
    safer than failing-open into spam."""
    # Per Hampton's directive: ONLY he can summon JARVIS. Anyone else
    # mentioning @hmlbjarvis (even in a whitelisted guild) is silently
    # dropped — there's no continuous-conversation path for non-owner
    # users. If Hampton's message instructs JARVIS to address someone
    # else, the REPLY content can mention them, but the trigger always
    # has to be Hampton's message.
    owner = _owner_id()
    if not owner:
        return False
    author_id = int(getattr(getattr(msg, "author", None), "id", 0) or 0)
    if author_id != owner:
        return False
    # Now check source: DMs from owner are always fine; guild messages
    # must additionally be in a whitelisted server.
    guild = getattr(msg, "guild", None)
    if guild is None:
        return True
    guild_id = int(getattr(guild, "id", 0) or 0)
    return guild_id != 0 and guild_id in _whitelist_server_ids()


def _is_triggered(msg: Any, own_id: int) -> bool:
    """Decide whether THIS message should fire a reply.

    DMs (no guild) → always trigger when source-allowed (already checked).
    Guild messages → trigger if we are @mentioned OR the message is a
                      reply to a message we sent.

    Mention detection: discord.py exposes msg.mentions as a list of
    User objects with .id; we check membership rather than scanning
    msg.content for the literal <@id> token (the discord client may
    render <@!id> or <@id> depending on nickname; mention list is
    canonical)."""
    if getattr(msg, "guild", None) is None:
        return True
    mentions = getattr(msg, "mentions", None) or []
    for u in mentions:
        if int(getattr(u, "id", 0) or 0) == own_id:
            return True
    return _is_reply_to_us(msg)


# ── Discord client ──────────────────────────────────────────────────────────
def _build_client() -> Any:
    """Construct the discord.py-self Client. Lazy-imports discord so
    edge.py can probe DISCORD_ENABLED before this module is imported.

    We set status='invisible' here so the selfbot doesn't broadcast a
    'JARVIS is online' presence — the IG comment responder's "tag and
    it shows up" UX is the model we want, not a presence-broadcasting
    bot."""
    import discord  # type: ignore[import]

    # NOTE: discord.py-self is the user-gateway library — selfbots have
    # the full user account's access and don't negotiate Intents the
    # way bots do. The Intents class was removed/never-present in the
    # selfbot fork; constructing the Client without intents= is the
    # supported pattern here.

    class JarvisClient(discord.Client):
        async def on_ready(self) -> None:
            try:
                me = self.user
                print(f"discord: connected as {me} (id={getattr(me, 'id', '?')}) "
                      f"to {len(self.guilds)} guild(s), status=invisible")
                # Set invisible. discord.Status.invisible is the
                # supported enum. change_presence() is async on the
                # user gateway.
                try:
                    await self.change_presence(status=discord.Status.invisible)
                except Exception as exc:  # noqa: BLE001
                    print(f"discord: failed to set invisible status: {exc!r}")
            except Exception as exc:  # noqa: BLE001
                print(f"discord on_ready: {exc!r}")
                traceback.print_exc()

        async def on_message(self, message: Any) -> None:
            # The OUTER try/except here is the last line of defense —
            # without it a bug in our handler would noisily crash the
            # client (discord.py logs the traceback but the connection
            # keeps running, which is fine, but we want quieter logs).
            try:
                await _on_message_safe(self, message)
            except Exception as exc:  # noqa: BLE001
                print(f"discord on_message: {exc!r}")
                traceback.print_exc()

    return JarvisClient()


# ── Message handler ────────────────────────────────────────────────────────
async def _on_message_safe(client: Any, msg: Any) -> None:
    """Per-message dispatch. See module docstring for the decision tree.

    All routing decisions log when they SKIP for debuggability — if
    Hampton can't get JARVIS to respond, the logs should make it obvious
    which gate dropped the message."""
    own_id = _own_id()
    if not own_id:
        # No self id configured — we can't even tell what's "us". Bail
        # silently on every message.
        return

    author = getattr(msg, "author", None)
    author_id = int(getattr(author, "id", 0) or 0)

    # Self-loop guard: a selfbot replying to its own messages is a
    # textbook infinite-recursion bug. msg.author.bot is also True for
    # the selfbot's own messages on discord.py-self (the lib flips that
    # flag for the connected user), so check both as belt + braces.
    if author_id == own_id:
        return
    if getattr(author, "bot", False):
        # External bots — drop. Don't engage with other bots.
        return

    # ── Passive context capture ──────────────────────────────────────────
    # Record every non-bot message in a whitelisted channel into the
    # rolling channel history, BEFORE the owner gate. This way when the
    # owner later @-mentions JARVIS to ask about something a different
    # user said, the brain already has the surrounding conversation.
    if _should_capture_context(msg):
        msg_text = (getattr(msg, "content", "") or "").strip()
        if msg_text:
            _history_append(msg,
                            str(getattr(author, "name", "") or "?"),
                            msg_text)

    if not _is_allowed_source(msg):
        return

    if not _is_triggered(msg, own_id):
        return

    # ── Trigger fired — build job + route ──
    content = (getattr(msg, "content", "") or "").strip()
    # Discord prefixes the mention as "<@123>" in raw content; strip it
    # for cleaner trigger-text matching. We don't strictly need to —
    # the IG triggers already tolerate noise — but it makes the prompt
    # logs cleaner.
    trigger_text = _strip_self_mention(content, own_id)
    guild = getattr(msg, "guild", None)
    is_dm = guild is None
    guild_name = getattr(guild, "name", "?") if guild is not None else ""
    print(f"discord: triggered by @{getattr(author, 'name', '?')} "
          f"({'DM' if is_dm else f'guild={guild_name}'}): "
          f"{trigger_text[:120]!r}")

    # If this message is itself a Discord reply, pull the chain above it
    # so the brain sees what we're actually replying to. Falls back to []
    # on any error — the brain still responds, just without context.
    reply_chain: list[tuple[str, str]] = []
    if getattr(msg, "reference", None) is not None:
        try:
            reply_chain = await _collect_reply_chain(msg)
            for i, (name, txt) in enumerate(reply_chain):
                print(f"discord: reply-chain[{i}] @{name}: {txt[:100]!r}")
        except Exception as exc:  # noqa: BLE001
            print(f"discord: reply-chain walk failed: {exc!r}")

    # Rolling channel history — captured passively by the loop above.
    # Pull what we have, excluding the trigger text itself so the brain
    # doesn't see it duplicated in sibling_comments. Trigger message
    # was already appended in the passive-capture branch.
    history = _history_recent(msg, exclude_text=trigger_text)

    # Channel backfill — when the rolling buffer is thin (cold start,
    # bot just joined, etc.), pull recent messages from Discord directly.
    # Async; runs every trigger but is cheap (one channel.history call).
    backfill = await _fetch_channel_backfill(msg)
    if backfill:
        print(f"discord: channel backfill={len(backfill)} message(s)")

    # Merge: passive history first (oldest), backfill next, reply-chain
    # last (most proximate). Dedup adjacent (name,text) pairs.
    sibling_comments = _merge_context(history, backfill, reply_chain)
    if sibling_comments:
        print(f"discord: context len={len(sibling_comments)} "
              f"(history={len(history)}, backfill={len(backfill)}, "
              f"reply_chain={len(reply_chain)})")
        # Log the FULL context so we can see exactly what the brain sees.
        for i, (name, txt) in enumerate(sibling_comments):
            print(f"discord: context[{i}] @{name}: {txt[:120]!r}")

    # Lazy-import the IG persona module so a discord.py-self bug can't
    # block edge.py startup at import time.
    try:
        import jarvis_ig_comment_responder as _ig  # type: ignore[import]
    except Exception as exc:  # noqa: BLE001
        print(f"discord: failed to import jarvis_ig_comment_responder: {exc!r}")
        return

    # Build the IG-shaped job dict. Discord has no media_pk/caption/
    # vision_description; we use empty strings so the prompt template
    # still formats cleanly.
    job = {
        "media_pk":           "",
        "media_type":         "",
        "caption":            "",
        "author_username":    str(getattr(author, "name", "") or ""),
        "author_user_id":     str(author_id),
        "trigger_comment_id": "",
        "trigger_text":       trigger_text,
        "tagger_username":    str(getattr(author, "name", "") or ""),
        "tagger_id":          str(author_id),
        "sibling_comments":   sibling_comments,
        "story_id":           f"discord:{getattr(msg, 'id', 0)}",
        "source":             "discord",
    }

    reply = await asyncio.get_event_loop().run_in_executor(
        None, _compose_reply, _ig, job, msg
    )
    if not reply:
        # Brain produced nothing usable (refusal that fell through both
        # Claude AND local, quality-check fail, etc.) — stay silent. The
        # IG path's "ghost over slop" rule applies here too.
        return

    try:
        sent = await msg.reply(reply, mention_author=False)
    except Exception as exc:  # noqa: BLE001
        print(f"discord: msg.reply failed: {exc!r}")
        return
    try:
        _remember_own_message(int(getattr(sent, "id", 0) or 0))
    except Exception:  # noqa: BLE001
        pass
    # Record JARVIS's own reply into the rolling history so the NEXT
    # turn in this conversation sees both sides of the exchange.
    _history_append(msg, "jarvis", reply)
    print(f"discord: replied to @{getattr(author, 'name', '?')}: {reply[:120]!r}")


def _strip_self_mention(text: str, own_id: int) -> str:
    """Strip leading/embedded <@id> / <@!id> mention tokens of OUR id.
    Leaves mentions of other users intact (they're part of the prompt
    context). Returns the stripped + whitespace-collapsed text."""
    if not text:
        return ""
    tokens = (f"<@{own_id}>", f"<@!{own_id}>")
    out = text
    for tok in tokens:
        out = out.replace(tok, " ")
    # Collapse runs of whitespace introduced by the strip.
    return " ".join(out.split()).strip()


# ── Reply composition (synchronous — runs in executor) ─────────────────────
def _compose_reply(ig_mod: Any, job: dict, msg: Any) -> str:
    """Dispatch to the right IG-persona path based on the trigger text.
    Runs in a thread executor because edge._claude_brain_raw + the
    song-ID pipeline are SYNC (subprocess.run + ffmpeg + shazamio
    asyncio.run inside). Returns the final reply string or "" to skip.

    The IG comment responder's _process_job is too IG-specific (assumes
    media_pk, instagrapi client, sibling-comments fetch) to call
    directly, so we replicate just the routing-and-brain part here. The
    actual prompts / quality rules / banned-word list are still imported
    from jarvis_ig_comment_responder so the persona stays unified."""
    trigger_text = job["trigger_text"]

    # ── Song-ID path: tag text reads like "id?" / "what song" / etc.,
    #    AND the message has at least one audio/video attachment we
    #    can fingerprint. If trigger fires but no attachment, fall
    #    through to the comment-persona path (will roast).
    if ig_mod._is_song_id_request(trigger_text):
        wav_path = _download_first_audio_attachment(msg)
        if wav_path:
            try:
                import jarvis_song_id as _song  # type: ignore[import]
                hit = _song.identify_from_wav(wav_path)
            except Exception as exc:  # noqa: BLE001
                print(f"discord: song_id crashed: {exc!r}")
                hit = None
            try:
                os.unlink(wav_path)
            except OSError:
                pass
            if hit and hit.get("title") and hit.get("artist"):
                return ig_mod._format_song_reply(hit)
            return "can't place it"
        # No attachment to fingerprint — fall through.

    # ── Q&A path: literal question. We don't have a post / caption /
    #    vision description on Discord, so the QA template's
    #    "what's in the post" context is empty. Still useful — the
    #    user might be asking "what's the chemical formula for X" or
    #    "who is the actor in Top Gun" where context isn't needed.
    if ig_mod._is_question_request(trigger_text):
        try:
            qa_prompt = ig_mod.QA_PROMPT_TEMPLATE.format(
                trigger_text=trigger_text.replace('"', "'")[:400],
                author_username=job["author_username"] or "unknown",
                caption="",
                vision_description="(discord — no post context)",
                sibling_comments_formatted="(none)",
            )
            import edge as _edge  # type: ignore[import]
            qa_reply_raw = _edge._claude_brain_raw(qa_prompt) or ""
        except Exception as exc:  # noqa: BLE001
            print(f"discord: Q&A brain crashed: {exc!r}")
            qa_reply_raw = ""
        qa_reply = ig_mod._clean_reply(qa_reply_raw)
        ok, reason = ig_mod._quality_check(qa_reply, allow_long=True)
        if ok:
            return qa_reply
        print(f"discord: Q&A quality fail ({reason}): {qa_reply[:120]!r}")
        # Fall through to comment-persona retry.

    # ── Default: full butler brain WITH MCP tools enabled.
    #    Discord is a conversational interface (only Hampton can talk
    #    to JARVIS — owner-only gate is upstream), not a public IG
    #    comment-section drop-in. Use brain_respond() so JARVIS has the
    #    same butler persona + full MCP toolbox (spotify / sonos /
    #    kube-read / personal / persona / google / etc.) that the voice
    #    JARVIS uses at the desk. Reply length is unbounded — Discord
    #    cap is ~2000 chars per message, plenty.
    try:
        import edge as _edge  # type: ignore[import]
        raw = _edge._claude_brain_discord(trigger_text) or ""
    except Exception as exc:  # noqa: BLE001
        print(f"discord: brain_respond crashed: {exc!r}")
        return ""
    # Strip wrapping quotes / prefix junk but don't apply the IG
    # one-liner quality gate — butler replies are allowed to be
    # multi-sentence and may include tool-driven content.
    reply = ig_mod._clean_reply(raw).strip()
    if not reply:
        print("discord: butler brain returned empty")
        return ""
    return reply


# ── Attachment helpers ──────────────────────────────────────────────────────
_AUDIO_EXTS = (".wav", ".mp3", ".m4a", ".ogg", ".opus", ".aac", ".flac")
_VIDEO_EXTS = (".mp4", ".mov", ".m4v", ".webm", ".mkv", ".gif")


def _download_first_audio_attachment(msg: Any) -> str | None:
    """Download the first audio or video attachment to /tmp and convert
    to 16kHz mono PCM WAV (the format jarvis_song_id / shazamio want).
    Returns the WAV path on success, None on failure or no eligible
    attachment.

    NOTE: this runs synchronously inside the executor — it does HTTP
    downloads and an ffmpeg subprocess. That's fine for a low-volume
    bot; we wouldn't want to do this on the main asyncio thread."""
    attachments = list(getattr(msg, "attachments", []) or [])
    if not attachments:
        return None
    target = None
    for a in attachments:
        fn = (getattr(a, "filename", "") or "").lower()
        if fn.endswith(_AUDIO_EXTS) or fn.endswith(_VIDEO_EXTS):
            target = a
            break
    if target is None:
        return None

    # Download to /tmp. discord.Attachment exposes a sync .save(path) in
    # discord.py-self that does the HTTP fetch; we use that rather than
    # await target.save() to keep this whole function sync.
    src_path = f"/tmp/discord_{int(time.time() * 1000)}_{getattr(target, 'filename', 'attach')}"
    try:
        # Some attachment .save() impls require an open file handle;
        # discord.py-self accepts a path string.
        # If the API requires async, we fall back to urlretrieve below.
        url = getattr(target, "url", None) or getattr(target, "proxy_url", None)
        if not url:
            return None
        import urllib.request as _u
        with _u.urlopen(url, timeout=20) as resp, open(src_path, "wb") as f:
            f.write(resp.read())
    except Exception as exc:  # noqa: BLE001
        print(f"discord: attachment download failed: {exc!r}")
        return None
    if not os.path.exists(src_path) or os.path.getsize(src_path) < 1024:
        try:
            os.unlink(src_path)
        except OSError:
            pass
        return None

    # ffmpeg -> 16k mono PCM. Reuse jarvis_reel_context._extract_audio
    # which already shells out with the right flags.
    out_path = src_path + ".wav"
    try:
        import jarvis_reel_context as _reel  # type: ignore[import]
        ok = _reel._extract_audio(src_path, out_path)
    except Exception as exc:  # noqa: BLE001
        print(f"discord: audio extract crashed: {exc!r}")
        ok = False
    try:
        os.unlink(src_path)
    except OSError:
        pass
    if not ok or not os.path.exists(out_path):
        return None
    return out_path


# ── Event-loop driver (per-thread) ──────────────────────────────────────────
async def _async_main() -> None:
    """Connect to Discord with backoff on auth/rate-limit failure. This
    coroutine is the body of the dedicated event loop — it exits only
    on KeyboardInterrupt (which doesn't happen inside a daemon thread)."""
    import discord  # type: ignore[import]

    token = (os.environ.get("DISCORD_USER_TOKEN") or "").strip()
    if not token:
        print("discord: DISCORD_USER_TOKEN unset, exiting thread")
        return

    # LoginFailure / HTTPException live under discord.errors in both
    # discord.py and discord.py-self; the top-level shortcuts (e.g.
    # discord.LoginFailure) are re-exports but the .errors path is the
    # one guaranteed across forks/versions.
    LoginFailure = getattr(discord.errors, "LoginFailure", Exception)
    HTTPException = getattr(discord.errors, "HTTPException", Exception)

    while True:
        client = _build_client()
        try:
            # client.start() is the long-running coroutine; it connects,
            # then dispatches events until the connection is closed.
            await client.start(token)
            # If start() returns cleanly the websocket closed normally;
            # we loop and reconnect.
            print("discord: client.start() returned cleanly — reconnecting")
        except LoginFailure as exc:
            print(f"discord: LoginFailure ({exc!r}) — token invalid? "
                  f"sleeping {_backoff_s()}s before retry")
        except HTTPException as exc:
            # RateLimited inherits from HTTPException on every version.
            print(f"discord: HTTPException ({exc!r}) — sleeping "
                  f"{_backoff_s()}s before retry")
        except Exception as exc:  # noqa: BLE001
            print(f"discord: unexpected error in client.start: {exc!r}")
            traceback.print_exc()
        finally:
            try:
                if not client.is_closed():
                    await client.close()
            except Exception:  # noqa: BLE001
                pass
        # Backoff sleep. asyncio.sleep so we don't block the loop
        # (irrelevant since we're alone on this loop, but cleaner).
        await asyncio.sleep(_backoff_s())


def _thread_main() -> None:
    """Daemon-thread entrypoint. Creates a fresh asyncio loop (discord
    needs to own its loop) and runs _async_main forever."""
    print("discord: thread starting")
    try:
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)
        try:
            loop.run_until_complete(_async_main())
        finally:
            try:
                loop.close()
            except Exception:  # noqa: BLE001
                pass
    except Exception as exc:  # noqa: BLE001
        # Catch-all so a thread crash never bubbles out and prints a
        # "Exception in thread" traceback that operators read as scary.
        print(f"discord: thread crashed: {exc!r}")
        traceback.print_exc()
    print("discord: thread exiting")


# ── Public entry point ─────────────────────────────────────────────────────
def start_thread() -> None:
    """Spawn the daemon thread that runs the asyncio loop + discord
    client. Idempotent — second + later calls are no-ops.

    Caller (edge.py main) must wrap this in try/except so an
    ImportError on discord.py-self (base image not rebuilt) or a
    misconfigured token doesn't crash the voice loop."""
    global _thread
    with _thread_lock:
        if _thread is not None and _thread.is_alive():
            print("discord: thread already running, skipping start")
            return
        # Cheap pre-flight: log when required env is missing so
        # operators see WHY nothing is happening instead of silent
        # failure. We still START the thread — _async_main does its
        # own check and exits cleanly if the token is missing.
        own = _own_id()
        owner = _owner_id()
        guilds = _whitelist_server_ids()
        if not os.environ.get("DISCORD_USER_TOKEN"):
            print("discord: WARN — DISCORD_USER_TOKEN unset; thread will start and exit")
        if not own:
            print("discord: WARN — DISCORD_USER_ID unset/invalid; "
                  "self-loop guard will silently drop everything")
        if not owner:
            print("discord: WARN — DISCORD_OWNER_USER_ID unset; DMs will be dropped")
        if not guilds:
            print("discord: WARN — DISCORD_WHITELIST_SERVER_IDS empty; "
                  "all guild messages will be dropped (DMs from owner still work)")

        t = threading.Thread(
            target=_thread_main,
            name="jarvis-discord",
            daemon=True,
        )
        t.start()
        _thread = t
        print(f"discord: started daemon thread (own_id={own}, owner_id={owner}, "
              f"whitelist_guilds={sorted(guilds)})")
