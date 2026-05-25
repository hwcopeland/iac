#!/usr/bin/env python3
"""Persist claude OAuth credentials from emptyDir back to the K8s Secret.

The main jarvis-edge container reads/writes /tmp/.claude/.credentials.json.
That path lives on an emptyDir volume so the claude CLI can refresh in
place — but the volume vanishes on pod restart, which means a refreshed
token is lost and Hampton has to manually re-sync from his Mac every ~8h.

This sidecar watches the file (poll-based; inotify isn't worth the dep
in a busybox-tier image). When it changes, we validate the JSON, base64
the contents, and PATCH the `claude_credentials.json` key on the
`jarvis-secrets` Secret using the pod's projected SA token. From then on
the next pod start's init container reseeds the emptyDir with the
refreshed creds and the cycle holds.

Hard rules:
- Never crash on patch failure. Anthropic-side issues or apiserver
  hiccups must not take this watcher down.
- Never write garbage to the Secret. Validate the JSON parses AND
  carries an access token before patching.
- Debounce. Claude can rewrite the file twice in quick succession during
  a refresh; only one PATCH per quiet window.
"""

from __future__ import annotations

import base64
import json
import logging
import os
import signal
import ssl
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

# ── Config ─────────────────────────────────────────────────────────────
CREDS_PATH = Path(os.environ.get("CREDS_PATH", "/tmp/.claude/.credentials.json"))
NAMESPACE = os.environ.get("NAMESPACE", "ai")
SECRET_NAME = os.environ.get("SECRET_NAME", "jarvis-secrets")
SECRET_KEY = os.environ.get("SECRET_KEY", "claude_credentials.json")
POLL_INTERVAL_S = float(os.environ.get("POLL_INTERVAL_S", "5"))
DEBOUNCE_S = float(os.environ.get("DEBOUNCE_S", "5"))

# In-cluster API access
SA_TOKEN_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/token"
SA_CA_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
KUBE_HOST = os.environ.get("KUBERNETES_SERVICE_HOST", "kubernetes.default.svc")
KUBE_PORT = os.environ.get("KUBERNETES_SERVICE_PORT", "443")

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
)
log = logging.getLogger("creds-watcher")


# ── SIGTERM handling so the pod shuts down promptly on rollout ────────
_stop = False


def _handle_sig(signum, _frame):  # noqa: D401
    global _stop
    log.info("received signal %s, exiting after current iteration", signum)
    _stop = True


signal.signal(signal.SIGTERM, _handle_sig)
signal.signal(signal.SIGINT, _handle_sig)


# ── Validation ────────────────────────────────────────────────────────
def validate_creds(raw: bytes) -> bool:
    """Return True iff `raw` parses as JSON carrying an access token.

    Claude credential files have evolved across CLI versions. Accept any
    of:
      - top-level `accessToken`
      - `claudeAiOauth.accessToken` (current Subscription/Max format)
    """
    try:
        data = json.loads(raw)
    except Exception as e:
        log.error("creds file is not valid JSON: %s", e)
        return False
    if not isinstance(data, dict):
        log.error("creds JSON root is not an object")
        return False
    if isinstance(data.get("accessToken"), str) and data["accessToken"]:
        return True
    oauth = data.get("claudeAiOauth")
    if isinstance(oauth, dict) and isinstance(oauth.get("accessToken"), str) and oauth["accessToken"]:
        return True
    log.error("creds JSON missing accessToken / claudeAiOauth.accessToken")
    return False


# ── K8s API client ────────────────────────────────────────────────────
def _read_token() -> str:
    return Path(SA_TOKEN_PATH).read_text().strip()


def _ssl_ctx() -> ssl.SSLContext:
    ctx = ssl.create_default_context(cafile=SA_CA_PATH)
    return ctx


def patch_secret(raw: bytes) -> bool:
    """Strategic-merge PATCH the secret's single key. Returns True on 2xx."""
    encoded = base64.b64encode(raw).decode("ascii")
    body = json.dumps({"data": {SECRET_KEY: encoded}}).encode("utf-8")

    url = (
        f"https://{KUBE_HOST}:{KUBE_PORT}/api/v1/namespaces/"
        f"{NAMESPACE}/secrets/{SECRET_NAME}"
    )
    req = urllib.request.Request(
        url=url,
        data=body,
        method="PATCH",
        headers={
            "Authorization": f"Bearer {_read_token()}",
            "Content-Type": "application/strategic-merge-patch+json",
            "Accept": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, context=_ssl_ctx(), timeout=15) as resp:
            if 200 <= resp.status < 300:
                log.info(
                    "patched %s/%s key=%s (creds bytes=%d)",
                    NAMESPACE, SECRET_NAME, SECRET_KEY, len(raw),
                )
                return True
            log.error("unexpected status %s patching secret", resp.status)
            return False
    except urllib.error.HTTPError as e:
        body_text = e.read().decode("utf-8", errors="replace")[:500]
        log.error("HTTP %s patching secret: %s", e.code, body_text)
        return False
    except Exception as e:
        # Anything else: network blip, SSL issue, transient apiserver pain.
        # Log + continue; next file change triggers another attempt.
        log.error("error patching secret: %s", e)
        return False


# ── Watch loop ────────────────────────────────────────────────────────
def file_signature() -> tuple[float, int] | None:
    """Return (mtime, size) or None if the file doesn't exist."""
    try:
        st = CREDS_PATH.stat()
    except FileNotFoundError:
        return None
    except Exception as e:
        log.warning("stat(%s) failed: %s", CREDS_PATH, e)
        return None
    return (st.st_mtime, st.st_size)


def try_sync(reason: str) -> bool:
    """Read, validate, and patch. Returns True on successful patch."""
    try:
        raw = CREDS_PATH.read_bytes()
    except FileNotFoundError:
        log.debug("creds file gone before read (%s)", reason)
        return False
    except Exception as e:
        log.error("read(%s) failed: %s", CREDS_PATH, e)
        return False

    if not validate_creds(raw):
        log.error("refusing to patch — creds failed validation (%s)", reason)
        return False

    return patch_secret(raw)


def main() -> int:
    log.info(
        "watching %s, target=%s/%s key=%s, poll=%ss debounce=%ss",
        CREDS_PATH, NAMESPACE, SECRET_NAME, SECRET_KEY,
        POLL_INTERVAL_S, DEBOUNCE_S,
    )

    last_sig = file_signature()
    last_synced_sig: tuple[float, int] | None = None
    pending_since: float | None = None

    # Do an initial sync if the file already exists. The init container
    # seeds it from the Secret on boot, so this is a no-op semantically
    # but cheap insurance against drift if a previous pod refreshed
    # without us running.
    if last_sig is not None and try_sync("startup"):
        last_synced_sig = last_sig

    while not _stop:
        time.sleep(POLL_INTERVAL_S)
        sig = file_signature()
        if sig is None:
            # File missing — init container hasn't seeded yet, or some
            # other transient state. Keep watching.
            continue

        if sig != last_sig:
            # File changed. Reset debounce window.
            log.info("change detected: %s -> %s", last_sig, sig)
            last_sig = sig
            pending_since = time.monotonic()
            continue

        # File stable. If we've got a pending change that's been quiet
        # long enough, sync it.
        if pending_since is not None and sig != last_synced_sig:
            quiet_for = time.monotonic() - pending_since
            if quiet_for >= DEBOUNCE_S:
                if try_sync("change"):
                    last_synced_sig = sig
                pending_since = None  # whether patch succeeded or not,
                # don't busy-loop. Next mtime change re-arms.

    log.info("exiting cleanly")
    return 0


if __name__ == "__main__":
    sys.exit(main())
