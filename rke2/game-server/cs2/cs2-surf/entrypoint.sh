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

# ── Step 0a: Set up Steam client libraries ─────────────────────────────────
# Steamcmd must run once to bootstrap its own libraries
if [ ! -f "${STEAMCMD_DIR}/linux64/steamclient.so" ]; then
    log "Bootstrapping steamcmd..."
    "${STEAMCMD_DIR}/steamcmd.sh" +quit
fi
mkdir -p /home/steam/.steam/sdk64 /home/steam/.steam/sdk32
ln -sf "${STEAMCMD_DIR}/linux64/steamclient.so" /home/steam/.steam/sdk64/steamclient.so
ln -sf "${STEAMCMD_DIR}/linux32/steamclient.so" /home/steam/.steam/sdk32/steamclient.so
log "Steam client libraries linked"

# ── Step 0b: Symlink cs2-dedicated -> cs2 for path compatibility ────────────
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
DB_TARGET="${CS2_DIR}/game/sharp/configs/timer.jsonc"

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

# ── Step 3c: Inject MySQL connection string into core.json ─────────────────
# Sharp.Modules.AdminCommands.SQLStorage reads its connection string from
# core.json under ConnectionStrings.AdminCommands.SQLStorage. ModSharp
# ships core.json with a placeholder (`User ID=your_user_name_goes_here`)
# which makes SQLStorage fail on every boot. Replace the entire
# AdminCommands.SQLStorage line with one built from MYSQL_USER /
# MYSQL_PASS / MYSQL_HOST / MYSQL_PORT env vars (sourced from the
# mysql-secret in the deployment).
CORE_JSON="${CS2_DIR}/game/sharp/configs/core.json"
if [ -f "${CORE_JSON}" ] && [ -n "${MYSQL_USER:-}" ] && [ -n "${MYSQL_PASS:-}" ]; then
    CLEAN_USER=$(printf '%s' "${MYSQL_USER}" | tr -d '\r\n')
    CLEAN_PASS=$(printf '%s' "${MYSQL_PASS}" | tr -d '\r\n')
    CLEAN_HOST=$(printf '%s' "${MYSQL_HOST:-10.43.43.43}" | tr -d '\r\n')
    CLEAN_PORT=$(printf '%s' "${MYSQL_PORT:-3306}" | tr -d '\r\n')
    CONN="mysql://Server=${CLEAN_HOST};Port=${CLEAN_PORT};Database=modsharp;User ID=${CLEAN_USER};Password=${CLEAN_PASS};"
    python3 - "${CORE_JSON}" "${CONN}" <<'PY'
import sys, re
path, conn = sys.argv[1], sys.argv[2]
with open(path) as f:
    src = f.read()
# Replace the entire AdminCommands.SQLStorage line value, preserving
# whatever quoting / formatting ModSharp chose.
new = re.sub(
    r'("AdminCommands\.SQLStorage"\s*:\s*")[^"]*(")',
    lambda m: m.group(1) + conn + m.group(2),
    src,
)
if new != src:
    with open(path, 'w') as f:
        f.write(new)
    print("core.json: AdminCommands.SQLStorage rewritten")
else:
    print("core.json: AdminCommands.SQLStorage line not found")
PY
    log "Wrote MySQL connection string into core.json"
else
    log "Skipping core.json MySQL injection (file or env vars missing)"
fi

# ── Step 3d: Re-enable RampFix if disabled ─────────────────────────────────
# ModSharp's RampFix module ships with a .disabled marker in some builds;
# surf servers want it on. Remove the marker so the loader picks it up.
RAMPFIX_DISABLED="${CS2_DIR}/game/sharp/modules/RampFix/RampFix.disabled"
if [ -f "${RAMPFIX_DISABLED}" ]; then
    rm -f "${RAMPFIX_DISABLED}"
    log "Removed RampFix.disabled marker"
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

log "Starting CS2 surf server..."
log "  Map:        ${MAP:-de_dust2} (startup: built-in)"
log "  Port:       ${PORT:-27015}"
log "  MaxPlayers: ${MAXPLAYERS:-32}"
log "  Tickrate:   ${TICKRATE:-128}"
if [ -n "${STARTUP_MAP:-}" ]; then
    log "  StartupMap: workshop ${STARTUP_MAP} (will switch after server is ready)"
fi

# Workshop map post-boot switch.
#
# We tried booting directly onto the workshop map with -dual_addon + AddonManager,
# but when the addon isn't already mounted the engine sleeps forever before
# binding a port. Falling back to the proven pattern: boot onto de_dust2 so the
# port binds fast, then RCON host_workshop_map once the server is listening.
# (CS2 has the workshop vpk on disk from an earlier subscribe.)
if [ -n "${STARTUP_MAP:-}" ] && [ -n "${RCON_PASSWORD:-}" ]; then
    # CS2 binds the game port to the pod's primary interface IP, NOT to
    # 127.0.0.1 or 0.0.0.0 — a /dev/tcp/127.0.0.1/$PORT probe will always
    # refuse, and RCON to localhost will never connect. Use hostname -i.
    SERVER_IP=$(hostname -i | awk '{print $1}')
    (
        sleep 45  # wait for server to fully start and mount workshop content
        for i in $(seq 1 30); do
            if bash -c "echo > /dev/tcp/${SERVER_IP}/${PORT:-27015}" 2>/dev/null; then
                log "Server ready at ${SERVER_IP}:${PORT:-27015} — switching to workshop map ${STARTUP_MAP}"
                # Step 1: flip to the workshop map. host_workshop_map kills the
                # current netsession, so the next RCON burst needs a fresh TCP
                # connection after the new map has fully loaded.
                python3 -c "
import socket, struct
def rcon(host, port, pw, cmd):
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.settimeout(10)
    s.connect((host, port))
    payload = struct.pack('<ii', 1, 3) + pw.encode() + b'\x00\x00'
    s.sendall(struct.pack('<i', len(payload)) + payload)
    s.recv(4096)
    payload = struct.pack('<ii', 2, 2) + cmd.encode() + b'\x00\x00'
    s.sendall(struct.pack('<i', len(payload)) + payload)
    s.close()
rcon('${SERVER_IP}', ${PORT:-27015}, '${RCON_PASSWORD}', 'host_workshop_map ${STARTUP_MAP}')
" 2>/dev/null && log "Map switch command sent" && break
                sleep 5
            fi
            sleep 10
        done

        # Step 2: map change resets surf movement cvars back to CS2 defaults.
        # Force them back (and re-exec server.cfg for everything else) ~20s
        # after the switch, when the new map is loaded and RCON is accepting
        # commands again. Retry a few times in case the map is still loading.
        force_surf_cvars() {
            local retries=5
            local i
            for i in $(seq 1 $retries); do
                if bash -c "echo > /dev/tcp/${SERVER_IP}/${PORT:-27015}" 2>/dev/null; then
                    python3 -c "
import socket, struct
def rcon(host, port, pw, cmd):
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.settimeout(10)
    s.connect((host, port))
    s.sendall(struct.pack('<iii', len(pw)+10, 0, 3) + pw.encode() + b'\x00\x00')
    s.recv(4096)
    s.sendall(struct.pack('<iii', len(cmd)+10, 1, 2) + cmd.encode() + b'\x00\x00')
    s.close()
cmds = [
    'sv_enablebunnyhopping 1',
    'sv_autobunnyhopping 1',
    'sv_airaccelerate 150',
    'sv_accelerate 10',
    'sv_friction 4',
    'sv_staminamax 0',
    'sv_staminajumpcost 0',
    'sv_staminalandcost 0',
    'sv_gravity 800',
    'sv_maxvelocity 3500',
    'exec server.cfg',
]
for c in cmds:
    rcon('${SERVER_IP}', ${PORT:-27015}, '${RCON_PASSWORD}', c)
" 2>/dev/null && return 0
                fi
                sleep 5
            done
            return 1
        }

        rcon_cmd() {
            local cmd="$1"
            python3 -c "
import socket, struct
s = socket.socket()
s.settimeout(10)
s.connect(('${SERVER_IP}', ${PORT:-27015}))
pw = '${RCON_PASSWORD}'
cmd = '${cmd}'
s.sendall(struct.pack('<iii', len(pw)+10, 0, 3) + pw.encode() + b'\x00\x00')
s.recv(4096)
s.sendall(struct.pack('<iii', len(cmd)+10, 1, 2) + cmd.encode() + b'\x00\x00')
s.close()
" 2>/dev/null
        }

        sleep 20
        if force_surf_cvars; then
            log "Surf cvars forced"
        fi

        # Map rotation is now owned by the SurfMapCommand ModSharp plugin
        # (configs/maprotation.txt + !rtv / !extend / !nominate). The only
        # remaining entrypoint job is the post-boot cvar force above, so
        # this subshell exits once that's done.
    ) &
fi

exec "${CS2_DIR}/game/bin/linuxsteamrt64/cs2" \
    -dedicated \
    -console \
    -usercon \
    -autoupdate \
    -port "${PORT:-27015}" \
    -tickrate "${TICKRATE:-128}" \
    -authkey "${API_KEY}" \
    +map "${MAP:-de_dust2}" \
    +sv_setsteamaccount "${STEAM_ACCOUNT}" \
    +rcon_password "${RCON_PASSWORD}" \
    +sv_visiblemaxplayers "${MAXPLAYERS:-32}" \
    +game_type "${GAME_TYPE:-0}" \
    +game_mode "${GAME_MODE:-0}" \
    +hostname "${SERVER_NAME:-CS2 Surf Server}"
