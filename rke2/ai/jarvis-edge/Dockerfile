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
     jarvis_overview_mcp.py \
     jarvis_mem0_mcp.py \
     jarvis_sonos_mcp.py \
     jarvis_persona_mcp.py \
     jarvis_tv.py jarvis_tv_mcp.py \
     jarvis_voice_id.py jarvis_voice_enroll.py jarvis_identity.py \
     jarvis_ig_polling.py jarvis_ig_consumer.py \
     jarvis_ig_comment_responder.py jarvis_reel_context.py \
     jarvis_ig_media_dl.py jarvis_song_id.py \
     jarvis_ig_followup_poller.py \
     jarvis_ig_cooldown.py \
     jarvis_local_brain.py \
     jarvis_discord.py \
     jarvis_runner_mcp.py \
     jarvis_run_announce.py \
     /app/
# Cluster-side agent runner (charter roadmap #2): the Job entrypoint +
# its narrowed MCP config. launch_run creates a Job whose container runs
# `python -u runner/jarvis_runner_entrypoint.py` from WORKDIR /app.
COPY runner/jarvis_runner_entrypoint.py runner/runner_mcp_config.json \
     /app/runner/

ENV EDGE_HTTP_PORT=8088 \
    VOICE_REF=/app/jarvis_voice.wav
EXPOSE 8088

CMD ["python", "-u", "edge.py"]
