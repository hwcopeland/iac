#!/usr/bin/env python3
"""One-time backfill: Spotify "Extended Streaming History" (GDPR export) -> Postgres.

Spotify's privacy export (Account -> Privacy -> "Extended streaming history",
arrives ~30 days after request) is a set of JSON files named like
`Streaming_History_Audio_2014-2015_0.json` (newer exports) or `endsong_0.json`
(older exports). Each file is a JSON ARRAY of play records:

    {
      "ts": "2021-06-01T12:34:56Z",            # end-of-play timestamp (UTC, ISO8601)
      "ms_played": 207000,                      # how long the track actually played
      "master_metadata_track_name": "Song",     # NULL for podcast episodes
      "master_metadata_album_artist_name": "Artist",
      "master_metadata_album_album_name": "Album",
      "spotify_track_uri": "spotify:track:6rqhFgbbKwnb9MLmUQDhG6",
      "reason_start": "trackdone",
      "reason_end": "trackdone",
      "shuffle": true,
      "skipped": false,
      "offline": false,
      ...
    }

This script is IDEMPOTENT and safe to re-run, and safe to run ALONGSIDE the live
exporter:

  * play_events PK is (played_at, track_id) -> INSERT ... ON CONFLICT DO NOTHING.
    The live exporter writes the SAME key from /recently-played, so overlapping
    plays simply collide and are skipped. Re-running this importer is a no-op.
  * Dimension upserts (tracks) use ON CONFLICT (track_id) DO UPDATE with COALESCE,
    so they never clobber richer data the live exporter may have written from the
    API (popularity, artist_id, album_id, genres).

What the GDPR export DOES and does NOT give us
----------------------------------------------
  * track_id: derived from `spotify_track_uri` (the part after `spotify:track:`).
  * ms_played: present -> stored on play_events.ms_played. This is the column the
    live /recently-played feed CANNOT provide, so the backfill is what unlocks
    skip-rate / completion-rate analysis.
  * skipped / reason_end: the export has these per play, but play_events has no
    column for them. We DERIVE a conservative `ms_played` (already in the export)
    and leave skip classification to the dashboards, which can threshold on
    ms_played vs tracks.duration_ms (e.g. played < 30s or < 50% of duration = skip).
  * artist / album: the export gives only NAMES, not Spotify IDs. tracks.artist_id
    and tracks.album_id are FKs to artists(artist_id)/albums(album_id) keyed by
    Spotify ID, which we don't have here. We therefore leave them NULL and let the
    live exporter (or a future API enrichment pass) fill them in when it next sees
    the track via /recently-played or /me/tracks. The track NAME is still stored so
    panels that join only tracks render immediately.

Connection: same env contract as db.py (SPOTIFY_DB_DSN, or PG* vars). The Job
manifest wires PGHOST/PGDATABASE/PGUSER + PGPASSWORD (APP_PASSWORD) identically
to the exporter Deployment, so this runs as the read/write `spotify_app` role.

Usage:
    python import_streaming_history.py /import        # dir of *.json export files
    python import_streaming_history.py /import/endsong_0.json   # single file
    SPOTIFY_IMPORT_DIR=/import python import_streaming_history.py
"""
from __future__ import annotations

import glob
import json
import os
import sys
from typing import Iterator, Optional

from db import DB_INSTANCE as DB

_TRACK_URI_PREFIX = "spotify:track:"


def _track_id_from_uri(uri: Optional[str]) -> Optional[str]:
    """`spotify:track:<id>` -> `<id>`; anything else (episodes, ads, None) -> None."""
    if not uri or not uri.startswith(_TRACK_URI_PREFIX):
        return None
    tid = uri[len(_TRACK_URI_PREFIX):].strip()
    return tid or None


def _iter_files(path: str) -> Iterator[str]:
    """Yield the export JSON files under `path` (a dir or a single file)."""
    if os.path.isdir(path):
        # Match both modern (Streaming_History_Audio_*) and legacy (endsong_*).
        pats = ("Streaming_History_Audio_*.json", "endsong_*.json", "*.json")
        seen: set[str] = set()
        for pat in pats:
            for f in sorted(glob.glob(os.path.join(path, pat))):
                if f not in seen:
                    seen.add(f)
                    yield f
        return
    yield path


def _iter_records(file_path: str) -> Iterator[dict]:
    """Yield play-record dicts from one export file (a top-level JSON array)."""
    with open(file_path, "r", encoding="utf-8") as f:
        data = json.load(f)
    if isinstance(data, dict):  # be liberal: some exports wrap the array
        data = data.get("items") or data.get("plays") or []
    if not isinstance(data, list):
        return
    for rec in data:
        if isinstance(rec, dict):
            yield rec


def import_path(path: str) -> tuple[int, int, int]:
    """Import every play under `path`. Returns (read, inserted, skipped_non_track)."""
    read = inserted = skipped = 0
    # Dedup tracks we've already upserted this run (cheap; the export repeats them
    # thousands of times) to avoid hammering the DB with redundant UPSERTs.
    upserted_tracks: set[str] = set()

    for file_path in _iter_files(path):
        print(f"import: reading {file_path}", flush=True)
        file_read = file_inserted = 0
        for rec in _iter_records(file_path):
            read += 1
            file_read += 1

            ts = rec.get("ts")
            track_uri = rec.get("spotify_track_uri")
            track_name = rec.get("master_metadata_track_name")
            track_id = _track_id_from_uri(track_uri)

            # Podcast episodes / local files / ads have a null track name or a
            # non-track URI -> not a music play we can key. Skip them.
            if not ts or not track_id or not track_name:
                skipped += 1
                continue

            ms_played = rec.get("ms_played")
            if not isinstance(ms_played, int):
                ms_played = None

            # Dimension: store the track NAME so name-joined panels work even
            # before the API enrichment links artist_id/album_id. artist_id and
            # album_id are NULL here (the export has names only, not IDs).
            if track_id not in upserted_tracks:
                DB.upsert_track(
                    track_id=track_id,
                    name=track_name,
                    artist_id=None,
                    album_id=None,
                    duration_ms=None,
                    popularity=None,
                )
                upserted_tracks.add(track_id)

            # Fact: one play. ON CONFLICT (played_at, track_id) DO NOTHING makes
            # this idempotent and safe next to the live /recently-played writer.
            # ms_played from the export is the value /recently-played can't give,
            # so this is what makes skip/completion analysis possible.
            if DB.insert_play(ts, track_id, ms_played):
                inserted += 1
                file_inserted += 1

        print(
            f"import: {file_path} -> read {file_read}, inserted {file_inserted}",
            flush=True,
        )

    return read, inserted, skipped


def main() -> int:
    path = (
        sys.argv[1]
        if len(sys.argv) > 1
        else os.environ.get("SPOTIFY_IMPORT_DIR", "/import")
    )
    if not os.path.exists(path):
        print(f"import: path not found: {path}", flush=True)
        return 2

    if not DB.enabled():
        print(
            "import: DB sink not configured (set SPOTIFY_DB_DSN or PG* env). "
            "Refusing to run a backfill with nowhere to write.",
            flush=True,
        )
        return 3

    print(f"import: starting backfill from {path}", flush=True)
    read, inserted, skipped = import_path(path)
    print(
        f"import: DONE. records read={read}, plays inserted={inserted} "
        f"(new), non-track/podcast skipped={skipped}, "
        f"duplicates ignored={read - inserted - skipped}",
        flush=True,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
