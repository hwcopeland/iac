---
project: "cs2"
maturity: "draft"
last_updated: "2026-03-28"
updated_by: "@staff-engineer"
scope: "Replace kus deployment with a surf-only custom Docker image using ModSharp framework and Source2Surf/Timer, with CI/CD to Kubernetes"
owner: "@staff-engineer"
dependencies: []
---

# TDD: CS2 Surf Server -- ModSharp + Source2Surf/Timer

## 1. Problem Statement

### What

Replace the existing `ghcr.io/kus/cs2-modded-server` deployment in-place with a purpose-built surf-only CS2 server image (`ghcr.io/hwcopeland/cs2-surf-server`). The new image uses the **ModSharp** framework (not CounterStrikeSharp) and **Source2Surf/Timer** (not SharpTimer) for surf timing. The deployment takes over the same Kubernetes service IP (`10.44.0.32`), same ports, same DNS, and same MySQL backend. The kus deployment is scaled to zero and effectively retired.

### Why Now

The kus image is a multi-mode server designed to switch between 25+ game modes at runtime. Using it as a dedicated surf server creates cascading problems:

1. **Startup overwrite cycle**: `install_docker.sh` runs `cp -R /home/cs2-modded-server/game/csgo/ /home/steam/cs2/game/` on every pod start, overwriting the entire game directory. Custom configs survive only through a secondary `cp -RT /home/custom_files/` merge. This forces all customization through an init container hack chain.

2. **648 MB of unused plugins**: The kus image bundles 30+ CounterStrikeSharp plugins (MatchZy, K4-Arenas, Deathmatch, Retakes, etc.) that are never loaded in surf mode. The `unload_plugins.cfg` -> `surf.cfg` -> load-disabled-plugins dance is fragile.

3. **Workshop map mounting is broken by default**: `changelevel` fails for workshop maps unless MultiAddonManager explicitly registers them. The kus image does not configure this -- it was patched in via ConfigMap and init container hacks.

4. **SharpTimer replay bot dies on map change**: The 0.3.1y replay bot spawns on first map load after server start, then silently fails to respawn after any `changelevel`. Source2Surf/Timer fixes this by design -- it uses native `CCSBotManager::BotAddCommand` hooks and re-registers bot state per map via `RepeatCallThisMap(3.0f)`.

5. **GMM v1.0.62 bundled by kus has KeyNotFoundException on workshop vote options**: Fixed by downloading v1.0.63 in the init container on every pod start -- an additional 30-second startup tax.

6. **Disk waste**: 200 GB PVC stores 37 GB of workshop content for ALL game modes (161 workshop IDs), 53 GB of game files, and 6.4 GB of community addons. Surf needs ~20 workshop maps.

7. **Framework opportunity**: ModSharp is a standalone native loader that does NOT require Metamod. This eliminates the Metamod dependency chain and the associated `gameinfo.gi` patching. Source2Surf/Timer is purpose-built for surf by the ModSharp creators (Nukoooo and Kxnrl), with native bot management that solves the replay bot issue without any code patches.

### Why ModSharp over CounterStrikeSharp

The original TDD (v1) designed around CounterStrikeSharp + SharpTimer. This revision switches to ModSharp + Source2Surf/Timer for three reasons:

1. **Replay bot fix is built-in.** Source2Surf/Timer hooks `CCSBotManager::BotAddCommand` and `MaintainBotQuota` natively, re-registering bot state on every map via `RepeatCallThisMap(3.0f)`. The SharpTimer stale `replayBotController` reference bug does not exist. This eliminates the need to fork SharpTimer and maintain a patched build.

2. **No Metamod dependency.** ModSharp is a standalone native loader. CounterStrikeSharp requires Metamod:Source, which requires `gameinfo.gi` patching on every CS2 update. ModSharp loads directly, reducing the startup complexity and eliminating a failure mode.

3. **Ecosystem alignment.** Source2Surf/Timer is a full surf timer (zones, records, replays, replay bots, styles, stages, HUD) built on ModSharp by ModSharp's own creators. The plugin ecosystem (MapChooserSharpMS, ms-advertisement, Rampfix, MS-NoBlock) is native ModSharp. There is no impedance mismatch between framework and plugins.

**Trade-offs acknowledged:** ModSharp is newer (77 stars vs CSS's established base), has a smaller plugin ecosystem, and the AGPL-3.0 license means derived works must be open-source (acceptable for a game server config, not for proprietary code). The smaller ecosystem is mitigated by us being surf-only -- we need fewer plugins, and the ones we need exist.

### Constraints

- Existing MySQL database (ClusterIP `10.43.43.43`, database `sharptimer`) stays. Source2Surf/Timer uses its own schema via SqlSugar ORM -- a new database will be created alongside the existing `sharptimer` database.
- The deployment replaces kus in-place. Same Service IP `10.44.0.32`, same ports, same DNS. No new firewall rules, no new DNS entries.
- Reuses existing secrets: `cs2-secret` (API_KEY, STEAM_ACCOUNT), `cs2-rcon` (RCON password), `mysql-secret` (MySQL credentials).
- Must work on the existing 4-node RKE2 cluster (K8s 1.34.3, Longhorn storage, Cilium CNI with LB-IPAM).
- Workshop maps are downloaded by the CS2 engine at runtime via `-authkey` and `subscribed_file_ids.txt`. They cannot be baked into the Docker image.

### Acceptance Criteria

1. `changelevel` works for all surf workshop maps after any number of map changes (not just the first).
2. RTV vote triggers, completes, and changes map correctly for workshop maps.
3. Replay bot spawns on every map that has a stored server record, including after map changes -- no stale reference, no manual intervention.
4. Server starts from cold (no PVC cache) in under 3 minutes (excluding initial CS2 download).
5. CI/CD builds and pushes the image to GHCR on git push to main, and deploys to the cluster.
6. Existing DNS and IP (`10.44.0.32:27015`) connects to the surf server.
7. No init container hacks for plugin installation or version pinning -- all plugins baked into the image.
8. Init container does only one thing: `envsubst` for database credentials.
9. Source2Surf/Timer stores records in MySQL, leaderboards work, replays play.
10. Surf movement cvars are applied correctly (airaccelerate 150, autobhop, etc.).

## 2. Context & Prior Art

### Current Architecture (kus)

```
Pod: cs2-modded-server
  initContainers:
    envsubst-config (alpine:3.20)
      - envsubst mysqlConfig.json (MYSQL_USER/MYSQL_PASS)
      - cp ConfigMap files -> custom_files PVC
      - wget GMM v1.0.63 -> custom_files PVC
      - append missing SharpTimer cvars -> custom_files PVC
  containers:
    cs2-modded-server (ghcr.io/kus/cs2-modded-server:latest)
      entrypoint: install_docker.sh
        1. steamcmd app_update 730 (CS2 update)
        2. cp -R /home/cs2-modded-server/game/csgo/ -> game PVC (OVERWRITES EVERYTHING)
        3. cp -RT /home/custom_files/ -> game PVC (merges custom_files PVC)
        4. Patch gameinfo.gi for Metamod
        5. Launch cs2 dedicated server

Volumes:
  cs2-data PVC (200 Gi, Longhorn RWX) -- game files + workshop downloads
  custom_files PVC (1 Gi, Longhorn RWX) -- config overrides, root-owned
  addon-config ConfigMap -- MAM cfg, surf mapexec, GMM json, MAM vdf

Service: cs2-modded-service
  IP: 10.44.0.32 (Cilium LB-IPAM)
  Ports: 27015 TCP/UDP, 27020 TCP/UDP
```

**Problems in detail:**

- `install_docker.sh` is the image ENTRYPOINT. It runs as root, creates the `steam` user, installs steamcmd, downloads CS2, copies kus defaults over the PVC, then merges custom_files, then launches the server as the `steam` user. Every pod restart re-downloads CS2 updates and re-copies 648 MB of plugins.

- The kus plugin loading model: `server.cfg` runs on every map load, calls `unload_plugins.cfg` (which unloads ~20 mode-specific plugins), then the mode cfg (e.g., `surf.cfg`) loads the ones it needs from `plugins/disabled/`. This load/unload cycle causes brief plugin state loss.

- Workshop maps: CS2 downloads workshop VPKs to `steamapps/workshop/content/730/{ID}/`. For `changelevel` to work with these maps, MAM must mount the VPKs. GMM reads `gamemodes_server.txt` for map names. When `changelevel` fails (VPK not mounted), GMM v1.0.63 falls back to `host_workshop_map {ID}`.

### ModSharp Framework

ModSharp (`Kxnrl/modsharp-public`, AGPL-3.0) is a standalone C# plugin framework for CS2 on .NET 10.0. Key architectural differences from CounterStrikeSharp:

- **No Metamod dependency.** ModSharp loads via its own native loader, not through the Metamod:Source plugin chain. No `gameinfo.gi` patching required.
- **Module system.** Built-in modules: MenuManager, ClientPreferences, LocalizerManager, InputManager, EntityEnhancements, AdminFlatFile.
- **Plugin directory:** Plugins install to `sharp/` under the game directory (not `addons/counterstrikesharp/plugins/`).
- **NuGet package:** `ModSharp.Sharp.Shared` for plugin development.
- **Release model:** ~109 releases on GitHub, actively developed (last update March 2026).
- **Installation:** Download release archive, extract to game root. The loader installs itself without Metamod's gameinfo.gi injection.

### Source2Surf/Timer

Source2Surf/Timer (`Source2Surf/Timer`, AGPL-3.0) by Nukoooo and Kxnrl (ModSharp creators). Key features:

- **Full surf timer:** Zones, records, replays, replay bots, styles (normal, sideways, W-only, etc.), stages, HUD.
- **Database:** MySQL + PostgreSQL via SqlSugar ORM, LiteDB fallback for development. Schema is managed by SqlSugar migrations -- tables auto-create on first run.
- **Replay bot architecture:** Uses native `CCSBotManager::BotAddCommand`, hooks `MaintainBotQuota`. Bot state re-registered per map via `RepeatCallThisMap(3.0f)`. This fundamentally solves the SharpTimer stale reference bug -- there is no cached controller reference that can go stale.
- **MapInfo module:** Handles per-map settings (movement cvars, zone definitions, stage data). Can apply surf-specific cvars automatically based on map configuration.
- **.NET 10**, last updated March 18, 2026.

### K4ryuu/CS2-Egg Architecture (Reference)

CS2-Egg (`K4ryuu/CS2-Egg`) is a Pterodactyl egg for CS2 servers. We adapt its patterns for our Dockerfile, not its Pterodactyl-specific machinery:

- **Modular updater scripts:** Individual bash functions pull framework releases from GitHub with semver version pinning.
- **Clean entrypoint:** steamcmd update -> addon/framework updates -> launch.
- **Framework toggle flags:** Environment variables control which frameworks are installed (we only need ModSharp).
- **Version comparison logic:** Compare installed version against desired version, skip download if current.

We cherry-pick the updater pattern (download-from-GitHub-release, version-compare, extract-to-correct-path) and discard the Pterodactyl startup/egg variable system.

### Prior Art

- **cm2network/steamcmd**: Minimal Docker image with steamcmd pre-installed. Clean starting point.
- **K4ryuu/CS2-Egg**: Reference architecture for modular framework installation. Proven GitHub release download + extract pattern.
- **SourceMod surf server patterns**: The Source 1 community standardized on single-mode images with all plugins baked in. This TDD follows that proven pattern.

## 3. Alternatives Considered

### Alternative A: Continue Patching the kus Image (Status Quo+)

Keep using `ghcr.io/kus/cs2-modded-server:latest`. Extend the init container to handle all workarounds.

**Strengths:**
- No new Docker image to build/maintain.
- Immediate -- no development time.

**Weaknesses:**
- Init container already does 7 distinct operations. Each kus update can break any of them.
- Cannot switch to ModSharp -- kus is hardcoded around CSS and Metamod.
- 648 MB of unused plugins. 161 workshop IDs downloaded when only 20 are needed.
- `install_docker.sh` overwrites game PVC on every pod start.
- Replay bot bug requires forking SharpTimer regardless.

**Verdict:** Incompatible with the ModSharp goal. Also the highest ongoing maintenance cost.

### Alternative B: CounterStrikeSharp + SharpTimer (Original TDD v1)

Build a new image using CSS + Metamod, fork SharpTimer to fix the replay bot, keep GMM for map voting.

**Strengths:**
- Larger CSS plugin ecosystem (more choices for each function).
- CSS is more established and widely used.

**Weaknesses:**
- Requires Metamod, which requires `gameinfo.gi` patching.
- Requires forking SharpTimer and maintaining a patched build for the replay bot fix.
- SharpTimer is a community fork (`Letaryat/poor-sharptimer`) of a fork -- maintenance lineage is uncertain.
- MAM (Metamod plugin) is required for workshop VPK mounting.
- More plugins to maintain (9 vs 6-7 for ModSharp).

**Verdict:** Viable but higher maintenance burden than ModSharp for a surf-specific use case.

### Alternative C: ModSharp + Source2Surf/Timer (Recommended)

Build a new image using ModSharp as the framework and Source2Surf/Timer as the surf timer. Use ModSharp-native plugins for supporting functions.

**Strengths:**
- No Metamod dependency. No `gameinfo.gi` patching.
- Replay bot works out of the box -- native `CCSBotManager` hooks, per-map re-registration.
- No SharpTimer fork to maintain.
- Source2Surf/Timer is purpose-built for surf, by the ModSharp creators. First-class integration.
- Smaller, more focused plugin stack.
- SqlSugar ORM handles database schema automatically.

**Weaknesses:**
- ModSharp is newer, smaller community (77 stars).
- Fewer plugin options if we need something not yet available (e.g., no chat tags plugin yet).
- AGPL-3.0 license on ModSharp and Source2Surf/Timer (acceptable for server deployment).
- Workshop VPK mounting solution needs investigation -- MAM is a Metamod plugin, so we need an alternative approach (see Section 4.2).

**Verdict:** Recommended. The replay bot fix alone justifies the switch -- it eliminates an entire fork-and-maintain workstream. The Metamod removal simplifies the stack. The trade-off (smaller ecosystem) is acceptable for a surf-only server.

### Alternative D: ModSharp + Metamod Side-by-Side (Hybrid)

Run ModSharp for plugins AND Metamod solely for MultiAddonManager (workshop VPK mounting).

**Strengths:**
- Gets MAM's proven workshop VPK mounting.
- ModSharp plugins for everything else.

**Weaknesses:**
- Two framework loaders running simultaneously -- potential conflicts in hooking, entity management, and memory.
- `gameinfo.gi` patching still required for Metamod, defeating one of ModSharp's advantages.
- Untested combination -- no community precedent for running both.

**Verdict:** Risky. Avoid unless native CS2 workshop support proves insufficient (see Section 4.2 fallback path).

## 4. Architecture & System Design

### 4.1 Docker Image: `ghcr.io/hwcopeland/cs2-surf-server`

**Base image:** `cm2network/steamcmd:latest`
- Provides: Ubuntu, steamcmd, `steam` user (UID 1000), `/home/steam/` home directory.
- Does NOT provide: CS2, any mods. Clean starting point.

**Build-time layers (Dockerfile):**

```
Layer 1: Base (cm2network/steamcmd)
Layer 2: System dependencies (gettext for envsubst, unzip, curl, jq)
Layer 3: ModSharp framework (pinned version from GitHub releases)
         - Download: https://github.com/Kxnrl/modsharp-public/releases/download/v{VERSION}/...
         - Installs to: /opt/cs2-surf/overlay/
         - No gameinfo.gi patching needed
Layer 4: Plugins (all pinned versions):
         - Source2Surf/Timer (github.com/Source2Surf/Timer)
         - MapChooserSharpMS (github.com/fltuna/MapChooserSharpMS)
         - ms-advertisement (github.com/partiusfabaa/ms-advertisement)
         - Nukoooo/Rampfix (github.com/Nukoooo/Rampfix)
         - MS-NoBlock (github.com/darkerz7/MS-NoBlock)
         - TnmsAdministrationPlatform + TnmsAdminUtils (github.com/fltuna)
Layer 5: Configuration files:
         - server.cfg (surf defaults)
         - Source2Surf/Timer config (database, zones, styles)
         - MapChooserSharpMS config (RTV, nominations, map list)
         - ms-advertisement config
         - admin config (AdminFlatFile or TnmsAdmin)
         - subscribed_file_ids.txt (surf workshop IDs only)
         - gamemodes_server.txt (for native CS2 mapgroup support)
Layer 6: Entrypoint script (entrypoint.sh)
         - Updater scripts (adapted from CS2-Egg patterns)
```

**Entrypoint design (`entrypoint.sh`):**

Adapted from CS2-Egg's modular updater approach:

```
#!/bin/bash
set -e

# 1. Update CS2 via steamcmd (idempotent, only downloads deltas)
steamcmd +login anonymous +app_update 730 +quit

# 2. Apply ModSharp overlay (version-stamped, skip if current)
apply_overlay_if_needed

# 3. envsubst database credentials into Source2Surf/Timer config
envsubst < /opt/cs2-surf/templates/database.json > $CS2_DIR/game/csgo/sharp/Timer/database.json

# 4. Apply any ConfigMap overlay (mounted at /opt/cs2-surf/overrides/)
if [ -d /opt/cs2-surf/overrides ]; then
    cp -rn /opt/cs2-surf/overrides/* $CS2_DIR/game/csgo/ 2>/dev/null || true
fi

# 5. Launch CS2
exec $CS2_DIR/game/bin/linuxsteamrt64/cs2 \
    -dedicated -console -usercon \
    -tickrate ${TICKRATE:-64} \
    -port ${PORT:-27015} \
    +map ${MAP:-surf_kitsune} \
    +sv_visiblemaxplayers ${MAXPLAYERS:-32} \
    -authkey ${API_KEY} \
    +sv_setsteamaccount ${STEAM_ACCOUNT} \
    +game_type 0 +game_mode 0 \
    +mapgroup mg_surf \
    +host_workshop_collection ${WORKSHOP_COLLECTION_ID} \
    +sv_lan 0 \
    +sv_password "${SERVER_PASSWORD}" \
    +rcon_password "${RCON_PASSWORD}" \
    +exec server.cfg
```

**Key differences from kus:**

| Aspect | kus | New Image |
|---|---|---|
| Framework | CounterStrikeSharp + Metamod | ModSharp (standalone) |
| Timer plugin | SharpTimer 0.3.1y (broken replay bot) | Source2Surf/Timer (native bot management) |
| Plugins installed | 30+ (all modes) | 6-7 (surf only) |
| Startup copy | `cp -R` entire csgo/ dir | No copy -- plugins baked into image overlay dir |
| Plugin loading | unload_plugins.cfg -> mode.cfg -> load disabled | All plugins active, no load/unload cycle |
| gameinfo.gi patching | Required (Metamod) | Not required (ModSharp standalone loader) |
| Config persistence | custom_files PVC merge | Image-baked configs, ConfigMap for overrides only |
| Entrypoint runs as | root, then `sudo -u steam` | steam user throughout |
| Replay bot | Stale reference bug, needs fork to fix | Native CCSBotManager hooks, works by design |
| Image size | ~1.2 GB | ~200 MB (plugins only, CS2 downloaded at runtime) |

**Plugin installation at build time (Dockerfile pattern, adapted from CS2-Egg):**

```dockerfile
# ModSharp framework
ARG MODSHARP_VERSION=1.0.0
RUN curl -sSL "https://github.com/Kxnrl/modsharp-public/releases/download/v${MODSHARP_VERSION}/modsharp-linux.tar.gz" \
    -o /tmp/modsharp.tar.gz \
    && tar -xzf /tmp/modsharp.tar.gz -C /opt/cs2-surf/overlay/ \
    && rm /tmp/modsharp.tar.gz

# Source2Surf/Timer
ARG S2S_TIMER_VERSION=1.0.0
RUN curl -sSL "https://github.com/Source2Surf/Timer/releases/download/v${S2S_TIMER_VERSION}/Timer.zip" \
    -o /tmp/timer.zip \
    && unzip -qo /tmp/timer.zip -d /opt/cs2-surf/overlay/ \
    && rm /tmp/timer.zip

# Pattern repeats for each plugin -- all version-pinned via ARG
```

All plugins are installed into `/opt/cs2-surf/overlay/`. The entrypoint copies this overlay into the CS2 game directory on first start (or when a version mismatch is detected), NOT on every restart. A version stamp file tracks whether the overlay has been applied.

**Overlay application logic:**

```bash
STAMP_FILE="$CS2_DIR/game/csgo/.surf-overlay-version"
IMAGE_VERSION=$(cat /opt/cs2-surf/VERSION)

if [ ! -f "$STAMP_FILE" ] || [ "$(cat $STAMP_FILE)" != "$IMAGE_VERSION" ]; then
    echo "Applying plugin overlay v${IMAGE_VERSION}..."
    cp -r /opt/cs2-surf/overlay/* $CS2_DIR/game/csgo/
    echo "$IMAGE_VERSION" > "$STAMP_FILE"
fi
```

New image build = new overlay version = overlay re-applied on next pod start. Pod restarts with the same image version skip the copy entirely.

### 4.2 Workshop Map VPK Mounting (Critical Design Decision)

**The problem:** MultiAddonManager (MAM) is a Metamod plugin. ModSharp does not use Metamod. Workshop map VPKs need to be mounted for `changelevel` to work.

**Research findings and recommended approach:**

**Option 1: Native CS2 Workshop Support (Recommended -- try first)**

CS2 has built-in workshop support via:
- `+host_workshop_collection {COLLECTION_ID}` on the launch command line
- `-authkey {API_KEY}` for Steam Web API authentication
- `host_workshop_map {WORKSHOP_ID}` console command
- `subscribed_file_ids.txt` for pre-downloading workshop content

When a workshop collection is set via `+host_workshop_collection`, CS2 downloads and mounts all maps in the collection. The `host_workshop_map` command handles VPK mounting natively without MAM.

**How this works with MapChooserSharpMS:**

MapChooserSharpMS needs to be configured to use `host_workshop_map {ID}` for map changes instead of `changelevel workshop/{ID}/{mapname}`. The `host_workshop_map` command is the CS2 engine's native workshop map loading path -- it downloads (if needed), mounts the VPK, and changes the level in one operation. This bypasses the need for pre-mounted VPKs entirely.

**Verification needed:** MapChooserSharpMS's source must be checked to confirm it supports `host_workshop_map` as a map change method, or if it only supports `changelevel`. If it only supports `changelevel`, we may need to contribute a PR or use a wrapper.

**Option 2: Source2Surf/Timer Built-in Map Management (Investigate)**

Source2Surf/Timer may handle workshop map loading as part of its map management. The Timer's MapInfo module manages maps and may use `host_workshop_map` internally. This needs investigation during Phase 1.

**Option 3: Metamod + MAM Alongside ModSharp (Fallback only)**

If Options 1 and 2 prove insufficient, run Metamod solely for MAM. This requires:
- Installing Metamod:Source and patching `gameinfo.gi`
- Installing only MAM as a Metamod plugin
- All other plugins remain ModSharp
- Risk: Two framework loaders in the same process. Test for conflicts.

This is the fallback of last resort. It re-introduces the Metamod dependency we are trying to eliminate.

**Decision checkpoint:** Phase 1 must validate Option 1 (native CS2 workshop support via `host_workshop_map`). If a full `changelevel` -> 5 consecutive map changes test passes without MAM, we proceed with Option 1. If it fails, evaluate Option 2, then Option 3.

### 4.3 Plugin Stack

| Plugin | Purpose | Source | Replaces (from kus) | Notes |
|---|---|---|---|---|
| **ModSharp** | Framework | github.com/Kxnrl/modsharp-public | Metamod + CounterStrikeSharp | Standalone native loader, no gameinfo.gi |
| **Source2Surf/Timer** | Surf timer, zones, records, replays, replay bot, HUD | github.com/Source2Surf/Timer | SharpTimer 0.3.1y + ST-Fixes | Fixes replay bot by design. SqlSugar ORM for DB. |
| **MapChooserSharpMS** | RTV, nominations, map voting | github.com/fltuna/MapChooserSharpMS | GameModeManager v1.0.63 | ~60% parity -- no game mode switching (we don't need it). Surf-only map list. |
| **ms-advertisement** | Server announcements, welcome messages | github.com/partiusfabaa/ms-advertisement | (kus had no equivalent) | Chat/center announcements on timer. |
| **Nukoooo/Rampfix** | Surf ramp bug fix | github.com/Nukoooo/Rampfix | cs2fixes-rampbugfix | Direct parity, native ModSharp. |
| **MS-NoBlock** | Player collision removal | github.com/darkerz7/MS-NoBlock | (handled by SharpTimer cvars) | Dedicated noblock is more reliable. |
| **TnmsAdministrationPlatform** | Admin commands (kick, ban, etc.) | github.com/fltuna | CSS admin module | ~80% parity. Uses ModSharp's AdminFlatFile or its own admin DB. |

**What we no longer need:**
- **Metamod:Source** -- ModSharp is standalone.
- **MultiAddonManager** -- Using native CS2 workshop support (see 4.2).
- **ST-Fixes / BotNavIgnore** -- Source2Surf/Timer handles bot spawning natively.
- **GameModeManager** -- Surf-only server, MapChooserSharpMS handles map voting.
- **MovementUnlocker** -- Source2Surf/Timer or server.cfg cvars handle movement.
- **WASDMenuAPI / MenuManagerAPI / PlayerSettings / CS2-CustomVotes** -- GMM dependencies, no longer needed. ModSharp has built-in MenuManager.

**Version pinning strategy:**

Every plugin version is pinned as a Docker build ARG:

```dockerfile
ARG MODSHARP_VERSION=x.y.z
ARG S2S_TIMER_VERSION=x.y.z
ARG MAPCHOOSER_VERSION=x.y.z
ARG RAMPFIX_VERSION=x.y.z
ARG NOBLOCK_VERSION=x.y.z
ARG ADVERTISEMENT_VERSION=x.y.z
ARG ADMIN_VERSION=x.y.z
```

Updates are explicit: change the ARG, push to main, CI builds and deploys. No silent upstream version drift.

### 4.4 Config Management Strategy

```
Baked into image (Layer 5):
  /opt/cs2-surf/overlay/
    cfg/
      server.cfg              -- surf server defaults (movement cvars, round settings)
    sharp/
      Timer/
        config.json           -- Source2Surf/Timer main config
        maps/                 -- per-map zone/settings files (if pre-configured)
      MapChooserSharpMS/
        config.json           -- RTV settings, map list, vote settings
      ms-advertisement/
        config.json           -- announcement messages and timers
      MS-NoBlock/
        config.json           -- noblock settings
      TnmsAdmin/
        config.json           -- admin settings, permissions
    gamemodes_server.txt      -- surf workshop maps only (for CS2 mapgroup)
    subscribed_file_ids.txt   -- surf workshop IDs only

Templates (envsubst at runtime):
  /opt/cs2-surf/templates/
    database.json             -- ${MYSQL_HOST}, ${MYSQL_USER}, ${MYSQL_PASS}, ${MYSQL_DB}

ConfigMap overlay (optional runtime overrides):
  /opt/cs2-surf/overrides/    -- mounted from K8s ConfigMap, merged on start
```

**Surf movement cvars (in server.cfg):**

These are the same proven values from the existing `surf_.cfg`, applied via `server.cfg` since this is a surf-only server:

```
sv_airaccelerate 150
sv_enablebunnyhopping 1
sv_autobunnyhopping 1
sv_falldamage_scale 0
sv_staminajumpcost 0
sv_staminalandcost 0
sv_friction 5.2
sv_accelerate 6.5
sv_maxvelocity 9876.0
sv_air_max_wishspeed 30.0
sv_gravity 800.0
sv_jump_impulse 301.993378
mp_roundtime 30
mp_respawn_on_death_ct true
mp_respawn_on_death_t true
mp_ignore_round_win_conditions true
```

Source2Surf/Timer's MapInfo module may also apply per-map cvars if specific maps need different settings (e.g., a map designed for different airaccelerate). This is configured in the timer's map data, not server.cfg.

**Secrets flow:**

- `MYSQL_USER` and `MYSQL_PASS`: ExternalSecret -> K8s Secret `mysql-secret` -> env vars -> envsubst at runtime. (Same secret as kus -- reused.)
- `API_KEY` and `STEAM_ACCOUNT`: ExternalSecret -> K8s Secret `cs2-secret` -> env vars. (Same secret as kus -- reused. Same GSLT since we are replacing, not running alongside.)
- `RCON_PASSWORD`: ExternalSecret -> K8s Secret `cs2-rcon` -> env var. (Same secret as kus -- reused.)

No new Bitwarden entries or ExternalSecrets needed. The kus deployment is scaled to 0, so its GSLT is freed for the replacement.

## 5. Data Models & Storage

### Database Strategy

Source2Surf/Timer uses SqlSugar ORM with its own schema. It does NOT use the SharpTimer `sharptimer` database schema. Two options:

**Option A: New database, fresh start (Recommended)**

Create a new MySQL database `source2surf` alongside the existing `sharptimer` database on the same MySQL server (`10.43.43.43`). Source2Surf/Timer's SqlSugar ORM auto-creates tables on first run.

**Rationale:**
- SharpTimer and Source2Surf/Timer schemas are incompatible. There is no way to share the same tables.
- A migration script (SharpTimer records -> Source2Surf/Timer schema) is complex, fragile, and the schemas may differ in how they store styles, stages, and checkpoints.
- The `sharptimer` database is preserved intact. If we ever need to roll back to SharpTimer, the data is there.
- Starting fresh means no risk of schema conflicts or data corruption.

**Trade-off:** Existing player records and server records from SharpTimer will not appear in Source2Surf/Timer. Players start with clean leaderboards. This is acceptable because:
1. The server is transitioning to a fundamentally different timer with different features.
2. Source2Surf/Timer may track different data (styles, stages) that SharpTimer did not.
3. The old records remain in `sharptimer` database as a historical archive.

**Option B: Migration script (Out of scope, documented for future consideration)**

Write a one-time migration script that reads SharpTimer's `PlayerRecords` table and converts records to Source2Surf/Timer's schema. This would preserve leaderboard history across the transition. Challenges:
- Schema mapping is non-trivial (different column names, data types, nullable fields).
- Style/mode encoding may differ between the two timers.
- Source2Surf/Timer's SqlSugar model classes would need to be reverse-engineered from the source code.
- Risk of introducing corrupted data that causes runtime errors.

**Recommendation:** Start with Option A. If players strongly request historical records after the transition, a migration script can be developed later as a separate effort, with Source2Surf/Timer's actual schema as a known target.

**Database creation:** The existing MySQL deployment runs as `mysql:8`. The new database can be created via a one-time init:

```sql
CREATE DATABASE IF NOT EXISTS source2surf;
GRANT ALL PRIVILEGES ON source2surf.* TO '<MYSQL_USER>'@'%';
FLUSH PRIVILEGES;
```

This can be done as a pre-deployment step (Phase 3) via `kubectl exec` into the MySQL pod, or via a Kubernetes Job.

### Source2Surf/Timer Database Config

The database config template (`database.json`) for Source2Surf/Timer uses SqlSugar connection format:

```json
{
    "DbType": "MySql",
    "ConnectionString": "Server=${MYSQL_HOST};Port=3306;Database=${MYSQL_DB};User=${MYSQL_USER};Password=${MYSQL_PASS};SslMode=None;AllowPublicKeyRetrieval=true"
}
```

Note: The exact config format must be confirmed from the Source2Surf/Timer source code during Phase 1. SqlSugar supports multiple configuration styles and the timer may use a different JSON structure.

### PVC Strategy

**Game data PVC: reuse existing `cs2-modded-claim` (200 Gi)**

The existing 200 Gi PVC (`cs2-modded-claim`) is reused rather than creating a new one. Rationale:
- It already contains ~35 GB of CS2 base game files. Reusing it avoids a ~35 GB re-download on first start.
- The 200 Gi size is more than enough (surf needs ~43 GB total).
- Longhorn PVC deletion and recreation is unnecessarily risky when we can just reuse the volume.
- Workshop content for non-surf maps will be cleaned up naturally as CS2 only downloads maps in `subscribed_file_ids.txt`.

The PVC is already RWX. While RWO would suffice for a single pod, changing the access mode requires deleting and recreating the PVC (data loss). Keep as-is.

**No custom_files PVC.** The kus pattern of a separate 1 Gi PVC for config overrides is eliminated. All configs are baked into the image. The `cs2-custom-files` PVC can be deleted after kus is decommissioned.

**Replay data:** Source2Surf/Timer writes replay files within its plugin data directory on the game data PVC. These persist across restarts.

### Disk Space Budget

| Component | Size | Location |
|---|---|---|
| CS2 base game | ~35 GB | PVC |
| Surf workshop maps (~20) | ~5 GB | PVC (steamapps/workshop/) |
| Source2Surf/Timer replays | ~2 GB (grows slowly) | PVC (sharp/Timer/replays/) |
| Plugin overlay | ~30 MB | Image (applied once to PVC) |
| **Total** | **~43 GB** | Existing 200 Gi PVC |

## 6. API Contracts

Not applicable -- this is a game server, not an API service. The external interface is the CS2 game protocol on UDP/TCP 27015 and 27020 (SourceTV).

### DNS & Network

| Endpoint | Protocol | Purpose |
|---|---|---|
| `10.44.0.32:27015` | UDP/TCP | Game server (player connections) -- same as existing kus |
| `10.44.0.32:27020` | UDP/TCP | SourceTV (spectators, if enabled) -- same as existing kus |

No new DNS entries. The existing DNS record(s) pointing to `10.44.0.32` continue working. Players connect to the same address -- they just get the new surf server instead of kus.

## 7. Kubernetes Deployment

### Deployment Strategy: In-Place Replacement

The kus deployment (`cs2-modded-server`) is replaced, not run alongside. Sequence:

1. Scale down kus: `kubectl scale deployment cs2-modded-server -n game-server --replicas=0`
2. Deploy the new `cs2-surf-server` deployment with the **same service** (`cs2-modded-service`) selector label, OR create a new service with the same LB-IPAM IP annotation.
3. The cleanest approach: create a new Deployment (`cs2-surf-server`) with new labels, update the existing Service's selector to point to the new deployment, then scale down kus.

**Recommended approach:** New Deployment + update Service selector.

- Create `cs2-surf-server` Deployment with label `app: cs2-surf-server`.
- Update `cs2-modded-service` selector from `app: cs2-modded-server` to `app: cs2-surf-server`.
- Scale `cs2-modded-server` to 0.
- The Service IP (`10.44.0.32`) and all port mappings remain unchanged.

This avoids creating a new Service (which would need to claim the same LB-IPAM IP, potentially conflicting during transition).

### New Deployment: `cs2-surf-server`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cs2-surf-server
  namespace: game-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app: cs2-surf-server
  template:
    metadata:
      labels:
        app: cs2-surf-server
    spec:
      initContainers:
        - name: config-setup
          image: alpine:3.20
          command: ["sh", "-c"]
          args:
            - |
              apk add --no-cache gettext
              envsubst < /templates/database.json > /config-out/database.json
          env:
            - name: MYSQL_HOST
              value: "10.43.43.43"
            - name: MYSQL_DB
              value: "source2surf"
            - name: MYSQL_USER
              valueFrom:
                secretKeyRef:
                  name: mysql-secret
                  key: username
            - name: MYSQL_PASS
              valueFrom:
                secretKeyRef:
                  name: mysql-secret
                  key: password
          volumeMounts:
            - name: db-template
              mountPath: /templates
            - name: config-out
              mountPath: /config-out
      containers:
        - name: cs2-surf-server
          image: ghcr.io/hwcopeland/cs2-surf-server:latest
          imagePullPolicy: Always
          ports:
            - containerPort: 27015
              protocol: TCP
              name: tcp-game
            - containerPort: 27015
              protocol: UDP
              name: udp-game
            - containerPort: 27020
              protocol: TCP
              name: tcp-sourcetv
            - containerPort: 27020
              protocol: UDP
              name: udp-sourcetv
          env:
            - name: API_KEY
              valueFrom:
                secretKeyRef:
                  name: cs2-secret
                  key: API_KEY
            - name: STEAM_ACCOUNT
              valueFrom:
                secretKeyRef:
                  name: cs2-secret
                  key: STEAM_ACCOUNT
            - name: RCON_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: cs2-rcon
                  key: password
            - name: PORT
              value: "27015"
            - name: MAXPLAYERS
              value: "32"
            - name: TICKRATE
              value: "64"
            - name: MAP
              value: "surf_kitsune"
          resources:
            requests:
              cpu: 2000m
              memory: 4Gi
            limits:
              cpu: 4000m
              memory: 8Gi
          volumeMounts:
            - name: cs2-data
              mountPath: /home/steam/cs2
            - name: config-out
              mountPath: /opt/cs2-surf/runtime-config
          startupProbe:
            tcpSocket:
              port: 27015
            initialDelaySeconds: 60
            periodSeconds: 10
            failureThreshold: 18
          livenessProbe:
            tcpSocket:
              port: 27015
            periodSeconds: 30
            failureThreshold: 3
      volumes:
        - name: cs2-data
          persistentVolumeClaim:
            claimName: cs2-modded-claim
        - name: db-template
          configMap:
            name: cs2-surf-db-template
        - name: config-out
          emptyDir: {}
```

**Design notes:**

- **Init container is minimal** -- ONLY `envsubst` for database credentials. Everything else is baked into the image.
- **Reuses existing PVC** (`cs2-modded-claim`) to avoid re-downloading 35 GB of CS2 base game files.
- **Reuses existing secrets** (`cs2-secret`, `cs2-rcon`, `mysql-secret`) -- no new Bitwarden entries needed.
- **No `SYS_PTRACE` capability** -- the kus image required this for its debugging. ModSharp does not. Removed for security.
- **Reduced memory** -- 4 Gi request / 8 Gi limit (vs kus 8 Gi / 16 Gi). Fewer plugins, no mode switching overhead.

### Service Update

The existing `cs2-modded-service` is updated (not replaced) to select the new deployment:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: cs2-modded-service
  namespace: game-server
  annotations:
    io.cilium/lb-ipam-ips: "10.44.0.32"    # SAME IP as before
spec:
  selector:
    app: cs2-surf-server    # CHANGED from cs2-modded-server
  ports:
    - name: tcp-27015
      protocol: TCP
      port: 27015
      targetPort: 27015
    - name: udp-27015
      protocol: UDP
      port: 27015
      targetPort: 27015
    - name: tcp-27020
      protocol: TCP
      port: 27020
      targetPort: 27020
    - name: udp-27020
      protocol: UDP
      port: 27020
      targetPort: 27020
  type: LoadBalancer
```

**Note on Ingress:** CS2 uses UDP game protocol, not HTTP. No Ingress resource -- the game-server namespace uses LoadBalancer services with Cilium LB-IPAM directly.

### ConfigMap: `cs2-surf-db-template`

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cs2-surf-db-template
  namespace: game-server
data:
  database.json: |
    {
        "DbType": "MySql",
        "ConnectionString": "Server=${MYSQL_HOST};Port=3306;Database=${MYSQL_DB};User=${MYSQL_USER};Password=${MYSQL_PASS};SslMode=None;AllowPublicKeyRetrieval=true"
    }
```

### Resources to Delete (After Validation)

Once the surf server is validated and stable:
- `cs2-custom-files` PVC (1 Gi) -- no longer needed, configs baked into image.
- `cs2-addon-config` ConfigMap -- contained MAM, GMM, SharpTimer configs, all replaced.
- The kus deployment (`cs2-modded-server`) can remain scaled to 0 as a rollback target, then deleted after a bake period (1-2 weeks).

### Resource Sizing Rationale

| Resource | kus Current | Surf Server | Rationale |
|---|---|---|---|
| CPU request | 2000m | 2000m | CS2 server is CPU-bound for physics |
| CPU limit | 6000m | 4000m | Surf has simpler physics than competitive |
| Memory request | 8 Gi | 4 Gi | Fewer plugins loaded, no mode switching |
| Memory limit | 16 Gi | 8 Gi | 32 players with replay bot, conservative |
| PVC | 200 Gi (2 PVCs) | 200 Gi (reuse, 1 PVC) | Reuse existing PVC to avoid re-download |

## 8. CI/CD Pipeline

### Repository Structure

The Dockerfile and all surf server configs live in a new directory within the existing IaC repo:

```
rke2/game-server/cs2/
  cs2-server/          # existing kus deployment manifests (kept for rollback)
  cs2-database/        # existing MySQL manifests (unchanged)
  custom_files/        # existing kus custom files (can be removed after bake period)
  cs2-surf/            # NEW
    Dockerfile
    entrypoint.sh
    scripts/
      update-modsharp.sh        # adapted from CS2-Egg updater pattern
      update-plugins.sh         # adapted from CS2-Egg updater pattern
      apply-overlay.sh          # version-stamp overlay logic
    configs/
      server.cfg
      gamemodes_server.txt
      subscribed_file_ids.txt
      sharp/
        Timer/
          config.json
          database.json.template
        MapChooserSharpMS/
          config.json
        ms-advertisement/
          config.json
        MS-NoBlock/
          config.json
        TnmsAdmin/
          config.json
    k8s/
      deployment.yaml
      service-patch.yaml         # selector update for existing service
      db-template-configmap.yaml
```

### GitHub Actions Workflow

```yaml
name: Build CS2 Surf Server
on:
  push:
    branches: [main]
    paths:
      - 'rke2/game-server/cs2/cs2-surf/**'
  workflow_dispatch:

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: hwcopeland/cs2-surf-server

jobs:
  build-and-push:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4

      - uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - uses: docker/metadata-action@v5
        id: meta
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=sha
            type=raw,value=latest

      - uses: docker/build-push-action@v6
        with:
          context: rke2/game-server/cs2/cs2-surf
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max

  deploy:
    needs: build-and-push
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Deploy to Kubernetes
        uses: steebchen/kubectl@v2.0.0
        with:
          config: ${{ secrets.KUBE_CONFIG_DATA }}
          command: rollout restart deployment/cs2-surf-server -n game-server

      - name: Wait for rollout
        uses: steebchen/kubectl@v2.0.0
        with:
          config: ${{ secrets.KUBE_CONFIG_DATA }}
          command: rollout status deployment/cs2-surf-server -n game-server --timeout=300s
```

**Trigger:** Push to `main` that modifies anything under `rke2/game-server/cs2/cs2-surf/`. Also supports manual dispatch.

**Deploy strategy:** `kubectl rollout restart` causes the deployment to pull the new `:latest` tag. The existing pod is terminated only after the new pod passes its startup probe (default RollingUpdate strategy).

**Alternative deploy strategy (Flux):** If Flux OCI is operational, the deploy step can be replaced with a Flux `ImagePolicy` + `ImageUpdateAutomation`. This is a post-launch enhancement.

### Workshop Map List Management

The workshop map list lives in two files:

1. `configs/gamemodes_server.txt` -- CS2 mapgroup with `workshop/{ID}/{mapname}` entries.
2. `configs/subscribed_file_ids.txt` -- plain list of workshop IDs for CS2 engine download.

To add/remove surf maps:
1. Edit both files in the repo.
2. Push to main.
3. CI builds a new image with the updated lists.
4. Deploy rolls out the new image.
5. On next pod start, the overlay is re-applied (new VERSION stamp), and CS2 downloads any new workshop content.

## 9. Migration & Rollout

### Cutover Plan

This is an in-place replacement, not a parallel deployment. The cutover is:

1. **Pre-cutover:** Build and test the image locally (Phase 1-2). Create `source2surf` database (Phase 3).
2. **Cutover window:** Choose a low-traffic time.
   a. Scale kus to 0: `kubectl scale deployment cs2-modded-server --replicas=0 -n game-server`
   b. Apply new deployment: `kubectl apply -f cs2-surf/k8s/deployment.yaml`
   c. Update service selector: `kubectl apply -f cs2-surf/k8s/service-patch.yaml`
   d. Wait for new pod to pass startup probe.
3. **Validation:** Run acceptance tests (Section 11). If any blocker, roll back.
4. **Bake period:** 1-2 weeks with kus at replicas=0. If stable, clean up kus resources.

### Rollback Plan

Rollback reverses the cutover:

1. Scale surf to 0: `kubectl scale deployment cs2-surf-server --replicas=0 -n game-server`
2. Revert service selector to `app: cs2-modded-server`.
3. Scale kus back to 1: `kubectl scale deployment cs2-modded-server --replicas=1 -n game-server`

**Data safety:** The `sharptimer` database is untouched. SharpTimer records are preserved. The `source2surf` database is additive. Rolling back loses any Source2Surf/Timer records set during the surf server's runtime, but SharpTimer picks up right where it left off.

**PVC safety:** The game data PVC (`cs2-modded-claim`) is shared. The surf server's overlay writes ModSharp files into the game directory. On rollback, kus's `install_docker.sh` will overwrite the entire `csgo/` directory anyway (that is its normal behavior), so ModSharp files are cleaned up automatically.

### No DNS or Firewall Changes

The same Service IP (`10.44.0.32`) is used. No DNS records change. No firewall rules change. Players connecting to the same address get the new server.

## 10. Risks & Open Questions

### Known Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Workshop VPK mounting without MAM fails -- `host_workshop_map` does not work reliably for all map transitions | High | Phase 1 validation gate. Test 5+ consecutive map changes via native CS2 workshop support. Fallback: Option 3 (Metamod + MAM alongside ModSharp). |
| ModSharp has an undocumented incompatibility with our CS2/OS version | Medium | Phase 1 local testing. ModSharp has 109 releases and active development. Join their Discord for support if needed. |
| MapChooserSharpMS does not support `host_workshop_map` as a map change method | Medium | Read MapChooserSharpMS source during Phase 1. If it only supports `changelevel`, contribute a PR or write a thin wrapper plugin. |
| Source2Surf/Timer SqlSugar schema auto-migration has issues on MySQL 8 | Low | Source2Surf/Timer explicitly lists MySQL as supported. Test during Phase 1 with a local MySQL 8 container. |
| CS2 update breaks ModSharp loader (native loader vs Metamod is a different update risk profile) | Medium | Same risk category as any CS2 plugin. Pin ModSharp version, test after CS2 updates before rebuilding. |
| Two databases (sharptimer + source2surf) -- MySQL resource usage increases | Low | Both databases are small (MB-range, not GB). MySQL 8 handles this trivially. |
| ModSharp AGPL-3.0 license | Low | We are deploying a game server, not distributing modified source. AGPL applies to source distribution, not server-side execution. Our Dockerfile and configs are our own work. |

### Open Questions

1. **Source2Surf/Timer config format**: What is the exact JSON structure for database configuration? SqlSugar supports multiple formats. Must be confirmed from the Timer source code during Phase 1.
   **Action:** Read `Source2Surf/Timer` source for database config loading.

2. **MapChooserSharpMS workshop map support**: Does it use `changelevel` or `host_workshop_map` for workshop maps? Does it read `gamemodes_server.txt` or its own map list format?
   **Action:** Read MapChooserSharpMS source during Phase 1.

3. **ModSharp plugin directory structure**: Is it `sharp/` or `game/csgo/sharp/` or something else? Need to confirm from ModSharp installation docs.
   **Action:** Download a ModSharp release and inspect the directory structure during Phase 1.

4. **Source2Surf/Timer replay file location**: Where are replay files stored? Needed to ensure they are on the persistent PVC.
   **Action:** Check Timer source/docs during Phase 1.

5. **AdminFlatFile vs TnmsAdmin**: ModSharp has a built-in AdminFlatFile module. Is TnmsAdministrationPlatform needed on top of it, or does AdminFlatFile provide sufficient admin commands?
   **Action:** Evaluate during Phase 1. If AdminFlatFile covers kick/ban/mute, skip TnmsAdmin.

6. **Workshop collection ID**: Do we have or need a Steam Workshop Collection for our surf maps, or is `subscribed_file_ids.txt` sufficient without `+host_workshop_collection`?
   **Action:** Test both approaches. A collection simplifies the launch command but requires creating/maintaining a Steam Workshop Collection.

7. **Flux integration**: Should the deploy step use Flux ImageUpdateAutomation instead of kubectl?
   **Checkpoint:** Evaluate after initial deployment stabilizes.

### Flagged Assumptions

- **Assumption:** ModSharp's native loader works without Metamod on the current CS2 Linux dedicated server build. **Verify:** Phase 1 local Docker test.
- **Assumption:** `host_workshop_map` is sufficient for workshop map loading without MAM. **Verify:** Phase 1 -- 5 consecutive map changes test.
- **Assumption:** Source2Surf/Timer creates its database schema automatically via SqlSugar on first connect. **Verify:** Phase 1 local test with empty MySQL database.
- **Assumption:** MapChooserSharpMS can be configured to use `host_workshop_map` for map changes. **Verify:** Phase 1 source code review.
- **Assumption:** The existing `cs2-modded-claim` PVC can be reused by the new deployment without data conflicts. **Verify:** The kus overlay writes to `csgo/` -- ModSharp writes to `csgo/sharp/`. No path conflicts, but verify during Phase 1.
- **Assumption:** The `steam` user in `cm2network/steamcmd` has UID 1000 and can write to `/home/steam/`. **Verify:** Check during Phase 1 Docker build.

## 11. Testing Strategy

### Pre-Deployment (Phase 1)

- **Local Docker build:** `docker build -t cs2-surf-server:test .` succeeds.
- **Local container run:** Container starts, steamcmd downloads CS2, ModSharp loads, server binds port 27015.
- **ModSharp verification:** Connect via RCON. Verify ModSharp framework reports loaded. Verify Source2Surf/Timer, MapChooserSharpMS, and other plugins are listed.
- **Database auto-creation:** Connect to a fresh MySQL 8 database. Verify Source2Surf/Timer creates its tables via SqlSugar on first load.
- **Workshop map test (critical):** Load a workshop surf map via `host_workshop_map`. Change map 5+ times. Verify every map loads successfully without MAM.

### Post-Deployment (Phase 4)

**Acceptance test matrix:**

| Test | Method | Pass Criteria |
|---|---|---|
| Server starts | kubectl logs, TCP probe | Port 27015 open within 3 minutes |
| ModSharp loads | RCON framework check | ModSharp reports version, all plugins loaded |
| Workshop download | RCON `status` after map load | Map loads, no "missing map" errors |
| Map change via host_workshop_map | Console command | Map changes successfully |
| RTV vote | In-game `!rtv` with 1 player | Vote starts, completes, map changes |
| Sequential map changes | 5 consecutive RTV cycles | All 5 maps load, no failures |
| Replay bot initial | Load map with SR | Bot spawns, replays the run |
| Replay bot after mapchange | Change map 3 times | Bot spawns on each map that has an SR |
| Source2Surf/Timer MySQL | Complete a surf run | Time appears in leaderboard and persists after pod restart |
| Surf movement | Join server, surf | sv_airaccelerate 150, autobhop, correct friction |
| Existing IP connectivity | CS2 client `connect 10.44.0.32` | Joins server, sees surf map |
| Startup time | Time from pod creation to port 27015 open | Under 3 minutes (warm PVC) |
| NoBlock | Walk through another player | No collision |
| Rampfix | Surf ramp transitions | No ramp bugs |

### Monitoring

No dedicated monitoring stack is prescribed. Operational signals:

- `kubectl logs deployment/cs2-surf-server -n game-server` for server console output.
- `kubectl get pods -n game-server -l app=cs2-surf-server` for pod health.
- In-game RCON for runtime diagnostics.

## 12. Implementation Phases

| Phase | Description | Dependencies | Complexity | Parallelizable With |
|---|---|---|---|---|
| 1 | Docker image (Dockerfile, entrypoint, ModSharp + plugins, configs) | None | M | -- |
| 1a | Workshop map validation (critical gate) | Phase 1 (image must boot) | S | -- |
| 2 | CI/CD pipeline (GitHub Actions workflow, GHCR push) | Phase 1 (image must build) | S | Phase 3 (K8s manifests) |
| 3 | Kubernetes deployment (manifests, DB creation, service selector update) | Phase 2 (image in GHCR) | S | Phase 2 (manifests can be written in parallel) |
| 4 | Validation + cutover (acceptance testing, scale kus to 0) | Phase 3 (server running) | S | -- |
| 5 | Cleanup (delete kus deployment, custom_files PVC, old ConfigMap) | Phase 4 + 1-2 week bake | S | -- |

**Total estimated effort:** M (Medium). Critical path is Phase 1 (Dockerfile + ModSharp integration) -> Phase 1a (workshop validation gate) -> Phase 2 (CI) -> Phase 3 (deploy) -> Phase 4 (test + cutover).

**Phase 1a is the critical gate.** If `host_workshop_map` does not work reliably for 5+ consecutive map changes, the project must evaluate the Metamod+MAM fallback (Option 3 from Section 4.2) before proceeding. This is a go/no-go decision point.

**Pre-requisites (operator action required before Phase 3):**
1. Verify `KUBE_CONFIG_DATA` secret exists in GitHub repo for CI/CD deploy step.
2. Create `source2surf` database on MySQL server (or confirm it can be auto-created).
3. Verify existing GSLT in `cs2-secret` is not locked to the kus server instance (GSLTs are usually server-agnostic, but verify).
