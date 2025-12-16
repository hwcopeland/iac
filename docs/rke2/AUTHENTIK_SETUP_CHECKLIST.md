# Authentik Setup Checklist

This document provides a step-by-step checklist for setting up Authentik OAuth/OIDC integration for all services.

## Prerequisites

- Authentik is deployed at `https://auth.hwcopeland.net`
- Admin access to Authentik
- Admin access to Bitwarden for credential storage
- Kubernetes cluster access for applying configurations

## Setup Order

Follow this order to set up the integrations:

### 1. Setup n8n OIDC Integration

**In Authentik:**
1. Navigate to **Applications → Providers → Create**
2. Select **OAuth2/OpenID Provider**
3. Configure:
   - Name: `n8n`
   - Authorization flow: `default-provider-authorization-implicit-consent`
   - Client Type: **Confidential**
   - Client ID: (auto-generated, save this)
   - Client Secret: (auto-generated, save this)
   - Redirect URIs: `https://n8n.hwcopeland.net/rest/oauth2-credential/callback`
   - Scopes: `openid`, `email`, `profile`, `groups`
   - Subject mode: `Based on the User's Email`
4. Click **Finish**

**In Bitwarden:**
1. Create new **Login** item
2. Name: `n8n-oidc-authentik`
3. Username: (Client ID from Authentik)
4. Password: (Client Secret from Authentik)
5. Save

**In Authentik (Create Application):**
1. Navigate to **Applications → Applications → Create**
2. Configure:
   - Name: `n8n`
   - Slug: `n8n`
   - Provider: Select the `n8n` provider created above
3. Click **Create**

**In Kubernetes:**
```bash
kubectl apply -f rke2/agent-system/n8n-oidc-secret.yaml
kubectl apply -f rke2/agent-system/n8n-deployment.yaml
kubectl rollout restart deployment/n8n -n agent-system
```

### 2. Setup ArgoCD OIDC Integration

**In Authentik:**
1. Navigate to **Applications → Providers → Create**
2. Select **OAuth2/OpenID Provider**
3. Configure:
   - Name: `ArgoCD`
   - Authorization flow: `default-provider-authorization-implicit-consent`
   - Client Type: **Confidential**
   - Client ID: (auto-generated, save this)
   - Client Secret: (auto-generated, save this)
   - Redirect URIs: `https://argocd.hwcopeland.net/auth/callback`
   - Scopes: `openid`, `profile`, `email`, `groups`
   - Subject mode: `Based on the User's Email`
4. Click **Finish**

**In Bitwarden:**
1. Create new **Login** item
2. Name: `argocd-oidc-authentik`
3. Username: (Client ID from Authentik)
4. Password: (Client Secret from Authentik)
5. Save

**In Authentik (Create Application):**
1. Navigate to **Applications → Applications → Create**
2. Configure:
   - Name: `ArgoCD`
   - Slug: `argocd`
   - Provider: Select the `ArgoCD` provider created above
3. Click **Create**

**In Kubernetes:**
```bash
kubectl apply -f rke2/argocd/argocd-oidc-secret.yaml

# Follow the instructions in rke2/argocd/README.md to:
# 1. Update argocd-cm ConfigMap with OIDC configuration
# 2. Update argocd-rbac-cm ConfigMap with group-based policies
# 3. Restart ArgoCD server
```

### 3. Setup Longhorn Forward Auth

**In Authentik:**
1. Navigate to **Applications → Providers → Create**
2. Select **Proxy Provider**
3. Configure:
   - Name: `Longhorn`
   - Authorization flow: `default-provider-authorization-implicit-consent`
   - Type: **Forward auth (single application)**
   - External host: `https://longhorn.hwcopeland.net`
4. Click **Finish**

**In Authentik (Create Application):**
1. Navigate to **Applications → Applications → Create**
2. Configure:
   - Name: `Longhorn`
   - Slug: `longhorn`
   - Provider: Select the `Longhorn` provider created above
   - Policy bindings: Bind to groups that should have access (e.g., `Longhorn Admins`, `Infrastructure Team`)
3. Click **Create**

**In Authentik (Create/Update Outpost):**
1. Navigate to **Applications → Outposts**
2. If `embedded-outpost` exists:
   - Edit it and add `Longhorn` to the Applications list
3. If not, create new:
   - Name: `embedded-outpost`
   - Type: **Proxy**
   - Applications: Select `Longhorn`
4. Note the outpost token

**In Bitwarden:**
1. Create new **Login** item
2. Name: `longhorn-authentik-token`
3. Password: (Outpost token from Authentik)
4. Save

**In Kubernetes:**
```bash
kubectl apply -f rke2/longhorn-system/authentik-outpost-secret.yaml
kubectl apply -f rke2/longhorn-system/httproute-longhorn.yaml
```

### 4. Setup Hubble Forward Auth

**In Authentik:**
1. Navigate to **Applications → Providers → Create**
2. Select **Proxy Provider**
3. Configure:
   - Name: `Hubble`
   - Authorization flow: `default-provider-authorization-implicit-consent`
   - Type: **Forward auth (single application)**
   - External host: `https://hubble.hwcopeland.net`
4. Click **Finish**

**In Authentik (Create Application):**
1. Navigate to **Applications → Applications → Create**
2. Configure:
   - Name: `Hubble`
   - Slug: `hubble`
   - Provider: Select the `Hubble` provider created above
   - Policy bindings: Bind to groups that should have access (e.g., `Hubble Admins`, `Infrastructure Team`)
3. Click **Create**

**In Authentik (Update Outpost):**
1. Navigate to **Applications → Outposts**
2. Edit `embedded-outpost`
3. Add `Hubble` to the Applications list
4. Save

**In Bitwarden:**
1. Create new **Login** item (or reuse `longhorn-authentik-token` if using same outpost)
2. Name: `hubble-authentik-token`
3. Password: (Outpost token from Authentik)
4. Save

**In Kubernetes:**
```bash
kubectl apply -f rke2/kube-system/cilium/hubble-oidc-secret.yaml
kubectl apply -f rke2/kube-system/cilium/httproute-hubble.yaml
```

### 5. Create User Groups

**In Authentik:**
1. Navigate to **Directory → Groups**
2. Create the following groups:
   - `Grafana Admins` - For full Grafana admin access
   - `ArgoCD Admins` - For full ArgoCD admin access
   - `n8n Users` - For n8n access
   - `Longhorn Admins` - For Longhorn UI access
   - `Hubble Admins` - For Hubble UI access
   - `Infrastructure Team` - For read-only access to infrastructure services

### 6. Assign Users to Groups

**In Authentik:**
1. Navigate to **Directory → Users**
2. For each user:
   - Click on the user
   - Go to **Groups** tab
   - Click **Add to existing group**
   - Select appropriate groups
   - Save

### 7. Update Homepage

The homepage has already been updated to include the new services. No additional action needed.

**Services added:**
- Hubble (https://hubble.hwcopeland.net)
- Authentik (https://auth.hwcopeland.net)

## Verification Steps

After completing the setup, verify each integration:

### Test n8n
```bash
# Open in browser
open https://n8n.hwcopeland.net

# Should show "Sign in with Authentik" button
# Click and authenticate
# Should successfully log in
```

### Test ArgoCD
```bash
# Open in browser
open https://argocd.hwcopeland.net

# Should show "Log in via Authentik" button
# Click and authenticate
# Should successfully log in with appropriate role
```

### Test Longhorn
```bash
# Open in browser
open https://longhorn.hwcopeland.net

# Should automatically redirect to Authentik login
# Authenticate
# Should redirect back to Longhorn UI
```

### Test Hubble
```bash
# Open in browser
open https://hubble.hwcopeland.net

# Should automatically redirect to Authentik login
# Authenticate
# Should redirect back to Hubble UI
```

### Test Grafana (Already Configured)
```bash
# Open in browser
open https://grafana.hwcopeland.net

# Should show "Sign in with Authentik" button
# Click and authenticate
# Role should be assigned based on group membership
```

## Troubleshooting

If any service fails to authenticate:

1. **Check External Secret Status:**
```bash
kubectl get externalsecrets -A
kubectl describe externalsecret <secret-name> -n <namespace>
```

2. **Check Provider Configuration:**
   - Verify redirect URIs match exactly
   - Verify client ID and secret are correct in Bitwarden
   - Check that provider is linked to application

3. **Check Application Bindings:**
   - Verify user is in appropriate groups
   - Verify groups are bound to application policies

4. **Check Logs:**
```bash
# For n8n
kubectl logs -n agent-system deployment/n8n -f

# For ArgoCD
kubectl logs -n argocd deployment/argocd-server -f

# For Authentik
kubectl logs -n authentik deployment/authentik-server -f
```

## Next Steps

After completing all integrations:

1. Test each service with multiple users
2. Verify group-based access control works
3. Document any custom policies or configurations
4. Set up monitoring for authentication failures
5. Consider enabling MFA in Authentik for additional security

## Reference Documentation

- Main Integration Guide: `docs/rke2/AUTHENTIK_INTEGRATION.md`
- ArgoCD Specific Setup: `rke2/argocd/README.md`
- Authentik Documentation: https://docs.goauthentik.io/
