# AutoDock Vina 1.2 docking engine
# Modern Vina from the Forli Lab with Python bindings.
# CPU-only, replaces legacy Vina 1.1.2 as the default CPU engine.
#
# Primary mode: batch worker (dock_batch.py) invoked as a K8s Job.
# The Flask app (dock.py) is retained for health checks only.

FROM python:3.11-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libxrender1 \
    libxext6 \
    swig \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

# Batch docking script (primary entrypoint for K8s Jobs)
COPY scripts/ /scripts/

# Flask app for health checks (retained for backward compat)
COPY scripts/dock.py ./dock.py

CMD ["python3", "/scripts/dock_batch.py"]
