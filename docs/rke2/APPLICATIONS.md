# Application Deployment Guide

This guide covers deploying and managing applications in the RKE2 Kubernetes cluster.

## Overview

Applications are organized by namespace and purpose. Each namespace contains manifests, Helm values files, or both.

## Deployment Workflow

### Using Kubectl

For manifest-based deployments:

```bash
# Deploy all resources in a namespace
kubectl apply -f rke2/namespace-name/

# Deploy a specific resource
kubectl apply -f rke2/namespace-name/resource.yaml
```

### Using Helm

For Helm-based deployments:

```bash
# Add Helm repository (if needed)
helm repo add repo-name https://charts.example.com
helm repo update

# Install with custom values
helm install release-name chart-name -f rke2/namespace-name/values.yaml

# Upgrade existing release
helm upgrade release-name chart-name -f rke2/namespace-name/values.yaml
```

### Using ArgoCD (GitOps)

ArgoCD applications are defined in `rke2/argocd/`:

```bash
# Deploy ArgoCD application
kubectl apply -f rke2/argocd/app-name.yaml

# ArgoCD will automatically sync from Git
```

## Core Infrastructure

### cert-manager (kube-system)

Certificate management for TLS/SSL certificates.

**Location**: `rke2/kube-system/cert-manager/`

**Deployment**:
```bash
# Install cert-manager (if not already installed via Helm)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.x.x/cert-manager.yaml

# Deploy issuers and certificates
kubectl apply -f rke2/kube-system/cert-manager/
```

**Resources**:
- `issuer.yml`: Certificate issuers (Let's Encrypt, etc.)
- `wildcard-cert.yml`: Wildcard certificates for domains

### Ingress (kube-system)

NGINX Ingress Controller for routing external traffic.

**Location**: `rke2/kube-system/ingress/`

**Configuration**:
- `internal.yaml`: Internal network ingress
- `rke2-ingress-nginx-config.yaml`: NGINX configuration

### MetalLB (metallb-system)

Load balancer for bare-metal Kubernetes clusters.

**Location**: `rke2/metallb-system/`

**Deployment**:
```bash
kubectl apply -f rke2/metallb-system/
```

**Configuration**:
- `internal.yaml`: Internal IP pool
- `external.yaml`: External IP pool

### Longhorn (longhorn-system)

Distributed block storage for Kubernetes.

**Location**: `rke2/longhorn-system/`

**Resources**:
- `longhorn-ui-svc.yaml`: Longhorn UI service
- `nfs-pv.yaml`: NFS persistent volumes

**Access UI**:
```bash
kubectl port-forward -n longhorn-system svc/longhorn-frontend 8080:80
# Visit http://localhost:8080
```

## Monitoring & Observability

### Monitoring Stack (monitor)

Prometheus and Grafana for metrics and visualization.

**Location**: `rke2/monitor/`

**Deployment**:
```bash
# Using Helm with custom values
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  -f rke2/monitor/values.yaml \
  -n monitor --create-namespace
```

**Resources**:
- `values.yaml`: Helm chart values
- `grafana-ingress.yaml`: Grafana ingress configuration
- `external-secret.yaml`: External secrets for sensitive data
- `mikrotik/`: Mikrotik router exporter

**Access Grafana**:
- Via ingress: Check `grafana-ingress.yaml` for hostname
- Port-forward: `kubectl port-forward -n monitor svc/grafana 3000:80`

## Media Services

### Plex (plex-system)

Plex media server.

**Location**: `rke2/plex-system/`

**Deployment**:
```bash
helm repo add plex https://raw.githubusercontent.com/plexinc/pms-docker/gh-pages
helm install plex plex/plex-media-server \
  -f rke2/plex-system/values.yaml \
  -n plex-system --create-namespace

# Deploy ingress
kubectl apply -f rke2/plex-system/plex-ingress.yaml
```

## Web Applications

### Web Server (web-server)

Web applications and services.

**Location**: `rke2/web-server/`

#### Excalidraw

Collaborative whiteboard tool.

```bash
kubectl apply -f rke2/web-server/excalidraw/excalidraw.yaml
```

#### Home Assistant

Home automation platform.

```bash
kubectl apply -f rke2/web-server/homeassistant/
```

**Resources**:
- `deployment.yaml`: Home Assistant deployment
- `ha-pvc.yaml`: Persistent volume claim for data

#### info.hwcopeland.net

Personal website (Next.js application).

**Location**: `rke2/web-server/info.hwcopeland.net/`

See the [application README](../../rke2/web-server/info.hwcopeland.net/README.md) for development details.

**Deployment**:
```bash
# Build and deploy using manifests in k8s/ directory
kubectl apply -f rke2/web-server/info.hwcopeland.net/k8s/
```

## Game Servers

### Counter-Strike 2 (game-server)

**Location**: `rke2/game-server/cs2/`

**Components**:
- `cs2-server/`: Game server deployment
- `cs2-database/`: Database for server stats/plugins
- `custom_files/`: Custom maps, plugins, and configurations

### Minecraft (game-server)

**Location**: `rke2/game-server/mc/`

**Deployments**:
- `atm9skies-deployment.yaml`: All The Mods 9 Sky modpack server
- `mc-router-values.yaml`: Minecraft router for multiple servers

## Secrets Management

### External Secrets (external-secrets)

Integration with external secret stores.

**Location**: `rke2/external-secrets/`

**Components**:

#### External Secrets Operator
```bash
kubectl apply -f rke2/external-secrets/external-secrets-operator/
```

#### Vaultwarden (Bitwarden)
Password manager and secret store:
```bash
kubectl apply -f rke2/external-secrets/vaultwarden/
```

#### Bitwarden CLI
Command-line interface for secret retrieval:
```bash
kubectl apply -f rke2/external-secrets/bitwarden-cli/
```

## GitOps with ArgoCD

### ArgoCD Setup

**Location**: `rke2/argocd/`

**Access ArgoCD UI**:
```bash
# Get admin password
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d

# Port-forward to access UI
kubectl port-forward svc/argocd-server -n argocd 8080:443
# Visit https://localhost:8080
```

**Deploy Application**:
```bash
kubectl apply -f rke2/argocd/hwcopeland-web-app.yaml
```

## Namespace Management

### Creating a New Namespace

```bash
kubectl create namespace new-namespace
```

### Viewing Resources by Namespace

```bash
# List all pods in a namespace
kubectl get pods -n namespace-name

# Get all resources
kubectl get all -n namespace-name

# Describe a specific resource
kubectl describe deployment deployment-name -n namespace-name
```

### Deleting a Namespace

```bash
kubectl delete namespace namespace-name
# Warning: This deletes all resources in the namespace
```

## Troubleshooting

### Pod Not Starting

```bash
# Check pod status
kubectl get pods -n namespace-name

# View pod logs
kubectl logs pod-name -n namespace-name

# Describe pod for events
kubectl describe pod pod-name -n namespace-name
```

### Service Not Accessible

```bash
# Check service endpoints
kubectl get endpoints service-name -n namespace-name

# Test service internally
kubectl run -it --rm debug --image=busybox --restart=Never -- wget -O- http://service-name.namespace-name.svc.cluster.local
```

### Persistent Volume Issues

```bash
# Check PV and PVC status
kubectl get pv
kubectl get pvc -n namespace-name

# Describe for details
kubectl describe pvc pvc-name -n namespace-name
```

### Ingress Not Working

```bash
# Check ingress status
kubectl get ingress -n namespace-name

# Check ingress controller logs
kubectl logs -n kube-system deployment/rke2-ingress-nginx-controller

# Verify DNS resolution
nslookup your-domain.com
```

## Best Practices

1. **Namespace Isolation**: Keep different application types in separate namespaces
2. **Resource Limits**: Define CPU and memory limits in deployments
3. **Health Checks**: Configure liveness and readiness probes
4. **GitOps**: Use ArgoCD for declarative, version-controlled deployments
5. **Secrets**: Never commit secrets to Git; use External Secrets or Sealed Secrets
6. **Backups**: Regularly backup persistent volumes and configurations
7. **Monitoring**: Enable Prometheus metrics for all applications
8. **Updates**: Keep applications and Helm charts up to date

## See Also

- [RKE2 Overview](README.md) - Namespace and directory structure
- [Getting Started](../GETTING_STARTED.md) - Initial setup guide
- [Ansible Documentation](../ansible/README.md) - Infrastructure provisioning
