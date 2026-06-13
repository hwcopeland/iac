# rke2/ai/mem0/ — JARVIS self-hosted memory stack (Phase 6)

MANUAL kubectl-apply, `ai` namespace. NOT Flux. Deploy AFTER Phase 5 (warm brain).

Components (all CPU-only — NOTHING here touches the RTX 3070):
- `qdrant-*.yaml`      — Qdrant vector store (StatefulSet + Longhorn PVC + Service) on microedge
- `mem0-server-*.yaml` — mem0 REST service (Deployment + Service) on microedge
- `build/`             — Dockerfile + app for the mem0 REST service (built on cluster, never on Mac)
- `external-secret-*.yaml` — Anthropic API key for mem0 extraction (Bitwarden via ESO)
- `jarvis_mem0_mcp.py` — stdio MCP shim to COPY into jarvis-edge/ later (see phase6-mem0.md)

Apply order:
  kubectl apply -f external-secret-mem0.yaml
  kubectl apply -f qdrant-pvc.yaml qdrant-statefulset.yaml qdrant-service.yaml
  kubectl apply -f mem0-server-deployment.yaml mem0-server-service.yaml
(image must be built+pushed to zot first — see build/)

See ../../docs/jarvis/phase6-mem0.md for architecture + the edge.py integration snippet.
