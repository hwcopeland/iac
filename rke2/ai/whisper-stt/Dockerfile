FROM zot.hwcopeland.net/ai/whisper-stt-base:latest

WORKDIR /app
COPY server.py .

ENV PORT=8766
EXPOSE 8766

CMD ["python", "server.py"]
