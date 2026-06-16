#!/usr/bin/env python3
"""Spotify → Prometheus exporter (standalone, no JARVIS dependency).

Polls the Spotify Web API on a background loop and exposes metrics on
:9112/metrics. OAuth tokens live in a shared Kubernetes Secret (see
tokenstore.py) so this exporter and JARVIS never fight over Spotify's
rotating PKCE refresh token.

The DURABLE listening record now lives in Postgres (see db.py): play_events,
tracks/artists/albums dimensions, the saved-library diff, and the genre rollup.
Postgres is the dedup authority. The /state PVC is only a Prometheus-only
fallback dedup store when the DB sink is disabled.

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
import genre_rollup as genre_rollup_mod
from db import DB_INSTANCE as DB

# ── Config ────────────────────────────────────────────────────────────────
STATE_DIR = os.environ.get("SPOTIFY_STATE_DIR", "/state")
PLAYS_PATH = os.path.join(STATE_DIR, "plays.json")
PORT = int(os.environ.get("METRICS_PORT", "9112"))

# Poll cadences (seconds)
NOW_PLAYING_INTERVAL = int(os.environ.get("NOW_PLAYING_INTERVAL", "15"))
RECENT_INTERVAL = int(os.environ.get("RECENT_INTERVAL", "120"))
TOP_INTERVAL = int(os.environ.get("TOP_INTERVAL", "1800"))  # 30 min
# Library snapshot diff runs on a slow cadence (default 12h).
LIBRARY_INTERVAL = int(os.environ.get("LIBRARY_INTERVAL", str(12 * 3600)))
# Enrichment backlog/coverage gauges refresh on a cheap, frequent cadence.
ENRICH_STATS_INTERVAL = int(os.environ.get("ENRICH_STATS_INTERVAL", "300"))  # 5 min

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

# DB-backed health/inventory (durable record now lives in Postgres).
M_DB_UP = Gauge("spotify_db_up", "1 if the Postgres analytics sink is reachable", registry=REG)
M_DB_PLAYS_INSERTED = PromCounter("spotify_db_plays_inserted", "New play_events rows inserted by this exporter", registry=REG)
M_LIBRARY_SAVED = Gauge("spotify_library_saved_tracks", "Currently-saved tracks (library_tracks, removed_at IS NULL)", registry=REG)

# Genre-enrichment health (the MusicBrainz CronJob does the work; these gauges
# are derived from the same music_ids / track_genres tables so the
# backlog/coverage are continuously scrapeable without scraping the short-lived
# CronJob pod).
M_ENRICH_MATCHED = Gauge("spotify_enrich_matched_tracks", "Tracks with a resolved MusicBrainz match (match_status='matched')", registry=REG)
M_ENRICH_NOMATCH = Gauge("spotify_enrich_nomatch_tracks", "Tracks that resolved to no MusicBrainz match (match_status='nomatch')", registry=REG)
M_ENRICH_BACKLOG = Gauge("spotify_enrich_backlog_tracks", "Tracks still awaiting enrichment (no terminal music_ids row)", registry=REG)
M_ENRICH_TAGGED = Gauge("spotify_enrich_tracks_with_mb_tags", "Distinct tracks with at least one MusicBrainz sub-genre tag", registry=REG)
M_ENRICH_OLD_FILLED = Gauge("spotify_enrich_untagged_old_filled", "Previously Spotify-untagged tracks now carrying a MusicBrainz sub-genre", registry=REG)


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


# ── Dimension helpers ────────────────────────────────────────────────────────
def _artist_genres(artist_id: str) -> list[str]:
    """Fetch genres[] for an artist, cached per id (genres rarely change).

    Some artists return genres: [] — that's fine, we store the empty array and
    handle 'untagged' at rollup time.
    """
    if not artist_id:
        return []
    cached = DB.artist_genre_cache.get(artist_id)
    if cached is not None:
        return cached
    d = _api(f"/artists/{artist_id}")
    genres = [g[:60] for g in ((d or {}).get("genres") or [])]
    DB.artist_genre_cache[artist_id] = genres
    return genres


def _upsert_track_dimensions(track: dict) -> Optional[str]:
    """Upsert artist/album/track dimensions for a recently-played track item.

    RESILIENCE CONTRACT: the irreplaceable datum is the *play* (the listening
    event), so no single failing dimension may ever block it. Each dimension is
    upserted independently; if the artist or album write fails (e.g. a bad
    release_date, a transient DB error), we null that FK on the track so the
    track row still lands. The track row is the minimal prerequisite for the
    play's FK — as long as it persists, the play persists. The dimension failure
    is logged but never propagated.

    Returns the track_id when the track row is present (so the caller can insert
    the play), or None when even the bare track couldn't be written.
    No-op (returns the id) when the DB sink is disabled.
    """
    if not DB.enabled():
        return (track or {}).get("id")
    track_id = (track or {}).get("id")
    if not track_id:
        return None

    artists = track.get("artists") or []
    primary_artist = artists[0] if artists else {}
    artist_id = primary_artist.get("id")
    if artist_id:
        genres = _artist_genres(artist_id)
        if not DB.upsert_artist(
            artist_id,
            primary_artist.get("name", ""),
            primary_artist.get("popularity"),  # usually absent on the track payload
            genres,
        ):
            # Artist dimension failed — drop the FK, keep the track + play.
            print(f"dim: artist upsert failed for {artist_id}; "
                  f"writing track {track_id} with artist_id=NULL", flush=True)
            artist_id = None

    album = track.get("album") or {}
    album_id = album.get("id")
    if album_id:
        if not DB.upsert_album(
            album_id,
            album.get("name", ""),
            album.get("release_date"),
            album.get("release_date_precision"),
        ):
            # Album dimension failed — drop the FK, keep the track + play.
            print(f"dim: album upsert failed for {album_id}; "
                  f"writing track {track_id} with album_id=NULL", flush=True)
            album_id = None

    # ISRC (recording identifier) lives under external_ids on the FULL track
    # object returned by both /me/tracks and /recently-played. It is the join
    # key for the downstream genre-enrichment pipeline (ISRC → MusicBrainz).
    isrc = ((track.get("external_ids") or {}).get("isrc") or None)
    if isrc:
        isrc = isrc.strip().upper()[:20] or None

    if not DB.upsert_track(
        track_id,
        track.get("name", ""),
        artist_id,
        album_id,
        track.get("duration_ms"),
        track.get("popularity"),
        isrc,
    ):
        # Even the bare track couldn't be written — the play FK would fail, so
        # signal the caller not to attempt it (avoids a noisy FK-violation log).
        print(f"dim: track upsert failed for {track_id}; play skipped this "
              f"cycle, retried next poll", flush=True)
        return None
    return track_id


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
    """Record each recently-played item as a row in play_events (DB-authoritative).

    Postgres is now the durable record and the dedup authority: each play is an
    INSERT ... ON CONFLICT (played_at, track_id) DO NOTHING, and the RETURNING
    tells us whether the row was new. We keep the live Prometheus plays counter
    (cheap, drives the now-playing/rate panels) but it is NO LONGER the system
    of record. The /state seen-set is only used as a fallback when the DB sink
    is disabled, so a Prometheus-only deploy still dedups across restarts.
    """
    d = _api("/me/player/recently-played", {"limit": 50})
    if not d or not d.get("items"):
        return

    db_on = DB.enabled()
    seen: set[str] = set(state.get("seen", []))
    new_seen = list(seen)
    counts: dict = state.setdefault("counts", {})
    added = 0

    for it in d["items"]:
        played_at = it.get("played_at")
        track = it.get("track") or {}
        if not played_at or not track.get("id"):
            continue

        artists = track.get("artists", [])
        primary = (artists[0]["name"] if artists else "unknown")[:120]

        if db_on:
            track_id = _upsert_track_dimensions(track)
            # DB dedup: only count/inc when a genuinely new row landed.
            is_new = DB.insert_play(played_at, track_id) if track_id else False
            if is_new:
                added += 1
                M_DB_PLAYS_INSERTED.inc()
                M_PLAYS.labels(artist=primary).inc()
                M_PLAYS.labels(artist="__all__").inc()
        else:
            # Prometheus-only fallback: dedup against the /state seen-set.
            if played_at in seen:
                continue
            seen.add(played_at)
            new_seen.append(played_at)
            added += 1
            M_PLAYS.labels(artist=primary).inc()
            M_PLAYS.labels(artist="__all__").inc()
            counts[primary] = counts.get(primary, 0) + 1
            counts["__all__"] = counts.get("__all__", 0) + 1

    if added and not db_on:
        # Bounded dedupe window; played_at is ISO8601 so lexical sort = chronological.
        state["seen"] = sorted(new_seen)[-500:]
        _persist_plays(state)


def poll_library(state: dict) -> None:
    """Snapshot-diff the saved-tracks library against library_tracks.

    Pages GET /me/tracks (50/page, added_at per track) and, for each page,
    upserts dimensions + a library_tracks row for any track that wasn't already
    open. Tracks no longer present get removed_at=now() at the end. Requires the
    user-library-read scope (already granted on the shared token — no re-auth).
    DB-only; no-op when the sink is disabled.

    MEMORY DISCIPLINE (this caused OOMKills at the 128Mi limit): the saved
    library is large (~4.5k tracks). The previous version hoarded EVERY track's
    full JSON payload (`page_track_payloads`) plus a payload_by_id index, then
    diffed — peak memory scaled with library size and the pod was OOMKilled
    mid-poll, so the library diff never completed and library_tracks stayed
    empty. We now stream: hold only the lightweight id->added_at map for the
    removal diff, fetch the open-saved set ONCE up front, and upsert
    dimensions/library rows per page so at most one page (50) of payloads is
    resident at a time.
    """
    if not DB.enabled():
        return

    # Snapshot the currently-open saved ids ONCE; everything new this run is
    # diffed against it without re-querying per page.
    previously_saved = DB.current_saved_track_ids()

    current_added: dict[str, str] = {}  # track_id -> added_at (lightweight)
    new_count = 0
    offset = 0
    while True:
        d = _api("/me/tracks", {"limit": 50, "offset": offset})
        items = (d or {}).get("items") or []
        if not items:
            break
        for it in items:
            track = it.get("track") or {}
            tid = track.get("id")
            if not tid:
                continue
            added_at = it.get("added_at")
            current_added[tid] = added_at
            # Only do the (heavier) dimension + library upsert for tracks that
            # weren't already open — keeps writes and memory bounded to deltas.
            if tid not in previously_saved:
                _upsert_track_dimensions(track)
                DB.library_add(tid, added_at)
                new_count += 1
        # Spotify returns `next`: null when paging is done.
        if not (d or {}).get("next"):
            break
        offset += 50
        if offset > 20000:  # safety cap (~20k tracks)
            break

    if not current_added:
        return

    now_saved = set(current_added.keys())
    no_longer_saved = previously_saved - now_saved
    if no_longer_saved:
        DB.library_remove(no_longer_saved)

    M_LIBRARY_SAVED.set(len(now_saved))
    print(
        f"library diff: {len(now_saved)} saved "
        f"(+{new_count} new, -{len(no_longer_saved)} removed)",
        flush=True,
    )


def poll_enrichment_stats() -> None:
    """Publish genre-enrichment backlog/coverage gauges from the DB.

    Cheap aggregate query; the enrichment CronJob writes the underlying
    music_ids / track_genres rows. Degrades silently (gauges unset) if the
    enrichment tables don't exist yet (pre-migration-002 DB).
    """
    if not DB.enabled():
        return
    stats = DB.enrichment_stats()
    if not stats:
        return
    if stats.get("matched") is not None:
        M_ENRICH_MATCHED.set(stats["matched"])
    if stats.get("nomatch") is not None:
        M_ENRICH_NOMATCH.set(stats["nomatch"])
    if stats.get("backlog") is not None:
        M_ENRICH_BACKLOG.set(stats["backlog"])
    if stats.get("tracks_with_mb_tags") is not None:
        M_ENRICH_TAGGED.set(stats["tracks_with_mb_tags"])
    if stats.get("untagged_old_filled") is not None:
        M_ENRICH_OLD_FILLED.set(stats["untagged_old_filled"])


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
    # Track per-artist counts for restart re-seeding (Prometheus-only fallback).
    if "counts" not in plays_state:
        plays_state["counts"] = {}

    # Seed the genre rollup table once on startup (idempotent UPSERT). The DB
    # layer no-ops gracefully if the sink is disabled.
    if DB.enabled():
        version, mappings = genre_rollup_mod.load()
        DB.seed_genre_rollup(mappings)
        print(f"genre_rollup: loaded v{version} ({len(mappings)} mappings)", flush=True)
    else:
        print("db: SPOTIFY_DB_DSN/PGHOST unset — running Prometheus-only", flush=True)

    start_http_server(PORT, registry=REG)
    print(f"spotify-exporter listening on :{PORT}/metrics", flush=True)

    last_recent = 0.0
    last_top = 0.0
    # last_library = 0.0 means `now - last_library` (a full unix epoch) >>
    # LIBRARY_INTERVAL on the first cycle, so the library snapshot runs ONCE
    # shortly after boot and then every LIBRARY_INTERVAL (default 12h) — it does
    # NOT wait a full interval before its first run. This is what populates
    # library_tracks promptly; previously the poll fired but the pod OOMKilled
    # mid-snapshot (see poll_library's memory note), so it never completed.
    last_library = 0.0
    last_enrich_stats = 0.0
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
            if now - last_library >= LIBRARY_INTERVAL:
                poll_library(plays_state)
                last_library = now
            if now - last_enrich_stats >= ENRICH_STATS_INTERVAL:
                poll_enrichment_stats()
                last_enrich_stats = now
        except Exception as exc:  # noqa: BLE001 — never let the loop die
            M_ERRORS.inc()
            cycle_ok = False
            print(f"poll error: {type(exc).__name__}: {exc}", flush=True)
        M_UP.set(1 if cycle_ok else 0)
        M_DB_UP.set(1 if (DB.enabled() and DB._connect() is not None) else 0)
        time.sleep(NOW_PLAYING_INTERVAL)


if __name__ == "__main__":
    main()
