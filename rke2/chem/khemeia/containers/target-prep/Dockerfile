# Target preparation sidecar for the Khemeia SBDD pipeline.
# Cleans receptor PDB structures via PDBFixer and defines binding-site boxes.

FROM python:3.11-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libxrender1 \
    libxext6 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py ./

EXPOSE 8000

CMD ["gunicorn", "-w", "2", "-b", "0.0.0.0:8000", "--timeout", "600", "app:app"]
