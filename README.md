# Infrastructure as Code (IaC)

## Overview

This repo contains all needed to deploy my homelab setup.

## Documentation

- [Changelog](docs/CHANGELOG.md) - Project changelog
- [Ansible Documentation](docs/ansible/TODO.md) - Ansible playbooks and roles
- [RKE2 Documentation](docs/rke2/README.md) - Kubernetes deployment information

## Project Structure

- `ansible/` - Ansible playbooks and roles for infrastructure provisioning
- `rke2/` - Kubernetes manifests and Helm charts for RKE2 deployment
- `docs/` - Documentation files

## Sources

The main inspiration for this comes from https://github.com/chkpwd/iac. Brian has helped me a great deal in understanding the concepts provided in this repo. So to him a great deal of credit is owed. I have used his repo as a reference for moving my homelab to k8s.
