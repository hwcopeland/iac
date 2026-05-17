FROM zot.hwcopeland.net/ai/chatterbox-tts-base:latest

WORKDIR /app
COPY server.py .

ENV PORT=8765
EXPOSE 8765

CMD ["python", "server.py"]
