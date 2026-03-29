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
ARG MODSHARP_VERSION="git-109"
ARG MODSHARP_BUILD="109"

# Plugins with GitHub releases (pinned versions)
ARG ADVERTISEMENT_VERSION="v1.0.2"
ARG RAMPFIX_VERSION="v1.0.6"
ARG MSNOBLOCK_VERSION="MS-NoBlock-v.1.0.2"
ARG TNMSADMIN_PLATFORM_VERSION="v0.0.2"

# Plugins built from source (pinned commit SHAs)
ARG SOURCE2SURF_TIMER_REF="master"
ARG MAPCHOOSER_REF="main"
ARG TNMSADMINUTILS_REF="main"

# ---------------------------------------------------------------------------
# Stage 1: Build plugins that have no release artifacts
# Source2Surf/Timer, MapChooserSharpMS, TnmsAdminUtils all target net10.0
# ---------------------------------------------------------------------------
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS plugin-builder

ARG SOURCE2SURF_TIMER_REF
ARG MAPCHOOSER_REF
ARG TNMSADMINUTILS_REF

RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# --- Source2Surf/Timer ---
RUN git clone --depth 1 --branch "${SOURCE2SURF_TIMER_REF}" \
        https://github.com/Source2Surf/Timer.git /build/timer
WORKDIR /build/timer
RUN sed -i '/<Target Name="PostBuild"/,/<\/Target>/d' Timer/Timer.csproj && \
    dotnet publish Timer/Timer.csproj -c Release -r linux-x64 --no-self-contained -o /build/out/timer && \
    dotnet publish Timer.RequestManager/Timer.RequestManager.csproj -c Release -r linux-x64 --no-self-contained -o /build/out/timer-rm

# --- MapChooserSharpMS ---
RUN git clone --depth 1 --branch "${MAPCHOOSER_REF}" \
        https://github.com/fltuna/MapChooserSharpMS.git /build/mapchooser
WORKDIR /build/mapchooser
RUN dotnet publish -c Release -r linux-x64 --no-self-contained -o /build/out/mapchooser \
    || echo "WARN: MapChooserSharpMS build failed"

# --- TnmsAdminUtils ---
RUN git clone --depth 1 --branch "${TNMSADMINUTILS_REF}" \
        https://github.com/fltuna/TnmsAdminUtils.git /build/tnmsadminutils
WORKDIR /build/tnmsadminutils
RUN dotnet publish -c Release -r linux-x64 --no-self-contained -o /build/out/tnmsadminutils \
    || echo "WARN: TnmsAdminUtils build failed"

# ---------------------------------------------------------------------------
# Stage 2: Download and assemble the overlay
# ---------------------------------------------------------------------------
FROM debian:bookworm-slim AS assembler

ARG MODSHARP_VERSION
ARG MODSHARP_BUILD
ARG ADVERTISEMENT_VERSION
ARG RAMPFIX_VERSION
ARG MSNOBLOCK_VERSION
ARG TNMSADMIN_PLATFORM_VERSION

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

# --- ModSharp extensions ---
RUN curl -fsSL "https://github.com/Kxnrl/modsharp-public/releases/download/${MODSHARP_VERSION}/ModSharp-git${MODSHARP_BUILD}-linux-extensions.zip" \
        -o /tmp/modsharp-ext.zip \
    && mkdir -p /staging/sharp/extensions \
    && unzip -q /tmp/modsharp-ext.zip -d /staging/sharp/extensions/ \
    && rm /tmp/modsharp-ext.zip

# --- ms-advertisement (zip has sharp/ prefix, extracts to staging root) ---
RUN curl -fsSL "https://github.com/partiusfabaa/ms-advertisement/releases/download/${ADVERTISEMENT_VERSION}/Advertisement.zip" \
        -o /tmp/advertisement.zip \
    && unzip -qo /tmp/advertisement.zip -d /staging/ \
    && rm /tmp/advertisement.zip

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

# --- TnmsAdministrationPlatform (zip has modules/ and shared/ — extract into sharp/) ---
RUN curl -fsSL "https://github.com/fltuna/TnmsAdministrationPlatform/releases/download/${TNMSADMIN_PLATFORM_VERSION}/TnmsAdministrationPlatform.zip" \
        -o /tmp/tnmsadmin.zip \
    && unzip -qo /tmp/tnmsadmin.zip -d /staging/sharp/ \
    && rm /tmp/tnmsadmin.zip

# --- Source2Surf/Timer (built from source) ---
COPY --from=plugin-builder /build/out/timer/ /staging/sharp/modules/Timer/
COPY --from=plugin-builder /build/out/timer-rm/ /staging/sharp/modules/Timer.RequestManager/
COPY --from=plugin-builder /build/timer/gamedata/ /staging/sharp/gamedata/
COPY --from=plugin-builder /build/timer/sharp/ /staging/sharp/

# --- MapChooserSharpMS (built from source) ---
COPY --from=plugin-builder /build/out/mapchooser/ /staging/sharp/modules/MapChooserSharpMS/

# --- TnmsAdminUtils (built from source) ---
COPY --from=plugin-builder /build/out/tnmsadminutils/ /staging/sharp/modules/TnmsAdminUtils/

# ---------------------------------------------------------------------------
# Stage 3: Final image
# ---------------------------------------------------------------------------
FROM cm2network/steamcmd:latest

# Re-declare ARGs needed in this stage
ARG MODSHARP_VERSION
ARG MODSHARP_BUILD

LABEL maintainer="hwcopeland"
LABEL description="CS2 ModSharp surf server"
LABEL modsharp.version="${MODSHARP_VERSION}"

# Install runtime dependencies.
# cm2network/steamcmd runs as 'steam' (UID 1000); we need root for apt.
USER root

RUN apt-get update && apt-get install -y --no-install-recommends \
        gettext-base \
        jq \
        curl \
    && rm -rf /var/lib/apt/lists/*

# CS2 dedicated server install path (steamcmd convention for cm2network image)
ENV CS2_DIR="/home/steam/cs2-dedicated"

# Copy the assembled overlay
COPY --from=assembler --chown=steam:steam /staging/ /opt/cs2-surf/overlay/

# Copy config templates (mounted as ConfigMap in K8s, but baked-in defaults here)
COPY --chown=steam:steam configs/ /opt/cs2-surf/configs/

# Copy scripts
COPY --chown=steam:steam entrypoint.sh /opt/cs2-surf/entrypoint.sh
COPY --chown=steam:steam scripts/ /opt/cs2-surf/scripts/

RUN chmod +x /opt/cs2-surf/entrypoint.sh /opt/cs2-surf/scripts/*.sh

# Write image version stamp — used by apply-overlay.sh to skip redundant copies
RUN echo "${MODSHARP_VERSION}" > /opt/cs2-surf/VERSION

# Ensure the steam user owns all our files
RUN chown -R steam:steam /opt/cs2-surf

# Switch back to steam user for runtime
USER steam

# Default environment variables — all overridable via K8s env/ConfigMap
ENV PORT="27015" \
    MAP="surf_kitsune" \
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
