# Gnina CNN-scored docking engine sidecar
# GPU-accelerated docking with CNN affinity and pose scoring.
# Requires CUDA runtime; actual GPU libs mounted from NixOS host at runtime.
#
# API: POST /dock, GET /health, GET /readyz

FROM nvidia/cuda:12.2.0-runtime-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Etc/UTC

RUN apt-get update && apt-get install -y --no-install-recommends \
    wget \
    ca-certificates \
    python3 \
    python3-pip \
    libopenbabel-dev \
    libboost-all-dev \
    && rm -rf /var/lib/apt/lists/*

# Install gnina from GitHub releases (static build)
RUN wget -q -O /usr/local/bin/gnina \
    "https://github.com/gnina/gnina/releases/latest/download/gnina" \
    && chmod +x /usr/local/bin/gnina

WORKDIR /app

COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

COPY app.py ./

EXPOSE 8000

# Single worker for GPU-bound workloads to avoid VRAM contention
CMD ["gunicorn", "-w", "1", "-b", "0.0.0.0:8000", "--timeout", "600", "app:app"]
