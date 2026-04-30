# GROMACS GPU molecular dynamics engine
# Two-stage build: compile GROMACS 2024.4 with CUDA 12.8 in builder, copy to
# slim runtime. Target: RTX 3070 (Ampere, SM 8.6) on nixos-gpu.
#
# Force field stack:
#   Protein — CHARMM36m (bundled with GROMACS)
#   Ligand  — OpenFF GAFF-2.11 via openff-toolkit + openff-interchange (no AmberTools needed)
#   Water   — TIP3P-CHARMM
#
# GPU offload: PME + nonbonded on GPU via CUDA; CPU handles constraints.

FROM nvidia/cuda:12.8.1-devel-ubuntu22.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    cmake \
    g++ \
    gcc \
    libfftw3-dev \
    wget \
    && rm -rf /var/lib/apt/lists/*

ARG GROMACS_VERSION=2024.4
RUN wget -q "https://ftp.gromacs.org/gromacs/gromacs-${GROMACS_VERSION}.tar.gz" \
    && tar xzf "gromacs-${GROMACS_VERSION}.tar.gz" \
    && cd "gromacs-${GROMACS_VERSION}" \
    && mkdir build && cd build \
    && cmake .. \
        -DGMX_GPU=CUDA \
        -DCUDA_TOOLKIT_ROOT_DIR=/usr/local/cuda \
        -DGMX_CUDA_TARGET_SM="86" \
        -DGMX_BUILD_OWN_FFTW=OFF \
        -DGMX_MPI=OFF \
        -DGMX_SIMD=AVX2_256 \
        -DREGRESSIONTEST_DOWNLOAD=OFF \
        -DGMX_BUILD_UNITTESTS=OFF \
        -DCMAKE_BUILD_TYPE=Release \
        -DGMX_DOUBLE=OFF \
        -DCMAKE_INSTALL_PREFIX=/opt/gromacs \
    && make -j$(nproc) \
    && make install \
    && cd / && rm -rf gromacs-${GROMACS_VERSION}*

FROM nvidia/cuda:12.8.1-cudnn-runtime-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    python3-pip \
    libfftw3-single3 \
    libgomp1 \
    openbabel \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /opt/gromacs /opt/gromacs

ENV PATH="/opt/gromacs/bin:$PATH"
ENV GMXLIB="/opt/gromacs/share/gromacs/top"

WORKDIR /scripts
COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

COPY scripts/ /scripts/
COPY mdp/ /mdp/

CMD ["python3", "/scripts/md_batch.py"]
