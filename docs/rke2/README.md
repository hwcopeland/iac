# RKE2 Documentation

## Overview

I am running the Rancher Kubernetes Engine version 2 for my k8s distro. Most deployments use helm charts, however some deployments just use manifests.

Documentation: https://docs.rke2.io/

## Namespace Organization

Services are broken down by namespace:

- `web-server` - Web servers and web applications
- `kube-system` - Core Kubernetes system components
- `longhorn-system` - Longhorn storage system
- `metallb-system` - MetalLB load balancer
- `monitor` - Monitoring stack (Grafana, Prometheus, etc.)
- `plex-system` - Plex media server
- `game-server` - Game servers (CS2, Minecraft)
- `external-secrets` - External secrets management

## Directory Structure

- `argocd/` - ArgoCD GitOps configurations
- `external-secrets/` - External secrets operator and related services
- `game-server/` - Game server deployments (CS2, Minecraft)
- `kube-system/` - Core Kubernetes components (cert-manager, ingress, etc.)
- `longhorn-system/` - Longhorn storage configurations
- `metallb-system/` - MetalLB load balancer configurations
- `monitor/` - Monitoring stack (Grafana, Prometheus)
- `plex-system/` - Plex media server
- `web-server/` - Web applications (Excalidraw, Home Assistant, personal sites)
