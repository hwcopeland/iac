# ADR-001: Device-Code (RFC 8628) flow is brand-global — gate new public clients

- **Status:** Accepted
- **Date:** 2026-07-12
- **Context issue:** [Florida-Man-Bioscience/peptodyssey#1](https://github.com/Florida-Man-Bioscience/peptodyssey/issues/1)
- **Applies to:** the `authentik-default` brand at `auth.hwcopeland.net`

## Context

The peptodyssey iOS app needs the OAuth 2.0 Device Authorization Grant (RFC 8628).
In Authentik 2026.2.1 there is **no per-provider device-code toggle**: device flow is
enabled by setting `flow_device_code` on the **Brand**, and that switch is
**brand-global** — it turns on `/application/o/device/` for *every* OAuth2 provider on
the brand, not just the one that asked for it.

The brand hosts other **secretless public clients** (`homeassistant`, `khemeia`). With
device flow naively enabled brand-wide, those become device-code **phishing** targets: an
attacker starts a device grant with a known/guessable `client_id`, socially-engineers a
logged-in user into entering the `user_code`, and — because a public client needs no
secret at the token poll — receives live tokens. `homeassistant` governs physical-home
control, so this is not hypothetical blast radius. This was caught by a 2-round consensus
review before apply.

## Decision

Enable `flow_device_code` brand-wide (unavoidable) **but gate completion** so only the
intended provider can finish a device grant:

1. The brand `flow_device_code` points at `default-authentication-flow` (login + code
   entry only — **no policy is bound here**, so interactive SSO is untouched).
2. Gating happens at **Phase B** (`CodeValidatorView`), which plans the *matched
   provider's own* `authorization_flow` with a device token in context:
   - Each intended device-flow provider (e.g. `peptodyssey`) gets a **dedicated
     authorization flow** — allowed.
   - Every other provider shares `default-provider-authorization-implicit-consent`, onto
     which an expression policy `deny-device-code-on-shared-authorization` is bound:
     `return "goauthentik.io/providers/oauth2/device" not in request.context`.
     Browser SSO has no device token in context → returns `True` (allow); a device grant
     → returns `False` (deny). The binding is `failure_result: true` (fail-open) because
     it sits on every app's browser-SSO path and the dict-membership expression cannot
     raise — fail-open protects homelab-wide SSO availability without enabling a device
     bypass.

See `blueprints/providers-peptodyssey.yaml` (mirrored into `blueprints-configmap.yaml`)
for the implementing entries.

## The invariant (READ THIS before adding an OAuth2 provider)

> **Any new secretless / `client_type: public` OAuth2 provider is device-code-phishable
> the moment it exists, because device flow is on brand-wide.**

Therefore, when you add a public client:

- **If it should NOT use device flow** (the common case): leave its `authorization_flow`
  as `default-provider-authorization-implicit-consent`. It inherits the deny binding and
  is protected automatically. **Do not** move it to a custom or explicit-consent flow
  without re-reading this ADR — `default-provider-authorization-explicit-consent` is
  **not** covered by the deny binding today (safe only for confidential clients whose
  token poll requires a secret).
- **If it SHOULD use device flow** (like peptodyssey): give it its **own dedicated
  authorization flow** (so it's exempt from the deny), and consider an explicit-consent
  stage given device grants + long-lived `offline_access` refresh tokens.

## Consequences

- **Positive:** device flow works for peptodyssey; all other current providers
  (`homeassistant`, `khemeia`, grafana, vg, jupyterhub, …) are protected; interactive SSO
  is provably unaffected.
- **Residual (accepted):** the gate depends on Authentik-internal behavior (the
  `goauthentik.io/providers/oauth2/device` context key) that can only be verified live,
  not statically — **re-run the device-deny test after any Authentik upgrade** (initiate a
  device grant for `homeassistant`; it must be denied at completion). The fail-open
  binding and the "future public client on a non-covered flow" footgun above are the known
  gaps this ADR exists to manage.

## Rollback

Authentik blueprints do **not** revert live fields when an entry is removed (see
`blueprints/metabase-removal.yaml`). To turn device flow back off you must **re-apply** an
explicit `{flow_device_code: null}` on the brand, and disable the deny binding via
`enabled: false` (do **not** use `state: absent` on the PolicyBinding — the 2026.2.x
importer crashes on it).

## Deployment note

The `authentik` namespace is reconciled by **Flux** (Kustomization `tooling/authentik`,
path `./rke2/authentik`, `prune: true`, 5m). Changes land **only via `main`** — a manual
`kubectl apply` here is reverted on the next reconcile. Merge to `main` and let Flux apply.
