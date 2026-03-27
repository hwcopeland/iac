---
project: "kai"
maturity: "draft"
last_updated: "2025-07-14"
updated_by: "@devops-engineer"
scope: "Full GitOps deployment of Kai (AI agent platform) onto the existing RKE2 cluster:
  kai-api (Go backend), kai-operator (CRD operator), PostgreSQL, Redis, AgentSandbox CRD,
  Authentik OIDC, Cilium Gateway API ingress, Flux image automation, and GitHub Actions CI."
owner: "@devops-engineer"
dependencies:
  - docs/spec/architecture.md
  - docs/spec/security.md
  - docs/spec/operations.md
---

# TDD: Kai Deployment Architecture on RKE2

## 1. Problem Statement

### 1.1 What and Why Now

Kai is an AI agent platform consisting of a Go backend (`kai-api`), a Kubernetes CRD operator
(`kai-operator`), and supporting data services (PostgreSQL, Redis). It needs a production-grade
GitOps deployment on the existing RKE2 cluster following the same patterns as other services in
this repository (`chem`, `openhands`, `theswamp`).

The verified goal is: **Kai deploys on the existing RKE2 cluster using Flux GitOps, following
the same patterns as other services in this repo. Namespace: `kai`. Domain: `kai.hwcopeland.net`.**

### 1.2 Constraints

- **Single cluster, single environment.** No staging tier — changes go directly to production.
  Rollback is via `git revert` followed by Flux reconcile.
- **Private registry at `zot.hwcopeland.net`.**  All Kai images must be pushed to
  `zot.hwcopeland.net/kai/`. The arc-chem runner has LAN access to the registry; GitHub cloud
  runners do not.
- **Flux is already installed** in the `tooling` namespace, watching
  `github.com/hwcopeland/iac.git` on `main`. The entry-point is
  `rke2/tooling/flux/tooling/apps.yaml`.
- **Gateway API ingress**: The cluster uses Cilium Gateway API. The gateway resource is
  `hwcopeland-gateway` in `kube-system` with a `https` section. NGINX ingress is not present.
- **Secrets via External Secrets Operator** pulling from `ClusterSecretStore: bitwarden-login`.
  All secrets must have a Bitwarden item UUID — no plaintext secrets in manifests.
- **kai-operator needs cluster-scoped permissions** (watches/manages `AgentSandbox` CRDs and
  creates sandboxed pods/network-policies across namespaces). This requires a `ClusterRole` /
  `ClusterRoleBinding`, not a namespaced `Role`.
- **No shared PostgreSQL found.** The `openhands` deployment provisions its own PostgreSQL via
  Helm. Kai will do the same — a dedicated in-namespace PostgreSQL StatefulSet managed by
  Kustomize, matching the `openhands` pattern.

---

## 2. Architecture Overview

```
GitHub (hwcopeland/iac)
    │
    │  push to main
    ▼
GitHub Actions (arc-chem runner — on-cluster, LAN access)
    │  build kai-api + kai-operator Docker images
    │  push → zot.hwcopeland.net/kai/{kai-api,kai-operator}:build-NNNNNN-<sha7>
    ▼
Flux image-reflector-controller (tooling ns)
    │  polls zot.hwcopeland.net/kai/
    │  ImagePolicy: alphabetical asc on ^build-[0-9]+
    │  ImageUpdateAutomation: commits updated tag back to main
    ▼
Flux kustomize-controller (tooling ns)
    │  reconciles rke2/kai/flux/ every 5m
    ▼
kai namespace
    ├── kai-api Deployment           (port 8080, Go binary + embedded static files)
    ├── kai-api Service              (ClusterIP :8080)
    ├── kai-api HPA                  (CPU + custom WebSocket metric)
    ├── kai-operator Deployment      (CRD controller, cluster-scoped)
    ├── AgentSandbox CRD             (cluster-scoped)
    ├── PostgreSQL StatefulSet       (longhorn PVC, ClusterIP)
    ├── Redis StatefulSet            (longhorn PVC, ClusterIP)
    ├── ExternalSecrets (×5)         → bitwarden-login ClusterSecretStore
    ├── HTTPRoute                    → hwcopeland-gateway (kube-system) → kai-api:8080
    ├── DNSRecord                    kai.hwcopeland.net → hwcopeland.net (CNAME, proxied)
    └── NetworkPolicy                (ingress from gateway, inter-component)

authentik namespace
    └── blueprints-configmap.yaml    (add providers-kai.yaml entry)
```

---

## 3. Directory Structure

```
rke2/
└── kai/
    └── flux/                          ← Flux Kustomize tree (watched by Flux)
        ├── kustomization.yaml         ← Root: lists apps.yaml + image-automation/
        ├── apps.yaml                  ← Flux Kustomization CRs (kai + kai-image-automation)
        ├── kai/
        │   ├── kustomization.yaml     ← Kustomize root for all kai manifests
        │   ├── namespace.yaml
        │   ├── crd/
        │   │   └── agentsandbox-crd.yaml
        │   ├── config/
        │   │   ├── kai-api-deployment.yaml
        │   │   ├── kai-api-service.yaml
        │   │   ├── kai-api-hpa.yaml
        │   │   ├── kai-operator-deployment.yaml
        │   │   ├── kai-operator-service.yaml
        │   │   ├── postgres-statefulset.yaml
        │   │   ├── postgres-service.yaml
        │   │   ├── postgres-pvc.yaml
        │   │   ├── redis-statefulset.yaml
        │   │   ├── redis-service.yaml
        │   │   ├── redis-pvc.yaml
        │   │   └── rbac.yaml
        │   ├── external-secrets.yaml  ← All ExternalSecret resources
        │   ├── httproute.yaml
        │   ├── dnsrecord.yaml
        │   └── networkpolicy.yaml
        └── image-automation/
            ├── kustomization.yaml
            ├── image-repository-kai-api.yaml
            ├── image-repository-kai-operator.yaml
            ├── image-policy-kai-api.yaml
            ├── image-policy-kai-operator.yaml
            ├── image-update-automation.yaml
            └── zot-image-pull-secret.yaml  ← ExternalSecret → zot-pull-secret

rke2/
└── authentik/
    └── blueprints/
        └── providers-kai.yaml         ← New: Kai OIDC provider + application blueprint
```

> **Note on blueprint loading**: The `authentik/blueprints-configmap.yaml` ConfigMap mounts
> all files in `rke2/authentik/blueprints/` as data keys. Adding `providers-kai.yaml` to that
> directory and adding the key to `blueprints-configmap.yaml` is sufficient for Authentik to
> pick up the blueprint on next reconcile.

---

## 4. Kubernetes Manifests

### 4.1 Namespace

**`rke2/kai/flux/kai/namespace.yaml`**

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: kai
```

### 4.2 AgentSandbox CRD

**`rke2/kai/flux/kai/crd/agentsandbox-crd.yaml`**

Cluster-scoped CRD installed before the operator (Kustomize ordering via `resources` list).

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: agentsandboxes.kai.hwcopeland.net
spec:
  group: kai.hwcopeland.net
  names:
    kind: AgentSandbox
    listKind: AgentSandboxList
    plural: agentsandboxes
    singular: agentsandbox
    shortNames:
      - as
  scope: Namespaced          # sandboxes are namespaced per tenant
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          required: ["spec"]
          properties:
            spec:
              type: object
              required: ["agentImage", "sessionID"]
              properties:
                agentImage:
                  type: string
                  description: "OCI image reference for the agent container"
                sessionID:
                  type: string
                  description: "Unique session identifier (UUID)"
                ttlSeconds:
                  type: integer
                  default: 3600
                  description: "Time-to-live in seconds before the operator GCs the sandbox"
                resources:
                  type: object
                  properties:
                    cpuLimit:
                      type: string
                      default: "500m"
                    memoryLimit:
                      type: string
                      default: "512Mi"
            status:
              type: object
              properties:
                phase:
                  type: string
                  enum: [Pending, Running, Terminating, Failed]
                podName:
                  type: string
                startedAt:
                  type: string
                  format: date-time
      subresources:
        status: {}
      additionalPrinterColumns:
        - name: Phase
          type: string
          jsonPath: .status.phase
        - name: SessionID
          type: string
          jsonPath: .spec.sessionID
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp
```

### 4.3 kai-api Deployment

**`rke2/kai/flux/kai/config/kai-api-deployment.yaml`**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kai-api
  namespace: kai
  labels:
    app.kubernetes.io/name: kai-api
    app.kubernetes.io/component: backend
spec:
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: kai-api
  template:
    metadata:
      labels:
        app.kubernetes.io/name: kai-api
        app.kubernetes.io/component: backend
    spec:
      serviceAccountName: kai-api
      imagePullSecrets:
        - name: zot-pull-secret
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
        - name: kai-api
          # Image tag managed by Flux ImageUpdateAutomation
          image: zot.hwcopeland.net/kai/kai-api:build-000001-0000000 # {"$imagepolicy": "kai:kai-api"}
          imagePullPolicy: Always
          ports:
            - containerPort: 8080
              name: http
              protocol: TCP
          env:
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: kai-postgres-credentials
                  key: database-url
            - name: REDIS_URL
              valueFrom:
                secretKeyRef:
                  name: kai-redis-credentials
                  key: redis-url
            - name: JWT_SIGNING_KEY
              valueFrom:
                secretKeyRef:
                  name: kai-jwt-key
                  key: signing-key
            - name: OIDC_CLIENT_ID
              valueFrom:
                secretKeyRef:
                  name: kai-oidc-secret
                  key: client-id
            - name: OIDC_CLIENT_SECRET
              valueFrom:
                secretKeyRef:
                  name: kai-oidc-secret
                  key: client-secret
            - name: OIDC_ISSUER_URL
              value: "https://auth.hwcopeland.net/application/o/kai/"
            - name: LLM_API_KEY
              valueFrom:
                secretKeyRef:
                  name: kai-llm-credentials
                  key: api-key
            - name: KUBERNETES_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
          resources:
            requests:
              cpu: 200m
              memory: 256Mi
            limits:
              cpu: "1"
              memory: 1Gi
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 15
            periodSeconds: 30
            timeoutSeconds: 5
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /readyz
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 10
            timeoutSeconds: 3
            failureThreshold: 3
          startupProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 5
            failureThreshold: 12        # 60s window for startup
```

### 4.4 kai-api Service

**`rke2/kai/flux/kai/config/kai-api-service.yaml`**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: kai-api
  namespace: kai
  labels:
    app.kubernetes.io/name: kai-api
spec:
  type: ClusterIP
  ports:
    - port: 8080
      targetPort: 8080
      protocol: TCP
      name: http
  selector:
    app.kubernetes.io/name: kai-api
```

### 4.5 kai-api HPA

**`rke2/kai/flux/kai/config/kai-api-hpa.yaml`**

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: kai-api
  namespace: kai
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: kai-api
  minReplicas: 2
  maxReplicas: 8
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300   # 5 min cool-down before scale-down
      policies:
        - type: Replicas
          value: 1
          periodSeconds: 60
    scaleUp:
      stabilizationWindowSeconds: 30
      policies:
        - type: Replicas
          value: 2
          periodSeconds: 60
```

> **Note on WebSocket metric**: HPA v2 supports custom metrics from Prometheus Adapter. If
> `kai-api` exposes a `kai_api_active_websocket_connections` Prometheus metric, a third
> `metrics` entry using `type: Pods` can be added in Phase 2 after Prometheus Adapter is
> configured for the `kai` namespace.

### 4.6 kai-operator Deployment

**`rke2/kai/flux/kai/config/kai-operator-deployment.yaml`**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kai-operator
  namespace: kai
  labels:
    app.kubernetes.io/name: kai-operator
    app.kubernetes.io/component: operator
spec:
  replicas: 1              # Leader-election not required for single-instance home cluster
  selector:
    matchLabels:
      app.kubernetes.io/name: kai-operator
  template:
    metadata:
      labels:
        app.kubernetes.io/name: kai-operator
        app.kubernetes.io/component: operator
    spec:
      serviceAccountName: kai-operator
      imagePullSecrets:
        - name: zot-pull-secret
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
      containers:
        - name: operator
          image: zot.hwcopeland.net/kai/kai-operator:build-000001-0000000 # {"$imagepolicy": "kai:kai-operator"}
          imagePullPolicy: Always
          ports:
            - containerPort: 8081
              name: metrics
          env:
            - name: WATCH_NAMESPACE
              value: ""              # Empty = cluster-wide watch
            - name: OPERATOR_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 256Mi
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8081
            initialDelaySeconds: 15
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /readyz
              port: 8081
            initialDelaySeconds: 10
            periodSeconds: 10
```

### 4.7 PostgreSQL StatefulSet

**`rke2/kai/flux/kai/config/postgres-statefulset.yaml`**

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: kai-postgres
  namespace: kai
  labels:
    app.kubernetes.io/name: kai-postgres
spec:
  serviceName: kai-postgres
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: kai-postgres
  template:
    metadata:
      labels:
        app.kubernetes.io/name: kai-postgres
    spec:
      securityContext:
        fsGroup: 999
      containers:
        - name: postgres
          image: postgres:16.3-alpine  # Pin minor version; update manually
          ports:
            - containerPort: 5432
              name: postgres
          env:
            - name: POSTGRES_DB
              value: kai
            - name: POSTGRES_USER
              valueFrom:
                secretKeyRef:
                  name: kai-postgres-credentials
                  key: username
            - name: POSTGRES_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: kai-postgres-credentials
                  key: password
            - name: PGDATA
              value: /var/lib/postgresql/data/pgdata
          resources:
            requests:
              cpu: 100m
              memory: 256Mi
            limits:
              cpu: 500m
              memory: 1Gi
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql/data
          livenessProbe:
            exec:
              command: ["pg_isready", "-U", "kai", "-d", "kai"]
            initialDelaySeconds: 30
            periodSeconds: 10
          readinessProbe:
            exec:
              command: ["pg_isready", "-U", "kai", "-d", "kai"]
            initialDelaySeconds: 5
            periodSeconds: 5
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: kai-postgres-data
```

**`rke2/kai/flux/kai/config/postgres-pvc.yaml`**

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: kai-postgres-data
  namespace: kai
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: longhorn
  resources:
    requests:
      storage: 20Gi
```

**`rke2/kai/flux/kai/config/postgres-service.yaml`**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: kai-postgres
  namespace: kai
spec:
  type: ClusterIP
  clusterIP: None   # Headless — StatefulSet DNS: kai-postgres-0.kai-postgres.kai.svc.cluster.local
  ports:
    - port: 5432
      targetPort: 5432
      name: postgres
  selector:
    app.kubernetes.io/name: kai-postgres
```

### 4.8 Redis StatefulSet

**`rke2/kai/flux/kai/config/redis-statefulset.yaml`**

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: kai-redis
  namespace: kai
  labels:
    app.kubernetes.io/name: kai-redis
spec:
  serviceName: kai-redis
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: kai-redis
  template:
    metadata:
      labels:
        app.kubernetes.io/name: kai-redis
    spec:
      containers:
        - name: redis
          image: redis:7.2-alpine  # Pin minor version
          command:
            - redis-server
            - --requirepass
            - $(REDIS_PASSWORD)
            - --appendonly
            - "yes"
          ports:
            - containerPort: 6379
              name: redis
          env:
            - name: REDIS_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: kai-redis-credentials
                  key: password
          resources:
            requests:
              cpu: 50m
              memory: 128Mi
            limits:
              cpu: 200m
              memory: 512Mi
          volumeMounts:
            - name: data
              mountPath: /data
          livenessProbe:
            exec:
              command: ["redis-cli", "ping"]
            initialDelaySeconds: 15
            periodSeconds: 10
          readinessProbe:
            exec:
              command: ["redis-cli", "ping"]
            initialDelaySeconds: 5
            periodSeconds: 5
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: kai-redis-data
```

**`rke2/kai/flux/kai/config/redis-pvc.yaml`**

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: kai-redis-data
  namespace: kai
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: longhorn
  resources:
    requests:
      storage: 5Gi
```

**`rke2/kai/flux/kai/config/redis-service.yaml`**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: kai-redis
  namespace: kai
spec:
  type: ClusterIP
  ports:
    - port: 6379
      targetPort: 6379
      name: redis
  selector:
    app.kubernetes.io/name: kai-redis
```

### 4.9 RBAC

**`rke2/kai/flux/kai/config/rbac.yaml`**

```yaml
# --- kai-api ServiceAccount (namespace-scoped, no cluster permissions needed) ---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kai-api
  namespace: kai
---
# --- kai-operator ServiceAccount ---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kai-operator
  namespace: kai
---
# --- ClusterRole: kai-operator needs cluster-wide permissions ---
# Manages AgentSandbox CRs (namespaced) and creates/watches pods/services/
# network-policies within sandbox namespaces.
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kai-operator
rules:
  # AgentSandbox CRD management
  - apiGroups: ["kai.hwcopeland.net"]
    resources: ["agentsandboxes", "agentsandboxes/status", "agentsandboxes/finalizers"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  # Pod lifecycle for sandboxes
  - apiGroups: [""]
    resources: ["pods", "pods/log", "pods/exec"]
    verbs: ["get", "list", "watch", "create", "delete", "patch"]
  # Services for sandbox networking
  - apiGroups: [""]
    resources: ["services"]
    verbs: ["get", "list", "watch", "create", "delete"]
  # NetworkPolicies for sandbox isolation
  - apiGroups: ["networking.k8s.io"]
    resources: ["networkpolicies"]
    verbs: ["get", "list", "watch", "create", "delete", "patch"]
  # Events for operator status reporting
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "patch"]
  # Namespace list/watch (operator needs to discover sandbox namespaces)
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kai-operator
subjects:
  - kind: ServiceAccount
    name: kai-operator
    namespace: kai
roleRef:
  kind: ClusterRole
  name: kai-operator
  apiGroup: rbac.authorization.k8s.io
---
# Role for kai-api to read its own namespace resources (ConfigMaps, Secrets lookup if needed)
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: kai-api
  namespace: kai
rules:
  - apiGroups: ["kai.hwcopeland.net"]
    resources: ["agentsandboxes"]
    verbs: ["get", "list", "watch", "create"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: kai-api
  namespace: kai
subjects:
  - kind: ServiceAccount
    name: kai-api
    namespace: kai
roleRef:
  kind: Role
  name: kai-api
  apiGroup: rbac.authorization.k8s.io
```

### 4.10 NetworkPolicy

**`rke2/kai/flux/kai/networkpolicy.yaml`**

```yaml
# Allow kai-api to receive traffic only from the Cilium gateway
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kai-api-ingress
  namespace: kai
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: kai-api
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
      ports:
        - port: 8080
  egress:
    - to:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: kai-postgres
      ports:
        - port: 5432
    - to:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: kai-redis
      ports:
        - port: 6379
    - {}   # Allow DNS + external LLM API calls; tighten in Phase 2
---
# PostgreSQL: only accept connections from kai-api
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kai-postgres-ingress
  namespace: kai
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: kai-postgres
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: kai-api
      ports:
        - port: 5432
---
# Redis: accept from kai-api and kai-operator
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kai-redis-ingress
  namespace: kai
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: kai-redis
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: kai-api
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: kai-operator
      ports:
        - port: 6379
```

---

## 5. Flux Kustomization Setup

### 5.1 Flux App Entry

Add to **`rke2/tooling/flux/tooling/apps.yaml`** (or a new `rke2/tooling/flux/kai/apps.yaml`
included from the root kustomization — follow existing pattern):

```yaml
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: kai
  namespace: tooling
spec:
  interval: 5m
  path: ./rke2/kai/flux/kai
  prune: true
  sourceRef:
    kind: GitRepository
    name: tooling
    namespace: tooling
  dependsOn:
    - name: kai-image-automation  # Wait for image pull secret to exist
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: kai-image-automation
  namespace: tooling
spec:
  interval: 5m
  path: ./rke2/kai/flux/image-automation
  prune: true
  sourceRef:
    kind: GitRepository
    name: tooling
    namespace: tooling
```

### 5.2 Kustomize Root (kai manifests)

**`rke2/kai/flux/kai/kustomization.yaml`**

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - namespace.yaml
  - crd/agentsandbox-crd.yaml       # CRD must come before operator that watches it
  - config/rbac.yaml
  - config/postgres-pvc.yaml
  - config/postgres-statefulset.yaml
  - config/postgres-service.yaml
  - config/redis-pvc.yaml
  - config/redis-statefulset.yaml
  - config/redis-service.yaml
  - config/kai-api-deployment.yaml
  - config/kai-api-service.yaml
  - config/kai-api-hpa.yaml
  - config/kai-operator-deployment.yaml
  - config/kai-operator-service.yaml
  - external-secrets.yaml
  - httproute.yaml
  - dnsrecord.yaml
  - networkpolicy.yaml
  # zot-pull-secret is managed by the image-automation Kustomization.
  # It is NOT listed here to avoid ExternalSecret owner conflict.
```

### 5.3 Image Automation Kustomize Root

**`rke2/kai/flux/image-automation/kustomization.yaml`**

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - zot-image-pull-secret.yaml
  - image-repository-kai-api.yaml
  - image-repository-kai-operator.yaml
  - image-policy-kai-api.yaml
  - image-policy-kai-operator.yaml
  - image-update-automation.yaml
```

---

## 6. ExternalSecret Configuration

All secrets are pulled from `ClusterSecretStore: bitwarden-login`. Before implementation,
the operator must create one Bitwarden item per secret group and note the UUID. The UUIDs
below are **placeholders** — replace with real UUIDs before committing manifests.

**`rke2/kai/flux/kai/external-secrets.yaml`**

### Secret Inventory

| K8s Secret Name | Bitwarden Item | Keys Needed | Notes |
|---|---|---|---|
| `kai-oidc-secret` | `kai-oidc` (new item) | `client-id`, `client-secret` | Generated by Authentik OIDC provider |
| `kai-postgres-credentials` | `kai-postgres` (new item) | `username`, `password`, `database-url` | `database-url` templated from user+password |
| `kai-redis-credentials` | `kai-redis` (new item) | `password`, `redis-url` | `redis-url` templated |
| `kai-jwt-key` | `kai-jwt` (new item) | `signing-key` | 256-bit random secret, generate with `openssl rand -base64 32` |
| `kai-llm-credentials` | `kai-llm-api-key` (can reuse xAI item from openhands) | `api-key` | Reuse Bitwarden item `890152c3-66e4-40d7-a115-291671038ecd` if sharing xAI key |

```yaml
# --- OIDC Client Secret ---
# Bitwarden item: kai-oidc (new)
# username field = client_id (set to static value from Authentik blueprint)
# password field = client_secret (generated by Authentik on provider creation)
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: kai-oidc-secret-es
  namespace: kai
spec:
  refreshInterval: "5m"
  secretStoreRef:
    kind: ClusterSecretStore
    name: bitwarden-login
  target:
    name: kai-oidc-secret
    creationPolicy: Owner
  data:
    - secretKey: client-id
      remoteRef:
        key: <KAI_OIDC_BITWARDEN_UUID>    # REPLACE: UUID of the kai-oidc Bitwarden item
        property: username
    - secretKey: client-secret
      remoteRef:
        key: <KAI_OIDC_BITWARDEN_UUID>
        property: password
---
# --- PostgreSQL Credentials ---
# Bitwarden item: kai-postgres (new)
# username = database username (e.g. "kai")
# password = database password (random, strong)
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: kai-postgres-credentials-es
  namespace: kai
spec:
  refreshInterval: "5m"
  secretStoreRef:
    kind: ClusterSecretStore
    name: bitwarden-login
  target:
    name: kai-postgres-credentials
    creationPolicy: Owner
    template:
      data:
        username: "{{ .pgUser }}"
        password: "{{ .pgPass }}"
        # DSN template consumed by kai-api DATABASE_URL env var
        database-url: "postgres://{{ .pgUser }}:{{ .pgPass }}@kai-postgres.kai.svc.cluster.local:5432/kai?sslmode=disable"
  data:
    - secretKey: pgUser
      remoteRef:
        key: <KAI_POSTGRES_BITWARDEN_UUID>  # REPLACE
        property: username
    - secretKey: pgPass
      remoteRef:
        key: <KAI_POSTGRES_BITWARDEN_UUID>
        property: password
---
# --- Redis Credentials ---
# Bitwarden item: kai-redis (new)
# password = Redis requirepass value (random, strong)
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: kai-redis-credentials-es
  namespace: kai
spec:
  refreshInterval: "5m"
  secretStoreRef:
    kind: ClusterSecretStore
    name: bitwarden-login
  target:
    name: kai-redis-credentials
    creationPolicy: Owner
    template:
      data:
        password: "{{ .redisPass }}"
        redis-url: "redis://:{{ .redisPass }}@kai-redis.kai.svc.cluster.local:6379/0"
  data:
    - secretKey: redisPass
      remoteRef:
        key: <KAI_REDIS_BITWARDEN_UUID>    # REPLACE
        property: password
---
# --- JWT Signing Key ---
# Bitwarden item: kai-jwt (new)
# password = base64-encoded 256-bit key: openssl rand -base64 32
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: kai-jwt-key-es
  namespace: kai
spec:
  refreshInterval: "5m"
  secretStoreRef:
    kind: ClusterSecretStore
    name: bitwarden-login
  target:
    name: kai-jwt-key
    creationPolicy: Owner
  data:
    - secretKey: signing-key
      remoteRef:
        key: <KAI_JWT_BITWARDEN_UUID>      # REPLACE
        property: password
---
# --- LLM API Key ---
# Reuses the same xAI key as openhands (item 890152c3-...) if sharing the same key,
# or a new Bitwarden item for a Kai-specific key.
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: kai-llm-credentials-es
  namespace: kai
spec:
  refreshInterval: "5m"
  secretStoreRef:
    kind: ClusterSecretStore
    name: bitwarden-login
  target:
    name: kai-llm-credentials
    creationPolicy: Owner
  data:
    - secretKey: api-key
      remoteRef:
        key: <KAI_LLM_BITWARDEN_UUID>      # REPLACE (or reuse 890152c3-66e4-40d7-a115-291671038ecd)
        property: password
```

### Zot Registry Pull Secret (image-automation)

**`rke2/kai/flux/image-automation/zot-image-pull-secret.yaml`**

```yaml
# Managed by image-automation Kustomization — provides zot-pull-secret in kai namespace
# Bitwarden item: 766ec5c7-6aa8-419d-bb27-e5982872bc5b (same as chem, already created)
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: zot-pull-secret-es
  namespace: kai
spec:
  refreshInterval: "5m"
  secretStoreRef:
    kind: ClusterSecretStore
    name: bitwarden-login
  target:
    name: zot-pull-secret
    creationPolicy: Owner
    template:
      type: kubernetes.io/dockerconfigjson
      data:
        .dockerconfigjson: |
          {"auths":{"zot.hwcopeland.net":{"username":"{{ .zotUser }}","password":"{{ .zotPass }}","auth":"{{ list .zotUser ":" .zotPass | join "" | b64enc }}"}}}
  data:
    - secretKey: zotUser
      remoteRef:
        key: 766ec5c7-6aa8-419d-bb27-e5982872bc5b
        property: username
    - secretKey: zotPass
      remoteRef:
        key: 766ec5c7-6aa8-419d-bb27-e5982872bc5b
        property: password
```

---

## 7. HTTPRoute and DNS

**`rke2/kai/flux/kai/httproute.yaml`**

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: kai
  namespace: kai
spec:
  hostnames:
    - kai.hwcopeland.net
  parentRefs:
    - name: hwcopeland-gateway
      namespace: kube-system
      sectionName: https
  rules:
    - backendRefs:
        - name: kai-api
          port: 8080
```

**`rke2/kai/flux/kai/dnsrecord.yaml`**

```yaml
apiVersion: cloudflare-operator.io/v1
kind: DNSRecord
metadata:
  name: kai
  namespace: kai
spec:
  name: kai.hwcopeland.net
  type: CNAME
  content: hwcopeland.net
  proxied: true
  ttl: 1
  interval: 5m
```

> **ReferenceGrant**: The `hwcopeland-gateway` is in `kube-system`. Gateway API requires
> a `ReferenceGrant` in `kube-system` to allow routes from the `kai` namespace — check
> whether the existing `ReferenceGrant` in `authentik/reference-grants.yaml` covers all
> namespaces or if a kai-specific one is needed. If the existing grant is cluster-wide
> (allows all namespaces → kube-system), no additional grant is required.

---

## 8. Flux Image Automation

### 8.1 ImageRepository Resources

**`rke2/kai/flux/image-automation/image-repository-kai-api.yaml`**

```yaml
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImageRepository
metadata:
  name: kai-api
  namespace: kai
spec:
  image: zot.hwcopeland.net/kai/kai-api
  interval: 5m
  secretRef:
    name: zot-pull-secret
```

**`rke2/kai/flux/image-automation/image-repository-kai-operator.yaml`**

```yaml
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImageRepository
metadata:
  name: kai-operator
  namespace: kai
spec:
  image: zot.hwcopeland.net/kai/kai-operator
  interval: 5m
  secretRef:
    name: zot-pull-secret
```

### 8.2 ImagePolicy Resources

Tag format matches CI: `build-<NNNNNN>-<sha7>` — zero-padded run number ensures
alphabetical == chronological order (same reasoning as `chem` — see `docs/tdd/github-actions-ci.md`).

**`rke2/kai/flux/image-automation/image-policy-kai-api.yaml`**

```yaml
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImagePolicy
metadata:
  name: kai-api
  namespace: kai
spec:
  imageRepositoryRef:
    name: kai-api
  filterTags:
    pattern: '^build-[0-9]+'
    extract: "$0"
  policy:
    alphabetical:
      order: asc
```

**`rke2/kai/flux/image-automation/image-policy-kai-operator.yaml`**

```yaml
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImagePolicy
metadata:
  name: kai-operator
  namespace: kai
spec:
  imageRepositoryRef:
    name: kai-operator
  filterTags:
    pattern: '^build-[0-9]+'
    extract: "$0"
  policy:
    alphabetical:
      order: asc
```

### 8.3 ImageUpdateAutomation

**`rke2/kai/flux/image-automation/image-update-automation.yaml`**

```yaml
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImageUpdateAutomation
metadata:
  name: kai
  namespace: kai
spec:
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: tooling
    namespace: tooling
  git:
    checkout:
      ref:
        branch: main
    commit:
      author:
        email: fluxcdbot@users.noreply.github.com
        name: fluxcdbot
      messageTemplate: |
        [ci skip] auto-update kai
    push:
      branch: main
  update:
    path: ./rke2/kai/flux/kai
    strategy: Setters
```

---

## 9. CI/CD Pipeline

### 9.1 GitHub Actions Workflow

**`.github/workflows/build-kai-images.yml`**

```yaml
# Build and push kai-api and kai-operator Docker images to zot.hwcopeland.net/kai/
#
# Runner: self-hosted [arc-chem] — on-cluster, LAN access to zot.hwcopeland.net.
# Registry credentials: ZOT_USERNAME / ZOT_PASSWORD (same secrets used by chem pipeline).
#
# Tag format: build-<NNNNNN>-<sha7>
#   Zero-padded run number ensures alphabetical == chronological, enabling
#   Flux ImagePolicy alphabetical/asc selection.
#
# Cache strategy: registry-based layer cache (type=registry, mode=max).
#   type=gha is not usable from on-cluster self-hosted runners.

name: Build and Push Kai Images

on:
  push:
    branches:
      - main
    paths:
      - 'kai/api/**'
      - 'kai/operator/**'
      - '.github/workflows/build-kai-images.yml'
  workflow_dispatch:

jobs:
  build-and-push:
    name: Build ${{ matrix.name }}
    runs-on: [self-hosted, arc-chem]
    strategy:
      fail-fast: false
      matrix:
        include:
          - name: kai-api
            image: zot.hwcopeland.net/kai/kai-api
            dockerfile: kai/api/Dockerfile
            context: kai/api/

          - name: kai-operator
            image: zot.hwcopeland.net/kai/kai-operator
            dockerfile: kai/operator/Dockerfile
            context: kai/operator/

    steps:
      - name: Checkout repository
        uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683  # v4.2.2

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@b5ca514318bd6ebac0fb2aedd5d36ec1b5c232a2  # v3.10.0

      - name: Log in to Zot registry
        uses: docker/login-action@74a5d142397b4f367a81961eba4e8cd7edddf772  # v3.4.0
        with:
          registry: zot.hwcopeland.net
          username: ${{ secrets.ZOT_USERNAME }}
          password: ${{ secrets.ZOT_PASSWORD }}

      - name: Compute image tags
        id: tags
        run: |
          SHORT_SHA="${GITHUB_SHA::7}"
          RUN_NUM=$(printf '%06d' "${{ github.run_number }}")
          BUILD_TAG="build-${RUN_NUM}-${SHORT_SHA}"
          echo "build_tag=${BUILD_TAG}" >> "$GITHUB_OUTPUT"
          echo "Building ${{ matrix.name }} with tag: ${BUILD_TAG}"

      - name: Build and push ${{ matrix.name }}
        uses: docker/build-push-action@14487ce63c7a62a4a324b0bfb37086795e31c6c1  # v6.16.0
        with:
          context: ${{ matrix.context }}
          file: ${{ matrix.dockerfile }}
          platforms: linux/amd64
          push: true
          tags: |
            ${{ matrix.image }}:${{ steps.tags.outputs.build_tag }}
            ${{ matrix.image }}:latest
          cache-from: type=registry,ref=${{ matrix.image }}:cache
          cache-to: type=registry,ref=${{ matrix.image }}:cache,mode=max
```

### 9.2 Go Dockerfile Pattern

Both `kai-api` and `kai-operator` are Go binaries. Use multi-stage builds to keep runtime
images minimal.

**`kai/api/Dockerfile`** (design pattern — implementation by @senior-engineer):

```dockerfile
# Stage 1: Build
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o kai-api ./cmd/api

# Stage 2: Runtime (distroless or alpine)
FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/kai-api /kai-api
# If serving embedded static files, the binary already contains them via go:embed
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/kai-api"]
```

**Key build requirements for CI**:
- `CGO_ENABLED=0` for static binary (works in distroless/alpine)
- `-ldflags="-w -s"` strips debug symbols (reduces image size ~30%)
- Static frontend files embedded via `go:embed` (zero separate nginx deployment needed)
- Health endpoints `/health` (liveness) and `/readyz` (readiness) must be implemented

---

## 10. Authentik Blueprint

### 10.1 Kai OIDC Provider and Application

**`rke2/authentik/blueprints/providers-kai.yaml`**

```yaml
version: 1
metadata:
  name: Kai Provider and Application
entries:
# --- OIDC Provider ---
- attrs:
    access_code_validity: minutes=1
    access_token_validity: hours=1
    authorization_flow: !Find [authentik_flows.flow, [slug, default-provider-authorization-implicit-consent]]
    client_id: kai                     # Static, stored in Bitwarden as username of kai-oidc item
    client_type: confidential
    include_claims_in_id_token: true
    invalidation_flow: !Find [authentik_flows.flow, [slug, default-provider-invalidation-flow]]
    issuer_mode: per_provider
    logout_method: backchannel
    name: kai
    property_mappings:
    - !Find [authentik_providers_oauth2.scopemapping, [scope_name, email]]
    - !Find [authentik_providers_oauth2.scopemapping, [scope_name, profile]]
    - !Find [authentik_providers_oauth2.scopemapping, [scope_name, openid]]
    - !Find [authentik_providers_oauth2.scopemapping, [scope_name, groups]]
    redirect_uris:
    - matching_mode: strict
      url: https://kai.hwcopeland.net/auth/callback   # Adjust to actual kai-api OIDC callback path
    refresh_token_threshold: seconds=0
    refresh_token_validity: days=30
    signing_key: !Find [authentik_crypto.certificatekeypair, [name, authentik Self-signed Certificate]]
    sub_mode: hashed_user_id
  conditions: []
  identifiers:
    pk: 20                             # Next available PK after existing providers (check current max)
  model: authentik_providers_oauth2.oauth2provider
  permissions: []
  state: present
# --- Application ---
- attrs:
    meta_description: Kai AI Agent Platform
    meta_launch_url: https://kai.hwcopeland.net
    name: kai
    policy_engine_mode: any
    provider: !Find [authentik_providers_oauth2.oauth2provider, [name, kai]]
    slug: kai
  conditions: []
  identifiers:
    pk: <NEW-UUID>                     # Generate with uuidgen before committing
  model: authentik_core.application
  permissions: []
  state: present
```

### 10.2 Blueprint ConfigMap Update

Add the new key to **`rke2/authentik/blueprints-configmap.yaml`**:

```yaml
# In the data: section, add:
  providers-kai.yaml: |
    <contents of providers-kai.yaml>
```

> **Workflow note**: The `blueprints-configmap.yaml` embeds all blueprint files inline. When
> adding `providers-kai.yaml`, copy its content into the ConfigMap's `data` section — Authentik
> reads mounted ConfigMap keys as blueprint files on startup and periodic reconcile.

---

## 11. Resource Sizing

| Component | CPU Request | CPU Limit | Memory Request | Memory Limit | Replicas | Notes |
|---|---|---|---|---|---|---|
| `kai-api` | 200m | 1000m (1 core) | 256Mi | 1Gi | 2 (HPA 2–8) | Go binary; scales horizontally |
| `kai-operator` | 50m | 200m | 64Mi | 256Mi | 1 | Single instance; no HA needed for home cluster |
| `kai-postgres` | 100m | 500m | 256Mi | 1Gi | 1 | StatefulSet; Longhorn 20Gi PVC |
| `kai-redis` | 50m | 200m | 128Mi | 512Mi | 1 | StatefulSet; Longhorn 5Gi PVC |

**Aggregate worst-case (all limits hit):**
- CPU: 1.9 cores
- Memory: 2.75Gi

**Aggregate baseline (requests, 2× kai-api):**
- CPU: 700m
- Memory: 1.08Gi

These numbers are conservative for a home RKE2 cluster. Tune based on observed usage after
the first two weeks of operation.

---

## 12. Implementation Phases

### Phase 1 — Core Infrastructure (Week 1)

**Goal**: Namespace, CRD, data services, external secrets, HTTPRoute live. No application pods yet.

1. Create Bitwarden items for all five secrets (postgres, redis, jwt, oidc, llm).
   Note all UUIDs.
2. Write `rke2/kai/flux/kai/` manifests:
   - `namespace.yaml`, `external-secrets.yaml`, `dnsrecord.yaml`, `networkpolicy.yaml`
   - CRD: `agentsandbox-crd.yaml`
   - RBAC: `rbac.yaml`
   - PostgreSQL: StatefulSet + Service + PVC
   - Redis: StatefulSet + Service + PVC
3. Write `rke2/kai/flux/image-automation/` manifests (pull secret, repositories, policies, automation).
4. Add Flux Kustomization CRs to `rke2/tooling/flux/tooling/apps.yaml`.
5. Commit and push → Flux reconciles namespace + data services.
6. **Verify**: `kubectl get pods -n kai` shows postgres and redis Running.

### Phase 2 — CI/CD Pipeline (Week 1–2)

**Goal**: GitHub Actions builds and pushes kai-api + kai-operator images on every push.

1. Create `kai/api/Dockerfile` and `kai/operator/Dockerfile` (coordinate with @senior-engineer
   on health endpoint paths and embedded static file approach).
2. Write `.github/workflows/build-kai-images.yml` using the `arc-chem` runner.
3. Push to main → confirm images appear in `zot.hwcopeland.net/kai/`.
4. Confirm Flux `ImagePolicy` picks up new tags and `ImageUpdateAutomation` commits back.

### Phase 3 — Application Deployment (Week 2)

**Goal**: kai-api + kai-operator deployed and accessible at `kai.hwcopeland.net`.

1. Write `kai-api-deployment.yaml` with correct env var names (coordinate with @senior-engineer).
2. Write `kai-operator-deployment.yaml`.
3. Write `httproute.yaml` → verify `kai.hwcopeland.net` resolves and returns HTTP 200.
4. Confirm HPA is active: `kubectl get hpa -n kai`.
5. Confirm ExternalSecrets synced: `kubectl get externalsecret -n kai`.

### Phase 4 — Authentik OIDC Integration (Week 2–3)

**Goal**: Kai authenticates users via Authentik at `auth.hwcopeland.net`.

1. Create Authentik Kai application in Bitwarden (get generated `client_secret`).
2. Write and commit `rke2/authentik/blueprints/providers-kai.yaml`.
3. Update `rke2/authentik/blueprints-configmap.yaml` with new key.
4. Apply → Authentik reconciles → provider and application appear in Authentik admin.
5. Update Bitwarden `kai-oidc` item with real `client_id` and `client_secret`.
6. **Verify**: Login to `kai.hwcopeland.net` redirects to `auth.hwcopeland.net` and completes OIDC flow.

### Phase 5 — Observability and Hardening (Week 3–4)

**Goal**: Production-ready observability and tighter security posture.

1. Add `ServiceMonitor` for `kai-api` and `kai-operator` (Prometheus scrape).
2. Add custom metric export for active WebSocket connections; configure HPA custom metric.
3. Tighten `NetworkPolicy` egress rules (replace `- {}` wildcard with explicit LLM API CIDRs).
4. Add `PodDisruptionBudget` for `kai-api` (min 1 available during node drain).
5. Verify Longhorn PVC backups are enabled for postgres and redis volumes.

---

## 13. Acceptance Criteria

### Phase 1 — Core Infrastructure
- [ ] `kubectl get ns kai` returns `Active`
- [ ] `kubectl get crd agentsandboxes.kai.hwcopeland.net` returns the CRD
- [ ] `kubectl get pods -n kai -l app.kubernetes.io/name=kai-postgres` returns `Running`
- [ ] `kubectl get pods -n kai -l app.kubernetes.io/name=kai-redis` returns `Running`
- [ ] `kubectl get pvc -n kai` shows two `Bound` PVCs on `longhorn`
- [ ] `kubectl get externalsecret -n kai` shows all secrets `SecretSynced`
- [ ] No ExternalSecret errors in `kubectl describe externalsecret -n kai`

### Phase 2 — CI/CD Pipeline
- [ ] Pushing a commit that touches `kai/api/**` triggers `build-kai-images.yml` on `arc-chem`
- [ ] Workflow completes without error
- [ ] `zot.hwcopeland.net/kai/kai-api` has a new `build-NNNNNN-<sha>` tag
- [ ] `kubectl get imagepolicy -n kai kai-api` shows the new tag as `latestImage`
- [ ] Flux commits back to `main` with `[ci skip] auto-update kai`

### Phase 3 — Application Deployment
- [ ] `kubectl get pods -n kai -l app.kubernetes.io/name=kai-api` shows 2× `Running`
- [ ] `kubectl get pods -n kai -l app.kubernetes.io/name=kai-operator` shows 1× `Running`
- [ ] `curl -I https://kai.hwcopeland.net/health` returns `HTTP/2 200`
- [ ] `kubectl get hpa -n kai kai-api` shows `TARGETS` reporting CPU utilization (not `<unknown>`)
- [ ] `kubectl get httproute -n kai kai` shows `Accepted: True, ResolvedRefs: True`

### Phase 4 — Authentik OIDC Integration
- [ ] Authentik admin shows `kai` application and provider under Applications
- [ ] Navigating to `kai.hwcopeland.net` redirects unauthenticated users to Authentik login
- [ ] Successful Authentik login redirects back to `kai.hwcopeland.net` with valid session
- [ ] `kubectl get secret -n kai kai-oidc-secret` exists and is not empty

### Phase 5 — Observability and Hardening
- [ ] Prometheus scrapes `kai-api` metrics (confirm in Prometheus targets UI)
- [ ] Longhorn UI shows snapshots enabled for `kai-postgres-data` and `kai-redis-data` volumes
- [ ] `kubectl get pdb -n kai` shows at least one PodDisruptionBudget
- [ ] Tightened NetworkPolicy: `kubectl describe netpol kai-api-ingress -n kai` shows no wildcard egress

---

## 14. Open Questions and Decisions Required

| # | Question | Stakeholder | Default Assumption |
|---|---|---|---|
| 1 | Does `kai-api` serve the frontend as `go:embed` static files, or is there a separate nginx sidecar/deployment? | @senior-engineer | Assume `go:embed` (simplest, avoids nginx deployment) |
| 2 | What is the exact OIDC callback URL for `kai-api`? (e.g., `/auth/callback`, `/oauth/callback`) | @senior-engineer | `/auth/callback` assumed in blueprint |
| 3 | Does `kai-operator` need to create `AgentSandbox` pods in the `kai` namespace only, or across arbitrary namespaces? | @senior-engineer | Cluster-wide watch, sandboxes created in `kai` ns |
| 4 | Shared xAI LLM API key (reuse `890152c3-...`) or separate Kai-specific key? | Operator | New item recommended to avoid openhands dependency |
| 5 | Does the existing `ReferenceGrant` in `authentik/reference-grants.yaml` cover all namespaces, or is a per-namespace grant needed for `kai`? | Operator (check cluster state) | Check before Phase 3 implementation |
| 6 | Do we need WebSocket sticky sessions (session affinity) on the HTTPRoute for agent event streaming? | @senior-engineer | No — Redis pub/sub decouples sessions from pod identity |
| 7 | Should `kai-postgres` use the PostgreSQL `postgres:16.x` official image or a Bitnami chart? | Operator | Official alpine image (matches openhands base pattern for simplicity) |

---

## 15. Risk Register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Flux `ImageUpdateAutomation` push fails (auth, branch protection) | Low | Medium | Verify `flux-system` secret in `tooling` ns has write access; test with `chem` automation first |
| `ReferenceGrant` missing for `kai` → HTTPRoute not admitted | Medium | High | Check `kubectl get referencegrant -n kube-system` before Phase 3; create if absent |
| `kai-operator` ClusterRole too broad (security) | Low | Medium | Scope pod creation to namespaces with label `kai.hwcopeland.net/sandbox: "true"` in Phase 5 |
| PostgreSQL PVC lost if Longhorn volume corrupted | Low | High | Enable Longhorn recurring backup to B2 (same as other Longhorn volumes) |
| Authentik blueprint PK collision | Medium | Low | Check existing max PK before committing; blueprint is idempotent on re-apply |
| Bitwarden UUID typo in ExternalSecret | Medium | High | ExternalSecret will error with clear message; catch in Phase 1 verification |
