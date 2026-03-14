# RKE2 Cluster Bootstrap Guide

## Prerequisites

- Ansible installed on your local machine
- SSH access to all control plane nodes
- Sudo privileges on target nodes

## 1. Generate Cluster Token

Generate a secure token and export it as an environment variable:

```bash
export RKE2_TOKEN=$(openssl rand -hex 32)
echo "Save this token: $RKE2_TOKEN"
```

> **Important**: Save this token securely. You'll need it for adding nodes to the cluster later.

## 2. Bootstrap First Control Plane Node

The first node initializes the cluster and becomes the initial etcd member:

```bash
ansible-playbook playbooks/rke2.yaml -l ctlpln1 --ask-become \
  -e rke2_token=$RKE2_TOKEN \
  -e rke2_bootstrap=true
```

Wait for the playbook to complete and verify the API server is ready.

## 3. Join Additional Control Plane Nodes

Once the first node is ready, join the remaining control plane nodes:

```bash
ansible-playbook playbooks/rke2.yaml -l 'ctlpln2,ctlpln3' --ask-become \
  -e rke2_token=$RKE2_TOKEN
```

These nodes will join via the HA VIP (10.41.0.100:9345).

## 4. Verify Cluster Health

SSH into any control plane node and check the cluster:

```bash
sudo /var/lib/rancher/rke2/bin/kubectl --kubeconfig /etc/rancher/rke2/rke2.yaml get nodes
```

Expected output:
```
NAME       STATUS   ROLES                       AGE   VERSION
ctlpln1    Ready    control-plane,etcd,master   5m    v1.xx.x+rke2r1
ctlpln2    Ready    control-plane,etcd,master   2m    v1.xx.x+rke2r1
ctlpln3    Ready    control-plane,etcd,master   2m    v1.xx.x+rke2r1
```

## 5. Join Worker Nodes (Optional)

Add worker nodes to the inventory, then run:

```bash
ansible-playbook playbooks/rke2.yaml -l workers --ask-become \
  -e rke2_token=$RKE2_TOKEN
```

## Architecture

```
                    ┌─────────────────┐
                    │   VIP: 10.41.0.100   │
                    │   (keepalived)  │
                    └────────┬────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
        ▼                    ▼                    ▼
┌───────────────┐   ┌───────────────┐   ┌───────────────┐
│   ctlpln1     │   │   ctlpln2     │   │   ctlpln3     │
│  10.41.0.1    │   │  10.41.0.2    │   │  10.41.0.3    │
│  rke2-server  │   │  rke2-server  │   │  rke2-server  │
│  etcd member  │   │  etcd member  │   │  etcd member  │
└───────────────┘   └───────────────┘   └───────────────┘
```

## Troubleshooting

### Check RKE2 service status
```bash
sudo systemctl status rke2-server
```

### View RKE2 logs
```bash
sudo journalctl -u rke2-server -f
```

### Check keepalived VIP
```bash
ip addr show | grep 10.41.0.100
```

### Verify etcd cluster health
```bash
sudo /var/lib/rancher/rke2/bin/etcdctl \
  --cacert /var/lib/rancher/rke2/server/tls/etcd/server-ca.crt \
  --cert /var/lib/rancher/rke2/server/tls/etcd/server-client.crt \
  --key /var/lib/rancher/rke2/server/tls/etcd/server-client.key \
  endpoint health --cluster
```
