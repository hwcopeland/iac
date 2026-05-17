FROM python:3.11-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libsndfile1 ffmpeg git \
    && rm -rf /var/lib/apt/lists/*

# Let chatterbox pull its preferred torch version (currently 2.6.0+cu124).
# The torch pip wheel bundles the CUDA runtime — no CUDA base image needed.
RUN pip install --no-cache-dir chatterbox-tts soundfile fastapi "uvicorn[standard]"

# Install torchvision matching the torch version chatterbox pulled.
RUN pip install --no-cache-dir torchvision \
    --index-url https://download.pytorch.org/whl/cu124

WORKDIR /app
COPY server.py .

ENV PORT=8765
EXPOSE 8765

CMD ["python", "server.py"]
