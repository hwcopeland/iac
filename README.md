# Infrastructure as Code (IaC)

## Overview

This repository contains everything needed to deploy and manage a homelab Kubernetes infrastructure using Ansible and RKE2.

## Quick Links

- **[Getting Started](docs/GETTING_STARTED.md)** - New to this project? Start here!
- **[Troubleshooting](docs/TROUBLESHOOTING.md)** - Having issues? Check common solutions
- **[Contributing](docs/CONTRIBUTING.md)** - Want to contribute? Read the guidelines

## Documentation

### Core Documentation
- **[Getting Started Guide](docs/GETTING_STARTED.md)** - Step-by-step setup instructions
- **[Changelog](docs/CHANGELOG.md)** - Project changelog and recent updates

### Infrastructure
- **[Ansible Setup Guide](docs/ansible/README.md)** - Comprehensive Ansible documentation
  - Inventory configuration
  - Playbook usage
  - Role documentation
  - [TODO List](docs/ansible/TODO.md) - Planned improvements
- **[RKE2 Overview](docs/rke2/README.md)** - Kubernetes cluster overview
  - Namespace organization
  - Directory structure
- **[Application Deployment Guide](docs/rke2/APPLICATIONS.md)** - Deploy and manage applications
  - Core infrastructure (cert-manager, ingress, MetalLB, Longhorn)
  - Monitoring and observability
  - Media services
  - Web applications
  - Game servers
  - Secrets management

### Operations
- **[Troubleshooting Guide](docs/TROUBLESHOOTING.md)** - Common issues and solutions
  - Ansible issues
  - RKE2 cluster issues
  - Networking problems
  - Storage problems
  - Application issues

### Contributing
- **[Contributing Guide](docs/CONTRIBUTING.md)** - How to contribute to this project

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

- **Automated Infrastructure**: Ansible playbooks for repeatable, idempotent deployments
- **RKE2 Kubernetes**: Lightweight, secure Kubernetes distribution
- **GitOps Ready**: ArgoCD integration for declarative deployments
- **Comprehensive Monitoring**: Prometheus and Grafana for observability
- **Persistent Storage**: Longhorn for distributed block storage
- **Load Balancing**: MetalLB for bare-metal load balancing
- **Secrets Management**: External Secrets Operator integration
- **SSL/TLS**: Automated certificate management with cert-manager

## Sources

The main inspiration for this comes from https://github.com/chkpwd/iac. Brian has helped me a great deal in understanding the concepts provided in this repo. So to him a great deal of credit is owed. I have used his repo as a reference for moving my homelab to k8s.
