#!/bin/bash
# =============================================================================
# apply-overlay.sh
#
# Copies the plugin overlay from /opt/cs2-surf/overlay/ to the CS2 game
# directory. Uses a version stamp to skip redundant copies on restart.
#
# The overlay contains:
#   sharp/          ModSharp framework + all plugins
#   cfg/            Server config files (if any baked-in configs exist)
#
# Version stamp: $CS2_DIR/game/csgo/.surf-overlay-version
# =============================================================================
set -euo pipefail

CS2_DIR="${CS2_DIR:-/home/steam/cs2}"
OVERLAY_DIR="/opt/cs2-surf/overlay"
VERSION_FILE="/opt/cs2-surf/VERSION"
STAMP_FILE="${CS2_DIR}/game/csgo/.surf-overlay-version"

log()  { echo "[apply-overlay] $(date '+%Y-%m-%d %H:%M:%S') $*"; }

# Read the image version
if [ ! -f "${VERSION_FILE}" ]; then
    log "WARNING: No VERSION file found at ${VERSION_FILE}. Forcing overlay copy."
    IMAGE_VERSION="unknown"
else
    IMAGE_VERSION=$(cat "${VERSION_FILE}")
fi

# Read the installed stamp (empty string if not present)
INSTALLED_VERSION=""
if [ -f "${STAMP_FILE}" ]; then
    INSTALLED_VERSION=$(cat "${STAMP_FILE}")
fi

# Compare
if [ "${IMAGE_VERSION}" = "${INSTALLED_VERSION}" ]; then
    log "Overlay up to date (version: ${IMAGE_VERSION}). Skipping copy."
    exit 0
fi

log "Overlay version mismatch (image: ${IMAGE_VERSION}, installed: ${INSTALLED_VERSION:-none}). Applying overlay..."

# Ensure target directory exists (CS2 may not have created csgo/ on first run
# before steamcmd app_update, but the entrypoint runs steamcmd first)
mkdir -p "${CS2_DIR}/game/csgo"

# Copy overlay contents into the game directory
# ModSharp installs to game/sharp/ (NOT game/csgo/sharp/)
# The zip extracts as sharp/ which goes directly under game/
cp -rf "${OVERLAY_DIR}/." "${CS2_DIR}/game/"

# Patch gameinfo.gi: add "Game sharp" before "Game csgo" per ModSharp docs
# Also strip any leftover Metamod/CSS entries from kus
GAMEINFO="${CS2_DIR}/game/csgo/gameinfo.gi"
if [ -f "${GAMEINFO}" ]; then
    # Remove old entries
    sed -i "/csgo\/sharp/d" "${GAMEINFO}"
    sed -i "/csgo\/addons\/metamod/d" "${GAMEINFO}"
    # Add "Game sharp" if not present (before "Game csgo")
    if ! grep -q "Game[[:space:]]*sharp" "${GAMEINFO}"; then
        sed -i '/Game_LowViolence.*csgo_lv/a\\t\t\tGame\tsharp' "${GAMEINFO}"
        log "Patched gameinfo.gi: added Game sharp"
    fi
fi

# Restore original libserver.so if previous runs messed with it
ORIGINAL="${CS2_DIR}/game/csgo/bin/linuxsteamrt64/libserver.so"
for backup in "${ORIGINAL}.valve_backup" "${ORIGINAL%/*}/libserver_original.so" "${ORIGINAL%/*}/libserver_valve.so"; do
    if [ -f "${backup}" ]; then
        [ -L "${ORIGINAL}" ] && rm -f "${ORIGINAL}"
        [ ! -f "${ORIGINAL}" ] && mv "${backup}" "${ORIGINAL}" && log "Restored original libserver.so from ${backup}"
    fi
done
rm -f "${CS2_DIR}/.modsharp-preload"

# Write the version stamp
echo "${IMAGE_VERSION}" > "${STAMP_FILE}"

log "Overlay applied successfully (version: ${IMAGE_VERSION})."
