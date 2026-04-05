.PHONY: dev dev-api dev-web build test test-api test-ui lint clean

# Local development — run API against cluster MySQL
dev-api:
	kubectl port-forward -n chem svc/docking-mysql 3306:3306 &
	cd api && MYSQL_HOST=localhost MYSQL_PORT=3306 MYSQL_USER=root \
		MYSQL_PASSWORD=$$(kubectl get secret docking-mysql-secret -n chem -o jsonpath='{.data.root-password}' | base64 -d) \
		MYSQL_DATABASE=docking AUTH_ENABLED=false \
		go run .

dev-web:
	cd web && npm run dev

dev: dev-api dev-web

# Build
build-api:
	cd api && go build -o ../bin/khemeia-api .

build-result-writer:
	cd result-writer && go build -o ../bin/result-writer .

build-web:
	cd web && npm run build

build: build-api build-result-writer build-web

# Test
test-api:
	cd api && go test ./...

test-result-writer:
	cd result-writer && go test ./...

test-ui:
	cd tests && npx playwright test

test: test-api test-result-writer test-ui

# Lint
lint-api:
	cd api && go vet ./...

lint-web:
	cd web && npm run check

lint: lint-api lint-web

# Clean
clean:
	rm -rf bin/ web/build/ web/.svelte-kit/

# Flux reconcile
deploy:
	flux reconcile kustomization chem --with-source -n chem

# Force image scan + deploy
deploy-fast:
	flux reconcile image repository docking-controller -n chem
	flux reconcile image repository result-writer -n chem
	flux reconcile image repository autodock-vina -n chem
	flux reconcile kustomization chem --with-source -n chem
