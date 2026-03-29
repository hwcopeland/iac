#!/bin/bash
# =============================================================================
# cs2-surf-server entrypoint
#
# 1. Update CS2 via steamcmd (delta update, fast on warm PVC)
# 2. Apply plugin overlay if version stamp differs
# 3. Substitute env vars into database config template
# 4. Launch CS2 dedicated server with surf-appropriate args
# =============================================================================
set -euo pipefail

CS2_DIR="${CS2_DIR:-/home/steam/cs2}"
STEAMCMD_DIR="${STEAMCMD_DIR:-/home/steam/steamcmd}"

# ── Logging helpers ──────────────────────────────────────────────────────────
log()  { echo "[cs2-surf] $(date '+%Y-%m-%d %H:%M:%S') $*"; }
warn() { echo "[cs2-surf] $(date '+%Y-%m-%d %H:%M:%S') WARN: $*" >&2; }
die()  { echo "[cs2-surf] $(date '+%Y-%m-%d %H:%M:%S') FATAL: $*" >&2; exit 1; }

# ── Step 0a: Symlink cs2-dedicated -> cs2 for path compatibility ────────────
# cm2network/steamcmd uses cs2-dedicated as default but our PVC mounts at /home/steam/cs2
if [ "${CS2_DIR}" != "/home/steam/cs2-dedicated" ] && [ ! -e "/home/steam/cs2-dedicated" ]; then
    ln -sf "${CS2_DIR}" /home/steam/cs2-dedicated
    log "Linked /home/steam/cs2-dedicated -> ${CS2_DIR}"
fi

# ── Step 0: Persist steamcmd app manifest on PVC ───────────────────────────
# Without this, every new pod re-downloads 62GB because steamcmd doesn't know
# CS2 is already installed on the PVC.
STEAM_APPS="/home/steam/Steam/steamapps"
PVC_STEAM_APPS="${CS2_DIR}/steamapps"
mkdir -p "${PVC_STEAM_APPS}" "${STEAM_APPS%/*}"
if [ ! -L "${STEAM_APPS}" ]; then
    rm -rf "${STEAM_APPS}"
    ln -sf "${PVC_STEAM_APPS}" "${STEAM_APPS}"
    log "Linked ${STEAM_APPS} -> ${PVC_STEAM_APPS}"
fi

# ── Step 1: Install / Update CS2 Dedicated Server ───────────────────────────
if [ -f "${CS2_DIR}/game/bin/linuxsteamrt64/cs2" ] && [ "${FORCE_UPDATE:-0}" != "1" ]; then
    log "CS2 already installed, skipping steamcmd. Set FORCE_UPDATE=1 to force."
else
    log "Installing/updating CS2 via steamcmd..."
    "${STEAMCMD_DIR}/steamcmd.sh" \
        +force_install_dir "${CS2_DIR}" \
        +login anonymous \
        +app_update 730 \
        +quit

    if [ ! -f "${CS2_DIR}/game/bin/linuxsteamrt64/cs2" ]; then
        die "CS2 binary not found after steamcmd update. Check disk space and network."
    fi
    log "CS2 server files up to date."
fi

# ── Step 2: Apply plugin overlay ────────────────────────────────────────────
log "Checking plugin overlay..."
/opt/cs2-surf/scripts/apply-overlay.sh

# ── Step 3: Database config substitution ────────────────────────────────────
# The database.json template lives at /opt/cs2-surf/configs/database.json
# or can be mounted via ConfigMap at /opt/cs2-surf/configs/.
# It uses ${MYSQL_USER}, ${MYSQL_PASS}, etc. — substituted by envsubst.

# Database config: init container already ran envsubst, just copy the result
DB_SOURCE="/opt/cs2-surf/config-out/timer.jsonc"
DB_TARGET="${CS2_DIR}/game/csgo/sharp/configs/timer.jsonc"

if [ -f "${DB_SOURCE}" ]; then
    log "Copying substituted timer.jsonc to sharp configs..."
    mkdir -p "$(dirname "${DB_TARGET}")"
    cp "${DB_SOURCE}" "${DB_TARGET}"
    log "Database config written to ${DB_TARGET}"
else
    # Fallback: use baked-in template with envsubst (for local Docker testing)
    DB_TEMPLATE="/opt/cs2-surf/configs/timer.jsonc"
    if [ -f "${DB_TEMPLATE}" ]; then
        log "Substituting database credentials into timer.jsonc..."
        mkdir -p "$(dirname "${DB_TARGET}")"
        envsubst < "${DB_TEMPLATE}" > "${DB_TARGET}"
        log "Database config written to ${DB_TARGET}"
    else
        warn "No database config found — skipping."
    fi
fi

# ── Step 4: Launch CS2 ──────────────────────────────────────────────────────
# ── Step 3b: Ensure engine .so files are available to csgo/bin ──────────────
# ModSharp's libserver.so in csgo/bin/ needs libv8.so from game/bin/
# CS2 16.9.2025 update requires these to be accessible from csgo/bin/
ENGINE_BIN="${CS2_DIR}/game/bin/linuxsteamrt64"
CSGO_BIN="${CS2_DIR}/game/csgo/bin/linuxsteamrt64"
if [ -d "${ENGINE_BIN}" ] && [ -d "${CSGO_BIN}" ]; then
    for so in "${ENGINE_BIN}"/*.so; do
        target="${CSGO_BIN}/$(basename "$so")"
        [ ! -e "${target}" ] && ln -sf "$so" "${target}"
    done
    log "Engine .so files linked to csgo/bin"
fi

# ── Step 3c: Set up ModSharp LD_PRELOAD ─────────────────────────────────────
PRELOAD_FILE="${CS2_DIR}/.modsharp-preload"
if [ -f "${PRELOAD_FILE}" ]; then
    export LD_PRELOAD="$(cat "${PRELOAD_FILE}")"
    log "LD_PRELOAD set to: ${LD_PRELOAD}"
fi

log "Starting CS2 surf server..."
log "  Map:        ${MAP:-surf_kitsune}"
log "  Port:       ${PORT:-27015}"
log "  MaxPlayers: ${MAXPLAYERS:-32}"
log "  Tickrate:   ${TICKRATE:-128}"

exec "${CS2_DIR}/game/bin/linuxsteamrt64/cs2" \
    -dedicated \
    -console \
    -usercon \
    -autoupdate \
    -port "${PORT:-27015}" \
    -tickrate "${TICKRATE:-128}" \
    -authkey "${API_KEY}" \
    +map "${MAP:-surf_kitsune}" \
    +sv_setsteamaccount "${STEAM_ACCOUNT}" \
    +rcon_password "${RCON_PASSWORD}" \
    +sv_visiblemaxplayers "${MAXPLAYERS:-32}" \
    +game_type "${GAME_TYPE:-0}" \
    +game_mode "${GAME_MODE:-0}" \
    +hostname "${SERVER_NAME:-CS2 Surf Server}"
