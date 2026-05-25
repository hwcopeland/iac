FROM zot.hwcopeland.net/ai/jarvis-edge-base:latest

WORKDIR /app
COPY edge.py .
# JARVIS persona reference for Chatterbox zero-shot voice cloning.
# Same file the Mac daemon uses (~/.openjarvis/jarvis_voice.wav).
COPY jarvis_voice.wav /app/jarvis_voice.wav

ENV EDGE_HTTP_PORT=8088 \
    VOICE_REF=/app/jarvis_voice.wav
EXPOSE 8088

CMD ["python", "-u", "edge.py"]
