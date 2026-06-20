FROM zot.hwcopeland.net/ai/jarvis-edge-base:latest

WORKDIR /app

# ── CAM++ speaker-verification ONNX (VOICE_MODEL=campplus) ───────────────────
# Provenance (PIN — do NOT use a floating "latest"):
#   wespeaker CAM++ VoxCeleb, English, 16 kHz, 80-dim fbank in / 192-d emb out.
#   Source:  https://wespeaker-1256283475.cos.ap-shanghai.myqcloud.com/
#            models/voxceleb/voxceleb_CAM++.onnx
#   Mirror (ModelScope, equivalent weights):
#     iic/speech_campplus_sv_en_voxceleb_16k  →  campplus_voxceleb.onnx
#   ~28 MB, CPU. I/O spec: input (1, T, 80) float32 CMN'd kaldi fbank;
#   output (1, 192) float32 embedding. See jarvis_voice_id._campplus_* and the
#   RUNBOOK "CAM++ I/O spec" section — the frontend MUST match these exactly.
#
# TODO-TO-FETCH: this build line is written against the documented URL but the
# file has NOT been verified from this environment. Before the first campplus
# build, confirm the URL resolves (or vendor the file into the build context as
# models/campplus_voxceleb.onnx and switch this RUN to a COPY). If the download
# fails the image still builds with resemblyzer working; campplus will raise
# FileNotFoundError at first use (guarded in _campplus_session).
ARG CAMPPLUS_ONNX_URL="https://wespeaker-1256283475.cos.ap-shanghai.myqcloud.com/models/voxceleb/voxceleb_CAM++.onnx"
RUN mkdir -p /app/models \
 && (curl -fsSL -o /app/models/campplus_voxceleb.onnx "${CAMPPLUS_ONNX_URL}" \
     || echo "WARN: CAM++ ONNX fetch failed; campplus backend unavailable until vendored")

COPY edge.py .
# JARVIS persona reference for Chatterbox zero-shot voice cloning.
COPY jarvis_voice.wav /app/jarvis_voice.wav
# MCP servers + their backing modules.
COPY jarvis_personal.py jarvis_personal_mcp.py \
     jarvis_delegate_mcp.py \
     jarvis_kube_mcp.py \
     jarvis_overview_mcp.py \
     jarvis_mem0_mcp.py \
     jarvis_sonos_mcp.py \
     jarvis_persona_mcp.py \
     jarvis_voice_id.py jarvis_voice_enroll.py jarvis_identity.py \
     calibrate_voiceid.py \
     jarvis_local_brain.py \
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
