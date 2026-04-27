# Smina docking engine sidecar
# Vina fork with custom scoring terms and flexible residue support.
# CPU-only.
#
# API: POST /dock, GET /health, GET /readyz

FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Etc/UTC

RUN apt-get update && apt-get install -y --no-install-recommends \
    wget \
    ca-certificates \
    python3 \
    python3-pip \
    libopenbabel-dev \
    && rm -rf /var/lib/apt/lists/*

# Install smina binary from sourceforge (static build)
RUN wget -q -O /usr/local/bin/smina \
    "https://sourceforge.net/projects/smina/files/smina.static/download" \
    && chmod +x /usr/local/bin/smina

WORKDIR /app

COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

COPY app.py ./

EXPOSE 8000

CMD ["gunicorn", "-w", "2", "-b", "0.0.0.0:8000", "--timeout", "300", "app:app"]
