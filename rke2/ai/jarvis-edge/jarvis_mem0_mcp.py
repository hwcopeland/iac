"""Stdio MCP server exposing JARVIS long-term memory (mem0) to the OWNER brain.

  *** DEPLOYMENT: this file is authored under rke2/ai/mem0/ but BELONGS in the
  jarvis-edge image. Copy it to rke2/ai/jarvis-edge/jarvis_mem0_mcp.py, add it
  to the jarvis-edge Dockerfile COPY block, and wire it into _write_mcp_config()
  + _RO_ALLOWED_TOOLS per docs/jarvis/phase6-mem0.md. It is NOT applied as a
  manifest — the claude CLI spawns it as a subprocess. ***

This is a THIN shim. All heavy lifting (extraction, embeddings, Qdrant) lives
in the mem0 REST service (rke2/ai/mem0/, Service jarvis-mem0.ai.svc:8800). The
shim just translates MCP tool calls into HTTP calls.

THE PARTITION INVARIANT — why this is owner-safe:
  The partition key (`user_id`) is taken from the JARVIS_MEM_SCOPE environment
  variable that edge.py's _claude_brain() already threads into the subprocess.
  The brain CANNOT override it — there is no `user_id` tool argument. For the
  owner that env is always "owner"; the locked (TRUSTED) brain has NO
  --mcp-config at all, so it never even loads this server. Result: owner
  memories are written to and read from the "owner" scope ONLY, and are
  structurally unreachable by any non-owner turn.

Protocol: JSON-RPC 2.0 over line-delimited stdin/stdout (MCP 2025-11-25).
Local/uncommitted like the rest of JARVIS.
"""
from __future__ import annotations

import json
import os
import sys
import traceback
import urllib.error
import urllib.request

MEM0_URL = os.environ.get(
    "MEM0_URL", "http://jarvis-mem0.ai.svc.cluster.local:8800"
).rstrip("/")

# The partition key. Threaded by edge.py:_claude_brain via JARVIS_MEM_SCOPE.
# If absent we FAIL CLOSED to a sentinel that maps to nothing useful rather
# than silently writing to a shared namespace.
_MEM_SCOPE = (os.environ.get("JARVIS_MEM_SCOPE") or "").strip()
# Shared world/homelab scope: cluster/database/metric/project facts the
# worldsync mappers + seeder write here (user_id "jarvis"), readable by every
# speaker's brain so homelab knowledge isn't trapped in one person's scope.
_WORLD_SCOPE = (os.environ.get("JARVIS_WORLD_SCOPE") or "jarvis").strip()

_HTTP_TIMEOUT = float(os.environ.get("MEM0_HTTP_TIMEOUT", "20"))


_TOOLS = [
    {
        "name": "memory_search",
        "description": (
            "Recall relevant long-term memories about the owner before "
            "answering. Use this when the owner refers to past conversations, "
            "preferences, people, projects, or anything you might have been "
            "told before ('what did I say about...', 'remember when...', "
            "'my usual...', 'who is...'). Returns the most relevant stored "
            "memories as JSON. Search is automatically scoped to the current "
            "speaker — you cannot and need not specify whose memory to search."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {"type": "string", "description": "what to recall"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 10},
            },
            "required": ["query"],
            "additionalProperties": False,
        },
    },
    {
        "name": "memory_add",
        "description": (
            "Persist a durable fact worth remembering for future "
            "conversations — a stated preference, a person, an ongoing "
            "project, a decision, a correction the owner made. Pass the raw "
            "statement; the memory engine extracts the salient fact itself. "
            "Do NOT store transient/one-off chatter (the weather, the time, "
            "small talk). Storage is automatically scoped to the current "
            "speaker."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "text": {
                    "type": "string",
                    "description": "the statement/fact to remember",
                },
            },
            "required": ["text"],
            "additionalProperties": False,
        },
    },
]


def _text_result(text: str, is_error: bool = False) -> dict:
    r: dict = {"content": [{"type": "text", "text": text}]}
    if is_error:
        r["isError"] = True
    return r


def _post(path: str, payload: dict) -> dict:
    data = json.dumps(payload).encode()
    req = urllib.request.Request(
        f"{MEM0_URL}{path}", data=data,
        headers={"Content-Type": "application/json"}, method="POST",
    )
    with urllib.request.urlopen(req, timeout=_HTTP_TIMEOUT) as r:
        return json.loads(r.read() or b"{}")


def _call(name: str, args: dict) -> dict:
    if not _MEM_SCOPE:
        # No scope = fail closed. Without it we'd risk writing to/reading from
        # an unscoped namespace, so refuse rather than guess.
        return _text_result(
            "Memory is unavailable for this turn (no memory scope).",
            is_error=True,
        )
    try:
        if name == "memory_search":
            query = (args.get("query") or "").strip()
            if not query:
                return _text_result("memory_search needs a query", is_error=True)
            limit = int(args.get("limit", 5))
            # Search the speaker's own scope AND the shared world/homelab scope
            # ("jarvis") where the worldsync mappers + initiative seeder write
            # cluster/database/metric/project facts. Without this the
            # owner-scoped brain never sees homelab knowledge ("what's the deal
            # with the mem0 rollout" returns nothing). Merge both.
            scopes = [_MEM_SCOPE]
            if _WORLD_SCOPE and _WORLD_SCOPE != _MEM_SCOPE:
                scopes.append(_WORLD_SCOPE)
            merged = []
            for sc in scopes:
                try:
                    out = _post("/search", {
                        "query": query, "user_id": sc, "limit": limit,
                    })
                    res = out.get("result", []) if isinstance(out, dict) else []
                    if isinstance(res, list):
                        merged.extend(res)
                except urllib.error.URLError:
                    pass
            return _text_result(json.dumps(merged))
        if name == "memory_add":
            text = (args.get("text") or "").strip()
            if not text:
                return _text_result("memory_add needs text", is_error=True)
            out = _post("/add", {"text": text, "user_id": _MEM_SCOPE})
            return _text_result(json.dumps(out.get("result", out)))
        return _text_result(f"unknown tool: {name}", is_error=True)
    except urllib.error.URLError as exc:
        return _text_result(f"memory service unreachable: {exc}", is_error=True)


def _handle(req: dict) -> dict | None:
    method = req.get("method")
    rid = req.get("id")
    if method == "initialize":
        return {
            "jsonrpc": "2.0", "id": rid,
            "result": {
                "protocolVersion": "2025-11-25",
                "capabilities": {"tools": {"listChanged": False}},
                "serverInfo": {"name": "jarvis_mem0", "version": "0.1.0"},
            },
        }
    if method == "notifications/initialized":
        return None
    if method == "tools/list":
        return {"jsonrpc": "2.0", "id": rid, "result": {"tools": _TOOLS}}
    if method == "tools/call":
        params = req.get("params") or {}
        try:
            result = _call(params.get("name", ""), params.get("arguments") or {})
            return {"jsonrpc": "2.0", "id": rid, "result": result}
        except Exception as exc:  # noqa: BLE001
            return {
                "jsonrpc": "2.0", "id": rid,
                "error": {"code": -32603, "message": f"{type(exc).__name__}: {exc}",
                          "data": traceback.format_exc()},
            }
    if rid is None:
        return None
    return {"jsonrpc": "2.0", "id": rid,
            "error": {"code": -32601, "message": f"method not found: {method}"}}


def main() -> None:
    out = sys.stdout
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except json.JSONDecodeError:
            continue
        resp = _handle(req)
        if resp is not None:
            out.write(json.dumps(resp) + "\n")
            out.flush()


if __name__ == "__main__":
    main()
