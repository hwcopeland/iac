# Ansible Setup Guide

This directory contains Ansible playbooks and roles for provisioning and configuring the homelab infrastructure.

## Directory Structure

```
ansible/
├── inventory/          # Inventory files defining hosts and variables
│   └── all.yml        # Main inventory file
├── playbooks/         # Ansible playbooks
│   ├── k8s-common.yml    # Common Kubernetes setup
│   └── kubernetes.yml    # RKE2 cluster deployment
├── roles/             # Ansible roles
│   ├── k8s-common/       # Common Kubernetes dependencies
│   ├── rke2-agent/       # RKE2 agent configuration
│   ├── rke2-common/      # Common RKE2 setup
│   └── rke2-server/      # RKE2 server configuration
└── ansible.cfg        # Ansible configuration
```

## Inventory Configuration

The inventory file (`inventory/all.yml`) defines your infrastructure:

### Example Configuration

```yaml
autodock:
  children:
    k8s_hosts:
      vars:
        K8S_ANSIBLE_USER: k8s_user
      hosts:
        server-node:
          ansible_host: 192.168.1.100
          ansible_user: "{{K8S_ANSIBLE_USER}}"
          type: server
          rke2_node_token: 'your-token-here'
        
        worker-node-1:
          ansible_host: 192.168.1.101
          ansible_user: "{{K8S_ANSIBLE_USER}}"
          type: agent
```

### Key Variables

- `K8S_ANSIBLE_USER`: User account for Kubernetes operations (must exist on target systems)
- `ansible_host`: IP address or hostname of the target server
- `ansible_user`: SSH user for Ansible connection
- `type`: Node type - either `server` (control plane) or `agent` (worker)
- `rke2_node_token`: Shared secret for cluster authentication

## Playbooks

### k8s-common.yml

Prepares all nodes with common dependencies and configurations.

**Purpose**: 
- Install system dependencies
- Configure system settings
- Set up user accounts
- Prepare environment for Kubernetes

**Usage**:
```bash
ansible-playbook -i inventory/all.yml playbooks/k8s-common.yml
```

### kubernetes.yml

Deploys RKE2 Kubernetes cluster.

**Purpose**:
- Install RKE2 on server nodes
- Install RKE2 on agent nodes
- Configure cluster networking
- Set up kubectl access

**Usage**:
```bash
ansible-playbook -i inventory/all.yml playbooks/kubernetes.yml
```

## Roles

### k8s-common

Common setup tasks for all Kubernetes nodes.

**Responsibilities**:
- System package installation
- Kernel parameter configuration
- User and group setup
- Directory structure creation

### rke2-server

RKE2 control plane server setup.

**Responsibilities**:
- RKE2 server installation
- Server configuration
- Generate cluster token
- Initialize cluster

**Configuration Files**:
- `/etc/rancher/rke2/config.yaml`: Main RKE2 server configuration

### rke2-agent

RKE2 worker node setup.

**Responsibilities**:
- RKE2 agent installation
- Agent configuration
- Join cluster using token
- Node labeling

**Configuration Files**:
- `/etc/rancher/rke2/config.yaml`: Main RKE2 agent configuration

### rke2-common

Common RKE2 tasks for both server and agent nodes.

**Responsibilities**:
- Download RKE2 binaries
- Set up systemd services
- Configure firewall rules
- Install kubectl

## Running Playbooks

### Full Deployment

Deploy everything from scratch:

```bash
cd ansible

# Step 1: Common setup
ansible-playbook -i inventory/all.yml playbooks/k8s-common.yml

# Step 2: Deploy Kubernetes
ansible-playbook -i inventory/all.yml playbooks/kubernetes.yml
```

### Targeted Execution

Run on specific hosts:

```bash
# Only on server nodes
ansible-playbook -i inventory/all.yml playbooks/kubernetes.yml --limit server-node

# Only on agent nodes
ansible-playbook -i inventory/all.yml playbooks/kubernetes.yml --limit worker-node-1
```

### Check Mode (Dry Run)

Preview changes without applying them:

```bash
ansible-playbook -i inventory/all.yml playbooks/kubernetes.yml --check
```

### Verbose Output

Get detailed execution information:

```bash
ansible-playbook -i inventory/all.yml playbooks/kubernetes.yml -vvv
```

## User Setup

The `k8s_user` must exist on all target systems. You can create it manually:

```bash
# On each target node
sudo useradd -m -s /bin/bash k8s_user
sudo usermod -aG sudo k8s_user
```

Or automate this in the playbooks (see TODO items).

## Troubleshooting

### Connection Issues

**Problem**: Cannot connect to hosts
```
UNREACHABLE! => {"changed": false, "msg": "Failed to connect to the host"}
```

**Solutions**:
- Verify SSH access: `ssh user@host`
- Check inventory IP addresses
- Ensure SSH keys are properly configured
- Verify firewall allows SSH (port 22)

### Permission Issues

**Problem**: Privilege escalation fails
```
FAILED! => {"msg": "Missing sudo password"}
```

**Solutions**:
- Use `--ask-become-pass` flag
- Configure passwordless sudo for the user
- Check user has sudo privileges

### Role Not Found

**Problem**: Role cannot be found
```
ERROR! the role 'role-name' was not found
```

**Solutions**:
- Verify you're running from the `ansible/` directory
- Check role exists in `roles/` directory
- Ensure playbook path is correct

## Best Practices

1. **Version Control**: Always commit inventory changes to track infrastructure evolution
2. **Secrets Management**: Use Ansible Vault for sensitive data (tokens, passwords)
3. **Idempotency**: Playbooks can be run multiple times safely
4. **Testing**: Use `--check` mode before applying changes to production
5. **Backups**: Back up `/etc/rancher/rke2/` configuration before major updates

## See Also

- [TODO List](TODO.md) - Planned improvements and known issues
- [RKE2 Documentation](../rke2/README.md) - Application deployment guide
- [Getting Started](../GETTING_STARTED.md) - Quick start guide
