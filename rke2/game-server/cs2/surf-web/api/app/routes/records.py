from fastapi import APIRouter, Query

from .. import db, steam
from ..models import RunRecord

router = APIRouter(prefix="/api/records", tags=["records"])


def _to_run(r: dict, avatar: str | None = None) -> RunRecord:
    return RunRecord(
        run_id=r["run_id"],
        map_id=r["map_id"],
        map_file=r["map_file"],
        steam_id=str(r["steam_id"]),
        player_name=r["player_name"],
        run_type=r["run_type"],
        track=r["track"],
        stage=r["stage"],
        style=r["style"],
        time=r["time"],
        jumps=r.get("jumps") or 0,
        strafes=r.get("strafes") or 0,
        sync=r.get("sync") or 0.0,
        date=r["date"],
        avatar=avatar,
    )


@router.get("/recent", response_model=list[RunRecord])
async def recent_records(limit: int = Query(25, ge=1, le=100)):
    rows = db.fetch_all(
        """
        SELECT
          r.Id AS run_id, r.MapId AS map_id, m.File AS map_file,
          r.SteamId AS steam_id, p.Name AS player_name,
          r.RunType AS run_type, r.Track AS track, r.Stage AS stage,
          r.Style AS style, r.Time AS time, r.Date AS date,
          r.Jumps AS jumps, r.Strafes AS strafes, r.Sync AS sync
        FROM surf_runs r
        JOIN surf_maps m ON m.MapId = r.MapId
        JOIN surf_players p ON p.SteamId = r.SteamId
        ORDER BY r.Date DESC
        LIMIT %s
        """,
        (limit,),
    )
    enriched = await steam.enrich(list({str(r["steam_id"]) for r in rows}))
    return [_to_run(r, enriched.get(str(r["steam_id"]), {}).get("avatar")) for r in rows]


@router.get("/wr", response_model=list[RunRecord])
async def world_records(limit: int = Query(25, ge=1, le=100)):
    """Most recently set map WRs (main track only)."""
    rows = db.fetch_all(
        """
        SELECT
          b.RunId AS run_id, b.MapId AS map_id, m.File AS map_file,
          b.SteamId AS steam_id, p.Name AS player_name,
          b.RunType AS run_type, b.Track AS track, b.Stage AS stage,
          b.Style AS style, b.BestTime AS time, b.UpdatedAt AS date,
          r.Jumps AS jumps, r.Strafes AS strafes, r.Sync AS sync
        FROM surf_player_best_runs b
        JOIN surf_maps m ON m.MapId = b.MapId
        JOIN surf_players p ON p.SteamId = b.SteamId
        LEFT JOIN surf_runs r ON r.Id = b.RunId
        INNER JOIN (
          SELECT MapId, MIN(BestTime) AS best
          FROM surf_player_best_runs
          WHERE RunType = 0 AND Track = 0 AND Style = 0
          GROUP BY MapId
        ) t ON t.MapId = b.MapId AND t.best = b.BestTime
        WHERE b.RunType = 0 AND b.Track = 0 AND b.Style = 0
        ORDER BY b.UpdatedAt DESC
        LIMIT %s
        """,
        (limit,),
    )
    enriched = await steam.enrich(list({str(r["steam_id"]) for r in rows}))
    return [_to_run(r, enriched.get(str(r["steam_id"]), {}).get("avatar")) for r in rows]
