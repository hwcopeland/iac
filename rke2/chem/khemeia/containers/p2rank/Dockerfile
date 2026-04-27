# P2Rank — machine-learning pocket prediction for binding-site definition (WP-1)
# Downloads a P2Rank release and wraps it in a Flask REST API.

FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONUNBUFFERED=1

# P2Rank version — bump this to upgrade
ENV P2RANK_VERSION=2.4.2

RUN apt-get update && apt-get install -y --no-install-recommends \
    openjdk-17-jre-headless \
    wget \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Download and extract P2Rank
RUN wget -q "https://github.com/rdk/p2rank/releases/download/${P2RANK_VERSION}/p2rank_${P2RANK_VERSION}.tar.gz" \
        -O /tmp/p2rank.tar.gz && \
    tar xzf /tmp/p2rank.tar.gz -C /opt/ && \
    mv /opt/p2rank_${P2RANK_VERSION} /opt/p2rank && \
    chmod +x /opt/p2rank/prank && \
    rm /tmp/p2rank.tar.gz

ENV PATH="/opt/p2rank:${PATH}"

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py ./

EXPOSE 8000

CMD ["gunicorn", "-w", "2", "-b", "0.0.0.0:8000", "--timeout", "300", "app:app"]
