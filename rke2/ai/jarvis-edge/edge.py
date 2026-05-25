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
EDGE_HOST_IP = os.environ.get("EDGE_HOST_IP", "")
BRAIN_MODE = os.environ.get("BRAIN_MODE", "echo")

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


# ── Brain (stub) ─────────────────────────────────────────────────────────────
def brain_respond(text: str) -> str:
    """Replace with a real brain call later.

    For now echoes so we can verify the full audio loop. To swap:
    - point at a brain HTTP service (cluster), OR
    - spawn a local `claude -p` here, OR
    - call Anthropic API directly with a small system prompt.
    """
    if BRAIN_MODE == "echo":
        return f"Sir, I heard: {text}"
    # Stub for an API mode — implement when ready.
    return f"Sir, I heard: {text}"


# ── Cluster TTS ──────────────────────────────────────────────────────────────
def tts_synthesize(text: str) -> bytes | None:
    """POST text to chatterbox, return WAV bytes."""
    t0 = time.time()
    body = json.dumps({"text": text}).encode()
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
    """Tell Sonos to fetch the current /turn-{N}.wav and play it, wait for done."""
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
                        _play_on_sonos(sonos, host_ip, EDGE_HTTP_PORT, turn_n)
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
