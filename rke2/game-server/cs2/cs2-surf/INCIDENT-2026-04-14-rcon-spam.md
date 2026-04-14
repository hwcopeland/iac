# Incident — 2026-04-14 — cs2-surf RCON chat spam

## Summary

External attacker(s) used CS2's `-usercon` TCP RCON listener (exposed publicly
via the cs2-modded-service LoadBalancer on port 27015/TCP) to authenticate
and run `say <spam>` server-side commands, broadcasting affiliate / scam
links to all connected players prefixed with `Console: ...`.

Reported by user at approximately 2026-04-14 ~00:50 UTC.

## Observed message

> `Console: (18+ Claim 3 free cases & a 50% ...)`

Full text was not captured — see "What we don't know" below.

## Vector

CS2 dedicated server with `-usercon +rcon_password <pw>` opens a Source-RCON
TCP listener on the same port as the game (27015). The cs2-modded-service
Service was a `type: LoadBalancer` exposing both UDP and **TCP** for ports
27015 and 27020. Anyone who could reach `10.44.0.32:27015/TCP` from the
public internet could attempt RCON authentication and, on success, run any
server command — including `say "<text>"`.

The "Console:" prefix in CS2 chat is the engine's marker for `say` issued
from the server (no client source) — exactly what RCON `say` produces.

## Likely attack chain

1. Attacker scrapes CS2 servers from the Steam server browser (or via
   `A2S_INFO` masterlist queries), filtering for ones that respond on
   27015/TCP from the public internet.
2. Attempts RCON authentication with `rcon_password` candidates: blank,
   common defaults, dictionary guesses, leaked password lists.
3. On success, sends `rcon say "<affiliate spam>"` periodically.
4. Disconnects. The attack is short-lived — no persistent connection,
   so neither `/proc/net/tcp` nor connection-tracking show evidence
   minutes after the fact.

How they got the password is unknown without per-attempt logs. Our
`rcon_password` is sourced from the `cs2-rcon` k8s secret which has not
been rotated since at least the kus deployment (~80 days). It is plausible
the password leaked at some earlier point or that the attacker bruteforced
it (CS2 has no per-IP RCON throttle by default).

## What we know

- **Pod**: `cs2-surf-server-55666fcdc-hpzg6` in `game-server` namespace,
  pod IP `10.42.4.80`.
- **Service before fix**: `cs2-modded-service` LoadBalancer at
  `10.44.0.32`, exposing tcp-27015, udp-27015, tcp-27020, udp-27020.
- **Service after fix**: tcp-27015 and tcp-27020 ports REMOVED. UDP
  ports retained for game traffic. `kubectl patch svc cs2-modded-service
  --type=json -p '[{"op":"replace","path":"/spec/ports","value":[...]}]'`
  applied at the time of incident response.
- **Internal RCON still works**: the entrypoint connects to the pod's own
  IP from inside the pod, which does not traverse the Service. Verified
  with `bash -c "echo > /dev/tcp/10.42.4.80/27015"` from inside the
  container — connect succeeds.
- **CS2's TCP listener inside the pod**: still bound to `10.42.4.80:27015`
  (state 0A = LISTEN). CS2 does NOT bind to 127.0.0.1, so loopback
  connections refuse.
- **Loki coverage**: monitor/loki is up but only has 2 `pod` label values
  total — promtail/alloy is not scraping the `game-server` namespace, so
  there is no historical log retention for cs2-surf-server beyond what
  `kubectl logs` can return for the live pod.
- **CS2 native chat log**: not enabled. `mp_logfile`/`sv_logfile` were
  never set, so no `csgo/logs/` directory exists. This means no historical
  record of the spam text or the attacker's `say` invocations.
- **No active attacker connections** at the time of forensic check. Only
  in-pod RCON connections (the entrypoint's two CLOSE_WAIT sockets to its
  own IP).

## What we don't know

- **Attacker source IP(s)**. Not in any log we have access to.
- **Full spam message text**. User reported only the truncated prefix
  `(18+ Claim 3 free cases & a 50%`. The rest, which would normally
  contain a URL and an affiliate / referral code, was not captured.
- **Affiliate / referral code**. Without the full text, we cannot
  identify the affiliate account that would have benefited from clicks
  on the spam link, and therefore cannot file an abuse report against
  that account with the gambling site. Common patterns:
  `?aff=<code>`, `?ref=<code>`, `/r/<code>`, `code=<code>`.
- **How long the attack had been ongoing** before the user noticed.
  Could be minutes (single bot run) or weeks (intermittent).
- **Whether the rcon_password was guessed, leaked, or hardcoded** in
  some old config that's now public.

## Mitigations applied

1. **Service ports**: TCP 27015 and TCP 27020 removed from
   cs2-modded-service. Public scanner bots can no longer reach the
   RCON TCP listener at all. Game traffic (UDP) unaffected. Applied
   live via `kubectl patch`; manifest update committed in
   `k8s/service-patch.yaml` so it sticks across redeploys.

## Recommended follow-ups

In priority order:

1. **Rotate `cs2-rcon` secret immediately.** Even with TCP exposure
   removed, treat the existing password as compromised. New value
   ~32 chars, alphanumeric + symbols.
   ```
   NEW=$(openssl rand -base64 24 | tr -d '/+=')
   kubectl -n game-server create secret generic cs2-rcon \
     --from-literal=password="$NEW" --dry-run=client -o yaml \
     | kubectl apply -f -
   kubectl -n game-server rollout restart deploy/cs2-surf-server
   ```

2. **Enable native CS2 chat logging** so the next attack is
   captured at source. Add to `configs/server.cfg`:
   ```
   log on
   mp_logfile 1
   mp_logmessages 1
   mp_logdetail 3
   sv_logfile 1
   ```
   CS2 will then write per-day log files to `csgo/logs/` including
   every `say` invocation with a source label.

3. **Add a chat listener to SurfMapCommand for non-command say** so
   plugin-side logging captures every chat line, including
   `Console:` server-side broadcasts. The current `OnClientSayCommand`
   only fires for player-issued chat, not `say` from the server itself.
   May need a different ModSharp hook (player chat event vs server
   command listener) — needs investigation.

4. **Cilium NetworkPolicy** as defense-in-depth. Even if someone
   re-adds TCP 27015 to the Service later, a NetworkPolicy on the
   cs2-surf-server pod can deny inbound TCP from non-cluster sources.
   ```yaml
   apiVersion: cilium.io/v2
   kind: CiliumNetworkPolicy
   metadata:
     name: cs2-surf-no-external-tcp
     namespace: game-server
   spec:
     endpointSelector:
       matchLabels:
         app: cs2-surf-server
     ingress:
       - toPorts:
           - ports:
               - port: "27015"
                 protocol: UDP
               - port: "27020"
                 protocol: UDP
   ```

5. **Configure promtail/alloy to scrape `game-server` namespace** into
   Loki. The fact that Loki only has 2 pod values total means most of
   the cluster's container logs are not being centrally collected.
   Without that, post-hoc forensics on any future incident is impossible
   for any pod outside whatever 2 namespaces are currently being scraped.

6. **`sv_rcon_whitelist_address`** (CS2 cvar): scope RCON to specific
   source IPs. As an additional belt to the suspenders, even if RCON
   TCP somehow gets re-exposed.

## Open question for the user

If you can recover the **full spam text** from your in-game console
scrollback (`con_logfile` may have it cached locally, or the developer
console might still have it before you closed the game), please paste
it. The affiliate / referral code embedded in the URL would let you
file an abuse report with the gambling site against that affiliate
account, which is the only path to actually punishing the attacker
beyond locking them out of our server.
