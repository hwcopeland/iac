"""Shared Spotify token store backed by a Kubernetes Secret.

Single source of truth for the Spotify OAuth token, shared between this
exporter and JARVIS (which reaches the same Secret via kubectl). Spotify's
PKCE flow rotates the refresh token on every refresh and revokes the old one,
so exactly one refresh may win a race. The protocol both sides follow:

    1. Read the Secret. If the access token is still valid, use it — no write.
    2. Only when expired, refresh and write the rotated token back.
    3. If a refresh returns invalid_grant, re-read the Secret: the peer may
       have just refreshed. Use its token, or retry once with the newer
       refresh token it wrote.

This shrinks the conflict window to two refreshes landing within a few seconds
of each other, and even that self-heals on the next read.

The exporter authenticates to the Kubernetes API with its mounted
ServiceAccount token; Secret access is granted by the Role in deployment.yaml.
Spotify HTTPS uses the certifi CA bundle; the k8s API uses the in-cluster CA.
"""
from __future__ import annotations

import base64
import json
import os
import ssl
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Optional

try:
    import certifi
    _SPOTIFY_CTX: Optional[ssl.SSLContext] = ssl.create_default_context(cafile=certifi.where())
except Exception:  # noqa: BLE001
    _SPOTIFY_CTX = None

_SA_DIR = "/var/run/secrets/kubernetes.io/serviceaccount"
_TOKEN_URL = "https://accounts.spotify.com/api/token"
_HTTP_TIMEOUT = 12
_UA = "spotify-exporter/1.0"

SECRET_NAME = os.environ.get("SPOTIFY_SECRET_NAME", "spotify-token")
SECRET_KEY = "tokens.json"
CLIENT_ID = os.environ.get("SPOTIFY_CLIENT_ID", "")
BOOTSTRAP_REFRESH = os.environ.get("SPOTIFY_REFRESH_TOKEN", "")


class TokenError(Exception):
    pass


# ── Kubernetes API plumbing (in-cluster) ─────────────────────────────────────
def _k8s_ctx() -> ssl.SSLContext:
    return ssl.create_default_context(cafile=os.path.join(_SA_DIR, "ca.crt"))


def _k8s_token() -> str:
    with open(os.path.join(_SA_DIR, "token")) as f:
        return f.read().strip()


def _k8s_namespace() -> str:
    try:
        with open(os.path.join(_SA_DIR, "namespace")) as f:
            return f.read().strip()
    except OSError:
        return "monitor"


def _k8s_base() -> str:
    host = os.environ.get("KUBERNETES_SERVICE_HOST", "kubernetes.default.svc")
    port = os.environ.get("KUBERNETES_SERVICE_PORT", "443")
    return f"https://{host}:{port}/api/v1/namespaces/{_k8s_namespace()}/secrets"


def _k8s_request(method: str, body: Optional[bytes] = None,
                 content_type: str = "application/json",
                 url: Optional[str] = None) -> tuple[int, bytes]:
    req = urllib.request.Request(url or f"{_k8s_base()}/{SECRET_NAME}",
                                 data=body, method=method)
    req.add_header("Authorization", f"Bearer {_k8s_token()}")
    req.add_header("Accept", "application/json")
    if body is not None:
        req.add_header("Content-Type", content_type)
    try:
        with urllib.request.urlopen(req, timeout=_HTTP_TIMEOUT, context=_k8s_ctx()) as r:
            return r.getcode(), r.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()
    except (urllib.error.URLError, OSError, TimeoutError) as e:
        raise TokenError(f"k8s API unreachable: {e}") from e


def _read_secret() -> dict:
    """Return the token dict from the Secret, or {} if absent/empty."""
    code, raw = _k8s_request("GET")
    if code == 404:
        return {}
    if code != 200:
        raise TokenError(f"k8s GET secret failed: HTTP {code} {raw[:160]!r}")
    data = (json.loads(raw).get("data") or {})
    blob = data.get(SECRET_KEY)
    if not blob:
        return {}
    try:
        return json.loads(base64.b64decode(blob))
    except (ValueError, json.JSONDecodeError):
        return {}


def _write_secret(tokens: dict) -> None:
    """Persist the token JSON to the Secret, creating it on first write."""
    b64 = base64.b64encode(json.dumps(tokens).encode()).decode()
    patch = json.dumps({"data": {SECRET_KEY: b64}}).encode()
    code, raw = _k8s_request("PATCH", patch,
                             content_type="application/strategic-merge-patch+json")
    if code in (200, 201):
        return
    if code == 404:  # Secret doesn't exist yet — create it.
        obj = json.dumps({
            "apiVersion": "v1", "kind": "Secret",
            "metadata": {"name": SECRET_NAME, "namespace": _k8s_namespace()},
            "type": "Opaque", "data": {SECRET_KEY: b64},
        }).encode()
        code, raw = _k8s_request("POST", obj, url=_k8s_base())
        if code in (200, 201):
            return
    raise TokenError(f"k8s write secret failed: HTTP {code} {raw[:160]!r}")


# ── Spotify refresh ──────────────────────────────────────────────────────────
def _spotify_refresh(refresh_token: str) -> tuple[Optional[dict], Optional[str]]:
    """Return (new_tokens, error). error is e.g. 'invalid_grant' or transport msg."""
    if not refresh_token or not CLIENT_ID:
        return None, "missing client_id or refresh_token"
    body = urllib.parse.urlencode({
        "grant_type": "refresh_token",
        "refresh_token": refresh_token,
        "client_id": CLIENT_ID,
    }).encode()
    req = urllib.request.Request(_TOKEN_URL, data=body, method="POST")
    req.add_header("User-Agent", _UA)
    req.add_header("Content-Type", "application/x-www-form-urlencoded")
    try:
        with urllib.request.urlopen(req, timeout=_HTTP_TIMEOUT, context=_SPOTIFY_CTX) as r:
            new = json.loads(r.read())
    except urllib.error.HTTPError as e:
        detail = ""
        try:
            detail = json.loads(e.read()).get("error", "")
        except Exception:  # noqa: BLE001
            pass
        return None, detail or f"http {e.code}"
    except (urllib.error.URLError, OSError, ValueError) as e:
        return None, str(e)
    # Spotify usually rotates the refresh token; if omitted, keep the old one.
    new.setdefault("refresh_token", refresh_token)
    new["expires_at"] = int(time.time()) + int(new.get("expires_in", 3600)) - 30
    return new, None


def _valid(tokens: dict) -> bool:
    return bool(tokens.get("access_token")) and tokens.get("expires_at", 0) > time.time() + 5


# ── Public API ────────────────────────────────────────────────────────────────
def access_token() -> Optional[str]:
    """Return a valid Spotify access token using the shared Secret as SoT.

    Reads the latest token; refreshes and writes back only on expiry; recovers
    from a lost refresh race by re-reading the peer's freshly-written token.
    """
    tokens = _read_secret()
    if not tokens.get("refresh_token") and BOOTSTRAP_REFRESH:
        tokens = {"refresh_token": BOOTSTRAP_REFRESH, "expires_at": 0}
        _write_secret(tokens)
    if _valid(tokens):
        return tokens["access_token"]

    rt = tokens.get("refresh_token")
    new, err = _spotify_refresh(rt) if rt else (None, "no refresh token")
    if new:
        _write_secret(new)
        return new["access_token"]

    # Refresh failed — likely the peer rotated the token out from under us.
    if err == "invalid_grant":
        peer = _read_secret()
        if _valid(peer):
            return peer["access_token"]
        if peer.get("refresh_token") and peer["refresh_token"] != rt:
            new, _ = _spotify_refresh(peer["refresh_token"])
            if new:
                _write_secret(new)
                return new["access_token"]
    return None
