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

### 4. Home Assistant
- **Type**: Forward Auth (Proxy Provider)
- **URL**: https://ha.hwcopeland.net
- **Authentication Method**: Authentik forward authentication
- **Files Created**:
  - `rke2/web-server/homeassistant/homeassistant-oidc-secret.yaml` - Outpost token secret
  - `rke2/web-server/httproute-web-apps.yaml` - Updated with forward auth
- **Group Access**: `Home Users`, `Infrastructure Team` groups

### 5. Homepage Dashboard
- **Type**: Forward Auth (Proxy Provider)
- **URL**: https://home.hwcopeland.net
- **Authentication Method**: Authentik forward authentication
- **Files Created**:
  - `rke2/web-server/homepage/homepage-oidc-secret.yaml` - Outpost token secret
  - `rke2/web-server/homepage/httproute.yaml` - Updated with forward auth
- **Group Access**: `Home Users`, `Infrastructure Team` groups

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

#### OIDC Services (n8n):
```
User → Service → Authentik Login → Group Check → Access Granted/Denied
```

#### Forward Auth Services (Longhorn, Hubble, Home Assistant, Homepage):
```
User → HTTPRoute → Authentik Proxy → Group Check → Backend Service
```

## Group-Based Access Control

### Recommended Groups

| Group Name | Purpose | Services |
|------------|---------|----------|
| `Grafana Admins` | Full Grafana admin access | Grafana |
| `n8n Users` | Access to automation platform | n8n |
| `Longhorn Admins` | Storage management | Longhorn |
| `Hubble Admins` | Network observability | Hubble |
| `Home Users` | Home automation and dashboard | Home Assistant, Homepage |
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

3. **`docs/rke2/IMPLEMENTATION_SUMMARY.md`**
   - Implementation overview
   - Architecture details
   - Testing checklist

## Deployment Prerequisites

Before deploying these changes:

1. **Bitwarden Items Required**:
   - `n8n-oidc-authentik` (Client ID and Secret)
   - `longhorn-authentik-token` (Outpost token)
   - `hubble-authentik-token` (Outpost token)
   - `homeassistant-authentik-token` (Outpost token)
   - `homepage-authentik-token` (Outpost token)

2. **Authentik Configuration**:
   - OAuth providers created for n8n
   - Proxy providers created for Longhorn, Hubble, Home Assistant, and Homepage
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
- [ ] Home Assistant redirects to Authentik for authentication
- [ ] Home Assistant grants access after authentication
- [ ] Homepage redirects to Authentik for authentication
- [ ] Homepage grants access after authentication
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

1. **Forward Auth Services**: Require Authentik outpost to be running
2. **Bitwarden Dependency**: All services depend on Bitwarden being accessible
3. **Initial Setup**: Requires manual configuration in Authentik UI

## Future Enhancements

1. Add Authentik integration for additional services
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
- [External Secrets Operator](https://external-secrets.io/)
