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
                     popularity: Optional[int],
                     isrc: Optional[str] = None,
                     artist_name: Optional[str] = None,
                     album_name: Optional[str] = None) -> bool:
        """Upsert a track dimension. Returns True on success.

        Returns False (without raising) when the row can't be written, so the
        caller can decide whether to still record the play. Pass album_id=None
        when the album dimension could not be upserted — the tracks.album_id FK
        is nullable, so a track with an unknown album still persists and keeps
        the play insertable.

        `isrc` (International Standard Recording Code) is captured from the
        Spotify track payload's `external_ids.isrc`. It is the JOIN KEY for the
        downstream genre-enrichment pipeline (ISRC → MusicBrainz recording →
        genre/style tags), so it MUST be populated when present. COALESCE keeps a
        previously-written isrc if a later slim payload omits external_ids.

        `artist_name` / `album_name` are the GDPR export's
        master_metadata_album_artist_name / _album_album_name strings (migration
        005). They are stored DENORMALIZED on the track so a track with NO
        resolvable Spotify artist_id (the ~51% of import rows that previously
        lost their artist entirely) still carries a usable artist identity:
          * genre enrichment matches them by NAME → MusicBrainz, and
          * dashboards rank them via COALESCE(artists.name, tracks.artist_name).
        COALESCE on UPDATE means a NULL never erases a previously-stored name, so
        re-running the importer BACKFILLS names onto existing rows and the live
        exporter (which passes them NULL) never clobbers them.
        """
        if not track_id:
            return False
        # NOTE: energy/danceability/tempo/valence/audio_features_fetched_at are
        # intentionally NOT written — /audio-features is 403 (deprecated). They
        # stay NULL and are filled by a future backfill if a source appears.
        return self._exec(
            """
            INSERT INTO tracks (track_id, name, artist_id, album_id, duration_ms,
                                popularity, isrc, artist_name, album_name)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
            ON CONFLICT (track_id) DO UPDATE
              SET name = EXCLUDED.name,
                  artist_id = COALESCE(EXCLUDED.artist_id, tracks.artist_id),
                  album_id = COALESCE(EXCLUDED.album_id, tracks.album_id),
                  duration_ms = COALESCE(EXCLUDED.duration_ms, tracks.duration_ms),
                  popularity = COALESCE(EXCLUDED.popularity, tracks.popularity),
                  isrc = COALESCE(EXCLUDED.isrc, tracks.isrc),
                  artist_name = COALESCE(EXCLUDED.artist_name, tracks.artist_name),
                  album_name = COALESCE(EXCLUDED.album_name, tracks.album_name)
            """,
            (track_id, name[:500], artist_id, album_id, duration_ms, popularity,
             isrc, (artist_name[:300] if artist_name else None),
             (album_name[:300] if album_name else None)),
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

    def backfill_play(
        self,
        played_at: str,
        track_id: str,
        ms_played: Optional[int] = None,
        *,
        skipped: Optional[bool] = None,
        reason_start: Optional[str] = None,
        reason_end: Optional[str] = None,
        shuffle: Optional[bool] = None,
        platform: Optional[str] = None,
        conn_country: Optional[str] = None,
        offline: Optional[bool] = None,
        incognito_mode: Optional[bool] = None,
    ) -> bool:
        """Insert/backfill one play WITH the rich GDPR-export fields.

        Unlike the live `insert_play` (DO NOTHING — used by /recently-played,
        which carries none of these columns), this is the one-time GDPR-export
        importer's writer. It uses ON CONFLICT (played_at, track_id) DO UPDATE so
        re-running the import BACKFILLS the rich columns onto rows that were
        already inserted (by a prior import run or by the live exporter), not
        just brand-new inserts.

        The rich fields (skipped/reason_start/reason_end/shuffle/platform/
        conn_country/offline/incognito_mode) exist ONLY in Spotify's "Extended
        Streaming History" export — the live /recently-played feed cannot supply
        them, so future live rows leave them NULL. COALESCE on UPDATE keeps any
        value already present (export is authoritative here, but COALESCE means a
        NULL in the export never erases a value), and ms_played is likewise only
        overwritten when the export provides one.

        Returns True if a NEW row was inserted (vs. an UPDATE/backfill of an
        existing row), so the caller can report fresh inserts separately.
        """
        if not played_at or not track_id:
            return False
        rows = self._query(
            """
            INSERT INTO play_events (
                played_at, track_id, ms_played,
                skipped, reason_start, reason_end, shuffle,
                platform, conn_country, offline, incognito_mode
            )
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
            ON CONFLICT (played_at, track_id) DO UPDATE SET
                ms_played      = COALESCE(EXCLUDED.ms_played,      play_events.ms_played),
                skipped        = COALESCE(EXCLUDED.skipped,        play_events.skipped),
                reason_start   = COALESCE(EXCLUDED.reason_start,   play_events.reason_start),
                reason_end     = COALESCE(EXCLUDED.reason_end,     play_events.reason_end),
                shuffle        = COALESCE(EXCLUDED.shuffle,        play_events.shuffle),
                platform       = COALESCE(EXCLUDED.platform,       play_events.platform),
                conn_country   = COALESCE(EXCLUDED.conn_country,   play_events.conn_country),
                offline        = COALESCE(EXCLUDED.offline,        play_events.offline),
                incognito_mode = COALESCE(EXCLUDED.incognito_mode, play_events.incognito_mode)
            RETURNING (xmax = 0) AS inserted
            """,
            (
                played_at, track_id, ms_played,
                skipped, reason_start, reason_end, shuffle,
                platform, conn_country, offline, incognito_mode,
            ),
        )
        return bool(rows and rows[0].get("inserted"))

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
    def seed_genre_rollup(self, mappings: dict[str, str],
                          source: str = "spotify") -> None:
        """Idempotent UPSERT of raw_genre -> parent_genre, tagged with a source.

        The rollup is used ONLY for optional parent grouping / drill-down — the
        fine sub-genres are never replaced by their parent in the primary view.
        `source` records provenance ('spotify' for the curated Spotify-tag map).
        The `genre_rollup.source` column is added by migration 002; this writer
        degrades to a sourceless insert if the column is absent (pre-002 DB) so
        it never crashes a Prometheus-only / unmigrated deploy.
        """
        if not mappings:
            return
        conn = self._connect()
        if conn is None:
            return
        rows = [(raw.lower(), parent, source) for raw, parent in mappings.items()]
        try:
            with conn.cursor() as cur:
                cur.executemany(
                    """
                    INSERT INTO genre_rollup (raw_genre, parent_genre, source)
                    VALUES (%s, %s, %s)
                    ON CONFLICT (raw_genre) DO UPDATE
                      SET parent_genre = EXCLUDED.parent_genre,
                          source = EXCLUDED.source
                    """,
                    rows,
                )
            print(f"db: seeded {len(mappings)} genre_rollup rows "
                  f"(source={source})", flush=True)
        except Exception:  # noqa: BLE001 — likely pre-002 (no source column)
            # Reconnect (the failed txn poisoned the connection) and retry without
            # the source column so a not-yet-migrated DB still seeds.
            try:
                conn.close()
            finally:
                self._conn = None
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
                        [(r[0], r[1]) for r in rows],
                    )
                print(f"db: seeded {len(mappings)} genre_rollup rows "
                      f"(no source col)", flush=True)
            except Exception as exc:  # noqa: BLE001
                print(f"db: genre seed failed: {type(exc).__name__}: {exc}",
                      flush=True)

    # ── enrichment: candidate selection ──────────────────────────────────────
    def enrichment_candidates(self, limit: int) -> list[dict]:
        """Tracks needing ID resolution, ACTIVE-LIBRARY-FIRST.

        Orders by: (a) is the track in play_events or the open library? then
        (b) most-recently active. A track is a candidate when it has NO
        music_ids row, or its row is still 'unmatched' (so a previous partial
        run is retried). Returns the columns the resolver needs: track_id, isrc,
        track name, artist name, primary release year.

        `artist_name` is COALESCE(linked artists.name, tracks.artist_name): the
        ~51% of tracks with NO resolved artist_id (artists.name is NULL) fall
        back to the GDPR export's denormalized name (migration 005). Without
        this, those rows returned a NULL artist and the artist-name → MusicBrainz
        fallback skipped them — the exact cause of their missing genres. With it,
        a link-less track is still matchable by name (the ISRC-less path).
        """
        return self._query(
            """
            SELECT t.track_id,
                   t.isrc,
                   t.artist_id,
                   t.name                       AS track_name,
                   COALESCE(a.name, t.artist_name) AS artist_name,
                   EXTRACT(year FROM al.release_date)::int AS year,
                   (lt.track_id IS NOT NULL
                    OR pe.track_id IS NOT NULL) AS active
            FROM tracks t
            LEFT JOIN artists a               ON a.artist_id = t.artist_id
            LEFT JOIN albums  al              ON al.album_id  = t.album_id
            LEFT JOIN music_ids mi            ON mi.track_id  = t.track_id
            LEFT JOIN library_tracks lt
                   ON lt.track_id = t.track_id AND lt.removed_at IS NULL
            LEFT JOIN LATERAL (
                   SELECT track_id FROM play_events pe2
                    WHERE pe2.track_id = t.track_id LIMIT 1
            ) pe ON true
            WHERE mi.track_id IS NULL
               OR mi.match_status = 'unmatched'
               -- One-shot retry of the link-less backlog: tracks previously
               -- marked 'nomatch' purely because they had NO artist to match on
               -- (artist_id NULL AND no recovered name) but that NOW carry a
               -- recovered tracks.artist_name (migration 005 backfill) and still
               -- have no MusicBrainz genre tags. This drains the ~4.7k tracks
               -- that the artist-name fallback can newly resolve, WITHOUT
               -- re-hammering genuinely-unmatchable rows (those keep their
               -- 'nomatch' once they've been tried WITH a name and a tag exists).
               OR (mi.match_status = 'nomatch'
                   AND t.artist_id IS NULL
                   AND t.artist_name IS NOT NULL
                   AND NOT EXISTS (SELECT 1 FROM track_genres tg2
                                    WHERE tg2.track_id = t.track_id
                                      AND tg2.source = 'musicbrainz'))
            ORDER BY active DESC, mi.track_id NULLS FIRST
            LIMIT %s
            """,
            (limit,),
        )

    def upsert_music_ids(self, track_id: str, *, isrc: Optional[str] = None,
                         mb_recording_id: Optional[str] = None,
                         mb_artist_id: Optional[str] = None,
                         mb_releasegroup_id: Optional[str] = None,
                         match_method: Optional[str] = None,
                         match_score: Optional[float] = None,
                         match_status: str = "matched") -> bool:
        """Persist resolved MusicBrainz IDs (COALESCE keeps prior non-null IDs)."""
        if not track_id:
            return False
        return self._exec(
            """
            INSERT INTO music_ids
              (track_id, isrc, mb_recording_id, mb_artist_id, mb_releasegroup_id,
               match_method, match_score, match_status, resolved_at)
            VALUES (%s,%s,%s,%s,%s,%s,%s,%s, now())
            ON CONFLICT (track_id) DO UPDATE SET
              isrc               = COALESCE(EXCLUDED.isrc, music_ids.isrc),
              mb_recording_id    = COALESCE(EXCLUDED.mb_recording_id, music_ids.mb_recording_id),
              mb_artist_id       = COALESCE(EXCLUDED.mb_artist_id, music_ids.mb_artist_id),
              mb_releasegroup_id = COALESCE(EXCLUDED.mb_releasegroup_id, music_ids.mb_releasegroup_id),
              match_method       = COALESCE(EXCLUDED.match_method, music_ids.match_method),
              match_score        = COALESCE(EXCLUDED.match_score, music_ids.match_score),
              match_status       = EXCLUDED.match_status,
              resolved_at        = now()
            """,
            (track_id, isrc, mb_recording_id, mb_artist_id, mb_releasegroup_id,
             match_method, match_score, match_status),
        )

    def mark_music_ids_status(self, track_id: str, status: str) -> bool:
        """Record a terminal non-match (nomatch/error) so it isn't retried as
        'unmatched' every run (still re-tried on a later schedule if desired)."""
        if not track_id:
            return False
        return self._exec(
            """
            INSERT INTO music_ids (track_id, match_status, resolved_at)
            VALUES (%s, %s, now())
            ON CONFLICT (track_id) DO UPDATE
              SET match_status = EXCLUDED.match_status, resolved_at = now()
            """,
            (track_id, status),
        )

    def add_track_tags(self, track_id: str, source: str,
                       tags: Iterable[tuple[str, str, Optional[float]]]) -> int:
        """Insert (raw_tag, tag_kind, weight) tags for a track. Idempotent
        (ON CONFLICT DO NOTHING). Returns the count attempted."""
        rows = [
            (track_id, source, raw[:120], (kind or "genre"), weight)
            for raw, kind, weight in tags if raw
        ]
        if not rows:
            return 0
        conn = self._connect()
        if conn is None:
            return 0
        try:
            with conn.cursor() as cur:
                cur.executemany(
                    """
                    INSERT INTO track_genres (track_id, source, raw_tag, tag_kind, weight)
                    VALUES (%s,%s,%s,%s,%s)
                    ON CONFLICT (track_id, source, raw_tag) DO NOTHING
                    """,
                    rows,
                )
            return len(rows)
        except Exception as exc:  # noqa: BLE001
            print(f"db: add_track_tags failed: {type(exc).__name__}: {exc}",
                  flush=True)
            try:
                conn.close()
            finally:
                self._conn = None
            return 0

    def add_artist_tags(self, artist_id: str, source: str,
                        tags: Iterable[tuple[str, str, Optional[float]]]) -> int:
        """Insert (raw_tag, tag_kind, weight) tags for an ARTIST. Idempotent
        (ON CONFLICT DO NOTHING). Returns the count attempted. Artist-level MB
        tags fill every track by that artist whose recording had no MB tags."""
        if not artist_id:
            return 0
        rows = [
            (artist_id, source, raw[:120], (kind or "genre"), weight)
            for raw, kind, weight in tags if raw
        ]
        if not rows:
            return 0
        conn = self._connect()
        if conn is None:
            return 0
        try:
            with conn.cursor() as cur:
                cur.executemany(
                    """
                    INSERT INTO artist_genres_enriched
                      (artist_id, source, raw_tag, tag_kind, weight)
                    VALUES (%s,%s,%s,%s,%s)
                    ON CONFLICT (artist_id, source, raw_tag) DO NOTHING
                    """,
                    rows,
                )
            return len(rows)
        except Exception as exc:  # noqa: BLE001
            print(f"db: add_artist_tags failed: {type(exc).__name__}: {exc}",
                  flush=True)
            try:
                conn.close()
            finally:
                self._conn = None
            return 0

    def enrichment_stats(self) -> dict:
        """Backlog + coverage counts for the enrichment Prometheus metrics.

        `untagged_old_filled` measures the headline win: tracks whose primary
        artist has NO Spotify genres (the `untagged` old/catalog case) that now
        carry at least one MusicBrainz fine sub-genre tag.
        """
        rows = self._query(
            """
            SELECT
              (SELECT count(*) FROM tracks)                                   AS tracks_total,
              (SELECT count(*) FROM music_ids WHERE match_status='matched')   AS matched,
              (SELECT count(*) FROM music_ids WHERE match_status='nomatch')   AS nomatch,
              (SELECT count(*) FROM tracks t
                 WHERE NOT EXISTS (SELECT 1 FROM music_ids mi
                                    WHERE mi.track_id=t.track_id
                                      AND mi.match_status<>'unmatched'))      AS backlog,
              (SELECT count(DISTINCT track_id) FROM track_genre_effective
                 WHERE source='musicbrainz')                                  AS tracks_with_mb_tags,
              -- Headline win: tracks whose primary artist has NO Spotify genres
              -- (the `untagged` old/catalog case) that now carry a MusicBrainz
              -- sub-genre via the effective view (recording- OR artist-level).
              (SELECT count(*) FROM tracks t
                 JOIN artists a ON a.artist_id = t.artist_id
                WHERE COALESCE(array_length(a.genres,1),0) = 0
                  AND EXISTS (SELECT 1 FROM track_genre_effective e
                               WHERE e.track_id = t.track_id
                                 AND e.source = 'musicbrainz'))               AS untagged_old_filled
            """
        )
        return rows[0] if rows else {}


# Module-level singleton, mirroring the exporter's other globals.
DB_INSTANCE = DB()
