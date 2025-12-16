# ArgoCD OIDC Configuration with Authentik

This directory contains configuration for ArgoCD OIDC integration with Authentik.

## Setup Instructions

### 1. Create OIDC Provider in Authentik

1. Log into Authentik at `https://auth.hwcopeland.net`
2. Navigate to Applications → Providers → Create
3. Select "OAuth2/OpenID Provider"
4. Configure:
   - Name: `ArgoCD`
   - Client Type: Confidential
   - Redirect URIs: `https://argocd.hwcopeland.net/auth/callback`
   - Scopes: `openid`, `profile`, `email`, `groups`
   - Subject Mode: Based on User's Email
5. Save and note the Client ID and Client Secret

### 2. Store Credentials in Bitwarden

Create a new Login item in Bitwarden:
- Name: `argocd-oidc-authentik`
- Username: Client ID from Authentik
- Password: Client Secret from Authentik

### 3. Apply External Secret

```bash
kubectl apply -f argocd-oidc-secret.yaml
```

### 4. Configure ArgoCD ConfigMap

Update the ArgoCD ConfigMap to enable OIDC:

```bash
kubectl edit configmap argocd-cm -n argocd
```

Add the following configuration:

```yaml
data:
  url: https://argocd.hwcopeland.net
  oidc.config: |
    name: Authentik
    issuer: https://auth.hwcopeland.net/application/o/argocd/
    clientID: $argocd-oidc-secret:client-id
    clientSecret: $argocd-oidc-secret:client-secret
    requestedScopes:
      - openid
      - profile
      - email
      - groups
    requestedIDTokenClaims:
      groups:
        essential: true
```

### 5. Configure RBAC (Optional)

Update ArgoCD RBAC ConfigMap for group-based access:

```bash
kubectl edit configmap argocd-rbac-cm -n argocd
```

Add the following:

```yaml
data:
  policy.csv: |
    g, ArgoCD Admins, role:admin
    g, Infrastructure Team, role:readonly
  policy.default: role:readonly
  scopes: '[groups, email]'
```

### 6. Restart ArgoCD Server

```bash
kubectl rollout restart deployment argocd-server -n argocd
```

## Testing

1. Navigate to `https://argocd.hwcopeland.net`
2. Click "Log in via Authentik"
3. Authenticate with Authentik credentials
4. Verify access based on group membership

## Troubleshooting

### Check External Secret Status

```bash
kubectl get externalsecret -n argocd argocd-oidc-secret
kubectl describe externalsecret -n argocd argocd-oidc-secret
```

### View ArgoCD Logs

```bash
kubectl logs -n argocd deployment/argocd-server -f
```

### Common Issues

- **"Invalid client credentials"**: Verify client ID and secret in Bitwarden match Authentik
- **"Redirect URI mismatch"**: Ensure redirect URI in Authentik includes `/auth/callback`
- **Users have no permissions**: Configure RBAC policies for groups

## References

- [ArgoCD OIDC Configuration](https://argo-cd.readthedocs.io/en/stable/operator-manual/user-management/#existing-oidc-provider)
- [Authentik OAuth2 Provider](https://docs.goauthentik.io/docs/providers/oauth2/)
