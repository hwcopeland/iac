FROM zot.hwcopeland.net/ai/jarvis-edge-base:latest

WORKDIR /app
COPY edge.py .
# JARVIS persona reference for Chatterbox zero-shot voice cloning.
COPY jarvis_voice.wav /app/jarvis_voice.wav
# MCP servers + their backing modules.
COPY jarvis_personal.py jarvis_personal_mcp.py \
     jarvis_spotify.py jarvis_spotify_mcp.py \
     jarvis_delegate_mcp.py \
     jarvis_kube_mcp.py \
     jarvis_sonos_mcp.py \
     jarvis_persona_mcp.py \
     jarvis_tv.py jarvis_tv_mcp.py \
     jarvis_voice_id.py jarvis_voice_enroll.py \
     /app/

ENV EDGE_HTTP_PORT=8088 \
    VOICE_REF=/app/jarvis_voice.wav
EXPOSE 8088

CMD ["python", "-u", "edge.py"]
