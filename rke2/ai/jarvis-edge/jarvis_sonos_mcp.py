"""Sonos control MCP for JARVIS — volume, mute, pause, query.

Defaults the target speaker to SONOS_IP env (the same Play:1 we play
JARVIS's voice through). Tools accept an optional `room` arg for
addressing other speakers by name (e.g. "Living Room") — discovers
via SSDP and matches case-insensitively.

soco is already installed in the base image (the edge daemon uses it
for playback).
"""
from __future__ import annotations

import json
import os
import sys
import traceback


_DEFAULT_IP = os.environ.get("SONOS_IP", "")
# Discovery cache (60s TTL) so repeat lookups don't ssdp-spam.
_disc_cache: dict = {"ts": 0.0, "devices": []}

# Persona state — edge.py reads sonos_volume from here on every speak
# turn. If we only set the LIVE Sonos value, the next speak turn would
# clobber it back to the persona/schedule value. Writing here makes the
# change stick. When the brain is asked to "go back to schedule", we
# remove the key so edge.py's _scheduled_sonos_volume() takes over.
_PERSONA_PATH = os.environ.get("PERSONA_PATH", "/state/persona.json")


def _persona_set_volume(level: int | None) -> None:
    """Write (or clear if level is None) the sonos_volume override in
    persona.json. Best-effort — failure here doesn't fail the tool call,
    just falls back to the live-only behaviour."""
    try:
        try:
            with open(_PERSONA_PATH) as f:
                d = json.load(f)
        except (OSError, ValueError):
            d = {}
        if level is None:
            d.pop("sonos_volume", None)
        else:
            d["sonos_volume"] = int(level)
        tmp = _PERSONA_PATH + ".tmp"
        with open(tmp, "w") as f:
            json.dump(d, f, indent=2)
        os.replace(tmp, _PERSONA_PATH)
    except Exception as exc:  # noqa: BLE001
        print(f"sonos mcp: persona write failed: {exc!r}",
              file=sys.stderr)


def _soco_for(room: str | None):
    """Return a SoCo device for ``room`` (case-insensitive name) or the
    default. Cached briefly."""
    import time as _time
    import soco
    if not room:
        if not _DEFAULT_IP:
            raise RuntimeError("no SONOS_IP env set; pass room=<name>")
        return soco.SoCo(_DEFAULT_IP)
    now = _time.time()
    if now - _disc_cache["ts"] > 60.0:
        _disc_cache["devices"] = list(soco.discover(timeout=4) or [])
        _disc_cache["ts"] = now
    rl = room.lower().strip()
    for d in _disc_cache["devices"]:
        if rl in (d.player_name or "").lower():
            return d
    raise RuntimeError(f"no Sonos speaker matching {room!r}")


_TOOLS = [
    {
        "name": "sonos_volume_set",
        "description": (
            "Set the Sonos volume to an absolute level (0-100). Default "
            "target is the Bedroom Play:1 (the speaker JARVIS plays "
            "through). Pass `room` for another speaker by name."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "level": {"type": "integer", "minimum": 0, "maximum": 100},
                "room": {"type": "string"},
            },
            "required": ["level"],
            "additionalProperties": False,
        },
    },
    {
        "name": "sonos_volume_step",
        "description": (
            "Change Sonos volume by `delta` (positive = louder, negative "
            "= quieter, typically ±5). For 'a little louder' use 5; for "
            "'much louder' use 15."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "delta": {"type": "integer", "minimum": -50, "maximum": 50},
                "room": {"type": "string"},
            },
            "required": ["delta"],
            "additionalProperties": False,
        },
    },
    {
        "name": "sonos_volume_schedule",
        "description": (
            "Clear any manual volume override and let JARVIS's day/night "
            "schedule drive the Sonos volume again. Use when the user "
            "says 'go back to auto', 'reset the volume', 'use the "
            "schedule', etc."
        ),
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
    {
        "name": "sonos_mute",
        "description": "Mute / unmute. state in {on,off,toggle}, default toggle.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "state": {"type": "string", "enum": ["on", "off", "toggle"]},
                "room": {"type": "string"},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "sonos_pause",
        "description": "Pause whatever is playing on the speaker.",
        "inputSchema": {
            "type": "object",
            "properties": {"room": {"type": "string"}},
            "additionalProperties": False,
        },
    },
    {
        "name": "sonos_play",
        "description": "Resume playback on the speaker.",
        "inputSchema": {
            "type": "object",
            "properties": {"room": {"type": "string"}},
            "additionalProperties": False,
        },
    },
    {
        "name": "sonos_now_playing",
        "description": (
            "What's currently playing on the speaker: track title, artist, "
            "album, transport state. Returns 'not playing' if idle."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {"room": {"type": "string"}},
            "additionalProperties": False,
        },
    },
    {
        "name": "sonos_list_speakers",
        "description": "List all Sonos speakers on the LAN with model + IP.",
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
    {
        "name": "sonos_play_spotify",
        "description": (
            "Play a Spotify track DIRECTLY on the Sonos via Sonos's "
            "native Spotify integration (NOT via Spotify Connect). "
            "Prefer this over spotify_search_and_play when the user "
            "wants playback on a Sonos speaker — works even when the "
            "Sonos isn't currently registered as a Spotify Connect "
            "device. Pass natural-language `query` like "
            "'toot it and boot it by YG'. Default room is the Bedroom "
            "Play:1; pass `room` for another speaker. Requires Spotify "
            "linked on the Sonos (Sonos app → Settings → Services & "
            "Voice → Music & Content → Spotify)."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {"type": "string"},
                "room": {"type": "string"},
            },
            "required": ["query"],
            "additionalProperties": False,
        },
    },
]


def _text(t: str) -> dict:
    return {"content": [{"type": "text", "text": t}]}


def _call(name: str, args: dict) -> dict:
    room = args.get("room")
    try:
        if name == "sonos_volume_set":
            s = _soco_for(room)
            try: s.unjoin()
            except Exception: pass
            level = max(0, min(100, int(args["level"])))
            s.volume = level
            # Persist as persona override so the next speak turn doesn't
            # immediately clobber this back to the schedule value.
            _persona_set_volume(level)
            return _text(json.dumps({"status": "ok", "room": s.player_name, "volume": level}))
        if name == "sonos_volume_step":
            s = _soco_for(room)
            cur = int(s.volume)
            target = max(0, min(100, cur + int(args["delta"])))
            s.volume = target
            _persona_set_volume(target)
            return _text(json.dumps({"status": "ok", "room": s.player_name,
                                      "from": cur, "to": target}))
        if name == "sonos_volume_schedule":
            # Clear the persona override so edge.py's day/night
            # _scheduled_sonos_volume() takes back over.
            _persona_set_volume(None)
            return _text(json.dumps({"status": "ok",
                                      "message": "volume now follows day/night schedule"}))
        if name == "sonos_play_spotify":
            # Native Sonos-Spotify integration. Bypasses Spotify Connect
            # entirely — Sonos handles the playback via its own stored
            # Spotify account. Requires Spotify linked on the Sonos
            # (Sonos app → Settings → Services & Voice → Spotify).
            from soco.music_services import MusicService
            s = _soco_for(room)
            try: s.unjoin()
            except Exception: pass
            query = (args.get("query") or "").strip()
            if not query:
                return _text(json.dumps({"status": "error",
                                          "detail": "query required"}))
            try:
                spotify = MusicService("Spotify")
                results = spotify.search(category="tracks", term=query, count=5)
            except Exception as exc:  # noqa: BLE001
                msg = str(exc)
                if "AuthToken" in msg or "Authorization" in msg:
                    return _text(json.dumps({
                        "status": "error",
                        "detail": ("Sonos Spotify auth expired. Re-link "
                                   "Spotify in the Sonos app: "
                                   "Settings → Services & Voice → "
                                   "Music & Content → Spotify."),
                    }))
                return _text(json.dumps({"status": "error",
                                          "detail": f"sonos search failed: {msg[:200]}"}))
            if not results:
                return _text(json.dumps({"status": "error",
                                          "detail": f"no Sonos-Spotify results for {query!r}"}))
            track = results[0]
            try:
                s.clear_queue()
            except Exception: pass
            try:
                s.add_to_queue(track)
                s.play_from_queue(0)
            except Exception as exc:  # noqa: BLE001
                return _text(json.dumps({"status": "error",
                                          "detail": f"sonos play failed: {str(exc)[:200]}"}))
            title = getattr(track, "title", str(track))
            return _text(json.dumps({"status": "ok", "room": s.player_name,
                                      "playing": title, "via": "sonos-native"}))
        if name == "sonos_mute":
            s = _soco_for(room)
            state = (args.get("state") or "toggle").lower()
            if state == "on":   s.mute = True
            elif state == "off": s.mute = False
            else:                s.mute = not s.mute
            return _text(json.dumps({"status": "ok", "room": s.player_name, "muted": bool(s.mute)}))
        if name == "sonos_pause":
            s = _soco_for(room); s.pause()
            return _text(json.dumps({"status": "ok", "room": s.player_name}))
        if name == "sonos_play":
            s = _soco_for(room); s.play()
            return _text(json.dumps({"status": "ok", "room": s.player_name}))
        if name == "sonos_now_playing":
            s = _soco_for(room)
            info = s.get_current_track_info() or {}
            transport = (s.get_current_transport_info() or {}).get("current_transport_state", "?")
            return _text(json.dumps({
                "status": "ok",
                "room": s.player_name,
                "state": transport,
                "title": info.get("title") or "",
                "artist": info.get("artist") or "",
                "album": info.get("album") or "",
            }))
        if name == "sonos_list_speakers":
            import time as _time, soco
            _disc_cache["devices"] = list(soco.discover(timeout=4) or [])
            _disc_cache["ts"] = _time.time()
            return _text(json.dumps({
                "status": "ok",
                "speakers": [{"name": d.player_name,
                              "model": (d.speaker_info or {}).get("model_name"),
                              "ip": d.ip_address}
                             for d in _disc_cache["devices"]],
            }))
    except Exception as exc:  # noqa: BLE001
        return _text(json.dumps({"status": "error", "message": str(exc)[:200]}))
    return _text(json.dumps({"status": "error", "message": f"unknown tool: {name}"}))


def _handle(req: dict) -> dict | None:
    method = req.get("method")
    rid = req.get("id")
    if method == "initialize":
        return {"jsonrpc": "2.0", "id": rid,
                "result": {"protocolVersion": "2025-11-25",
                           "capabilities": {"tools": {"listChanged": False}},
                           "serverInfo": {"name": "jarvis_sonos", "version": "0.1.0"}}}
    if method == "notifications/initialized":
        return None
    if method == "tools/list":
        return {"jsonrpc": "2.0", "id": rid, "result": {"tools": _TOOLS}}
    if method == "tools/call":
        p = req.get("params") or {}
        try:
            return {"jsonrpc": "2.0", "id": rid,
                    "result": _call(p.get("name", ""), p.get("arguments") or {})}
        except Exception as exc:  # noqa: BLE001
            return {"jsonrpc": "2.0", "id": rid,
                    "error": {"code": -32603, "message": f"{type(exc).__name__}: {exc}",
                              "data": traceback.format_exc()}}
    if rid is None: return None
    return {"jsonrpc": "2.0", "id": rid,
            "error": {"code": -32601, "message": f"method not found: {method}"}}


def main() -> None:
    for line in sys.stdin:
        line = line.strip()
        if not line: continue
        try:
            req = json.loads(line)
        except json.JSONDecodeError:
            continue
        resp = _handle(req)
        if resp is not None:
            sys.stdout.write(json.dumps(resp) + "\n")
            sys.stdout.flush()


if __name__ == "__main__":
    main()
