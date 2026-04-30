# DiffDock blind docking engine.
# Deep learning diffusion model for blind protein-ligand docking.
# Takes protein PDB + ligand SMILES (no binding site box required).
#
# Runtime: GPU (CUDA 11.8 compatible). Uses NVIDIA drivers from host
# via /run/opengl-driver mount (NixOS node) — same pattern as gnina.
#
# Model weights (v1.1) downloaded from GitHub releases (not LFS).
# ESM2-650M embeddings computed at inference time and cached per-job.

FROM nvidia/cuda:11.8.0-cudnn8-runtime-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONUNBUFFERED=1

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip python3-dev \
    git wget unzip ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# PyTorch 1.13.1 with CUDA 11.7 (backward-compatible with 11.8 runtime).
# DiffDock was developed and tested against this version.
RUN pip3 install --no-cache-dir \
    torch==1.13.1+cu117 \
    --extra-index-url https://download.pytorch.org/whl/cu117

# PyG scatter/sparse/cluster wheels for torch-1.13.1+cu117
RUN pip3 install --no-cache-dir \
    torch_scatter==2.1.0 \
    torch_sparse==0.6.16 \
    torch_cluster==1.6.0 \
    torch_spline_conv==1.2.1 \
    -f https://data.pyg.org/whl/torch-1.13.1+cu117.html \
    && pip3 install --no-cache-dir torch_geometric==2.2.0

# DiffDock Python deps (pinned to versions from the paper's environment)
RUN pip3 install --no-cache-dir \
    biopython==1.79 \
    rdkit \
    e3nn==0.5.0 \
    spyrmsd \
    pandas \
    tqdm \
    "fair-esm>=2.0.0" \
    mysql-connector-python \
    boto3

# Clone DiffDock source (no LFS needed; model weights are in GitHub Releases).
RUN git clone --depth=1 https://github.com/gcorso/DiffDock /opt/diffdock

# Download and extract model weights from GitHub release v1.1.
# The zip extracts to workdir/v1.1/{score_model,confidence_model}/ inside /opt/diffdock/.
RUN wget -q -O /tmp/diffdock_models.zip \
    https://github.com/gcorso/DiffDock/releases/download/v1.1/diffdock_models.zip \
    && unzip -q /tmp/diffdock_models.zip -d /opt/diffdock \
    && rm /tmp/diffdock_models.zip

# Verify model weights were extracted successfully.
RUN python3 -c "import os,sys; files=['/opt/diffdock/workdir/v1.1/score_model/best_ema_inference_epoch_model.pt','/opt/diffdock/workdir/v1.1/confidence_model/best_model_epoch75.pt']; sizes=[(f,os.path.getsize(f) if os.path.exists(f) else 0) for f in files]; [print(f,s) for f,s in sizes]; sys.exit(1) if any(s<1000000 for _,s in sizes) else print('Model weights OK')"

# Pre-download ESM2-650M weights so inference doesn't hit the network at runtime.
RUN python3 -c "import esm; esm.pretrained.esm2_t33_650M_UR50D()" \
    || echo "WARNING: ESM pre-download failed; will retry at inference time"

COPY scripts/dock_batch.py /scripts/dock_batch.py

CMD ["python3", "/scripts/dock_batch.py"]
