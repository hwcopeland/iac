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
ARG ADVERTISEMENT_VERSION="v1.0.2"
ARG RAMPFIX_VERSION="v1.0.6"
ARG MSNOBLOCK_VERSION="MS-NoBlock-v.1.0.2"
ARG ADDONMANAGER_VERSION="v1.0.1"
# Tnms shared libs — required at runtime by MapChooserSharpMS (transitively
# through TnmsPluginFoundation NuGet package).
ARG TNMS_LOCALIZATION_VERSION="v0.0.2"
ARG TNMS_ADMINISTRATION_VERSION="v0.0.2"
ARG TNMS_TARGETING_VERSION="v0.1.1"
ARG TNMS_FOUNDATION_VERSION="v0.0.6"

# Plugins built from source (pinned refs)
ARG SOURCE2SURF_TIMER_REF="master"
ARG MAPCHOOSER_REF="main"

# ---------------------------------------------------------------------------
# Stage 1: Build plugins that have no release artifacts
# Source2Surf/Timer targets net10.0
# ---------------------------------------------------------------------------
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS plugin-builder

ARG SOURCE2SURF_TIMER_REF
ARG MAPCHOOSER_REF

RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# --- Source2Surf/Timer ---
RUN git clone --depth 1 --branch "${SOURCE2SURF_TIMER_REF}" \
        https://github.com/Source2Surf/Timer.git /build/timer
WORKDIR /build/timer
RUN sed -i '/<Target Name="PostBuild"/,/<\/Target>/d' Timer/Timer.csproj && \
    dotnet publish Timer/Timer.csproj -c Release -r linux-x64 --no-self-contained -o /build/out/timer && \
    dotnet publish Timer.RequestManager/Timer.RequestManager.csproj -c Release -r linux-x64 --no-self-contained -o /build/out/timer-rm && \
    dotnet publish Timer.Shared/Timer.Shared.csproj -c Release -r linux-x64 --no-self-contained -o /build/out/timer-shared

# --- MapChooserSharpMS (RTV, map vote, nominations, map cycle) ---
# Provides !map, !nominate, !rtv chat commands on top of ModSharp.
#
# Upstream MCS csproj has two bugs that make it fail to load at runtime:
#   1. Pins ModSharp.Sharp.Shared 2.1.100 while TnmsPluginFoundation 0.0.6
#      (and ModSharp core git-118) target 2.1.117 — ABI mismatch means MCS's
#      class hierarchy doesn't resolve against the runtime, and ModSharp's
#      module scanner silently filters it out.
#   2. TnmsPluginFoundation is referenced as a plain PackageReference without
#      ExcludeAssets="runtime", so dotnet publish bundles TnmsPluginFoundation.dll
#      into the module output dir — duplicating the one in sharp/shared/ and
#      creating a type-identity mismatch in the AssemblyLoadContext.
#
# We patch both at build time with sed, then post-publish delete the Shared
# DLLs that should live in sharp/shared/ (the project-reference Shared is
# always bundled by dotnet publish regardless of Private/ExcludeAssets).
RUN git clone --depth 1 --branch "${MAPCHOOSER_REF}" \
        https://github.com/fltuna/MapChooserSharpMS.git /build/mcs
WORKDIR /build/mcs
RUN sed -i \
        -e 's|ModSharp.Sharp.Shared" Version="2.1.100"|ModSharp.Sharp.Shared" Version="2.1.117"|' \
        -e 's|"TnmsPluginFoundation" Version="0.0.6" />|"TnmsPluginFoundation" Version="0.0.6" ExcludeAssets="runtime" />|' \
        MapChooserSharpMS/MapChooserSharpMS.csproj && \
    echo "--- patched MCS csproj ---" && \
    cat MapChooserSharpMS/MapChooserSharpMS.csproj
RUN dotnet publish MapChooserSharpMS.Shared/MapChooserSharpMS.Shared.csproj \
        -c Release -r linux-x64 --no-self-contained \
        -p:DebugType=None -p:DebugSymbols=false \
        -o /build/out/mcs-shared && \
    dotnet publish MapChooserSharpMS/MapChooserSharpMS.csproj \
        -c Release -r linux-x64 --no-self-contained \
        -p:DebugType=None -p:DebugSymbols=false \
        -o /build/out/mcs && \
    # Strip shared DLLs from the module dir — they live in sharp/shared/ and
    # bundling locally creates the duplicate-ALC / type-identity bug.
    rm -f /build/out/mcs/MapChooserSharpMS.Shared.dll \
          /build/out/mcs/TnmsPluginFoundation.dll

# ---------------------------------------------------------------------------
# Stage 2: Download and assemble the overlay
# ---------------------------------------------------------------------------
FROM debian:bookworm-slim AS assembler

ARG MODSHARP_VERSION
ARG MODSHARP_BUILD
ARG ADVERTISEMENT_VERSION
ARG RAMPFIX_VERSION
ARG MSNOBLOCK_VERSION
ARG ADDONMANAGER_VERSION
ARG TNMS_LOCALIZATION_VERSION
ARG TNMS_ADMINISTRATION_VERSION
ARG TNMS_TARGETING_VERSION
ARG TNMS_FOUNDATION_VERSION
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

# --- ms-advertisement DISABLED — spams chat/screen ads ---
# RUN curl -fsSL "https://github.com/partiusfabaa/ms-advertisement/releases/download/${ADVERTISEMENT_VERSION}/Advertisement.zip" \
#         -o /tmp/advertisement.zip \
#     && unzip -qo /tmp/advertisement.zip -d /staging/ \
#     && rm /tmp/advertisement.zip

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

# --- AddonManager (handles -dual_addon workshop extraction to sharp/assets) ---
# Replaces the manual entrypoint RCON host_workshop_map switch. With AddonManager
# installed, CS2 launched with -dual_addon <ids> auto-downloads and extracts the
# addon content into sharp/assets at startup.
# NOTE: AddonManager.zip internally has its own `sharp/` prefix, so extract
# into /staging/ (not /staging/sharp/) to avoid sharp/sharp/modules/.
RUN curl -fsSL "https://github.com/Ariiisu/AddonManager/releases/download/${ADDONMANAGER_VERSION}/AddonManager.zip" \
        -o /tmp/addonmanager.zip \
    && unzip -qo /tmp/addonmanager.zip -d /staging/ \
    && rm /tmp/addonmanager.zip

# --- Tnms runtime shared libs (required by MapChooserSharpMS) ---
# TnmsLocalizationPlatform ships both a module and a shared lib.
# TnmsPluginFoundation (Core) is a shared lib only.
# Both zips use the modules/ + shared/ layout, so extract into /staging/sharp/.
RUN curl -fsSL "https://github.com/fltuna/TnmsLocalizationPlatform/releases/download/${TNMS_LOCALIZATION_VERSION}/TnmsLocalizationPlatform.zip" \
        -o /tmp/tlp.zip \
    && unzip -qo /tmp/tlp.zip -d /staging/sharp/ \
    && rm /tmp/tlp.zip
RUN curl -fsSL "https://github.com/fltuna/TnmsAdministrationPlatform/releases/download/${TNMS_ADMINISTRATION_VERSION}/TnmsAdministrationPlatform.zip" \
        -o /tmp/tap.zip \
    && unzip -qo /tmp/tap.zip -d /staging/sharp/ \
    && rm /tmp/tap.zip
RUN curl -fsSL "https://github.com/fltuna/TnmsExtendableTargeting/releases/download/${TNMS_TARGETING_VERSION}/TnmsExtendableTargeting.zip" \
        -o /tmp/tet.zip \
    && unzip -qo /tmp/tet.zip -d /staging/sharp/ \
    && rm /tmp/tet.zip
RUN curl -fsSL "https://github.com/fltuna/TNMSPluginFoundation/releases/download/${TNMS_FOUNDATION_VERSION}/TnmsPluginFoundation-Core.zip" \
        -o /tmp/tpf.zip \
    && unzip -qo /tmp/tpf.zip -d /staging/sharp/ \
    && rm /tmp/tpf.zip

# --- Source2Surf/Timer (built from source) ---
COPY --from=plugin-builder /build/out/timer/ /staging/sharp/modules/Timer/
COPY --from=plugin-builder /build/out/timer-rm/ /staging/sharp/modules/Timer.RequestManager/
COPY --from=plugin-builder /build/out/timer-shared/ /staging/sharp/shared/Timer.Shared/

# --- MapChooserSharpMS (built from source) ---
COPY --from=plugin-builder /build/out/mcs/ /staging/sharp/modules/MapChooserSharpMS/
COPY --from=plugin-builder /build/out/mcs-shared/ /staging/sharp/shared/MapChooserSharpMS.Shared/

# Generate runtimeconfig.json if missing (UseCommonOutputDirectory suppresses
# it for some source-built modules; AddonManager's release zip doesn't ship
# one either — without it, ModSharp silently skips the module).
RUN for mod in Timer Timer.RequestManager Timer.Shared MapChooserSharpMS MapChooserSharpMS.Shared AddonManager; do \
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
