# JARVIS Phase 7 — Full-duplex audio: AEC / barge-in (design spec)

**Status:** design only. No code changed. User-flagged must-have (plan Phase 7).
**Scope:** Stage-1 (capture) of the spine. The brain, gate, TTS, and Sonos output stages
are untouched conceptually — this is about whether the mic can stay live while JARVIS speaks.

---

## TL;DR recommendation

**Do NOT attempt classic reference-signal AEC against the Sonos path. It cannot lock.**
Instead, build a **lightweight always-on "interrupt spotter"** that runs *only* while
`_jarvis_speaking` is True: a parallel, cheap detection pass on the live mic that looks for
the owner addressing JARVIS ("jarvis", "stop", "wait", "cancel"), and on a hit **stops the
Sonos playback immediately** (`sonos.stop()` / `pause`), clears the echo-suppress flag, and
lets the normal capture→STT→gate→brain pipeline take the next utterance. This is the same
strategy the openjarvis Mac daemon uses (`_StopSpotter`), and it is the realistic, robust
answer here.

Keep `ECHO_SUPPRESS_S` and `_jarvis_speaking` exactly as they are for the *non-interrupt*
case — they remain the cheap, correct defense against JARVIS transcribing itself. Phase 7
adds an **escape hatch** (barge-in) on top of echo-suppression rather than replacing it
with AEC.

The rest of this doc justifies why full AEC is impractical with Sonos, what reference signal
is (and isn't) obtainable, the three honest options with trade-offs, and an `edge.py`
integration sketch for the recommended path plus a fallback "local-playback AEC" path if the
user ever decides the Sonos-in-the-room constraint can be dropped.

---

## 1. The current state (what we're replacing)

Half-duplex via **echo-suppression**, not AEC:

- `_jarvis_speaking` (bool) set True the moment `_stream_on_sonos_impl` starts enqueuing,
  False when the Sonos queue drains (`edge.py` ~1703 / ~1792).
- `_jarvis_done_at` (timestamp) set when playback finishes.
- The capture loop's echo gate (`edge.py` ~2040): if `_jarvis_speaking` **or** the utterance
  started within `ECHO_SUPPRESS_S` (3.5s) of `_jarvis_done_at`, the utterance is **dropped
  before STT**. This is what kills the "Nineteen, sir. → 19, sir. → Nineteen what, sir?"
  self-feedback loop.

Consequence: **the owner physically cannot interrupt.** Anything said while JARVIS talks (and
for 3.5s after) is discarded sight-unseen. The mic is effectively muted during playback.

Why this exists at all: the Yeti on nixos-gpu and the Sonos Play:1 in the bedroom share an
acoustic space. The Yeti *will* pick up JARVIS's own TTS. Without suppression, JARVIS
transcribes itself and replies to itself forever.

---

## 2. What classic AEC needs — and why Sonos breaks it

Acoustic Echo Cancellation (WebRTC `audioprocessing`, SpeexDSP `speex_echo`) is an
**adaptive filter**. Conceptually:

```
mic_in(t)  =  near_speech(t)  +  echo(t)
echo(t)    ≈  (room_impulse_response) ⊛ reference(t − Δ)
AEC output =  mic_in(t)  −  estimate{ filter ⊛ reference(t − Δ) }
```

For the adaptive filter (NLMS/MDF) to converge, it needs:

1. **The reference signal** — the exact samples being played out the speaker, available to
   the canceller.
2. **Tight, stable time alignment** — `reference(t)` and `mic_in(t)` sample-aligned to within
   the filter's tail length (tens of ms), with **bounded, low jitter**. The filter tracks
   slow room changes; it cannot track playback-latency that wanders by hundreds of ms.
3. **Same clock domain (or a known fixed offset)** — sample-rate drift between the capture
   device clock and the playback device clock must be small or compensated.

Now map that onto **TTS → Sonos over the network**:

| Requirement | Local soundcard | This setup (Sonos) |
|---|---|---|
| Reference samples available | Yes — loopback / what you wrote to the DAC | **No.** We hand Sonos an *HTTP URL to a WAV* (`add_uri_to_queue` + `play_from_queue`). We never see the played PCM stream. We know *what* WAV, not *when each sample hits the air*. |
| Time alignment mic↔ref | Sub-ms, hardware loopback | **Unknown + variable.** Latency from `play_from_queue(0)` to first-sound-in-room is network fetch + Sonos buffer + DAC + acoustic propagation. Soco gives no sample-accurate "now playing sample N" callback. The Sonos transport state polls at 0.4s granularity in our own code. |
| Jitter | Negligible | **Hundreds of ms, non-stationary.** Sonos buffers adaptively; Wi-Fi reorders/retransmits; queue transitions between sentence chunks add gaps. |
| Clock domain | One clock (same box) | **Two independent clocks** — the Yeti ADC on nixos-gpu and the Sonos DAC across the room. Free-running, drifting relative to each other. |
| Acoustic path | Speaker→mic, fixed-ish | Speaker (bedroom Play:1) and mic (Yeti) are **physically separated across the room**, long and variable room impulse response. |

**Bottom line:** AEC's hard precondition — a sample-aligned, low-jitter reference — is exactly
the thing we cannot obtain on the Sonos path. We have the *content* of the reference (the WAV
bytes we synthesized) but not its *timing in the acoustic domain*, and the timing is the part
AEC depends on. An adaptive filter fed a reference that floats ±hundreds of ms against the
mic will never converge; it will at best do nothing and at worst inject artifacts. This is the
crux, and it is not a tuning problem — it is a missing-signal problem.

### Could we estimate the delay and align?

In principle: cross-correlate the known reference WAV against the mic capture to estimate Δ,
then feed AEC the time-shifted reference. Problems:
- Δ is **not constant** within a turn (per-sentence queue gaps, Wi-Fi jitter) — a single Δ
  estimate is stale within ~1s.
- Continuous online Δ re-estimation (a delay-tracking AEC like WebRTC's "extended filter" /
  delay-agnostic mode) is designed to absorb *tens of ms* of drift, not the multi-hundred-ms,
  bursty drift of a networked speaker.
- Clock drift means even a perfect Δ at t=0 walks off over a multi-second reply.
- We'd be building a bespoke delay-tracking front-end in front of WebRTC AEC, all to handle a
  case where a much simpler strategy (interrupt spotter) gives the user-visible behavior they
  actually want (barge-in) without needing cancellation at all.

This is the "is ther no other distributed duckling" question from the plan title: the elegant
distributed-AEC duckling doesn't exist for networked speakers. The honest answer is to stop
chasing reference alignment.

---

## 3. The three options

### Option A — Full reference-signal AEC against Sonos ❌ (rejected)

Feed WebRTC/Speex AEC the synthesized WAV as reference, with a delay estimator.

- **Pro:** mic could stay truly live; cleanest conceptual model *if it worked*.
- **Con:** It won't lock (Section 2). Two clocks, no loopback, hundreds-of-ms non-stationary
  jitter. High effort, low/zero payoff, fragile. Adds `webrtc-audio-processing` native dep
  and a custom delay tracker for a result that degrades the moment Wi-Fi hiccups.
- **Verdict:** Reject. Not a tuning problem; a missing-signal problem.

### Option B — Interrupt spotter (barge-in without cancellation) ✅ (recommended)

While JARVIS is speaking, run a **cheap parallel detector** on the live mic for a small set
of interrupt cues. On a hit: **stop Sonos playback now**, drop the echo flag, and hand the
turn to the normal pipeline. We never cancel echo; we just *stop the source of the echo* the
instant the owner wants the floor.

- **Pro:**
  - Delivers the user-visible behavior actually requested: "owner can talk over JARVIS and
    JARVIS shuts up and listens."
  - **No reference alignment required** — we don't subtract anything, so Sonos timing jitter
    is irrelevant.
  - Reuses what's already in the box: Silero VAD, the `_jarvis_speaking` flag, and (optionally)
    a tiny keyword/wake check. Once playback is stopped, echo-suppression is moot because the
    speaker is silent.
  - This is the proven openjarvis `_StopSpotter` design — known to work in the same acoustic
    setup.
- **Con:**
  - Not "full duplex" in the textbook sense — JARVIS stops rather than continuing to talk
    while also hearing you. For a butler assistant this is the *desired* UX (you interrupt, it
    yields), not a limitation.
  - Need a detector robust to JARVIS's own voice triggering it (self-trigger). Mitigations
    below.
- **Verdict:** Recommend. Highest payoff/effort ratio; matches the real requirement.

### Option C — Local-playback AEC (drop Sonos for the room) ⚠️ (fallback / future)

If the user ever decides the bedroom speaker can be a **locally-attached** output on
nixos-gpu (USB speaker / line-out) instead of Sonos, then *real* AEC becomes feasible: we own
the playback PCM stream and both ends share (or can be made to share) a clock, so
WebRTC/Speex AEC can lock.

- **Pro:** true full-duplex; reference + alignment both obtainable.
- **Con:** abandons the whole point of the current design (room audio via the nice Sonos
  Play:1; music snapshot/restore; multi-room). Requires a soundcard + speaker on the GPU box.
  Big behavioral change. Out of scope for Phase 7 as currently framed.
- **Verdict:** Document as the only path to *textbook* full-duplex, but not recommended unless
  the Sonos-in-the-room requirement is dropped. Keep the AEC integration notes here so the
  work is shovel-ready if that decision is ever made.

---

## 4. Recommended design — the Interrupt Spotter (Option B)

### 4.1 Behavior

```
JARVIS starts speaking (Sonos queue playing)
   │
   ├─ normal capture loop: echo-suppress ON (unchanged) — drops self-echo, no replies
   │
   └─ interrupt spotter (NEW, runs only while _jarvis_speaking):
        listen on a SEPARATE short-frame VAD pass
        on detected owner-interrupt cue:
            sonos.stop()                      # kill the echo source immediately
            _jarvis_speaking = False
            _jarvis_done_at = 0               # disable the post-roll grace for THIS turn
            signal main loop: "take the next utterance, skip echo-suppress once"
```

After the stop, the speaker is silent within ~1 fetch/buffer cycle, so the very next
utterance the main loop captures is clean (no echo to suppress), and it flows through
`gate_and_respond` normally.

### 4.2 What counts as an "interrupt cue"

Two tiers, cheapest first:

1. **Energy + VAD barge-in (coarse):** sustained near-field speech energy during playback
   that exceeds an adaptive threshold. Fast, no STT. Risk: triggers on the owner's *and*
   on JARVIS's own echo. Use only as a *gate* into tier 2, never to stop on its own.
2. **Keyword confirm (precise):** on a tier-1 gate, run a *very* short STT or an
   openWakeWord pass over the last ~1s and require one of `{"jarvis", "stop", "wait",
   "cancel", "hold on", "nevermind"}`. Only then stop. This rejects self-echo because
   JARVIS's TTS won't contain those interrupt tokens (and if it ever does, see 4.4).

openWakeWord is **already in the base image** (`Dockerfile.base:45`, currently unused since
ambient mode replaced wake-word). That makes it the natural "stop spotter" — a small model
that fires on "jarvis" with low latency and no GPU STT round-trip. Recommend reviving it
**only** for the interrupt path (a `stop`/`jarvis` model), not for general wake.

### 4.3 Self-trigger problem and mitigations

The Yeti hears JARVIS's own TTS. We must not let JARVIS interrupt itself.

- **Token filter (primary):** require an interrupt *keyword* (tier 2). JARVIS's replies are
  butler-terse answers; they essentially never contain "stop / wait / cancel / nevermind".
  "jarvis" is the risk — JARVIS could say its own name. Mitigate by **excluding "jarvis" from
  the interrupt vocabulary during self-speech** and relying on `{stop, wait, cancel, hold on,
  nevermind}` plus a strong energy gate; OR
- **Known-output suppression (secondary):** we *do* have the reference WAV content (just not
  its acoustic timing). A loose, *non-real-time* check — "is the spotter's captured text a
  substring of what JARVIS is currently saying?" — can veto a candidate interrupt. This is
  NOT AEC (no sample alignment needed); it's a text-level sanity check that's robust to
  timing jitter. Cheap and effective against self-echo false-fires.
- **Directional/energy bias:** the owner near the Yeti is louder than the Play:1 across the
  room. An adaptive energy threshold (calibrated from the current echo floor) raises the bar
  so only near-field speech passes the tier-1 gate.

### 4.4 Where it plugs into `edge.py`

This is **Stage-1 only**. No change to gate/brain/TTS. Touch points:

- **New module recommended:** `jarvis_barge_in.py` (keeps `edge.py` from growing; mirrors how
  identity/discord/ig are split out). Exposes `start_spotter(stream_or_factory, on_interrupt)`
  and a thread that only does work while `_jarvis_speaking` is True.
- **Globals:** reuse `_jarvis_speaking` / `_jarvis_done_at` (module-level in `edge.py`,
  ~1656). Add a `_interrupt_requested = threading.Event()` the spotter sets and the main loop
  checks/clears.
- **Mic contention:** the Yeti `InputStream` is opened once in `main()` (`edge.py` ~1956) and
  read serially by the capture loop. PortAudio/sounddevice does **not** support two
  `InputStream`s on the same device cleanly. So the spotter **must not open its own stream.**
  Two viable wirings:
  - **(preferred) Single reader, fan-out:** keep one `InputStream`; in the read path, when
    `_jarvis_speaking`, also push each chunk into a `queue.Queue` the spotter thread drains.
    The capture loop already reads `stream.read(native_chunk)` in its inner `while` — but
    note that inner loop only runs *between* turns. During playback the main loop is blocked
    inside `_stream_on_sonos` waiting for the queue to drain (`edge.py` ~1774). **This is the
    key structural issue:** today nobody reads the mic while JARVIS speaks. So we need a
    dedicated reader during playback.
  - **(cleaner) Move playback off the main thread:** run `_stream_on_sonos` in a worker
    thread so the main capture loop keeps reading the mic during playback; the spotter logic
    can then live *in the main loop* (check interrupt cues on each chunk while
    `_jarvis_speaking`). On interrupt, signal the playback worker to `sonos.stop()`. This is
    the more invasive but more correct refactor and is the recommended target.

Recommended: **the second wiring.** Make playback non-blocking so the existing single mic
reader stays live during speech; add the interrupt check inline where echo-suppress is today.

- **Echo-suppress interaction (`edge.py` ~2040):** today the gate is "drop if speaking or
  within grace." Change to: while `_jarvis_speaking`, instead of *dropping*, run the utterance
  through the **interrupt classifier**. If it's an interrupt cue → `sonos.stop()`, clear flags,
  and let *this same utterance* (or the next one) proceed to STT/gate. If it's NOT an interrupt
  cue → drop as echo (current behavior preserved). After playback ends, keep a *shortened*
  `ECHO_SUPPRESS_S` post-roll (echo tail from the room) but allow interrupts to bypass it.

### 4.5 Integration sketch (illustrative — NOT to be applied)

```python
# edge.py — module globals (near _jarvis_speaking, ~1656)
_interrupt_requested = threading.Event()
_INTERRUPT_TOKENS = ("stop", "wait", "cancel", "hold on", "nevermind", "jarvis")
_playback_thread: threading.Thread | None = None

def _is_interrupt(text: str) -> bool:
    low = (text or "").lower()
    # Veto self-echo: if the captured text is a substring of what JARVIS is
    # currently saying, it's the Play:1 bleeding into the Yeti — not the owner.
    if _current_tts_text and low.strip() and low.strip() in _current_tts_text.lower():
        return False
    return any(tok in low for tok in _INTERRUPT_TOKENS)

# --- make playback non-blocking so the mic stays live during speech ---
def _speak_async(sonos, sentences, host_ip, port, turn_n, stash):
    global _playback_thread
    def _run():
        _stream_on_sonos(sonos, sentences, host_ip, port, turn_n, stash)
    _playback_thread = threading.Thread(target=_run, daemon=True)
    _playback_thread.start()

# --- in the capture loop, replace the echo-DROP with echo-OR-INTERRUPT ---
# (edge.py ~2040, inside the turn try-block, after we have audio_16k + STT)
if _jarvis_speaking:
    # We're hearing audio while JARVIS talks. Transcribe the short window
    # ONLY to check for an interrupt cue (cheap STT or openWakeWord).
    cue = res.get("text", "")            # already transcribed above
    if _is_interrupt(cue):
        print(f"  BARGE-IN: {cue!r} — stopping Sonos")
        try:
            sonos.stop()
        except Exception:
            pass
        _jarvis_speaking = False
        _jarvis_done_at = 0.0            # skip post-roll grace this turn
        # fall through: let this utterance (or the next) reach gate_and_respond
    else:
        # not an interrupt → it's self-echo, drop exactly as today
        turn_outcome = "echo_drop"; METRIC_ECHO_DROPS.inc(); continue
```

Notes on the sketch:
- `_current_tts_text` would be set by `_stream_on_sonos_impl` to the concatenated reply text
  (the self-echo veto in 4.3). This is the only "reference" we use — **text**, not aligned PCM.
- The cheap-STT-on-interrupt path can be the existing whisper round-trip (one extra ~290ms
  call only while speaking) or a revived openWakeWord model for sub-100ms, GPU-free detection.
  Recommend openWakeWord for the spotter to avoid hammering whisper during every reply.
- The non-blocking playback (`_speak_async`) is the main structural change; everything else is
  additive around the existing echo gate.

### 4.6 New metrics (extend the existing Prometheus block, ~155)

- `jarvis_barge_in_total{outcome="stopped|self_veto|missed"}` — interrupts detected, vetoed
  as self-echo, or (later, via user report) missed.
- `jarvis_barge_in_latency_seconds` — cue-detected → `sonos.stop()` returned. Watch this;
  Sonos stop has its own SOAP latency (~100–300ms).

### 4.7 Dependencies

- **No new heavy deps for Option B.** openWakeWord + tflite-runtime are already in
  `Dockerfile.base` (lines 45–50). Silero VAD already loaded. soco already used.
- If the spotter uses cheap-STT instead of openWakeWord, **zero** new deps (reuses whisper).
- (Option C / future AEC only) would add `webrtc-audio-processing` (native lib) — *not*
  needed for the recommended path.

---

## 5. If the user insists on textbook full-duplex AEC (Option C notes)

Only viable by dropping Sonos as the room output. Sketch for completeness:

- Output to a **local** device on nixos-gpu (USB speaker / line-out) via sounddevice
  `OutputStream`, so we own the PCM and can capture a loopback reference.
- Use `webrtc-audio-processing` (Python bindings: `webrtcvad` is *only* VAD; for AEC use
  `webrtc-audio-processing` / `pywebrtc` style bindings, or `speexdsp` `speex_echo_*`).
- Run capture and playback from the **same** sounddevice callback (shared clock) so reference
  alignment is sample-accurate; feed `(near=mic_block, far=played_block)` to the canceller
  each block.
- Then the mic genuinely stays live and JARVIS can keep talking while hearing the owner.

Trade-off: loses Sonos multi-room, the music snapshot/restore (`Snapshot`, ~1714), and the
"speaks from the nice bedroom Play:1" experience. Recommend against unless explicitly chosen.

---

## 6. Honest recommendation (restated)

- **Ship Option B (interrupt spotter).** It is the realistic, robust way to give the owner
  barge-in over a networked Sonos speaker, it reuses assets already in the image, and it's the
  proven openjarvis approach. Full reference-AEC against Sonos is a missing-signal problem and
  will not converge — don't build it.
- **Keep echo-suppression** for the non-interrupt case; Phase 7 layers barge-in *on top*.
- The one real refactor is making **playback non-blocking** so the single mic stream stays
  live during speech; the rest is additive around the existing echo gate.
- **Document Option C** as the only route to textbook full-duplex, gated on dropping the
  Sonos-in-the-room requirement — a product decision, not an engineering tuning task.

---

## 7. Verification (per plan §9 "AEC barge-in without echo self-loop")

1. Owner asks a long question → JARVIS starts a multi-sentence reply on Sonos.
2. Owner says "jarvis stop" mid-reply → Sonos playback halts within ~300ms; no self-reply
   loop; next utterance is taken cleanly. `jarvis_barge_in_total{outcome="stopped"}` +1.
3. Negative: JARVIS speaks a reply that happens to contain "stop" as content (rare) → the
   self-echo text veto fires; `outcome="self_veto"`; playback continues.
4. Negative: ambient cross-talk during playback that is NOT an interrupt cue → dropped as echo
   exactly as today (`jarvis_echo_drops_total` +1); no false barge-in.
5. Latency: `jarvis_barge_in_latency_seconds` p95 < ~0.4s.
