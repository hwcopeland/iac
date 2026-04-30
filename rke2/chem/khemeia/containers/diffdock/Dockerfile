# DiffDock blind docking engine.
# Deep learning diffusion model for blind protein-ligand docking.
# Takes protein PDB + ligand SMILES (no binding site box required).
#
# Runtime: GPU (CUDA 11.8 compatible). Uses NVIDIA drivers from host
# via /run/opengl-driver mount (NixOS node) — same pattern as gnina.
#
# Model weights (v1.1) downloaded from GitHub releases (not LFS).
# ESM2-650M embeddings downloaded at build time.
#
# torch 2.1+cu118: supports driver 520+ (host is 580). DiffDock v1.1
# inference.py is compatible with torch 2.x via PyG 2.4.
#
# Multi-stage: builder compiles C extensions + downloads weights;
# runtime image strips gcc/make/git/wget/unzip/python3-dev.

# ── Stage 1: builder ─────────────────────────────────────────────────────────
FROM nvidia/cuda:11.8.0-cudnn8-runtime-ubuntu22.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONUNBUFFERED=1

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip python3-dev \
    gcc g++ make \
    git wget unzip ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# numpy<2 required by fair-esm (compiled against NumPy 1.x ABI).
RUN pip3 install --no-cache-dir "numpy<2"

# PyTorch 2.1.2 + CUDA 11.8 — supports driver 520+ (host is 580).
RUN pip3 install --no-cache-dir \
    torch==2.1.2+cu118 \
    --extra-index-url https://download.pytorch.org/whl/cu118

# PyG wheels for torch-2.1.2+cu118
RUN pip3 install --no-cache-dir \
    torch_scatter \
    torch_sparse \
    torch_cluster \
    torch_spline_conv \
    -f https://data.pyg.org/whl/torch-2.1.2+cu118.html \
    && pip3 install --no-cache-dir torch_geometric==2.4.0

# DiffDock Python deps
RUN pip3 install --no-cache-dir \
    biopython==1.79 \
    rdkit \
    e3nn==0.5.0 \
    spyrmsd \
    pandas \
    tqdm \
    pyyaml \
    prody \
    "fair-esm>=2.0.0" \
    mysql-connector-python \
    boto3

# Clone DiffDock source (no LFS needed; model weights are in GitHub Releases).
RUN git clone --depth=1 https://github.com/gcorso/DiffDock /opt/diffdock

# Download and extract model weights from GitHub release v1.1.
RUN mkdir -p /opt/diffdock/workdir/v1.1 \
    && wget -O /tmp/diffdock_models.zip \
       https://github.com/gcorso/DiffDock/releases/download/v1.1/diffdock_models.zip \
    && echo "Downloaded $(wc -c < /tmp/diffdock_models.zip) bytes" \
    && unzip -q /tmp/diffdock_models.zip -d /opt/diffdock/workdir/v1.1 \
    && ls -la /opt/diffdock/workdir/v1.1/ \
    && rm /tmp/diffdock_models.zip

# Pre-download ESM2-650M weights so inference doesn't hit the network at runtime.
RUN python3 -c "import esm; esm.pretrained.esm2_t33_650M_UR50D()"

# ── Stage 2: runtime ──────────────────────────────────────────────────────────
FROM nvidia/cuda:11.8.0-cudnn8-runtime-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONUNBUFFERED=1

# Runtime only — no gcc/make/git/wget/unzip/python3-dev
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy all pip-installed packages from builder
COPY --from=builder /usr/local/lib/python3.10 /usr/local/lib/python3.10

# DiffDock source and model weights
COPY --from=builder /opt/diffdock /opt/diffdock

# ESM2 pre-downloaded weights (~2.6GB, avoids network hit at inference time)
COPY --from=builder /root/.cache /root/.cache

COPY scripts/dock_batch.py /scripts/dock_batch.py

CMD ["python3", "/scripts/dock_batch.py"]
