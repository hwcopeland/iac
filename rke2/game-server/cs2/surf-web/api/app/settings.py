from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="SURFWEB_", case_sensitive=False)

    mysql_host: str = "10.43.43.43"
    mysql_port: int = 3306
    mysql_database: str = "source2surf"
    mysql_user: str = "surfweb_ro"
    mysql_password: str = ""
    mysql_pool_size: int = 8

    steam_api_key: str = ""
    steam_cache_ttl_seconds: int = 86400

    gameserver_host: str = "cs2-modded-service.game-server.svc.cluster.local"
    gameserver_port: int = 27015
    gameserver_cache_ttl_seconds: float = 10.0

    cors_origins: str = "https://surf.hwcopeland.net"


settings = Settings()
