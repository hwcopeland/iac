# Terra++ 1:1-scale Earth Minecraft server + mc-router

A Forge 1.12.2 Minecraft server that generates the real Earth at 1:1 scale, plus
the `mc-router` that fronts all homelab Minecraft servers behind a single
WAN-exposed port.

---

## 1. Mod versions (exact, pinned, mutually compatible)

The 1:1 Earth generator is **three tightly version-coupled mods**. They must be
built from matching source revisions or the world generator throws on load.

| Mod | Version | CI build | Immutable download URL |
|-----|---------|----------|------------------------|
| **CubicChunks** | `1.12.2-0.0.1301.0` | OpenCubicChunks Jenkins `MC_1.12` #80 | `https://jenkins.daporkchop.net/job/OpenCubicChunks/job/CubicChunks/job/MC_1.12/80/artifact/build/libs/CubicChunks-1.12.2-0.0.1301.0-SNAPSHOT-all.jar` |
| **CubicWorldGen** | `1.12.2-0.0.225.0` | OpenCubicChunks Jenkins `MC_1.12` #55 | `https://jenkins.daporkchop.net/job/OpenCubicChunks/job/CubicWorldGen/job/MC_1.12/55/artifact/build/libs/CubicWorldGen-1.12.2-0.0.225.0-SNAPSHOT-all.jar` |
| **Terraplusplus** | `1.1.0.660-1.12.2` | BuildTheEarth Jenkins `terraplusplus/master` #147 | `https://jenkins.daporkchop.net/job/BuildTheEarth/job/terraplusplus/job/master/147/artifact/build/libs/terraplusplus-1.1.0.660-1.12.2.jar` |
| **Minecraft Forge** | `1.12.2-14.23.5.2847` | n/a (installed by itzg entrypoint) | https://files.minecraftforge.net/ |

The `-all` suffix on the CubicChunks/CWG jars is the shadow (fat) jar with
bundled shaded dependencies — the variant a server should load.

### Why these versions are compatible

Terraplusplus is the integrating mod and declares its exact compile-time
dependencies in [`build.gradle`](https://github.com/BuildTheEarth/terraplusplus/blob/master/build.gradle):

```
minecraft { version = "1.12.2-14.23.5.2847" }
deobfProvided "io.github.opencubicchunks:cubicchunks:1.12.2-0.0.1282.0-SNAPSHOT"
provided      "io.github.opencubicchunks:cubicworldgen:1.12.2-0.0.206.0-SNAPSHOT:dev"
```

So the *contract* is: Forge `14.23.5.2847`, CubicChunks `≥ 0.0.1282`, CubicWorldGen
`≥ 0.0.206`. We pin the **current heads** of all three repos' `MC_1.12`/`master`
branches (CC `0.0.1301` ≥ 1282, CWG `0.0.225` ≥ 206, T++ `#147`). These are the
exact artifacts T++ master is built and tested against on the same Jenkins, so
they are a known-good set. The mods' public, stable distribution channels are:
- Terraplusplus stable: https://www.curseforge.com/minecraft/mc-mods/terraplusplus/files
- Per-commit CI (used here for deterministic pinning): `jenkins.daporkchop.net`

> The GitHub *Releases* pages are stale (T++ last release v1.1.0 in 2021; the CC/CWG
> GitHub releases only carry 1.17 dev builds), which is why the canonical 1.12.2
> jars come from Jenkins, not GitHub releases.

### Runtime data dependency

Terra++ fetches OpenStreetMap + elevation + treecover tiles **at runtime** from
DaPorkchop-hosted services as players explore. **The pod must have outbound
internet egress** (no default-deny NetworkPolicy blocking egress in `game-server`,
which is the current state). First-time generation of a region is slow and
network-bound.

---

## 2. Files in this change

```
rke2/game-server/mc/earth/
  Dockerfile               # FROM itzg/minecraft-server:java8-multiarch, bakes the 3 mods
  earth-deployment.yaml    # Deployment + 200Gi Longhorn PVC + ClusterIP svc "mc-earth"
  zot-pull-secret.yaml     # ExternalSecret adopting game-server/zot-pull-secret
  README.md                # this file
rke2/game-server/mc/mc-router/
  mc-router.yaml           # Deployment + LoadBalancer (10.44.0.40) on tcp/25565
  island-internal-svc.yaml # ClusterIP "atm9skies-island" for the skyblocker server
.github/workflows/
  build-mc-earth.yml        # arc-chem build → zot (OCI mediatypes, no registry cache)
```

---

## 3. Image build approach

- `FROM itzg/minecraft-server:java8-multiarch` (Forge 1.12.2 requires Java 8).
- The three mod jars are **baked into `/mods`** at build time (downloaded from the
  pinned Jenkins URLs above). itzg's entrypoint copies `/mods` →
  the server mods dir on every boot (`COPY_MODS_SRC` defaults to `/mods`,
  verified against the image's `scripts/start-setupMounts`), so a fresh PVC
  self-populates with no runtime mod fetch.
- `EULA=TRUE`, `TYPE=FORGE`, `VERSION=1.12.2`, `FORGE_VERSION=14.23.5.2847` set
  as image env.
- Built on the **arc-chem** self-hosted runner and pushed to
  `zot.hwcopeland.net/game-server/mc-earth` with
  `outputs: type=image,oci-mediatypes=true,push=true` (zot rejects Docker
  schema-2 with HTTP 415). **No `cache-to: type=registry`** (inline cache export
  reverts buildkit to Docker media types → also 415s).
- Tags: `sha-<7char>` (immutable) + `latest`.
- The workflow has **no deploy job** on purpose — see cutover plan below.

---

## 4. mc-router mapping table

Single LoadBalancer: **`10.44.0.40`** : `tcp/25565` (Cilium `default-ipv4-ippool`,
verified unused). Routing is by the hostname the client connects with:

| Client hostname | Backend (cluster DNS) | Server |
|-----------------|------------------------|--------|
| `earth.mc.hwcopeland.net` | `mc-earth.game-server.svc.cluster.local:25565` | Terra++ Earth |
| `sky.mc.hwcopeland.net`   | `atm9skies-island.game-server.svc.cluster.local:25565` | **island = the "skyblocker" server** (live LB 10.44.0.34) |
| `atm9.mc.hwcopeland.net`  | `atm9skies-external.game-server.svc.cluster.local:25565` | ATM9 Skies (original) |
| _default / no SNI_        | `atm9skies-external...:25565` | fallback (`DEFAULT` env) |

> **The "skyblocker server" the user referenced = `atm9skies-island`** (deployment
> `atm9skies-island`, LB `10.44.0.34`). `sky.mc.hwcopeland.net` maps there.

---

## 5. Human steps remaining (DO NOT auto-applied — review required)

Nothing in this change has been applied to the live cluster. Wiring mc-router
changes tcp/25565 routing and could disrupt the already-running
atm9skies/island/satisfactory players, so all live applies are manual.

### 5a. Apply order (in `game-server`)

```bash
# 1. Build the image first (push to main triggers build-mc-earth.yml, or run
#    the workflow_dispatch). Confirm the tag exists in zot:
#      zot.hwcopeland.net/game-server/mc-earth:sha-XXXXXXX

# 2. (Optional GitOps cleanup) adopt the existing zot-pull-secret declaratively.
#    This changes ownership of the live game-server/zot-pull-secret to ESO.
#    Verify it re-syncs identical creds before relying on it.
kubectl apply -f rke2/game-server/mc/earth/zot-pull-secret.yaml
kubectl -n game-server get externalsecret zot-pull-secret -w   # wait for SecretSynced

# 3. Internal ClusterIP for the island server (additive; does not touch the LB).
kubectl apply -f rke2/game-server/mc/mc-router/island-internal-svc.yaml

# 4. Earth server. Pin the image to the immutable sha (NOT :latest).
kubectl apply -f rke2/game-server/mc/earth/earth-deployment.yaml
kubectl -n game-server set image deployment/mc-earth \
  mc-earth-server=zot.hwcopeland.net/game-server/mc-earth:sha-XXXXXXX
kubectl -n game-server rollout status deployment/mc-earth --timeout=600s
# First boot installs Forge + mods; can take several minutes.

# 5. mc-router LAST. Claims 10.44.0.40 only; does not disturb existing LBs.
kubectl apply -f rke2/game-server/mc/mc-router/mc-router.yaml
kubectl -n game-server rollout status deployment/mc-router --timeout=120s
kubectl -n game-server get svc mc-router   # confirm EXTERNAL-IP == 10.44.0.40
```

### 5b. MikroTik dst-nat rule (CCR-2004 @ 10.0.0.1 — operator runs this)

Forward WAN tcp/25565 → the mc-router LoadBalancer `10.44.0.40`. Replace
`ether1`/`pppoe-out1` with the actual WAN interface (and adjust `in-interface`
or use an `in-interface-list=WAN`).

```
/ip firewall nat add chain=dstnat action=dst-nat \
  protocol=tcp dst-port=25565 in-interface=ether1 \
  to-addresses=10.44.0.40 to-ports=25565 \
  comment="Minecraft -> mc-router LB"
```

Ensure a corresponding forward-chain accept exists (if the firewall is not
already permissive for forwarded NATed traffic):

```
/ip firewall filter add chain=forward action=accept \
  protocol=tcp dst-port=25565 dst-address=10.44.0.40 \
  comment="Allow Minecraft to mc-router"
```

> Only **one** WAN dst-nat for tcp/25565 is needed now — mc-router multiplexes
> all servers behind it by hostname. Any pre-existing 25565 dst-nat pointing at
> 10.44.0.5 (atm9skies-local) or 10.44.0.34 (island) should be **removed/replaced**
> by this one rule once cutover is verified.

### 5c. DNS records (point each hostname at the WAN IP)

mc-router routes by hostname, so every Minecraft hostname resolves to the **same
WAN IP** (`<WAN_IP>`). Use A records (simplest), or SRV if you want to avoid
exposing the port in the client:

```
; A records — all to the WAN IP
earth.mc.hwcopeland.net.  300  IN  A   <WAN_IP>
sky.mc.hwcopeland.net.    300  IN  A   <WAN_IP>
atm9.mc.hwcopeland.net.   300  IN  A   <WAN_IP>

; Optional SRV (lets clients connect without ":25565")
_minecraft._tcp.earth.mc.hwcopeland.net. 300 IN SRV 0 5 25565 earth.mc.hwcopeland.net.
_minecraft._tcp.sky.mc.hwcopeland.net.   300 IN SRV 0 5 25565 sky.mc.hwcopeland.net.
_minecraft._tcp.atm9.mc.hwcopeland.net.  300 IN SRV 0 5 25565 atm9.mc.hwcopeland.net.
```

### 5d. Safe mc-router cutover plan

1. **Stage without cutover.** Apply everything in 5a. mc-router gets 10.44.0.40
   but the MikroTik still points WAN:25565 at the old direct LB — current players
   are unaffected.
2. **Verify routing internally**, before touching the MikroTik:
   ```bash
   # From a pod or a LAN host that can reach 10.44.0.40:
   # SNI-based routing test — mc-router reads the handshake hostname.
   mcstatus 10.44.0.40 status            # default -> atm9skies
   # Point each DNS name at 10.44.0.40 in /etc/hosts on a test client and connect
   # with the real MC client to earth./sky./atm9. to confirm each lands correctly.
   kubectl -n game-server logs deploy/mc-router   # shows routing decisions
   ```
3. **Cut over the MikroTik** (5b) only after internal routing is confirmed. Add
   the new dst-nat to 10.44.0.40, then remove the old 25565 NAT(s).
4. **Verify externally** from off-LAN: connect to each hostname.
5. **Rollback**: re-point the MikroTik dst-nat back to the previous LB
   (10.44.0.5 / 10.44.0.34) and `kubectl -n game-server delete -f mc-router.yaml`.
   The backend servers themselves are never modified by the router, so rollback
   is just NAT + deleting the router.

---

## 6. Risks

- **25565 is the blast radius.** mc-router becomes the single ingress for *all*
  MC servers. If its pod is down, every server is unreachable from WAN (a single
  replica is used; bump replicas if HA matters — routing is stateless).
- **Annotation auto-discovery deliberately OFF.** `atm9skies-external` carries
  `mc-router.itzg.me/{externalServerName,defaultServer}` annotations. We do NOT
  pass `--in-kube-cluster`, so those are ignored and static `MAPPING` is
  authoritative. If someone later enables `--in-kube-cluster`, those annotations
  will start competing with `MAPPING`/`DEFAULT` — review before doing so.
- **zot-pull-secret ownership change.** `zot-pull-secret.yaml` adopts an existing
  manually-created Secret. If the Bitwarden item creds differ from what's live,
  ESO will overwrite them. Verify `SecretSynced` and a successful image pull
  before deleting any manual copy.
- **Terra++ is RAM- and IO-heavy.** 6Gi req / 12Gi limit, Xmx 10G. First-gen of
  new regions is network-bound on external tile servers; if egress is ever
  restricted by a NetworkPolicy, world-gen will stall/fail.
- **Longhorn 200Gi.** Terra++ worlds grow with exploration; 200Gi is generous but
  monitor actual usage. Longhorn replicates — confirm the cluster has the backing
  capacity (200Gi × replica count).
- **Two pods on one RWO volume = corruption.** The Deployment uses
  `strategy: Recreate` to guarantee the old pod is gone before a new one mounts
  the world. Do not switch to RollingUpdate.
