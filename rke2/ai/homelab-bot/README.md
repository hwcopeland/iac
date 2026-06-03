# homelab-bot

Discord bot for homelab communities (test server + eventually r/homelab).
Built on [Red-DiscordBot](https://docs.discord.red) with a custom cog
([labbotbrain](https://github.com/hwcopeland/LabBot-Cogs/tree/feat/homelab_jarvis/labbotbrain))
that calls an xAI Grok (or fallback Ollama) backend with two homelab-tuned
tools. Access to the bot is gated by a Discord role allow-list.

## Architecture

```
Discord (test server → eventually r/homelab)
    │
    ▼
Red-DiscordBot pod (this Deployment)
    ├── existing LabBot cogs (google, autoreact, feed, etc.) — fork inherits them
    └── labbotbrain cog (role-gated)
            │
            ▼
    Ollama qwen3:8b (cluster-local at ollama.ai.svc.cluster.local:11434)
        │
        └── native function-calling →
            ├── web_search(query)        — DuckDuckGo HTML, no key
            └── stackexchange_search(q)  — SE API, no key (300/day quota)
```

## Setup (one-time)

1. **Create a Discord Bot Application** at https://discord.com/developers/applications
   - Bot tab → enable **Message Content Intent**, **Server Members Intent**
   - Copy the bot token (only shown once)
   - OAuth2 → URL generator → scopes: `bot` `applications.commands` → bot
     permissions: `Send Messages`, `Read Message History`, `Use Slash Commands`,
     `Embed Links`
2. **Invite the bot** to your test server with that URL.
3. **Create the K8s Secret** (replace `<...>` with real values):
   ```bash
   kubectl -n ai create secret generic homelab-bot-credentials \
       --from-literal=DISCORD_BOT_TOKEN='<bot-token>' \
       --from-literal=OWNER_USER_ID='<your-discord-user-id>'
   ```
4. **Apply** the manifests:
   ```bash
   kubectl apply -f rke2/ai/homelab-bot/pvc.yaml
   kubectl apply -f rke2/ai/homelab-bot/deployment.yaml
   ```
5. **First-run Red bootstrap** — Red wants an "owner" approval on first
   startup. Watch the pod logs:
   ```bash
   kubectl -n ai logs deploy/homelab-bot -f
   ```
   Once it says it's connected, in Discord run `[p]load labbotbrain`
   (you'll need to be set as the owner — handled by the OWNER env).
6. **Populate `LABBOTBRAIN_ALLOWED_ROLES`** in the deployment with the
   comma-separated Discord role IDs that may use the bot. Empty/unset
   means owner-only (fail closed). To get role IDs: User Settings →
   Advanced → Developer Mode on, then right-click a role → Copy Role ID.

## Usage

- `@HomelabBot what's the cheapest used 10G NIC right now?`
- `@HomelabBot why does Proxmox say "TASK ERROR: storage 'X' does not exist"`
- `[p]ask how do I expose a service via Tailscale in k3s?`

## Updating the cog

Push a commit to the `feat/homelab_jarvis` branch of
[hwcopeland/LabBot-Cogs](https://github.com/hwcopeland/LabBot-Cogs).
The init container clones fresh on every pod start, so:

```bash
kubectl -n ai rollout restart deploy/homelab-bot
```

is enough — no rebuild needed.

## Tunables (env on the `red` container)

| Var | Default | What it controls |
|---|---|---|
| `OLLAMA_URL` | `http://ollama.ai.svc.cluster.local:11434` | Brain endpoint |
| `OLLAMA_MODEL` | `qwen3:8b` | Model name; must support function calling |
| `LABBOTBRAIN_MAX_TURNS` | `3` | Tool-use loop budget |
| `LABBOTBRAIN_MAX_REPLY_CHARS` | `1800` | Reply truncation cap (Discord hard cap is 2000) |
| `LABBOTBRAIN_BAIT_PROB` | `0.20` | BAIT MODE roll, 0.0–1.0 |
| `LABBOTBRAIN_ALLOWED_ROLES` | _(empty)_ | CSV of Discord role IDs allowed to invoke the bot. Empty => owner-only. |
| `PREFIX` | `!` | Red command prefix |

## Free-tier API quotas

| API | Free quota | What we hit it for |
|---|---|---|
| DuckDuckGo HTML | none formal; be polite | All `web_search` calls |
| StackExchange | 300 req/day per IP (no key) | All `stackexchange_search` calls |

If we outgrow either, register for a free key (StackExchange has a
trivial app-registration flow) and add to the cog env.
