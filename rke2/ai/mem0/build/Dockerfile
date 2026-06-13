# mem0 REST service for JARVIS — CPU-only.
#
# Built ON THE CLUSTER (Kaniko/nerdctl/CI → zot), NEVER on the Mac.
# Pushed to zot.hwcopeland.net/ai/jarvis-mem0:latest.
#
# Heavy deps (mem0, fastembed/onnxruntime, anthropic) live HERE, not in the
# jarvis-edge image — the edge only ships a thin stdio MCP shim that HTTP-calls
# this service. That keeps the GPU-node edge image small and the memory engine
# isolated on a Longhorn node.
FROM python:3.11-slim

# onnxruntime (fastembed backend) needs libgomp; curl for the healthcheck.
RUN apt-get update \
    && apt-get install -y --no-install-recommends libgomp1 curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Pre-download the fastembed model at build time so the first request isn't a
# cold download. BAAI/bge-small-en-v1.5 = 384-dim, ~130MB, strong CPU model.
RUN python -c "from fastembed import TextEmbedding; TextEmbedding('BAAI/bge-small-en-v1.5')"

COPY server.py .

ENV MEM0_HTTP_PORT=8800 \
    FASTEMBED_MODEL=BAAI/bge-small-en-v1.5 \
    FASTEMBED_DIMS=384 \
    QDRANT_HOST=qdrant.ai.svc.cluster.local \
    QDRANT_PORT=6333 \
    QDRANT_COLLECTION=jarvis_mem0 \
    EXTRACTION_MODEL=claude-haiku-4-5-20251001
EXPOSE 8800

CMD ["python", "-u", "server.py"]
