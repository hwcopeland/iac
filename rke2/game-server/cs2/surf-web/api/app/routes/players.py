from fastapi import APIRouter, HTTPException, Query

from .. import db, steam
from ..models import PlayerDetail, PlayerProfile, PlayerSummary, RunRecord

router = APIRouter(prefix="/api/players", tags=["players"])


@router.get("", response_model=list[PlayerSummary])
async def list_players(
    limit: int = Query(50, ge=1, le=500),
    offset: int = Query(0, ge=0),
    search: str | None = Query(None, min_length=1, max_length=64),
    sort: str | None = Query(None, pattern="^(points|completions|name)$"),
    order: str | None = Query("desc", pattern="^(asc|desc)$"),
):
    where = ""
    params: list = []
    if search:
        where = "WHERE p.Name LIKE %s"
        params.append(f"%{search}%")

    sort_col = {
        "points": "p.Points",
        "completions": "map_completions",
        "name": "p.Name",
    }.get(sort or "points", "p.Points")
    direction = "ASC" if order == "asc" else "DESC"

    rows = db.fetch_all(
        f"""
        SELECT
          p.SteamId, p.Name, p.Points,
          COALESCE(c.maps, 0) AS map_completions
        FROM surf_players p
        LEFT JOIN (
          SELECT SteamId, COUNT(DISTINCT MapId) AS maps
          FROM surf_player_best_runs
          WHERE RunType = 0 AND Track = 0 AND Style = 0
          GROUP BY SteamId
        ) c ON c.SteamId = p.SteamId
        {where}
        ORDER BY {sort_col} {direction}, p.Points DESC
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
            map_completions=r["map_completions"],
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
          p.SteamId, p.Name, p.Points, p.UpdatedAt,
          (SELECT COUNT(DISTINCT MapId) FROM surf_player_best_runs
             WHERE SteamId = p.SteamId AND RunType = 0 AND Track = 0 AND Style = 0
          ) AS map_completions,
          (SELECT COUNT(*) FROM surf_player_best_runs b
            INNER JOIN (
              SELECT MapId, Track, Stage, RunType, MIN(BestTime) AS best
              FROM surf_player_best_runs
              WHERE Style = 0
              GROUP BY MapId, Track, Stage, RunType
            ) t ON t.MapId = b.MapId AND t.Track = b.Track
               AND t.Stage = b.Stage AND t.RunType = b.RunType
               AND t.best = b.BestTime
            WHERE b.SteamId = p.SteamId AND b.Style = 0
          ) AS record_count
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
        map_completions=player["map_completions"] or 0,
        updated_at=player["UpdatedAt"],
        record_count=player["record_count"] or 0,
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
