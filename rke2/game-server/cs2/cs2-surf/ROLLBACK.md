# CS2 Surf Server Rollback Procedure

Step-by-step commands to revert from the cs2-surf-server back to the original
kus cs2-modded-server deployment.

## Prerequisites

- `kubectl` access to the `game-server` namespace
- The kus `cs2-modded-server` deployment still exists (do NOT delete it)

## Rollback Steps

### 1. Scale surf deployment to zero

```bash
kubectl scale deployment/cs2-surf-server -n game-server --replicas=0
```

Wait for the pod to terminate:

```bash
kubectl get pods -n game-server -l app=cs2-surf-server -w
```

### 2. Revert service selector to kus

Apply the original service definition that targets `app: cs2-modded-server`:

```bash
kubectl patch service cs2-modded-service -n game-server \
  -p '{"spec":{"selector":{"app":"cs2-modded-server"}}}'
```

### 3. Scale kus deployment back up

```bash
kubectl scale deployment/cs2-modded-server -n game-server --replicas=1
```

### 4. Verify

Confirm the kus pod is running and the service routes to it:

```bash
# Pod should be Running
kubectl get pods -n game-server -l app=cs2-modded-server

# Service endpoints should list the kus pod IP
kubectl get endpoints cs2-modded-service -n game-server

# Server should respond on game port
kubectl exec -n game-server deploy/cs2-modded-server -- \
  sh -c 'ss -tlnp | grep 27015' 2>/dev/null || echo "Check pod logs if port not yet listening"
```

### 5. Confirm players can connect

Join the server at `10.44.0.32:27015` from a game client to confirm connectivity.

## Notes

- The PVC `cs2-modded-claim` is shared. Both deployments can mount it, but
  only one should be running at a time to avoid file contention.
- If the kus deployment was deleted, re-apply it from
  `rke2/game-server/cs2/cs2-server/deployment.yaml`.
