# AutoDock Vina 1.2 docking engine sidecar
# Modern Vina from the Forli Lab with Python bindings.
# CPU-only, replaces legacy Vina 1.1.2 as the default CPU engine.
#
# API: POST /dock, GET /health, GET /readyz

FROM python:3.11-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libxrender1 \
    libxext6 \
    swig \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY scripts/dock.py ./dock.py

EXPOSE 8000

CMD ["gunicorn", "-w", "2", "-b", "0.0.0.0:8000", "--timeout", "300", "dock:app"]
