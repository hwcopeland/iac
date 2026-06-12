# Provisioning the `u4u-engine` API Token (GEN-03)

> **Audience:** Khemeia operator (you). This is an **out-of-band, one-time** provisioning
> runbook for the long-lived service-account token the external **u4u-engine** uses to call
> Khemeia's genomics REST surface (`/api/v1/genome/*`).
>
> **The token value is a secret. It is provisioned by hand, stored in Bitwarden, and handed to
> the u4u maintainer through a secure channel. It NEVER lands in git, a manifest, or this file.**

---

## 1. What u4u is, and the trust model

u4u (`Florida-Man-Bioscience/u4u-engine`) is an **external** genomics variant pipeline. It is
**not deployed in the cluster**. It is an ordinary HTTP client that calls Khemeia to obtain
structural/biophysical signal for missense variants and folds the returned scores into its own
prioritization (additively, never overwriting ClinVar — see the core TDD §5.4).

Treat u4u as a **low-trust external client**:

- **Trust boundary is at Khemeia's ingress, not the network.** u4u reaches the controller
  through the ingress, so its requests carry `X-Forwarded-For`. That header **disables** the
  internal-CIDR auth bypass in `AuthMiddleware` (`api/auth.go:204`), forcing u4u to present a
  valid `Authorization: Bearer <token>`. Do **not** rely on network origin / source IP to
  authorize u4u — assume the token is the only thing standing between the internet and
  `/api/v1/genome/*`.
- **u4u runs a zero-config, fail-open posture.** Per the u4u investigation, the u4u team will
  carry exactly **one** secret: a single environment variable holding the bearer token. Khemeia's
  job is to make that the *only* knob u4u needs, and to enforce all the real security at our
  ingress (rate limiting, the `expires_at` rotation horizon, and the `revoked` kill-switch).
- **The token is all-or-nothing today.** A static `api_tokens` row grants access to every
  authenticated route, not just genome routes. There is no per-token scope column yet. This is
  the open question Q4 in the core TDD — see §7 below.

---

## 2. How Khemeia authenticates the token (no code changes)

The token is a row in the existing `api_tokens` table, checked by the `checkStaticToken` fast
path **before** any OIDC/JWT logic (`api/auth.go:224,322-335`). No new auth code is written for
u4u — we only seed a row.

`api_tokens` schema (`api/auth.go:347-356`, authoritative):

| Column       | Type           | Notes                                                        |
|--------------|----------------|--------------------------------------------------------------|
| `id`         | `SERIAL PK`    | auto                                                         |
| `token`      | `CHAR(64) UNIQUE` | the secret itself — **stored in plaintext**, compared verbatim. 32 random bytes hex-encoded → 64 chars. |
| `label`      | `VARCHAR(255)` | human label; set to `u4u-engine`. Surfaces as `submitted_by` on genome jobs for attribution. |
| `created_by` | `VARCHAR(255)` | defaults to `admin`                                          |
| `created_at` | `TIMESTAMP`    | auto                                                         |
| `expires_at` | `TIMESTAMP NULL` | the rotation horizon. `NULL` = never expires (avoid — set a horizon). |
| `revoked`    | `BOOLEAN`      | the kill-switch. `TRUE` ⇒ token rejected immediately.       |

The lookup the token must satisfy (`api/auth.go:328`):

```sql
SELECT label FROM api_tokens
 WHERE token = ?                                  -- exact match
   AND (expires_at IS NULL OR expires_at > NOW()) -- not expired
   AND revoked = FALSE;                           -- not revoked
```

> **Note on plaintext storage:** the `token` column holds the raw secret, not a hash. So anyone
> with read access to the `api_tokens` table (or a DB backup) can read every live token. Keep DB
> access tight, and treat the Bitwarden item — not the DB — as the system of record for the
> value. (Hashing the column is a separate hardening item; flagged for @security-engineer, out
> of scope for GEN-03.)

---

## 3. Provision the token — preferred path (admin REST endpoint)

Khemeia already exposes an **admin-only, internal-only** token endpoint
(`POST /api/v1/tokens`, `api/main.go:644`, `api/handlers.go:434`). It generates a
cryptographically-random 64-char token, inserts the row, and returns the value **once**. Because
the request originates from inside the cluster (no `X-Forwarded-For`), the internal-CIDR bypass
applies and no admin credential is needed — but for the same reason the endpoint must **never**
be exposed through the ingress.

`expires_in_hours` is required to set a horizon (the handler defaults to 72h if omitted, which is
**too short** for a service account — always pass an explicit long horizon, e.g. 1 year = 8760h).

```sh
# Run from a debug pod / port-forward INSIDE the cluster (internal IP, no ingress).
# Example via port-forward to the controller Service on :8080:
#   kubectl -n chem port-forward svc/khemeia-controller 8080:8080

curl -s -X POST http://localhost:8080/api/v1/tokens \
  -H 'Content-Type: application/json' \
  -d '{"label":"u4u-engine","expires_in_hours":8760}'
```

Response (the `token` value appears here and **nowhere else** — capture it immediately):

```json
{
  "token": "<64-hex-char-secret>",
  "label": "u4u-engine",
  "expires_at": "2027-06-12T00:00:00Z"
}
```

Immediately store `token` in Bitwarden (§5), then scrub it from your shell history.

## 3b. Alternative — direct SQL insert

If the admin endpoint is unavailable, insert the row directly. Generate the token with the same
shape the app uses (32 random bytes, hex):

```sh
TOKEN=$(openssl rand -hex 32)          # 64 hex chars — matches generateToken()
echo "store this in Bitwarden: $TOKEN"
```

```sql
-- Run against the shared Khemeia Postgres (the DB that holds api_tokens).
INSERT INTO api_tokens (token, label, created_by, expires_at, revoked)
VALUES (
  '<paste the 64-hex TOKEN here>',
  'u4u-engine',
  'operator',
  NOW() + INTERVAL '365 days',   -- rotation horizon; adjust as policy dictates
  FALSE
);
```

Verify (does **not** print the secret):

```sql
SELECT id, label, created_by, created_at, expires_at, revoked
  FROM api_tokens WHERE label = 'u4u-engine';
```

---

## 4. How u4u uses it (one env var, one header)

Hand u4u exactly two things: the **token value** and the **base URL**. u4u sets a single
environment variable and sends it as a bearer header on every Khemeia call.

```sh
# In u4u's environment (their secret store — one variable, that's the whole config):
export KHEMEIA_API_TOKEN="<the 64-hex secret>"
```

```sh
# Every request to Khemeia carries:
#   Authorization: Bearer ${KHEMEIA_API_TOKEN}
curl -X POST https://<khemeia-host>/api/v1/genome/variant/submit \
  -H "Authorization: Bearer ${KHEMEIA_API_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{ "variants": [ ... ], "calculations": [ "ddg_stability" ] }'
```

That is the entire integration contract on the auth side: **one env var, presented as a Bearer
token.** No OAuth flow, no token refresh, no client registration. The detailed request/response
bodies for the genome endpoints live in the core TDD §5.4 / §6 (and will be expanded in the
GEN-13 u4u call-set doc).

---

## 5. Bitwarden custody

- Store the token value as a Bitwarden item, e.g. **`khemeia-u4u-engine-token`**, in the same
  vault/collection as the other Khemeia secrets (alongside `garage-khemeia`).
  - **password / value:** the 64-hex token.
  - **notes:** label `u4u-engine`, the `expires_at` you set, the date provisioned, and "shared
    with Noah / u4u via <secure channel>".
- The Bitwarden item is the **system of record** for the value. Do not paste it into Slack,
  tickets, manifests, or this repo.
- **This token is NOT wired through ExternalSecret.** Unlike the Garage creds, it is not consumed
  by an in-cluster pod — it lives in u4u's environment, outside our cluster. So there is no
  `ExternalSecret` / `garage-secret`-style manifest for it, and there must not be a token value
  in any YAML under `rke2/`.

---

## 6. Rotation and the revoke kill-switch

**Revoke (instant cut-off).** Flip `revoked` — the next request fails closed:

```sql
UPDATE api_tokens SET revoked = TRUE WHERE label = 'u4u-engine';
```

(or by `id`: `UPDATE api_tokens SET revoked = TRUE WHERE id = <id>;`, mirroring the
`RevokeAPIToken` handler at `api/handlers.go:543`.)

**Rotate.** Provision a fresh row (§3) with a new value and a new horizon, hand the new value to
u4u, let them cut over, then revoke the old row. Because lookups match the exact token string,
two rows can be valid simultaneously during the cutover window — that is the zero-downtime
rotation path.

**Expiry.** When `NOW() > expires_at`, the token stops working automatically (no action needed).
Set a calendar reminder ahead of `expires_at` so u4u is rotated before it lapses.

---

## 7. Security boundary — what the operator owns

The token is the whole trust gate, so the controls below are **operator responsibilities at the
ingress**, not things u4u or the token row enforce on their own:

- **Rate-limit `/api/v1/genome/*` at the ingress.** u4u fans out batches
  (`ThreadPoolExecutor(max_workers=8)`); a runaway or compromised client could otherwise flood
  the controller and the GPU calc queue. Apply a per-route / per-client rate limit at the ingress
  (HTTPRoute / gateway policy). (Core TDD R3, Q3 — coordinate with @security-engineer.)
- **Do not trust network origin.** External requests are authorized by the bearer token only; the
  internal-CIDR bypass deliberately does not apply to forwarded requests.
- **Keep the admin token endpoint internal.** `/api/v1/tokens*` must never be reachable through
  the ingress — it would let anyone mint tokens.
- **Tight DB access.** Because tokens are stored in plaintext, read access to `api_tokens` (and
  DB backups) is equivalent to holding every live token.

### Open item to confirm with Noah (u4u maintainer)

- [ ] **Confirm u4u will carry exactly ONE API-key env var** (`KHEMEIA_API_TOKEN`) and send it as
  `Authorization: Bearer`. The whole design above assumes u4u's fail-open, zero-config posture
  means one secret, set once — verify that matches how u4u wants to wire it before issuing the
  token. If u4u wants per-environment tokens (staging vs prod), provision one labeled row each
  (`u4u-engine-staging`, `u4u-engine-prod`) so the kill-switch and rotation horizons are
  independent.
- [ ] (Stretch, ties to core Q4) If u4u's all-or-nothing access is a concern, raise the
  `api_tokens.scope` column proposal with @security-engineer so the u4u token can be constrained
  to genome routes only. Out of scope for GEN-03.

---

## 8. Checklist

- [ ] Token row created with `label='u4u-engine'`, an explicit `expires_at` horizon, `revoked=FALSE`.
- [ ] Token value stored in Bitwarden (`khemeia-u4u-engine-token`); **not** committed anywhere.
- [ ] Token value delivered to Noah / u4u over a secure channel.
- [ ] u4u confirmed they set it as one env var → `Authorization: Bearer`.
- [ ] Ingress rate-limit on `/api/v1/genome/*` in place (coordinate with @security-engineer).
- [ ] Calendar reminder set ahead of `expires_at` for rotation.
