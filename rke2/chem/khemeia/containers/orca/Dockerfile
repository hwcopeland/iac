# ORCA quantum chemistry engine — RESP charge calculation for ligand FF preparation.
#
# ORCA is free for academic use but is not freely redistributable.
# Download the Linux x86-64 shared-library build from:
#   https://orcaforum.kofo.mpg.de/  (registration required)
# Extract the archive and place the contents in the build context at ./orca/
# (the directory must contain the `orca` executable and its shared libraries).
#
# CI runners must have ORCA pre-staged at:
#   <repo-root>/rke2/chem/khemeia/containers/orca/orca/
# That directory is .gitignored to keep the binary out of version control.

FROM zot.hwcopeland.net/library/ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    libopenmpi-dev \
    openmpi-bin \
    libgomp1 \
    libgfortran5 \
    python3-pip \
    openbabel \
    && rm -rf /var/lib/apt/lists/*

# ORCA binary and shared libraries — must be placed in build context at ./orca/
COPY orca/ /opt/orca/
RUN test -x /opt/orca/orca || ( \
    echo "FATAL: ORCA binary not found at /opt/orca/orca." && \
    echo "Download from https://orcaforum.kofo.mpg.de/ and extract to containers/orca/orca/" && \
    exit 1 \
)

ENV PATH="/opt/orca:${PATH}" \
    LD_LIBRARY_PATH="/opt/orca:${LD_LIBRARY_PATH}" \
    OMPI_ALLOW_RUN_AS_ROOT=1 \
    OMPI_ALLOW_RUN_AS_ROOT_CONFIRM=1

WORKDIR /scripts
COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt \
    && echo '/usr/local/lib/python3.10/dist-packages/acpype/amber_linux/lib' \
       > /etc/ld.so.conf.d/acpype-amber.conf \
    && ldconfig

COPY scripts/ /scripts/

RUN mkdir -p /scratch /input
WORKDIR /scratch

CMD ["python3", "/scripts/resp_charges.py"]
