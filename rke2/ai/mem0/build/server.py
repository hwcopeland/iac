"""mem0 REST service for JARVIS — CPU-only memory engine.

Architecture (Phase 6 of the JARVIS identity/memory platform):

    jarvis-edge OWNER brain  (claude CLI subprocess, JARVIS_MEM_SCOPE in env)
        └─ stdio MCP: jarvis_mem0_mcp.py  (ships in the edge image)
              └─ HTTP ─▶ THIS service (mem0 lib + fastembed + xAI Grok)
                            └─ Qdrant  (vector store, Longhorn-backed)

Why a separate service (not mem0-in-edge):
  * keeps mem0 / fastembed (onnxruntime) / anthropic OUT of the edge image
  * keeps everything off the GPU node (this runs on a Longhorn node)
  * one place owns the Qdrant client + extraction key

Partitioning — THE security-relevant invariant:
  Every /add and /search REQUIRES a non-empty `user_id`. That value is the
  caller's `mem_scope`:  "owner" for the owner (voice OR platform, collapsed),
  else the per-user_id ("voice:alex", "discord:123", ...). mem0 stores and
  retrieves strictly within that user_id namespace, so a TRUSTED user can never
  read the owner's memories — AND, because the locked brain has no MCP config at
  all, a non-owner never even reaches this service. Defense in depth: the gate
  (no tool) + this API (scoped user_id).

Embeddings: fastembed (BAAI/bge-small-en-v1.5, 384-dim) — pure CPU, no GPU,
no ollama. The 3070 stays free for Chatterbox/Whisper.

Extraction LLM: xAI Grok via mem0's OpenAI-compatible provider (xAI's API is
OpenAI-API-compatible at https://api.x.ai/v1). The key is XAI_API_KEY, REUSED
from the existing `homelab-bot-credentials` Secret in this same `ai` namespace —
there is exactly ONE xAI key in the cluster, no new key and no new spend. This
is NOT the edge's subscription OAuth — mem0's LLM provider needs an API key, and
the subscription token cannot be handed to it. See docs/jarvis/phase6-mem0.md.
"""
from __future__ import annotations

import json
import logging
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

logging.basicConfig(level=logging.INFO, format="%(asctime)s mem0 %(levelname)s %(message)s")
log = logging.getLogger("mem0-server")

PORT = int(os.environ.get("MEM0_HTTP_PORT", "8800"))
FASTEMBED_MODEL = os.environ.get("FASTEMBED_MODEL", "BAAI/bge-small-en-v1.5")
FASTEMBED_DIMS = int(os.environ.get("FASTEMBED_DIMS", "384"))
QDRANT_HOST = os.environ.get("QDRANT_HOST", "qdrant.ai.svc.cluster.local")
QDRANT_PORT = int(os.environ.get("QDRANT_PORT", "6333"))
QDRANT_COLLECTION = os.environ.get("QDRANT_COLLECTION", "jarvis_mem0")
EXTRACTION_MODEL = os.environ.get("EXTRACTION_MODEL", "grok-3-mini")
# xAI is OpenAI-API-compatible; mem0's OpenAI provider talks to it via a
# base_url override. Default to xAI's endpoint but allow an env override.
XAI_BASE_URL = os.environ.get("XAI_BASE_URL", "https://api.x.ai/v1")

# ── Build the mem0 Memory once, at import time ───────────────────────────────
# mem0 config wires THREE pluggable pieces to our self-hosted choices:
#   llm        → xAI Grok (via mem0's OpenAI provider + base_url override) for
#                fact extraction on add(). One shared XAI_API_KEY (see header).
#   embedder   → fastembed (CPU) for vectorizing facts + queries
#   vector_store → qdrant (our StatefulSet)
_MEM0_CONFIG = {
    "llm": {
        # xAI exposes an OpenAI-compatible Chat Completions API, so we use
        # mem0's "openai" provider and point openai_base_url at xAI. The key is
        # passed as api_key (the OpenAI client reads it from there). We also set
        # OPENAI_API_KEY/OPENAI_BASE_URL in env below so any code path that
        # reads the env (rather than this config) lands on xAI too.
        "provider": "openai",
        "config": {
            "model": EXTRACTION_MODEL,
            "api_key": os.environ.get("XAI_API_KEY", ""),
            "openai_base_url": XAI_BASE_URL,
            "temperature": 0.1,
            "max_tokens": 1024,
        },
    },
    "embedder": {
        "provider": "fastembed",
        "config": {
            "model": FASTEMBED_MODEL,
        },
    },
    "vector_store": {
        "provider": "qdrant",
        "config": {
            "collection_name": QDRANT_COLLECTION,
            "host": QDRANT_HOST,
            "port": QDRANT_PORT,
            "embedding_model_dims": FASTEMBED_DIMS,
        },
    },
}

_memory = None


def _get_memory():
    """Lazily construct the mem0 Memory (importing mem0 is heavy)."""
    global _memory
    if _memory is None:
        xai_key = os.environ.get("XAI_API_KEY", "")
        if not xai_key:
            raise RuntimeError(
                "XAI_API_KEY is empty — mem0 extraction needs the shared xAI "
                "Grok key (XAI_API_KEY from the homelab-bot-credentials Secret)."
            )
        # mem0's OpenAI provider may construct its client from env rather than
        # from the passed config in some code paths; mirror the key + base_url
        # into the env the OpenAI SDK reads so it always targets xAI, not OpenAI.
        os.environ.setdefault("OPENAI_API_KEY", xai_key)
        os.environ.setdefault("OPENAI_BASE_URL", XAI_BASE_URL)
        from mem0 import Memory  # imported here so /health can answer pre-init
        _memory = Memory.from_config(_MEM0_CONFIG)
        log.info("mem0 Memory initialized (embed=%s qdrant=%s:%s coll=%s extract=%s)",
                 FASTEMBED_MODEL, QDRANT_HOST, QDRANT_PORT, QDRANT_COLLECTION,
                 EXTRACTION_MODEL)
    return _memory


def _require_scope(body: dict) -> str:
    """Pull the partition key. We accept `user_id` (mem0's native field) and
    `mem_scope` as an alias. EMPTY OR MISSING IS REJECTED — never let an add or
    search run against the global/implicit namespace."""
    scope = (body.get("user_id") or body.get("mem_scope") or "").strip()
    if not scope:
        raise ValueError("user_id (mem_scope) is required and must be non-empty")
    return scope


class Handler(BaseHTTPRequestHandler):
    def _send(self, code: int, payload: dict):
        data = json.dumps(payload).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, fmt, *args):  # quiet the default noisy logger
        log.info("%s - %s", self.address_string(), fmt % args)

    def _read_json(self) -> dict:
        length = int(self.headers.get("Content-Length", 0) or 0)
        if length <= 0:
            return {}
        raw = self.rfile.read(length)
        return json.loads(raw or b"{}")

    def do_GET(self):
        if self.path.rstrip("/") == "/health":
            # Cheap liveness: report whether mem0 has been initialized and the
            # config it WILL use. Doesn't force-init (that pulls onnxruntime).
            self._send(200, {
                "ok": True,
                "initialized": _memory is not None,
                "embed_model": FASTEMBED_MODEL,
                "embed_dims": FASTEMBED_DIMS,
                "qdrant": f"{QDRANT_HOST}:{QDRANT_PORT}",
                "collection": QDRANT_COLLECTION,
                "extraction_model": EXTRACTION_MODEL,
                "extraction_base_url": XAI_BASE_URL,
                "has_api_key": bool(os.environ.get("XAI_API_KEY")),
            })
            return
        self._send(404, {"error": "not found"})

    def do_POST(self):
        try:
            body = self._read_json()
        except Exception as exc:  # noqa: BLE001
            self._send(400, {"error": f"bad json: {exc}"})
            return

        path = self.path.rstrip("/")
        try:
            if path == "/add":
                # Body: {messages|text, user_id|mem_scope, metadata?}
                scope = _require_scope(body)
                messages = body.get("messages")
                if messages is None:
                    text = (body.get("text") or "").strip()
                    if not text:
                        raise ValueError("provide `messages` or `text`")
                    messages = [{"role": "user", "content": text}]
                metadata = body.get("metadata") or None
                mem = _get_memory()
                result = mem.add(messages, user_id=scope, metadata=metadata)
                self._send(200, {"ok": True, "user_id": scope, "result": result})
                return

            if path == "/search":
                # Body: {query, user_id|mem_scope, limit?}
                scope = _require_scope(body)
                query = (body.get("query") or "").strip()
                if not query:
                    raise ValueError("`query` is required")
                limit = int(body.get("limit", 5))
                mem = _get_memory()
                result = mem.search(query, user_id=scope, limit=limit)
                self._send(200, {"ok": True, "user_id": scope, "result": result})
                return

            if path == "/get_all":
                scope = _require_scope(body)
                mem = _get_memory()
                result = mem.get_all(user_id=scope)
                self._send(200, {"ok": True, "user_id": scope, "result": result})
                return

            self._send(404, {"error": "not found"})
        except ValueError as exc:
            self._send(400, {"error": str(exc)})
        except Exception as exc:  # noqa: BLE001
            log.exception("mem0 op failed")
            self._send(500, {"error": str(exc)})


def main():
    srv = ThreadingHTTPServer(("0.0.0.0", PORT), Handler)
    log.info("mem0 server listening on :%d (extraction=%s)", PORT, EXTRACTION_MODEL)
    srv.serve_forever()


if __name__ == "__main__":
    main()
