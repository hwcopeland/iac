# Base image for building
FROM zot.hwcopeland.net/library/ubuntu:20.04 AS builder
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Etc/UTC
WORKDIR /autodock/

# Install necessary build tools and dependencies
RUN apt-get update && apt-get install -y \
    wget \
    build-essential \
    git \
    cmake \
    libnetcdf-dev \
    libboost-all-dev && \
    rm -rf /var/lib/apt/lists/*

# Download AutoDock Vina 1.1.2
# vina.scripps.edu redirects to forlilab.org with a mismatched TLS cert;
# using wget with --no-check-certificate to work around the server-side
# cert misconfiguration (content is from the legitimate CCSB Scripps server).
RUN wget --no-check-certificate -q \
    https://vina.scripps.edu/wp-content/uploads/sites/55/2020/12/autodock_vina_1_1_2_linux_x86.tgz && \
    tar xzvf autodock_vina_1_1_2_linux_x86.tgz

# Runtime image
FROM zot.hwcopeland.net/library/ubuntu:20.04
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Etc/UTC

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    wget \
    openbabel \
    nano \
    parallel \
    libnetcdf-dev \
    libboost-filesystem-dev \
    libboost-system-dev \
    libboost-program-options-dev \
    python3 \
    python3-pip \
    git && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /autodock/

# Copy binaries from builder stage
COPY --from=builder /autodock/autodock_vina_1_1_2_linux_x86/bin/vina ./vina
RUN ldconfig

# Install Python packages
RUN python3 -m pip install git+https://github.com/Valdes-Tresanco-MS/AutoDockTools_py3

# Install Python3 dependencies
COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

# Copy script files using COPY instead of ADD
COPY scripts/ ./scripts/

# Ensure scripts are executable
RUN chmod +x ./scripts/*.py 2>/dev/null || true
