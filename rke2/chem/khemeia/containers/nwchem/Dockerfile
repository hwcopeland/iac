# NWChem - Northwest Computational Chemistry Package
# Uses the official NWChem community image as the base and adds the
# runtime layout expected by the Khemeia plugin (working dir /scratch,
# input mount at /input, nwchem + mpirun in PATH).

FROM nwchemorg/nwchem-qc:latest AS upstream

# Runtime image -- rebase onto zot-mirrored Ubuntu for registry consistency
# and layer-cache benefits shared with other Khemeia images.
FROM zot.hwcopeland.net/library/ubuntu:22.04

RUN apt-get update && apt-get install -y \
    libopenmpi-dev \
    openmpi-bin \
    libscalapack-openmpi-dev \
    libgfortran5 \
    python3-minimal \
    --no-install-recommends && \
    rm -rf /var/lib/apt/lists/*

# Copy the NWChem installation from the upstream image.
COPY --from=upstream /opt/nwchem /opt/nwchem

# NWChem needs NWCHEM_TOP, NWCHEM_TARGET, and the basis library path.
ENV NWCHEM_TOP=/opt/nwchem \
    NWCHEM_TARGET=LINUX64 \
    NWCHEM_BASIS_LIBRARY=/opt/nwchem/src/basis/libraries/ \
    PATH="/opt/nwchem/bin/LINUX64:${PATH}" \
    FFIELD=amber \
    AMBER_1=/opt/nwchem/src/data/amber_s/ \
    AMBER_2=/opt/nwchem/src/data/amber_q/ \
    AMBER_3=/opt/nwchem/src/data/amber_x/ \
    AMBER_4=/opt/nwchem/src/data/amber_u/ \
    SPCE=/opt/nwchem/src/data/solvents/spce.rst \
    OMPI_ALLOW_RUN_AS_ROOT=1 \
    OMPI_ALLOW_RUN_AS_ROOT_CONFIRM=1

# Khemeia runtime layout: plugin commands cp /input/* to /scratch.
RUN mkdir -p /scratch /input
WORKDIR /scratch
