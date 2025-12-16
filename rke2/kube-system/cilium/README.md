# Cilium CNI with Gateway API Configuration

## Current State (December 16, 2025)

### Overview
Successfully migrated from Canal CNI + NGINX Ingress to Cilium CNI + Gateway API.

- **CNI**: Cilium v1.18.0
- **Gateway API**: v1.1.0 CRDs
- **Gateway IP**: 10.44.0.1 (HTTPS on port 443, HTTP on port 80)
- **Status**: ✅ Production - All services operational

### Architecture

```
Internet → Cloudflare (proxied) → 10.44.0.1 → Cilium Gateway → HTTPRoutes → Services
```

### Active Components

1. **Cilium CNI** (`rke2-cilium-config.yaml`)
   - Version: 1.18.0
   - kube-proxy replacement: enabled
   - Gateway API controller: enabled
   - Ingress controller: enabled (for Gateway API)
   - Deployed via RKE2 HelmChart CRD

2. **Gateway** (`gateway.yaml`)
   - Name: `hwcopeland-gateway`
   - Namespace: `kube-system`
   - GatewayClass: `cilium` (auto-created by Cilium)
   - LoadBalancer IP: 10.44.0.1
   - Listeners:
     - HTTP (port 80) - All namespaces
     - HTTPS (port 443) - TLS termination with wildcard cert
   - Certificate: `cf-wildcard-cert-secret` (managed by cert-manager)

3. **HTTPRoutes** (6 total across 4 namespaces)
   - `n8n` (agent-system) → n8n:5678
   - `vaultwarden` (external-secrets) → vault-warden:80
   - `grafana` (monitor) → kube-prometheus-stack-grafana:3000
   - `plex` (plex-system) → plex-plex-media-server:32400
   - `homeassistant` + `hwcopeland-web` (web-server) → ha-service:80, hwcopeland-web:3000

### Migration History

#### Phase 1: Cilium CNI Migration (December 2025)
- Migrated from Canal (Calico+Flannel) to Cilium 1.16.5
- Initial attempt failed due to CNI cleanup script wiping `/opt/cni/bin`
- Cluster recovered via host reboot and Cilium redeployment
- Enabled kube-proxy replacement

#### Phase 2: Gateway API Implementation Attempt (Cilium 1.16)
- Installed Gateway API v1.1.0 CRDs
- Gateway controller failed to start: CRD version incompatibility
- Cilium 1.16 expected `grpcroutes.gateway.networking.k8s.io/v1` but got `v1beta1`

#### Phase 3: Cilium Upgrade to 1.18.0
- Reason: Better Gateway API v1.1.0 support
- Manually created GatewayClass conflicted with Helm upgrade
- Deleted all Gateway resources and retried
- Upgrade successful - Gateway API controller initialized with TLSRoute support

#### Phase 4: Gateway API Deployment
- Created Gateway with HTTP/HTTPS listeners
- Created 6 HTTPRoutes with correct backend service names
- **Key Finding**: HTTPRoutes only attached to HTTPS listener (sectionName: https in manifests)
- Initial testing on HTTP port 80 failed (404)
- Testing on HTTPS port 443 successful (200 OK)

#### Phase 5: LoadBalancer IP Stabilization
- Gateway initially assigned 10.44.0.13
- Attempted to change to 10.44.0.1 via spec.addresses (failed - "StaticAddress can't be used")
- Attempted LoadBalancer service patching (service deleted during troubleshooting)
- Gateway recreated and assigned 10.44.0.1
- Service: `cilium-ingress` with externalIPs configured

#### Phase 6: Legacy Resource Cleanup
- Deleted all NGINX Ingress resources (6 Ingress objects)
- Scaled down NGINX Ingress Controller to 0 replicas
- Deleted NGINX LoadBalancer service

### Files in this Directory

- **`rke2-cilium-config.yaml`**: Cilium Helm configuration (deployed via RKE2)
- **`gateway.yaml`**: Cilium Gateway definition (single entry point)
- **`README.md`**: This file

### HTTPRoute Configuration

All HTTPRoutes follow this pattern:
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: <service-name>
  namespace: <app-namespace>
  annotations:
    cloudflare-operator.io/content: "<hostname>"
    cloudflare-operator.io/type: "CNAME"
    cloudflare-operator.io/proxied: "true"
spec:
  parentRefs:
    - name: hwcopeland-gateway
      namespace: kube-system
      sectionName: https  # Routes only to HTTPS listener
  hostnames:
    - "<hostname>"
  rules:
    - backendRefs:
        - name: <service-name>
          port: <service-port>
```

### Integration Points

1. **cert-manager**: Manages TLS certificates via `cf-issuer` ClusterIssuer
2. **Cloudflare Operator**: Creates DNS records via HTTPRoute annotations
3. **Cilium**: Provides CNI, kube-proxy replacement, and Gateway API controller
4. **RKE2**: Manages Cilium deployment via HelmChart CRD

### Troubleshooting

**Gateway not responding:**
```bash
kubectl get gateway -n kube-system hwcopeland-gateway
kubectl describe gateway -n kube-system hwcopeland-gateway
kubectl get svc -n kube-system cilium-ingress
```

**HTTPRoute not routing:**
```bash
kubectl describe httproute -n <namespace> <name>
# Check: Accepted=True, ResolvedRefs=True
# Verify backend service exists and has endpoints
```

**Test connectivity:**
```bash
curl -I -H "Host: <hostname>" https://10.44.0.1 --insecure
```

### Known Issues

- HTTPRoutes only work on HTTPS (port 443), not HTTP (port 80)
  - Cause: All HTTPRoutes specify `sectionName: https`
  - Resolution: Routes intentionally HTTPS-only; Cloudflare forces HTTPS anyway

### Maintenance

**Update Cilium version:**
1. Edit `rke2-cilium-config.yaml` version field
2. `sudo cp rke2-cilium-config.yaml /var/lib/rancher/rke2/server/manifests/`
3. Monitor: `kubectl get helmchart -n kube-system cilium -w`

**Add new HTTPRoute:**
1. Create HTTPRoute manifest in appropriate namespace directory
2. Apply: `kubectl apply -f <httproute-file>.yaml`
3. Verify: `kubectl describe httproute -n <namespace> <name>`

**Change Gateway IP:**
- Not supported via spec.addresses
- Requires manual service patching or Cilium LB-IPAM configuration
- Current IP (10.44.0.1) is stable via externalIPs

### References

- Cilium Gateway API docs: https://docs.cilium.io/en/stable/network/servicemesh/gateway-api/
- Gateway API spec: https://gateway-api.sigs.k8s.io/
- Cilium version: 1.18.0
- Gateway API version: v1.1.0
