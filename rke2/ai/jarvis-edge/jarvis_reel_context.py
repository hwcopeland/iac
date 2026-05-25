"""jarvis_reel_context — describe an Instagram reel in 2-4 sentences.

Public API:

    import jarvis_reel_context
    desc: str = jarvis_reel_context.analyze(media_pk)

Pipeline (best-effort, never raises):

  1. Borrow the poller's logged-in instagrapi Client via
     jarvis_ig_polling.get_client(). If the poller isn't up yet, return a
     stub from `client.media_info(media_pk)` so callers always get SOMETHING.
  2. `client.video_download(media_pk, folder=/tmp)` → MP4 on disk.
  3. ffmpeg → 6 evenly-spaced keyframes, longest-edge 768px JPEGs.
  4. ffmpeg → 16kHz mono PCM WAV (decoded to float32 numpy for the
     existing /v1/transcribe endpoint — it takes raw float32 bytes, not
     a multipart upload).
  5. POST the float32 audio bytes to ${STT_URL}/v1/transcribe?sr=16000.
     If hallucination flag is set or transcript is empty, blank it out.
  6. Call edge._claude_brain_vision(prompt, image_paths) to get a 2-4
     sentence description grounded in the keyframes + transcript.
  7. Cache description + raw transcript to
     /state/ig_reel_descriptions/<pk>.json so repeated tags on the same
     reel don't re-process the whole pipeline.
  8. Cleanup the MP4 + intermediate files on success (keep the JSON
     cache, that's the point of caching).

Any stage failure → graceful fallback string. This module MUST NOT raise
into the comment-reply consumer; the comment will still go out with the
fallback context, just less specific.

Env (read at call time, not import):

    STT_URL                 cluster whisper endpoint (default http://localhost:8766)
    REEL_CACHE_DIR          override /state/ig_reel_descriptions
    REEL_KEYFRAME_COUNT     override 6 keyframes
    REEL_KEYFRAME_MAX_EDGE  override 768px longest edge
    REEL_AUDIO_TIMEOUT_S    STT request timeout (default 30)
    REEL_FFMPEG_TIMEOUT_S   per-ffmpeg-call timeout (default 60)
"""
from __future__ import annotations

import base64
import json
import os
import subprocess
import time
import traceback
import urllib.request
from typing import Any


# ── Cache layout ────────────────────────────────────────────────────────────
def _cache_dir() -> str:
    return os.environ.get("REEL_CACHE_DIR", "/state/ig_reel_descriptions")


def _cache_path(media_pk: str) -> str:
    return os.path.join(_cache_dir(), f"{media_pk}.json")


def _load_cached(media_pk: str) -> dict | None:
    """Return the cached description dict, or None on miss / corrupt cache.
    Corrupt cache is treated as miss — we'll re-run the pipeline."""
    path = _cache_path(media_pk)
    try:
        with open(path) as f:
            return json.load(f)
    except (OSError, ValueError, json.JSONDecodeError):
        return None


def _save_cached(media_pk: str, description: str, transcript: str) -> None:
    """Persist description + raw transcript atomically. Best-effort —
    if disk is full or path isn't writable, we still return the
    description to the caller (the cache write isn't load-bearing for
    correctness, only for not re-processing the same reel)."""
    path = _cache_path(media_pk)
    tmp = path + ".tmp"
    try:
        os.makedirs(os.path.dirname(path), exist_ok=True)
    except OSError:
        pass
    try:
        with open(tmp, "w") as f:
            json.dump({
                "media_pk": str(media_pk),
                "description": description,
                "transcript": transcript,
                "cached_at": time.time(),
            }, f)
        os.replace(tmp, path)
    except OSError as exc:
        print(f"reel context: cache write failed for {media_pk}: {exc!r}")


# ── instagrapi client (lazy import) ─────────────────────────────────────────
def _get_client() -> Any | None:
    """Borrow the poller's live Client. Two parallel logins from the same
    IP trips a challenge; we MUST share."""
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


# ── ffmpeg subprocess helpers ───────────────────────────────────────────────
def _ffmpeg_path() -> str:
    return os.environ.get("FFMPEG_PATH", "ffmpeg")


def _ffmpeg_timeout() -> float:
    try:
        return float(os.environ.get("REEL_FFMPEG_TIMEOUT_S", "60"))
    except ValueError:
        return 60.0


def _probe_duration(video_path: str) -> float:
    """Use ffmpeg to read the duration. Returns 0.0 on any failure
    (caller falls back to evenly-distributed timestamps starting at 0)."""
    try:
        proc = subprocess.run(
            [_ffmpeg_path(), "-i", video_path],
            capture_output=True, text=True, timeout=_ffmpeg_timeout(),
        )
        # ffmpeg writes "Duration: HH:MM:SS.ms" to stderr.
        out = proc.stderr or ""
        for line in out.splitlines():
            line = line.strip()
            if line.startswith("Duration:"):
                # "Duration: 00:00:14.21, start: ..., bitrate: ..."
                ts = line.split(",", 1)[0].split("Duration:", 1)[1].strip()
                h, m, s = ts.split(":")
                return int(h) * 3600 + int(m) * 60 + float(s)
    except Exception as exc:  # noqa: BLE001
        print(f"reel context: probe duration failed: {exc!r}")
    return 0.0


def _extract_keyframes(video_path: str, out_dir: str, media_pk: str,
                       count: int, max_edge: int) -> list[str]:
    """Extract `count` evenly spaced JPEG frames. Returns the list of
    paths that successfully wrote. Best-effort — partial success returns
    whatever we got."""
    duration = _probe_duration(video_path)
    # Even spacing avoiding the very last frame (often a black tail);
    # if duration probe fails, fall back to 1s steps.
    if duration <= 0.1:
        timestamps = [float(i) for i in range(count)]
    else:
        step = duration / (count + 1)
        timestamps = [step * (i + 1) for i in range(count)]

    out_paths: list[str] = []
    for i, ts in enumerate(timestamps):
        out = os.path.join(out_dir, f"{media_pk}_kf_{i}.jpg")
        # -ss BEFORE -i for fast seek; scale to longest-edge max_edge.
        # `iw` / `ih` is the actual frame size; the scale expression picks
        # the larger axis to set max_edge and lets the other one auto.
        scale_expr = f"scale='if(gt(iw,ih),{max_edge},-2)':'if(gt(iw,ih),-2,{max_edge})'"
        try:
            subprocess.run(
                [_ffmpeg_path(),
                 "-ss", f"{ts:.3f}",
                 "-i", video_path,
                 "-frames:v", "1",
                 "-vf", scale_expr,
                 "-q:v", "3",  # JPEG quality (2-5 = high)
                 "-y", out],
                capture_output=True, text=True, timeout=_ffmpeg_timeout(),
                check=True,
            )
            if os.path.exists(out) and os.path.getsize(out) > 0:
                out_paths.append(out)
        except Exception as exc:  # noqa: BLE001
            print(f"reel context: keyframe {i} at t={ts:.2f}s failed: {exc!r}")
    return out_paths


def _extract_audio(video_path: str, out_path: str) -> bool:
    """Extract a 16kHz mono PCM WAV. Returns True on success."""
    try:
        subprocess.run(
            [_ffmpeg_path(),
             "-i", video_path,
             "-vn", "-ac", "1", "-ar", "16000",
             "-f", "wav",
             "-y", out_path],
            capture_output=True, text=True, timeout=_ffmpeg_timeout(),
            check=True,
        )
        return os.path.exists(out_path) and os.path.getsize(out_path) > 0
    except Exception as exc:  # noqa: BLE001
        print(f"reel context: audio extract failed: {exc!r}")
        return False


# ── STT (reuse cluster /v1/transcribe) ──────────────────────────────────────
def _transcribe_wav(wav_path: str) -> str:
    """Decode the 16kHz mono PCM WAV → float32 numpy → POST raw bytes to
    /v1/transcribe?sr=16000 (the same endpoint edge.transcribe() uses).
    Returns the transcript text, or "" on any failure / hallucination /
    silent audio."""
    stt_url = os.environ.get("STT_URL", "http://localhost:8766")
    try:
        import wave
        import numpy as np
    except Exception as exc:  # noqa: BLE001
        print(f"reel context: numpy/wave import failed: {exc!r}")
        return ""

    try:
        with wave.open(wav_path, "rb") as w:
            sr = w.getframerate()
            n_channels = w.getnchannels()
            sampwidth = w.getsampwidth()
            n_frames = w.getnframes()
            raw = w.readframes(n_frames)
    except Exception as exc:  # noqa: BLE001
        print(f"reel context: wav read failed: {exc!r}")
        return ""

    if sr != 16000 or n_channels != 1:
        # ffmpeg should have given us exactly 16k mono — anything else
        # means the extract was wrong; bail rather than ship garbage.
        print(f"reel context: unexpected wav fmt sr={sr} ch={n_channels}")
        return ""

    try:
        if sampwidth == 2:
            arr = np.frombuffer(raw, dtype=np.int16).astype(np.float32) / 32768.0
        elif sampwidth == 4:
            arr = np.frombuffer(raw, dtype=np.int32).astype(np.float32) / 2147483648.0
        else:
            print(f"reel context: unsupported sample width {sampwidth}")
            return ""
    except Exception as exc:  # noqa: BLE001
        print(f"reel context: wav decode failed: {exc!r}")
        return ""

    if arr.size < 16000 * 0.3:
        # < 300ms of audio — treat as silent.
        return ""

    # Cheap RMS-based silence gate; whisper hallucinates on pure silence.
    try:
        import numpy as _np
        rms = float(_np.sqrt(_np.mean(arr.astype(_np.float32) ** 2)))
    except Exception:  # noqa: BLE001
        rms = 0.0
    if rms < 0.005:
        return ""

    try:
        timeout = float(os.environ.get("REEL_AUDIO_TIMEOUT_S", "30"))
    except ValueError:
        timeout = 30.0

    body = arr.astype(np.float32).tobytes()
    req = urllib.request.Request(
        f"{stt_url}/v1/transcribe?sr=16000",
        data=body,
        headers={"Content-Type": "application/octet-stream"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            data = json.loads(r.read())
    except Exception as exc:  # noqa: BLE001
        print(f"reel context: STT request failed: {exc!r}")
        return ""

    if data.get("hallucination"):
        return ""
    text = (data.get("text") or "").strip()
    # Whisper sometimes emits canned phrases on near-silence; drop those.
    _CANNED = {
        "thank you.", "thanks for watching.", "thank you for watching.",
        "you", ".", "thanks.", "please subscribe.",
    }
    if text.lower() in _CANNED:
        return ""
    return text


# ── Fallback stub ───────────────────────────────────────────────────────────
def _fallback_from_media(media_pk: str, client: Any | None) -> str:
    """When the pipeline can't run, return a one-liner that still gives
    the brain enough context to write a comment ("reel by @user, caption:
    ..."). Best-effort — if even media_info fails, return a bare stub."""
    if client is None:
        return f"<reel {media_pk}>"
    try:
        info = client.media_info(media_pk)
        username = getattr(getattr(info, "user", None), "username", "") or ""
        caption = (getattr(info, "caption_text", "") or "").strip()
        if caption:
            return f"<reel by {username}, caption: {caption[:120]}>"
        return f"<reel by {username}>"
    except Exception as exc:  # noqa: BLE001
        print(f"reel context: media_info fallback failed for {media_pk}: {exc!r}")
        return f"<reel {media_pk}>"


# ── Public entry point ─────────────────────────────────────────────────────
def analyze(media_pk: str) -> str:
    """Best-effort 2-4 sentence description of an IG reel. NEVER raises.

    Cache-hits return synchronously without re-running ffmpeg / STT /
    Claude. Cache writes happen AFTER a successful pipeline run."""
    media_pk = str(media_pk)

    # 1. Cache lookup. Cached descriptions never expire — IG reels are
    #    immutable content; if we've described it once that description
    #    remains valid for the lifetime of the post.
    cached = _load_cached(media_pk)
    if cached and cached.get("description"):
        return cached["description"]

    client = _get_client()
    if client is None:
        print(f"reel context: no instagrapi client for {media_pk}; returning bare stub")
        return _fallback_from_media(media_pk, None)

    # 2. media_info — also gives us caption + username for the prompt.
    try:
        info = client.media_info(media_pk)
        username = getattr(getattr(info, "user", None), "username", "") or ""
        caption = (getattr(info, "caption_text", "") or "").strip()
    except Exception as exc:  # noqa: BLE001
        print(f"reel context: media_info failed for {media_pk}: {exc!r}")
        traceback.print_exc()
        return _fallback_from_media(media_pk, client)

    # 3. video_download — instagrapi accepts a str folder, returns Path.
    video_path: str | None = None
    try:
        path = client.video_download(media_pk, folder="/tmp")
        video_path = str(path) if path else None
    except Exception as exc:  # noqa: BLE001
        print(f"reel context: video_download failed for {media_pk}: {exc!r}")
        traceback.print_exc()
        return _fallback_from_media(media_pk, client)
    if not video_path or not os.path.exists(video_path):
        print(f"reel context: video_download returned no file for {media_pk}")
        return _fallback_from_media(media_pk, client)

    # 4. ffmpeg: keyframes + audio. Either failing is non-fatal — the
    #    vision call can still run with no images (in which case it
    #    degrades to a text-only summary built from the caption).
    try:
        kf_count = int(os.environ.get("REEL_KEYFRAME_COUNT", "6"))
    except ValueError:
        kf_count = 6
    try:
        max_edge = int(os.environ.get("REEL_KEYFRAME_MAX_EDGE", "768"))
    except ValueError:
        max_edge = 768

    keyframes: list[str] = []
    audio_path = f"/tmp/{media_pk}.wav"
    try:
        keyframes = _extract_keyframes(video_path, "/tmp", media_pk, kf_count, max_edge)
    except Exception as exc:  # noqa: BLE001
        print(f"reel context: keyframe extract failed for {media_pk}: {exc!r}")
        traceback.print_exc()

    audio_ok = False
    try:
        audio_ok = _extract_audio(video_path, audio_path)
    except Exception as exc:  # noqa: BLE001
        print(f"reel context: audio extract crashed for {media_pk}: {exc!r}")
        traceback.print_exc()

    # 5. Transcribe (only if we got audio).
    transcript = ""
    if audio_ok:
        try:
            transcript = _transcribe_wav(audio_path)
        except Exception as exc:  # noqa: BLE001
            print(f"reel context: STT crashed for {media_pk}: {exc!r}")
            traceback.print_exc()

    # 6. Vision call. If we have zero keyframes AND no transcript, skip
    #    the brain call and ship the caption-based fallback — paying for
    #    a Claude turn with no signal is wasteful.
    if not keyframes and not transcript:
        desc = _fallback_from_media(media_pk, client)
        _cleanup(video_path, audio_path, keyframes)
        return desc

    prompt_lines = [
        "Describe what's happening in this Instagram reel in 2-4 sentences.",
        "Include: who's in frame, what they're doing, mood, any visible text overlays, and what the audio says.",
        "Be specific. No generic 'the video shows...' framing. Describe directly.",
        "",
        f"Posted by: @{username}",
        f"Caption: \"{caption[:240]}\"",
    ]
    if transcript:
        prompt_lines += ["", f"Audio transcript: \"{transcript[:600]}\""]
    else:
        prompt_lines += ["", "Audio: (silent or no speech)"]
    prompt_lines += ["", "Description:"]
    prompt = "\n".join(prompt_lines)

    description = ""
    try:
        # Lazy import — edge.py is fully initialised by the time analyze()
        # is called from a consumer thread.
        import edge as _edge  # type: ignore[import]
        description = _edge._claude_brain_vision(prompt, keyframes)
    except Exception as exc:  # noqa: BLE001
        print(f"reel context: vision call crashed for {media_pk}: {exc!r}")
        traceback.print_exc()
        description = ""

    if not description.strip():
        description = _fallback_from_media(media_pk, client)

    # 7. Cache + 8. cleanup.
    _save_cached(media_pk, description.strip(), transcript)
    _cleanup(video_path, audio_path, keyframes)
    return description.strip()


def _cleanup(video_path: str | None, audio_path: str, keyframes: list[str]) -> None:
    """Remove the intermediate MP4 / WAV / JPEG files. Cache JSON stays.
    Best-effort — disk reclaim is the goal, not correctness."""
    paths = []
    if video_path:
        paths.append(video_path)
    paths.append(audio_path)
    paths.extend(keyframes)
    for p in paths:
        try:
            if p and os.path.exists(p):
                os.unlink(p)
        except OSError as exc:
            print(f"reel context: cleanup failed for {p}: {exc!r}")
