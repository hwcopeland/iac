# Terra++ 1:1-scale Earth Minecraft server (Forge 1.12.2).
#
# Bakes the three tightly-coupled mods into the image so the world generator is
# deterministic and the pod needs no mod-download step at boot. Built on the
# arc-chem self-hosted runner (which has internet) and pushed to zot with OCI
# media types (see .github/workflows/build-mc-earth.yml).
#
# Mod set — all built from the CURRENT heads of their repos by DaPorkchop's
# Jenkins, which is exactly what terraplusplus compiles against, so the three
# are guaranteed mutually compatible (see README.md for the full rationale):
#   - CubicChunks   1.12.2-0.0.1301.0   (CI build #80)
#   - CubicWorldGen 1.12.2-0.0.225.0    (CI build #55)
#   - Terraplusplus 1.1.0.660-1.12.2    (CI build #147)
# Forge is pinned by terraplusplus' gradle: 1.12.2-14.23.5.2847.
#
# The base itzg/minecraft-server tag is pinned by digest in the manifest's
# `image:` field; here we pin the readable tag and the workflow/manifest pin
# the immutable sha. Mods are copied to /modpack and surfaced via COPY_MODS so
# itzg's entrypoint installs them into /data/mods on first boot.

FROM itzg/minecraft-server:java8-multiarch

# Forge 1.12.2 mods require Java 8 (java8-multiarch base provides it).

ARG CUBICCHUNKS_URL="https://jenkins.daporkchop.net/job/OpenCubicChunks/job/CubicChunks/job/MC_1.12/80/artifact/build/libs/CubicChunks-1.12.2-0.0.1301.0-SNAPSHOT-all.jar"
ARG CUBICWORLDGEN_URL="https://jenkins.daporkchop.net/job/OpenCubicChunks/job/CubicWorldGen/job/MC_1.12/55/artifact/build/libs/CubicWorldGen-1.12.2-0.0.225.0-SNAPSHOT-all.jar"
ARG TERRAPLUSPLUS_URL="https://jenkins.daporkchop.net/job/BuildTheEarth/job/terraplusplus/job/master/147/artifact/build/libs/terraplusplus-1.1.0.660-1.12.2.jar"

# Stage the mods in /mods — itzg's COPY_MODS_SRC defaults to /mods and copies
# them into the server mods dir on every boot (verified against the image's
# scripts/start-setupMounts), so a fresh PVC self-populates without any
# network fetch at runtime.
RUN mkdir -p /mods \
 && curl -fSL --retry 5 --retry-delay 5 -o /mods/cubicchunks.jar   "${CUBICCHUNKS_URL}" \
 && curl -fSL --retry 5 --retry-delay 5 -o /mods/cubicworldgen.jar "${CUBICWORLDGEN_URL}" \
 && curl -fSL --retry 5 --retry-delay 5 -o /mods/terraplusplus.jar "${TERRAPLUSPLUS_URL}" \
 && ls -l /mods

ENV EULA="TRUE" \
    TYPE="FORGE" \
    VERSION="1.12.2" \
    FORGE_VERSION="14.23.5.2847"

# Health/probes and resource limits are set in the Kubernetes manifest, not here.
