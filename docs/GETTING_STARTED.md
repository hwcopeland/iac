# Getting Started

This guide will help you get started with deploying and managing the homelab infrastructure using this repository.

## Prerequisites

### Software Requirements

- **Ansible**: Version 2.9 or higher
  - Install: `pip install ansible`
- **kubectl**: Kubernetes command-line tool
  - Install: See [Kubernetes documentation](https://kubernetes.io/docs/tasks/tools/)
- **Helm**: Package manager for Kubernetes (optional but recommended)
  - Install: See [Helm documentation](https://helm.sh/docs/intro/install/)

### Infrastructure Requirements

- One or more Linux servers (physical or virtual)
- SSH access to all target servers
- User with sudo privileges on target servers
- Network connectivity between nodes

## Quick Start

### 1. Clone the Repository

```bash
git clone https://github.com/hwcopeland/iac.git
cd iac
```

### 2. Configure Inventory

Edit the Ansible inventory file to match your infrastructure:

```bash
vim ansible/inventory/all.yml
```

Update the following:
- Host names and IP addresses
- SSH connection details
- Node types (server/agent)
- RKE2 node token (will be generated during first server setup)

### 3. Set Up Infrastructure with Ansible

#### Install Common Dependencies

```bash
cd ansible
ansible-playbook -i inventory/all.yml playbooks/k8s-common.yml
```

#### Deploy RKE2 Kubernetes Cluster

```bash
ansible-playbook -i inventory/all.yml playbooks/kubernetes.yml
```

This will:
- Install RKE2 on designated server node(s)
- Install and configure RKE2 agents on worker nodes
- Set up kubectl access

### 4. Verify Kubernetes Cluster

```bash
# Copy kubeconfig from server
export KUBECONFIG=/etc/rancher/rke2/rke2.yaml

# Check cluster status
kubectl get nodes
kubectl get pods -A
```

### 5. Deploy Applications

Once the cluster is running, you can deploy applications from the `rke2/` directory:

```bash
# Example: Deploy cert-manager
kubectl apply -f rke2/kube-system/cert-manager/

# Example: Deploy monitoring stack
helm install kube-prometheus-stack -f rke2/monitor/values.yaml
```

## Next Steps

- Review [Ansible Documentation](ansible/TODO.md) for configuration details
- Check [RKE2 Documentation](rke2/README.md) for application deployment guides
- See [CHANGELOG](CHANGELOG.md) for recent updates and TODO items

## Troubleshooting

### Common Issues

**Issue**: Ansible playbook fails with connection error
- **Solution**: Verify SSH access and credentials in inventory file

**Issue**: RKE2 server fails to start
- **Solution**: Check system logs with `journalctl -u rke2-server -f`

**Issue**: Nodes not joining cluster
- **Solution**: Verify `rke2_node_token` is correctly set in inventory

For more help, check the individual component documentation in the `docs/` directory.
