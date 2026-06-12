# P2Rank — machine-learning pocket prediction for binding-site definition (WP-1)
# Downloads a P2Rank release and wraps it in a Flask REST API.
#
# Dual role (GEN-31): the DEFAULT entrypoint (gunicorn, app:app) is the unchanged
# P2Rank Flask sidecar. The genomics `pocket_proximity` worker
# (scripts/pocket_proximity.py) is baked in as an ALTERNATE entrypoint, selected
# by the plugin command override (`cd /opt/p2rank && python3 scripts/pocket_proximity.py`).
# Reuses this same image — no new image, no new science.

FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONUNBUFFERED=1

# P2Rank version — bump this to upgrade
ENV P2RANK_VERSION=2.4.2

RUN apt-get update && apt-get install -y --no-install-recommends \
    openjdk-17-jre-headless \
    wget \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Download and extract P2Rank
RUN wget -q "https://github.com/rdk/p2rank/releases/download/${P2RANK_VERSION}/p2rank_${P2RANK_VERSION}.tar.gz" \
        -O /tmp/p2rank.tar.gz && \
    tar xzf /tmp/p2rank.tar.gz -C /opt/ && \
    mv /opt/p2rank_${P2RANK_VERSION} /opt/p2rank && \
    chmod +x /opt/p2rank/prank && \
    rm /tmp/p2rank.tar.gz

ENV PATH="/opt/p2rank:${PATH}"

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py ./

# Genomics pocket_proximity worker (GEN-31): baked into the P2Rank home so the
# plugin command (`cd /opt/p2rank && python3 scripts/pocket_proximity.py`) finds
# it. Pure stdlib + psycopg2-binary + boto3 (the PDB distance math is ported from
# api/handlers_pocket.go, no extra structural libs). Does NOT change the default
# gunicorn entrypoint below, so the existing P2Rank sidecar usage is unaffected.
COPY scripts/ /opt/p2rank/scripts/

EXPOSE 8000

CMD ["gunicorn", "-w", "2", "-b", "0.0.0.0:8000", "--timeout", "300", "app:app"]
