FROM python:3.11-slim

# Runtime libs are kept; build tools + dev headers are installed only for the
# pip step and removed in the same layer to keep the image small.
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
        ffmpeg \
        libcairo2 \
        libpango-1.0-0 \
        libpangocairo-1.0-0 \
        libgirepository-1.0-1 \
        ca-certificates \
        build-essential \
        pkg-config \
        libcairo2-dev \
        libpango1.0-dev \
        meson \
        ninja-build \
    ; \
    pip install --no-cache-dir \
        manim==0.18.* \
        numpy \
        awscli \
    ; \
    apt-get purge -y --auto-remove \
        build-essential \
        pkg-config \
        libcairo2-dev \
        libpango1.0-dev \
        meson \
        ninja-build \
    ; \
    rm -rf /var/lib/apt/lists/* /root/.cache

WORKDIR /work
COPY laue.py /work/laue.py

# Self-check is fast (~0.1s) and aborts the build if physics is wrong.
RUN python laue.py --self-check

ENTRYPOINT ["manim"]
CMD ["-qh", "laue.py", "LaueDiffraction", "-o", "laue.mp4"]
