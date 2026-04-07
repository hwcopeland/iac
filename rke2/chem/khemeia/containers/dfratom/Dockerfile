# DFRATOM - Dirac-Fock-Roothaan Atomic Calculator
# Relativistic atomic structure calculations for heavy elements

# Stage 1: Compile DFRATOM from Fortran source
FROM ubuntu:24.04 AS builder

RUN apt-get update && apt-get install -y \
    gfortran \
    make \
    liblapack-dev \
    libblas-dev \
    && rm -rf /var/lib/apt/lists/*

COPY src/dfratom.f /opt/dfratom-src/dfratom.f
WORKDIR /opt/dfratom-src
RUN gfortran -fallow-argument-mismatch -mcmodel=large -O2 \
    dfratom.f -o DFRATOM.exe -llapack -lblas && \
    strip DFRATOM.exe

# Stage 2: Minimal runtime
FROM ubuntu:24.04

RUN apt-get update && apt-get install -y \
    libgfortran5 \
    liblapack3 \
    libblas3 \
    libopenblas0-pthread \
    python3 \
    --no-install-recommends && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /opt/dfratom-src/DFRATOM.exe /opt/dfratom/DFRATOM.exe
RUN chmod +x /opt/dfratom/DFRATOM.exe

COPY input_gen.py /app/input_gen.py

WORKDIR /scratch
ENV PYTHONUNBUFFERED=1
