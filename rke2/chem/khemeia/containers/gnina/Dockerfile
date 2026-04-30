# Gnina CNN-scored docking engine
# GPU-accelerated docking with CNN affinity and pose scoring.
# Requires CUDA runtime; actual GPU libs mounted from NixOS host at runtime.
#
# Primary mode: batch worker (dock_batch.py) invoked as a K8s Job.
# The Flask app (app.py) is retained for health checks only.

FROM nvidia/cuda:12.2.0-runtime-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Etc/UTC

# Install cuDNN 9 runtime (required by gnina 1.3.2 binary — links libcudnn.so.9).
# The NVIDIA machine learning repo provides cudnn packages for Ubuntu 22.04.
RUN apt-get update && apt-get install -y --no-install-recommends \
    wget \
    ca-certificates \
    python3 \
    python3-pip \
    libopenbabel-dev \
    libboost-all-dev \
    && wget -q -O /tmp/cuda-keyring.deb \
       https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/x86_64/cuda-keyring_1.1-1_all.deb \
    && dpkg -i /tmp/cuda-keyring.deb \
    && rm /tmp/cuda-keyring.deb \
    && apt-get update \
    && apt-get install -y --no-install-recommends libcudnn9-cuda-12 \
    && rm -rf /var/lib/apt/lists/*

# Install gnina from GitHub releases (static build, pinned to v1.3.2)
RUN wget -q -O /usr/local/bin/gnina \
    "https://github.com/gnina/gnina/releases/download/v1.3.2/gnina.1.3.2" \
    && chmod +x /usr/local/bin/gnina

WORKDIR /app

COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

# Batch docking script (primary entrypoint for K8s Jobs)
COPY scripts/ /scripts/

# Flask app retained for health checks
COPY app.py ./

CMD ["python3", "/scripts/dock_batch.py"]
