# AutoDock-GPU v1.6 docking engine sidecar
# Pre-built CUDA 12 binary from ccsb-scripps/AutoDock-GPU
# GPU-accelerated molecular docking with AutoDock4 scoring function
#
# API: POST /dock, GET /health, GET /readyz

FROM nvidia/cuda:12.2.0-runtime-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates wget python3 python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Download pre-built AutoDock-GPU v1.6 (CUDA 12, 128 work items)
RUN wget -q -O /usr/local/bin/autodock_gpu \
    "https://github.com/ccsb-scripps/AutoDock-GPU/releases/download/v1.6/adgpu-v1.6_linux_x64_cuda12_128wi" \
    && chmod +x /usr/local/bin/autodock_gpu

WORKDIR /app

COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

COPY app.py ./

EXPOSE 8000

CMD ["gunicorn", "-w", "1", "-b", "0.0.0.0:8000", "--timeout", "600", "app:app"]
