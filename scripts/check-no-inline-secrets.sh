#!/usr/bin/env bash
# Guard: fail if any tracked YAML contains an inline Kubernetes `kind: Secret`
# with a populated data:/stringData: block.
#
# Policy (docs/SECURITY.md): secrets in this repo must be either
#   - kind: ExternalSecret  (ESO → Bitwarden), or
#   - a gitignored dotfile   (never committed).
# A plaintext `kind: Secret` is base64 (encoding, not encryption) and is a leak
# on a public repo. This is the single source of truth used by BOTH the
# pre-commit hook and the secret-scan CI workflow.
#
# Usage:
#   scripts/check-no-inline-secrets.sh [file ...]   # given files (pre-commit)
#   scripts/check-no-inline-secrets.sh              # all tracked YAML (CI)
set -euo pipefail

files=("$@")
if [ ${#files[@]} -eq 0 ]; then
  while IFS= read -r line; do files+=("$line"); done < <(git ls-files '*.yaml' '*.yml')
fi

violations=0
for f in "${files[@]}"; do
  [ -f "$f" ] || continue
  hit=$(awk '
    /^---[[:space:]]*$/            { kind=""; inblock=0; next }   # doc boundary
    /^kind:[[:space:]]*Secret[[:space:]]*$/ { kind="Secret"; next }
    /^(data|stringData):[[:space:]]*$/ { if (kind=="Secret") inblock=1; next }
    /^[^[:space:]#]/               { inblock=0 }                  # new top-level key
    inblock && /^[[:space:]]+[A-Za-z0-9._-]+:[[:space:]]*[^[:space:]#]/ {
      print NR": "$0; exit
    }
  ' "$f" || true)
  if [ -n "$hit" ]; then
    echo "❌ $f"
    echo "     inline kind:Secret with populated data →${hit}"
    violations=$((violations + 1))
  fi
done

if [ "$violations" -gt 0 ]; then
  echo
  echo "Blocked ${violations} inline Kubernetes Secret(s)."
  echo "Convert to 'kind: ExternalSecret' (ESO/Bitwarden) or move the value to a"
  echo "gitignored dotfile. See docs/SECURITY.md."
  exit 1
fi
echo "✓ no inline Kubernetes Secrets with populated data"
