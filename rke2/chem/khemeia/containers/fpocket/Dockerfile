# fpocket — pocket detection for binding-site definition (WP-1)
# Builds fpocket 4.x from source and wraps it in a Flask REST API.

FROM ubuntu:22.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    make \
    gcc \
    g++ \
    libnetcdf-dev \
    && rm -rf /var/lib/apt/lists/*

RUN git clone --depth 1 https://github.com/Discngine/fpocket.git /opt/fpocket-src

WORKDIR /opt/fpocket-src
RUN make && make install

# Runtime stage
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONUNBUFFERED=1

RUN apt-get update && apt-get install -y --no-install-recommends \
    libnetcdf19 \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Copy fpocket binaries from builder
COPY --from=builder /usr/local/bin/fpocket /usr/local/bin/fpocket
COPY --from=builder /usr/local/bin/dpocket /usr/local/bin/dpocket
COPY --from=builder /usr/local/bin/mdpocket /usr/local/bin/mdpocket
COPY --from=builder /usr/local/bin/tpocket /usr/local/bin/tpocket

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py ./

EXPOSE 8000

CMD ["gunicorn", "-w", "2", "-b", "0.0.0.0:8000", "--timeout", "120", "app:app"]
