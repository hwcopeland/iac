from datetime import datetime

from pydantic import BaseModel, Field


class PlayerSummary(BaseModel):
    steam_id: str
    name: str
    points: int
    runs: int
    rank: int | None = None
    avatar: str | None = None
    profile_url: str | None = None


class PlayerDetail(PlayerSummary):
    updated_at: datetime
    map_count: int
    wr_count: int


class MapSummary(BaseModel):
    map_id: int
    file: str
    tier: int
    stages: int
    base_pot: int
    completions: int
    wr_holder: str | None = None
    wr_time: float | None = None


class RunRecord(BaseModel):
    run_id: int
    map_id: int
    map_file: str
    steam_id: str
    player_name: str
    run_type: int = Field(description="0 = main run, 1 = stage run")
    track: int = Field(description="0 = main, 1 = bonus")
    stage: int
    style: int
    time: float
    jumps: int
    strafes: int
    sync: float
    date: datetime
    avatar: str | None = None


class TrackTop(BaseModel):
    track: int
    stage: int
    run_type: int
    times: list[RunRecord]


class MapDetail(BaseModel):
    map: MapSummary
    main_top: list[RunRecord]
    stages: list[TrackTop]
    bonuses: list[TrackTop]


class PlayerProfile(BaseModel):
    player: PlayerDetail
    personal_bests: list[RunRecord]
    recent_runs: list[RunRecord]
