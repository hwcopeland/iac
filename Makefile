.PHONY: lint validate kyverno pre-commit-install

lint:
	yamllint -c .yamllint.yaml rke2/

validate:
	find rke2 -name "*.yaml" \
		-not -path "*/gotk-components.yaml" \
		-not -path "*/chem/compute-infrastructure/*" \
		-not -name "values.yaml" \
		| xargs kubeconform \
			-kubernetes-version 1.30.0 \
			-schema-location default \
			-schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' \
			-ignore-missing-schemas \
			-summary

kyverno:
	kyverno apply .kyverno/ --resource rke2/ --detailed-results 2>&1 || true

pre-commit-install:
	pre-commit install
