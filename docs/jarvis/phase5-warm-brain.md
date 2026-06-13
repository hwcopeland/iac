# Phase 5 — Warm Persistent Brain (the `Brain` swap)

**Status:** design spec only. No code in `edge.py` is changed by this document.
**Scope:** `~/iac/rke2/ai/jarvis-edge/edge.py` (OWNER voice path only).
**Prereq for:** Phase 6 (mem0 continuity — needs a long-lived session for cross-turn memory).
**Plan reference:** `~/.claude/plans/is-ther-no-other-distributed-duckling.md` § Phase 5.

---

## 1. Problem & goal

`_claude_brain(text, timeout, mem_scope)` (`edge.py:828`) spawns a **cold `claude -p`
subprocess every turn**: `--append-system-prompt`, `--mcp-config`, `--allowed-tools`,
`--model claude-haiku-4-5-20251001`, `--max-turns 6`, `--output-format json`. Each turn pays:

- **MCP server cold-start** — six `python3 /app/jarvis_*_mcp.py` servers re-spawned and
  re-handshaked per turn (the dominant cost; ~2–3 s before the model even runs).
- **No cross-turn continuity** — the model never sees the previous turn. The owner can't say
  "and the one after that?" The greeting shortcut papers over this for briefings only.
- **mem0 (Phase 6) is structurally useless without it** — a memory MCP wired into a fresh
  subprocess every turn has no conversation to anchor retrieval/extraction against.

**Goal:** a single long-lived `claude` process for the OWNER, reused across turns via
stream-json, that keeps the MCP servers warm and the conversation alive. Replace the cold call
**only on the OWNER path inside `brain_respond`**. Everything else (TRUSTED locked, open-mode,
greeting shortcut, metrics, error envelope) is preserved exactly.

This is the **Stage-4 `Brain` swap** named in the plan's spine: `cold_subprocess` → `warm_session`
behind `brain_respond`, at one seam, no rewrite of the gate.

---

## 2. Hard isolation requirement (the load-bearing constraint)

The warm session holds **owner tools + owner conversation history + (Phase 6) owner memories**.
A TRUSTED turn entering it would leak all three. Therefore:

> **TRUSTED turns MUST NEVER touch the warm owner session.** They stay on the existing
> **stateless** `_claude_brain_voice_locked` (`edge.py:900`) — a cold `claude -p` with **no
> `--mcp-config`**, `--max-turns 1`, no fallthrough.

This is enforced **in Python, in `gate_and_respond`**, before any model call — the same
deterministic-gate principle the plan insists on (the model is never the boundary). Concretely:

- `gate_and_respond` already branches by `principal.role` (`edge.py:1262-1270`). The warm
  session is reached **only** through the OWNER branch → `brain_respond(...)` →
  `_claude_brain` swap. The TRUSTED and UNKNOWN branches never call `brain_respond` and never
  see the warm session handle.
- The warm session is a **module-global singleton** (one owner, one session). TRUSTED/UNKNOWN
  code paths simply never reference it. There is no per-principal session pool — that would
  invite a routing bug where a TRUSTED principal gets a warm handle. One global, OWNER-only,
  by construction.
- **Belt-and-suspenders:** `brain_respond` itself takes `mem_scope` and, in the warm path,
  asserts `mem_scope == "owner"` (open-mode passes `""` → falls back to the **cold** path, see
  §4.3). If a future refactor ever routes a non-owner scope into `brain_respond`, it degrades
  to a stateless cold call rather than joining the warm session. Defense in depth, not the
  primary control — the primary control is that `gate_and_respond` never calls `brain_respond`
  for TRUSTED/UNKNOWN.

Layer-A guarantee is unchanged: TRUSTED argv still has no `--mcp-config`, so a trusted user
**physically cannot** reach a tool, warm session or not.

---

## 3. `claude` CLI warm-session mechanics

openjarvis's `ClaudeCodeBrain` drives the `claude` CLI in **streaming JSON** mode with a
persistent session. edge.py already proves the wire format works here — `_claude_brain_vision`
(`edge.py:1118-1162`) runs `claude -p --input-format stream-json --output-format stream-json`
and parses the one-event-per-line output for the `{"type":"result","result":...}` envelope. The
warm port extends that from one-shot to long-lived.

### 3.1 Two viable session models

**(A) Long-lived subprocess, stdin held open (preferred).**
Spawn `claude` once with `--input-format stream-json --output-format stream-json`, keep
`stdin`/`stdout` pipes open, and write **one user-message JSON object per line per turn**, then
read stdout lines until the turn's `result` event. The process — and every MCP server it
launched — stays resident between turns. This is the warmest option (MCP servers never
re-spawn) and matches `ClaudeCodeBrain`'s design.

**(B) `--resume <session-id>` re-invocation (fallback).**
Assign a fixed session id with `--session-id <uuid>` on first turn; on later turns call
`claude -p <text> --resume <session-id> ...`. This preserves **conversation continuity** (the
prereq for mem0) but **re-spawns a fresh process + MCP servers each turn**, so it does NOT
recover the ~2–3 s MCP cold-start. Use only if (A) proves unstable on NixOS/hostNetwork.

**Decision:** implement (A). Keep (B)'s `--session-id` assignment in the spawn argv anyway, so
that if the long-lived process dies mid-conversation we can **respawn with `--resume <same-id>`**
and recover history (§4.4). This gives warmth from (A) and crash-recovery from (B).

### 3.2 Spawn argv (warm OWNER session)

```
claude
  --input-format  stream-json
  --output-format stream-json
  --verbose                       # stream-json requires verbose for full event stream
  --session-id    <persisted-uuid>
  --append-system-prompt <persona + persona-tuning + now-context>   # see §3.4
  --mcp-config    /tmp/jarvis_mcp.json          # full owner toolbox (already written at boot)
  --allowed-tools <_RO_ALLOWED_TOOLS>           # edge.py:628 — unchanged
  --model         claude-haiku-4-5-20251001     # unchanged; keep current model
  # NO --max-turns: a warm session runs many turns over its lifetime.
  # Per-turn tool-loop bounding is handled by the turn-level read timeout (§4.2),
  # not --max-turns (which would cap the whole session).
```

Auth is inherited from env exactly as the cold path: prefer `~/.claude/.credentials.json`
(subscription), else `ANTHROPIC_API_KEY`. The init container seeds creds to the
jarvis-state PVC; the running CLI refreshes tokens back onto the PVC — **the warm process holds
those creds open for its whole lifetime**, which is fine (the CLI refreshes in place).

### 3.3 Per-turn wire protocol

Per turn, write exactly one line to stdin (mirrors `_claude_brain_vision`'s user-message shape):

```json
{"type":"user","message":{"role":"user","content":[{"type":"text","text":"<turn text, see 3.4>"}]}}
```

Then read stdout lines until the terminating event for this turn:

- `{"type":"result","result":"<spoken text>","is_error":false, ...}` → return `result`.
- `{"type":"result","is_error":true, ...}` → treat as brain error (same envelope mapping as
  cold path, §5).
- `{"type":"assistant","message":{...}}` events stream in between (tool calls, partial text) —
  last-assistant-text is the fallback if no `result` event ships, identical to vision's
  scan-loop (`edge.py:1156-1161`).

stdin is **not** closed between turns (that's what keeps the session alive — closing stdin is
what makes the one-shot vision call exit). One newline-terminated JSON object per turn.

### 3.4 Persona + mem_scope flow per turn

Cold path concatenates `_PERSONA_SYSTEM + _render_persona_prompt() + _now_context()` into a
single `--append-system-prompt` **every turn** (`edge.py:846-848`), because each turn is a fresh
process. A warm session sets `--append-system-prompt` **once at spawn**. Two things change per
turn and must be re-injected into the live session:

1. **Wall-clock (`_now_context()`)** — drifts; the session was spawned at boot. Re-inject the
   current time as a lightweight prefix on each turn's user text (NOT a system-prompt edit — the
   process is already running; we can only feed user messages). Prefix form:
   `"[context: <_now_context()>]\n<actual text>"`. Cheap, inert, keeps greetings/"is it late?"
   correct.
2. **Persona tuning (`_render_persona_prompt()`)** — live-adjustable via the persona MCP. When
   the owner changes a dimension mid-session, the persona MCP writes `/state/persona.json`; the
   running session can re-read it because the persona MCP tool result reflects the new value,
   and the owner's adjust request is in-conversation. For drift safety, also fold the rendered
   persona line into the same per-turn context prefix when `_load_persona()` mtime changed since
   last turn (cheap mtime check, already cached at `edge.py:535`).

**`mem_scope`** (Phase 6 hook): the warm OWNER session is, by definition, `mem_scope="owner"`.
The cold path threads it via `JARVIS_MEM_SCOPE` env (`edge.py:852`). The warm process is spawned
**once** with `JARVIS_MEM_SCOPE=owner` in its env — it never changes for the life of the session
(owner session ⇒ owner scope, invariant). Phase 6's mem0 MCP reads that env at MCP-server
spawn, which now happens once and stays warm. This is strictly better for mem0 than the cold
path: a persistent memory server with a stable scope and a live conversation to extract from.

### 3.5 Greeting shortcut interaction

`_maybe_greeting_shortcircuit` (`edge.py:1178`) runs **before** any brain call inside
`brain_respond` (`edge.py:1207`) and returns a pre-composed briefing, bypassing the model
entirely. **Keep this ahead of the warm session, unchanged.** Rationale:

- It's a latency win (skips a model round-trip) that a warm session doesn't replace.
- But the briefing then never enters the warm session's history — so if the owner follows
  "good morning" with "tell me more about that first item", the warm session has no context for
  "that". **Mitigation:** when a greeting shortcut fires, **also feed the briefing into the warm
  session as a synthetic assistant turn** so the conversation stays coherent. This is a
  fire-and-forget write (`{"type":"assistant",...}` is not a valid *input* event — instead send
  a user message `"[note: I just told the owner the morning briefing: <text>]"` so the session
  records the context). Low priority; can be a follow-up within Phase 5. If skipped, the only
  cost is that briefing follow-ups lose context — acceptable for v1.

---

## 4. Implementation spec (`edge.py`)

### 4.1 New module-level state & class

Add near the brain section (after `_claude_brain`, ~`edge.py:892`):

```python
# ── Warm OWNER brain session (Phase 5) ───────────────────────────────────────
_WARM_SESSION_ID_PATH = os.environ.get(
    "WARM_SESSION_ID_PATH", "/state/warm_session_id")
_WARM_BRAIN_ENABLED = os.environ.get("WARM_BRAIN", "1") == "1"   # kill-switch

class _WarmBrain:
    """Single long-lived `claude` stream-json session for the OWNER path ONLY.

    Thread-safe: the mic loop is single-threaded today, but the Phase-3 HTTP
    ingest endpoint can call gate_and_respond concurrently. A turn lock
    serializes writes/reads on the one stdin/stdout pair (one conversation,
    one turn at a time — concurrent owner turns are nonsensical anyway)."""
    def __init__(self): ...
    def _spawn(self, resume: bool) -> None: ...        # build argv, Popen, store pid
    def _alive(self) -> bool: ...                      # proc is not None and poll() is None
    def ask(self, text: str, timeout: float) -> str: ...  # one turn; respawn-on-death
    def shutdown(self) -> None: ...                    # close stdin, terminate

_warm_brain: _WarmBrain | None = None
_warm_brain_lock = threading.Lock()                    # guards singleton construction
```

`threading` is already imported (`edge.py:37`).

### 4.2 `_WarmBrain.ask` — the per-turn contract

```python
def ask(self, text: str, timeout: float = 60.0) -> str:
    """Run one OWNER turn on the warm session. Returns spoken text, or one of
    the SAME error strings _claude_brain returns (so brain_respond's metric
    classification at edge.py:1220-1232 keeps working unchanged)."""
```

Behavior:
1. Acquire the per-instance turn lock (serialize turns).
2. If `not self._alive()`: respawn (§4.4) with `--resume <persisted id>`.
3. Build the per-turn user text: `"[context: <_now_context()>]"` + (persona line if mtime
   changed) + `"\n" + text`.
4. Write the user-message JSON line to stdin, flush.
5. Read stdout lines with a **per-turn deadline** (`time.monotonic()` + `timeout`). This
   replaces `--max-turns 6` as the runaway-tool-loop bound: if the turn exceeds `timeout`
   without a `result` event, return `"That took too long, sir — try again."` (same string the
   cold path returns on `TimeoutExpired`, `edge.py:889`) and **mark the session for respawn**
   (a wedged turn may have left the pipe mid-stream).
6. On `result`/`is_error`/empty → map to the **exact** cold-path return strings (§5).
7. Wrap the whole read loop so a `BrokenPipeError`/`OSError` (process died mid-turn) triggers
   one respawn-and-retry; a second failure returns `"I lost my connection there, sir."`.

The read loop must run on a thread or use non-blocking reads with a deadline — a naive
`for line in proc.stdout` blocks forever if the process wedges. Use a reader thread that pushes
lines onto a `queue.Queue` (already imported, `edge.py:33`) and `queue.get(timeout=remaining)`,
or `select` on the pipe fd. Reader-thread + queue is the simpler, portable choice on the pod.

### 4.3 `brain_respond` — the routing change (the ONLY behavioral edit)

Current OWNER body (`edge.py:1217`): `reply = _claude_brain(text, mem_scope=mem_scope)`.

Replace with a warm/cold selector that preserves every surrounding line (the greeting
shortcut at 1207, the mode span attribute, the metric classification at 1220-1232, the
`finally` block at 1234-1240):

```python
# inside brain_respond, replacing only line 1217:
if (_WARM_BRAIN_ENABLED and mem_scope == "owner"
        and os.environ.get("BRAIN_MODE", "claude") == "claude"):
    reply = _get_warm_brain().ask(text)        # warm OWNER session
else:
    reply = _claude_brain(text, mem_scope=mem_scope)   # cold: open-mode, or kill-switch
```

`_get_warm_brain()` lazily constructs the singleton under `_warm_brain_lock`. Key points:

- **OWNER → warm.** `gate_and_respond` calls `brain_respond(text, mem_scope="owner")` only for
  OWNER (`edge.py:1263`). That's the sole route to `mem_scope == "owner"`, so the warm path is
  OWNER-only by construction.
- **Open-mode → cold.** `gate_and_respond(None, ...)` calls `brain_respond(text)` with
  `mem_scope=""` (`edge.py:1260`) → falls to the cold `_claude_brain`. Open mode (no owner
  enrolled) keeps today's stateless behavior; we do not stand up a warm session for an
  unidentified speaker.
- **TRUSTED/UNKNOWN → never here.** They don't call `brain_respond` at all (§2).
- **Kill-switch.** `WARM_BRAIN=0` env forces the cold path everywhere — instant rollback to
  current behavior without redeploying code, useful if the warm session misbehaves on the GPU
  node.

`_claude_brain`, `_claude_brain_voice_locked`, `_claude_brain_discord*`, `gate_and_respond`'s
role branches, and the metric/error strings are **untouched**.

### 4.4 Lifecycle: prewarm, reuse, respawn-on-death

**Prewarm at boot.** In `main()` (`edge.py:1821`), after `_write_mcp_config()` +
`_check_brain_auth()` (`edge.py:1828-1833`), add — gated on `_WARM_BRAIN_ENABLED` and
`_vid_has_owner()` (no point warming an owner session if no owner is enrolled / open mode):

```python
if _WARM_BRAIN_ENABLED and BRAIN_MODE == "claude" and _vid_has_owner():
    try:
        _get_warm_brain()          # spawn now → MCP servers warm before first turn
        print("warm brain: OWNER session prewarmed")
    except Exception as exc:        # FAIL-OPEN, like every other main() subsystem
        print(f"warm brain: prewarm failed ({exc}) — falling back to cold per-turn")
```

Fail-open matches the file's house style (OTEL, Prometheus, IG threads all fail-open at boot).
If prewarm fails, `_get_warm_brain()` is retried lazily on the first OWNER turn; if that also
fails, `ask` returns a cold-path error string and the turn still completes.

**Session-id persistence.** First spawn generates a UUID (`uuid.uuid4()`), writes it to
`/state/warm_session_id` (jarvis-state PVC), and passes `--session-id <uuid>`. Reuse the file's
id on respawn so `--resume <uuid>` recovers history after a process death **or a pod restart**
(the PVC survives restarts; the `claude` CLI persists session transcripts under the
`/state/.claude/` subPath the brain already uses). If the file is absent, mint a new id.

**Reuse.** Every OWNER turn calls `_get_warm_brain().ask(...)`; the singleton holds the live
process. No per-turn spawn.

**Respawn-on-death.** `ask` checks `_alive()` first and on pipe errors mid-turn. Respawn uses
`--resume <persisted id>` to rejoin the conversation. Respawn re-launches MCP servers (one-time
cost on death, not per turn). Cap respawns to one retry per turn to avoid a crash loop wedging
the voice loop; on repeated failure, `ask` returns the cold-path connection-error string and
(optionally) `brain_respond` could be made to fall through to `_claude_brain` cold — but keep
v1 simple: return the error string, let the owner retry, lazy respawn on next turn.

**Shutdown.** On `shutdown_request` (the agent shutdown flow) / SIGTERM, call
`_warm_brain.shutdown()` to close stdin and terminate the child so MCP servers don't orphan.
Wire into `main()`'s teardown / a `try/finally` around the mic loop.

---

## 5. Error-envelope parity (do not regress metrics)

`brain_respond` classifies the brain's **return string** into Prometheus reasons
(`edge.py:1220-1232`): `"no brain credentials"`, `"took too long"`, `"lost my connection"`,
`"blank"`/`"didn't have anything"`, `"something went wrong"`, `"brain error"`. `_WarmBrain.ask`
**must return these exact strings** for the matching failure classes so the existing classifier
keeps slicing correctly with zero changes to `brain_respond`'s metric block:

| Condition (warm) | Return string (must match cold `_claude_brain`) |
|---|---|
| no creds + no API key | `"No brain credentials, sir — neither subscription nor API key configured."` |
| turn deadline exceeded | `"That took too long, sir — try again."` |
| process dead, respawn failed | `"I lost my connection there, sir."` |
| `result` event empty | `"I didn't have anything to say there, sir."` |
| `is_error: true` | `"Something went wrong, sir — try again."` |
| unexpected exception | `f"Brain error, sir — {exc}"` |

`METRIC_BRAIN_DURATION` and the trace span are recorded by `brain_respond` around the call, so
warm turns are measured identically to cold turns — and you'll *see* the latency win in the
existing `jarvis_brain_duration_seconds` histogram with no new instrumentation.

---

## 6. Risks & mitigations

- **Warm-session memory bleed (the critical risk).** A non-owner turn entering the owner
  session leaks tools + history + (Phase 6) memories. **Mitigation:** OWNER-only by
  construction — only `gate_and_respond`'s OWNER branch reaches `brain_respond`; the singleton
  is global and OWNER-scoped; `brain_respond` asserts `mem_scope=="owner"` before the warm path
  and degrades to cold otherwise (§2, §4.3). TRUSTED Layer-A (no `--mcp-config`) is unchanged.
- **Prompt-cache interaction (coordinate with the cache-optimization workstream).** A warm
  stream-json session is exactly the shape prompt caching rewards: a stable `--append-system-prompt`
  prefix (persona + tools, set once at spawn) followed by a growing message tail. **Conceptual
  guidance for that workstream:** keep the spawn-time system prompt **byte-stable** — do NOT
  fold the per-turn wall-clock or persona-tuning into the system prompt (we put them in the
  per-turn *user* text precisely so the cached prefix never changes; see §3.4). Reordering tools
  or editing the persona system prompt mid-session would invalidate the prefix cache. The
  cache-optimization workstream owns breakpoint placement; this design's contract with it is:
  *the warm prefix is frozen at spawn; volatile context rides the user turn.* Cross-check with
  that team before changing where `_now_context()`/persona lands.
- **Session bloat over a long day.** A session that runs for hours grows unboundedly and can
  approach the model's context window. **Mitigation:** the CLI handles its own context
  management; if turns start failing with context errors, respawn with a fresh session id
  (drop history) — acceptable degradation. Phase 6 mem0 is the durable-memory answer; the warm
  session is conversational working memory, not the system of record. Add a max-session-age
  (e.g. respawn the session every N hours or M turns) as a follow-up knob if bloat is observed.
- **GPU/MCP resource hold.** The warm process keeps six MCP servers + the CLI resident. That's
  RAM, not GPU (the brain is API/subscription-backed; the GPU is whisper/tts/ollama). Net RAM
  is comparable to the cold path's peak (which also spawns all six per turn) but now sustained.
  Within the pod's 16–48Gi. No new GPU contention with @chem-engineer's workers.
- **Concurrency (mic loop + Phase-3 HTTP ingest).** Both can call `gate_and_respond`. The
  per-instance turn lock serializes turns on the single conversation (correct — concurrent
  owner turns are nonsensical; the second waits). The lock must not be held across the full
  read deadline in a way that wedges the mic loop indefinitely — the turn deadline bounds it.
- **Wedged pipe / zombie child.** A model that streams partial output then stalls wedges the
  reader. **Mitigation:** the queue-based reader + per-turn deadline (§4.2); on deadline,
  mark-for-respawn so the next turn starts clean.
- **Token refresh on a long-lived process.** The CLI refreshes OAuth tokens onto the PVC; a
  multi-hour process refreshes in place (same as the Discord bot's expectations). If a refresh
  fails mid-session, the next turn surfaces a creds error string and respawn re-reads the
  refreshed file.

---

## 7. Verification plan

Run from `kubectl -n ai`. Compare against the existing cold-path baseline.

1. **Prewarm at boot.** Restart the pod; `kubectl logs -n ai deploy/jarvis-stack -c jarvis-edge`
   shows `warm brain: OWNER session prewarmed`. `ps` inside the pod shows ONE persistent
   `claude` process + its `jarvis_*_mcp.py` children, present *before* the first turn.
   ```bash
   kubectl exec -n ai deploy/jarvis-stack -c jarvis-edge -- ps aux | grep -E 'claude|jarvis_.*_mcp'
   ```
2. **Reuse (no per-turn spawn).** Speak two OWNER turns. The `claude` PID does **not** change
   between turns (warm). The MCP children PIDs do **not** change. Contrast: on the cold path the
   PIDs would differ every turn.
3. **Continuity (the mem0 prereq).** OWNER: "what's the weather" → answer. Then: "and
   tomorrow?" → the session resolves "tomorrow" against the prior weather turn (cold path
   cannot — it has no prior turn). Confirms the warm conversation is live.
4. **Latency win.** Watch `jarvis_brain_duration_seconds` (Prometheus / the jarvis voice
   dashboard, @monitor-engineer). Warm OWNER turns should drop the ~2–3 s MCP cold-start vs the
   cold-path baseline. Same metric, no new instrumentation.
5. **Tool turn warm.** OWNER: "is the cluster healthy" → invokes `mcp__jarvis_kube__*` on the
   warm session, returns an answer; PID unchanged after.
6. **Isolation (the key guard).** A TRUSTED voice asks a general question → log shows
   `_claude_brain_voice_locked` (no `--mcp-config`), and the warm `claude` PID is **unchanged**
   (the trusted turn never touched it). A TRUSTED owner-referential query → deflected pre-brain
   (Layer B), warm PID unchanged, no subprocess spawned.
7. **Respawn-on-death.** `kubectl exec ... -- kill <warm claude pid>`; next OWNER turn logs a
   respawn, a NEW `claude` PID appears, and the turn still answers — and references the prior
   conversation (proves `--resume <persisted id>` recovered history).
8. **Pod-restart recovery.** Restart the pod; the new warm session resumes the persisted
   `/state/warm_session_id` (history survives via the PVC), or mints a new id cleanly if the
   file is absent.
9. **Kill-switch.** Set `WARM_BRAIN=0`, restart; OWNER turns use the cold `_claude_brain`
   (per-turn spawn returns), confirming instant rollback.
10. **Error parity.** Force a creds failure (rename `~/.claude/.credentials.json`, unset
    `ANTHROPIC_API_KEY`); the warm `ask` returns the same `"No brain credentials, sir…"` string
    and `jarvis_brain_errors_total{reason="no_credentials"}` increments — proving the metric
    classifier still works against warm-path return strings.

---

## 8. Out of scope for Phase 5 (handoffs)

- **mem0 wiring (Phase 6)** — this design only guarantees a stable `mem_scope="owner"` env and a
  live conversation for the memory MCP to attach to.
- **Image build** — custom edge image builds on ARC→zot (@tooling-engineer); after merge,
  restart the pod to pick it up.
- **Prompt-cache breakpoint placement** — coordinate with the cache-optimization workstream
  (§6); this design's only contract is "frozen spawn-time prefix, volatile per-turn user text."
- **TRUSTED/Discord/IG warm sessions** — explicitly NOT done; those stay stateless/cold by the
  isolation requirement.
