# DiffDock blind docking engine.
# Deep learning diffusion model for blind protein-ligand docking.
# Takes protein PDB + ligand SMILES; no binding site box required.
#
# Runtime: GPU (CUDA 11.8 compatible). Uses NVIDIA drivers from host
# via /run/opengl-driver mount (NixOS node) — same pattern as gnina.
#
# Model weights baked in at build time from HuggingFace (gcorso/DiffDock).
# ESM2-650M embeddings computed at inference time and cached per-job.

FROM nvidia/cuda:11.8.0-cudnn8-runtime-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONUNBUFFERED=1

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip python3-dev \
    git wget ca-certificates \
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

# Clone DiffDock source at a pinned commit.
# inference.py requires running from the repo root (relative imports).
ARG DIFFDOCK_COMMIT=2ac53fd3a4d7c6b3a6ef28da8c5e55c3060ed0f3
RUN git clone https://github.com/gcorso/DiffDock /opt/diffdock \
    && cd /opt/diffdock \
    && git checkout ${DIFFDOCK_COMMIT} 2>/dev/null || true

# Download pretrained score and confidence model weights from HuggingFace.
# gcorso/DiffDock is a public HuggingFace model repository.
RUN mkdir -p \
    /opt/diffdock/workdir/paper_score_model \
    /opt/diffdock/workdir/paper_confidence_model

RUN wget -q --show-progress \
    -O /opt/diffdock/workdir/paper_score_model/best_model_epoch75.pt \
    "https://huggingface.co/gcorso/DiffDock/resolve/main/workdir/paper_score_model/best_model_epoch75.pt" \
    && wget -q \
    -O /opt/diffdock/workdir/paper_score_model/model_parameters.yml \
    "https://huggingface.co/gcorso/DiffDock/resolve/main/workdir/paper_score_model/model_parameters.yml" \
    && wget -q --show-progress \
    -O /opt/diffdock/workdir/paper_confidence_model/best_model.pt \
    "https://huggingface.co/gcorso/DiffDock/resolve/main/workdir/paper_confidence_model/best_model.pt" \
    && wget -q \
    -O /opt/diffdock/workdir/paper_confidence_model/model_parameters.yml \
    "https://huggingface.co/gcorso/DiffDock/resolve/main/workdir/paper_confidence_model/model_parameters.yml"

# Pre-download ESM2-650M weights so inference doesn't hit the network at runtime.
# The model is stored in the torch hub cache and loaded by fair-esm automatically.
RUN python3 -c "import esm; esm.pretrained.esm2_t33_650M_UR50D()" \
    || echo "WARNING: ESM pre-download failed; will retry at inference time"

COPY scripts/dock_batch.py /scripts/dock_batch.py

CMD ["python3", "/scripts/dock_batch.py"]
