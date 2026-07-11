# Secrets policy

This is a **public** repository. Every secret here must be one of:

1. **`kind: ExternalSecret`** — a reference resolved at runtime by External Secrets
   Operator from Bitwarden/Vaultwarden. No secret value is ever in git.
2. **A gitignored dotfile** — `.env`, `*.local.yaml`, `*.pem`, `kubeconfig`, etc.
   (see `.gitignore`). Real values live only on disk / in the cluster.

A plaintext `kind: Secret` with a `data:` / `stringData:` block is **forbidden**.
base64 is encoding, not encryption — a committed `Secret` is a public credential.

## Enforcement (three gates)

| Gate | Where | What it blocks |
|------|-------|----------------|
| `scripts/check-no-inline-secrets.sh` | pre-commit + CI | any `kind: Secret` with populated data |
| gitleaks (`.gitleaks.toml`) | pre-commit + CI | high-entropy secrets, keys, tokens in tree + history |
| `.gitignore` dotfile patterns | local | stops real-value files being staged |

Enable the local hooks once:

```sh
pipx install pre-commit
pre-commit install
```

CI runs both scanners on every push and PR (`.github/workflows/secret-scan.yml`)
on GitHub-hosted runners — never the in-cluster `arc-chem` runners.

## Adding a secret (the ExternalSecret pattern)

1. Store the value in Bitwarden. For multi-field secrets use **custom fields** and
   the `bitwarden-fields` ClusterSecretStore; for a simple login use
   `bitwarden-login` with `username` / `password`.
2. Write a `kind: ExternalSecret` referencing the item UUID. Examples in-repo:
   - multi-field: `rke2/longhorn-system/b2-back-secret.yaml`
   - login: `rke2/authentik/github-oauth-secret.yaml`
   - rendered file (e.g. a datasource): `rke2/monitor/spotify-postgres/grafana-datasource.yaml`

## If a secret is leaked

1. **Rotate first** — assume any value that reached this public repo is
   compromised and already scraped. Rotate at the source (Garage key, Postgres
   role, etc.). Rotation is the real fix; git cleanup is secondary.
2. Convert the manifest to `ExternalSecret`.
3. Purge from history (`git filter-repo`), force-push, and ask GitHub Support to
   evict cached blob views.
