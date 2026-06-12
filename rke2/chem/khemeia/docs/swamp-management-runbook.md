# Swamp Management Runbook — Florida Man Bioscience access

> Companion to `rke2/chem/khemeia/docs/tdd/khemeia-genomics-platform.md` (Part A).
> Covers the two **non-git, one-time / on-demand** operator steps the declarative
> blueprint + Grafana role mapping cannot express on their own:
>
> 1. Granting the **"Florida Man Bioscience" (FMB)** collaborators **Admin** on the
>    Grafana **"Swamp"** folder (one-time, post-folder-creation).
> 2. Adding **Curtis / Tom** to the FMB group once they self-enroll and you learn
>    their Authentik usernames (a one-line blueprint edit in **two** files).
>
> Everything else (the group itself, the Editor-at-org role mapping, the Swamp
> dashboard) is already declarative and git-managed. This runbook is the *only*
> click-ops / out-of-band part, and it is small by design.

---

## 0. What "FMB manages the swamp" actually means (the enforceability boundary)

Be precise about the grant so no one over-promises. Per platform TDD §4.2.2, "Florida
Man Bioscience manages the swamp" decomposes into exactly two **enforceable-today**
capabilities, plus a set of things that are explicitly **out of scope**:

| Capability | Mechanism | Enforceable today? |
|---|---|---|
| Watch genomics tooling health / triage failures | Swamp Grafana dashboard + **Editor** org-role + **Admin** on the Swamp folder | **Yes** |
| Log into Khemeia web (browse jobs/results UI) | existing `khemeia` OIDC app; FMB member signs in via GitHub → Authentik → Khemeia | **Yes** (app is open: `policy_engine_mode: any`) |
| Gate Khemeia web to *only* FMB + Infrastructure | add a `policybinding` FMB → khemeia app (TDD §4.2.3) | Possible, but **DEFERRED** — operator decision; changes who can use Khemeia globally |
| Submit/cancel genome jobs **as themselves** via the API | would need Khemeia to honor the `groups` claim for `/api/v1/genome/*` write-auth | **No** — aspirational; API auth is `api_token`/OIDC-JWT, not group-aware (core TDD) |
| Rotate/revoke the u4u `api_token` | DB/Bitwarden operation, platform-owner action | **No** — by design; token custody stays with the owner |

**Honest one-line summary:** *FMB membership = full ownership of the Swamp Grafana
folder + interactive Khemeia web access.* It is **not** API-level admin over genome
jobs — that would require group-aware authorization the platform layer deliberately
does not build (PRD scopes out multi-tenancy). This is the correct least-privilege
answer; state it plainly to Curtis/Tom so expectations match reality.

What the declarative layer already gives you (no action needed here):

- **Group "Florida Man Bioscience"** — pinned UUID `5f7efde0-fe9b-48f1-b443-e42947bf7f2e`,
  defined in BOTH `rke2/authentik/blueprints/groups.yaml` and the mirrored `groups.yaml`
  key inside `rke2/authentik/blueprints-configmap.yaml` (the ConfigMap is what is
  mounted into the Authentik server+worker; `rke2/authentik/update.sh` is only a
  `helm upgrade`, **not** a blueprint generator — so both files are hand-maintained
  and must stay in sync).
- **Grafana org role = Editor** for any FMB member — via the `role_attribute_path`
  nested ternary in `rke2/monitor/kube-prometheus-stack-values.yaml`:
  `contains(groups[*], 'Grafana Admins') && 'GrafanaAdmin' || contains(groups[*], 'Florida Man Bioscience') && 'Editor' || 'Viewer'`
  (`role_attribute_strict: false` keeps non-members at Viewer).

---

## 1. One-time: grant FMB **Admin** on the Grafana "Swamp" folder

### Why this is a manual step
Grafana's OIDC `role_attribute_path` sets the **org role** only (Viewer / Editor /
Admin / GrafanaAdmin). **Folder-scoped** permissions are *not* derivable from an OIDC
claim — they live on the folder object. So the design (TDD §4.2.1, Decision B1) splits:

- **Editor at org** — claim-driven, immediate, git-tracked (already done). Lets FMB
  members edit/save dashboards and own their panels.
- **Admin on the Swamp folder** — a one-time folder-permission grant, documented here.
  Gives them permission/alert management *within* the Swamp folder only, without
  touching any other folder and without making them Grafana server admins.

### Prerequisite
The **"Swamp"** folder must already exist. It is auto-created by the Grafana sidecar the
first time the `grafana-dashboard-swamp` ConfigMap is loaded (GEN-51, the
`grafana_folder: "Swamp"` annotation). Confirm it exists before granting:

```bash
# The folder shows up once the dashboard ConfigMap is applied and the sidecar
# has reconciled. Verify the dashboard landed in a "Swamp" folder:
kubectl get configmap -A -l grafana_dashboard=1 | grep -i swamp
# Then in Grafana: Dashboards → Folders → "Swamp" should be listed.
```

### Option A (recommended today) — grant the **Editor** role Admin on the Swamp folder

Because every FMB member is an **Editor** (org role, via the claim) and — today — the
only Editors are FMB members, granting the **Editor role** `Admin` on the **Swamp folder
only** cleanly equals "FMB admins the Swamp folder" without affecting any other folder.

**Via the Grafana UI (click-ops, fastest):**
1. Sign in to https://grafana.hwcopeland.net as a GrafanaAdmin (a "Grafana Admins"
   group member).
2. Dashboards → Folders → **Swamp** → **Permissions**.
3. **Add a permission** → Role = **Editor** → **Admin** → Save.
4. (Leave the default inherited permissions in place; you are *adding* Admin-for-Editor,
   not removing anything.)

**Via the Grafana HTTP API (scriptable, repeatable):**
```bash
# Requires a Grafana admin API token or basic auth as a GrafanaAdmin.
GRAFANA=https://grafana.hwcopeland.net
# 1) Find the Swamp folder UID:
curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA/api/folders" | jq -r '.[] | select(.title=="Swamp") | .uid'
# 2) Set folder permissions: grant the Editor role Admin on this folder.
#    (permission: 1=Viewer, 2=Editor, 4=Admin)
curl -s -X POST -H "Authorization: Bearer $GRAFANA_TOKEN" \
  -H "Content-Type: application/json" \
  "$GRAFANA/api/folders/<SWAMP_FOLDER_UID>/permissions" \
  -d '{"items":[{"role":"Editor","permission":4}]}'
```
> Note: setting folder permissions **replaces** the explicit permission list for that
> folder. Include any other explicit grants you want to keep in the same `items` array.
> When in doubt, use the UI which shows the current list before you edit.

### Caveat — the day a non-FMB user becomes an Editor
Option A keys on the **Editor role**, not the FMB group. The moment another group is
mapped to Editor at org level, *those* users would also inherit Admin on the Swamp
folder. Today that cannot happen (only FMB → Editor). If you ever add a second
Editor-mapped group, migrate to **Option B** below before doing so.

### Option B (cleaner upgrade, deferred) — a Grafana **Team** synced from the FMB group

Provision a Grafana **Team** "Florida Man Bioscience", grant *that team* Admin on the
Swamp folder, and sync team membership from the group. This makes the folder grant
group-scoped (not role-scoped), so it stays correct even if other groups become Editors.

Deferred (TDD §6 Q2): Grafana team-sync from generic OAuth needs `team_ids` /
`team_ids_attribute_path` config that is **not present** in
`kube-prometheus-stack-values.yaml` today, and a custom Authentik scope mapping to emit
team IDs. Not worth it for two users. Revisit only if folder-RBAC churn appears.

---

## 2. On-demand: add Curtis / Tom to the FMB group (deferred membership)

Curtis and Tom have **no Authentik accounts yet** and the group ships with **no
members** — by design (TDD §4.3). The group, the Editor role mapping, and the dashboard
are all live regardless; the empty group simply has no members and **nothing is
blocked**. Do **not** invent usernames.

### Enrollment flow (existing, unchanged)
1. Curtis/Tom visit https://khemeia.net (or https://grafana.hwcopeland.net) and choose
   **"Sign up with GitHub"** → the existing `github-invite-enrollment` flow creates the
   account **inactive** and shows a "pending approval" deny message. You (the owner) get
   a `new-enrollment-notification` email.
2. In Authentik admin (https://auth.hwcopeland.net), **activate** the new user
   (Directory → Users → toggle Active).
3. Note their exact Authentik **username** (the `username` field, not the display name).

### The one-line membership edit — **MUST edit BOTH files**

The FMB group lives in two hand-maintained, must-stay-in-sync places. Add the user(s)
under the group's `attrs.users:` list in **both**. The pattern is identical to how
`hwcopeland` is added to "Infrastructure".

**File 1 — `rke2/authentik/blueprints/groups.yaml`** (base indentation, 4 spaces for
`name`/`users`):
```yaml
    name: Florida Man Bioscience
    users:
    - !Find [authentik_core.user, [username, CURTIS_USERNAME_TBD]]
    - !Find [authentik_core.user, [username, TOM_USERNAME_TBD]]
```

**File 2 — `rke2/authentik/blueprints-configmap.yaml`** (the same entry, but inside the
`groups.yaml: |` block scalar, so indented **+4 more spaces** — `name`/`users` at 8
spaces, list items at 8):
```yaml
        name: Florida Man Bioscience
        users:
        - !Find [authentik_core.user, [username, CURTIS_USERNAME_TBD]]
        - !Find [authentik_core.user, [username, TOM_USERNAME_TBD]]
```

> **Why both:** `update.sh` is only `helm upgrade` — it does NOT regenerate the
> ConfigMap from `blueprints/`. The ConfigMap is what Authentik actually mounts. Edit
> `blueprints/groups.yaml` for the readable source-of-truth AND the mirrored key in
> `blueprints-configmap.yaml` for the runtime, or the membership change is silently
> dropped at reconcile. Keep the `pk`, `attrs.attributes`, and `model` identical to the
> FMB entry already present (pk `5f7efde0-fe9b-48f1-b443-e42947bf7f2e`) — you are only
> adding the `users:` list, not changing identity.

### Apply
1. Commit both edits (one logical change).
2. Re-run `cd rke2/authentik && ./update.sh` (the `helm upgrade` that re-mounts the
   ConfigMap), **or** let GitOps/Flux apply the ConfigMap if it is reconciled that way.
3. Authentik's server/worker reconcile blueprints from the mounted ConfigMap on a timer
   — **no pod restart needed** for a blueprint change. Allow a minute, then verify.

### Verify
```bash
# The mounted ConfigMap contains the FMB group with the new members:
kubectl get configmap authentik-blueprints -n authentik -o yaml \
  | grep -A12 'Florida Man Bioscience'
```
- In Authentik admin: Directory → Groups → **Florida Man Bioscience** → the user(s)
  appear as members.
- Have the user log into Grafana; in Grafana admin → Users, their **Org role** should
  read **Editor** (the claim-driven half), and they should be able to manage the Swamp
  folder (the Option-A folder grant).

---

## 3. Deferred / operator-gated extras (do NOT apply without sign-off)

- **Gate Khemeia web to FMB + Infrastructure** (TDD §4.2.3, §6 Q3): add a
  `policybinding` (FMB → `khemeia` application) in
  `rke2/authentik/blueprints/providers-khemeia.yaml` (+ the configmap mirror), mirroring
  the Hubble/Longhorn/Synology/JupyterHub → Infrastructure idiom. This **changes who can
  reach Khemeia web globally** (the app is open today: `policy_engine_mode: any`), so it
  is an explicit operator decision, not a default. Khemeia does **not** consume the
  `groups` claim today (its provider `property_mappings` omit the `groups` scope), so the
  binding only gates *reaching* the app, not in-app authorization.
- **Pass FMB group names into Khemeia** for future in-app gating: add the `groups` scope
  mapping to the khemeia provider's `property_mappings` (a one-line `!Find` add). Flagged
  for when Khemeia learns to read the claim (TDD §6 R2) — application-side work, not this
  runbook.
- **Group-aware API authorization** for `/api/v1/genome/*` write ops — core/workers
  concern (TDD §6 R2), out of platform scope.

---

## Quick reference

| Thing | Value |
|---|---|
| FMB group name | `Florida Man Bioscience` |
| FMB group UUID (pinned) | `5f7efde0-fe9b-48f1-b443-e42947bf7f2e` |
| Group blueprint (source) | `rke2/authentik/blueprints/groups.yaml` |
| Group blueprint (mounted mirror) | `rke2/authentik/blueprints-configmap.yaml` (`groups.yaml` key) |
| Grafana role mapping | `rke2/monitor/kube-prometheus-stack-values.yaml` → `auth.generic_oauth.role_attribute_path` |
| Swamp dashboard ConfigMap | `rke2/chem/khemeia/deploy/grafana-dashboard-swamp.yaml` (GEN-51) |
| Khemeia OIDC app | provider `khemeia` (public), app slug `khemeia`, open (`policy_engine_mode: any`) |
| Apply blueprints | `cd rke2/authentik && ./update.sh` (helm upgrade) or GitOps reconcile |
