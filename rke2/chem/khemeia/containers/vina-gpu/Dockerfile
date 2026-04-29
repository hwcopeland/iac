# Vina-GPU 2.1 docking engine sidecar
# GPU-accelerated AutoDock Vina using OpenCL for massively parallel docking.
# Multi-stage build: compile from source, then copy binary + OpenCL kernels.
#
# Build note: uses BUILD_KERNEL_FROM_SOURCE so kernels are compiled on first
# run rather than requiring prebuilt .bin files. This avoids GPU-architecture
# mismatch between build host and runtime host.
#
# API: POST /dock, GET /health, GET /readyz

# ---------------------------------------------------------------------------
# Build stage: compile Vina-GPU 2.1 from source
# ---------------------------------------------------------------------------
FROM nvidia/cuda:12.2.0-devel-ubuntu22.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    make \
    g++ \
    wget \
    libboost-all-dev \
    ocl-icd-opencl-dev \
    && rm -rf /var/lib/apt/lists/*

# Clone Vina-GPU 2.1 source
RUN git clone --depth 1 https://github.com/DeltaGroupNJUPT/Vina-GPU-2.1.git /opt/vina-gpu

WORKDIR /opt/vina-gpu/AutoDock-Vina-GPU-2.1

# Patch the Makefile for container build paths:
#   - BOOST_LIB_PATH: system-installed boost headers are in /usr/include
#   - OPENCL_LIB_PATH: CUDA toolkit provides OpenCL in /usr/local/cuda
#   - OPENCL_VERSION: use OpenCL 3.0 (recommended for Linux per upstream docs)
#   - GPU_PLATFORM: NVIDIA
#   - DOCKING_BOX_SIZE: support large boxes for flexible virtual screening
RUN sed -i \
    -e 's|^WORK_DIR=.*|WORK_DIR=/opt/vina-gpu/AutoDock-Vina-GPU-2.1|' \
    -e 's|^BOOST_LIB_PATH=.*|BOOST_LIB_PATH=/usr/include|' \
    -e 's|^OPENCL_LIB_PATH=.*|OPENCL_LIB_PATH=/usr/local/cuda|' \
    -e 's|^OPENCL_VERSION=.*|OPENCL_VERSION=-DOPENCL_3_0|' \
    -e 's|^GPU_PLATFORM=.*|GPU_PLATFORM=-DNVIDIA_PLATFORM|' \
    -e 's|^DOCKING_BOX_SIZE=.*|DOCKING_BOX_SIZE=-DLARGE_BOX|' \
    Makefile

# Build with BUILD_KERNEL_FROM_SOURCE so OpenCL kernels are compiled at
# runtime on the actual GPU, avoiding architecture mismatch.
RUN make source

# ---------------------------------------------------------------------------
# Runtime stage
# ---------------------------------------------------------------------------
FROM nvidia/cuda:12.2.0-runtime-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    python3 \
    python3-pip \
    libboost-program-options1.74.0 \
    libboost-system1.74.0 \
    libboost-filesystem1.74.0 \
    libboost-thread1.74.0 \
    ocl-icd-libopencl1 \
    && rm -rf /var/lib/apt/lists/*

# Copy the compiled binary
COPY --from=builder \
    /opt/vina-gpu/AutoDock-Vina-GPU-2.1/AutoDock-Vina-GPU-2-1 \
    /usr/local/bin/vina-gpu

# Copy OpenCL kernel source files (compiled on first run by the binary)
COPY --from=builder \
    /opt/vina-gpu/AutoDock-Vina-GPU-2.1/OpenCL/ \
    /opt/vina-gpu/OpenCL/

# Pre-compiled kernel binaries if they exist (optional, speeds up first run)
COPY --from=builder \
    /opt/vina-gpu/AutoDock-Vina-GPU-2.1/Kernel*.bin \
    /opt/vina-gpu/

WORKDIR /app

COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

COPY app.py ./

EXPOSE 8000

# Single worker for GPU-bound workloads to avoid VRAM contention
CMD ["gunicorn", "-w", "1", "-b", "0.0.0.0:8000", "--timeout", "600", "app:app"]
