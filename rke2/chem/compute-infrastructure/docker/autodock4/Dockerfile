# Base image for building
FROM ubuntu:20.04 AS builder
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

# Download AutoDock4
ADD https://autodock.scripps.edu/wp-content/uploads/sites/56/2021/10/autodocksuite-4.2.6-x86_64Linux2.tar ./
RUN tar xf autodocksuite-4.2.6-x86_64Linux2.tar

# Runtime image
FROM ubuntu:20.04
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
COPY --from=builder /autodock/x86_64Linux2/autogrid4 ./autogrid4
COPY --from=builder /usr/local/lib/libfpocket.so* /usr/local/lib/
COPY --from=builder /autodock/x86_64Linux2/autodock4 ./autodock4
RUN ldconfig

# Install Python packages
RUN python3 -m pip install git+https://github.com/Valdes-Tresanco-MS/AutoDockTools_py3

# Install Python3 dependencies
COPY requirements.txt ./
RUN pip3 install --no-cache-dir -r requirements.txt

# Copy script files using COPY instead of ADD
COPY scripts/ ./scripts/

# Convert CRLF to LF and set execute permissions for all .sh scripts
RUN apt-get update && apt-get install -y sed && \
    sed -i 's/\r$//' ./scripts/*.sh && \
    chmod +x ./scripts/*.sh && \
    rm -rf /var/lib/apt/lists/*
