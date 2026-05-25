"""Vizio SmartCast control + Chromecast media casting for JARVIS.

Reads TV config from ``~/.openjarvis/config.toml`` ``[tv]`` section:

    [tv]
    brand = "vizio"
    ip = "10.0.0.69"
    auth_token = "Zehsvuaqo5"
    device_name = "Living Room TV"

TV control (power/inputs/volume/apps) shells out to the ``pyvizio`` CLI
that lives in the openjarvis venv — keeps async wrangling out of this
module and matches the "shell out to a reliable binary" pattern already
used elsewhere (curl in jarvis_personal, claude in _claude_brain).

Media casting uses ``pychromecast``: Vizio SmartCast TVs have
Chromecast built in (port 8009), so we cast a media URL to the
Default Media Receiver and let the TV play it.

Local/uncommitted like the rest of JARVIS.
"""
from __future__ import annotations

import os
import shutil
import subprocess
import time
from typing import Any

# Lazy: don't crash import on a box without tomllib (3.11+).
try:
    import tomllib
except ImportError:  # pragma: no cover
    tomllib = None  # type: ignore

_CFG_PATH = os.path.expanduser("~/.openjarvis/config.toml")
_PYVIZIO = os.path.expanduser("~/openjarvis/.venv/bin/pyvizio")


def _cfg() -> dict:
    if tomllib is None or not os.path.exists(_CFG_PATH):
        return {}
    try:
        with open(_CFG_PATH, "rb") as f:
            data = tomllib.load(f)
    except Exception:
        return {}
    return (data.get("tv") or {}) if isinstance(data, dict) else {}


def _ip_auth() -> tuple[str, str]:
    c = _cfg()
    return str(c.get("ip", "")), str(c.get("auth_token", ""))


def _run(*args: str, timeout: float = 8.0) -> tuple[int, str, str]:
    """Invoke the pyvizio CLI with --ip and --auth filled in.
    Returns (returncode, merged_output, stderr).

    pyvizio always exits 0 even when its HTTPS call to the TV times out
    (it just logs "ERROR:pyvizio:Failed to execute command"). So callers
    must inspect the merged output for that string in addition to rc."""
    ip, auth = _ip_auth()
    if not ip or not auth:
        return 2, "", "tv not configured: missing [tv] ip / auth_token in config.toml"
    if not (os.path.exists(_PYVIZIO) or shutil.which("pyvizio")):
        return 2, "", "pyvizio binary not found in openjarvis venv"
    cmd = [_PYVIZIO if os.path.exists(_PYVIZIO) else "pyvizio",
           f"--ip={ip}", f"--auth={auth}", *args]
    try:
        proc = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
        merged = (proc.stdout or "") + (proc.stderr or "")
        rc = proc.returncode
        # Promote pyvizio's silent failures to a real non-zero exit so callers
        # don't treat "Turning ON … Connection timeout" as success.
        if rc == 0 and ("Failed to execute command" in merged or
                        "ERROR:pyvizio" in merged):
            rc = 1
        return rc, merged, proc.stderr or ""
    except subprocess.TimeoutExpired:
        return 124, "", f"pyvizio timed out after {timeout}s"
    except Exception as exc:  # noqa: BLE001
        return 1, "", f"{type(exc).__name__}: {exc}"


# ── Wake-on-LAN: Vizios can't be turned ON over HTTPS when fully off
#   (the network stack is asleep). A WoL magic packet to the TV's MAC
#   brings it back up; then pyvizio commands work normally.
def _wol_send(mac: str) -> bool:
    """Send a WoL magic packet to ``mac`` on broadcast :9."""
    import socket
    mac = mac.replace(":", "").replace("-", "").strip()
    if len(mac) != 12:
        return False
    try:
        packet = b"\xff" * 6 + bytes.fromhex(mac) * 16
    except ValueError:
        return False
    sent = False
    for tgt in ("255.255.255.255", "10.255.255.255"):
        try:
            s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            s.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)
            s.sendto(packet, (tgt, 9))
            s.close()
            sent = True
        except Exception:
            pass
    return sent


def _tv_mac() -> str | None:
    """Return the TV's MAC from config.toml [tv] mac, else look it up via ARP."""
    cfg = _cfg()
    mac = (cfg.get("mac") or "").strip()
    if mac:
        return mac
    ip = cfg.get("ip") or ""
    if not ip:
        return None
    # Fallback: parse `arp -n <ip>` output for the MAC. Only works if the
    # TV was recently reachable so the ARP entry exists.
    try:
        out = subprocess.run(["arp", "-n", ip], capture_output=True,
                             text=True, timeout=3).stdout
        import re
        m = re.search(r"([0-9a-fA-F]{1,2}(?::[0-9a-fA-F]{1,2}){5})", out)
        if m:
            # Normalize to colon-padded
            parts = ["{:0>2}".format(p) for p in m.group(1).split(":")]
            return ":".join(parts)
    except Exception:
        pass
    return None


def _line_after(prefix: str, blob: str) -> str:
    """Extract a value from a pyvizio INFO: log line."""
    for ln in blob.splitlines():
        if prefix in ln:
            return ln.split(prefix, 1)[1].strip()
    return ""


# ── power ─────────────────────────────────────────────────────────────────────

def power_state() -> dict:
    rc, out, _ = _run("get-power-state")
    if rc != 0:
        return {"status": "error", "message": out.strip() or "rc != 0"}
    txt = _line_after("Device is", out).lower()
    on = "on" in txt and "off" not in txt
    return {"status": "ok", "power": "on" if on else "off"}


def power(state: str) -> dict:
    """state in {'on','off','toggle'}.

    For 'on': try WoL FIRST (Vizios can't be woken via HTTPS when fully
    off — the network stack is asleep), then send the SmartCast power-on
    as belt-and-suspenders. WoL wakes the network adapter; the HTTPS call
    then ensures the screen actually comes on in cases where WoL only
    woke the network."""
    state = (state or "").lower()
    if state not in ("on", "off", "toggle"):
        return {"status": "error", "message": f"unknown state '{state}' (use on|off|toggle)"}

    wol_sent = False
    if state == "on":
        mac = _tv_mac()
        if mac:
            wol_sent = _wol_send(mac)
            # The Vizio's network stack takes ~12s to come up from a cold
            # power-off after WoL. Shorter waits → the HTTPS poke times out
            # and the user thinks JARVIS broke. If you ever shorten this,
            # also stop reporting success below when pyvizio still fails.
            time.sleep(12.0)

    rc, out, _ = _run("power", state, timeout=12.0)
    ok = rc == 0
    # For power-on, WoL by itself is usually enough — the screen comes
    # alive once the TV finishes booting. Treat WoL-sent as success even
    # if pyvizio still timed out.
    if state == "on" and wol_sent:
        ok = True
    return {
        "status": "ok" if ok else "error",
        "message": out.strip()[:200],
        "wol_sent": wol_sent,
    }


# ── volume ────────────────────────────────────────────────────────────────────

def volume_get() -> dict:
    rc, out, _ = _run("get-volume-level")
    if rc != 0:
        return {"status": "error", "message": out.strip()}
    val = _line_after("Current volume:", out) or _line_after("volume:", out) or out.strip()
    # `get-volume-level` returns "None" when the TV is off.
    if val.strip().lower() in ("none", ""):
        return {"status": "ok", "volume": None, "note": "TV may be off"}
    return {"status": "ok", "volume": val}


def volume_set(level: int) -> dict:
    level = max(0, min(100, int(level)))
    rc, out, _ = _run("audio-setting", "volume", str(level))
    return {"status": "ok" if rc == 0 else "error", "volume": level, "message": out.strip()[:200]}


def volume_step(delta: int) -> dict:
    delta = int(delta)
    if delta == 0:
        return volume_get()
    direction = "up" if delta > 0 else "down"
    rc, out, _ = _run("volume", direction, str(abs(delta)))
    return {"status": "ok" if rc == 0 else "error", "delta": delta, "message": out.strip()[:200]}


def mute(state: str = "toggle") -> dict:
    state = (state or "toggle").lower()
    if state not in ("on", "off", "toggle"):
        state = "toggle"
    rc, out, _ = _run("mute", state)
    return {"status": "ok" if rc == 0 else "error", "message": out.strip()[:200]}


# ── inputs ────────────────────────────────────────────────────────────────────

def inputs_list() -> dict:
    rc, out, _ = _run("get-inputs-list")
    if rc != 0:
        return {"status": "error", "message": out.strip()}
    names: list[str] = []
    started = False
    for ln in out.splitlines():
        s = ln.strip()
        if not s or "Name" in s and "Nickname" in s:
            started = True
            continue
        if started and s.startswith("---"):
            continue
        if started and s and "INFO:" not in s:
            # Split on first whitespace gap
            parts = s.split()
            if parts:
                names.append(parts[0])
    return {"status": "ok", "inputs": names}


def input_current() -> dict:
    rc, out, _ = _run("get-current-input")
    if rc != 0:
        return {"status": "error", "message": out.strip()}
    cur = _line_after("Current input:", out)
    return {"status": "ok", "input": cur}


def input_set(name: str) -> dict:
    """Switch to a named input (CAST, HDMI-1, HDMI-2, etc.)."""
    if not name:
        return {"status": "error", "message": "missing input name"}
    rc, out, _ = _run("input", name)
    return {"status": "ok" if rc == 0 else "error", "input": name, "message": out.strip()[:200]}


# ── apps ──────────────────────────────────────────────────────────────────────

def current_app() -> dict:
    rc, out, _ = _run("get-current-app")
    if rc != 0:
        return {"status": "error", "message": out.strip()}
    name = _line_after("Current app:", out) or _line_after("app:", out) or out.strip()
    return {"status": "ok", "app": name}


def launch_app(name: str) -> dict:
    if not name:
        return {"status": "error", "message": "missing app name"}
    rc, out, _ = _run("launch-app", name)
    return {"status": "ok" if rc == 0 else "error", "app": name, "message": out.strip()[:200]}


# ── status snapshot ───────────────────────────────────────────────────────────

def status() -> dict:
    """One-shot snapshot for the brain: power + input + current app."""
    p = power_state()
    if p.get("power") != "on":
        return {"status": "ok", "power": "off"}
    inp = input_current()
    app = current_app()
    return {
        "status": "ok",
        "power": "on",
        "input": inp.get("input"),
        "app": app.get("app"),
    }


# ── media casting via Chromecast built-in ─────────────────────────────────────

_CAST_CACHE: dict[str, Any] = {"cast": None, "ts": 0.0}


def _cast_device():
    """Connect to the Vizio Chromecast (cached for 60s)."""
    ip, _ = _ip_auth()
    now = time.time()
    if _CAST_CACHE["cast"] is not None and now - _CAST_CACHE["ts"] < 60.0:
        return _CAST_CACHE["cast"]
    try:
        import pychromecast
    except Exception as exc:
        raise RuntimeError(f"pychromecast import failed: {exc}")
    chromecasts, browser = pychromecast.get_listed_chromecasts(known_hosts=[ip])
    try:
        if not chromecasts:
            raise RuntimeError(f"no Chromecast at {ip}:8009")
        cc = chromecasts[0]
        cc.wait(timeout=8.0)
        _CAST_CACHE["cast"] = cc
        _CAST_CACHE["ts"] = now
        return cc
    finally:
        try:
            pychromecast.discovery.stop_discovery(browser)
        except Exception:
            pass


def cast_url(url: str, content_type: str = "video/mp4", title: str = "") -> dict:
    """Cast an HTTP(S) media URL to the TV's built-in Chromecast.
    YouTube/Spotify need their own app launches — Default Media Receiver
    only plays direct media URLs (mp4, jpg, mp3, hls m3u8)."""
    if not url:
        return {"status": "error", "message": "missing url"}
    try:
        cc = _cast_device()
        mc = cc.media_controller
        mc.play_media(url, content_type, title=title or None)
        mc.block_until_active(timeout=10.0)
        return {"status": "ok", "url": url, "title": title or url}
    except Exception as exc:  # noqa: BLE001
        return {"status": "error", "message": f"{type(exc).__name__}: {exc}"}


def cast_stop() -> dict:
    try:
        cc = _cast_device()
        cc.media_controller.stop()
        return {"status": "ok"}
    except Exception as exc:  # noqa: BLE001
        return {"status": "error", "message": f"{type(exc).__name__}: {exc}"}


# ── YouTube: cast a specific video/queue via pychromecast YouTubeController ──

_YT_ID_RE = None


def _youtube_id(url_or_id: str) -> str:
    """Extract an 11-char YouTube video ID from a URL or take a bare ID."""
    global _YT_ID_RE
    if _YT_ID_RE is None:
        import re
        _YT_ID_RE = re.compile(
            r"(?:youtu\.be/|v=|/embed/|/shorts/)([A-Za-z0-9_-]{11})"
        )
    s = (url_or_id or "").strip()
    if not s:
        return ""
    if len(s) == 11 and "/" not in s and "=" not in s:
        return s  # bare ID
    m = _YT_ID_RE.search(s)
    return m.group(1) if m else ""


def youtube_play(url_or_id: str) -> dict:
    """Cast a YouTube video to the TV's built-in YouTube cast app.
    Accepts a video URL (youtube.com/watch?v=…, youtu.be/…) or a raw ID."""
    vid = _youtube_id(url_or_id)
    if not vid:
        return {"status": "error", "message": f"could not parse youtube id from '{url_or_id}'"}
    try:
        from pychromecast.controllers.youtube import YouTubeController
        cc = _cast_device()
        yt = YouTubeController()
        cc.register_handler(yt)
        yt.play_video(vid)
        return {"status": "ok", "video_id": vid}
    except Exception as exc:  # noqa: BLE001
        return {"status": "error", "message": f"{type(exc).__name__}: {exc}"}


def youtube_add_to_queue(url_or_id: str) -> dict:
    vid = _youtube_id(url_or_id)
    if not vid:
        return {"status": "error", "message": f"could not parse youtube id from '{url_or_id}'"}
    try:
        from pychromecast.controllers.youtube import YouTubeController
        cc = _cast_device()
        yt = YouTubeController()
        cc.register_handler(yt)
        yt.add_to_queue(vid)
        return {"status": "ok", "video_id": vid}
    except Exception as exc:  # noqa: BLE001
        return {"status": "error", "message": f"{type(exc).__name__}: {exc}"}


# ── Spotify Connect to TV (requires Spotify Premium + signed-in TV app) ──────

def _spotify_token() -> str | None:
    try:
        import sys as _sys
        _sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
        import jarvis_spotify as _sp
        return _sp._refresh_if_needed()
    except Exception:
        return None


def _spotify_curl(method: str, path: str, body: dict | None = None) -> tuple[int, str]:
    """Call Spotify Web API with the refreshed access token. Returns (http_code, body)."""
    tok = _spotify_token()
    if not tok:
        return 401, '{"error": "no spotify token (run jarvis_spotify.py auth)"}'
    url = "https://api.spotify.com/v1" + path
    cmd = ["curl", "-sS", "--max-time", "8", "-X", method, "-w", "\n%{http_code}",
           "-H", f"Authorization: Bearer {tok}", "-H", "Content-Type: application/json", url]
    if body is not None:
        import json as _json
        cmd += ["--data", _json.dumps(body)]
    try:
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=10)
        out = r.stdout
        if "\n" in out:
            payload, code = out.rsplit("\n", 1)
            try:
                return int(code.strip()), payload
            except ValueError:
                return 0, out
        return 0, out
    except Exception as exc:  # noqa: BLE001
        return 0, f"{type(exc).__name__}: {exc}"


def _spotify_find_tv_device() -> dict | None:
    """Return the Spotify Connect device that looks like the TV, or None."""
    code, body = _spotify_curl("GET", "/me/player/devices")
    if code != 200:
        return None
    try:
        import json as _json
        data = _json.loads(body)
    except ValueError:
        return None
    # Match the configured TV's device_name; also accept common TV names.
    cfg = _cfg()
    needles = {
        (cfg.get("device_name") or "").lower(),
        "vizio", "smartcast", "living room tv", "tv", "smart tv",
    }
    for d in data.get("devices", []):
        name = (d.get("name") or "").lower()
        if any(n and n in name for n in needles) or (d.get("type") or "").lower() == "tv":
            return d
    return None


def spotify_play_on_tv(uri: str = "", query: str = "") -> dict:
    """Transfer Spotify playback to the TV and start `uri` (spotify:track:…
    / spotify:playlist:… / spotify:album:…). If `query` is given instead, the
    top track search hit is used. Requires Spotify Premium and the Spotify
    app to be signed in on the TV.

    If the TV isn't visible as a Spotify Connect device, this launches the
    Spotify app on the TV and retries once.
    """
    if not uri and not query:
        return {"status": "error", "message": "need uri or query"}

    # Resolve query → uri if needed
    if not uri and query:
        code, body = _spotify_curl(
            "GET", f"/search?q={subprocess.list2cmdline([query])}&type=track&limit=1",
        )
        # urlencode properly via curl --data-urlencode would be cleaner; rebuild:
        import urllib.parse as _u
        code, body = _spotify_curl(
            "GET", "/search?" + _u.urlencode({"q": query, "type": "track", "limit": "1"}),
        )
        if code != 200:
            return {"status": "error", "message": f"search failed http={code}"}
        try:
            import json as _json
            tracks = _json.loads(body).get("tracks", {}).get("items", [])
        except ValueError:
            tracks = []
        if not tracks:
            return {"status": "error", "message": f"no track found for '{query}'"}
        uri = tracks[0]["uri"]

    # Find TV in Spotify Connect device list; if missing, launch the app and retry
    dev = _spotify_find_tv_device()
    if dev is None:
        launch_app("Spotify")
        time.sleep(6)
        dev = _spotify_find_tv_device()
    if dev is None:
        return {
            "status": "error",
            "message": "TV's Spotify app not signed in / not visible as a Connect "
                       "device. Open Spotify on the TV, log in once with the user's "
                       "account, then retry.",
        }

    # Transfer playback to the TV
    code, body = _spotify_curl(
        "PUT", "/me/player",
        body={"device_ids": [dev["id"]], "play": False},
    )
    if code not in (200, 202, 204):
        return {"status": "error", "message": f"transfer failed http={code} body={body[:200]}"}

    # Start the URI on the TV
    play_body: dict = {}
    if uri.startswith("spotify:track:"):
        play_body["uris"] = [uri]
    else:
        play_body["context_uri"] = uri
    code, body = _spotify_curl(
        "PUT", f"/me/player/play?device_id={dev['id']}",
        body=play_body,
    )
    if code not in (200, 202, 204):
        return {"status": "error", "message": f"play failed http={code} body={body[:200]}"}
    return {"status": "ok", "device": dev.get("name"), "uri": uri}


def spotify_pause() -> dict:
    code, _ = _spotify_curl("PUT", "/me/player/pause")
    return {"status": "ok" if code in (200, 202, 204) else "error", "http": code}


# ── Plex: search the library and cast a stream URL to the TV ─────────────────

def _plex_server():
    """Connect to the user's Plex server (uses [plex] from config.toml)."""
    cfg = _cfg()  # this returns the [tv] section; we want [plex]
    plex_cfg = {}
    if tomllib is not None and os.path.exists(_CFG_PATH):
        try:
            with open(_CFG_PATH, "rb") as f:
                plex_cfg = (tomllib.load(f).get("plex") or {})
        except Exception:
            plex_cfg = {}
    baseurl = plex_cfg.get("baseurl") or "http://10.44.0.3:32400"
    token = plex_cfg.get("token")
    if not token:
        raise RuntimeError("plex token not configured ([plex] token in ~/.openjarvis/config.toml)")
    from plexapi.server import PlexServer
    return PlexServer(baseurl, token, timeout=8)


def plex_libraries() -> dict:
    """List the Plex library sections (movies, shows, music, etc.)."""
    try:
        srv = _plex_server()
        return {"status": "ok", "libraries": [
            {"title": s.title, "type": s.type, "count": s.totalSize}
            for s in srv.library.sections()
        ]}
    except Exception as exc:  # noqa: BLE001
        return {"status": "error", "message": f"{type(exc).__name__}: {exc}"}


def plex_search(query: str, limit: int = 5) -> dict:
    """Search Plex across all libraries; return up to `limit` hits."""
    if not query:
        return {"status": "error", "message": "missing query"}
    try:
        srv = _plex_server()
        results = srv.search(query, limit=limit)
        return {"status": "ok", "results": [
            {"title": r.title, "type": r.type, "year": getattr(r, "year", None),
             "key": r.key, "ratingKey": r.ratingKey}
            for r in results
        ]}
    except Exception as exc:  # noqa: BLE001
        return {"status": "error", "message": f"{type(exc).__name__}: {exc}"}


def plex_play(query: str) -> dict:
    """Search Plex for `query`, find a playable item, and start it on the TV.

    Three-tier strategy:
    1. If the TV's Plex app registers as a Plex client → use proper Plex
       remote control (preserves resume position, real session).
    2. Else if dumb-casting works (Default Media Receiver) → cast the
       stream URL. Works for some content; Plex's DASH/HLS variants are
       hit-or-miss on the DMR.
    3. Else → launch the Plex app on the TV and return the resolved
       title so the brain can tell the user what to navigate to.

    Music libraries are skipped — use Spotify for those.
    """
    if not query:
        return {"status": "error", "message": "missing query"}
    try:
        srv = _plex_server()
        results = srv.search(query, limit=5)
        pick = None
        for r in results:
            if r.type in ("movie", "episode", "show"):
                pick = r
                break
        if pick is None:
            return {"status": "error", "message": f"no video result for '{query}'"}
        # Resolve a show to the next-unwatched episode (real "resume watching").
        if pick.type == "show":
            eps = pick.episodes()
            if not eps:
                return {"status": "error", "message": "show has no episodes"}
            unwatched = [e for e in eps if not e.isWatched]
            pick = unwatched[0] if unwatched else eps[0]
        title = getattr(pick, "title", "")
        if hasattr(pick, "grandparentTitle") and pick.grandparentTitle:
            ep_label = ""
            if hasattr(pick, "seasonEpisode") and pick.seasonEpisode:
                ep_label = f" {pick.seasonEpisode}"
            title = f"{pick.grandparentTitle}{ep_label} — {title}"

        # Tier 1: real Plex client
        try:
            clients = srv.clients()
        except Exception:
            clients = []
        if clients:
            # Prefer a client whose name/device looks like the TV.
            cfg = _cfg()
            needles = {(cfg.get("device_name") or "").lower(),
                       "vizio", "smartcast", "tv", "living room"}
            target = None
            for c in clients:
                name = (getattr(c, "title", "") or "").lower()
                if any(n and n in name for n in needles):
                    target = c
                    break
            target = target or clients[0]
            try:
                target.playMedia(pick)
                return {"status": "ok", "via": "plex_client",
                        "client": target.title, "title": title}
            except Exception as exc:  # noqa: BLE001
                # fall through to next tier
                last_exc = f"plex client play failed: {exc}"
        else:
            last_exc = "no plex clients registered"

        # Tier 3: launch Plex app on the TV and return for manual nav.
        # (We skip dumb-DMR-cast tier 2 because it accepts the URL silently
        # but Plex's DASH/HLS streams don't reliably play on the Vizio DMR —
        # leading to false-positive "casting" replies. Launching the app is
        # honest.)
        launch_app("Plex")
        return {
            "status": "partial",
            "via": "app_launch",
            "title": title,
            "message": f"Plex app opened on the TV — Plex client API not "
                       f"available for direct play ({last_exc}). Navigate "
                       f"to: {title}",
        }
    except Exception as exc:  # noqa: BLE001
        return {"status": "error", "message": f"{type(exc).__name__}: {exc}"}


if __name__ == "__main__":
    # Quick smoke for the user: python jarvis_tv.py status
    import json
    import sys
    if len(sys.argv) >= 2 and sys.argv[1] == "status":
        print(json.dumps(status(), indent=2))
    elif len(sys.argv) >= 3 and sys.argv[1] == "power":
        print(json.dumps(power(sys.argv[2]), indent=2))
    elif len(sys.argv) >= 3 and sys.argv[1] == "volume":
        print(json.dumps(volume_set(int(sys.argv[2])), indent=2))
    elif len(sys.argv) >= 3 and sys.argv[1] == "input":
        print(json.dumps(input_set(sys.argv[2]), indent=2))
    elif len(sys.argv) >= 3 and sys.argv[1] == "cast":
        print(json.dumps(cast_url(sys.argv[2]), indent=2))
    else:
        print("usage: jarvis_tv.py {status|power on|off|toggle|volume N|input NAME|cast URL}")
