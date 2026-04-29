# Vina-GPU 2.1 docking engine
# Uses the upstream AutoDock-Vina-GPU 2.1 binary and precompiled OpenCL kernels.
# The host NVIDIA/OpenCL runtime is exposed via the nixos-gpu host-path mounts.

FROM ubuntu:22.04

ARG VINA_GPU_REPO_COMMIT=180272b8a5265d6ed9664178345933cebe2cd349
ARG VINA_GPU_METHOD=AutoDock-Vina-GPU-2.1
ARG VINA_GPU_BINARY=AutoDock-Vina-GPU-2-1

ENV DEBIAN_FRONTEND=noninteractive \
    PYTHONUNBUFFERED=1 \
    VINA_GPU_BIN=/opt/vina-gpu/bin/AutoDock-Vina-GPU-2-1 \
    VINA_GPU_OPENCL_BINARY_PATH=/opt/vina-gpu/bin \
    LD_LIBRARY_PATH=/run/opengl-driver/lib:/usr/lib/x86_64-linux-gnu

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates wget python3 python3-pip \
    libboost-filesystem1.74.0 libboost-program-options1.74.0 libboost-system1.74.0 \
    libgomp1 ocl-icd-libopencl1 \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/vina-gpu/bin \
    && wget -q -O /opt/vina-gpu/bin/${VINA_GPU_BINARY} \
        https://raw.githubusercontent.com/DeltaGroupNJUPT/Vina-GPU-2.1/${VINA_GPU_REPO_COMMIT}/${VINA_GPU_METHOD}/${VINA_GPU_BINARY} \
    && chmod +x /opt/vina-gpu/bin/${VINA_GPU_BINARY} \
    && wget -q -O /opt/vina-gpu/bin/Kernel1_Opt.bin \
        https://raw.githubusercontent.com/DeltaGroupNJUPT/Vina-GPU-2.1/${VINA_GPU_REPO_COMMIT}/${VINA_GPU_METHOD}/Kernel1_Opt.bin \
    && wget -q -O /opt/vina-gpu/bin/Kernel2_Opt.bin \
        https://raw.githubusercontent.com/DeltaGroupNJUPT/Vina-GPU-2.1/${VINA_GPU_REPO_COMMIT}/${VINA_GPU_METHOD}/Kernel2_Opt.bin \
    && ln -sf /opt/vina-gpu/bin/${VINA_GPU_BINARY} /usr/local/bin/vina-gpu

WORKDIR /app

COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

COPY scripts/ /scripts/
COPY app.py ./

CMD ["python3", "/scripts/dock_batch.py"]
