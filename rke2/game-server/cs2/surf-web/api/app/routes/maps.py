from fastapi import APIRouter, HTTPException

from .. import db, steam
from ..models import MapDetail, MapSummary, RunRecord, TrackTop

router = APIRouter(prefix="/api/maps", tags=["maps"])


def _map_summary_rows() -> list[dict]:
    return db.fetch_all(
        """
        SELECT
          m.MapId, m.File, m.Tier, m.Stages, m.BasePot,
          (SELECT COUNT(DISTINCT SteamId) FROM surf_player_best_runs
             WHERE MapId = m.MapId AND RunType = 0 AND Track = 0) AS completions,
          wr.SteamId AS wr_steam, wr.BestTime AS wr_time, wp.Name AS wr_name
        FROM surf_maps m
        LEFT JOIN (
          SELECT b.MapId, b.SteamId, b.BestTime
          FROM surf_player_best_runs b
          INNER JOIN (
            SELECT MapId, MIN(BestTime) AS best
            FROM surf_player_best_runs
            WHERE RunType = 0 AND Track = 0 AND Style = 0
            GROUP BY MapId
          ) t ON t.MapId = b.MapId AND t.best = b.BestTime
          WHERE b.RunType = 0 AND b.Track = 0 AND b.Style = 0
        ) wr ON wr.MapId = m.MapId
        LEFT JOIN surf_players wp ON wp.SteamId = wr.SteamId
        GROUP BY m.MapId, m.File, m.Tier, m.Stages, m.BasePot,
                 wr.SteamId, wr.BestTime, wp.Name
        ORDER BY m.Tier, m.File
        """
    )


@router.get("", response_model=list[MapSummary])
def list_maps():
    rows = _map_summary_rows()
    return [
        MapSummary(
            map_id=r["MapId"],
            file=r["File"],
            tier=r["Tier"],
            stages=r["Stages"],
            base_pot=r["BasePot"],
            completions=r["completions"] or 0,
            wr_holder=r["wr_name"],
            wr_time=r["wr_time"],
        )
        for r in rows
    ]


def _top(map_id: int, track: int, stage: int, run_type: int, limit: int = 50) -> list[dict]:
    return db.fetch_all(
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
        WHERE b.MapId = %s AND b.Track = %s AND b.Stage = %s AND b.RunType = %s AND b.Style = 0
        ORDER BY b.BestTime ASC
        LIMIT %s
        """,
        (map_id, track, stage, run_type, limit),
    )


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


@router.get("/{file}", response_model=MapDetail)
async def map_detail(file: str):
    summary_row = db.fetch_one(
        """
        SELECT m.MapId, m.File, m.Tier, m.Stages, m.BasePot
        FROM surf_maps m WHERE m.File = %s
        """,
        (file,),
    )
    if not summary_row:
        raise HTTPException(404, "map not found")

    map_id = summary_row["MapId"]
    stages_count = summary_row["Stages"]

    # bonus tracks for this map (track > 0)
    bonus_tracks = db.fetch_all(
        "SELECT DISTINCT Track FROM surf_maps_tracks WHERE MapId = %s AND Track > 0 ORDER BY Track",
        (map_id,),
    )

    main = _top(map_id, track=0, stage=0, run_type=0)
    stage_rows = [
        TrackTop(track=0, stage=s, run_type=1, times=[_to_run(r) for r in _top(map_id, 0, s, 1, limit=10)])
        for s in range(1, (stages_count or 0) + 1)
    ]
    bonus_rows = [
        TrackTop(track=t["Track"], stage=0, run_type=0, times=[_to_run(r) for r in _top(map_id, t["Track"], 0, 0, limit=10)])
        for t in bonus_tracks
    ]

    summary = MapSummary(
        map_id=map_id,
        file=summary_row["File"],
        tier=summary_row["Tier"],
        stages=summary_row["Stages"],
        base_pot=summary_row["BasePot"],
        completions=db.fetch_one(
            "SELECT COUNT(DISTINCT SteamId) c FROM surf_player_best_runs "
            "WHERE MapId = %s AND RunType = 0 AND Track = 0",
            (map_id,),
        )["c"],
        wr_holder=(main[0]["player_name"] if main else None),
        wr_time=(main[0]["time"] if main else None),
    )

    steam_ids = {str(r["steam_id"]) for r in main}
    for tt in stage_rows + bonus_rows:
        steam_ids.update(t.steam_id for t in tt.times)
    enriched = await steam.enrich(list(steam_ids))

    main_top = [_to_run(r, enriched.get(str(r["steam_id"]), {}).get("avatar")) for r in main]
    for tt in stage_rows + bonus_rows:
        for t in tt.times:
            t.avatar = enriched.get(t.steam_id, {}).get("avatar")

    return MapDetail(map=summary, main_top=main_top, stages=stage_rows, bonuses=bonus_rows)
