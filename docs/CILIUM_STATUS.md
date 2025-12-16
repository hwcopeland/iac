# Cilium CNI + Gateway API - Current Status

**Last Updated**: December 16, 2025  
**Migration Status**: ✅ COMPLETE  
**Production Status**: ✅ OPERATIONAL

## Quick Summary

Successfully migrated from:
- **Old**: Canal CNI + NGINX Ingress
- **New**: Cilium 1.18.0 CNI + Gateway API v1.1.0

All services operational on **10.44.0.1** (HTTPS).

## Current Configuration

### Infrastructure
- **Cilium Version**: 1.18.0
- **Gateway API**: v1.1.0 CRDs
- **kube-proxy**: Replaced by Cilium
- **Gateway IP**: 10.44.0.1 (ports 80, 443)
- **TLS**: Wildcard cert managed by cert-manager

### Services (6 HTTPRoutes)
All routed through `hwcopeland-gateway` in `kube-system`:

| Service | Hostname | Namespace | Backend | Port |
|---------|----------|-----------|---------|------|
| n8n | n8n.hwcopeland.net | agent-system | n8n | 5678 |
| Vaultwarden | vault.hwcopeland.net | external-secrets | vault-warden | 80 |
| Grafana | grafana.hwcopeland.net | monitor | kube-prometheus-stack-grafana | 3000 |
| Plex | plex.hwcopeland.net | plex-system | plex-plex-media-server | 32400 |
| Home Assistant | ha.hwcopeland.net | web-server | ha-service | 80 |
| Web | info.hwcopeland.net | web-server | hwcopeland-web | 3000 |

## File Locations

### Cilium Configuration
- **Helm Config**: `iac/rke2/kube-system/cilium/rke2-cilium-config.yaml`
- **Gateway**: `iac/rke2/kube-system/cilium/gateway.yaml`
- **Documentation**: `iac/rke2/kube-system/cilium/README.md`

### HTTPRoute Manifests
- `iac/rke2/agent-system/httproute-n8n.yaml`
- `iac/rke2/external-secrets/httproute-vaultwarden.yaml`
- `iac/rke2/monitor/httproute-grafana.yaml`
- `iac/rke2/plex-system/httproute-plex.yaml`
- `iac/rke2/web-server/httproute-web-apps.yaml`

## Quick Commands

### Check Gateway Status
```bash
kubectl get gateway -n kube-system hwcopeland-gateway
kubectl get svc -n kube-system cilium-ingress
```

### List All HTTPRoutes
```bash
kubectl get httproute --all-namespaces
```

### Test Service
```bash
curl -I -H "Host: n8n.hwcopeland.net" https://10.44.0.1 --insecure
```

### Check Cilium Health
```bash
kubectl -n kube-system exec -it ds/cilium -- cilium status
```

## Integration

- **cert-manager**: Auto-renews TLS certs via `cf-issuer`
- **Cloudflare Operator**: Auto-creates DNS via HTTPRoute annotations
- **RKE2**: Manages Cilium via HelmChart CRD

## Migration Timeline

| Date | Phase | Status |
|------|-------|--------|
| Dec 16 | Canal → Cilium 1.16.5 | ⚠️ Failed (CNI cleanup issue) |
| Dec 16 | Cluster Recovery | ✅ Recovered via host reboot |
| Dec 16 | Cilium 1.16.5 Redeployed | ✅ Success |
| Dec 16 | Gateway API on 1.16.5 | ❌ Failed (CRD version mismatch) |
| Dec 16 | Cilium → 1.18.0 Upgrade | ✅ Success |
| Dec 16 | Gateway API Deployment | ✅ Success (HTTPS only) |
| Dec 16 | LoadBalancer IP → 10.44.0.1 | ✅ Success |
| Dec 16 | Legacy Cleanup (NGINX) | ✅ Complete |

## Known Limitations

- HTTPRoutes only work on HTTPS (port 443), not HTTP (port 80)
- This is intentional - all routes specify `sectionName: https`
- Cloudflare forces HTTPS anyway, so not an issue

## Troubleshooting

See detailed troubleshooting in:
- `iac/rke2/kube-system/cilium/README.md`
- `iac/docs/TROUBLESHOOTING.md`

## Validation

All services tested and operational:
```bash
# Test all services
for host in n8n.hwcopeland.net vault.hwcopeland.net grafana.hwcopeland.net plex.hwcopeland.net ha.hwcopeland.net info.hwcopeland.net; do
  echo "Testing $host..."
  curl -s -o /dev/null -w "%{http_code}\n" -H "Host: $host" https://10.44.0.1 --insecure
done
```

Expected: All return 200 (or service-specific codes like 302 for redirects)

## Next Steps

Migration is complete. Standard operations:

1. **Add new service**: Create HTTPRoute manifest, apply
2. **Update Cilium**: Edit `rke2-cilium-config.yaml`, copy to `/var/lib/rancher/rke2/server/manifests/`
3. **Monitor**: Use `cilium status`, `hubble observe`, or Prometheus/Grafana

---

For detailed migration history and technical details, see: `iac/rke2/kube-system/cilium/README.md`
