"""jarvis_edge — thin satellite for JARVIS.

Architecture:

  [ EDGE BOX (Surface Pro, Pi, etc.) ]   [ CLUSTER + Sonos ]
    wake-word listen (openWakeWord)
    audio capture (VAD-endpointed)  ──→  whisper-stt :8766
                                         ←── transcript
    brain(transcript)                     (stub for now → swap for real brain)
    text                            ──→  chatterbox :8765 /synthesize
                                         ←── WAV bytes
    serve WAV via embedded HTTP     ──→  Sonos play_uri(http://edge:8088/turn.wav)
    wait for playback to finish

Config via env vars (or edit defaults below):

    STT_URL       http://10.44.0.20:8766
    TTS_URL       http://10.44.0.8:8765
    SONOS_IP      (REQUIRED — Play:5 LAN IP)
    EDGE_HTTP_PORT 8088  (port we serve WAV on; reachable from Sonos)
    EDGE_HOST_IP  (optional — IP we tell Sonos to fetch from; defaults to
                   auto-detect of the route used to reach SONOS_IP)
    BRAIN_MODE    echo  (stub) | api (anthropic) — for now stay on `echo`
"""
from __future__ import annotations

import io
import json
import os
import socket
import sys
import tempfile
import threading
import time
import urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer
from math import gcd

import numpy as np
import sounddevice as sd
from openwakeword.model import Model as OWWModel
from scipy.signal import resample_poly

# ── Config ────────────────────────────────────────────────────────────────────
STT_URL = os.environ.get("STT_URL", "http://10.44.0.20:8766")
TTS_URL = os.environ.get("TTS_URL", "http://10.44.0.8:8765")
SONOS_IP = os.environ.get("SONOS_IP", "")
EDGE_HTTP_PORT = int(os.environ.get("EDGE_HTTP_PORT", "8088"))
# Port that Sonos hits in the URL — may differ from EDGE_HTTP_PORT when
# the pod is exposed via a NodePort Service (containerPort 8088 maps to
# nodePort 30088, for example). Defaults to EDGE_HTTP_PORT for direct
# host-network deployments.
EDGE_ADVERTISED_PORT = int(os.environ.get("EDGE_ADVERTISED_PORT",
                                          str(EDGE_HTTP_PORT)))
EDGE_HOST_IP = os.environ.get("EDGE_HOST_IP", "")
BRAIN_MODE = os.environ.get("BRAIN_MODE", "claude")

WAKE_THRESHOLD = 0.65        # was 0.5 — bumped to cut kitchen false-fires
SILENCE_SECS = 0.6
MAX_UTTERANCE_SECS = 8.0
MIN_UTTERANCE_SECS = 0.4
ADDRESSEE_WINDOW_S = 6.0     # was 20 — shorter so a false-fire's window
                              # closes before a kitchen voice can sneak in
OWW_RATE = 16000
OWW_CHUNK = 1280
SONOS_VOLUME = int(os.environ.get("SONOS_VOLUME", "60"))  # 0-100


# ── Mic resolve ──────────────────────────────────────────────────────────────
def _pick_mic() -> tuple[int, int, int]:
    """Pick the best input device. MIC_NAME env (case-insensitive substring)
    wins if set; else prefer Yeti / Blue / Microphones / Webcam / ReSpeaker
    / USB; fallback to system default."""
    wanted = (os.environ.get("MIC_NAME") or "").lower().strip()
    candidates = ("yeti", "blue", "microphones", "respeaker",
                  "webcam", "c922", "usb")
    mic_idx = None
    for i, d in enumerate(sd.query_devices()):
        if d.get("max_input_channels", 0) > 0 and d["max_input_channels"] <= 4:
            name = d["name"].lower()
            if wanted and wanted in name:
                mic_idx = i
                break
            if not wanted and any(s in name for s in candidates):
                mic_idx = i
                break
    if mic_idx is None:
        info = sd.query_devices(kind="input")
        mic_idx = info["index"] if isinstance(info, dict) else None
    dev = sd.query_devices(mic_idx)
    native_rate = int(dev["default_samplerate"])
    native_chunk = int(OWW_CHUNK * native_rate / OWW_RATE)
    print(f"mic: {dev['name']}  native={native_rate}Hz")
    return mic_idx, native_rate, native_chunk


def _to_16k(arr: np.ndarray, sr: int) -> np.ndarray:
    if sr == 16000:
        return arr.astype(np.float32)
    g = gcd(sr, OWW_RATE)
    return resample_poly(arr, OWW_RATE // g, sr // g).astype(np.float32)


# ── Cluster STT ──────────────────────────────────────────────────────────────
def transcribe(audio_16k: np.ndarray) -> dict:
    t0 = time.time()
    body = audio_16k.astype(np.float32).tobytes()
    req = urllib.request.Request(
        f"{STT_URL}/v1/transcribe?sr=16000",
        data=body,
        headers={"Content-Type": "application/octet-stream"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=10.0) as r:
            data = json.loads(r.read())
    except Exception as exc:
        return {"text": "", "error": str(exc), "wire_ms": int((time.time()-t0)*1000)}
    data["wire_ms"] = int((time.time() - t0) * 1000)
    return data


# ── Brain: real Claude (Haiku for fast turns) ────────────────────────────────
# Two modes:
#   echo    — debug stub, returns f"Sir, I heard: {text}"
#   claude  — invokes the `claude` CLI with the JARVIS persona system
#             prompt, optional MCP config for tool calls. Uses
#             ANTHROPIC_API_KEY (read from the jarvis-secrets Secret).
#
# Cold per-turn (no persistent session yet — that's a future port of
# openjarvis's ClaudeCodeBrain). Adds ~2-3s vs the Mac daemon's warm
# stream-json session, but keeps the implementation small.

# Persona — short JARVIS butler tone, terse, real time-of-day responses.
_PERSONA_SYSTEM = """You are JARVIS, the assistant from Iron Man. Address the user as "sir".

Speech rules — output WILL be read aloud through a Sonos speaker:
- Keep replies under 25 words by default.
- Never use markdown, URLs, code blocks, lists, asterisks, or bullets.
- Spell out numbers when natural ("forty-two" not "42") in casual contexts.
- No filler like "let me check…" — just answer.
- Decline to read URLs out loud.

Tool use:
- When the user asks about the TV (on/off, volume, mute, switch input,
  what's playing, launch an app, play a Plex show), use the
  mcp__jarvis_tv__* tools. Never invent that you did something — only
  report what the tool actually returned.

You are running on a cluster pod with a microphone in the kitchen / a
Sonos in the bedroom. The current owner is Hampton."""

_MCP_CONFIG_PATH = "/tmp/jarvis_mcp.json"


def _write_mcp_config() -> None:
    """Materialize the MCP server config for the brain to consume."""
    cfg = {
        "mcpServers": {
            "jarvis_tv": {
                "command": "python3",
                "args": ["/app/jarvis_tv_mcp.py"],
            },
        }
    }
    with open(_MCP_CONFIG_PATH, "w") as f:
        json.dump(cfg, f)


_RO_ALLOWED_TOOLS = " ".join([
    "mcp__jarvis_tv__tv_status",
    "mcp__jarvis_tv__tv_power",
    "mcp__jarvis_tv__tv_volume_set",
    "mcp__jarvis_tv__tv_volume_step",
    "mcp__jarvis_tv__tv_mute",
    "mcp__jarvis_tv__tv_inputs",
    "mcp__jarvis_tv__tv_input_set",
    "mcp__jarvis_tv__tv_launch_app",
    "mcp__jarvis_tv__tv_youtube_play",
    "mcp__jarvis_tv__tv_spotify_play",
    "mcp__jarvis_tv__tv_plex_play",
    "mcp__jarvis_tv__tv_plex_search",
    "mcp__jarvis_tv__tv_plex_libraries",
    "mcp__jarvis_tv__tv_cast_url",
    "mcp__jarvis_tv__tv_cast_stop",
    "WebFetch",
    "WebSearch",
])


def _claude_brain(text: str, timeout: float = 30.0) -> str:
    """Subprocess `claude` with the persona + MCP config."""
    import subprocess as _sp
    if not os.environ.get("ANTHROPIC_API_KEY"):
        return "API key missing, sir — I can't reach my brain."
    try:
        proc = _sp.run(
            ["claude", "-p", text,
             "--append-system-prompt", _PERSONA_SYSTEM,
             "--mcp-config", _MCP_CONFIG_PATH,
             "--allowed-tools", _RO_ALLOWED_TOOLS,
             "--model", "claude-haiku-4-5-20251001",
             "--max-turns", "6",
             "--output-format", "text"],
            capture_output=True, text=True, timeout=timeout,
        )
        if proc.returncode != 0:
            print(f"  brain stderr: {proc.stderr[:300]}")
            return "I lost my connection there, sir."
        return (proc.stdout or "").strip()
    except _sp.TimeoutExpired:
        return "That took too long, sir — try again."
    except Exception as exc:  # noqa: BLE001
        return f"Brain error, sir — {exc}"


def brain_respond(text: str) -> str:
    if BRAIN_MODE == "echo":
        return f"Sir, I heard: {text}"
    return _claude_brain(text)


# ── Cluster TTS ──────────────────────────────────────────────────────────────
# Load the JARVIS persona voice once at startup so every /synthesize call
# can include it as audio_prompt_b64 — Chatterbox is zero-shot voice
# cloning, no training, just give it ~10-30s of reference and every
# generation comes out in that voice. Without it Chatterbox uses its
# default voice (not the persona).
import base64 as _base64
_VOICE_REF_PATH = os.environ.get("VOICE_REF", "/app/jarvis_voice.wav")
_VOICE_PROMPT_B64 = ""
try:
    with open(_VOICE_REF_PATH, "rb") as _f:
        _VOICE_PROMPT_B64 = _base64.b64encode(_f.read()).decode("ascii")
    print(f"voice ref loaded: {_VOICE_REF_PATH} "
          f"({len(_VOICE_PROMPT_B64) * 3 // 4} bytes)")
except Exception as _exc:
    print(f"voice ref unavailable ({_exc}) — Chatterbox will use default voice")


def tts_synthesize(text: str) -> bytes | None:
    """POST text to chatterbox, return WAV bytes. Includes the persona
    voice reference if loaded so output is cloned to that voice."""
    t0 = time.time()
    payload: dict = {"text": text}
    if _VOICE_PROMPT_B64:
        payload["audio_prompt_b64"] = _VOICE_PROMPT_B64
    body = json.dumps(payload).encode()
    req = urllib.request.Request(
        f"{TTS_URL}/synthesize",
        data=body,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=30.0) as r:
            wav = r.read()
            print(f"  tts: {len(wav)} bytes in {int((time.time()-t0)*1000)}ms")
            return wav
    except Exception as exc:
        print(f"  tts error: {exc}")
        return None


# ── Embedded HTTP server (Sonos pulls audio from us) ─────────────────────────
class _AudioStash:
    """Holds the current WAV blob the HTTP server serves."""
    def __init__(self) -> None:
        self.wav: bytes = b""
        self.path = "/turn.wav"  # rotated each turn for cache-bust


class _AudioHandler(BaseHTTPRequestHandler):
    stash: _AudioStash = None  # type: ignore[assignment]

    def do_GET(self):  # noqa: N802
        if self.stash is None or not self.stash.wav:
            self.send_response(404)
            self.end_headers()
            return
        self.send_response(200)
        self.send_header("Content-Type", "audio/wav")
        self.send_header("Content-Length", str(len(self.stash.wav)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(self.stash.wav)

    def log_message(self, *args, **kwargs):  # silence default access log
        pass


class _ReusableHTTPServer(HTTPServer):
    # Allow restart without the kernel's TIME_WAIT keeping the port held.
    allow_reuse_address = True


def _start_http_server(stash: _AudioStash, port: int) -> None:
    _AudioHandler.stash = stash
    srv = _ReusableHTTPServer(("0.0.0.0", port), _AudioHandler)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    print(f"audio http server: 0.0.0.0:{port}")


def _resolve_host_ip_for(remote_ip: str) -> str:
    """Find the local IP that the kernel routes to `remote_ip`. Avoids
    hard-coding 10.0.0.X — works whether Sonos is on LAN or Tailscale."""
    if EDGE_HOST_IP:
        return EDGE_HOST_IP
    if not remote_ip:
        return socket.gethostbyname(socket.gethostname())
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        s.connect((remote_ip, 1))
        return s.getsockname()[0]
    finally:
        s.close()


# ── Sonos ────────────────────────────────────────────────────────────────────
def _sonos():
    if not SONOS_IP:
        return None
    import soco
    return soco.SoCo(SONOS_IP)


# ── Desktop notification (toast) ─────────────────────────────────────────────
# Pops a notification on the active user's desktop via the standard
# notify-send / DBus path. Works from SSH if the user has an active
# graphical session and /run/user/<uid>/bus is reachable.
_DBUS_ADDR = os.environ.get(
    "DBUS_SESSION_BUS_ADDRESS",
    f"unix:path=/run/user/{os.getuid()}/bus",
)


def notify(title: str, body: str = "", urgency: str = "low",
           expire_ms: int = 4000) -> None:
    """Best-effort desktop toast. Silent on failure (e.g. no GUI session)."""
    import subprocess
    try:
        subprocess.Popen(
            ["notify-send",
             f"--urgency={urgency}",
             f"--expire-time={expire_ms}",
             "--icon=audio-input-microphone",
             title, body],
            env={**os.environ, "DBUS_SESSION_BUS_ADDRESS": _DBUS_ADDR},
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        )
    except Exception:
        pass


def _play_on_sonos(sonos, host_ip: str, http_port: int, turn: int) -> None:
    """Tell Sonos to fetch the current /turn-{N}.wav and play it, wait for done.

    ``http_port`` is the port Sonos hits in the URL — this is
    EDGE_ADVERTISED_PORT (the NodePort) when running in k8s, not the
    container's listen port."""
    url = f"http://{host_ip}:{http_port}/turn-{turn}.wav"
    t0 = time.time()
    try:
        # Force-set every turn — Sonos's reported `.volume` can be stale
        # when the speaker is in a group / when another app is in control,
        # so skipping on "near-match" was leaving us at the previous level.
        # Also un-join from any group so the volume targets this device
        # directly (group volume can override single-device set).
        try:
            sonos.unjoin()
        except Exception:
            pass
        sonos.volume = SONOS_VOLUME
        print(f"  sonos vol set → {SONOS_VOLUME}")
    except Exception as exc:
        print(f"  sonos vol set failed: {exc}")
    sonos.play_uri(url, title="JARVIS")
    # Wait for transport to leave PLAYING state.
    while True:
        time.sleep(0.4)
        info = sonos.get_current_transport_info()
        state = info.get("current_transport_state", "")
        if state in ("STOPPED", "PAUSED_PLAYBACK"):
            break
        if time.time() - t0 > 60:
            break
    print(f"  sonos: played in {int((time.time()-t0)*1000)}ms")


# ── Main loop ────────────────────────────────────────────────────────────────
def main() -> None:
    mic_idx, native_rate, native_chunk = _pick_mic()
    print(f"stt: {STT_URL}")
    print(f"tts: {TTS_URL}")
    print(f"sonos: {SONOS_IP or 'NOT SET — set SONOS_IP env var'}")

    print("loading openWakeWord...")
    oww = OWWModel(wakeword_models=["hey_jarvis"], inference_framework="onnx")

    if BRAIN_MODE == "claude":
        _write_mcp_config()
        print(f"brain: claude (mcp config at {_MCP_CONFIG_PATH})")

    stash = _AudioStash()
    _start_http_server(stash, EDGE_HTTP_PORT)
    host_ip = _resolve_host_ip_for(SONOS_IP or "8.8.8.8")
    print(f"edge host ip (Sonos fetches from here): {host_ip}")

    sonos = None
    if SONOS_IP:
        try:
            sonos = _sonos()
            info = sonos.speaker_info
            print(f"sonos: {info.get('model_name')} '{sonos.player_name}' OK")
        except Exception as exc:
            print(f"sonos init failed ({exc}); will print responses instead")

    try:
        import torch  # noqa: F401
        from silero_vad import load_silero_vad, VADIterator
        vad_model = load_silero_vad(onnx=True)
        has_vad = True
        print("endpointing: Silero VAD")
    except Exception as exc:
        print(f"VAD unavailable ({exc}); using RMS fallback")
        vad_model = None
        has_vad = False

    turn_n = 0
    # Addressee follow-up window: once JARVIS replies, the next utterance
    # within this many seconds doesn't need to re-say "jarvis".
    ADDRESSEE_WINDOW = ADDRESSEE_WINDOW_S
    engaged_until = 0.0
    # Set True for one turn after wake fires. The wake word itself
    # counts as the addressee signal for THIS turn regardless of how
    # long capture took (avoids losing turns where kitchen voices
    # padded the capture out past the engagement window).
    just_woke = False
    with sd.InputStream(samplerate=native_rate, channels=1, dtype="float32",
                        blocksize=native_chunk, device=mic_idx) as stream:
        print("\nready — say 'Hey JARVIS' to trigger. Ctrl-C to quit.\n")
        try:
            while True:
                # ── Wake ──────────────────────────────────────────────
                while True:
                    data, _ = stream.read(native_chunk)
                    mono = data.flatten().astype(np.float32)
                    resampled = _to_16k(mono, native_rate)
                    if len(resampled) < OWW_CHUNK:
                        resampled = np.pad(resampled, (0, OWW_CHUNK - len(resampled)))
                    else:
                        resampled = resampled[:OWW_CHUNK]
                    pcm16 = (resampled * 32767).astype(np.int16)
                    score = oww.predict(pcm16).get("hey_jarvis", 0.0)
                    if score > WAKE_THRESHOLD:
                        oww.reset()
                        # The wake word IS the addressee signal — the next
                        # utterance is implicitly directed at JARVIS even
                        # if the user doesn't say "jarvis" inside it. Set
                        # both the window AND a one-shot flag so a long
                        # capture (e.g. kitchen voices padding it out) can't
                        # cause the window to expire before the check.
                        engaged_until = time.time() + ADDRESSEE_WINDOW
                        just_woke = True
                        print(f"\n[{time.strftime('%H:%M:%S')}] WAKE  score={score:.2f}")
                        notify("JARVIS", f"Listening… (wake {score:.2f})",
                               urgency="normal", expire_ms=2500)
                        break

                # ── Capture ──────────────────────────────────────────
                if has_vad:
                    vad_iter = VADIterator(
                        vad_model, sampling_rate=16000,
                        min_silence_duration_ms=int(SILENCE_SECS * 1000),
                        speech_pad_ms=120,
                    )
                frames: list[np.ndarray] = []
                t_start = time.time()
                heard = False
                silence_start: float | None = None
                vad_buf = np.empty(0, dtype=np.float32)
                while True:
                    data, _ = stream.read(native_chunk)
                    mono = data.flatten().astype(np.float32)
                    frames.append(mono)
                    if has_vad:
                        # Buffer between iterations so we don't drop the
                        # trailing < 512-sample remainder of each chunk
                        # (Silero VADIterator requires exactly 512 samples).
                        vad_buf = np.concatenate([vad_buf, _to_16k(mono, native_rate)])
                        while len(vad_buf) >= 512:
                            frame, vad_buf = vad_buf[:512], vad_buf[512:]
                            ev = vad_iter(frame, return_seconds=False)
                            if ev and "start" in ev:
                                if not heard:
                                    print(f"    vad: speech_start  ({int((time.time()-t_start)*1000)}ms after wake)")
                                heard = True
                                silence_start = None
                            elif ev and "end" in ev and heard:
                                silence_start = time.time()
                                print(f"    vad: speech_end → ending turn")
                        if silence_start and (time.time() - silence_start) >= 0.05:
                            break
                    else:
                        rms = float(np.sqrt(np.mean(mono ** 2)))
                        if rms > 0.01:
                            heard = True
                            silence_start = None
                        elif heard:
                            silence_start = silence_start or time.time()
                            if time.time() - silence_start > SILENCE_SECS:
                                break
                    if (time.time() - t_start) > MAX_UTTERANCE_SECS:
                        break

                dur = sum(len(f) for f in frames) / native_rate
                if dur < MIN_UTTERANCE_SECS or not heard:
                    print(f"  → ignored (dur={dur:.2f}s, heard={heard})")
                    continue

                audio_native = np.concatenate(frames)
                audio_16k = _to_16k(audio_native, native_rate)
                print(f"  → {dur:.2f}s captured; transcribing...")
                res = transcribe(audio_16k)
                if res.get("error"):
                    print(f"  ✗ STT error: {res['error']}")
                    continue
                if res.get("hallucination"):
                    print(f"  → hallucination: '{res.get('raw_text','')}' dropped")
                    continue
                user_text = res.get("text", "").strip()
                if not user_text:
                    print("  → empty transcript")
                    continue
                print(f"  YOU: {user_text!r}  ({res.get('wire_ms','?')}ms wire / {res.get('model_ms','?')}ms model)")

                # ── Addressee gate ──────────────────────────────────
                # Wake fired so we're inside an utterance; the wake itself
                # is the addressee signal for THIS turn (just_woke). For
                # follow-up turns within the engagement window or any turn
                # that explicitly says "jarvis", also pass.
                addressed = (just_woke
                             or "jarvis" in user_text.lower()
                             or time.time() < engaged_until)
                just_woke = False  # consume the one-shot
                if not addressed:
                    print(f"  → not addressed (no 'jarvis' in transcript) — dropped")
                    continue

                # ── Brain ───────────────────────────────────────────
                reply = brain_respond(user_text)
                print(f"  JARVIS: {reply!r}")

                # ── TTS ─────────────────────────────────────────────
                wav = tts_synthesize(reply)
                if not wav:
                    continue

                # ── Stash + tell Sonos to fetch ─────────────────────
                turn_n += 1
                stash.wav = wav
                # Re-route handler on the rotating path so Sonos cache-busts.
                _AudioHandler.stash.path = f"/turn-{turn_n}.wav"
                if sonos is not None:
                    try:
                        _play_on_sonos(sonos, host_ip,
                                       EDGE_ADVERTISED_PORT, turn_n)
                        engaged_until = time.time() + ADDRESSEE_WINDOW
                    except Exception as exc:
                        print(f"  sonos error: {exc}")
                else:
                    # Sonos not configured — write to disk so the user can audit
                    out = f"/tmp/jarvis_edge_turn_{turn_n}.wav"
                    with open(out, "wb") as f:
                        f.write(wav)
                    print(f"  → saved to {out}")
        except KeyboardInterrupt:
            print("\nbye")


if __name__ == "__main__":
    main()
