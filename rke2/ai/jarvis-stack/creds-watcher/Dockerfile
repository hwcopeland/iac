# Tiny sidecar: persist claude OAuth creds back to the K8s Secret.
# Pure stdlib — no pip install, no extra deps, no inotify dep.
# Alpine python:3.12-alpine is ~50MB total and starts in <1s.
FROM python:3.12-alpine

# Drop root in the image. The pod runs the main container as uid 1000
# so the emptyDir mount is owned by 1000:100; this sidecar needs to
# read the same files, so match the uid.
RUN addgroup -g 100 -S app 2>/dev/null || true \
    && adduser -u 1000 -G app -S -H app

WORKDIR /app
COPY watch.py /app/watch.py

USER 1000:100

# Unbuffered logs so kubectl logs is immediate.
ENV PYTHONUNBUFFERED=1
# Don't try to write __pycache__/ — image fs is read-only at runtime
# (see deployment.yaml securityContext.readOnlyRootFilesystem: true).
ENV PYTHONDONTWRITEBYTECODE=1

ENTRYPOINT ["python3", "/app/watch.py"]
