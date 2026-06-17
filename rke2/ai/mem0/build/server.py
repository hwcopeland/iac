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
            # Extraction must emit a COMPLETE JSON object. A long fact (an
            # initiative paragraph) yields many atomic facts; at 1024 tokens the
            # JSON was truncated mid-string and mem0 logged "Error in
            # new_retrieved_facts: Unterminated string" and stored nothing.
            # 4096 gives the extraction/decision JSON room to finish.
            "max_tokens": 4096,
            # mem0's BaseLlmConfig defaults top_p=0 and ALWAYS forwards it to the
            # Chat Completions call. OpenAI tolerates top_p=0, but xAI rejects it
            # ("top_p must be positive but top_p = 0", HTTP 400). Set a valid
            # positive value so xAI accepts the extraction request.
            "top_p": 1.0,
        },
    },
    "embedder": {
        # "fastembed" is NOT a built-in mem0 0.1.55 provider; server.py
        # registers a custom CPU adapter under this name at startup (see
        # _register_fastembed_provider). embedding_dims must match the model
        # and the qdrant collection's vector size (BAAI/bge-small = 384).
        "provider": "fastembed",
        "config": {
            "model": FASTEMBED_MODEL,
            "embedding_dims": FASTEMBED_DIMS,
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


# Module-level so it is importable by a dotted path. mem0's EmbedderFactory.create
# does `load_class(class_type)` which `importlib.import_module(module).getattr(cls)`
# — it REQUIRES a "module.ClassName" string, not a class object. A class nested in
# a function has no importable path, so it must live at module scope and be
# registered as f"{__name__}._FastEmbedEmbedder".
class _FastEmbedEmbedder:
    """mem0 embedder backed by fastembed (onnxruntime, pure CPU).

    Subclasses mem0's EmbeddingBase at construction time (imported lazily so the
    class body doesn't pull mem0 at module import — /health must answer before
    the heavy mem0/onnxruntime import). EmbedderFactory.create passes a
    BaseEmbedderConfig instance as `config`.
    """

    def __init__(self, config=None):
        from fastembed import TextEmbedding
        self.config = config
        model = getattr(config, "model", None) or FASTEMBED_MODEL
        self._dims = getattr(config, "embedding_dims", None) or FASTEMBED_DIMS
        self._model = TextEmbedding(model)
        log.info("fastembed embedder ready (model=%s dims=%s)", model, self._dims)

    def embed(self, text, memory_action=None):  # noqa: ARG002
        # mem0 0.1.55 calls embed(text) with a single string; accept an optional
        # memory_action kwarg for forward-compat with newer mem0.
        vector = next(iter(self._model.embed([text])))
        return vector.tolist()


def _register_fastembed_provider():
    """Wire a CPU-only fastembed embedder into mem0's EmbedderFactory.

    Why this exists: mem0ai 0.1.55 ships NO built-in "fastembed" embedder
    provider (its EmbedderFactory only knows openai/ollama/huggingface/
    azure_openai/gemini/vertexai/together). Asking for provider="fastembed"
    therefore fails with HTTP 400 "Unsupported embedding provider: fastembed".
    The only other CPU option, "huggingface", needs `sentence-transformers`
    which is NOT in the image.

    The `fastembed` package (+ the BAAI/bge-small-en-v1.5 model) IS already
    installed and pre-downloaded in this image, so instead of an image rebuild
    we register a tiny adapter class under the provider name "fastembed". This
    keeps the CPU-only / no-GPU / no-new-key intent and needs only a server.py
    change (no new dependency). If a future mem0 bump adds a native fastembed
    provider, drop this shim and use it directly.
    """
    from mem0.utils.factory import EmbedderFactory

    # Register the DOTTED PATH (not the class object). load_class() does
    # `module_path, class_name = class_type.rsplit(".", 1)`, so a class object
    # would crash with "type object ... has no attribute 'rsplit'". __name__ is
    # "__main__" when run as `python server.py` (importlib can import __main__),
    # or "server" when imported — both resolve _FastEmbedEmbedder above.
    EmbedderFactory.provider_to_class["fastembed"] = f"{__name__}._FastEmbedEmbedder"
    log.info("registered custom 'fastembed' embedder provider into mem0 (%s)",
             EmbedderFactory.provider_to_class["fastembed"])

    # CRITICAL: registering in the factory is NOT enough. mem0 0.1.55 validates
    # the embedder provider name against a HARDCODED allowlist in a pydantic v2
    # field_validator (mem0/embeddings/configs.py::EmbedderConfig.validate_config)
    # BEFORE the factory is ever consulted. With "fastembed" absent from that
    # list, Memory.from_config raises:
    #   "1 validation error for MemoryConfig ... Unsupported embedding provider:
    #    fastembed"
    # and /add + /search return HTTP 400. So we ALSO relax that validator to
    # accept "fastembed" (every other provider still validates exactly as
    # upstream does — a bogus provider is still rejected).
    #
    # Pydantic v2 compiles the validator into the model's core schema at class
    # definition, so you cannot just reassign the attribute or model_rebuild():
    # the nested MemoryConfig still carries the old compiled validator. The
    # reliable approach is to (a) build a sibling model that declares the same
    # field with a permissive validator so pydantic produces a correctly-bound
    # decorator descriptor, (b) splice that descriptor's `.func` into the real
    # EmbedderConfig's decorator registry, then (c) force-rebuild BOTH
    # EmbedderConfig and MemoryConfig so the new validator is recompiled into
    # the schema mem0 actually uses.
    from typing import Optional as _Optional

    from pydantic import BaseModel as _BaseModel
    from pydantic import Field as _Field
    from pydantic import field_validator as _field_validator

    from mem0.configs import base as _mem0_base
    from mem0.embeddings import configs as _emb_configs

    _ALLOWED = [
        "openai", "ollama", "huggingface", "azure_openai",
        "gemini", "vertexai", "together", "fastembed",
    ]

    class _PatchedEmbedderConfig(_BaseModel):
        provider: str = _Field(default="openai")
        config: _Optional[dict] = _Field(default={})

        @_field_validator("config")
        def validate_config(cls, v, values):  # noqa: N805
            provider = values.data.get("provider")
            if provider in _ALLOWED:
                return v
            raise ValueError(f"Unsupported embedding provider: {provider}")

    _good = _PatchedEmbedderConfig.__pydantic_decorators__.field_validators[
        "validate_config"
    ]
    _emb_configs.EmbedderConfig.__pydantic_decorators__.field_validators[
        "validate_config"
    ].func = _good.func
    _emb_configs.EmbedderConfig.model_rebuild(force=True)
    _mem0_base.MemoryConfig.model_rebuild(force=True)
    log.info("patched EmbedderConfig validator to allow 'fastembed' provider")


# Fact-extraction prompt tuned for the JARVIS homelab/world scope. mem0 0.1.55's
# default FACT_RETRIEVAL_PROMPT is a "Personal Information Organizer" that only
# extracts facts/preferences ABOUT A USER (first-person), so it DISCARDS the
# third-person infrastructure/project facts we seed and sync (the initiative
# seeds and the worldsync_* mappers all came back with {"facts": []}). This
# prompt instead extracts durable facts about the homelab, its deployments,
# databases, and projects — while still returning [] for greetings/chatter.
_HOMELAB_FACT_PROMPT = """You are JARVIS's memory extractor for a homelab and its \
software/science projects. Extract durable, self-contained FACTS from the input \
and return them as JSON. Facts can be about the homelab cluster, its \
deployments/services, databases, metrics, and the owner's projects/initiatives \
(their name, goal, current status, and key components), as well as the owner's \
own preferences, people, and decisions. Keep each fact a single standalone \
statement; preserve concrete names, namespaces, services, and status.

Return ONLY JSON of the form {{"facts": ["...", "..."]}}. If there is nothing \
durable to remember (a greeting, small talk, an empty/trivial message), return \
{{"facts": []}}.

Here are some examples:

Input: Hi.
Output: {{"facts": []}}

Input: The mem0 rollout is the JARVIS unified-memory project. It uses a Qdrant \
vector store and CPU-only fastembed embeddings, and is currently broken on a \
stale image.
Output: {{"facts": ["The mem0 rollout is the JARVIS unified-memory project", \
"mem0 uses a Qdrant vector store and CPU-only fastembed embeddings", "The mem0 \
rollout is currently broken on a stale image"]}}

Input: khemeia is a Kubernetes computational-chemistry platform in the chem \
namespace; the docking prototype works but most work packages are not started.
Output: {{"facts": ["khemeia is a Kubernetes computational-chemistry platform \
in the chem namespace", "khemeia's docking prototype works", "Most khemeia work \
packages are not started"]}}

Input: My name is Hampton and I prefer the brain to run on Sonnet.
Output: {{"facts": ["Name is Hampton", "Prefers the brain to run on Sonnet"]}}

Today's date is {today}. Detect the input language and record facts in the same \
language. Return only the JSON object, nothing else."""


def _install_homelab_fact_prompt():
    """Swap mem0's personal-only extraction prompt for the homelab-aware one.

    mem0.memory.utils.get_fact_retrieval_messages returns
    (FACT_RETRIEVAL_PROMPT, "Input:\\n<msg>"), and mem0.memory.main imports that
    function by value at import time. We replace the function in BOTH modules so
    every add() uses our prompt regardless of which reference mem0 calls.
    """
    from datetime import datetime

    from mem0.memory import main as _mem_main
    from mem0.memory import utils as _mem_utils

    today = datetime.now().strftime("%Y-%m-%d")
    system_prompt = _HOMELAB_FACT_PROMPT.format(today=today)

    def _homelab_fact_messages(message):
        return system_prompt, f"Input:\n{message}"

    _mem_utils.get_fact_retrieval_messages = _homelab_fact_messages
    _mem_main.get_fact_retrieval_messages = _homelab_fact_messages
    log.info("installed homelab/world fact-extraction prompt (replaces mem0's "
             "personal-only default)")


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
        # mem0 0.1.55 has no native fastembed provider — add ours before build.
        _register_fastembed_provider()
        # Replace mem0's personal-only extraction prompt with the homelab one so
        # third-person infra/project facts actually get extracted + stored.
        _install_homelab_fact_prompt()
        from mem0 import Memory  # imported here so /health can answer pre-init
        _memory = Memory.from_config(_MEM0_CONFIG)
        _raise_extraction_token_budget(_memory)
        log.info("mem0 Memory initialized (embed=%s qdrant=%s:%s coll=%s extract=%s)",
                 FASTEMBED_MODEL, QDRANT_HOST, QDRANT_PORT, QDRANT_COLLECTION,
                 EXTRACTION_MODEL)
    return _memory


# Extraction/decision token budget. grok-3-mini is a REASONING model: its
# reasoning tokens are billed against max_tokens, leaving little for the actual
# JSON output. mem0's add() calls llm.generate_response() WITHOUT a max_tokens
# arg, so it falls back to the OpenAILLM default of 100 — far too small once
# reasoning (~400 tokens) is subtracted. The visible symptom is a truncated JSON
# response and "Error in new_retrieved_facts: Unterminated string", after which
# mem0 swallows the error and stores NOTHING. We bump that effective default.
_EXTRACTION_MAX_TOKENS = int(os.environ.get("EXTRACTION_MAX_TOKENS", "8192"))


def _raise_extraction_token_budget(memory) -> None:
    """Wrap the LLM's generate_response so the extraction + memory-decision
    calls (which omit max_tokens, defaulting to 100) get a budget large enough
    for grok-3-mini's reasoning + the full JSON output."""
    llm = memory.llm
    orig = llm.generate_response

    def _patched(messages, response_format=None, tools=None,
                 tool_choice="auto", max_tokens=None):
        if not max_tokens or max_tokens <= 100:
            max_tokens = _EXTRACTION_MAX_TOKENS
        return orig(messages=messages, response_format=response_format,
                    tools=tools, tool_choice=tool_choice, max_tokens=max_tokens)

    llm.generate_response = _patched
    log.info("raised extraction/decision max_tokens default to %d "
             "(grok-3-mini reasoning budget)", _EXTRACTION_MAX_TOKENS)


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
