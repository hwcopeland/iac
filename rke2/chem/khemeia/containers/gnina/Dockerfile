# Gnina CNN-scored docking engine
# GPU-accelerated docking with CNN affinity and pose scoring.
# Requires CUDA runtime; actual GPU libs mounted from NixOS host at runtime.
#
# Primary mode: batch worker (dock_batch.py) invoked as a K8s Job.
# The Flask app (app.py) is retained for health checks only.

FROM nvidia/cuda:12.8.1-cudnn-runtime-ubuntu22.04

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

# gnina 1.3.2 CUDA 12.8 variant — built against cuDNN 9 + CUDA 12.8.
# Required for driver 580 (default binary targets CUDA 12.2 which deadlocks
# with cuDNN 9 on driver 580 due to cuDNN 9 requiring CUDA >= 12.3).
RUN wget -q -O /usr/local/bin/gnina \
    "https://github.com/gnina/gnina/releases/download/v1.3.2/gnina.1.3.2.cuda12.8" \
    && chmod +x /usr/local/bin/gnina

WORKDIR /app

COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

# Batch docking script (primary entrypoint for K8s Jobs).
# Also ships the genomics pgx_docking.py orchestrator (calc=pgx_docking) — it
# reuses this image's gnina CLI as the default engine; no new image (workers TDD §7).
COPY scripts/ /scripts/

# Curated PGx drug-name->SMILES table consumed by pgx_docking.py at
# /scripts/data/pgx_drug_smiles.csv (workers TDD §7.3 / R-W6). Pure data, baked in.
COPY data/ /scripts/data/

# Flask app retained for health checks
COPY app.py ./

CMD ["python3", "/scripts/dock_batch.py"]
