FROM zot.hwcopeland.net/ai/jarvis-edge-base:latest

WORKDIR /app
COPY edge.py .

ENV EDGE_HTTP_PORT=8088
EXPOSE 8088

CMD ["python", "-u", "edge.py"]
