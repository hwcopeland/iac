# GROMACS GPU molecular dynamics engine
# Uses NVIDIA NGC pre-built GROMACS image — CUDA-enabled, no compile step.
# Just adds Python deps and our scripts on top.
#
# Target: RTX 3070 (Ampere, SM 8.6) on nixos-gpu.

FROM nvcr.io/hpc/gromacs:2023.2

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3-pip \
    openbabel \
    libhdf4-0-alt \
    && rm -rf /var/lib/apt/lists/* \
    && pip3 install --no-cache-dir --upgrade pip

WORKDIR /scripts
COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

COPY scripts/ /scripts/
COPY mdp/ /mdp/

CMD ["python3", "/scripts/md_batch.py"]
