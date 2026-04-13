# =============================================================================
# cs2-surf-server Docker Image
#
# ModSharp-based CS2 surf server image.
# - Installs CS2 dedicated server via steamcmd (deferred to entrypoint for PVC)
# - Bundles ModSharp framework and surf plugins at pinned versions
# - Plugins without releases are built from source in a multi-stage build
#
# Build: docker build -t cs2-surf-server:latest .
# =============================================================================

# ---------------------------------------------------------------------------
# ARGs — pin every dependency version
# ---------------------------------------------------------------------------
ARG MODSHARP_VERSION="git-118"
ARG MODSHARP_BUILD="118"

# Plugins with GitHub releases (pinned versions)
ARG RAMPFIX_VERSION="v1.0.6"
ARG MSNOBLOCK_VERSION="MS-NoBlock-v.1.0.2"

# Plugins built from source (pinned refs)
ARG SOURCE2SURF_TIMER_REF="master"

# ---------------------------------------------------------------------------
# Stage 1: Build plugins that have no release artifacts
# Source2Surf/Timer targets net10.0
# ---------------------------------------------------------------------------
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS plugin-builder

ARG SOURCE2SURF_TIMER_REF

RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# --- Source2Surf/Timer (hwcopeland fork with surf patches) ---
# Fork carries: bhop enabled in start zone (upstream hard-disables it),
# prejump velocity limiter removed entirely, WR → SR rename in user-facing
# strings, graceful nav-patch failure.
RUN git clone --depth 1 --branch "${SOURCE2SURF_TIMER_REF}" \
        https://github.com/hwcopeland/Timer.git /build/timer
WORKDIR /build/timer
RUN sed -i '/<Target Name="PostBuild"/,/<\/Target>/d' Timer/Timer.csproj && \
    dotnet publish Timer/Timer.csproj -c Release -r linux-x64 --no-self-contained -o /build/out/timer && \
    dotnet publish Timer.RequestManager/Timer.RequestManager.csproj -c Release -r linux-x64 --no-self-contained -o /build/out/timer-rm && \
    dotnet publish Timer.Shared/Timer.Shared.csproj -c Release -r linux-x64 --no-self-contained -o /build/out/timer-shared

# --- SurfMapCommand (in-repo, minimum-viable !map chat plugin) ---
# Native ModSharp module implementing IClientListener.OnClientSayCommand.
# ~100 lines total; built from plugins/SurfMapCommand/ in this repo.
COPY plugins/SurfMapCommand /build/surf-map-command
WORKDIR /build/surf-map-command
RUN dotnet publish SurfMapCommand.csproj \
        -c Release -r linux-x64 --no-self-contained \
        -p:DebugType=None -p:DebugSymbols=false \
        -o /build/out/surf-map-command

# MapChooserSharpMS removed entirely — see git log for rationale. TL;DR:
# upstream CS#→ModSharp port was ~5% done, pulled in 4 broken Tnms libs,
# and provided zero actual functionality. SurfMapCommand above replaces
# the only feature we actually needed (!map in chat).

# ---------------------------------------------------------------------------
# Stage 2: Download and assemble the overlay
# ---------------------------------------------------------------------------
FROM debian:bookworm-slim AS assembler

ARG MODSHARP_VERSION
ARG MODSHARP_BUILD
ARG RAMPFIX_VERSION
ARG MSNOBLOCK_VERSION
RUN apt-get update && apt-get install -y --no-install-recommends \
        curl unzip ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /staging

# The overlay mirrors the csgo/ directory structure.
# ModSharp installs to csgo/sharp/ — NOT addons/counterstrikesharp/.
# No Metamod, no gameinfo.gi patching.

# --- ModSharp core ---
RUN curl -fsSL "https://github.com/Kxnrl/modsharp-public/releases/download/${MODSHARP_VERSION}/ModSharp-git${MODSHARP_BUILD}-linux.zip" \
        -o /tmp/modsharp.zip \
    && unzip -q /tmp/modsharp.zip -d /staging/ \
    && rm /tmp/modsharp.zip
# Result: /staging/sharp/{bin,core,configs,gamedata,locales,modules,shared}/

# --- .NET 10.0 runtime (required by ModSharp — Steam RT3 can't use system .NET) ---
ARG DOTNET_VERSION="10.0.5"
RUN curl -fsSL "https://dotnetcli.azureedge.net/dotnet/Runtime/${DOTNET_VERSION}/dotnet-runtime-${DOTNET_VERSION}-linux-x64.tar.gz" \
        -o /tmp/dotnet-runtime.tar.gz \
    && mkdir -p /staging/sharp/runtime \
    && tar xzf /tmp/dotnet-runtime.tar.gz -C /staging/sharp/runtime/ \
    && rm /tmp/dotnet-runtime.tar.gz

# --- ModSharp extensions ---
RUN curl -fsSL "https://github.com/Kxnrl/modsharp-public/releases/download/${MODSHARP_VERSION}/ModSharp-git${MODSHARP_BUILD}-linux-extensions.zip" \
        -o /tmp/modsharp-ext.zip \
    && mkdir -p /staging/sharp/extensions \
    && unzip -q /tmp/modsharp-ext.zip -d /staging/sharp/extensions/ \
    && rm /tmp/modsharp-ext.zip

# --- Rampfix (zip has modules/ and gamedata/ — extract into sharp/) ---
RUN curl -fsSL "https://github.com/Nukoooo/Rampfix/releases/download/${RAMPFIX_VERSION}/RampFix-${RAMPFIX_VERSION}.zip" \
        -o /tmp/rampfix.zip \
    && unzip -qo /tmp/rampfix.zip -d /staging/sharp/ \
    && rm /tmp/rampfix.zip

# --- MS-NoBlock (zip has modules/ — extract into sharp/) ---
RUN curl -fsSL "https://github.com/darkerz7/MS-NoBlock/releases/download/${MSNOBLOCK_VERSION}/${MSNOBLOCK_VERSION}.zip" \
        -o /tmp/msnoblock.zip \
    && unzip -qo /tmp/msnoblock.zip -d /staging/sharp/ \
    && rm /tmp/msnoblock.zip

# --- Source2Surf/Timer (built from source) ---
COPY --from=plugin-builder /build/out/timer/ /staging/sharp/modules/Timer/
COPY --from=plugin-builder /build/out/timer-rm/ /staging/sharp/modules/Timer.RequestManager/
COPY --from=plugin-builder /build/out/timer-shared/ /staging/sharp/shared/Timer.Shared/

# --- SurfMapCommand (in-repo native !map plugin) ---
COPY --from=plugin-builder /build/out/surf-map-command/ /staging/sharp/modules/SurfMapCommand/

# Generate runtimeconfig.json if missing (UseCommonOutputDirectory suppresses
# it for source-built modules; without it ModSharp silently skips the module).
RUN for mod in Timer Timer.RequestManager Timer.Shared SurfMapCommand; do \
      for base in /staging/sharp/modules /staging/sharp/shared; do \
        dir="${base}/${mod}"; \
        if [ -d "${dir}" ] && [ ! -f "${dir}/${mod}.runtimeconfig.json" ]; then \
          echo '{"runtimeOptions":{"tfm":"net10.0","framework":{"name":"Microsoft.NETCore.App","version":"10.0.5"},"rollForward":"Major"}}' > "${dir}/${mod}.runtimeconfig.json"; \
        fi; \
      done; \
    done
COPY --from=plugin-builder /build/timer/gamedata/ /staging/sharp/gamedata/
COPY --from=plugin-builder /build/timer/sharp/ /staging/sharp/

# ---------------------------------------------------------------------------
# Stage 3: Final image
# ---------------------------------------------------------------------------
FROM registry.gitlab.steamos.cloud/steamrt/sniper/sdk:latest

# Re-declare ARGs needed in this stage
ARG MODSHARP_VERSION
ARG MODSHARP_BUILD

LABEL maintainer="hwcopeland"
LABEL description="CS2 ModSharp surf server"
LABEL modsharp.version="${MODSHARP_VERSION}"

# Steam RT3 image — required for ModSharp native loader compatibility.
# Set up steamcmd and steam user.
USER root

RUN dpkg --add-architecture i386 && \
    apt-get update && apt-get install -y --no-install-recommends \
        gettext-base jq curl ca-certificates lib32gcc-s1 \
    && rm -rf /var/lib/apt/lists/*

# Create steam user
RUN useradd -m -s /bin/bash steam

# Install steamcmd
RUN mkdir -p /home/steam/steamcmd && \
    curl -fsSL https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz | \
    tar xzf - -C /home/steam/steamcmd && \
    chown -R steam:steam /home/steam

ENV CS2_DIR="/home/steam/cs2"
ENV STEAMCMD_DIR="/home/steam/steamcmd"

# Copy the assembled overlay
COPY --from=assembler --chown=steam:steam /staging/ /opt/cs2-surf/overlay/

# Copy config templates (mounted as ConfigMap in K8s, but baked-in defaults here)
COPY --chown=steam:steam configs/ /opt/cs2-surf/configs/

# Copy scripts
COPY --chown=steam:steam entrypoint.sh /opt/cs2-surf/entrypoint.sh
COPY --chown=steam:steam scripts/ /opt/cs2-surf/scripts/

RUN chmod +x /opt/cs2-surf/entrypoint.sh /opt/cs2-surf/scripts/*.sh

# Write image version stamp — used by apply-overlay.sh to skip redundant copies.
# Include a build timestamp so every image rebuild busts the PVC stamp and
# re-applies the overlay. Without this, plugin additions/removals after the
# initial git-NNN tag would be silently skipped on the next pod start.
RUN echo "${MODSHARP_VERSION}-$(date -u +%Y%m%d%H%M%S)" > /opt/cs2-surf/VERSION

# Ensure the steam user owns all our files
RUN chown -R steam:steam /opt/cs2-surf

# Switch back to steam user for runtime
USER steam

# Default environment variables — all overridable via K8s env/ConfigMap
ENV PORT="27015" \
    MAP="de_dust2" \
    MAXPLAYERS="32" \
    GAME_TYPE="0" \
    GAME_MODE="0" \
    STEAM_ACCOUNT="" \
    API_KEY="" \
    RCON_PASSWORD="" \
    SERVER_NAME="CS2 Surf Server" \
    TICKRATE="128"

EXPOSE 27015/tcp 27015/udp 27020/tcp 27020/udp

ENTRYPOINT ["/opt/cs2-surf/entrypoint.sh"]
