"""jarvis_local_brain — uncensored local-model fallback for the Claude
subscription brain.

The Claude API politely refuses ~5% of edgy IG content. We want JARVIS
to NEVER refuse, so when Claude bails (returns refusal text or the
literal ABSTAIN sentinel), we re-run the same prompt against a local
Ollama-hosted abliterated model. Slower and less culturally fluent
than Claude on a good day, but won't fight us on dark-humor IG
comments.

Endpoint: env LOCAL_BRAIN_URL (default http://ollama.ai.svc.cluster.local:11434).
Model:    env LOCAL_BRAIN_MODEL (default huihui_ai/qwen3-abliterated:30b-a3b).

Public API:
    generate(prompt: str, system: str = "", max_tokens: int = 120,
             timeout: float = 60.0) -> str

Returns the model's text response, or "" on failure. NEVER raises.
"""
from __future__ import annotations

import json
import os
import urllib.request
import urllib.error


def _endpoint() -> str:
    # Default to localhost:NodePort because jarvis-edge runs hostNetwork
    # on the same node as ollama. Cluster DNS / ClusterIP routing can
    # be flaky from hostNetwork; NodePort (32732 → 11434) works because
    # kube-proxy/Cilium binds it on every node's interface including
    # localhost. Override with LOCAL_BRAIN_URL if needed.
    return os.environ.get("LOCAL_BRAIN_URL", "http://localhost:32732").rstrip("/")


def _model() -> str:
    # qwen2.5:7b — 4.7GB Q4, fits cleanly alongside whisper+chatterbox
    # on the shared 3070. The abliterated 30B-a3b MoE works too but is
    # heavy under VRAM contention. Override via LOCAL_BRAIN_MODEL.
    return os.environ.get("LOCAL_BRAIN_MODEL", "qwen2.5:7b")


def generate(prompt: str, system: str = "", max_tokens: int = 120,
             timeout: float = 60.0) -> str:
    """Run the local abliterated model. Uses Ollama's /api/chat endpoint
    with chat messages. Falls back to empty string on ANY error so
    callers can treat this as best-effort."""
    url = f"{_endpoint()}/api/chat"
    messages = []
    if system:
        messages.append({"role": "system", "content": system})
    messages.append({"role": "user", "content": prompt})
    payload = {
        "model": _model(),
        "messages": messages,
        "stream": False,
        "options": {
            "num_predict": max_tokens,
            "temperature": 0.8,
            "top_p": 0.9,
        },
    }
    body = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url, data=body, method="POST",
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read()
    except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError) as exc:
        print(f"local brain: {type(exc).__name__}: {exc}")
        return ""
    try:
        data = json.loads(raw)
    except (ValueError, json.JSONDecodeError) as exc:
        print(f"local brain: json parse failed: {exc!r}")
        return ""
    msg = (data.get("message") or {}).get("content") or ""
    # qwen3 family emits <think>...</think> reasoning blocks before the
    # actual answer. Strip them so the caller gets only the reply.
    if "</think>" in msg:
        msg = msg.split("</think>", 1)[1]
    return msg.strip()


def is_available() -> bool:
    """Cheap reachability probe. Used by callers to decide whether to
    even attempt fallback. NEVER raises."""
    url = f"{_endpoint()}/api/tags"
    try:
        with urllib.request.urlopen(url, timeout=3) as resp:
            return resp.status == 200
    except Exception:  # noqa: BLE001
        return False
