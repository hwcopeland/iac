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
cp -rf "${OVERLAY_DIR}/." "${CS2_DIR}/game/csgo/"

# ModSharp's loader shim MUST replace the original libserver.so
# The shim is at sharp/bin/linuxsteamrt64/libserver.so (17KB)
# The original is at bin/linuxsteamrt64/libserver.so (38MB)
SHIM="${CS2_DIR}/game/csgo/sharp/bin/linuxsteamrt64/libserver.so"
ORIGINAL="${CS2_DIR}/game/csgo/bin/linuxsteamrt64/libserver.so"
if [ -f "${SHIM}" ]; then
    if [ -f "${ORIGINAL}" ]; then
        cp "${ORIGINAL}" "${ORIGINAL}.valve_backup"
        log "Backed up original libserver.so ($(stat -c%s "${ORIGINAL}") bytes)"
    fi
    cp -f "${SHIM}" "${ORIGINAL}"
    log "Installed ModSharp loader shim as libserver.so ($(stat -c%s "${ORIGINAL}") bytes)"
fi

# Write the version stamp
echo "${IMAGE_VERSION}" > "${STAMP_FILE}"

log "Overlay applied successfully (version: ${IMAGE_VERSION})."
