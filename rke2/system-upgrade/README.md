# `system-upgrade` namespace — RKE2 in-cluster upgrades

Rancher [system-upgrade-controller](https://github.com/rancher/system-upgrade-controller)
(SUC) **v0.14.2** plus the two RKE2 upgrade Plans. Installed manually on the live
cluster (no prior source in this repo); codified here from the running state.

**Important:** RKE2 nodes are normally provisioned and version-pinned via
`ansible/` (`rke2_version` in `inventory/group_vars/all.yaml`). SUC is the
*in-cluster, opt-in* path for rolling node upgrades. Keep the Ansible var in
sync with the Plan `version` so a re-provision does not drift.

## Files

| File | Captures |
|---|---|
| `namespace.yaml` | The `system-upgrade` namespace |
| `rbac.yaml` | ServiceAccount `system-upgrade`, ClusterRoles `system-upgrade-controller` (+`-drainer`), their ClusterRoleBindings, and the namespaced Role/RoleBinding |
| `configmap.yaml` | `default-controller-env` — controller tunables (threads, job TTL, polling interval, kubectl image) |
| `deployment.yaml` | The `system-upgrade-controller` Deployment (v0.14.2), control-plane-only via nodeAffinity |
| `plans.yaml` | The `rke2-server` and `rke2-agent` Plans |

## Deploy method (manual `kubectl apply`)

Nothing here is Flux-managed (Flux depends on the foundation layer). Apply in order:

```bash
kubectl apply -f namespace.yaml
kubectl apply -f rbac.yaml
kubectl apply -f configmap.yaml
kubectl apply -f deployment.yaml
kubectl apply -f plans.yaml
```

The Plan CRD (`plans.upgrade.cattle.io`) is created by the controller's
ClusterRole grant on first run; if applying Plans fails with "no matches for
kind Plan", wait for the controller to install the CRD, then re-apply `plans.yaml`.

## What the Plans target

- **Current target version:** `v1.35.5+rke2r2` (both Plans — they MUST match).
- `rke2-server` → control-plane nodes (`node-role.kubernetes.io/control-plane In [true]`).
- `rke2-agent` → worker nodes (control-plane `NotIn [true]`), with a `prepare`
  step (`prepare rke2-server`) that blocks agent upgrades until the server Plan
  completes.
- `concurrency: 1`, `cordon: true` — one node at a time, drained before upgrade.

## Node opt-in label

A node is only upgraded when it carries:

```bash
kubectl label node <node> rke2-upgrade=true
```

Remove the label after the upgrade completes (`kubectl label node <node> rke2-upgrade-`).

## Upgrade runbook (high blast radius — snapshot first)

1. `rke2 etcd-snapshot save` on a control-plane node.
2. Bump the `version:` field in **both** Plans in `plans.yaml` and apply.
3. Label the target nodes `rke2-upgrade=true`.
4. Watch the server Plan complete (`kubectl get plan -n system-upgrade`,
   `kubectl get jobs -n system-upgrade`), then agents.
5. Unlabel nodes; bump `rke2_version` in Ansible to match.

**nixos-gpu is excluded from SUC and Ansible** — upgrade it separately or it drifts.
