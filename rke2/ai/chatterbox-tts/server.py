"""Chatterbox TTS HTTP server — runs on nixos-gpu, CUDA-accelerated."""

import io
import os

import soundfile as sf
import torch
from fastapi import FastAPI, HTTPException
from fastapi.responses import Response
from pydantic import BaseModel

app = FastAPI()
_model = None
_sample_rate = None


@app.on_event("startup")
async def load_model():
    global _model, _sample_rate
    from chatterbox.tts_turbo import ChatterboxTurboTTS
    _model = ChatterboxTurboTTS.from_pretrained(device="cuda")
    _sample_rate = _model.sr


class SynthRequest(BaseModel):
    text: str
    exaggeration: float = 0.35
    cfg_weight: float = 0.5
    audio_prompt_b64: str = ""  # base64-encoded WAV for voice cloning


@app.post("/synthesize")
async def synthesize(req: SynthRequest):
    if _model is None:
        raise HTTPException(503, "Model not loaded")

    ref_path = None
    if req.audio_prompt_b64:
        import base64, tempfile
        audio_bytes = base64.b64decode(req.audio_prompt_b64)
        tmp = tempfile.NamedTemporaryFile(suffix=".wav", delete=False)
        tmp.write(audio_bytes)
        tmp.close()
        ref_path = tmp.name

    try:
        wav = _model.generate(
            req.text,
            audio_prompt_path=ref_path,
            exaggeration=req.exaggeration,
            cfg_weight=req.cfg_weight,
        )
    finally:
        if ref_path:
            os.unlink(ref_path)

    audio_np = wav.squeeze().cpu().numpy()
    buf = io.BytesIO()
    sf.write(buf, audio_np, _sample_rate, format="WAV")
    buf.seek(0)

    return Response(
        content=buf.read(),
        media_type="audio/wav",
        headers={"X-Sample-Rate": str(_sample_rate)},
    )


@app.get("/health")
def health():
    return {"status": "ok", "model_loaded": _model is not None}


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=int(os.getenv("PORT", "8765")))
