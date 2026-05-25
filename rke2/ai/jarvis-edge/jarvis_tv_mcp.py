"""Minimal stdio MCP server exposing JARVIS's TV control layer.

Wraps :mod:`jarvis_tv` (Vizio SmartCast control + Chromecast casting)
as MCP tools so the Claude Code brain can pull them in via
``--mcp-config`` and surface them as **deferred** tools through
ToolSearch — zero per-prompt context cost until used.

Run directly (or via the daemon's mcp config pointing here):

    ~/openjarvis/.venv/bin/python jarvis_tv_mcp.py

Protocol: JSON-RPC 2.0 over line-delimited stdin/stdout.
Implements: initialize / tools/list / tools/call.

Local/uncommitted like the rest of JARVIS.
"""
from __future__ import annotations

import json
import os
import sys
import traceback

_HERE = os.path.dirname(os.path.abspath(__file__))
if _HERE not in sys.path:
    sys.path.insert(0, _HERE)

import jarvis_tv as tv  # noqa: E402


_TOOLS = [
    {
        "name": "tv_status",
        "description": (
            "One-shot snapshot of the TV: power state, current input, and "
            "current app (if any). Returns JSON: {status, power, input, app}. "
            "Prefer this over individual getters for a quick check."
        ),
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
    {
        "name": "tv_power",
        "description": (
            "Turn the TV on, off, or toggle. State must be one of "
            "'on' / 'off' / 'toggle'. Returns {status, message}."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {"state": {"type": "string", "enum": ["on", "off", "toggle"]}},
            "required": ["state"],
            "additionalProperties": False,
        },
    },
    {
        "name": "tv_volume_set",
        "description": (
            "Set the TV volume to an absolute level (0-100). Returns "
            "{status, volume}. Use tv_volume_step for relative changes."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {"level": {"type": "integer", "minimum": 0, "maximum": 100}},
            "required": ["level"],
            "additionalProperties": False,
        },
    },
    {
        "name": "tv_volume_step",
        "description": (
            "Step the TV volume by `delta` (positive = up, negative = down, "
            "typically ±5)."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {"delta": {"type": "integer", "minimum": -50, "maximum": 50}},
            "required": ["delta"],
            "additionalProperties": False,
        },
    },
    {
        "name": "tv_mute",
        "description": (
            "Mute / unmute the TV. State in {'on','off','toggle'}; default "
            "'toggle'."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {"state": {"type": "string", "enum": ["on", "off", "toggle"]}},
            "additionalProperties": False,
        },
    },
    {
        "name": "tv_inputs",
        "description": (
            "List available TV inputs by name (CAST, HDMI-1, HDMI-2, HDMI-3, "
            "COMP, TV)."
        ),
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
    {
        "name": "tv_input_set",
        "description": (
            "Switch the TV to a named input (e.g. 'HDMI-1', 'CAST'). Get the "
            "list with tv_inputs first if unsure."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {"name": {"type": "string"}},
            "required": ["name"],
            "additionalProperties": False,
        },
    },
    {
        "name": "tv_launch_app",
        "description": (
            "Launch a SmartCast app by exact name (e.g. 'Netflix', 'YouTube', "
            "'Hulu', 'Prime Video'). Use SmartCast Home to return to the "
            "main screen."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {"name": {"type": "string"}},
            "required": ["name"],
            "additionalProperties": False,
        },
    },
    {
        "name": "tv_cast_url",
        "description": (
            "Cast a direct media URL (mp4/jpg/mp3/hls m3u8) to the TV's "
            "built-in Chromecast. Does NOT work for YouTube/Spotify links — "
            "those need their own SmartCast apps (use tv_launch_app)."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "url": {"type": "string"},
                "content_type": {"type": "string"},
                "title": {"type": "string"},
            },
            "required": ["url"],
            "additionalProperties": False,
        },
    },
    {
        "name": "tv_cast_stop",
        "description": "Stop whatever the Chromecast is currently playing.",
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
    {
        "name": "tv_youtube_play",
        "description": (
            "Cast a YouTube video to the TV's built-in YouTube app. Accepts "
            "a full URL (youtube.com/watch?v=…, youtu.be/…) or a raw 11-char "
            "video ID. For 'play that <topic> video' style requests, web-search "
            "for the URL first, then call this with the URL."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {"url_or_id": {"type": "string"}},
            "required": ["url_or_id"],
            "additionalProperties": False,
        },
    },
    {
        "name": "tv_youtube_queue",
        "description": "Append a YouTube video to the current TV queue (same URL/ID format as tv_youtube_play).",
        "inputSchema": {
            "type": "object",
            "properties": {"url_or_id": {"type": "string"}},
            "required": ["url_or_id"],
            "additionalProperties": False,
        },
    },
    {
        "name": "tv_spotify_play",
        "description": (
            "Play a Spotify track / playlist / album on the TV via Spotify "
            "Connect (requires Premium + the Spotify app to be signed in on "
            "the TV — this is a one-time setup the user does manually). "
            "Provide either `uri` (spotify:track:…/playlist:…/album:…) or "
            "`query` (free-text search; the top track hit is used)."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "uri": {"type": "string"},
                "query": {"type": "string"},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "tv_spotify_pause",
        "description": "Pause Spotify playback on whatever device is active.",
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
    {
        "name": "tv_plex_search",
        "description": (
            "Search the user's Plex library across all sections. Returns up "
            "to `limit` results with title/type/year. Use before `tv_plex_play` "
            "if you need to disambiguate."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 20},
            },
            "required": ["query"],
            "additionalProperties": False,
        },
    },
    {
        "name": "tv_plex_play",
        "description": (
            "Search Plex for `query`, take the top movie/episode hit, and "
            "cast its direct-stream URL to the TV. For TV shows, picks the "
            "first unwatched episode (or S01E01 if all watched). Music libs "
            "are skipped — use Spotify for music."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {"query": {"type": "string"}},
            "required": ["query"],
            "additionalProperties": False,
        },
    },
    {
        "name": "tv_plex_libraries",
        "description": "List the Plex library sections (Movies, TV Shows, etc.) with counts.",
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
]


def _text_result(text: str) -> dict:
    return {"content": [{"type": "text", "text": text}]}


def _call(name: str, args: dict) -> dict:
    if name == "tv_status":
        return _text_result(json.dumps(tv.status()))
    if name == "tv_power":
        return _text_result(json.dumps(tv.power(args.get("state", ""))))
    if name == "tv_volume_set":
        return _text_result(json.dumps(tv.volume_set(int(args.get("level", 20)))))
    if name == "tv_volume_step":
        return _text_result(json.dumps(tv.volume_step(int(args.get("delta", 0)))))
    if name == "tv_mute":
        return _text_result(json.dumps(tv.mute(args.get("state", "toggle"))))
    if name == "tv_inputs":
        return _text_result(json.dumps(tv.inputs_list()))
    if name == "tv_input_set":
        return _text_result(json.dumps(tv.input_set(args.get("name", ""))))
    if name == "tv_launch_app":
        return _text_result(json.dumps(tv.launch_app(args.get("name", ""))))
    if name == "tv_cast_url":
        return _text_result(json.dumps(tv.cast_url(
            args.get("url", ""),
            args.get("content_type", "video/mp4"),
            args.get("title", ""),
        )))
    if name == "tv_cast_stop":
        return _text_result(json.dumps(tv.cast_stop()))
    if name == "tv_youtube_play":
        return _text_result(json.dumps(tv.youtube_play(args.get("url_or_id", ""))))
    if name == "tv_youtube_queue":
        return _text_result(json.dumps(tv.youtube_add_to_queue(args.get("url_or_id", ""))))
    if name == "tv_spotify_play":
        return _text_result(json.dumps(tv.spotify_play_on_tv(
            uri=args.get("uri", ""), query=args.get("query", ""),
        )))
    if name == "tv_spotify_pause":
        return _text_result(json.dumps(tv.spotify_pause()))
    if name == "tv_plex_search":
        return _text_result(json.dumps(tv.plex_search(
            args.get("query", ""), int(args.get("limit", 5)),
        )))
    if name == "tv_plex_play":
        return _text_result(json.dumps(tv.plex_play(args.get("query", ""))))
    if name == "tv_plex_libraries":
        return _text_result(json.dumps(tv.plex_libraries()))
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
                "serverInfo": {"name": "jarvis_tv", "version": "0.1.0"},
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
