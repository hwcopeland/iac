# ESMFold single-sequence structure-prediction worker (GPU).
# Registered in .github/workflows/build-chem-images.yml's detect-changes matrix (key: esmfold).
#
# Folds a protein sequence -> PDB + pLDDT on the RTX 3070 (nixos-gpu) with no
# MSA. Serves DOUBLE DUTY for the Khemeia genomics layer (workers TDD §4):
#   - resolve_fallback role: fold the WT sequence only when AlphaFold DB misses,
#     supplying the resolved structure every downstream calc needs.
#   - calc role (calculation=esmfold): fold WT + mutant, report pLDDT + CA-RMSD.
#
# WEIGHTS ARE NOT BAKED INTO THE IMAGE (workers TDD §4.1 decision). ESMFold v1 +
# its ESM-2 3B backbone are several GB; baking them bloats every registry pull
# and the Kaniko build. They live on a node-local hostPath PVC on nixos-gpu
# (deploy/esmfold-weights-pvc.yaml, GEN-21), mounted RW at $TORCH_HOME, and are
# populated once by the esmfold-weights-warm Job (GEN-21) — mirroring the
# vina-gpu kernel-cache precedent. fold.py loads from $TORCH_HOME (= the torch
# hub cache the fair-esm loader reads/writes).
#
# CUDA/torch line MIRRORS the diffdock image (the live fair-esm/ESM GPU worker
# on this exact node): CUDA 11.8 + torch 2.1.2+cu118 supports the host driver
# 580 and is the validated fair-esm stack here. (Workers TDD §4.1 suggested CUDA
# 12.1; diffdock is the in-repo precedent for fair-esm specifically, so we follow
# it for ABI/driver parity — see the GEN-20 report note.)
#
# Build on the cluster ONLY (Kaniko via Flux image-automation, GEN-21). Never
# on the Mac.

# ── Stage 1: builder ─────────────────────────────────────────────────────────
# -devel (not -runtime): openfold compiles custom CUDA kernels at install and
# needs nvcc, which only ships in the -devel CUDA image. The runtime stage below
# stays on -runtime and just copies the built site-packages.
FROM nvidia/cuda:11.8.0-cudnn8-devel-ubuntu22.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONUNBUFFERED=1

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip python3-dev \
    gcc g++ make \
    git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# numpy<2 required by fair-esm (compiled against the NumPy 1.x ABI).
RUN pip3 install --no-cache-dir "numpy<2"

# PyTorch 2.1.2 + CUDA 11.8 — supports driver 520+ (host is 580). Matches diffdock.
RUN pip3 install --no-cache-dir \
    torch==2.1.2+cu118 \
    --extra-index-url https://download.pytorch.org/whl/cu118

# ESMFold deps. fair-esm[esmfold] pulls the ESMFold structure module which needs
# openfold (pinned commit — openfold has no stable PyPI release) and its helpers.
# Requirements are split into a file for visibility / cache reuse.
COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

# openfold: ESMFold's structure module (no stable PyPI release). Its setup.py
# COMPILES custom CUDA attention kernels at install time, which requires nvcc —
# hence the -devel builder base above. TORCH_CUDA_ARCH_LIST targets the RTX 3070
# (sm_86) on nixos-gpu so the kernels match the deployment GPU; the compiled .so
# are carried into the runtime stage with the rest of site-packages.
#
# SUPPLY-CHAIN PIN (VULN-5): pinned to an IMMUTABLE commit SHA, not the mutable
# @v1.0.1 git tag (tags can be force-moved upstream). The SHA below is exactly
# what refs/tags/v1.0.1 pointed to as of `git ls-remote` on 2026-06-12:
#   aqlaboratory/openfold  tag v1.0.1 -> 42e71db7fa327e0810eb0e371abc9f82aa9b7a6a
# dllogger has no release tags; pinned to default-branch HEAD on 2026-06-12
#   NVIDIA/dllogger  master @ 0478734ff7be75adde8d160e04872664d1c62e5f
ENV CUDA_HOME=/usr/local/cuda
ENV TORCH_CUDA_ARCH_LIST="8.6"
ENV FORCE_CUDA=1
RUN pip3 install --no-cache-dir \
    'dllogger @ git+https://github.com/NVIDIA/dllogger.git@0478734ff7be75adde8d160e04872664d1c62e5f' \
    'openfold @ git+https://github.com/aqlaboratory/openfold.git@42e71db7fa327e0810eb0e371abc9f82aa9b7a6a'

# NOTE: no model-weight download here. Weights load at runtime from the PVC
# ($TORCH_HOME), populated by the GEN-21 warm Job. This keeps the image lean and
# the Kaniko build offline-friendly.

# ── Stage 2: runtime ─────────────────────────────────────────────────────────
FROM nvidia/cuda:11.8.0-cudnn8-runtime-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONUNBUFFERED=1

# TORCH_HOME points at the node-local weights PVC mount (GEN-21). The fair-esm
# loader (torch.hub) reads/writes pretrained weights under $TORCH_HOME/hub.
# ESM_CACHE is an explicit alias fold.py also honours. Both default here so the
# image is runnable standalone; GEN-21's pod spec re-asserts the mount path.
ENV TORCH_HOME=/opt/esmfold/weights
ENV ESM_CACHE=/opt/esmfold/weights

# Runtime only — no gcc/make/git/python3-dev.
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy all pip-installed packages from the builder.
COPY --from=builder /usr/local/lib/python3.10 /usr/local/lib/python3.10
COPY --from=builder /usr/local/bin /usr/local/bin

# Weights dir is a mount point at runtime; create it so a missing PVC still
# yields a clear "empty cache" path rather than an import error.
RUN mkdir -p /opt/esmfold/weights

WORKDIR /opt/esmfold
COPY scripts/ scripts/

CMD ["python3", "scripts/fold.py"]
