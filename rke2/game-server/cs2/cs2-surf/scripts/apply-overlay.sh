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

# ── Clean up kus remnants from PVC ──────────────────────────────────────────
# Remove .disabled module dirs left by kus-era disable/enable toggling
SHARP_MODULES="${CS2_DIR}/game/sharp/modules"
if [ -d "${SHARP_MODULES}" ]; then
    disabled_count=0
    for d in "${SHARP_MODULES}"/*.disabled; do
        [ -d "$d" ] && rm -rf "$d" && disabled_count=$((disabled_count + 1))
    done
    [ "$disabled_count" -gt 0 ] && log "Removed ${disabled_count} .disabled module dirs from PVC"
fi

# Wipe kus cfg files (25-mode configs: 1v1, aim, awp, bhop, comp, etc.)
# Keep only Valve defaults (boot.vcfg, banned_*.cfg) and our server.cfg
CFG_DIR="${CS2_DIR}/game/csgo/cfg"
if [ -d "${CFG_DIR}" ]; then
    kus_count=0
    for f in "${CFG_DIR}"/*.cfg; do
        [ ! -f "$f" ] && continue
        fname="$(basename "$f")"
        case "${fname}" in
            server.cfg|banned_ip.cfg|banned_user.cfg) continue ;;
        esac
        rm -f "$f" && kus_count=$((kus_count + 1))
    done
    # Remove kus game-mode subdirs
    for d in cs2-executes cs2-retakes sharptimer; do
        [ -d "${CFG_DIR}/${d}" ] && rm -rf "${CFG_DIR}/${d}" && kus_count=$((kus_count + 1))
    done
    # Remove kus GeoIP database
    rm -f "${CFG_DIR}/GeoLite2-Country.mmdb" && kus_count=$((kus_count + 1))
    [ "$kus_count" -gt 0 ] && log "Cleaned ${kus_count} kus-era files/dirs from cfg/"
fi

# Copy overlay contents into the game directory
# ModSharp installs to game/sharp/ (NOT game/csgo/sharp/)
# The zip extracts as sharp/ which goes directly under game/
cp -rf "${OVERLAY_DIR}/." "${CS2_DIR}/game/"

# ── Deploy baked-in configs to PVC ──────────────────────────────────────────
CONFIGS_DIR="/opt/cs2-surf/configs"

# server.cfg → csgo/cfg/
if [ -f "${CONFIGS_DIR}/server.cfg" ]; then
    mkdir -p "${CS2_DIR}/game/csgo/cfg"
    cp -f "${CONFIGS_DIR}/server.cfg" "${CS2_DIR}/game/csgo/cfg/server.cfg"
    log "Deployed server.cfg"
fi

# gamemodes_server.txt → csgo/
if [ -f "${CONFIGS_DIR}/gamemodes_server.txt" ]; then
    cp -f "${CONFIGS_DIR}/gamemodes_server.txt" "${CS2_DIR}/game/csgo/gamemodes_server.txt"
    log "Deployed gamemodes_server.txt"
fi

# mapcycle.txt → csgo/ (native CS2 map rotation)
if [ -f "${CONFIGS_DIR}/mapcycle.txt" ]; then
    cp -f "${CONFIGS_DIR}/mapcycle.txt" "${CS2_DIR}/game/csgo/mapcycle.txt"
    log "Deployed mapcycle.txt"
fi

# subscribed_file_ids.txt → csgo/ (workshop maps)
if [ -f "${CONFIGS_DIR}/subscribed_file_ids.txt" ]; then
    cp -f "${CONFIGS_DIR}/subscribed_file_ids.txt" "${CS2_DIR}/game/csgo/subscribed_file_ids.txt"
    log "Deployed subscribed_file_ids.txt"
fi

# timer-replay.jsonc → sharp/configs/ (disable replay bot spam)
if [ -f "${CONFIGS_DIR}/timer-replay.jsonc" ]; then
    mkdir -p "${CS2_DIR}/game/sharp/configs"
    cp -f "${CONFIGS_DIR}/timer-replay.jsonc" "${CS2_DIR}/game/sharp/configs/timer-replay.jsonc"
    log "Deployed timer-replay.jsonc"
fi

# admins.jsonc → sharp/configs/ (ModSharp admin system)
if [ -f "${CONFIGS_DIR}/admins.jsonc" ]; then
    mkdir -p "${CS2_DIR}/game/sharp/configs"
    cp -f "${CONFIGS_DIR}/admins.jsonc" "${CS2_DIR}/game/sharp/configs/admins.jsonc"
    log "Deployed admins.jsonc"
fi

# maprotation.txt → sharp/configs/ (workshop ID rotation list)
# Always overwrite from the image — git is the source of truth for the
# full rotation list. Admin !addmap / !removemap edits on the PVC are
# transient (they persist until the next deploy, which is fine for
# testing maps before committing them to git).
if [ -f "${CONFIGS_DIR}/maprotation.txt" ]; then
    mkdir -p "${CS2_DIR}/game/sharp/configs"
    cp -f "${CONFIGS_DIR}/maprotation.txt" "${CS2_DIR}/game/sharp/configs/maprotation.txt"
    log "Deployed maprotation.txt"
fi

# mapnames.txt → sharp/configs/ (name → workshop id resolver)
# SurfMapCommand reads this so admins can `!map surf_kitsune` instead of
# memorizing publish file ids. Always overwrite — this is a baked
# reference table, not user state.
if [ -f "${CONFIGS_DIR}/mapnames.txt" ]; then
    mkdir -p "${CS2_DIR}/game/sharp/configs"
    cp -f "${CONFIGS_DIR}/mapnames.txt" "${CS2_DIR}/game/sharp/configs/mapnames.txt"
    log "Deployed mapnames.txt"
fi

# maptiers.txt → sharp/configs/ (tier data for vote display)
if [ -f "${CONFIGS_DIR}/maptiers.txt" ]; then
    mkdir -p "${CS2_DIR}/game/sharp/configs"
    cp -f "${CONFIGS_DIR}/maptiers.txt" "${CS2_DIR}/game/sharp/configs/maptiers.txt"
    log "Deployed maptiers.txt"
fi

# Clean up modules dropped from the image (MCS, AddonManager, the Tnms set).
# apply-overlay does cp -rf which adds + overwrites but doesn't remove, so
# deletions upstream in the image need explicit cleanup here.
for dead in MapChooserSharpMS AddonManager TnmsLocalizationPlatform \
            TnmsAdministrationPlatform TnmsExtendableTargeting; do
    if [ -d "${CS2_DIR}/game/sharp/modules/${dead}" ]; then
        rm -rf "${CS2_DIR}/game/sharp/modules/${dead}"
        log "Removed stale module ${dead}"
    fi
done
for dead in MapChooserSharpMS.Shared TnmsLocalizationPlatform.Shared \
            TnmsAdministrationPlatform.Shared TnmsExtendableTargeting.Shared \
            TnmsPluginFoundation; do
    if [ -d "${CS2_DIR}/game/sharp/shared/${dead}" ]; then
        rm -rf "${CS2_DIR}/game/sharp/shared/${dead}"
        log "Removed stale shared ${dead}"
    fi
done
# addon_manager.jsonc is also dead
rm -f "${CS2_DIR}/game/sharp/configs/addon_manager.jsonc"

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
