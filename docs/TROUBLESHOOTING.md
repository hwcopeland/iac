# Troubleshooting Guide

Common issues and their solutions for the homelab infrastructure.

## Table of Contents

- [Ansible Issues](#ansible-issues)
- [RKE2 Cluster Issues](#rke2-cluster-issues)
- [Networking Issues](#networking-issues)
- [Storage Issues](#storage-issues)
- [Application Issues](#application-issues)
- [Performance Issues](#performance-issues)

## Ansible Issues

### SSH Connection Failures

**Symptom**: Ansible cannot connect to hosts
```
fatal: [host]: UNREACHABLE! => {"changed": false, "msg": "Failed to connect"}
```

**Solutions**:
1. Verify SSH access manually:
   ```bash
   ssh user@host
   ```
2. Check inventory file has correct IP addresses
3. Verify SSH keys are configured:
   ```bash
   ssh-copy-id user@host
   ```
4. Check firewall allows SSH:
   ```bash
   sudo ufw allow 22
   ```

### Privilege Escalation Errors

**Symptom**: Cannot become root/sudo
```
FAILED! => {"msg": "Missing sudo password"}
```

**Solutions**:
1. Add become password flag:
   ```bash
   ansible-playbook playbook.yml --ask-become-pass
   ```
2. Configure passwordless sudo:
   ```bash
   echo "username ALL=(ALL) NOPASSWD:ALL" | sudo tee /etc/sudoers.d/username
   ```

### Role Not Found

**Symptom**: Ansible cannot find roles
```
ERROR! the role 'role-name' was not found
```

**Solutions**:
1. Ensure running from correct directory:
   ```bash
   cd ansible/
   ```
2. Check `ansible.cfg` for correct roles path
3. Verify role exists in `roles/` directory

## RKE2 Cluster Issues

### Server Won't Start

**Symptom**: RKE2 server service fails to start

**Check logs**:
```bash
sudo journalctl -u rke2-server -f
```

**Common causes and solutions**:

1. **Port already in use**: Another process is using required ports
   ```bash
   # Check what's using port 6443
   sudo netstat -tulpn | grep 6443
   ```

2. **Insufficient resources**: Not enough CPU/memory
   ```bash
   # Check system resources
   free -h
   df -h
   ```

3. **Previous installation remnants**:
   ```bash
   # Clean up previous installation
   sudo /usr/local/bin/rke2-uninstall.sh
   # Reinstall using Ansible
   ```

### Agent Cannot Join Cluster

**Symptom**: Agent node doesn't appear in cluster
```bash
kubectl get nodes  # Agent missing
```

**Solutions**:

1. **Check node token**:
   - Ensure `rke2_node_token` matches server token
   - Get server token: `sudo cat /var/lib/rancher/rke2/server/node-token`

2. **Check connectivity to server**:
   ```bash
   # From agent node
   telnet server-ip 9345
   ```

3. **Check agent logs**:
   ```bash
   sudo journalctl -u rke2-agent -f
   ```

4. **Verify server URL in agent config**:
   ```bash
   cat /etc/rancher/rke2/config.yaml
   # Should have: server: https://server-ip:9345
   ```

### Kubectl Not Working

**Symptom**: kubectl commands fail
```
The connection to the server localhost:8080 was refused
```

**Solutions**:

1. **Set KUBECONFIG**:
   ```bash
   export KUBECONFIG=/etc/rancher/rke2/rke2.yaml
   ```

2. **Fix permissions (securely)**:
   ```bash
   sudo chmod 600 /etc/rancher/rke2/rke2.yaml

3. **Copy config to user directory**:
   ```bash
   mkdir -p ~/.kube
   sudo cp /etc/rancher/rke2/rke2.yaml ~/.kube/config
   sudo chown $USER:$USER ~/.kube/config
   ```

## Networking Issues

### Pods Cannot Communicate

**Symptom**: Pods cannot reach other pods or services

**Diagnosis**:
```bash
# Check CNI (Canal) is running
kubectl get pods -n kube-system | grep canal

# Check pod networking
kubectl exec -it pod-name -- ping other-pod-ip
```

**Solutions**:

1. **Restart Canal**:
   ```bash
   kubectl rollout restart daemonset/rke2-canal -n kube-system
   ```

2. **Check firewall rules**:
   ```bash
   # Ensure required ports are open
   sudo ufw allow 6443/tcp  # Kubernetes API
   sudo ufw allow 10250/tcp # Kubelet
   sudo ufw allow 2379:2380/tcp # etcd
   ```

### MetalLB Not Assigning IPs

**Symptom**: Services of type LoadBalancer stuck in `<pending>`

**Check MetalLB**:
```bash
kubectl get pods -n metallb-system
kubectl logs -n metallb-system deployment/controller
```

**Solutions**:

1. **Verify IP pool configuration**:
   ```bash
   kubectl get ipaddresspools -n metallb-system
   ```

2. **Check IP pool is not exhausted**:
   - Ensure IP range has available addresses
   - Review `rke2/metallb-system/internal.yaml` and `external.yaml`

3. **Restart MetalLB**:
   ```bash
   kubectl rollout restart deployment/controller -n metallb-system
   ```

### Ingress Not Accessible

**Symptom**: Cannot access services via ingress

**Diagnosis**:
```bash
# Check ingress resources
kubectl get ingress -A

# Check ingress controller
kubectl get pods -n kube-system | grep ingress
kubectl logs -n kube-system deployment/rke2-ingress-nginx-controller
```

**Solutions**:

1. **Verify DNS resolution**:
   ```bash
   nslookup your-domain.com
   ```

2. **Check ingress configuration**:
   ```bash
   kubectl describe ingress ingress-name -n namespace
   ```

3. **Verify TLS certificates**:
   ```bash
   kubectl get certificates -A
   kubectl describe certificate cert-name -n namespace
   ```

## Storage Issues

### PVC Stuck in Pending

**Symptom**: PersistentVolumeClaim remains in `Pending` state

**Check status**:
```bash
kubectl get pvc -A
kubectl describe pvc pvc-name -n namespace
```

**Solutions**:

1. **Check storage class**:
   ```bash
   kubectl get storageclass
   kubectl describe storageclass longhorn
   ```

2. **Check Longhorn status**:
   ```bash
   kubectl get pods -n longhorn-system
   ```

3. **Verify sufficient disk space**:
   ```bash
   df -h
   ```

### Longhorn Volume Issues

**Symptom**: Longhorn volumes degraded or failed

**Check Longhorn UI**:
```bash
kubectl port-forward -n longhorn-system svc/longhorn-frontend 8080:80
# Visit http://localhost:8080
```

**Solutions**:

1. **Check node disk space**:
   - Longhorn requires sufficient space on each node
   - Default location: `/var/lib/longhorn`

2. **Restart Longhorn pods**:
   ```bash
   kubectl rollout restart deployment -n longhorn-system
   ```

3. **Check volume replicas**:
   - In Longhorn UI, verify volume has healthy replicas
   - Minimum recommended: 3 replicas

## Application Issues

### Pod CrashLoopBackOff

**Symptom**: Pod repeatedly crashes and restarts

**Check logs**:
```bash
kubectl logs pod-name -n namespace
kubectl logs pod-name -n namespace --previous  # Previous instance logs
```

**Common causes**:

1. **Application error**: Fix application code/configuration
2. **Missing dependencies**: Check required secrets, configmaps
3. **Resource limits**: Pod OOM killed due to memory limits
   ```bash
   kubectl describe pod pod-name -n namespace | grep -A 5 "Last State"
   ```

### ImagePullBackOff

**Symptom**: Cannot pull container image

**Check**:
```bash
kubectl describe pod pod-name -n namespace
```

**Solutions**:

1. **Verify image name and tag**:
   - Check for typos in image name
   - Ensure tag exists

2. **Check image registry credentials**:
   ```bash
   kubectl get secrets -n namespace
   ```

3. **Test image pull manually**:
   ```bash
   docker pull image-name:tag
   ```

### Service Not Accessible

**Symptom**: Cannot connect to service

**Diagnosis**:
```bash
# Check service endpoints
kubectl get endpoints service-name -n namespace

# Check if pods are ready
kubectl get pods -n namespace -l app=label
```

**Solutions**:

1. **Verify selector labels match**:
   ```bash
   kubectl describe service service-name -n namespace
   kubectl get pods -n namespace --show-labels
   ```

2. **Test from within cluster**:
   ```bash
   kubectl run -it --rm debug --image=busybox --restart=Never -- \
     wget -O- http://service-name.namespace.svc.cluster.local:port
   ```

## Performance Issues

### High CPU Usage

**Check resource usage**:
```bash
kubectl top nodes
kubectl top pods -A
```

**Solutions**:

1. **Identify resource-intensive pods**:
   ```bash
   kubectl top pods -A --sort-by=cpu
   ```

2. **Set resource limits**:
   ```yaml
   resources:
     limits:
       cpu: 1000m
       memory: 1Gi
     requests:
       cpu: 100m
       memory: 128Mi
   ```

3. **Scale down replicas**:
   ```bash
   kubectl scale deployment deployment-name --replicas=1 -n namespace
   ```

### Disk Space Issues

**Check disk usage**:
```bash
df -h
du -sh /var/lib/rancher/*
du -sh /var/lib/longhorn/*
```

**Solutions**:

1. **Clean up old container images**:
   ```bash
   # On each node
   crictl rmi --prune
   ```

2. **Clean up old logs**:
   ```bash
   sudo journalctl --vacuum-time=7d
   ```

3. **Remove unused volumes**:
   ```bash
   kubectl delete pvc unused-pvc -n namespace
   ```

### Network Latency

**Test connectivity**:
```bash
# Between pods
kubectl exec -it pod1 -n namespace -- ping pod2-ip

# To external services
kubectl exec -it pod -n namespace -- ping 8.8.8.8
```

**Solutions**:

1. **Check MTU settings**: Ensure consistent MTU across network
2. **Review network policies**: Ensure policies aren't blocking traffic
3. **Check node network performance**: Run `iperf` tests between nodes

## Getting Help

If issues persist:

1. **Check logs systematically**:
   - Node level: `journalctl -u rke2-server` or `rke2-agent`
   - Pod level: `kubectl logs pod-name -n namespace`
   - Events: `kubectl get events -n namespace --sort-by='.lastTimestamp'`

2. **Review configurations**:
   - Compare with working examples
   - Check for typos in YAML files

3. **Search documentation**:
   - [RKE2 Docs](https://docs.rke2.io/)
   - [Kubernetes Docs](https://kubernetes.io/docs/)
   - Component-specific documentation

4. **Community support**:
   - RKE2 GitHub Issues
   - Kubernetes Slack
   - Stack Overflow

## See Also

- [Getting Started](GETTING_STARTED.md)
- [Ansible Guide](ansible/README.md)
- [Application Deployment](rke2/APPLICATIONS.md)
