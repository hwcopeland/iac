FROM python:3.12-slim AS base
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
WORKDIR /srv

COPY pyproject.toml ./
# Dep list is hardcoded (not `pip install .` from pyproject) — keep in
# sync with pyproject.toml. The OTEL deps load FastAPIInstrumentor in
# app/main.py only when OTEL_EXPORTER_OTLP_ENDPOINT is set in env.
RUN pip install --no-cache-dir \
      "fastapi>=0.115" \
      "uvicorn[standard]>=0.32" \
      "mysql-connector-python>=9.1" \
      "httpx>=0.27" \
      "pydantic>=2.9" \
      "pydantic-settings>=2.6" \
      "opentelemetry-distro>=0.50b0" \
      "opentelemetry-exporter-otlp-proto-http>=1.30.0" \
      "opentelemetry-instrumentation-fastapi>=0.50b0"

COPY app/ ./app/

EXPOSE 8080
# opentelemetry-instrument auto-configures the TracerProvider + OTLP
# exporter from OTEL_* env vars (set in k8s/api-deployment.yaml). With
# no OTEL env, it's a near-noop and uvicorn runs normally.
CMD ["opentelemetry-instrument", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8080", "--proxy-headers"]
