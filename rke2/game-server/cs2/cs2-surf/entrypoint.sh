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

CS2_DIR="${CS2_DIR:-/home/steam/cs2-dedicated}"
STEAMCMD_DIR="${STEAMCMD_DIR:-/home/steam/steamcmd}"

# ── Logging helpers ──────────────────────────────────────────────────────────
log()  { echo "[cs2-surf] $(date '+%Y-%m-%d %H:%M:%S') $*"; }
warn() { echo "[cs2-surf] $(date '+%Y-%m-%d %H:%M:%S') WARN: $*" >&2; }
die()  { echo "[cs2-surf] $(date '+%Y-%m-%d %H:%M:%S') FATAL: $*" >&2; exit 1; }

# ── Step 1: Install / Update CS2 Dedicated Server ───────────────────────────
log "Updating CS2 dedicated server via steamcmd..."

"${STEAMCMD_DIR}/steamcmd.sh" \
    +force_install_dir "${CS2_DIR}" \
    +login anonymous \
    +app_update 730 validate \
    +quit

if [ ! -f "${CS2_DIR}/game/bin/linuxsteamrt64/cs2" ]; then
    die "CS2 binary not found after steamcmd update. Check disk space and network."
fi

log "CS2 server files up to date."

# ── Step 2: Apply plugin overlay ────────────────────────────────────────────
log "Checking plugin overlay..."
/opt/cs2-surf/scripts/apply-overlay.sh

# ── Step 3: Database config substitution ────────────────────────────────────
# The database.json template lives at /opt/cs2-surf/configs/database.json
# or can be mounted via ConfigMap at /opt/cs2-surf/configs/.
# It uses ${MYSQL_USER}, ${MYSQL_PASS}, etc. — substituted by envsubst.

DB_TEMPLATE="/opt/cs2-surf/configs/timer.jsonc"
DB_TARGET="${CS2_DIR}/game/csgo/sharp/configs/timer.jsonc"

if [ -f "${DB_TEMPLATE}" ]; then
    log "Substituting database credentials into timer.jsonc..."
    mkdir -p "$(dirname "${DB_TARGET}")"
    envsubst < "${DB_TEMPLATE}" > "${DB_TARGET}"
    log "Database config written to ${DB_TARGET}"
else
    warn "No database template found at ${DB_TEMPLATE} — skipping substitution."
fi

# ── Step 4: Launch CS2 ──────────────────────────────────────────────────────
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
