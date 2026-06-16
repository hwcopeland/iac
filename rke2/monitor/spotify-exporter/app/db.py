"""Postgres analytics sink for the Spotify exporter (psycopg3).

The durable record of listening history now lives in Postgres, not the
`/state` PVC. This module owns the connection and all writes/reads against the
schema deployed by data-engineer under
`rke2/monitor/spotify-postgres/` (the DDL there is the AUTHORITATIVE contract —
column names below mirror it; reconcile here if anything diverges).

Tables written by this exporter:
  play_events(played_at, track_id, ms_played NULL)        — one row per play
  tracks(track_id, name, artist_id, album_id, duration_ms,
         popularity, energy/danceability/tempo/valence NULL,
         audio_features_fetched_at NULL)                   — dimension upsert
  artists(artist_id, name, popularity, genres[])           — dimension upsert
  albums(album_id, name, release_date)                     — dimension upsert
  library_tracks(track_id, added_at, removed_at NULL)      — saved-library diff
  genre_rollup(raw_genre, parent_genre)                    — seeded from yaml

Audio-feature columns (energy/danceability/tempo/valence,
audio_features_fetched_at) are left NULL: the /audio-features endpoint returns
403 under Spotify's Nov-2024 deprecation for our app. Do NOT write them.

Connection: a single libpq connection string from $SPOTIFY_DB_DSN, or assembled
from PG* env (PGHOST/PGPORT/PGDATABASE/PGUSER/PGPASSWORD). deployment.yaml
inlines host/port/db/user (svc `spotify-postgres`, db `spotify`, role
`spotify_app`) and pulls only PGPASSWORD from the `spotify-postgres` Secret
(key APP_PASSWORD) — data-engineer's schema ships no host/user creds Secret.
If no DSN/PGHOST is configured, db.enabled() is False and the exporter runs in
Prometheus-only mode (degrades gracefully, never crashes the poll loop).
"""
from __future__ import annotations

import os
import time
from typing import Iterable, Optional

try:
    import psycopg
    from psycopg.rows import dict_row
    _HAVE_PSYCOPG = True
except Exception:  # noqa: BLE001 — exporter must still serve metrics without the driver
    psycopg = None  # type: ignore
    dict_row = None  # type: ignore
    _HAVE_PSYCOPG = False


def _dsn() -> Optional[str]:
    dsn = os.environ.get("SPOTIFY_DB_DSN", "").strip()
    if dsn:
        return dsn
    # Fall back to standard libpq PG* vars if a full DSN wasn't provided.
    host = os.environ.get("PGHOST", "").strip()
    if not host:
        return None
    parts = [f"host={host}"]
    for env, key in (
        ("PGPORT", "port"),
        ("PGDATABASE", "dbname"),
        ("PGUSER", "user"),
        ("PGPASSWORD", "password"),
    ):
        val = os.environ.get(env, "").strip()
        if val:
            parts.append(f"{key}={val}")
    parts.append("connect_timeout=10")
    return " ".join(parts)


def _normalize_release_date(release_date: Optional[str],
                            precision: Optional[str] = None) -> Optional[str]:
    """Coerce Spotify's release_date into a value the Postgres `date` column accepts.

    Spotify returns reduced-precision strings depending on
    `release_date_precision`:
      precision="year"  -> "1975"      (year only)
      precision="month" -> "1975-03"   (year-month)
      precision="day"   -> "1975-03-21"

    The `date` column rejects "1975" / "1975-03" (InvalidDatetimeFormat), which
    previously aborted the album insert and cascaded into dropped tracks AND
    plays via the album/track FKs. We normalize to a full ISO date by padding
    the missing components to January / the 1st:
      "1975"    -> "1975-01-01"
      "1975-03" -> "1975-03-01"
      "1975-03-21" -> unchanged

    `precision` is the authoritative signal when present; when the album payload
    omits it (some slim payloads do), we infer from the string shape (count of
    '-'-separated parts) so the normalization still holds.
    """
    if not release_date:
        return None
    rd = release_date.strip()
    if not rd:
        return None
    # Prefer the explicit precision when Spotify provides it.
    if precision == "year":
        return f"{rd[:4]}-01-01"
    if precision == "month":
        return f"{rd[:7]}-01" if len(rd) >= 7 else f"{rd[:4]}-01-01"
    if precision == "day":
        return rd
    # No (or unexpected) precision — infer from the string shape.
    parts = rd.split("-")
    if len(parts) == 1:            # "1975"
        return f"{parts[0]}-01-01"
    if len(parts) == 2:           # "1975-03"
        return f"{parts[0]}-{parts[1]}-01"
    return rd                     # already YYYY-MM-DD (or finer — pass through)


class DB:
    """Thin lazy-reconnecting wrapper. All methods are best-effort: a transient
    DB error is logged and swallowed so the Prometheus poll loop keeps running.
    """

    def __init__(self) -> None:
        self._dsn = _dsn()
        self._conn = None  # type: ignore
        # Per-id genre cache so we don't re-hit /v1/artists for every play.
        self.artist_genre_cache: dict[str, list[str]] = {}

    def enabled(self) -> bool:
        return bool(self._dsn) and _HAVE_PSYCOPG

    # ── connection plumbing ──────────────────────────────────────────────────
    def _connect(self):
        if not self.enabled():
            return None
        if self._conn is not None and not self._conn.closed:
            return self._conn
        try:
            self._conn = psycopg.connect(self._dsn, autocommit=True, row_factory=dict_row)
            print("db: connected to postgres", flush=True)
        except Exception as exc:  # noqa: BLE001
            print(f"db: connect failed: {type(exc).__name__}: {exc}", flush=True)
            self._conn = None
        return self._conn

    def _exec(self, sql: str, params: tuple = ()) -> bool:
        conn = self._connect()
        if conn is None:
            return False
        try:
            with conn.cursor() as cur:
                cur.execute(sql, params)
            return True
        except Exception as exc:  # noqa: BLE001
            print(f"db: exec failed: {type(exc).__name__}: {exc}", flush=True)
            # Drop the connection so the next call reconnects cleanly.
            try:
                conn.close()
            finally:
                self._conn = None
            return False

    def _query(self, sql: str, params: tuple = ()) -> list[dict]:
        conn = self._connect()
        if conn is None:
            return []
        try:
            with conn.cursor() as cur:
                cur.execute(sql, params)
                return cur.fetchall()
        except Exception as exc:  # noqa: BLE001
            print(f"db: query failed: {type(exc).__name__}: {exc}", flush=True)
            try:
                conn.close()
            finally:
                self._conn = None
            return []

    # ── dimension upserts ────────────────────────────────────────────────────
    def upsert_artist(self, artist_id: str, name: str,
                      popularity: Optional[int], genres: list[str]) -> bool:
        """Upsert an artist dimension. Returns True on success (False on any
        write failure, without raising), so the caller can null the track's
        artist_id rather than let an artist hiccup cascade into a dropped play.
        """
        if not artist_id:
            return False
        return self._exec(
            """
            INSERT INTO artists (artist_id, name, popularity, genres)
            VALUES (%s, %s, %s, %s)
            ON CONFLICT (artist_id) DO UPDATE
              SET name = EXCLUDED.name,
                  popularity = COALESCE(EXCLUDED.popularity, artists.popularity),
                  genres = EXCLUDED.genres
            """,
            (artist_id, name[:300], popularity, genres or []),
        )

    def upsert_album(self, album_id: str, name: str,
                     release_date: Optional[str],
                     release_date_precision: Optional[str] = None) -> bool:
        """Upsert an album dimension. Returns True on success.

        `release_date` is normalized to a full ISO date via
        _normalize_release_date so reduced-precision values ("1975", "1975-03")
        no longer raise InvalidDatetimeFormat and abort the insert. Returns
        False (without raising) when the album cannot be written, so the caller
        can still persist the track (with album_id=NULL) and the play.
        """
        if not album_id:
            return False
        normalized = _normalize_release_date(release_date, release_date_precision)
        return self._exec(
            """
            INSERT INTO albums (album_id, name, release_date)
            VALUES (%s, %s, %s)
            ON CONFLICT (album_id) DO UPDATE
              SET name = EXCLUDED.name,
                  release_date = COALESCE(EXCLUDED.release_date, albums.release_date)
            """,
            (album_id, name[:300], normalized),
        )

    def upsert_track(self, track_id: str, name: str, artist_id: Optional[str],
                     album_id: Optional[str], duration_ms: Optional[int],
                     popularity: Optional[int]) -> bool:
        """Upsert a track dimension. Returns True on success.

        Returns False (without raising) when the row can't be written, so the
        caller can decide whether to still record the play. Pass album_id=None
        when the album dimension could not be upserted — the tracks.album_id FK
        is nullable, so a track with an unknown album still persists and keeps
        the play insertable.
        """
        if not track_id:
            return False
        # NOTE: energy/danceability/tempo/valence/audio_features_fetched_at are
        # intentionally NOT written — /audio-features is 403 (deprecated). They
        # stay NULL and are filled by a future backfill if a source appears.
        return self._exec(
            """
            INSERT INTO tracks (track_id, name, artist_id, album_id, duration_ms, popularity)
            VALUES (%s, %s, %s, %s, %s, %s)
            ON CONFLICT (track_id) DO UPDATE
              SET name = EXCLUDED.name,
                  artist_id = COALESCE(EXCLUDED.artist_id, tracks.artist_id),
                  album_id = COALESCE(EXCLUDED.album_id, tracks.album_id),
                  duration_ms = COALESCE(EXCLUDED.duration_ms, tracks.duration_ms),
                  popularity = COALESCE(EXCLUDED.popularity, tracks.popularity)
            """,
            (track_id, name[:500], artist_id, album_id, duration_ms, popularity),
        )

    # ── play events ──────────────────────────────────────────────────────────
    def insert_play(self, played_at: str, track_id: str,
                    ms_played: Optional[int] = None) -> bool:
        """Insert one play. Returns True if a NEW row was inserted (dedup signal).

        ms_played is NULL: recently-played gives no per-play listen duration.
        """
        if not played_at or not track_id:
            return False
        rows = self._query(
            """
            INSERT INTO play_events (played_at, track_id, ms_played)
            VALUES (%s, %s, %s)
            ON CONFLICT (played_at, track_id) DO NOTHING
            RETURNING played_at
            """,
            (played_at, track_id, ms_played),
        )
        return len(rows) > 0

    # ── library snapshot diff ────────────────────────────────────────────────
    def current_saved_track_ids(self) -> set[str]:
        """track_ids currently saved (removed_at IS NULL)."""
        rows = self._query(
            "SELECT track_id FROM library_tracks WHERE removed_at IS NULL"
        )
        return {r["track_id"] for r in rows}

    def library_add(self, track_id: str, added_at: str) -> None:
        """Mark a track saved (open a row).

        The deployed schema enforces "at most one open (removed_at IS NULL) row
        per track" via the PARTIAL unique index `library_tracks_open_uidx`
        (... WHERE removed_at IS NULL). There is NO plain unique constraint on
        track_id, so the ON CONFLICT clause MUST repeat that index predicate
        (`WHERE removed_at IS NULL`) to select the partial index as the conflict
        arbiter — otherwise Postgres raises "no unique or exclusion constraint
        matching the ON CONFLICT specification". The exporter only ever calls
        this for tracks not currently open (newly_saved), so the UPDATE branch
        just refreshes added_at on the existing open row if a race re-adds it.
        """
        if not track_id:
            return
        self._exec(
            """
            INSERT INTO library_tracks (track_id, added_at, removed_at)
            VALUES (%s, %s, NULL)
            ON CONFLICT (track_id) WHERE removed_at IS NULL DO UPDATE
              SET added_at = EXCLUDED.added_at
            """,
            (track_id, added_at),
        )

    def library_remove(self, track_ids: Iterable[str]) -> None:
        ids = [t for t in track_ids if t]
        if not ids:
            return
        self._exec(
            """
            UPDATE library_tracks
               SET removed_at = now()
             WHERE removed_at IS NULL
               AND track_id = ANY(%s)
            """,
            (ids,),
        )

    # ── genre rollup seed ────────────────────────────────────────────────────
    def seed_genre_rollup(self, mappings: dict[str, str]) -> None:
        """Idempotent UPSERT of raw_genre -> parent_genre."""
        if not mappings:
            return
        conn = self._connect()
        if conn is None:
            return
        try:
            with conn.cursor() as cur:
                cur.executemany(
                    """
                    INSERT INTO genre_rollup (raw_genre, parent_genre)
                    VALUES (%s, %s)
                    ON CONFLICT (raw_genre) DO UPDATE
                      SET parent_genre = EXCLUDED.parent_genre
                    """,
                    [(raw.lower(), parent) for raw, parent in mappings.items()],
                )
            print(f"db: seeded {len(mappings)} genre_rollup rows", flush=True)
        except Exception as exc:  # noqa: BLE001
            print(f"db: genre seed failed: {type(exc).__name__}: {exc}", flush=True)


# Module-level singleton, mirroring the exporter's other globals.
DB_INSTANCE = DB()
