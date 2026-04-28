# Library preparation sidecar for the Khemeia SBDD pipeline.
# Compound standardization, filtering, and 3D conformer generation.

FROM python:3.11-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libxrender1 \
    libxext6 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

# Install meeko separately with --no-deps to avoid dependency conflicts
# with rdkit-pypi. Meeko's required runtime deps (numpy, rdkit) are already
# satisfied by requirements.txt above.
RUN pip install --no-cache-dir --no-deps meeko

COPY app.py ./

EXPOSE 8000

CMD ["gunicorn", "-w", "2", "-b", "0.0.0.0:8000", "--timeout", "600", "app:app"]
