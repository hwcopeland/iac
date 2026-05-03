# ADMET prediction sidecar for the Khemeia SBDD pipeline.
# Serves Stanford Chemprop-D models (admet_ai) for full ADMET endpoint prediction.
# Falls back to rule-based descriptors if admet_ai models fail to load.

FROM python:3.11-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libxrender1 \
    libxext6 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py ./

EXPOSE 8000

# 2 workers, no --preload: each worker initializes its own CUDA context after fork.
# --preload causes SIGSEGV because CUDA contexts are not fork-safe.
# Worker 1 handles predictions; worker 2 keeps health probes alive during long inference.
CMD ["gunicorn", "-w", "2", "-b", "0.0.0.0:8000", "--timeout", "3600", "app:app"]
