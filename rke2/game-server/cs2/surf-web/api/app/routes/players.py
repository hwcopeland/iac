from fastapi import APIRouter, HTTPException, Query

from .. import db, steam
from ..models import PlayerDetail, PlayerProfile, PlayerSummary, RunRecord

router = APIRouter(prefix="/api/players", tags=["players"])


@router.get("", response_model=list[PlayerSummary])
async def list_players(
    limit: int = Query(50, ge=1, le=500),
    offset: int = Query(0, ge=0),
    search: str | None = Query(None, min_length=1, max_length=64),
):
    where = ""
    params: list = []
    if search:
        where = "WHERE Name LIKE %s"
        params.append(f"%{search}%")

    rows = db.fetch_all(
        f"""
        SELECT SteamId, Name, Points, Runs
        FROM surf_players
        {where}
        ORDER BY Points DESC, Runs DESC
        LIMIT %s OFFSET %s
        """,
        tuple(params + [limit, offset]),
    )

    enriched = await steam.enrich([str(r["SteamId"]) for r in rows])
    return [
        PlayerSummary(
            steam_id=str(r["SteamId"]),
            name=enriched.get(str(r["SteamId"]), {}).get("name") or r["Name"],
            points=r["Points"],
            runs=r["Runs"],
            rank=offset + i + 1,
            avatar=enriched.get(str(r["SteamId"]), {}).get("avatar"),
            profile_url=enriched.get(str(r["SteamId"]), {}).get("profile_url"),
        )
        for i, r in enumerate(rows)
    ]


@router.get("/{steam_id}", response_model=PlayerProfile)
async def get_player(steam_id: str):
    if not steam_id.isdigit():
        raise HTTPException(400, "steam_id must be numeric SteamID64")

    player = db.fetch_one(
        """
        SELECT
          p.SteamId, p.Name, p.Points, p.Runs, p.UpdatedAt,
          (SELECT COUNT(DISTINCT MapId) FROM surf_player_best_runs
             WHERE SteamId = p.SteamId AND RunType = 0 AND Track = 0) AS map_count,
          (SELECT COUNT(*) FROM (
              SELECT MapId, Track, Stage, RunType, Style,
                     MIN(Time) AS best_time
              FROM surf_runs GROUP BY MapId, Track, Stage, RunType, Style
            ) t
            JOIN surf_runs me ON me.SteamId = p.SteamId
              AND me.MapId = t.MapId AND me.Track = t.Track
              AND me.Stage = t.Stage AND me.RunType = t.RunType
              AND me.Style = t.Style AND me.Time = t.best_time
          ) AS wr_count
        FROM surf_players p
        WHERE p.SteamId = %s
        """,
        (int(steam_id),),
    )
    if not player:
        raise HTTPException(404, "player not found")

    pbs = db.fetch_all(
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
        WHERE b.SteamId = %s
        ORDER BY b.UpdatedAt DESC
        LIMIT 200
        """,
        (int(steam_id),),
    )

    recent = db.fetch_all(
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
        WHERE r.SteamId = %s
        ORDER BY r.Date DESC
        LIMIT 25
        """,
        (int(steam_id),),
    )

    enriched = await steam.enrich([steam_id])
    info = enriched.get(steam_id, {})

    detail = PlayerDetail(
        steam_id=str(player["SteamId"]),
        name=info.get("name") or player["Name"],
        points=player["Points"],
        runs=player["Runs"],
        updated_at=player["UpdatedAt"],
        map_count=player["map_count"] or 0,
        wr_count=player["wr_count"] or 0,
        avatar=info.get("avatar"),
        profile_url=info.get("profile_url"),
    )

    def to_run(r: dict) -> RunRecord:
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
        )

    return PlayerProfile(
        player=detail,
        personal_bests=[to_run(r) for r in pbs],
        recent_runs=[to_run(r) for r in recent],
    )
