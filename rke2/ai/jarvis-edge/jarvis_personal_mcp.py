"""Minimal stdio MCP server exposing JARVIS's personal-assistant layer.

Wraps :mod:`jarvis_personal` (macOS Calendar + Reminders + cluster alerts
+ daily briefing) as MCP tools so the Claude Code brain can pull them in
via ``--mcp-config`` and surface them as **deferred** tools through
ToolSearch — zero per-prompt context cost until used.

Run directly; the daemon's MCP config points Claude Code at this file:

    python jarvis_personal_mcp.py

Protocol: JSON-RPC 2.0 over line-delimited stdin/stdout (MCP 2025-11-25).
Implements the minimum needed: initialize / tools/list / tools/call.

Local/uncommitted like the rest of JARVIS.
"""
from __future__ import annotations

import json
import os
import sys
import traceback

# Repo root contains jarvis_personal.py; this file lives next to it.
_HERE = os.path.dirname(os.path.abspath(__file__))
if _HERE not in sys.path:
    sys.path.insert(0, _HERE)

import jarvis_personal as jp  # noqa: E402


_TOOLS = [
    {
        "name": "calendar_today",
        "description": (
            "Return today's macOS Calendar events for the signed-in user. "
            "Output is JSON: {status, events:[{title, when}]}. "
            "status='unauthorized' means Calendar TCC isn't granted to "
            "the daemon's terminal."
        ),
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
    {
        "name": "reminders_open",
        "description": (
            "Return the user's open (incomplete) Apple Reminders. "
            "Up to `limit` items, most recent first. JSON array of strings."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {"limit": {"type": "integer", "minimum": 1, "maximum": 50}},
            "additionalProperties": False,
        },
    },
    {
        "name": "reminders_due_today",
        "description": (
            "Return Apple Reminders due today. JSON array of strings."
        ),
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
    {
        "name": "briefing",
        "description": (
            "Spoken-friendly daily briefing: calendar + reminders + overnight "
            "critical cluster alerts + weather + overnight world headlines. "
            "Returns the cached briefing if fresh (prefer this — instant); "
            "pass refresh=true to recompute live (slow, ~5-8s)."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {"refresh": {"type": "boolean"}},
            "additionalProperties": False,
        },
    },
    {
        "name": "weather",
        "description": (
            "Today's weather for the user's location (auto-geolocated by IP) "
            "or a specified `location`. JSON with status, location, summary, "
            "temp_f, feels_f, high_f, low_f, conditions, humidity."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {"location": {"type": "string"}},
            "additionalProperties": False,
        },
    },
    {
        "name": "greeting",
        "description": (
            "Time-aware spoken greeting: 'Good morning/afternoon/evening, "
            "sir', current local time, current temp + conditions + high, "
            "and a rain-transition narrative if rain starts or stops today "
            "('Expect rain around 3 PM' / 'Currently raining, halt around "
            "9 PM'). Use for 'good morning' / 'i'm home' / 'wake up' style "
            "questions, or whenever a brief situational opener is wanted. "
            "Composed live (one wttr.in fetch, ~0.5s) — no cache needed."
        ),
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
    {
        "name": "news_overnight",
        "description": (
            "World headlines published in the last `hours` hours (default "
            "12 = overnight). Answers 'what happened last night' / 'what "
            "did I miss while sleeping'. JSON array of cleaned headlines."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "hours": {"type": "integer", "minimum": 1, "maximum": 48},
                "limit": {"type": "integer", "minimum": 1, "maximum": 20},
            },
            "additionalProperties": False,
        },
    },
]


def _text_result(text: str) -> dict:
    return {"content": [{"type": "text", "text": text}]}


def _call(name: str, args: dict) -> dict:
    if name == "calendar_today":
        return _text_result(json.dumps(jp.calendar_today()))
    if name == "reminders_open":
        limit = int(args.get("limit", 12))
        return _text_result(json.dumps(jp.reminders_open(limit=limit)))
    if name == "reminders_due_today":
        return _text_result(json.dumps(jp.reminders_due_today()))
    if name == "briefing":
        if args.get("refresh"):
            return _text_result(jp.rebuild_and_cache())
        cached = jp.read_cache()
        if cached.get("text"):
            return _text_result(cached["text"])
        return _text_result(jp.rebuild_and_cache())
    if name == "greeting":
        return _text_result(jp.greeting())
    if name == "weather":
        return _text_result(json.dumps(jp.weather(args.get("location", ""))))
    if name == "news_overnight":
        return _text_result(json.dumps(jp.news_overnight(
            hours=int(args.get("hours", 12)),
            limit=int(args.get("limit", 5)),
        )))
    return {"content": [{"type": "text", "text": f"unknown tool: {name}"}], "isError": True}


def _handle(req: dict) -> dict | None:
    method = req.get("method")
    rid = req.get("id")
    if method == "initialize":
        return {
            "jsonrpc": "2.0", "id": rid,
            "result": {
                "protocolVersion": "2025-11-25",
                "capabilities": {"tools": {"listChanged": False}},
                "serverInfo": {"name": "jarvis_personal", "version": "0.1.0"},
            },
        }
    if method == "notifications/initialized":
        return None  # notification — no response
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
        return None  # unknown notification — ignore
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
