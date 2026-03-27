helm repo add 98jan https://98jan.github.io/helm-charts/
helm repo update
helm upgrade --install satisfactory 98jan/satisfactory -n game-server -f values.yaml
