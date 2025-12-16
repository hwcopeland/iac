# Authentik Integration Guide

This guide explains how to set up Authentik OIDC integration for various services in the cluster.

## Overview

Authentik is deployed at `https://auth.hwcopeland.net` and provides centralized authentication for multiple services using OAuth2/OIDC.

## Services Integrated with Authentik

### 1. Grafana (Completed)

Grafana uses OIDC for authentication with group-based role mapping.

**Configuration Location**: `rke2/monitor/values.yaml`

**Role Mapping**:
- Members of `Grafana Admins` group → GrafanaAdmin role
- All other authenticated users → Viewer role

**Credentials Secret**: `grafana-oidc-secret` in `monitor` namespace

### 2. n8n (New)

n8n uses OIDC for SSO authentication.

**Configuration Location**: `rke2/agent-system/n8n-deployment.yaml`

**Required Authentik Setup**:
1. Create OAuth2/OIDC Provider in Authentik:
   - Name: `n8n`
   - Client Type: Confidential
   - Redirect URIs: `https://n8n.hwcopeland.net/rest/oauth2-credential/callback`
   - Scopes: `openid`, `email`, `profile`, `groups`

2. Store credentials in Bitwarden:
   - Item name: `n8n-oidc-authentik`
   - Username: Client ID from Authentik
   - Password: Client Secret from Authentik

**Credentials Secret**: `n8n-oidc-secret` in `agent-system` namespace

### 3. Longhorn (New)

Longhorn UI uses Authentik's forward auth (proxy provider) for authentication.

**Configuration Location**: `rke2/longhorn-system/httproute-longhorn.yaml`

**Required Authentik Setup**:
1. Create Proxy Provider in Authentik:
   - Name: `Longhorn`
   - Authorization Flow: default-provider-authorization-implicit-consent
   - External Host: `https://longhorn.hwcopeland.net`
   - Mode: Forward auth (single application)

2. Create Application in Authentik:
   - Name: `Longhorn`
   - Slug: `longhorn`
   - Provider: Select the proxy provider created above
   - Policy Bindings: Add groups that should have access

3. Create Outpost:
   - Name: `embedded-outpost`
   - Type: Proxy
   - Applications: Select Longhorn application

4. Store outpost token in Bitwarden:
   - Item name: `longhorn-authentik-token`
   - Password: Outpost token from Authentik

**Credentials Secret**: `longhorn-oidc-secret` in `longhorn-system` namespace

### 4. Hubble UI (New)

Hubble UI (Cilium network observability) uses Authentik's forward auth (proxy provider) for authentication.

**Configuration Location**: `rke2/kube-system/cilium/httproute-hubble.yaml`

**Required Authentik Setup**:
1. Create Proxy Provider in Authentik:
   - Name: `Hubble`
   - Authorization Flow: default-provider-authorization-implicit-consent
   - External Host: `https://hubble.hwcopeland.net`
   - Mode: Forward auth (single application)

2. Create Application in Authentik:
   - Name: `Hubble`
   - Slug: `hubble`
   - Provider: Select the proxy provider created above
   - Policy Bindings: Add groups that should have access (e.g., `Infrastructure Team`)

3. Use the same Outpost as Longhorn:
   - Add Hubble application to the existing `embedded-outpost`

4. Store outpost token in Bitwarden:
   - Item name: `hubble-authentik-token`
   - Password: Outpost token from Authentik (can reuse same token)

**Credentials Secret**: `hubble-oidc-secret` in `kube-system` namespace

### 5. Home Assistant (New)

Home Assistant uses Authentik's forward auth (proxy provider) for authentication.

**Configuration Location**: `rke2/web-server/homeassistant/homeassistant-oidc-secret.yaml`, `rke2/web-server/httproute-web-apps.yaml`

**Required Authentik Setup**:
1. Create Proxy Provider in Authentik:
   - Name: `Home Assistant`
   - Authorization Flow: default-provider-authorization-implicit-consent
   - External Host: `https://ha.hwcopeland.net`
   - Mode: Forward auth (single application)

2. Create Application in Authentik:
   - Name: `Home Assistant`
   - Slug: `homeassistant`
   - Provider: Select the proxy provider created above
   - Policy Bindings: Add groups that should have access (e.g., `Home Users`, `Infrastructure Team`)

3. Use the same Outpost as Longhorn/Hubble:
   - Add Home Assistant application to the existing `embedded-outpost`

4. Store outpost token in Bitwarden:
   - Item name: `homeassistant-authentik-token`
   - Password: Outpost token from Authentik (can reuse same token)

**Credentials Secret**: `homeassistant-oidc-secret` in `web-server` namespace

### 6. Homepage (New)

Homepage uses Authentik's forward auth (proxy provider) for authentication.

**Configuration Location**: `rke2/web-server/homepage/homepage-oidc-secret.yaml`, `rke2/web-server/homepage/httproute.yaml`

**Required Authentik Setup**:
1. Create Proxy Provider in Authentik:
   - Name: `Homepage`
   - Authorization Flow: default-provider-authorization-implicit-consent
   - External Host: `https://home.hwcopeland.net`
   - Mode: Forward auth (single application)

2. Create Application in Authentik:
   - Name: `Homepage`
   - Slug: `homepage`
   - Provider: Select the proxy provider created above
   - Policy Bindings: Add groups that should have access (e.g., `Home Users`, `Infrastructure Team`)

3. Use the same Outpost as Longhorn/Hubble:
   - Add Homepage application to the existing `embedded-outpost`

4. Store outpost token in Bitwarden:
   - Item name: `homepage-authentik-token`
   - Password: Outpost token from Authentik (can reuse same token)

**Credentials Secret**: `homepage-oidc-secret` in `web-server` namespace

## Group-Based Access Control

### Creating Groups in Authentik

1. Navigate to Directory → Groups
2. Create groups as needed:
   - `Grafana Admins` - Full admin access to Grafana
   - `n8n Users` - Access to n8n automation platform
   - `Longhorn Admins` - Access to Longhorn storage UI
   - `Hubble Admins` - Access to Hubble network observability UI
   - `Home Users` - Access to Home Assistant and Homepage
   - `Infrastructure Team` - Access to all infrastructure services (read-only for most)

### Assigning Users to Groups

1. Navigate to Directory → Users
2. Select a user
3. Go to Groups tab
4. Add user to appropriate groups

## External Secrets Setup

All OIDC credentials are stored in Bitwarden and synchronized to Kubernetes using External Secrets Operator.

**Required Bitwarden Items**:
- `n8n-oidc-authentik` (Login type)
- `longhorn-authentik-token` (Login type)
- `hubble-authentik-token` (Login type)
- `homeassistant-authentik-token` (Login type)
- `homepage-authentik-token` (Login type)
- `grafana-oidc-secret` (Login type) - Already exists

## Testing Authentication

### Grafana
1. Navigate to `https://grafana.hwcopeland.net`
2. Click "Sign in with Authentik"
3. Authenticate with Authentik credentials
4. Verify role assignment based on group membership

### n8n
1. Navigate to `https://n8n.hwcopeland.net`
2. Click "Sign in with Authentik" (or similar SSO option)
3. Authenticate with Authentik credentials
4. Verify access to n8n workflows

### Longhorn
1. Navigate to `https://longhorn.hwcopeland.net`
2. Authentik forward auth will automatically redirect to login
3. Authenticate with Authentik credentials
4. Verify access to Longhorn UI

### Hubble
1. Navigate to `https://hubble.hwcopeland.net`
2. Authentik forward auth will automatically redirect to login
3. Authenticate with Authentik credentials
4. Verify access to Hubble network observability UI

### Home Assistant
1. Navigate to `https://ha.hwcopeland.net`
2. Authentik forward auth will automatically redirect to login
3. Authenticate with Authentik credentials
4. Verify access to Home Assistant

### Homepage
1. Navigate to `https://home.hwcopeland.net`
2. Authentik forward auth will automatically redirect to login
3. Authenticate with Authentik credentials
4. Verify access to Homepage dashboard

## Troubleshooting

### Check External Secrets Sync Status

```bash
# Check if secrets are syncing properly
kubectl get externalsecret -n monitor grafana-oidc-secret
kubectl get externalsecret -n agent-system n8n-oidc-secret
kubectl get externalsecret -n longhorn-system longhorn-oidc-secret
kubectl get externalsecret -n kube-system hubble-oidc-secret
kubectl get externalsecret -n web-server homeassistant-oidc-secret
kubectl get externalsecret -n web-server homepage-oidc-secret

# View secret details
kubectl describe externalsecret -n agent-system n8n-oidc-secret
```

### Verify Authentik Configuration

1. Log into Authentik admin interface at `https://auth.hwcopeland.net`
2. Navigate to Applications → Providers
3. Verify provider configurations match the service settings
4. Check Application bindings and policies

### Common Issues

**Issue**: "Invalid client ID" error
- **Solution**: Verify the client ID in Bitwarden matches the Authentik provider

**Issue**: "Redirect URI mismatch" error
- **Solution**: Ensure redirect URIs in Authentik provider include the service callback URL

**Issue**: Users can't access service after authentication
- **Solution**: Check user group membership and policy bindings in Authentik

## Security Considerations

- All client secrets are stored encrypted in Bitwarden
- Secrets are automatically rotated when updated in Bitwarden (within 5 minutes)
- Use group-based access control to limit service access
- Enable MFA in Authentik for additional security
- Review audit logs regularly in Authentik

## References

- [Authentik Documentation](https://docs.goauthentik.io/)
- [n8n OIDC Configuration](https://docs.n8n.io/hosting/configuration/environment-variables/oidc/)
- [Grafana OAuth Configuration](https://grafana.com/docs/grafana/latest/setup-grafana/configure-security/configure-authentication/generic-oauth/)
