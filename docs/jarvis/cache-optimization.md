# JARVIS Edge Brain — Anthropic Prompt-Cache Optimization

**Scope:** the cluster edge brain (`rke2/ai/jarvis-edge/edge.py`). Diagnosis + ordered
recommendations + exact spec'd edits. **No code is changed by this document** — it is a spec for
@ai-engineer / Hampton to apply.

**TL;DR — the bug is real.** `_now_context()` returns the wall clock **to the minute** and is
concatenated *into* the `--append-system-prompt` string on every turn (`edge.py:846-848`). The
claude CLI puts the system prompt + tool definitions in the prompt-cache prefix with a
`cache_control` breakpoint. Prompt caching is a **prefix match** — any byte change in the prefix
invalidates everything after it. A timestamp that changes every 60 seconds means the entire
~25-35k-token system+tools prefix is reprocessed **uncached** on essentially every turn. This is
the single highest-value latency/cost fix available to the edge brain, and it is a one-line move,
not a rewrite.

---

## 1. Confirmed diagnosis

### 1.1 How Anthropic prompt caching works (authoritative)

From the Anthropic caching model (claude-api skill, `shared/prompt-caching.md`):

- **Caching is a prefix match.** The cache key is the exact bytes of the rendered prompt up to
  each `cache_control` breakpoint. *Any* byte change at position N invalidates the cache for every
  breakpoint at positions ≥ N.
- **Render order is `tools` → `system` → `messages`.** A breakpoint on the system prompt caches
  tools + system together. The system prompt sits *ahead of* the conversation in the prefix.
- **The claude CLI (Claude Code) caches tools + system by default.** Claude Code writes a
  `cache_control` breakpoint over the tool definitions and system prompt on every request — that
  is the large, stable prefix it is designed to reuse turn-to-turn. `--append-system-prompt`
  *appends to the system prompt*, so its content lands **inside that cached prefix**.
- **TTL.** Default ephemeral cache is 5-minute TTL (write cost ~1.25× base input). A 1-hour TTL
  is available (write cost ~2×). Cache **reads** cost ~0.1× base input.
- **Silent-invalidator #1 in the audit table is literally `datetime.now()` in the system
  prompt.** That is exactly what `_now_context()` does.

### 1.2 The specific cache-buster line

`edge.py:846-848`, inside `_claude_brain()`:

```python
persona_prompt = (_PERSONA_SYSTEM
                  + "\n\n" + _render_persona_prompt()
                  + "\n\n" + _now_context())     # ← cache-buster
```

passed at `edge.py:855-861`:

```python
["claude", "-p", text,
 "--append-system-prompt", persona_prompt,   # ← volatile content in the cached prefix
 "--mcp-config", _MCP_CONFIG_PATH,
 "--allowed-tools", _RO_ALLOWED_TOOLS,
 "--model", "claude-haiku-4-5-20251001",
 "--max-turns", "6",
 "--output-format", "json"]
```

And `_now_context()` itself (`edge.py:609-625`) renders **minute-resolution** wall clock:

```python
stamp = now.strftime("%-I:%M %p, %A, %B %-d, %Y")   # "4:18 AM, Monday, May 25, 2026"
```

Because `%-I:%M %p` changes every minute, `persona_prompt` changes every minute, the
`--append-system-prompt` bytes change every minute, the cached system+tools prefix is invalidated
every minute → **a cold cache write on (almost) every real turn**, never a read.

`_render_persona_prompt()` is a *secondary*, weaker buster: it only changes when Hampton retunes
humor/formality/terseness/sass, so in steady state it's stable — but it sits in the same
concatenated blob, so it must move too (see §3).

### 1.3 Same bug in the sibling brains

The identical pattern is in three other brain functions — all four must be fixed together or the
fix is partial:

| Function | Line | Buster |
|---|---|---|
| `_claude_brain` (owner, full tools) | 846-848 | `_render_persona_prompt()` + `_now_context()` |
| `_claude_brain_voice_locked` (trusted) | 912-914 | `_now_context()` |
| `_claude_brain_discord_locked` | 792-796 | `_render_persona_prompt()` + `_now_context()` |
| `_local_brain` / discord full (`~737`) | 737-738 | `_render_persona_prompt()` |

(The Discord ones are currently dormant — Discord token invalidated — but the source should be
fixed so they're correct when re-enabled.)

### 1.4 Quantified impact

Assumptions (conservative, Haiku 4.5 pricing: $1.00/MTok input, $0.10/MTok cache read,
$1.25/MTok 5-min cache write):

- System prompt (`_PERSONA_SYSTEM`, ~466 lines of persona) + tool definitions (the full
  `_RO_ALLOWED_TOOLS` MCP surface: personal/spotify/kube/delegate/sonos/persona/web/google
  tools, each with JSON schema) ≈ **25-35k tokens** of stable prefix. Call it 30k.

**Per-turn input-token cost of the prefix:**

| State | Prefix billing | $ per turn (30k prefix) |
|---|---|---|
| **Today** (busted every minute) | 30k @ full input = uncached | ~$0.030 |
| **Fixed, cache hit** | 30k @ cache-read 0.1× | ~$0.003 |
| **Fixed, cache miss (first turn / >TTL gap)** | 30k @ cache-write 1.25× | ~$0.0375 (once) |

So the steady-state per-turn input cost of the prefix drops **~10×** (from ~$0.030 to ~$0.003)
once consecutive turns land inside the 5-min TTL.

**Latency** is the bigger win for a voice assistant. Reprocessing 30k uncached input tokens vs.
reading them from cache is roughly a **second-or-more of time-to-first-token** difference on
Haiku (cache reads skip prefill of the cached span). For a Sonos voice loop where the whole turn
budget is a few seconds, shaving ~1s off prefill on every back-to-back turn is the difference
between "snappy" and "laggy". This is **architectural, not model choice** — switching models
wouldn't fix it; the prefix is busted regardless.

**Caveat on the magnitude:** the win only materializes for turns that fall **within the cache
TTL of the previous turn**. JARVIS traffic is bursty (a few turns in a conversation, then idle for
hours). The first turn of every burst is always a cold write regardless of this fix — see §2.2.

---

## 2. Ordered recommendations

### 2.0 (Priority 0) Move volatile content out of the cached prefix

**This is the fix.** Stop concatenating `_now_context()` (and the live persona line) into
`--append-system-prompt`. Instead:

1. Keep `--append-system-prompt` **byte-stable** = `_PERSONA_SYSTEM` only (a frozen string).
2. Move the wall-clock + (when changed) persona line into the **per-turn user text**, as a
   prefix on `text`. The user message sits *after* the cached system+tools prefix, so it never
   invalidates it.

This is exactly the contract the Phase 5 warm-brain spec already wrote down for this workstream
(`docs/jarvis/phase5-warm-brain.md` §3.4 lines 136-152, and §"Prompt-cache interaction" lines
343-352): *"keep the spawn-time system prompt byte-stable — do NOT fold the per-turn wall-clock
or persona-tuning into the system prompt … volatile context rides the user turn."* Phase 5
explicitly defers breakpoint placement to this doc and asks us to land `_now_context()` on the
per-turn user text. **Doing it now in the cold path means Phase 5 inherits a correct contract for
free** — the cold `_claude_brain` and the future warm `_WarmBrain.ask` use the identical
"frozen prefix + `[context: …]` user prefix" shape, so the warm-brain port is a behavior-preserving
swap rather than a second prompt-shape migration.

The user-prefix form (matching Phase 5 §4.2 line 221):

```
[context: <_now_context()>]
<actual user text>
```

When the persona mtime changed since the last turn, also fold in the rendered persona line on
that same prefix (cheap mtime check; `_load_persona()` already mtime-caches at `edge.py:535`). In
the **cold** path every turn is a fresh process with no "last turn" memory, so the simplest
correct cold-path behavior is: always include `_now_context()` in the user prefix, and always
include the persona line too (it's small — a handful of lines — and putting it in the user turn
costs a few uncached tokens per turn, far cheaper than busting the 30k prefix). The mtime-delta
optimization is a warm-brain refinement; the cold path can just always-include.

### 2.1 (Priority 1) Apply to all four brain functions

Fix `_claude_brain`, `_claude_brain_voice_locked`, `_claude_brain_discord_locked`, and the
discord-full brain identically. The voice-locked one is the most security-sensitive (Layer-A
trusted-speaker control) **and** the most latency-sensitive (it's in the same voice loop) — it
must keep `_VOICE_LOCKED_PERSONA_ADDENDUM` in the **frozen** system prompt (the addendum is
static; only `_now_context()` is volatile there), so its prefix stays stable and cacheable.

### 2.2 (Priority 2) Cache TTL — 5-min vs 1-hour, for bursty traffic

**Recommendation: switch the edge brains to the 1-hour TTL.**

JARVIS traffic is bursty and idle-heavy: a short conversation, then hours of silence. With the
default 5-min TTL, every conversation that starts >5 min after the last one pays a cold cache
write on its first turn — i.e. most first-turns are cold. The 1-hour TTL keeps the prefix warm
across the typical gaps between interactions in an active day (you walk in, ask something, walk
away, come back in 40 min).

Economics: 1h write is ~2× base vs 5-min's ~1.25×, and breakeven needs ≥3 reads (2× + 0.2× <
3×) vs 5-min's 2 reads. The 30k prefix is identical across *all* JARVIS turns of the same
role, so within any active hour you easily clear 3 reads. The extra write cost is ~$0.022 once
per warm hour — trivial vs. the ~$0.027/turn saved on every cached turn.

**How to set it via the claude CLI:** this is the one open question. The claude CLI manages its
own `cache_control` placement; whether it exposes a `--cache-ttl`/`CLAUDE_*` knob for 1h vs 5min
needs verification against the installed CLI version (`claude --help | grep -i cache`, and check
for a `cacheControl`/TTL field in `~/.claude/settings.json`). If the CLI does **not** expose TTL
control, the edge stays on the CLI's default (5-min) and the 1h win is unavailable without
dropping to the Anthropic SDK directly (out of scope — the edge intentionally uses the CLI for
subscription-OAuth auth). **Action: verify CLI TTL support before promising the 1h win.** If
unsupported, the §2.0 fix still delivers the full intra-conversation 10× win on the 5-min TTL.

### 2.3 (Priority 3) Boot prewarm (`max_tokens: 0`) — NOT worth it on the CLI edge

Prewarming (a `max_tokens: 0` prefill at boot to write the cache before the first real request)
is a real technique, but **skip it here**, for three reasons:

1. `max_tokens: 0` is an Anthropic **API** parameter. The edge brain shells out to the `claude`
   CLI, which does not expose a "prefill only, emit nothing" mode. You'd have to bypass the CLI
   and call the SDK with the *exact same tools+system bytes* the CLI would generate — fragile and
   high-maintenance (the CLI owns tool-schema serialization).
2. Even if you fired a cheap warm-up `claude -p "ping"` at boot, the 5-min default TTL means the
   warm cache is cold again long before the first real voice turn in most days — the prewarm is
   wasted unless traffic is continuous.
3. Prewarm's value is "kill the cold-miss latency on the *first* user-visible request." On a
   home voice assistant the first turn of a burst is already a one-off; the §2.0 fix makes every
   *subsequent* turn in the burst fast, which is the part the user actually notices (rapid
   back-and-forth). Prewarm optimizes the wrong turn here.

If/when Phase 5's warm persistent session lands (one long-lived `claude` process), the in-session
prefix stays warm for the life of the process and prewarm becomes moot anyway — the warm session
*is* the prewarm.

### 2.4 (Priority 4) The "One moment, sir." watchdog ack — does it exist on the edge?

**Checked: it does NOT exist in `edge.py`.** There is no "One moment" / "one moment" / watchdog
ack string anywhere in the edge brain (grep returns nothing). The edge has no early-ack mask for
a slow/cold turn — it just blocks on the `claude` subprocess for up to `timeout` (60s) and speaks
the result. The "One moment, sir." ack is therefore **Mac-daemon-only** (openjarvis), not on the
cluster edge.

Implication: today, a cold-prefix turn's full prefill latency is **unmasked** on the edge — the
user hears silence until TTS starts. That makes the §2.0 cache fix *more* valuable on the edge
than on the Mac (the Mac can hide cold latency behind the ack; the edge can't). A future
enhancement (out of scope for this cache doc, flag for @ai-engineer): port a lightweight
"One moment, sir." TTS ack on the edge for turns that exceed a short threshold, as a latency mask
for the rare cold/over-TTL turn that the cache fix can't eliminate. Low priority once §2.0 lands.

### 2.5 Interaction with cold-subprocess-today vs warm-session-later

- **Cold today (`claude -p` per turn):** each turn is a fresh process, but the *Anthropic-side*
  prompt cache is keyed on the prompt bytes, not the process — so consecutive cold subprocesses
  with an identical frozen prefix **do hit the cache** as long as they're within TTL. The §2.0
  fix works in the cold path with zero warm-session dependency. This is why it's worth doing
  **now**, independently of Phase 5.
- **Warm later (Phase 5 `_WarmBrain`):** the warm session sets `--append-system-prompt` **once at
  spawn** and feeds user turns over stdin. The cache prefix is then naturally frozen for the life
  of the process, and the `[context: …]` user-prefix is the only place volatile content can go
  anyway (you can't edit a running process's system prompt). So §2.0's shape is *required* for
  Phase 5 — landing it now means the warm-brain port doesn't have to also migrate the prompt
  shape. The contract is bidirectional: this doc owns breakpoint placement (answer: rely on the
  CLI's default tools+system breakpoint; keep that prefix byte-stable), Phase 5 owns the session
  lifecycle.

---

## 3. Exact spec'd edits (do NOT apply from this doc — this is the spec)

All line numbers are against the current `rke2/ai/jarvis-edge/edge.py`.

### Edit A — add a per-turn user-prefix helper (new function, near `_now_context` ~line 626)

Add a small helper that builds the volatile per-turn prefix. It composes `_now_context()` and
(for the owner/discord brains) the live persona line, and prepends them to the user text:

```python
def _turn_context_prefix(include_persona: bool = False) -> str:
    """Volatile per-turn context that MUST ride the user message, never the
    system prompt — so the cached tools+system prefix stays byte-stable and
    Anthropic prompt caching actually hits. See docs/jarvis/cache-optimization.md."""
    parts = [f"[context: {_now_context()}]"]
    if include_persona:
        parts.append(_render_persona_prompt())
    return "\n".join(parts) + "\n"
```

### Edit B — `_claude_brain` (owner) — lines 846-848 and 855

Replace the volatile concatenation:

```python
# BEFORE (846-848)
persona_prompt = (_PERSONA_SYSTEM
                  + "\n\n" + _render_persona_prompt()
                  + "\n\n" + _now_context())
```

```python
# AFTER — frozen system prompt; volatile content moves to the user turn
persona_prompt = _PERSONA_SYSTEM                       # byte-stable, cacheable
user_text = _turn_context_prefix(include_persona=True) + text
```

And in the argv (line 855), pass `user_text` instead of `text`:

```python
# BEFORE
["claude", "-p", text, ...]
# AFTER
["claude", "-p", user_text, ...]
```

Everything else in the argv (mcp-config, allowed-tools, model, max-turns, output-format) is
unchanged. `_PERSONA_SYSTEM` is now passed verbatim and never changes → cacheable prefix.

### Edit C — `_claude_brain_voice_locked` (trusted) — lines 912-914 and 917

```python
# BEFORE (912-914)
persona_prompt = (_PERSONA_SYSTEM
                  + _VOICE_LOCKED_PERSONA_ADDENDUM
                  + "\n\n" + _now_context())
```

```python
# AFTER — addendum is STATIC, keep it in the frozen system prompt;
# only _now_context() is volatile → move it to the user turn.
persona_prompt = _PERSONA_SYSTEM + _VOICE_LOCKED_PERSONA_ADDENDUM   # byte-stable
user_text = _turn_context_prefix(include_persona=False) + text       # no persona line (locked)
```

argv (line 917): pass `user_text` instead of `text`. **Security note:** this preserves the
Layer-A guarantee exactly — no `--mcp-config`, `--max-turns 1`, addendum still in the system
prompt. We are only relocating the timestamp; the trusted-speaker lockdown is untouched.

### Edit D — `_claude_brain_discord_locked` — lines 792-796 (+ the discord-full brain ~737-738)

Same pattern. For the discord brains, `include_persona=True` (they honor live persona tuning),
and the discord persona constant (`_DISCORD_PERSONA_SYSTEM`) becomes the frozen
`--append-system-prompt`. Note Discord is text, not voice — latency is less critical — but fixing
it keeps all four brains on one correct shape and avoids a future "why does only voice cache" foot-gun.

### Edit E (optional, Priority 2) — 1-hour cache TTL

Only after verifying the CLI exposes a TTL knob (see §2.2). If it does, set it to 1h for the edge
brains (likely via `~/.claude/settings.json` `cacheControl`/`ttl` or a CLI flag — verify exact
name). If it does not, no edit — the §2.0 fix stands on the default 5-min TTL.

### What NOT to change

- Do **not** reduce `_now_context()` resolution (e.g. to the hour) as a "fix" — that's a partial
  hack that still busts the cache hourly and degrades "is it late?" reasoning. Moving it to the
  user turn is strictly better and keeps minute resolution.
- Do **not** add prewarm to the CLI path (§2.3).
- Do **not** touch the tool list / `_RO_ALLOWED_TOOLS` ordering — reordering tools invalidates the
  prefix cache (it renders at position 0). It's already stable; leave it.

---

## 4. Verification after applying

1. **Confirm cache hits.** The claude CLI's `--output-format json` envelope reports usage. After
   two back-to-back owner turns (within TTL), the second turn's `usage` should show
   `cache_read_input_tokens` ≈ the prefix size (~30k) and `input_tokens` ≈ only the small user
   text. If `cache_read_input_tokens` is 0 on the second turn, a buster remains — diff the two
   rendered prompts. (Check whether the installed CLI surfaces `cache_read_input_tokens` in its
   JSON result; if not, measure via time-to-first-audio instead.)
2. **Latency.** Time two consecutive voice turns; the second should start TTS noticeably faster
   than the first (cache read vs cold write).
3. **Correctness.** "What time is it?" and "is it late?" must still answer correctly — confirms
   `_now_context()` survived the move into the user turn.
4. **Trusted lockdown intact.** A trusted (non-owner) voice turn still must not reach any tool
   (no `--mcp-config` in `_claude_brain_voice_locked` argv) — Edit C must not have added one.

---

## Summary

**Is the `_now_context()` cache-buster real? Yes — confirmed.** `_now_context()` (`edge.py:609`)
returns minute-resolution wall clock and is concatenated into `--append-system-prompt`
(`edge.py:846-848`, passed at `:855-861`). The claude CLI caches tools+system as the prompt-cache
prefix; Anthropic caching is prefix-match, so a value that changes every 60 seconds invalidates
the entire ~30k-token prefix on essentially every turn — a cold cache write per turn instead of a
~0.1× read. The fix is to freeze `--append-system-prompt` to `_PERSONA_SYSTEM` (+ static
addendum for the locked brain) and move `_now_context()` / the live persona line onto a
`[context: …]` prefix on the per-turn **user** text — exactly the contract Phase 5's warm-brain
spec already defined for this workstream. This is a ~one-line-per-brain change across four brain
functions, delivers a ~10× cut in steady-state prefix input cost and ~1s+ of prefill latency on
back-to-back voice turns, and is worth doing now in the cold path independently of Phase 5
(consecutive cold subprocesses still hit the Anthropic-side cache within TTL). The "One moment,
sir." ack does **not** exist on the edge (Mac-only), so cold-prefix latency is currently unmasked
— making this fix more valuable here than on the Mac. Boot prewarm is not worth it on the CLI
edge; switching to the 1-hour TTL is worthwhile for JARVIS's bursty traffic **if** the CLI
exposes a TTL knob (verify).
```