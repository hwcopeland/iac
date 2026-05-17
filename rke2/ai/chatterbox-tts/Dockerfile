FROM python:3.11-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libsndfile1 ffmpeg git curl \
    && rm -rf /var/lib/apt/lists/*

# Install PyTorch with CUDA 12.1 support first so chatterbox gets the right torch
RUN pip install --no-cache-dir \
    torch==2.3.0 torchvision==0.18.0 torchaudio==2.3.0 \
    --index-url https://download.pytorch.org/whl/cu121

RUN pip install --no-cache-dir \
    "transformers>=4.40.0" \
    chatterbox-tts \
    soundfile \
    fastapi \
    "uvicorn[standard]"

WORKDIR /app
COPY server.py .

ENV PORT=8765
EXPOSE 8765

CMD ["python", "server.py"]
