#!/bin/bash
set -e
echo "[kai-agent] role=${KAI_AGENT_ROLE} run=${KAI_RUN_ID}"
echo "[kai-agent] stub: no LLM integration in Phase 0"
# Phase 2 will replace this with real agent logic:
#   - Read KAI_RUN_ID, KAI_AGENT_ROLE, KAI_CALLBACK_URL, KAI_CALLBACK_TOKEN from env
#   - Execute role-specific work (planner / researcher / coder / reviewer)
#   - POST result to KAI_CALLBACK_URL with X-Kai-Callback-Token header
