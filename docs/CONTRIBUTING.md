# Contributing Guide

Thank you for your interest in contributing to this homelab infrastructure project!

## Getting Started

1. **Fork the repository**
2. **Clone your fork**:
   ```bash
   git clone https://github.com/your-username/iac.git
   cd iac
   ```
3. **Create a feature branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```

## Making Changes

### Documentation

- All documentation lives in the `docs/` directory
- Use clear, concise language
- Include code examples where appropriate
- Update the main README if adding new sections

### Ansible Playbooks and Roles

- Follow [Ansible best practices](https://docs.ansible.com/ansible/latest/user_guide/playbooks_best_practices.html)
- Use meaningful variable names
- Document variables in role defaults/vars
- Test playbooks before submitting:
  ```bash
  ansible-playbook --syntax-check playbook.yml
  ansible-playbook --check playbook.yml  # Dry run
  ```

### Kubernetes Manifests

- Follow Kubernetes manifest conventions
- Include namespace in metadata
- Set resource limits and requests
- Use ConfigMaps for configuration
- Use Secrets for sensitive data (never commit secrets!)
- Validate YAML syntax:
  ```bash
  kubectl apply --dry-run=client -f manifest.yaml
  ```

### Helm Values

- Document all customized values with comments
- Keep values organized by component
- Use consistent formatting
- Test Helm charts:
  ```bash
  helm lint -f values.yaml
  helm template release-name chart-name -f values.yaml
  ```

## Code Style

### YAML Files

- Use 2 spaces for indentation
- Use lowercase for keys
- Quote string values when necessary
- Keep lines under 120 characters when possible

Example:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
  namespace: default
spec:
  selector:
    app: my-app
  ports:
    - protocol: TCP
      port: 80
      targetPort: 8080
```

### Markdown Files

- Use ATX-style headers (`#` for h1, `##` for h2, etc.)
- Include a blank line before and after headers
- Use fenced code blocks with language identifiers
- Keep lines reasonably short for readability

## Testing Your Changes

### Ansible Changes

1. **Syntax check**:
   ```bash
   ansible-playbook --syntax-check playbooks/playbook.yml
   ```

2. **Dry run**:
   ```bash
   ansible-playbook --check -i inventory/all.yml playbooks/playbook.yml
   ```

3. **Test in staging environment** (if available):
   ```bash
   ansible-playbook -i inventory/staging.yml playbooks/playbook.yml
   ```

### Kubernetes Changes

1. **Validate YAML**:
   ```bash
   kubectl apply --dry-run=client -f rke2/namespace/manifest.yaml
   ```

2. **Test in isolated namespace**:
   ```bash
   kubectl create namespace test
   kubectl apply -f rke2/namespace/manifest.yaml -n test
   # Verify it works
   kubectl delete namespace test
   ```

3. **Check resource limits**:
   - Ensure pods have appropriate CPU/memory limits
   - Test under load if possible

## Commit Guidelines

### Commit Messages

Follow the [Conventional Commits](https://www.conventionalcommits.org/) format:

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types**:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, no logic change)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

**Examples**:
```
feat(ansible): add role for automatic k8s_user creation

docs(rke2): update monitoring stack deployment guide

fix(metallb): correct IP pool configuration for external network
```

### Commit Best Practices

- Make atomic commits (one logical change per commit)
- Write clear, descriptive commit messages
- Reference issues when applicable: `fixes #123`
- Keep commits focused and small

## Pull Request Process

1. **Update documentation** if needed
2. **Test your changes** thoroughly
3. **Push to your fork**:
   ```bash
   git push origin feature/your-feature-name
   ```
4. **Create Pull Request**:
   - Use a clear, descriptive title
   - Describe what changes you made and why
   - Reference related issues
   - Include testing steps

### PR Description Template

```markdown
## Description
Brief description of changes

## Motivation
Why are these changes needed?

## Changes Made
- Change 1
- Change 2
- Change 3

## Testing
How were these changes tested?

## Checklist
- [ ] Code follows project style guidelines
- [ ] Documentation updated
- [ ] Changes tested locally
- [ ] No secrets committed
- [ ] YAML files validated
```

## Areas for Contribution

### High Priority

See [docs/ansible/TODO.md](ansible/TODO.md) for current TODO items:
- Automate k8s_user creation
- Remove hard-coded values in roles
- Consolidate playbook structure
- Improve ansible rke2_node_token handling

### Documentation

- Expand application deployment guides
- Add troubleshooting tips
- Create video tutorials or diagrams
- Improve getting started guide

### Infrastructure

- Add support for multi-master RKE2 setup
- Implement automated backup solutions
- Add monitoring and alerting configurations
- Create disaster recovery procedures

### Applications

- Add new application deployments
- Update Helm charts to latest versions
- Improve resource configurations
- Add health checks and probes

## Security

### Never Commit Secrets

- Use `.gitignore` to exclude sensitive files
- Use External Secrets Operator for production secrets
- Use Ansible Vault for encrypted variables
- Review changes before committing:
  ```bash
  git diff
  ```

### Security Best Practices

- Keep dependencies up to date
- Use specific image tags (not `latest`)
- Run containers as non-root when possible
- Implement network policies
- Regular security scans

## Questions or Help?

- Open an issue for questions
- Tag with `question` label
- Be respectful and patient

## License

By contributing, you agree that your contributions will be licensed under the same license as the project.

## Thank You!

Your contributions help make this project better for everyone. We appreciate your time and effort!
