FROM pytorch/pytorch:2.3.0-cuda12.1-cudnn8-runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    libsndfile1 ffmpeg git \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir \
    chatterbox-tts \
    soundfile \
    fastapi \
    "uvicorn[standard]"

WORKDIR /app
COPY server.py .

ENV PORT=8765
EXPOSE 8765

CMD ["python", "server.py"]
