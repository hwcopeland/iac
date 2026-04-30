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
    libcurl4 \
    libjpeg8 \
    && rm -rf /var/lib/apt/lists/* \
    && pip3 install --no-cache-dir --upgrade pip

WORKDIR /scripts
COPY requirements.txt ./
# Install Python deps, then register ACPYPE's bundled AmberTools libs with ldconfig
# so teLeap can find libhdf5_hl.so.310 etc. without needing LD_LIBRARY_PATH.
RUN pip3 install --no-cache-dir -r requirements.txt \
    && echo '/usr/local/lib/python3.10/dist-packages/acpype/amber_linux/lib' \
       > /etc/ld.so.conf.d/acpype-amber.conf \
    && ldconfig

COPY scripts/ /scripts/
COPY mdp/ /mdp/

CMD ["python3", "/scripts/md_batch.py"]
