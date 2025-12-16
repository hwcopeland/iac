# Implementation Summary: Authentik OAuth Integration

## Overview

This implementation adds Authentik OAuth/OIDC integration for multiple infrastructure services, enabling centralized authentication and group-based access control.

## Services Integrated

### 1. n8n Automation Platform
- **Type**: OIDC Integration
- **URL**: https://n8n.hwcopeland.net
- **Authentication Method**: OAuth2/OIDC with Authentik
- **Files Modified/Created**:
  - `rke2/agent-system/n8n-deployment.yaml` - Added OIDC environment variables
  - `rke2/agent-system/n8n-oidc-secret.yaml` - External secret for credentials
- **Group Access**: `n8n Users` group

### 2. Longhorn Storage UI
- **Type**: Forward Auth (Proxy Provider)
- **URL**: https://longhorn.hwcopeland.net
- **Authentication Method**: Authentik forward authentication
- **Files Created**:
  - `rke2/longhorn-system/httproute-longhorn.yaml` - HTTPRoute with forward auth
  - `rke2/longhorn-system/authentik-outpost-secret.yaml` - Outpost token secret
- **Group Access**: `Longhorn Admins`, `Infrastructure Team` groups

### 3. Hubble Network Observability
- **Type**: Forward Auth (Proxy Provider)
- **URL**: https://hubble.hwcopeland.net
- **Authentication Method**: Authentik forward authentication
- **Files Created**:
  - `rke2/kube-system/cilium/httproute-hubble.yaml` - HTTPRoute with forward auth
  - `rke2/kube-system/cilium/hubble-oidc-secret.yaml` - Outpost token secret
- **Group Access**: `Hubble Admins`, `Infrastructure Team` groups

### 4. ArgoCD GitOps
- **Type**: OIDC Integration
- **URL**: https://argocd.hwcopeland.net
- **Authentication Method**: OAuth2/OIDC with Authentik
- **Files Created**:
  - `rke2/argocd/argocd-oidc-secret.yaml` - External secret for credentials
  - `rke2/argocd/README.md` - Detailed setup instructions
- **Group Access**: `ArgoCD Admins` (admin), `Infrastructure Team` (read-only)

## Homepage Updates

Updated Homepage dashboard (`rke2/web-server/homepage/config.yaml`) to include:
- **Hubble** - Network observability dashboard
- **Authentik** - Identity provider management UI

Layout changed from 4 to 6 columns in Infrastructure section to accommodate new services.

## Security Architecture

### Credential Management
All OAuth/OIDC credentials are managed using:
1. **Bitwarden** - Secure credential storage
2. **External Secrets Operator** - Automatic synchronization to Kubernetes
3. **Kubernetes Secrets** - Runtime credential access

### Authentication Flow

#### OIDC Services (n8n, ArgoCD):
```
User → Service → Authentik Login → Group Check → Access Granted/Denied
```

#### Forward Auth Services (Longhorn, Hubble):
```
User → HTTPRoute → Authentik Proxy → Group Check → Backend Service
```

## Group-Based Access Control

### Recommended Groups

| Group Name | Purpose | Services |
|------------|---------|----------|
| `Grafana Admins` | Full Grafana admin access | Grafana |
| `ArgoCD Admins` | Full ArgoCD admin access | ArgoCD |
| `n8n Users` | Access to automation platform | n8n |
| `Longhorn Admins` | Storage management | Longhorn |
| `Hubble Admins` | Network observability | Hubble |
| `Infrastructure Team` | Read-only infrastructure access | All services |

## Documentation Created

1. **`docs/rke2/AUTHENTIK_INTEGRATION.md`**
   - Comprehensive integration guide
   - Configuration details for each service
   - Troubleshooting section
   - Security considerations

2. **`docs/rke2/AUTHENTIK_SETUP_CHECKLIST.md`**
   - Step-by-step setup instructions
   - Authentik UI configuration steps
   - Kubernetes deployment commands
   - Verification procedures

3. **`rke2/argocd/README.md`**
   - ArgoCD-specific setup guide
   - ConfigMap configuration examples
   - RBAC policy configuration

## Deployment Prerequisites

Before deploying these changes:

1. **Bitwarden Items Required**:
   - `n8n-oidc-authentik` (Client ID and Secret)
   - `argocd-oidc-authentik` (Client ID and Secret)
   - `longhorn-authentik-token` (Outpost token)
   - `hubble-authentik-token` (Outpost token)

2. **Authentik Configuration**:
   - OAuth providers created for n8n and ArgoCD
   - Proxy providers created for Longhorn and Hubble
   - Applications created and linked to providers
   - Outpost configured for forward auth services
   - User groups created and configured

3. **Kubernetes Resources**:
   - External Secrets Operator installed and configured
   - Bitwarden ClusterSecretStore configured
   - Gateway API resources available
   - Cloudflare DNS operator installed

## Testing Checklist

After deployment, verify each service:

- [ ] n8n displays "Sign in with Authentik" option
- [ ] n8n successfully authenticates via Authentik
- [ ] ArgoCD displays "Log in via Authentik" option
- [ ] ArgoCD successfully authenticates with role assignment
- [ ] Longhorn redirects to Authentik for authentication
- [ ] Longhorn grants access after authentication
- [ ] Hubble redirects to Authentik for authentication
- [ ] Hubble grants access after authentication
- [ ] Homepage displays Hubble and Authentik services
- [ ] All service links work correctly from Homepage

## Benefits

1. **Centralized Authentication**: Single sign-on across all infrastructure services
2. **Group-Based Access**: Easy management of user permissions
3. **Security**: No local passwords, OAuth token-based authentication
4. **Auditability**: All authentication events logged in Authentik
5. **Consistency**: Same authentication pattern across services
6. **Maintainability**: Credentials managed in Bitwarden, automatically synced

## Limitations

1. **ArgoCD ConfigMap**: Requires manual ConfigMap updates (not managed via Helm values)
2. **Forward Auth Services**: Require Authentik outpost to be running
3. **Bitwarden Dependency**: All services depend on Bitwarden being accessible
4. **Initial Setup**: Requires manual configuration in Authentik UI

## Future Enhancements

1. Add Authentik integration for additional services (Home Assistant, etc.)
2. Implement automated Authentik provider configuration via Terraform
3. Add Grafana dashboards for authentication metrics
4. Set up alerts for authentication failures
5. Enable multi-factor authentication requirements per service

## Support

For issues or questions:
1. Review documentation in `docs/rke2/AUTHENTIK_INTEGRATION.md`
2. Check troubleshooting section in the integration guide
3. Review service-specific logs in Kubernetes
4. Check Authentik admin panel for authentication events

## Related Documentation

- [Authentik Documentation](https://docs.goauthentik.io/)
- [n8n OIDC Configuration](https://docs.n8n.io/hosting/configuration/environment-variables/oidc/)
- [ArgoCD OIDC Configuration](https://argo-cd.readthedocs.io/en/stable/operator-manual/user-management/#existing-oidc-provider)
- [External Secrets Operator](https://external-secrets.io/)
