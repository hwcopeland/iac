from collections.abc import Iterator
from contextlib import contextmanager
from datetime import datetime, timezone
from typing import Any

import mysql.connector
from mysql.connector.conversion import MySQLConverter
from mysql.connector.pooling import MySQLConnectionPool

from .settings import settings


class UTCConverter(MySQLConverter):
    """Source2Surf.Timer stores DATETIME columns as naive UTC. Tag them so
    Pydantic serializes with an offset and the browser doesn't reinterpret
    them as local time (which would make recent records look like the future)."""

    def _DATETIME_to_python(self, value, dsc=None):  # noqa: N802
        dt = super()._DATETIME_to_python(value, dsc)
        if isinstance(dt, datetime) and dt.tzinfo is None:
            return dt.replace(tzinfo=timezone.utc)
        return dt

    def _TIMESTAMP_to_python(self, value, dsc=None):  # noqa: N802
        dt = super()._TIMESTAMP_to_python(value, dsc)
        if isinstance(dt, datetime) and dt.tzinfo is None:
            return dt.replace(tzinfo=timezone.utc)
        return dt


_pool: MySQLConnectionPool | None = None


def init_pool() -> None:
    global _pool
    _pool = MySQLConnectionPool(
        pool_name="surfweb",
        pool_size=settings.mysql_pool_size,
        pool_reset_session=True,
        host=settings.mysql_host,
        port=settings.mysql_port,
        database=settings.mysql_database,
        user=settings.mysql_user,
        password=settings.mysql_password,
        autocommit=True,
        connection_timeout=5,
        converter_class=UTCConverter,
    )


@contextmanager
def cursor() -> Iterator[Any]:
    assert _pool is not None, "DB pool not initialised"
    conn = _pool.get_connection()
    try:
        cur = conn.cursor(dictionary=True)
        try:
            yield cur
        finally:
            cur.close()
    finally:
        conn.close()


def fetch_all(sql: str, params: tuple | dict | None = None) -> list[dict]:
    with cursor() as cur:
        cur.execute(sql, params or ())
        return cur.fetchall()


def fetch_one(sql: str, params: tuple | dict | None = None) -> dict | None:
    with cursor() as cur:
        cur.execute(sql, params or ())
        return cur.fetchone()
