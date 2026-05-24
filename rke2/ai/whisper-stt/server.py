"""whisper-stt — fast cluster-side STT service.

POST /v1/transcribe       raw float32 mono LE PCM, body, ?sr=<N> query param.
                          Returns JSON {text, language, duration_s, model_ms}.
GET  /health              {status, model_loaded}.

Backend: faster-whisper (CTranslate2) running whisper-large-v3-turbo on CUDA
(RTX 3070). FP16 weights, ~3 GB VRAM. ~50 ms for a 5 s utterance.

Hallucination filter discards Whisper's known no-speech patterns
(ellipses, "thanks for watching", "subscribe", etc.) and very short
outputs so noisy ambient audio never produces a phantom turn.

Audio is internally resampled to 16 kHz via scipy.resample_poly so any
mic native rate works (16k / 24k / 32k / 44.1k / 48k).

Runs on the nixos-gpu node — same NixOS driver mounts as chatterbox-tts
(/run/opengl-driver + /nix/store).
"""
from __future__ import annotations

import os
import re
import time
from typing import Optional

import numpy as np
from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import JSONResponse
import uvicorn

# ── Whisper model ────────────────────────────────────────────────────────────
# Use the community CTranslate2 conversion of whisper-large-v3-turbo.
# Cached to /models (PVC at runtime).
_MODEL_REPO = os.environ.get("WHISPER_MODEL", "deepdml/faster-whisper-large-v3-turbo-ct2")
_DEVICE = os.environ.get("WHISPER_DEVICE", "cuda")
_COMPUTE = os.environ.get("WHISPER_COMPUTE", "float16")
_PORT = int(os.environ.get("PORT", "8766"))

_model = None
_model_loaded = False

# ── Hallucination filter ─────────────────────────────────────────────────────
# Whisper has a tight set of failure modes when fed silence or noise.
# Drop these before they can become phantom user turns downstream.
_HALLUCINATION_PHRASES = (
    "thanks for watching",
    "thank you for watching",
    "please subscribe",
    "like and subscribe",
    "see you next time",
    "see you in the next",
    "thank you so much",
    "thank you very much",
    "see you guys later",
    "bye bye",
    "the end",
    "you",  # single word
    "thanks",
    "thank you",
    "music",
    "[music]",
    "[applause]",
    "[laughter]",
)
_PUNCT_ONLY_RE = re.compile(r"^[\s\W_]+$")


def is_hallucination(text: str) -> bool:
    t = (text or "").strip().lower()
    if len(t) < 2:
        return True
    if _PUNCT_ONLY_RE.fullmatch(t):
        return True
    # Squash whitespace + trailing punctuation for the phrase match.
    norm = re.sub(r"[^a-z\s]", "", t).strip()
    norm = re.sub(r"\s+", " ", norm)
    return norm in _HALLUCINATION_PHRASES


# ── FastAPI app ──────────────────────────────────────────────────────────────
app = FastAPI(title="whisper-stt", version="0.1.0")


@app.on_event("startup")
def _load_model() -> None:
    global _model, _model_loaded
    print(f"[startup] loading {_MODEL_REPO} on {_DEVICE} ({_COMPUTE})", flush=True)
    t0 = time.time()
    from faster_whisper import WhisperModel
    _model = WhisperModel(
        _MODEL_REPO,
        device=_DEVICE,
        compute_type=_COMPUTE,
        download_root="/models",
    )
    # Warm-up with a 1-second silent clip so the first real request is the
    # steady-state fast path.
    silent = np.zeros(16000, dtype=np.float32)
    list(_model.transcribe(silent, language="en", beam_size=1)[0])
    _model_loaded = True
    print(f"[startup] model loaded in {time.time()-t0:.1f}s", flush=True)


@app.get("/health")
def health():
    return {"status": "ok", "model_loaded": _model_loaded, "model": _MODEL_REPO}


@app.post("/v1/transcribe")
async def transcribe(req: Request):
    if not _model_loaded or _model is None:
        raise HTTPException(503, "model not loaded yet")
    sr = int(req.query_params.get("sr", "16000"))
    language = req.query_params.get("lang") or "en"
    beam_size = int(req.query_params.get("beam", "1"))
    vad = req.query_params.get("vad", "1") not in ("0", "false", "no")

    raw = await req.body()
    if not raw:
        raise HTTPException(400, "empty body")
    # Input is little-endian float32 mono PCM.
    audio = np.frombuffer(raw, dtype=np.float32)
    if audio.size == 0:
        raise HTTPException(400, "no samples")

    # Resample to 16 kHz if needed (CTranslate2 expects 16k).
    if sr != 16000:
        from scipy.signal import resample_poly
        from math import gcd
        g = gcd(sr, 16000)
        audio = resample_poly(audio, 16000 // g, sr // g).astype(np.float32)

    duration_s = float(len(audio) / 16000.0)
    t0 = time.time()
    segments, info = _model.transcribe(
        audio,
        language=language,
        beam_size=beam_size,
        vad_filter=vad,
        condition_on_previous_text=False,  # voice-assistant turns are independent
    )
    text_parts = [s.text for s in segments]
    text = "".join(text_parts).strip()
    model_ms = int((time.time() - t0) * 1000)

    hallucination = is_hallucination(text)
    out = {
        "text": "" if hallucination else text,
        "raw_text": text,
        "hallucination": hallucination,
        "language": info.language,
        "language_probability": round(info.language_probability, 3),
        "duration_s": round(duration_s, 2),
        "model_ms": model_ms,
        "model": _MODEL_REPO,
    }
    return JSONResponse(out)


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=_PORT, log_level="info")
