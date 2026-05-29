# Spotify → Prometheus exporter. Built on the arc-chem self-hosted runner
# (has LAN access to zot.hwcopeland.net) and pushed to
# zot.hwcopeland.net/monitor/spotify-exporter. linux/amd64 nodes.
FROM python:3.12-slim

ENV PYTHONUNBUFFERED=1 \
    SPOTIFY_STATE_DIR=/state

WORKDIR /app

COPY app/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app/exporter.py app/tokenstore.py ./

RUN useradd -u 10001 -r -s /usr/sbin/nologin exporter \
    && mkdir -p /state && chown exporter:exporter /state
USER 10001

EXPOSE 9112
ENTRYPOINT ["python", "/app/exporter.py"]
