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

# Model loading can take 30-60s; gunicorn preload + generous timeout.
# CPU inference on 500-compound batches takes 10-30 min; 3600s covers worst case.
CMD ["gunicorn", "-w", "2", "-b", "0.0.0.0:8000", "--timeout", "3600", "--preload", "app:app"]
