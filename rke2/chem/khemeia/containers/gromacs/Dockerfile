# GROMACS GPU molecular dynamics engine
# Uses NVIDIA NGC pre-built GROMACS image — CUDA-enabled, no compile step.
# Just adds Python deps and our scripts on top.
#
# Target: RTX 3070 (Ampere, SM 8.6) on nixos-gpu.

FROM nvcr.io/hpc/gromacs:2023.2

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    software-properties-common \
    && add-apt-repository -y ppa:deadsnakes/ppa \
    && apt-get update && apt-get install -y --no-install-recommends \
    python3.11 \
    python3.11-dev \
    python3.11-distutils \
    openbabel \
    && rm -rf /var/lib/apt/lists/* \
    && python3.11 -m ensurepip --upgrade

WORKDIR /scripts
COPY requirements.txt ./
RUN python3.11 -m pip install --no-cache-dir -r requirements.txt

COPY scripts/ /scripts/
COPY mdp/ /mdp/

CMD ["python3.11", "/scripts/md_batch.py"]
