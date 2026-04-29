# GPU-accelerated docking using NVIDIA's official AutoDock container
# https://catalog.ngc.nvidia.com/orgs/hpc/containers/autodock
#
# API: POST /dock, GET /health, GET /readyz

FROM nvcr.io/hpc/autodock:v1.5.1 AS base

# The NVIDIA AutoDock image has CUDA + AutoDock-GPU pre-installed.
# Add Python + Flask for the REST API wrapper.
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

COPY app.py ./

EXPOSE 8000

# Single worker — GPU-bound
CMD ["gunicorn", "-w", "1", "-b", "0.0.0.0:8000", "--timeout", "600", "app:app"]
