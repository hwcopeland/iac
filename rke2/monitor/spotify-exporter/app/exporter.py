#!/usr/bin/env python3
"""Spotify → Prometheus exporter (standalone, no JARVIS dependency).

Polls the Spotify Web API on a background loop and exposes metrics on
:9112/metrics. OAuth tokens live in a shared Kubernetes Secret (see
tokenstore.py) so this exporter and JARVIS never fight over Spotify's
rotating PKCE refresh token. Play counts persist to the /state PVC.

Metric families
---------------
  spotify_up                                 1 if the last poll cycle succeeded
  spotify_scrape_errors_total                cumulative poll/API errors

  spotify_now_playing                        1 if something is actively playing
  spotify_now_playing_progress_ratio         0..1 position within current track
  spotify_now_playing_info{track,artist,album}   always 1 (label carrier)

  spotify_top_artist_rank{range,artist,genre}    1=most played (lower = higher)
  spotify_artist_popularity{range,artist}        Spotify popularity 0..100
  spotify_top_track_rank{range,track,artist}     1=most played
  spotify_genre_score{range,genre}               rank-weighted genre share %

  spotify_plays_total{artist}                plays observed via recently-played
  spotify_plays_total (label artist="__all__") global play counter

`range` is one of short (~4w) / medium (~6mo) / long (years).

Everything is local/personal scale, so per-artist label cardinality is fine.
"""
from __future__ import annotations

import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request
from collections import Counter
from typing import Optional

from prometheus_client import (
    CollectorRegistry,
    Counter as PromCounter,
    Gauge,
    start_http_server,
)

import tokenstore

# ── Config ────────────────────────────────────────────────────────────────
STATE_DIR = os.environ.get("SPOTIFY_STATE_DIR", "/state")
PLAYS_PATH = os.path.join(STATE_DIR, "plays.json")
PORT = int(os.environ.get("SPOTIFY_EXPORTER_PORT", "9112"))

# Poll cadences (seconds)
NOW_PLAYING_INTERVAL = int(os.environ.get("NOW_PLAYING_INTERVAL", "15"))
RECENT_INTERVAL = int(os.environ.get("RECENT_INTERVAL", "120"))
TOP_INTERVAL = int(os.environ.get("TOP_INTERVAL", "1800"))  # 30 min

_HTTP_TIMEOUT = 12
_UA = "spotify-exporter/1.0 (+homelab)"
_API = "https://api.spotify.com/v1"
_RANGES = {"short": "short_term", "medium": "medium_term", "long": "long_term"}

# ── Metrics ─────────────────────────────────────────────────────────────────
REG = CollectorRegistry()
M_UP = Gauge("spotify_up", "1 if the last Spotify poll cycle succeeded", registry=REG)
M_ERRORS = PromCounter("spotify_scrape_errors_total", "Cumulative Spotify poll/API errors", registry=REG)

M_NP = Gauge("spotify_now_playing", "1 if a track is actively playing", registry=REG)
M_NP_PROG = Gauge("spotify_now_playing_progress_ratio", "Position within current track (0..1)", registry=REG)
M_NP_INFO = Gauge("spotify_now_playing_info", "Current track (label carrier)", ["track", "artist", "album"], registry=REG)

M_TOP_ARTIST = Gauge("spotify_top_artist_rank", "Top artist rank (1=most played)", ["range", "artist", "genre"], registry=REG)
M_ARTIST_POP = Gauge("spotify_artist_popularity", "Spotify artist popularity (0..100)", ["range", "artist"], registry=REG)
M_TOP_TRACK = Gauge("spotify_top_track_rank", "Top track rank (1=most played)", ["range", "track", "artist"], registry=REG)
M_GENRE = Gauge("spotify_genre_score", "Rank-weighted genre share (%)", ["range", "genre"], registry=REG)

M_PLAYS = PromCounter("spotify_plays", "Plays observed via recently-played", ["artist"], registry=REG)


# ── Spotify Web API (token comes from the shared cluster Secret) ─────────────
def _http_get(url: str, headers: dict) -> tuple[int, bytes]:
    req = urllib.request.Request(url, method="GET")
    req.add_header("User-Agent", _UA)
    for k, v in headers.items():
        req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=_HTTP_TIMEOUT,
                                    context=tokenstore._SPOTIFY_CTX) as r:
            return r.getcode(), r.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()
    except (urllib.error.URLError, OSError, TimeoutError):
        return 0, b""


def _api(path: str, params: Optional[dict] = None) -> Optional[dict]:
    try:
        tok = tokenstore.access_token()
    except tokenstore.TokenError as exc:
        M_ERRORS.inc()
        print(f"token store error: {exc}", flush=True)
        return None
    if not tok:
        M_ERRORS.inc()
        return None
    url = _API + path
    if params:
        url += "?" + urllib.parse.urlencode(params)
    code, raw = _http_get(url, headers={"Authorization": f"Bearer {tok}"})
    if code == 204:
        return {"__empty__": True}
    if code != 200:
        M_ERRORS.inc()
        return None
    try:
        return json.loads(raw)
    except ValueError:
        M_ERRORS.inc()
        return None


# ── Pollers ──────────────────────────────────────────────────────────────────
def poll_now_playing() -> None:
    d = _api("/me/player/currently-playing")
    M_NP_INFO.clear()
    if not d or d.get("__empty__") or not d.get("item"):
        M_NP.set(0)
        M_NP_PROG.set(0)
        return
    item = d["item"]
    playing = bool(d.get("is_playing"))
    dur = item.get("duration_ms") or 0
    prog = d.get("progress_ms") or 0
    M_NP.set(1 if playing else 0)
    M_NP_PROG.set((prog / dur) if dur else 0)
    M_NP_INFO.labels(
        track=item.get("name", "")[:120],
        artist=", ".join(a["name"] for a in item.get("artists", []))[:120],
        album=(item.get("album") or {}).get("name", "")[:120],
    ).set(1)


def poll_recently_played(state: dict) -> None:
    d = _api("/me/player/recently-played", {"limit": 50})
    if not d or not d.get("items"):
        return
    seen: set[str] = set(state.get("seen", []))
    new_seen = list(seen)
    counts: dict = state.setdefault("counts", {})
    added = 0
    for it in d["items"]:
        pid = it.get("played_at")
        if not pid or pid in seen:
            continue
        seen.add(pid)
        new_seen.append(pid)
        added += 1
        artists = it.get("track", {}).get("artists", [])
        primary = (artists[0]["name"] if artists else "unknown")[:120]
        M_PLAYS.labels(artist=primary).inc()
        M_PLAYS.labels(artist="__all__").inc()
        counts[primary] = counts.get(primary, 0) + 1
        counts["__all__"] = counts.get("__all__", 0) + 1
    if added:
        # Keep the dedupe window bounded; played_at is ISO8601 so lexical sort = chronological.
        state["seen"] = sorted(new_seen)[-500:]
        _persist_plays(state)


def _persist_plays(state: dict) -> None:
    os.makedirs(STATE_DIR, exist_ok=True)
    tmp = PLAYS_PATH + ".tmp"
    with open(tmp, "w") as f:
        json.dump(state, f)
    os.replace(tmp, PLAYS_PATH)


def _restore_plays() -> dict:
    try:
        with open(PLAYS_PATH) as f:
            state = json.load(f)
    except (OSError, ValueError):
        state = {}
    # Re-seed counters so rate() is continuous across restarts.
    for artist, n in (state.get("counts") or {}).items():
        M_PLAYS.labels(artist=artist).inc(n)
    return state


def poll_top(state: dict) -> None:
    M_TOP_ARTIST.clear()
    M_ARTIST_POP.clear()
    M_TOP_TRACK.clear()
    M_GENRE.clear()
    for short, full in _RANGES.items():
        arts = _api("/me/top/artists", {"time_range": full, "limit": 50})
        items = (arts or {}).get("items") or []
        gw: Counter = Counter()
        n = len(items)
        for i, a in enumerate(items):
            rank = i + 1
            name = a.get("name", "")[:120]
            genres = a.get("genres") or []
            primary = (genres[0] if genres else "untagged")[:60]
            M_TOP_ARTIST.labels(range=short, artist=name, genre=primary).set(rank)
            M_ARTIST_POP.labels(range=short, artist=name).set(a.get("popularity") or 0)
            for g in genres:
                gw[g[:60]] += (n - i)  # rank-weighted
        total = sum(gw.values()) or 1
        for g, w in gw.most_common(20):
            M_GENRE.labels(range=short, genre=g).set(round(100 * w / total, 2))

        trks = _api("/me/top/tracks", {"time_range": full, "limit": 50})
        for i, t in enumerate(((trks or {}).get("items") or [])):
            M_TOP_TRACK.labels(
                range=short,
                track=t.get("name", "")[:120],
                artist=", ".join(a["name"] for a in t.get("artists", []))[:120],
            ).set(i + 1)


# ── Main loop ─────────────────────────────────────────────────────────────────
def main() -> None:
    plays_state = _restore_plays()
    # Track per-artist counts for restart re-seeding.
    if "counts" not in plays_state:
        plays_state["counts"] = {}

    start_http_server(PORT, registry=REG)
    print(f"spotify-exporter listening on :{PORT}/metrics", flush=True)

    last_recent = 0.0
    last_top = 0.0
    while True:
        cycle_ok = True
        try:
            poll_now_playing()
            now = time.time()
            if now - last_recent >= RECENT_INTERVAL:
                poll_recently_played(plays_state)
                last_recent = now
            if now - last_top >= TOP_INTERVAL:
                poll_top(plays_state)
                last_top = now
        except Exception as exc:  # noqa: BLE001 — never let the loop die
            M_ERRORS.inc()
            cycle_ok = False
            print(f"poll error: {type(exc).__name__}: {exc}", flush=True)
        M_UP.set(1 if cycle_ok else 0)
        time.sleep(NOW_PLAYING_INTERVAL)


if __name__ == "__main__":
    main()
