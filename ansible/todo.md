# TODO:

- [ ] install rke2
- [ ] install longhorn
- [ ] install metallb
- [ ] install cert-manager



## Project Structure & Cleanup
**General**
- [ ] k8s_user is a required user that needs to be created outside of ansible. This user should be automatically created or the user should be defined in all hard coded home paths. (I prefer user creation)
**Playbooks**
- [ ] General: limit the amount of playbooks in the root playbook directory so we have an infrastructure playbook to create the infra and a deployment playbook to perform the deployment. These two playbooks can call other playbooks to allow for discrete execution of operations that should be contained in nested folders.
- [ ] kubernetes.yml automate the ansible rke2_node_token. We can detect that it is stored in a file. If not, generate the id and store it in a file. If it is present, load it from a variable.

**Roles**
- [ ] roles/backend/tasks/main.yml contains hard coded ip addresses that we can leverage inventory hosts for.
- [ ] roles/rke2-agent/defaults/main.yaml contains hard coded values that should be pulled from the inventory.
- [ ] roles/rke2-agent/templates/config.yaml.j2 contains hard coded values that should be pulled from the inventory.
