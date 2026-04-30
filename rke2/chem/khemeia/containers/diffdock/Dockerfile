# DiffDock blind docking engine.
# Deep learning diffusion model for blind protein-ligand docking.
# Takes protein PDB + ligand SMILES; no binding site box required.
#
# Runtime: GPU (CUDA 11.8 compatible). Uses NVIDIA drivers from host
# via /run/opengl-driver mount (NixOS node) — same pattern as gnina.
#
# Source + model weights cloned from gcorso/DiffDock on GitHub via Git LFS.
# ESM2-650M embeddings computed at inference time and cached per-job.

FROM nvidia/cuda:11.8.0-cudnn8-runtime-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONUNBUFFERED=1

# git-lfs is required to pull model weight files from GitHub (stored as LFS objects)
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip python3-dev \
    git git-lfs ca-certificates \
    && git lfs install \
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

# Clone DiffDock source + model weights via Git LFS.
# The workdir/ directory contains LFS-tracked model weight files (~150MB each).
# GIT_LFS_SKIP_SMUDGE=0 forces LFS objects to be downloaded (the default, made explicit).
RUN GIT_LFS_SKIP_SMUDGE=0 git clone --depth=1 \
    https://github.com/gcorso/DiffDock /opt/diffdock

# Verify model weights were downloaded (LFS pointers would be ~130 bytes; real files are >50MB)
RUN python3 -c "
import os, sys
for f in [
    '/opt/diffdock/workdir/paper_score_model/best_model_epoch75.pt',
    '/opt/diffdock/workdir/paper_confidence_model/best_model.pt',
]:
    size = os.path.getsize(f) if os.path.exists(f) else 0
    print(f'{f}: {size} bytes')
    if size < 1_000_000:
        print('ERROR: file too small — LFS download may have failed')
        sys.exit(1)
print('Model weights OK')
"

# Pre-download ESM2-650M weights so inference doesn't hit the network at runtime.
# The model is stored in the torch hub cache and loaded by fair-esm automatically.
RUN python3 -c "import esm; esm.pretrained.esm2_t33_650M_UR50D()" \
    || echo "WARNING: ESM pre-download failed; will retry at inference time"

COPY scripts/dock_batch.py /scripts/dock_batch.py

CMD ["python3", "/scripts/dock_batch.py"]
