from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from . import a2s, db
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

# OpenTelemetry: auto-instrument all FastAPI routes when
# OTEL_EXPORTER_OTLP_ENDPOINT is set. Env-driven so the same binary works
# in/out of the cluster. The instrumentation is a no-op if the OTLP
# exporter can't reach Tempo — endpoint unreachable just drops spans.
import os
if os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT"):
    from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
    FastAPIInstrumentor.instrument_app(app)

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


@app.get("/api/server/live")
def server_live() -> dict:
    return a2s.fetch_cached(
        settings.gameserver_host,
        settings.gameserver_port,
        settings.gameserver_cache_ttl_seconds,
    )


app.include_router(players.router)
app.include_router(maps.router)
app.include_router(records.router)
