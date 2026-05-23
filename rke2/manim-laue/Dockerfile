FROM python:3.11-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        ffmpeg \
        libcairo2 \
        libpango-1.0-0 \
        libpangocairo-1.0-0 \
        libgirepository-1.0-1 \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir \
        manim==0.18.* \
        numpy \
        awscli

WORKDIR /work
COPY laue.py /work/laue.py

# Self-check is fast (~0.1s) and aborts the build if physics is wrong.
RUN python laue.py --self-check

ENTRYPOINT ["manim"]
CMD ["-qh", "laue.py", "LaueDiffraction", "-o", "laue.mp4"]
