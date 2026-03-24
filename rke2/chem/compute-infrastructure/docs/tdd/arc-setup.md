---
project: "compute-infrastructure"
maturity: "draft"
last_updated: "2026-03-23"
updated_by: "@devops-engineer"
scope: "ARC v2 installation runbook and verification guide for the arc-chem runner scale-set"
owner: "@devops-engineer"
dependencies:
  - docs/tdd/github-actions-ci.md
---

# Operator Runbook: ARC v2 Installation and chem Runner Scale-Set

This runbook installs GitHub Actions Runner Controller (ARC) v2 onto the RKE2 cluster and
registers a runner scale-set that enables `.github/workflows/build-and-push.yml` to run on
cluster-hosted runners with direct network access to `zot.hwcopeland.net`.

**Why ARC on cluster:** The private Zot registry at `zot.hwcopeland.net` is not reachable
from GitHub cloud-hosted runners. Runner pods inside the cluster LAN solve this without
firewall changes or tunnel management. See `docs/tdd/github-actions-ci.md` Section 3 for
the full alternatives analysis.

**ARC v2 components:**
- `actions-runner-controller` — the Kubernetes controller that manages runner pod lifecycle
- `gha-runner-scale-set` (`chem-runners`) — an AutoScalingRunnerSet that defines runner
  pods registered to `Khemiatic-Energistics/compute-infrastructure`

---

## Prerequisites

| Prerequisite | How to verify |
|---|---|
| `kubectl` configured for the RKE2 cluster | `kubectl get nodes` |
| `helm` v3.12+ installed | `helm version` |
| `bw` (Bitwarden CLI) installed and unlocked | `bw status` |
| GitHub organization admin access (to create the GitHub App) | GitHub Settings available |
| A Bitwarden collection to store the GitHub App credentials | (use the same collection as other cluster secrets) |
| External Secrets Operator installed on the cluster | `kubectl get clusteroperator bitwarden-login` or `kubectl get clustersecretstore` |

---

## Step 1: Create the arc-system Namespace

```bash
kubectl create namespace arc-system
```

Label the namespace to enable ExternalSecrets to write into it:

```bash
kubectl label namespace arc-system \
  app.kubernetes.io/managed-by=flux \
  kubernetes.io/metadata.name=arc-system
```

---

## Step 2: Create the GitHub App for Runner Registration

ARC v2 uses a GitHub App (not a PAT) for runner registration. GitHub deprecated PAT-based
ARC registration in the scale-set controller.

### 2.1 Create the GitHub App

1. Go to: `https://github.com/organizations/Khemiatic-Energistics/settings/apps/new`
   (or your personal account if this repo is personal: `https://github.com/settings/apps/new`)

2. Fill in:
   - **GitHub App name:** `arc-chem-runner` (must be unique across GitHub)
   - **Homepage URL:** `https://github.com/Khemiatic-Energistics/compute-infrastructure`
   - **Webhooks:** Uncheck "Active" — ARC v2 uses polling, not webhooks

3. Set **Repository permissions:**
   - `Actions`: Read and write
   - `Administration`: Read and write
   - `Metadata`: Read-only (required by GitHub, cannot be disabled)

4. Set **Where can this GitHub App be installed?**: Only on this account

5. Click **Create GitHub App**

6. Note the **App ID** (displayed at the top of the app settings page after creation)

### 2.2 Generate a Private Key

1. On the app settings page, scroll to "Private keys"
2. Click **Generate a private key**
3. Save the downloaded `.pem` file — you will store it in Bitwarden in the next step

### 2.3 Install the App on the Repository

1. On the app settings page, click **Install App** (left sidebar)
2. Click **Install** next to the Khemiatic-Energistics organization (or your account)
3. Select **Only select repositories** and choose `compute-infrastructure`
4. Click **Install**
5. Note the **Installation ID** from the URL after install:
   `https://github.com/organizations/Khemiatic-Energistics/settings/installations/<INSTALLATION_ID>`

---

## Step 3: Store GitHub App Credentials in Bitwarden

Create a new Secure Note in Bitwarden named `arc-github-app-chem`:

```
Item name: arc-github-app-chem
App ID: <the numeric App ID from Step 2.1>
Installation ID: <the numeric Installation ID from Step 2.3>
Private key: <full contents of the .pem file from Step 2.2>
```

Store the private key as a custom field named `private_key`. It is multi-line; paste the
full PEM including `-----BEGIN RSA PRIVATE KEY-----` and `-----END RSA PRIVATE KEY-----`.

Record the Bitwarden item UUID after saving:
```bash
bw get item arc-github-app-chem --raw | jq -r '.id'
```

This UUID is used in the ExternalSecret below.

---

## Step 4: Create ExternalSecret for GitHub App Credentials

The ExternalSecret materializes the GitHub App credentials as a Kubernetes Secret in
`arc-system`. ARC reads this secret to authenticate to the GitHub API.

Replace `<BITWARDEN-ITEM-UUID>` with the UUID from Step 3.

```bash
cat <<'EOF' | kubectl apply -f -
---
# GitHub App credentials for ARC runner registration in arc-system.
# Bitwarden item: arc-github-app-chem
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: arc-github-app-secret
  namespace: arc-system
spec:
  refreshInterval: "1h"
  secretStoreRef:
    kind: ClusterSecretStore
    name: bitwarden-login
  target:
    name: arc-github-app-secret
    creationPolicy: Owner
  data:
    - secretKey: github_app_id
      remoteRef:
        key: <BITWARDEN-ITEM-UUID>
        property: github_app_id
    - secretKey: github_app_installation_id
      remoteRef:
        key: <BITWARDEN-ITEM-UUID>
        property: github_app_installation_id
    - secretKey: github_app_private_key
      remoteRef:
        key: <BITWARDEN-ITEM-UUID>
        property: private_key
EOF
```

Verify the ExternalSecret synced successfully:

```bash
kubectl get externalsecret arc-github-app-secret -n arc-system
# Expected STATUS: SecretSynced
kubectl get secret arc-github-app-secret -n arc-system
# Expected: exists with type=Opaque, 3 data keys
```

**Note:** The field names in the Kubernetes Secret (`github_app_id`,
`github_app_installation_id`, `github_app_private_key`) must match what ARC expects in its
`githubConfigSecret`. Verify against current ARC chart docs if this fails.

---

## Step 5: Add GitHub Repository Secrets for CI

The workflow uses `ZOT_USERNAME` and `ZOT_PASSWORD` to authenticate to the Zot registry
for image pushes. These are read from Bitwarden item `766ec5c7-6aa8-419d-bb27-e5982872bc5b`
(the same credentials used for pull secrets in the cluster).

```bash
# Read credentials from Bitwarden (requires bw session)
ZOT_USER=$(bw get item 766ec5c7-6aa8-419d-bb27-e5982872bc5b --raw | jq -r '.login.username')
ZOT_PASS=$(bw get item 766ec5c7-6aa8-419d-bb27-e5982872bc5b --raw | jq -r '.login.password')

# Set repository secrets using GitHub CLI
gh secret set ZOT_USERNAME --body "$ZOT_USER" \
  --repo Khemiatic-Energistics/compute-infrastructure
gh secret set ZOT_PASSWORD --body "$ZOT_PASS" \
  --repo Khemiatic-Energistics/compute-infrastructure
```

Verify:
```bash
gh secret list --repo Khemiatic-Energistics/compute-infrastructure
# Expected: ZOT_USERNAME and ZOT_PASSWORD appear in the list
```

**Rotation procedure:** When Zot credentials in Bitwarden are rotated, repeat the `gh secret set`
commands above. The ExternalSecret in the cluster (`chem/zot-pull-secret`) rotates
automatically via its 1h refresh interval.

---

## Step 6: Install ARC Controller via Helm

The Helm values file lives at `rke2/arc-system/arc-controller-values.yaml` in the `iac` repo.

```bash
helm install arc-controller \
  oci://ghcr.io/actions/actions-runner-controller-charts/gha-runner-scale-set-controller \
  --version 0.10.1 \
  --namespace arc-system \
  --create-namespace \
  --values /Users/hwcopeland/iac/rke2/arc-system/arc-controller-values.yaml \
  --wait
```

Verify the controller is running:

```bash
kubectl get pods -n arc-system -l app.kubernetes.io/name=gha-runner-scale-set-controller
# Expected: 1/1 Running
kubectl logs -n arc-system -l app.kubernetes.io/name=gha-runner-scale-set-controller --tail=20
# Expected: log lines showing "Starting manager", no errors
```

---

## Step 7: Install the chem Runner Scale-Set via Helm

The Helm values file lives at `rke2/arc-system/arc-runners-chem-values.yaml`.

```bash
helm install arc-runners-chem \
  oci://ghcr.io/actions/actions-runner-controller-charts/gha-runner-scale-set \
  --version 0.10.1 \
  --namespace arc-system \
  --values /Users/hwcopeland/iac/rke2/arc-system/arc-runners-chem-values.yaml \
  --wait
```

The scale-set name `arc-chem` (set in `runnerScaleSetName`) becomes the runner label
that the workflow uses via `runs-on: [self-hosted, arc-chem]`.

---

## Step 8: Verify Runner Registration

### 8.1 Check AutoScalingRunnerSet status

```bash
kubectl get autoscalingrunnerset -n arc-system
# Expected: arc-chem shows Ready
```

### 8.2 Check GitHub UI

Navigate to:
`https://github.com/organizations/Khemiatic-Energistics/settings/actions/runners`

Or for a personal repo:
`https://github.com/Khemiatic-Energistics/compute-infrastructure/settings/actions/runners`

You should see the `arc-chem` runner scale-set listed. The count shows 0 runners when idle
(minRunners=0), which is expected — runners scale up on demand.

### 8.3 Trigger a test workflow run

```bash
gh workflow run build-and-push.yml \
  --repo Khemiatic-Energistics/compute-infrastructure \
  --ref main
```

Monitor the run:
```bash
gh run list --repo Khemiatic-Energistics/compute-infrastructure --limit 5
gh run view <run-id> --repo Khemiatic-Energistics/compute-infrastructure
```

Watch runner pods spin up during the run:
```bash
kubectl get pods -n arc-system -w
# Expected: 1-4 runner pods appear with status Running during the workflow job(s),
# then terminate after completion
```

### 8.4 Verify images pushed to Zot

After a successful run, verify all four images landed in the registry. The tag format is
`sha-<7-char-sha>` matching the commit SHA of the triggering push.

Check by browsing the Zot UI at `https://zot.hwcopeland.net` or via the Zot API:

```bash
# Check docking-controller tags
curl -s -u "$ZOT_USER:$ZOT_PASS" \
  https://zot.hwcopeland.net/v2/chem/docking-controller/tags/list | jq .

# Check autodock4 tags
curl -s -u "$ZOT_USER:$ZOT_PASS" \
  https://zot.hwcopeland.net/v2/chem/autodock4/tags/list | jq .

# Check autodock-vina tags
curl -s -u "$ZOT_USER:$ZOT_PASS" \
  https://zot.hwcopeland.net/v2/chem/autodock-vina/tags/list | jq .

# Check jupyter-chem tags
curl -s -u "$ZOT_USER:$ZOT_PASS" \
  https://zot.hwcopeland.net/v2/chem/jupyter-chem/tags/list | jq .
```

Expected output (example SHA `a1b2c3d`):
```json
{"name": "chem/docking-controller", "tags": ["sha-a1b2c3d", "latest"]}
```

---

## Troubleshooting

### Runner pods fail to start / "no runners available"

```bash
# Check ARC controller logs
kubectl logs -n arc-system -l app.kubernetes.io/name=gha-runner-scale-set-controller --tail=50

# Check AutoScalingRunnerSet events
kubectl describe autoscalingrunnerset arc-chem -n arc-system

# Check EphemeralRunnerSet objects
kubectl get ephemeralrunnerset -n arc-system
```

Common causes:
- **GitHub App secret not synced:** `kubectl get externalsecret arc-github-app-secret -n arc-system` — check STATUS
- **Wrong Installation ID or App ID:** Re-read from GitHub App settings and update the Bitwarden item
- **Private key format:** The PEM file must include the header/footer lines. Verify with:
  `kubectl get secret arc-github-app-secret -n arc-system -o jsonpath='{.data.github_app_private_key}' | base64 -d | head -3`

### Docker socket not accessible in runner pod

If the build step fails with "Cannot connect to the Docker daemon at /var/run/docker.sock":

```bash
# SSH to the node running the runner pod
# Verify the Docker socket exists
stat /var/run/docker.sock

# If using containerd-only (no Docker daemon), switch to DinD sidecar approach.
# Update arc-runners-chem-values.yaml to add a DinD container and remove the hostPath volume.
```

### Network egress failures (autodock/jupyter builds)

The autodock4 and autodock-vina Dockerfiles download binaries from:
- `autodock.scripps.edu` (autodock4 binary)
- `vina.scripps.edu` (vina binary)

The jupyter Dockerfile downloads Miniconda from `repo.anaconda.com`.

If Cilium NetworkPolicy blocks egress from `arc-system`, create a policy allowing egress:

```bash
cat <<'EOF' | kubectl apply -f -
---
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata:
  name: arc-runner-egress
  namespace: arc-system
spec:
  endpointSelector:
    matchLabels:
      # Applied to all runner pods in arc-system
      actions.github.com/scale-set-name: arc-chem
  egress:
    # Allow all egress for runner pods (they need internet access for builds)
    # Tighten if security posture requires: add specific CIDR/port rules
    - toEntities:
        - world
EOF
```

### Helm release upgrade

When upgrading ARC chart version:
```bash
# Update image.tag in arc-controller-values.yaml first, then:
helm upgrade arc-controller \
  oci://ghcr.io/actions/actions-runner-controller-charts/gha-runner-scale-set-controller \
  --version <new-version> \
  --namespace arc-system \
  --values /Users/hwcopeland/iac/rke2/arc-system/arc-controller-values.yaml

helm upgrade arc-runners-chem \
  oci://ghcr.io/actions/actions-runner-controller-charts/gha-runner-scale-set \
  --version <new-version> \
  --namespace arc-system \
  --values /Users/hwcopeland/iac/rke2/arc-system/arc-runners-chem-values.yaml
```

### Uninstall ARC

If you need to remove ARC without affecting cluster workloads:

```bash
helm uninstall arc-runners-chem -n arc-system
helm uninstall arc-controller -n arc-system
# Namespace can be left in place (contains ExternalSecret)
# Or delete if fully decommissioning:
# kubectl delete namespace arc-system
```

---

## ExternalSecret Reference Pattern

The ExternalSecret for ARC credentials follows the same Bitwarden pattern used throughout
this cluster. For comparison, the pattern from `k8s-jobs/zot-pull-secret.yaml`:

```yaml
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: zot-pull-secret
  namespace: chem
spec:
  refreshInterval: "1h"
  secretStoreRef:
    kind: ClusterSecretStore
    name: bitwarden-login
  target:
    name: zot-pull-secret
    creationPolicy: Owner
    template:
      type: kubernetes.io/dockerconfigjson
      ...
  data:
    - secretKey: username
      remoteRef:
        key: 766ec5c7-6aa8-419d-bb27-e5982872bc5b
        property: username
```

The ARC ExternalSecret follows the same structure but creates an Opaque secret (no
`template.type`) with three keys: `github_app_id`, `github_app_installation_id`, and
`github_app_private_key`.

---

## Post-Installation State Summary

After completing all steps, the following resources exist in the cluster:

| Resource | Namespace | Purpose |
|---|---|---|
| `Namespace/arc-system` | — | Isolation boundary for ARC workloads |
| `ExternalSecret/arc-github-app-secret` | arc-system | Syncs GitHub App credentials from Bitwarden |
| `Secret/arc-github-app-secret` | arc-system | GitHub App ID, installation ID, private key |
| `HelmRelease` (manual) `arc-controller` | arc-system | ARC controller manager pod |
| `HelmRelease` (manual) `arc-runners-chem` | arc-system | AutoScalingRunnerSet for arc-chem |
| `AutoScalingRunnerSet/arc-chem` | arc-system | Manages ephemeral runner pod lifecycle |

GitHub state:
| Resource | Where |
|---|---|
| GitHub App `arc-chem-runner` | Khemiatic-Energistics org |
| App installation on `compute-infrastructure` | GitHub |
| Repository secrets `ZOT_USERNAME`, `ZOT_PASSWORD` | `Khemiatic-Energistics/compute-infrastructure` |
| Runner scale-set `arc-chem` registered | GitHub Actions settings |
