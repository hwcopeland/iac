from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from . import db
from .routes import maps, players, records
from .settings import settings


@asynccontextmanager
async def lifespan(_app: FastAPI):
    db.init_pool()
    yield


app = FastAPI(
    title="CS2 Surf Leaderboard API",
    version="0.1.0",
    lifespan=lifespan,
)

origins = [o.strip() for o in settings.cors_origins.split(",") if o.strip()]
app.add_middleware(
    CORSMiddleware,
    allow_origins=origins,
    allow_methods=["GET"],
    allow_headers=["*"],
)


@app.get("/healthz")
def healthz() -> dict:
    return {"status": "ok"}


@app.get("/api/server")
def server_info() -> dict:
    counts = db.fetch_one(
        "SELECT "
        "(SELECT COUNT(*) FROM surf_players) AS players, "
        "(SELECT COUNT(*) FROM surf_runs) AS runs, "
        "(SELECT COUNT(*) FROM surf_maps) AS maps"
    )
    return {"counts": counts}


app.include_router(players.router)
app.include_router(maps.router)
app.include_router(records.router)
