# Phase 6 — mem0 long-term memory for JARVIS (self-hosted)

**Status:** manifests authored, NOT deployed. Deploy is a later manual step.
**Depends on:** Phase 5 (warm brain) — see `phase5-warm-brain.md`. Memory tools work without
the warm brain, but *cross-turn continuity* (the point of memory) needs the long-lived OWNER
session; on the cold per-turn path each turn is a fresh subprocess that can only recall what it
explicitly `memory_search`es. With the warm session, recalled memories persist in-context across
the conversation.
**Scope key:** binds to the `mem_scope` thread built in Phase 0/1.
**Manifests:** `rke2/ai/mem0/`.

---

## 1. Why this shape

The plan locks: **self-hosted, Qdrant + mem0 + fastembed (CPU), no ollama, no GPU**, exposed to
the **OWNER brain only** as an MCP server. The single RTX 3070 must stay free for
Chatterbox/Whisper, so nothing in this stack touches the GPU node.

```
  jarvis-edge pod (nixos-gpu, OWNER brain only)
    └─ claude CLI subprocess  (JARVIS_MEM_SCOPE in env — already threaded by _claude_brain)
         └─ stdio MCP: jarvis_mem0_mcp.py     [thin shim, ships in edge image]
               │  HTTP  (cluster-internal)
               ▼
  jarvis-mem0  Deployment (microedge, CPU)     [mem0 lib + fastembed + anthropic]
    └─ TCP
         ▼
  qdrant  StatefulSet (microedge, CPU, Longhorn PVC)   [vector store]
```

**Three-process split, on purpose:**
- **Heavy deps off the edge.** mem0 + fastembed (onnxruntime) + anthropic are big. Keeping them
  in a separate `jarvis-mem0` image keeps the edge image (on the GPU node) small. The edge only
  gains a ~200-line stdio shim with zero new pip deps (stdlib `urllib` only).
- **Off the GPU node.** Both Qdrant and the mem0 server run on **microedge** (a Longhorn node),
  exactly like `open-webui`. This dodges the nixos-gpu Cilium host→pod quirk AND gives real CSI
  storage (nixos-gpu has no CSI driver). No `hostNetwork` needed — these are plain ClusterIP
  services reached over Service DNS.
- **One owner of the Qdrant client + extraction key.** The mem0 server is the only thing that
  holds the Anthropic key and talks to Qdrant.

---

## 2. The extraction-LLM decision (the open question, resolved)

mem0 runs an LLM **on every `add()`** to extract durable facts from raw turns. mem0's LLM
provider expects an **API key**. The edge brain authenticates to Claude via **subscription
OAuth** (`~/.claude/.credentials.json`, the `claude` CLI) — there is **no `ANTHROPIC_API_KEY`**
on the edge.

Options considered:

| Option | Verdict |
|---|---|
| **(a) Dedicated Anthropic API key for mem0 extraction (Haiku)** | **CHOSEN.** |
| (b) Route extraction through the `claude` CLI | Rejected — infeasible. |
| (c) "Cheaper extraction model" | Subsumed — Haiku *is* the cheap model. |

**Why (b) is infeasible:** the `claude` CLI is an interactive *agent*, not an
OpenAI/Anthropic-compatible chat completion endpoint that mem0's LLM provider can POST to. mem0
calls `llm.generate_response(messages)`; there is no clean way to make that drive a CLI
subprocess per extraction, and the subscription OAuth token cannot be handed to mem0's
`anthropic` provider (it's not an API key, and using it programmatically would violate the
subscription's terms). So extraction needs its own real API key regardless.

**Why (a) over (c):** "a cheaper model" is the same decision — the cheapest *capable* extraction
model is **Haiku 4.5**. Extraction is a small, well-bounded task (read a short turn + a few
existing memories, emit structured facts), so Haiku is the right tier; no need for Sonnet.

**Cost (rough):** per `add()` ≈ ~1.1k input + ~0.3k output tokens. At Haiku pricing (~$1/Mtok
in, ~$5/Mtok out) that's **~$0.0026/turn**:
- 50 owner turns/day → **~$3.90/mo**
- 200 owner turns/day → **~$15.60/mo**

Trivial, and it only fires on OWNER turns (TRUSTED never reaches the memory tools — see §4).
The key is **dedicated and scoped** (`mem0-anthropic-credentials` Secret, Bitwarden-sourced),
separate from any other Anthropic key, so spend is isolated and it can be rotated/revoked alone.

> Future option if cost ever matters: point the mem0 server's extraction LLM at a local model.
> Explicitly **not** done now — that would mean ollama/GPU, which the plan forbids. fastembed
> already covers the *embedding* half on CPU; only *extraction* needs the API key.

**Embeddings:** `fastembed` with `BAAI/bge-small-en-v1.5` (384-dim, ~130MB, strong CPU model),
pre-downloaded at image build. Pure CPU, no GPU, no ollama, no per-call API cost.

---

## 3. Partitioning: `mem_scope` → mem0 `user_id`

mem0 stores and retrieves memories within a **`user_id` namespace**. JARVIS maps its
`mem_scope` directly onto that:

- `Principal.mem_scope` (`jarvis_identity.py`) = `"owner"` for the owner (voice **or** platform,
  canonically collapsed), else the per-user `user_id` (`voice:alex`, `discord:123`, …).
- `_claude_brain` already threads it: `JARVIS_MEM_SCOPE = principal.mem_scope` in the subprocess
  env (`edge.py:852`). Today it's inert plumbing; Phase 6 makes it live.
- `jarvis_mem0_mcp.py` reads `JARVIS_MEM_SCOPE` from its **own** env (inherited from the brain
  subprocess) and sends it as `user_id` on every `/add` and `/search`. **The brain cannot
  override it** — there is no `user_id` tool argument. The model never chooses whose memory to
  touch.
- The mem0 server **rejects any request with an empty `user_id`** (`_require_scope`), so a
  misconfigured caller fails loudly instead of writing to a shared namespace.

### Why owner memories are structurally unreachable by non-owners

This is the same guarantee as the rest of the gate — enforced by *plumbing*, not by trusting the
model. Two independent barriers:

1. **The locked (TRUSTED) brain has no `--mcp-config` at all** (`_claude_brain_voice_locked`,
   `edge.py:900`). It loads **zero** MCP servers, so `jarvis_mem0` simply isn't present on a
   non-owner turn. There is no memory tool to call.
2. **Even if a tool were somehow reachable, the scope is env-pinned.** A TRUSTED turn would
   carry `JARVIS_MEM_SCOPE = "voice:alex"` (its own scope), never `"owner"`, so it could only
   ever touch *its own* namespace — never the owner's.

Barrier (1) is the primary control (matches the plan's Layer-A: "no `--mcp-config` → memory tool
structurally unreachable"). Barrier (2) is defense in depth. **Do not** add the mem0 server to
the locked brain's config under any circumstance.

---

## 4. Manifests (what's in `rke2/ai/mem0/`)

MANUAL kubectl-apply, `ai` namespace, NOT Flux — matches jarvis-edge/chatterbox style.

| File | What |
|---|---|
| `qdrant-statefulset.yaml` | Qdrant `qdrant/qdrant:v1.12.4`, 1 replica, microedge, `volumeClaimTemplate` 10Gi Longhorn. ClusterIP only. |
| `qdrant-service.yaml` | `qdrant.ai.svc:6333` (http) / `:6334` (grpc), ClusterIP. |
| `qdrant-pvc.yaml` | OPTIONAL standalone PVC (doc of size/class intent). The StatefulSet uses its own template; don't double-apply. |
| `mem0-server-deployment.yaml` | `jarvis-mem0` Deployment, microedge, CPU-only, `Recreate`. Pulls the Anthropic key from the Secret. Image `zot.hwcopeland.net/ai/jarvis-mem0:latest`. |
| `mem0-server-service.yaml` | `jarvis-mem0.ai.svc:8800`, ClusterIP. |
| `external-secret-mem0.yaml` | ESO → Bitwarden (`bitwarden-fields`) → `mem0-anthropic-credentials` Secret (`ANTHROPIC_API_KEY`). **TODO: paste the Bitwarden item UUID.** |
| `build/Dockerfile` + `build/server.py` + `build/requirements.txt` | The `jarvis-mem0` image. Built on the cluster (Kaniko/nerdctl/CI → zot), NEVER on the Mac. |
| `jarvis_mem0_mcp.py` | The stdio MCP shim — **belongs in the edge image** (copy to `jarvis-edge/`, see §5). |

**Apply order** (after the image is built+pushed to zot):
```
kubectl apply -f rke2/ai/mem0/external-secret-mem0.yaml
kubectl apply -f rke2/ai/mem0/qdrant-statefulset.yaml -f rke2/ai/mem0/qdrant-service.yaml
kubectl apply -f rke2/ai/mem0/mem0-server-deployment.yaml -f rke2/ai/mem0/mem0-server-service.yaml
```

**Build the image** (on the cluster, NOT the Mac — hand off to @tooling-engineer / the ARC build
path that builds whisper/tts/jarvis): build `rke2/ai/mem0/build/` →
`zot.hwcopeland.net/ai/jarvis-mem0:latest`, then `kubectl -n ai rollout restart
deploy/jarvis-mem0`.

**Verify:**
```
kubectl -n ai get pods -l component=jarvis-mem0 -o wide      # both on microedge
curl -s http://jarvis-mem0.ai.svc.cluster.local:8800/health | jq .
curl -s http://qdrant.ai.svc.cluster.local:6333/readyz
# round-trip (owner scope):
curl -s -XPOST http://jarvis-mem0.ai.svc:8800/add \
  -d '{"text":"I take my coffee black","user_id":"owner"}' | jq .
curl -s -XPOST http://jarvis-mem0.ai.svc:8800/search \
  -d '{"query":"how do I take coffee","user_id":"owner"}' | jq .
# scope isolation: a different user_id must NOT see the owner's fact:
curl -s -XPOST http://jarvis-mem0.ai.svc:8800/search \
  -d '{"query":"coffee","user_id":"voice:alex"}' | jq .   # → empty
```

---

## 5. EXACT edge.py integration (apply later — do NOT apply now)

> The agent that produced this doc did **not** edit `edge.py` (you own it on the critical path).
> Below is everything to apply yourself. Three edits + one Dockerfile line. None of this changes
> the gate or the TRUSTED/locked paths — it only adds two tools to the OWNER brain's surface.

### 5.1 Ship the shim in the edge image

```sh
cp rke2/ai/mem0/jarvis_mem0_mcp.py rke2/ai/jarvis-edge/jarvis_mem0_mcp.py
```

Add it to the Dockerfile COPY block (`rke2/ai/jarvis-edge/Dockerfile`), e.g. on the line with
the other MCP modules:

```dockerfile
     jarvis_sonos_mcp.py \
     jarvis_persona_mcp.py \
     jarvis_mem0_mcp.py \
```

### 5.2 Register the MCP server — `_write_mcp_config()` (edge.py ~474)

Add ONE entry to the `mcpServers` dict (alongside `jarvis_persona`). The shim reads
`JARVIS_MEM_SCOPE` from the **subprocess env**, which `_claude_brain` already sets — so nothing
needs to be passed in `env` here. (Optionally pin `MEM0_URL` if you ever move the service.)

```python
            "jarvis_mem0": {
                "command": "python3",
                "args": ["/app/jarvis_mem0_mcp.py"],
            },
```

> IMPORTANT: add this **only** in `_write_mcp_config()` (the OWNER/full-brain config). Do NOT
> give the locked brain any `--mcp-config`. `_claude_brain_voice_locked` and
> `_claude_brain_discord_locked` must stay config-less — that is what makes owner memories
> structurally unreachable by non-owners (§3).

### 5.3 Allow the two tools — `_RO_ALLOWED_TOOLS` (edge.py ~628)

Add two entries to the list (anywhere; next to the persona tools reads well):

```python
    # Long-term memory (mem0) — OWNER ONLY. The locked brain has no
    # --mcp-config, so these are unreachable on non-owner turns. Scope is
    # pinned to JARVIS_MEM_SCOPE in the subprocess env; the brain can't
    # choose whose memory to touch.
    "mcp__jarvis_mem0__memory_search",
    "mcp__jarvis_mem0__memory_add",
```

### 5.4 (Optional) nudge the persona to use memory

The tools are self-describing, but a one-liner in `_PERSONA_SYSTEM` (edge.py ~438, the Tools
block) improves recall behavior:

```
- For "remember that...", "don't forget...", a stated preference, or a fact
  about a person/project worth keeping, call mcp__jarvis_mem0__memory_add.
  Before answering anything that refers to the past ("what did I say about",
  "my usual", "who is..."), call mcp__jarvis_mem0__memory_search first.
```

### 5.5 What you do NOT change

- `gate_and_respond` — untouched. OWNER still → `brain_respond`; TRUSTED → locked; UNKNOWN →
  state machine.
- `_claude_brain_voice_locked` / `_claude_brain_discord_locked` — stay config-less. Never add
  the mem0 server here.
- `JARVIS_MEM_SCOPE` threading (`edge.py:852`) — already correct; no change. The Phase 5 warm
  session keeps threading it the same way.
- No change to the TRUSTED, UNKNOWN, or open-mode paths.

---

## 6. Interaction with Phase 5 (warm brain)

- Phase 5 keeps `_RO_ALLOWED_TOOLS` and `--mcp-config _MCP_CONFIG_PATH` as the warm OWNER
  session's tool surface (`phase5-warm-brain.md` §3.2). So **the §5.2/§5.3 edits light up the
  mem0 tool in both the cold path (today) and the warm session (Phase 5) with no extra work.**
- The warm session is OWNER-only and TRUSTED never enters it — the same property that keeps the
  warm session owner-scoped keeps the memory tool owner-scoped.
- Continuity payoff lands with Phase 5: recalled memories stay in the warm session's context
  across turns instead of being re-fetched per cold subprocess.

---

## 7. Known gotchas / follow-ups

- **Bitwarden UUID is a placeholder** in `external-secret-mem0.yaml` — create the "Anthropic —
  JARVIS mem0" item (custom field `api_key`) and paste its UUID before applying, or the Secret
  stays empty and the mem0 server fails its first `add()` loudly (`/health` shows
  `has_api_key: false`).
- **Embedding dims are fixed at collection-create.** `FASTEMBED_DIMS=384` matches
  `bge-small-en-v1.5`. If you ever swap the embed model to a different dimension, you must drop &
  recreate the Qdrant collection (it's keyed to dim) — there's no in-place reindex.
- **Single-replica Qdrant, single owner.** Fine for one person's memory corpus. If it ever needs
  HA, that's a Qdrant clustering change, not a JARVIS change.
- **Storage growth is slow** (extracted facts are tiny), but the 10Gi Longhorn PVC is generous;
  watch it in Longhorn like any other PVC.
- **No HTTPRoute / no external exposure** by design — both services are ClusterIP. The only
  client is the in-cluster edge pod.
