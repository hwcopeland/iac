# Spotify → Prometheus exporter. Built on the arc-chem self-hosted runner
# (has LAN access to zot.hwcopeland.net) and pushed to
# zot.hwcopeland.net/monitor/spotify-exporter. linux/amd64 nodes.
FROM python:3.12-slim

ENV PYTHONUNBUFFERED=1 \
    SPOTIFY_STATE_DIR=/state

WORKDIR /app

COPY app/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app/exporter.py app/tokenstore.py app/db.py app/genre_rollup.py app/genre_rollup.yaml ./
# One-time GDPR "Extended Streaming History" backfill, run as a Job (see
# import/streaming-history-import.yaml) reusing this same image.
COPY app/import_streaming_history.py ./
# MusicBrainz genre enrichment, run as a CronJob (see spotify-postgres/
# cronjob-enrich.yaml) reusing this same image: `python /app/enrich.py`.
COPY app/enrich.py ./

RUN useradd -u 10001 -r -s /usr/sbin/nologin exporter \
    && mkdir -p /state && chown exporter:exporter /state
USER 10001

EXPOSE 9112
ENTRYPOINT ["python", "/app/exporter.py"]
