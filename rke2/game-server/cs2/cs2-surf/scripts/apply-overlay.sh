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

# ModSharp's loader shim MUST replace the original libserver.so
# The shim is at sharp/bin/linuxsteamrt64/libserver.so (17KB)
# The original is at bin/linuxsteamrt64/libserver.so (38MB)
# ModSharp's loader shim lives at game/sharp/bin/linuxsteamrt64/libserver.so
# The engine loads it automatically — no gameinfo.gi patch or replacement needed.
# The shim's dlopen("../../csgo/bin/linuxsteamrt64/libserver.so") from CWD
# game/bin/linuxsteamrt64/ resolves to the original server lib.

# Clean up any previous bad installs
GAMEINFO="${CS2_DIR}/game/csgo/gameinfo.gi"
[ -f "${GAMEINFO}" ] && sed -i "/csgo\/sharp/d" "${GAMEINFO}" 2>/dev/null

# Restore original if we previously moved/symlinked it
ORIGINAL="${CS2_DIR}/game/csgo/bin/linuxsteamrt64/libserver.so"
BACKUP="${CS2_DIR}/game/csgo/bin/linuxsteamrt64/libserver_original.so"
VALVE="${CS2_DIR}/game/csgo/bin/linuxsteamrt64/libserver_valve.so"
if [ -L "${ORIGINAL}" ]; then rm -f "${ORIGINAL}"; fi
[ -f "${BACKUP}" ] && mv "${BACKUP}" "${ORIGINAL}"
[ -f "${VALVE}" ] && [ ! -f "${ORIGINAL}" ] && mv "${VALVE}" "${ORIGINAL}"
rm -f "${CS2_DIR}/.modsharp-preload"

# Restore original libserver.so if we previously replaced it
VALVE_BACKUP="${CS2_DIR}/game/csgo/bin/linuxsteamrt64/libserver_valve.so"
ORIGINAL="${CS2_DIR}/game/csgo/bin/linuxsteamrt64/libserver.so"
if [ -f "${VALVE_BACKUP}" ]; then
    mv "${VALVE_BACKUP}" "${ORIGINAL}"
    log "Restored original libserver.so"
fi

# Write the version stamp
echo "${IMAGE_VERSION}" > "${STAMP_FILE}"

log "Overlay applied successfully (version: ${IMAGE_VERSION})."
