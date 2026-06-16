#!/usr/bin/env python3
"""MusicBrainz genre enrichment backfill for the Spotify analytics pipeline.

Run as a CronJob (same image as the exporter; reuses db.py). For each track
that still lacks resolved external IDs, resolve it against MusicBrainz and write
the results into `music_ids` (resolved MBIDs, permanent) and `track_genres` /
`artist_genres_enriched` (FINE sub-genre tags — MusicBrainz curated genres plus
folksonomy tags). The downstream `track_genre_effective` VIEW (migration 002)
then surfaces the de-duped UNION of those fine sub-genres per track WITHOUT
collapsing them to coarse parents.

WHY MusicBrainz only, and WHY it needs ZERO user steps:
  MusicBrainz requires NO API key / signup — only a descriptive User-Agent with
  a contact address (policy), and a hard 1 req/s rate limit. So this job runs
  immediately on deploy with no secret to provision. The headline win is OLD /
  catalog music: Spotify frequently returns NO genres for older artists (they
  come back `untagged`), and MusicBrainz fills those with fine sub-genres.

Match chain (first hit wins for ID resolution; all responding tags are stored):
  ISRC → MusicBrainz recording   (match_method='isrc')   — the primary path
  artist + release year → MB     (match_method='artist_year') — ISRC-less fallback
In BOTH cases, once we have an artist MBID we also pull artist-level genres/tags
so every track by that artist inherits sub-genres even if the recording had none.

Pacing: a single token bucket enforces 1 req/s. The run has a bounded budget
($ENRICH_MAX_TRACKS, default 300) and a wall-clock cap ($ENRICH_MAX_SECONDS,
default 3000 = 50min); it drains the active-library-first candidate queue then
exits 0 (the CronJob re-runs on the next schedule). All writes are idempotent
(ON CONFLICT), so a killed run loses nothing.

Run-summary counts are printed as a structured line; the durable backlog /
coverage gauges are published continuously by the exporter (it reads the same
music_ids / track_genres tables), so no pushgateway is needed.
"""
from __future__ import annotations

import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Optional

from db import DB_INSTANCE as DB

# ── Config (all env-overridable) ─────────────────────────────────────────────
MAX_TRACKS = int(os.environ.get("ENRICH_MAX_TRACKS", "300"))
MAX_SECONDS = int(os.environ.get("ENRICH_MAX_SECONDS", "3000"))
# A contact in the UA is REQUIRED by MusicBrainz policy. Override with a real
# address via $ENRICH_CONTACT.
CONTACT = os.environ.get("ENRICH_CONTACT", "hampton888@gmail.com")
_UA = f"homelab-spotify-enrich/1.0 ( {CONTACT} )"
_HTTP_TIMEOUT = 15

MB_API = "https://musicbrainz.org/ws/2"

# Cache artist MBID → artist tags within a single run so a 50-track album by one
# artist costs one artist lookup, not fifty.
_ARTIST_TAG_CACHE: dict[str, list[tuple[str, str, float]]] = {}


# ── Token bucket: simplest correct pacing (MusicBrainz = 1 req/s) ────────────
class RateLimiter:
    """Block until the next request is allowed. min_interval seconds between
    requests (1.05 for MusicBrainz = a hair over 1 req/s, for safety)."""

    def __init__(self, min_interval: float) -> None:
        self.min_interval = min_interval
        self._last = 0.0

    def wait(self) -> None:
        now = time.monotonic()
        delta = now - self._last
        if delta < self.min_interval:
            time.sleep(self.min_interval - delta)
        self._last = time.monotonic()


MB_LIMIT = RateLimiter(1.05)


def _get_json(url: str) -> Optional[dict]:
    req = urllib.request.Request(url, method="GET")
    req.add_header("User-Agent", _UA)
    req.add_header("Accept", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=_HTTP_TIMEOUT) as r:
            if r.getcode() != 200:
                return None
            return json.loads(r.read())
    except urllib.error.HTTPError as e:
        if e.code == 503:  # MB throttle / transient — caller backs off
            time.sleep(2)
        return None
    except (urllib.error.URLError, OSError, TimeoutError, ValueError):
        return None


def _tags_from(obj: dict) -> list[tuple[str, str, float]]:
    """Pull both curated `genres` and folksonomy `tags` off an MB entity. Both
    are FINE sub-genres; tag_kind records which (genre|tag)."""
    out: list[tuple[str, str, float]] = []
    for g in (obj.get("genres") or []):
        nm = g.get("name")
        if nm:
            out.append((nm, "genre", float(g.get("count") or 1)))
    for tg in (obj.get("tags") or []):
        nm = tg.get("name")
        if nm:
            out.append((nm, "tag", float(tg.get("count") or 1)))
    return out


# ── MusicBrainz lookups (no auth) ────────────────────────────────────────────
def mb_by_isrc(isrc: str) -> Optional[dict]:
    """ISRC → recording MBID + artist MBID + release-group + fine genre/tags.

    /ws/2/isrc/{isrc}?inc=artist-credits+release-groups+tags+genres
    """
    MB_LIMIT.wait()
    inc = "artist-credits+release-groups+tags+genres"
    url = (f"{MB_API}/isrc/{urllib.parse.quote(isrc)}"
           f"?fmt=json&inc={urllib.parse.quote(inc)}")
    d = _get_json(url)
    if not d:
        return None
    recordings = d.get("recordings") or []
    if not recordings:
        return None
    rec = recordings[0]
    out: dict = {
        "mb_recording_id": rec.get("id"),
        "mb_artist_id": None,
        "mb_releasegroup_id": None,
        "tags": _tags_from(rec),
    }
    credits = rec.get("artist-credit") or []
    if credits:
        artist = (credits[0] or {}).get("artist") or {}
        out["mb_artist_id"] = artist.get("id")
    rgs = rec.get("release-groups") or []
    if rgs:
        out["mb_releasegroup_id"] = rgs[0].get("id")
    return out


def mb_by_artist_year(artist: str, year: Optional[int]) -> Optional[dict]:
    """ISRC-less fallback: search recordings by artist name (+ release year) and
    take the best recording → recording/artist MBID + fine tags."""
    if not artist:
        return None
    MB_LIMIT.wait()
    q = f'artist:"{artist}"'
    if year:
        q += f" AND firstreleasedate:[{year}-01-01 TO {year}-12-31]"
    url = (f"{MB_API}/recording?fmt=json&limit=1&inc=artist-credits+genres+tags"
           f"&query={urllib.parse.quote(q)}")
    d = _get_json(url)
    recs = (d or {}).get("recordings") or []
    if not recs:
        return None
    rec = recs[0]
    out: dict = {
        "mb_recording_id": rec.get("id"),
        "mb_artist_id": None,
        "mb_releasegroup_id": None,
        "tags": _tags_from(rec),
    }
    credits = rec.get("artist-credit") or []
    if credits:
        out["mb_artist_id"] = (credits[0] or {}).get("artist", {}).get("id")
    return out


def mb_artist_tags(mb_artist_id: str) -> list[tuple[str, str, float]]:
    """Artist-level fine genres/tags (run-cached). Mapped onto every track by
    the artist by the track_genre_effective view, so a single artist lookup
    fills the artist's whole catalog."""
    if not mb_artist_id:
        return []
    if mb_artist_id in _ARTIST_TAG_CACHE:
        return _ARTIST_TAG_CACHE[mb_artist_id]
    MB_LIMIT.wait()
    url = (f"{MB_API}/artist/{urllib.parse.quote(mb_artist_id)}"
           f"?fmt=json&inc=tags+genres")
    d = _get_json(url)
    tags = _tags_from(d) if d else []
    _ARTIST_TAG_CACHE[mb_artist_id] = tags
    return tags


# ── Per-track resolution ─────────────────────────────────────────────────────
def resolve_track(row: dict) -> str:
    """Resolve one candidate. Returns a short method code for the run summary
    ('isrc', 'artist_year', 'nomatch')."""
    tid = row["track_id"]
    artist_id = row.get("artist_id")
    isrc = (row.get("isrc") or "").strip() or None
    artist = (row.get("artist_name") or "").strip()
    year = row.get("year")

    mb: Optional[dict] = None
    method: Optional[str] = None

    # 1) ISRC → MusicBrainz (the primary, highest-confidence path).
    if isrc:
        mb = mb_by_isrc(isrc)
        if mb:
            method = "isrc"

    # 2) Fallback: artist + release year (covers ISRC-less / old catalog).
    if mb is None and artist:
        mb = mb_by_artist_year(artist, year)
        if mb:
            method = "artist_year"

    if mb is None:
        DB.mark_music_ids_status(tid, "nomatch")
        return "nomatch"

    # Recording-level fine tags.
    tags_written = DB.add_track_tags(tid, "musicbrainz", mb.get("tags") or [])

    # Artist-level fine tags (fills tracks whose recording had none, and every
    # other track by the same artist via the view). Keyed on the SPOTIFY
    # artist_id so the view can join artist_genres_enriched.artist_id = tracks.artist_id.
    mb_artist_id = mb.get("mb_artist_id")
    if mb_artist_id and artist_id:
        atags = mb_artist_tags(mb_artist_id)
        if atags:
            DB.add_artist_tags(artist_id, "musicbrainz", atags)
            tags_written += len(atags)

    DB.upsert_music_ids(
        tid,
        isrc=isrc,
        mb_recording_id=mb.get("mb_recording_id"),
        mb_artist_id=mb_artist_id,
        mb_releasegroup_id=mb.get("mb_releasegroup_id"),
        match_method=method,
        match_score=1.0 if method == "isrc" else 0.6,
        match_status="matched" if tags_written else "nomatch",
    )
    return method if tags_written else "nomatch"


def main() -> None:
    if not DB.enabled():
        print("enrich: DB sink disabled (no PGHOST/DSN) — nothing to do",
              flush=True)
        return

    print(
        f"enrich: starting | source=musicbrainz (no API key required) "
        f"contact={CONTACT} | budget={MAX_TRACKS} tracks / {MAX_SECONDS}s",
        flush=True,
    )

    candidates = DB.enrichment_candidates(MAX_TRACKS)
    print(f"enrich: {len(candidates)} candidate tracks "
          f"(active-library-first)", flush=True)

    deadline = time.monotonic() + MAX_SECONDS
    counts = {"isrc": 0, "artist_year": 0, "nomatch": 0, "error": 0}
    processed = 0
    for row in candidates:
        if time.monotonic() > deadline:
            print("enrich: wall-clock budget reached; stopping (resumes next run)",
                  flush=True)
            break
        try:
            method = resolve_track(row)
            counts[method] = counts.get(method, 0) + 1
        except Exception as exc:  # noqa: BLE001 — one bad track must not abort the run
            counts["error"] += 1
            print(f"enrich: track {row.get('track_id')} failed: "
                  f"{type(exc).__name__}: {exc}", flush=True)
            try:
                DB.mark_music_ids_status(row["track_id"], "error")
            except Exception:  # noqa: BLE001
                pass
        processed += 1
        if processed % 25 == 0:
            print(f"enrich: progress {processed}/{len(candidates)} {counts}",
                  flush=True)

    stats = DB.enrichment_stats()
    # Structured one-liner the exporter's gauges corroborate continuously.
    print(
        "enrich: run complete | "
        f"processed={processed} matched_isrc={counts['isrc']} "
        f"artist_year={counts['artist_year']} "
        f"nomatch={counts['nomatch']} error={counts['error']} | "
        f"db_matched={stats.get('matched')} db_backlog={stats.get('backlog')} "
        f"tracks_with_mb_tags={stats.get('tracks_with_mb_tags')} "
        f"untagged_old_filled={stats.get('untagged_old_filled')}",
        flush=True,
    )


if __name__ == "__main__":
    main()
