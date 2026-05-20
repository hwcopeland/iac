"""A2S_INFO query for Source-engine servers (CS2)."""
from __future__ import annotations

import socket
import struct
import threading
import time
from dataclasses import asdict, dataclass


_A2S_INFO_HEADER = b"\xff\xff\xff\xff\x54Source Engine Query\x00"
_PACKET_PREFIX = b"\xff\xff\xff\xff"
_RESP_INFO = 0x49
_RESP_CHALLENGE = 0x41


@dataclass(frozen=True)
class LiveServer:
    name: str
    map: str
    folder: str
    game: str
    players: int
    max_players: int
    bots: int
    visibility: int
    vac: int
    version: str
    keywords: str | None
    fetched_at: float


def _read_cstr(buf: memoryview, offset: int) -> tuple[str, int]:
    end = offset
    while end < len(buf) and buf[end] != 0:
        end += 1
    s = bytes(buf[offset:end]).decode("utf-8", errors="replace")
    return s, end + 1


def _parse_info(payload: bytes) -> LiveServer:
    if not payload.startswith(_PACKET_PREFIX):
        raise ValueError("missing packet prefix")
    buf = memoryview(payload)[4:]
    if buf[0] != _RESP_INFO:
        raise ValueError(f"unexpected response header 0x{buf[0]:02x}")
    off = 1
    off += 1  # protocol byte
    name, off = _read_cstr(buf, off)
    map_, off = _read_cstr(buf, off)
    folder, off = _read_cstr(buf, off)
    game, off = _read_cstr(buf, off)
    off += 2  # appid (short)
    players = buf[off]; off += 1
    max_players = buf[off]; off += 1
    bots = buf[off]; off += 1
    off += 1  # server_type
    off += 1  # environment
    visibility = buf[off]; off += 1
    vac = buf[off]; off += 1
    version, off = _read_cstr(buf, off)
    keywords: str | None = None
    if off < len(buf):
        edf = buf[off]; off += 1
        if edf & 0x80:
            off += 2  # port
        if edf & 0x10:
            off += 8  # steamid
        if edf & 0x40:
            off += 2  # spectator port
            _, off = _read_cstr(buf, off)  # spectator name
        if edf & 0x20:
            keywords, off = _read_cstr(buf, off)
        # 0x01 gameid (long long) — ignored
    return LiveServer(
        name=name,
        map=map_,
        folder=folder,
        game=game,
        players=players,
        max_players=max_players,
        bots=bots,
        visibility=visibility,
        vac=vac,
        version=version,
        keywords=keywords,
        fetched_at=time.time(),
    )


def query(host: str, port: int, timeout: float = 1.5) -> LiveServer:
    """Send an A2S_INFO request, honoring a challenge response if returned."""
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
        s.settimeout(timeout)
        s.sendto(_A2S_INFO_HEADER, (host, port))
        data, _ = s.recvfrom(4096)
        if len(data) >= 5 and data[4] == _RESP_CHALLENGE:
            challenge = data[5:9]
            s.sendto(_A2S_INFO_HEADER + challenge, (host, port))
            data, _ = s.recvfrom(4096)
        return _parse_info(data)


class _Cache:
    def __init__(self, ttl: float) -> None:
        self._ttl = ttl
        self._lock = threading.Lock()
        self._value: LiveServer | None = None
        self._error: str | None = None
        self._expires: float = 0.0

    def get(self, host: str, port: int) -> tuple[LiveServer | None, str | None]:
        now = time.time()
        with self._lock:
            if now < self._expires:
                return self._value, self._error
        try:
            v = query(host, port)
            err: str | None = None
        except Exception as e:  # noqa: BLE001
            v = None
            err = f"{type(e).__name__}: {e}"
        with self._lock:
            self._value = v
            self._error = err
            self._expires = time.time() + self._ttl
        return v, err


_cache: _Cache | None = None


def fetch_cached(host: str, port: int, ttl: float) -> dict:
    global _cache
    if _cache is None or _cache._ttl != ttl:
        _cache = _Cache(ttl)
    info, err = _cache.get(host, port)
    return {
        "online": info is not None,
        "error": err,
        "info": asdict(info) if info else None,
    }
