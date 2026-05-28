"""Spotify integration for JARVIS — OAuth 2.0 PKCE + listening-data API.

One-time setup:

    python3 jarvis_spotify.py auth

Opens the browser to Spotify's authorization page; on success the
callback at http://127.0.0.1:8888/callback receives the auth code,
JARVIS exchanges it for tokens, and persists them to
``~/.openjarvis/spotify_tokens.json`` (mode 0600). After that the MCP
server uses the stored tokens and refreshes them automatically on expiry.

PKCE flow — no client secret, safe for native/local apps. The
``client_id`` lives in ``~/.openjarvis/config.toml`` under ``[spotify]``.

Local/uncommitted, like the rest of JARVIS.
"""
from __future__ import annotations

import base64
import hashlib
import http.server
import json
import os
import secrets
import subprocess
import sys
import threading
import time
import urllib.parse
import webbrowser
from typing import Optional, Tuple

_TOKENS_PATH = os.path.expanduser("~/.openjarvis/spotify_tokens.json")
_CONFIG_PATH = os.path.expanduser("~/.openjarvis/config.toml")
_REDIRECT_URI = "http://127.0.0.1:8888/callback"
_REDIRECT_PORT = 8888
_SCOPES = " ".join([
    "user-read-currently-playing",
    "user-read-playback-state",
    "user-read-recently-played",
    "user-top-read",
    # Playback control — required for spotify_play_track etc.
    # If user has an old token without these, re-run `auth` to re-grant.
    "user-modify-playback-state",
])
_HTTP_TIMEOUT = 12
_UA = "JARVIS-spotify/0.1"


# ── Config / credential plumbing ─────────────────────────────────────────
def _client_id() -> str:
    try:
        import tomllib
        with open(_CONFIG_PATH, "rb") as f:
            cfg = tomllib.load(f)
        cid = (cfg.get("spotify") or {}).get("client_id", "")
        if cid:
            return cid
    except (OSError, ValueError, ModuleNotFoundError):
        pass
    return os.environ.get("SPOTIFY_CLIENT_ID", "")


# ── HTTP via curl (system trust store; python.org Python 3.12 SSL is unreliable)
def _curl(method: str, url: str, *, data: Optional[dict] = None,
          headers: Optional[dict] = None,
          json_body: Optional[dict] = None) -> Tuple[int, str]:
    """Run curl, return (http_code, body). 0 means transport failure.
    Pass `data` for form-encoded body (legacy auth flow).
    Pass `json_body` for JSON body (Spotify Web API playback endpoints).
    Caller is responsible for the corresponding Content-Type header
    when using json_body; if absent, we add it."""
    args = ["curl", "-sS", "--max-time", str(_HTTP_TIMEOUT),
            "-A", _UA, "-X", method, "-w", "\n%{http_code}", url]
    if headers:
        for k, v in headers.items():
            args += ["-H", f"{k}: {v}"]
    if data is not None:
        args += ["-H", "Content-Type: application/x-www-form-urlencoded",
                 "--data", urllib.parse.urlencode(data)]
    elif json_body is not None:
        ct_already = headers and any(k.lower() == "content-type"
                                      for k in headers)
        if not ct_already:
            args += ["-H", "Content-Type: application/json"]
        args += ["--data", json.dumps(json_body)]
    try:
        r = subprocess.run(args, capture_output=True, text=True,
                           timeout=_HTTP_TIMEOUT + 2)
        out = r.stdout
        if "\n" in out:
            body, code_str = out.rsplit("\n", 1)
            try:
                return int(code_str.strip()), body
            except ValueError:
                return 0, out
        return 0, out
    except (subprocess.SubprocessError, OSError):
        return 0, ""


# ── PKCE helpers ─────────────────────────────────────────────────────────
def _pkce_pair() -> Tuple[str, str]:
    verifier = secrets.token_urlsafe(64)[:128]
    digest = hashlib.sha256(verifier.encode("ascii")).digest()
    challenge = base64.urlsafe_b64encode(digest).rstrip(b"=").decode("ascii")
    return verifier, challenge


# ── Token storage + refresh ──────────────────────────────────────────────
def _save_tokens(d: dict) -> None:
    os.makedirs(os.path.dirname(_TOKENS_PATH), exist_ok=True)
    d.setdefault("saved_at", int(time.time()))
    if "expires_in" in d and "expires_at" not in d:
        # 30s safety margin so we never present a soon-to-expire token
        d["expires_at"] = int(time.time()) + int(d["expires_in"]) - 30
    with open(_TOKENS_PATH, "w") as f:
        json.dump(d, f, indent=2)
    os.chmod(_TOKENS_PATH, 0o600)


def _load_tokens() -> dict:
    try:
        with open(_TOKENS_PATH) as f:
            return json.load(f)
    except (OSError, ValueError):
        return {}


def _refresh_if_needed() -> Optional[str]:
    """Return a valid access token, refreshing on expiry. None on failure."""
    t = _load_tokens()
    if not t:
        return None
    if t.get("expires_at", 0) > time.time() + 5:
        return t.get("access_token")
    rt = t.get("refresh_token")
    cid = _client_id()
    if not rt or not cid:
        return None
    code, body = _curl(
        "POST", "https://accounts.spotify.com/api/token",
        data={"grant_type": "refresh_token",
              "refresh_token": rt,
              "client_id": cid},
    )
    if code != 200:
        return None
    try:
        new = json.loads(body)
    except ValueError:
        return None
    # Spotify sometimes omits a new refresh_token on refresh; preserve old.
    new.setdefault("refresh_token", rt)
    _save_tokens(new)
    return new.get("access_token")


# ── Thin Spotify Web API wrapper ─────────────────────────────────────────
def _api(path: str, params: Optional[dict] = None,
         method: str = "GET", body: Optional[dict] = None) -> dict:
    tok = _refresh_if_needed()
    if not tok:
        return {"status": "unauthorized",
                "detail": "run `python3 ~/openjarvis/jarvis_spotify.py auth`"}
    url = "https://api.spotify.com/v1" + path
    if params:
        url += "?" + urllib.parse.urlencode(params)
    headers = {"Authorization": f"Bearer {tok}"}
    code, raw = _curl(method, url, headers=headers, json_body=body)
    # 200 = OK with body, 202/204 = OK no body (playback endpoints
    # typically respond 204 on success).
    if code in (200, 201, 202, 204):
        if not raw:
            return {"status": "ok", "data": None}
        try:
            return {"status": "ok", "data": json.loads(raw)}
        except ValueError:
            return {"status": "ok", "data": None}
    return {"status": "error", "code": code, "detail": (raw or "")[:200]}


# ── Public functions (consumed by the MCP server + the CLI) ──────────────
_TIME_RANGES = {"short": "short_term", "medium": "medium_term",
                "long": "long_term",
                # passthrough for already-correct values
                "short_term": "short_term", "medium_term": "medium_term",
                "long_term": "long_term"}


def top_artists(time_range: str = "medium", limit: int = 10) -> dict:
    """Top artists. ``time_range``: short (~4w), medium (~6mo), long (years)."""
    tr = _TIME_RANGES.get(time_range, "medium_term")
    res = _api("/me/top/artists",
               {"time_range": tr, "limit": max(1, min(50, int(limit)))})
    if res.get("status") != "ok":
        return res
    items = (res["data"] or {}).get("items") or []
    return {"status": "ok", "time_range": tr,
            "artists": [{"name": a["name"],
                         "genres": (a.get("genres") or [])[:3],
                         "popularity": a.get("popularity")} for a in items]}


def top_tracks(time_range: str = "medium", limit: int = 10) -> dict:
    tr = _TIME_RANGES.get(time_range, "medium_term")
    res = _api("/me/top/tracks",
               {"time_range": tr, "limit": max(1, min(50, int(limit)))})
    if res.get("status") != "ok":
        return res
    items = (res["data"] or {}).get("items") or []
    return {"status": "ok", "time_range": tr,
            "tracks": [{"name": t["name"],
                        "artists": [a["name"] for a in t.get("artists", [])]}
                       for t in items]}


def recently_played(limit: int = 10) -> dict:
    res = _api("/me/player/recently-played",
               {"limit": max(1, min(50, int(limit)))})
    if res.get("status") != "ok":
        return res
    items = (res["data"] or {}).get("items") or []
    return {"status": "ok",
            "tracks": [{"name": i["track"]["name"],
                        "artists": [a["name"]
                                    for a in i["track"].get("artists", [])],
                        "played_at": i.get("played_at")} for i in items]}


def current_track() -> dict:
    res = _api("/me/player/currently-playing")
    if res.get("status") != "ok":
        return res
    d = res.get("data")
    if not d or not d.get("item"):
        return {"status": "ok", "playing": False}
    t = d["item"]
    return {"status": "ok",
            "playing": d.get("is_playing", False),
            "name": t["name"],
            "artists": [a["name"] for a in t.get("artists", [])],
            "album": (t.get("album") or {}).get("name", ""),
            "progress_ms": d.get("progress_ms")}


# ── Playback control (requires user-modify-playback-state scope) ─────────
def search_tracks(query: str, limit: int = 5) -> dict:
    """Search Spotify for tracks matching `query`. Returns list of
    candidates with name, artists, album, URI, and id."""
    res = _api("/search",
               {"q": query, "type": "track",
                "limit": max(1, min(20, int(limit)))})
    if res.get("status") != "ok":
        return res
    items = ((res.get("data") or {}).get("tracks") or {}).get("items") or []
    return {"status": "ok",
            "tracks": [{
                "name": t["name"],
                "artists": [a["name"] for a in t.get("artists", [])],
                "album": (t.get("album") or {}).get("name", ""),
                "uri": t["uri"],
                "id": t["id"],
                "duration_ms": t.get("duration_ms"),
            } for t in items]}


def devices() -> dict:
    """List Spotify Connect devices the user can play on (Sonos, phones,
    laptops, etc.). Each entry has id, name, type, is_active, volume."""
    res = _api("/me/player/devices")
    if res.get("status") != "ok":
        return res
    return {"status": "ok",
            "devices": (res.get("data") or {}).get("devices") or []}


def _find_device(name_or_id: str) -> Optional[str]:
    """Resolve a user-friendly device name (substring, case-insensitive)
    or accept a literal device id. Returns the device id or None."""
    if not name_or_id:
        return None
    d = devices()
    if d.get("status") != "ok":
        return None
    target = name_or_id.lower().strip()
    for dev in d.get("devices", []):
        if dev["id"] == name_or_id:
            return dev["id"]
        if target in (dev.get("name") or "").lower():
            return dev["id"]
    return None


def play_track(uri: str, device: Optional[str] = None) -> dict:
    """Start playback of a Spotify track URI on `device` (name or id).
    If device is None, plays on the user's currently-active device.
    Pass a URI like `spotify:track:...` from search_tracks results."""
    params = {}
    if device:
        did = _find_device(device)
        if not did:
            return {"status": "error",
                    "detail": f"no Spotify Connect device matching {device!r}"}
        params["device_id"] = did
    body = {"uris": [uri]} if uri.startswith("spotify:track:") else {"context_uri": uri}
    res = _api("/me/player/play", params=params or None,
               method="PUT", body=body)
    if res.get("status") != "ok":
        return res
    return {"status": "ok", "uri": uri, "device": device or "<active>"}


def search_and_play(query: str, device: Optional[str] = None) -> dict:
    """One-shot: search Spotify, pick the top hit, play it on `device`.
    Convenience wrapper for the common voice path."""
    s = search_tracks(query, limit=1)
    if s.get("status") != "ok":
        return s
    tracks = s.get("tracks") or []
    if not tracks:
        return {"status": "error", "detail": f"no Spotify results for {query!r}"}
    top = tracks[0]
    p = play_track(top["uri"], device=device)
    if p.get("status") != "ok":
        return p
    return {"status": "ok",
            "playing": top["name"],
            "artists": top["artists"],
            "device": device or "<active>"}


def pause() -> dict:
    res = _api("/me/player/pause", method="PUT")
    return res if res.get("status") == "ok" else res


def resume() -> dict:
    res = _api("/me/player/play", method="PUT")
    return res if res.get("status") == "ok" else res


def skip_next() -> dict:
    res = _api("/me/player/next", method="POST")
    return res if res.get("status") == "ok" else res


def skip_previous() -> dict:
    res = _api("/me/player/previous", method="POST")
    return res if res.get("status") == "ok" else res


# ── Interactive OAuth bootstrap ──────────────────────────────────────────
def _auth_url(challenge: str, state: str) -> str:
    return "https://accounts.spotify.com/authorize?" + urllib.parse.urlencode({
        "response_type": "code",
        "client_id": _client_id(),
        "scope": _SCOPES,
        "redirect_uri": _REDIRECT_URI,
        "state": state,
        "code_challenge_method": "S256",
        "code_challenge": challenge,
    })


class _AuthHandler(http.server.BaseHTTPRequestHandler):
    captured: dict = {}

    def do_GET(self):  # noqa: N802
        parsed = urllib.parse.urlparse(self.path)
        if parsed.path != "/callback":
            self.send_response(404)
            self.end_headers()
            return
        q = urllib.parse.parse_qs(parsed.query)
        _AuthHandler.captured["code"] = (q.get("code") or [None])[0]
        _AuthHandler.captured["state"] = (q.get("state") or [None])[0]
        _AuthHandler.captured["error"] = (q.get("error") or [None])[0]
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.end_headers()
        if _AuthHandler.captured["code"]:
            html = ("<h2>JARVIS Spotify auth complete.</h2>"
                    "<p>You can close this tab and return to the terminal.</p>")
        else:
            err = _AuthHandler.captured.get("error") or "unknown"
            html = f"<h2>Auth failed</h2><pre>{err}</pre>"
        self.wfile.write(html.encode())

    def log_message(self, format, *args):  # noqa: A002 — match stdlib API
        return  # silence default access logging


def _run_auth_flow() -> int:
    cid = _client_id()
    if not cid:
        print(f"ERROR: no client_id. Set [spotify] client_id in {_CONFIG_PATH} "
              "or SPOTIFY_CLIENT_ID env var.", file=sys.stderr)
        return 2

    verifier, challenge = _pkce_pair()
    state = secrets.token_urlsafe(16)
    server = http.server.HTTPServer(("127.0.0.1", _REDIRECT_PORT), _AuthHandler)
    threading.Thread(target=server.serve_forever, daemon=True).start()

    url = _auth_url(challenge, state)
    print("Opening browser for Spotify authorization...")
    print(f"  If it doesn't open, paste this URL:\n    {url}\n")
    try:
        webbrowser.open(url)
    except Exception:  # noqa: BLE001
        pass

    deadline = time.time() + 300  # 5 minutes
    while time.time() < deadline and "code" not in _AuthHandler.captured:
        time.sleep(0.2)
    server.shutdown()

    cap = _AuthHandler.captured
    if not cap.get("code"):
        print("Timed out or auth was cancelled.", file=sys.stderr)
        return 3
    if cap.get("state") != state:
        print("State mismatch — possible CSRF; aborting.", file=sys.stderr)
        return 4

    code, body = _curl(
        "POST", "https://accounts.spotify.com/api/token",
        data={"grant_type": "authorization_code",
              "code": cap["code"],
              "redirect_uri": _REDIRECT_URI,
              "client_id": cid,
              "code_verifier": verifier},
    )
    if code != 200:
        print(f"Token exchange failed: HTTP {code}\n{body}", file=sys.stderr)
        return 5
    try:
        tokens = json.loads(body)
    except ValueError:
        print(f"Token exchange returned non-JSON:\n{body}", file=sys.stderr)
        return 6
    _save_tokens(tokens)
    print(f"Saved tokens to {_TOKENS_PATH} (mode 0600).")

    t = top_artists("short", 3)
    if t.get("status") == "ok":
        names = ", ".join(a["name"] for a in t["artists"])
        print(f"Smoke test — your top artists (last 4 weeks): {names}")
    else:
        print(f"Smoke test failed: {t}")
    return 0


# ── CLI ──────────────────────────────────────────────────────────────────
def main(argv) -> int:
    if len(argv) < 2:
        print("Usage: jarvis_spotify.py <auth|top|recent|current>",
              file=sys.stderr)
        return 1
    cmd = argv[1]
    if cmd == "auth":
        return _run_auth_flow()
    if cmd == "top":
        print(json.dumps(top_artists("medium", 10), indent=2))
        return 0
    if cmd == "recent":
        print(json.dumps(recently_played(10), indent=2))
        return 0
    if cmd == "current":
        print(json.dumps(current_track(), indent=2))
        return 0
    print(f"unknown command: {cmd}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
