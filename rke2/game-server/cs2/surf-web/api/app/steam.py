import asyncio
import time

import httpx

from .settings import settings

_PROFILE_URL = "https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v2/"
_cache: dict[str, tuple[float, dict]] = {}
_lock = asyncio.Lock()


async def _fetch_batch(client: httpx.AsyncClient, steam_ids: list[str]) -> list[dict]:
    if not steam_ids or not settings.steam_api_key:
        return []
    params = {"key": settings.steam_api_key, "steamids": ",".join(steam_ids)}
    try:
        r = await client.get(_PROFILE_URL, params=params, timeout=5.0)
        r.raise_for_status()
        return r.json().get("response", {}).get("players", [])
    except (httpx.HTTPError, ValueError):
        return []


async def enrich(steam_ids: list[str]) -> dict[str, dict]:
    """Return {steamid64: {avatar, profile_url, name}}; cached for 24h."""
    out: dict[str, dict] = {}
    now = time.time()
    missing: list[str] = []

    for sid in steam_ids:
        hit = _cache.get(sid)
        if hit and now - hit[0] < settings.steam_cache_ttl_seconds:
            out[sid] = hit[1]
        else:
            missing.append(sid)

    if missing:
        async with _lock, httpx.AsyncClient() as client:
            # Steam API max 100 ids per call
            for i in range(0, len(missing), 100):
                batch = missing[i : i + 100]
                players = await _fetch_batch(client, batch)
                for p in players:
                    info = {
                        "avatar": p.get("avatarfull") or p.get("avatarmedium"),
                        "profile_url": p.get("profileurl"),
                        "name": p.get("personaname"),
                    }
                    sid = str(p.get("steamid"))
                    _cache[sid] = (now, info)
                    out[sid] = info

    return out
