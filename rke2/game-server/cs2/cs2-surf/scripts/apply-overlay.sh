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
# 1. Rename original libserver.so so engine won't find it at csgo/bin/
# 2. Engine finds the shim at csgo/sharp/bin/ via gameinfo.gi
# 3. Shim's dlopen("../../csgo/bin/linuxsteamrt64/libserver.so") from CWD
#    resolves to game/csgo/bin/linuxsteamrt64/libserver.so
# 4. We symlink that path to the renamed original so the shim finds it
ORIGINAL="${CS2_DIR}/game/csgo/bin/linuxsteamrt64/libserver.so"
BACKUP="${CS2_DIR}/game/csgo/bin/linuxsteamrt64/libserver_original.so"
SHIM="${CS2_DIR}/game/csgo/sharp/bin/linuxsteamrt64/libserver.so"

if [ -f "${SHIM}" ] && [ -f "${ORIGINAL}" ] && [ ! -L "${ORIGINAL}" ]; then
    # Only if original is real file (not already a symlink from previous run)
    SIZE=$(stat -c%s "${ORIGINAL}")
    if [ "$SIZE" -gt 100000 ]; then
        # Original is the real 38MB server lib, not the 17KB shim
        mv "${ORIGINAL}" "${BACKUP}"
        # Symlink so shim's relative dlopen still finds "libserver.so" here
        # but it loads the original (different inode than the shim)
        ln -sf "${BACKUP}" "${ORIGINAL}"
        log "Moved original libserver.so -> libserver_original.so, symlinked back"
    fi
fi

# Patch gameinfo.gi to add csgo/sharp search path
GAMEINFO="${CS2_DIR}/game/csgo/gameinfo.gi"
if [ -f "${GAMEINFO}" ] && ! grep -q "csgo/sharp" "${GAMEINFO}"; then
    sed -i 's/^\(\s*Game\s*csgo\s*$\)/\t\t\tGame\tcsgo\/sharp\n\1/' "${GAMEINFO}"
    log "Patched gameinfo.gi with csgo/sharp search path"
fi

# No LD_PRELOAD needed with this approach
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
