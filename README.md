# Infrastructure as Code (IaC)

## Overview

This repository contains everything needed to deploy and manage a homelab Kubernetes infrastructure using Ansible and RKE2.

## Quick Links

- **[Getting Started](docs/GETTING_STARTED.md)** - New to this project? Start here!
- **[Contributing](docs/CONTRIBUTING.md)** - Want to contribute? Read the guidelines

### Infrastructure
- **[Ansible Setup Guide](docs/ansible/README.md)** - Comprehensive Ansible documentation
  - Inventory configuration
  - Playbook usage
  - Role documentation
- **[RKE2 Overview](docs/rke2/README.md)** - Kubernetes cluster overview
  - Namespace organization
  - Directory structure
- **[Application Deployment Guide](docs/rke2/APPLICATIONS.md)** - Deploy and manage applications
  - Core infrastructure (Cert-manager, Longhorn)
  - Monitoring and observability
  - Media services
  - Web applications
  - Game servers
  - Secrets management

## Project Structure

```
.
├── ansible/           # Ansible playbooks and roles
│   ├── inventory/    # Host inventory files
│   ├── playbooks/    # Ansible playbooks
│   └── roles/        # Ansible roles
├── rke2/             # Kubernetes manifests and Helm values
│   ├── argocd/       # GitOps configurations
│   ├── kube-system/  # Core K8s components
│   ├── monitor/      # Monitoring stack
│   ├── web-server/   # Web applications
│   └── ...           # Other namespaces
└── docs/             # Documentation
    ├── ansible/      # Ansible-specific docs
    └── rke2/         # RKE2-specific docs
```

## Features

- **Automated Infrastructure**: Ansible playbooks for os enviorment
- **RKE2 Kubernetes**: Rancher's Kubernetes Engine 2
  - Cilium: container networking & gateway controller
  - Longhorn: distributed block store
- **CI/CD**: Continuous Integration / Continuous Delivery
  - Flux: Automated cluster-repo reconcile (push-to-prod)
  - ARC: Automated container builds and testing
  - ZOT: Local OCI Image repos 
- **Centralized Authentication**: Authentik OAuth/OIDC for SSO across services
- **Comprehensive Monitoring**: Prometheus and Grafana for observability
- **Secrets Management**: External Secrets Operator with Bitwarden integration
- **SSL/TLS**: Automated certificate management with cert-manager

## Sources

The main inspiration for this comes from https://github.com/chkpwd/iac. Brian has helped me a great deal in understanding the concepts provided in this repo. So to him a great deal of credit is owed. I have used his repo as a reference for moving my homelab to k8s. Special shout outs to Acelink, Senk0 and other k8s friends of the r/Homelab discord for always providing interesting tools, etc.

## AI Disclosure & Usage

A significant portion of the code in this repository is AI-generated. While this has streamlined the development of my infrastructure, I recognize that it may not always align with traditional "best practices" or manual coding standards. This repository serves primarily as a personal lab environment for experimentation. As such, it is provided as-is and is not intended to be a definitive guide for production-grade software development.
