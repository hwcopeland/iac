"""Stdio MCP server exposing JARVIS's Spotify listening data.

Companion to ``jarvis_personal_mcp.py``. Surfaces top artists / top
tracks / recently played / current track as MCP tools that Claude Code
mounts as **deferred** (zero per-prompt cost until used).

Requires a one-time auth: ``python3 jarvis_spotify.py auth``.
"""
from __future__ import annotations

import json
import os
import sys
import traceback

_HERE = os.path.dirname(os.path.abspath(__file__))
if _HERE not in sys.path:
    sys.path.insert(0, _HERE)

import jarvis_spotify as sp  # noqa: E402


_TOOLS = [
    {
        "name": "top_artists",
        "description": (
            "User's top Spotify artists. `time_range`: 'short' (last ~4 "
            "weeks), 'medium' (last ~6 months, default), 'long' (years). "
            "Returns JSON with names + up to 3 genres + popularity."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "time_range": {"type": "string",
                                "enum": ["short", "medium", "long"]},
                "limit": {"type": "integer", "minimum": 1, "maximum": 50},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "top_tracks",
        "description": (
            "User's top Spotify tracks for the given window "
            "(short/medium/long). JSON with track name + artists."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "time_range": {"type": "string",
                                "enum": ["short", "medium", "long"]},
                "limit": {"type": "integer", "minimum": 1, "maximum": 50},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "recently_played",
        "description": (
            "User's most recently played Spotify tracks (up to 50). "
            "JSON: name + artists + ISO `played_at`."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "limit": {"type": "integer", "minimum": 1, "maximum": 50},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "current_track",
        "description": (
            "What the user is listening to right now on Spotify. "
            "Returns `{playing: false}` if nothing is playing; otherwise "
            "name + artists + album + progress_ms."
        ),
        "inputSchema": {"type": "object", "properties": {},
                         "additionalProperties": False},
    },
]


def _text_result(text: str) -> dict:
    return {"content": [{"type": "text", "text": text}]}


def _call(name: str, args: dict) -> dict:
    if name == "top_artists":
        return _text_result(json.dumps(sp.top_artists(
            args.get("time_range", "medium"),
            int(args.get("limit", 10)),
        )))
    if name == "top_tracks":
        return _text_result(json.dumps(sp.top_tracks(
            args.get("time_range", "medium"),
            int(args.get("limit", 10)),
        )))
    if name == "recently_played":
        return _text_result(json.dumps(sp.recently_played(
            int(args.get("limit", 10)),
        )))
    if name == "current_track":
        return _text_result(json.dumps(sp.current_track()))
    return {"content": [{"type": "text", "text": f"unknown tool: {name}"}],
            "isError": True}


def _handle(req: dict) -> dict | None:
    method = req.get("method")
    rid = req.get("id")
    if method == "initialize":
        return {"jsonrpc": "2.0", "id": rid, "result": {
            "protocolVersion": "2025-11-25",
            "capabilities": {"tools": {"listChanged": False}},
            "serverInfo": {"name": "jarvis_spotify", "version": "0.1.0"},
        }}
    if method == "notifications/initialized":
        return None
    if method == "tools/list":
        return {"jsonrpc": "2.0", "id": rid, "result": {"tools": _TOOLS}}
    if method == "tools/call":
        params = req.get("params") or {}
        try:
            return {"jsonrpc": "2.0", "id": rid,
                    "result": _call(params.get("name", ""),
                                     params.get("arguments") or {})}
        except Exception as exc:  # noqa: BLE001
            return {"jsonrpc": "2.0", "id": rid,
                    "error": {"code": -32603,
                              "message": f"{type(exc).__name__}: {exc}",
                              "data": traceback.format_exc()}}
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
