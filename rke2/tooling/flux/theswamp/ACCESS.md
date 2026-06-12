# Working on `theswamp` directly (cluster access)

FMB collaborators (`curtisdearing`, `jonesnoaht`) get **namespace-admin in
`theswamp` and nothing else**. RBAC is in `rbac.yaml` (applied by Flux). Two ways
in — pick one.

## Option A — API key (static ServiceAccount token, no browser) ← simplest

The `swamp-dev` ServiceAccount token is the API key. An admin (you) mints it once
and hands it to the collaborator; it cannot be self-served (that would require
namespace access first — chicken/egg).

**Mint a time-boxed token (preferred — it expires):**
```sh
kubectl -n theswamp create token swamp-dev --duration=2160h   # 90 days
```
**…or read the long-lived (non-expiring) token from the Secret in rbac.yaml:**
```sh
kubectl -n theswamp get secret swamp-dev-token -o jsonpath='{.data.token}' | base64 -d; echo
```

**Build a kubeconfig to hand over:**
```sh
API=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
TOKEN=<paste the token from above>

kubectl config --kubeconfig=swamp.kubeconfig set-cluster swamp \
  --server="$API" --insecure-skip-tls-verify=true
kubectl config --kubeconfig=swamp.kubeconfig set-credentials swamp-dev --token="$TOKEN"
kubectl config --kubeconfig=swamp.kubeconfig set-context swamp \
  --cluster=swamp --user=swamp-dev --namespace=theswamp
kubectl config --kubeconfig=swamp.kubeconfig use-context swamp
```
Then they run `KUBECONFIG=swamp.kubeconfig kubectl get pods` — works **only** in
`theswamp`; any verb in any other namespace is denied.

> For TLS verification instead of `--insecure-skip-tls-verify`, pull the cluster
> CA from the token Secret (`...get secret swamp-dev-token -o
> jsonpath='{.data.ca\.crt}'`), base64-decode it to `ca.crt`, and use
> `--certificate-authority=ca.crt --embed-certs=true`.

## Option B — per-user SSO (OIDC, auditable)

Because `curtisdearing` / `jonesnoaht` are in the Authentik **Florida Man
Bioscience** group, they can authenticate as *themselves* via the cluster's OIDC
flow (the same one the `Kubernetes Users` group uses), landing on the same
`theswamp`-admin scope. Needs the `kubelogin` (`kubectl oidc-login`) plugin and a
kubeconfig pointing at `auth.hwcopeland.net`. Use this if you want per-person
attribution instead of a shared key.

## Scope & caveats
- Both paths bind to the in-namespace `admin` ClusterRole via **RoleBinding** →
  full control inside `theswamp`, **zero** access to any other namespace / nodes
  / cluster resources.
- **Reachability:** the kube-apiserver must be reachable from wherever they run
  `kubectl` (home LAN / VPN). RBAC does not grant network reachability.
- **Rotate** the static key by deleting + recreating the `swamp-dev-token`
  Secret (or just use short `create token` durations and skip the Secret).
