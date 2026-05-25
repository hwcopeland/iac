"""Persona-tuning MCP for JARVIS — lets the brain self-adjust its tone
(humor / formality / terseness / sass) and TTS / Sonos knobs in response
to voice commands like "Jarvis, less humor" or "turn up the cadence by 10%".

State is persisted to /state/persona.json on a PVC so adjustments survive
pod restarts. Writes are atomic (tmp + rename) so a crashed write can't
corrupt the file.

Numeric ranges:
  - humor, formality, terseness, sass, tts_exaggeration, tts_cfg: 0.0–1.0
  - sonos_volume: 0–100 (integer)
"""
from __future__ import annotations

import json
import os
import sys
import traceback


_STATE_PATH = os.environ.get("PERSONA_STATE_PATH", "/state/persona.json")

_DEFAULTS: dict = {
    "humor": 0.5,
    "formality": 0.7,
    "terseness": 0.9,
    "sass": 0.3,
    "tts_exaggeration": 0.7,
    "tts_cfg": 0.4,
    "sonos_volume": 30,
}

# Per-key (lo, hi, is_int) ranges. Keep in sync with _load_persona() in edge.py.
_RANGES: dict[str, tuple[float, float, bool]] = {
    "humor":            (0.0, 1.0, False),
    "formality":        (0.0, 1.0, False),
    "terseness":        (0.0, 1.0, False),
    "sass":             (0.0, 1.0, False),
    "tts_exaggeration": (0.0, 1.0, False),
    "tts_cfg":          (0.0, 1.0, False),
    "sonos_volume":     (0,   100, True),
}


def _clamp(key: str, value: float) -> float | int:
    lo, hi, is_int = _RANGES[key]
    v = max(lo, min(hi, value))
    return int(round(v)) if is_int else float(v)


def _read_state() -> dict:
    """Read state from disk; fall back to defaults on any error.

    Defaults are deep-copied so callers can mutate freely; missing keys
    are backfilled so a partial file from an older version still works.
    """
    try:
        with open(_STATE_PATH) as f:
            data = json.load(f)
        if not isinstance(data, dict):
            data = {}
    except (FileNotFoundError, json.JSONDecodeError, OSError):
        data = {}
    merged = dict(_DEFAULTS)
    for k, v in data.items():
        if k in _RANGES and isinstance(v, (int, float)):
            merged[k] = _clamp(k, v)
    return merged


def _write_state(state: dict) -> None:
    """Atomic write — tmp file then rename, so a crash mid-write can't
    leave a half-written persona.json."""
    os.makedirs(os.path.dirname(_STATE_PATH) or ".", exist_ok=True)
    tmp = f"{_STATE_PATH}.tmp"
    with open(tmp, "w") as f:
        json.dump(state, f, indent=2, sort_keys=True)
        f.flush()
        os.fsync(f.fileno())
    os.replace(tmp, _STATE_PATH)


_TOOLS = [
    {
        "name": "persona_get",
        "description": (
            "Return the current persona tuning state: humor, formality, "
            "terseness, sass (each 0.0-1.0), tts_exaggeration, tts_cfg "
            "(0.0-1.0), and sonos_volume (0-100). Use this when the user "
            "asks 'what's your humor at' / 'how are you tuned'."
        ),
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
    {
        "name": "persona_set",
        "description": (
            "Set ONE persona dimension to an absolute value. Values are "
            "clamped to the valid range (0.0-1.0 for most; 0-100 for "
            "sonos_volume). Use for commands like 'set humor to 0.8' or "
            "'sonos volume 45'."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "key":   {"type": "string", "enum": list(_RANGES.keys())},
                "value": {"type": "number"},
            },
            "required": ["key", "value"],
            "additionalProperties": False,
        },
    },
    {
        "name": "persona_adjust",
        "description": (
            "Adjust ONE persona dimension by a delta expressed as a "
            "PERCENT of the full range (e.g. delta_pct=10 raises a "
            "0.0-1.0 key by 0.1; raises sonos_volume by 10). Negative "
            "values lower. Use for 'more humor' (+10), 'less formality' "
            "(-15), 'turn up the cadence by 10 percent', etc. Final "
            "value is clamped."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "key":       {"type": "string", "enum": list(_RANGES.keys())},
                "delta_pct": {"type": "number", "minimum": -100, "maximum": 100},
            },
            "required": ["key", "delta_pct"],
            "additionalProperties": False,
        },
    },
    {
        "name": "persona_reset",
        "description": (
            "Restore ALL persona dimensions to their factory defaults. "
            "Use only when the user explicitly asks ('reset your persona' "
            "/ 'go back to defaults')."
        ),
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
]


def _text(t: str) -> dict:
    return {"content": [{"type": "text", "text": t}]}


def _call(name: str, args: dict) -> dict:
    try:
        if name == "persona_get":
            state = _read_state()
            return _text(json.dumps({"status": "ok", "state": state}))

        if name == "persona_set":
            key = args["key"]
            if key not in _RANGES:
                return _text(json.dumps({"status": "error",
                                          "message": f"unknown key: {key}"}))
            state = _read_state()
            old = state.get(key)
            new = _clamp(key, float(args["value"]))
            state[key] = new
            _write_state(state)
            return _text(json.dumps({"status": "ok", "key": key,
                                      "from": old, "to": new}))

        if name == "persona_adjust":
            key = args["key"]
            if key not in _RANGES:
                return _text(json.dumps({"status": "error",
                                          "message": f"unknown key: {key}"}))
            lo, hi, _ = _RANGES[key]
            span = hi - lo
            delta = (float(args["delta_pct"]) / 100.0) * span
            state = _read_state()
            old = state.get(key, _DEFAULTS[key])
            new = _clamp(key, float(old) + delta)
            state[key] = new
            _write_state(state)
            return _text(json.dumps({"status": "ok", "key": key,
                                      "delta_pct": args["delta_pct"],
                                      "from": old, "to": new}))

        if name == "persona_reset":
            state = dict(_DEFAULTS)
            _write_state(state)
            return _text(json.dumps({"status": "ok", "state": state,
                                      "message": "persona reset to defaults"}))

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
                           "serverInfo": {"name": "jarvis_persona", "version": "0.1.0"}}}
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
    if rid is None:
        return None
    return {"jsonrpc": "2.0", "id": rid,
            "error": {"code": -32601, "message": f"method not found: {method}"}}


def main() -> None:
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
            sys.stdout.write(json.dumps(resp) + "\n")
            sys.stdout.flush()


if __name__ == "__main__":
    main()
