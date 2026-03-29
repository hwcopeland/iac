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
# ModSharp loader installation:
# The shim at sharp/bin/linuxsteamrt64/libserver.so must REPLACE the original
# at csgo/bin/linuxsteamrt64/libserver.so. The shim's dlopen uses a path
# relative to CWD (game/bin/linuxsteamrt64/) to load the original:
#   ../../csgo/bin/linuxsteamrt64/libserver.so
# Since the shim IS at that path now, we need the original available there too.
# Solution: the shim replaces the file, but we keep the original as
# libserver_original.so and patch the shim... no that won't work.
#
# Actual solution from the loader source: the shim is loaded by the engine,
# calls dlopen on the relative path which resolves to the same file (itself).
# dlopen returns the already-loaded handle. Then CreateInterface calls the
# shim's own CreateInterface in a loop.
#
# The REAL install method: the shim must be at game/csgo/bin/linuxsteamrt64/
# AND the original must be loadable. Looking at CS2-Egg and other installs,
# ModSharp uses LD_PRELOAD or the engine loads it differently.
#
# For now: use gameinfo.gi Game search path so engine finds shim first,
# keep original untouched at csgo/bin/linuxsteamrt64/libserver.so.
# The shim's dlopen will load the original (different inode, different path).
GAMEINFO="${CS2_DIR}/game/csgo/gameinfo.gi"
if [ -f "${GAMEINFO}" ] && ! grep -q "csgo/sharp" "${GAMEINFO}"; then
    sed -i 's/^\(\s*Game\s*csgo\s*$\)/\t\t\tGame\tcsgo\/sharp\n\1/' "${GAMEINFO}"
    log "Patched gameinfo.gi with csgo/sharp search path"
fi

# Restore original if we previously replaced it
VALVE_BACKUP="${CS2_DIR}/game/csgo/bin/linuxsteamrt64/libserver_valve.so"
ORIGINAL="${CS2_DIR}/game/csgo/bin/linuxsteamrt64/libserver.so"
if [ -f "${VALVE_BACKUP}" ]; then
    mv "${VALVE_BACKUP}" "${ORIGINAL}"
    log "Restored original libserver.so"
fi

# Write the version stamp
echo "${IMAGE_VERSION}" > "${STAMP_FILE}"

log "Overlay applied successfully (version: ${IMAGE_VERSION})."
