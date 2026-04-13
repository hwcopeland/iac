# Psi4 - Open-Source Quantum Chemistry
# Uses the official Psi4 conda-forge image and rebases the runtime onto
# zot-mirrored Ubuntu for registry consistency. The plugin command expects
# psi4 at /opt/conda/bin/psi4 and working dirs /scratch + /input.

FROM psi4/psi4:latest AS upstream

# Runtime -- slim Ubuntu with the conda env copied in.
FROM zot.hwcopeland.net/library/ubuntu:22.04

RUN apt-get update && apt-get install -y \
    libgomp1 \
    libgfortran5 \
    --no-install-recommends && \
    rm -rf /var/lib/apt/lists/*

# Copy the entire conda environment from the upstream image.
COPY --from=upstream /opt/conda /opt/conda

ENV PATH="/opt/conda/bin:${PATH}" \
    PSI_SCRATCH=/scratch

# Khemeia runtime layout: plugin commands cp /input/* to /scratch.
RUN mkdir -p /scratch /input
WORKDIR /scratch
