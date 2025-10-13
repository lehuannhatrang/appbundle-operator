# Deployment Guide

This guide covers different deployment scenarios for the AppBundle Operator.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Local Development](#local-development)
- [Deploy to Cluster](#deploy-to-cluster)
- [Deploy with Kustomize](#deploy-with-kustomize)
- [Deploy with Helm (Future)](#deploy-with-helm)
- [Configuration Options](#configuration-options)
- [Monitoring](#monitoring)
- [Troubleshooting](#troubleshooting)

## Prerequisites

- Kubernetes cluster v1.19+
- kubectl configured
- cluster-admin access (for CRD installation)
- Docker (for building images)
- Go 1.21+ (for local development)

## Local Development

Best for testing and development.

### Step 1: Install CRDs

```bash
make install
```

Verify:
```bash
kubectl get crd appbundles.app.example.com
```

### Step 2: Run Locally

```bash
make run
```

The operator will run on your local machine and connect to your cluster.

### Step 3: Test with Sample

In another terminal:
```bash
kubectl apply -f config/samples/app_v1alpha1_appbundle.yaml
kubectl get appbundle -w
```

### Step 4: Cleanup

```bash
# Stop the operator (Ctrl+C in the first terminal)
make uninstall  # Remove CRDs
```

## Deploy to Cluster

For production or shared development environments.

### Step 1: Build and Push Image

```bash
# Set your image registry
export IMG=<your-registry>/appbundle-operator:v0.1.0

# Build the image
make docker-build IMG=$IMG

# Push to registry
make docker-push IMG=$IMG
```

For Docker Hub:
```bash
export IMG=<your-dockerhub-username>/appbundle-operator:v0.1.0
make docker-build docker-push IMG=$IMG
```

For Google Container Registry:
```bash
export IMG=gcr.io/<your-project>/appbundle-operator:v0.1.0
make docker-build docker-push IMG=$IMG
```

### Step 2: Deploy Operator

```bash
make deploy IMG=$IMG
```

This will:
- Create the `appbundle-operator-system` namespace
- Install CRDs
- Deploy RBAC resources
- Deploy the operator

### Step 3: Verify Deployment

```bash
# Check namespace
kubectl get namespace appbundle-operator-system

# Check deployment
kubectl get deployment -n appbundle-operator-system

# Check pods
kubectl get pods -n appbundle-operator-system

# Check logs
kubectl logs -n appbundle-operator-system deployment/appbundle-operator-controller-manager -f
```

### Step 4: Create AppBundle

```bash
kubectl apply -f config/samples/app_v1alpha1_appbundle.yaml
kubectl get appbundle appbundle-sample -w
```

### Step 5: Cleanup

```bash
# Delete AppBundles first
kubectl delete appbundle --all

# Undeploy operator
make undeploy
```

## Deploy with Kustomize

For customized deployments.

### Create Overlay

```bash
mkdir -p config/overlays/production
```

Create `config/overlays/production/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: appbundle-operator-system

bases:
  - ../../default

images:
  - name: controller
    newName: <your-registry>/appbundle-operator
    newTag: v0.1.0

# Customize resources
patchesStrategicMerge:
  - manager_patch.yaml

# Set resource limits
patches:
  - patch: |-
      - op: add
        path: /spec/template/spec/containers/0/resources
        value:
          limits:
            cpu: 200m
            memory: 256Mi
          requests:
            cpu: 100m
            memory: 128Mi
    target:
      kind: Deployment
      name: appbundle-operator-controller-manager
```

Create `config/overlays/production/manager_patch.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: appbundle-operator-controller-manager
  namespace: appbundle-operator-system
spec:
  replicas: 2  # High availability
  template:
    spec:
      containers:
        - name: manager
          args:
            - --leader-elect
            - --zap-log-level=info
```

### Deploy with Overlay

```bash
kubectl apply -k config/overlays/production
```

## Deploy with Helm

*(Future enhancement - Helm chart to be created)*

## Configuration Options

### Environment Variables

Configure the operator behavior through environment variables:

```yaml
env:
  - name: ENABLE_WEBHOOKS
    value: "false"
  - name: METRICS_ADDR
    value: ":8080"
  - name: HEALTH_PROBE_ADDR
    value: ":8081"
  - name: LEADER_ELECT
    value: "true"
```

### Command-Line Arguments

Available flags:

```
--leader-elect              Enable leader election for controller manager
--metrics-bind-address      The address the metric endpoint binds to (default ":8080")
--health-probe-bind-address The address the probe endpoint binds to (default ":8081")
--zap-log-level             Zap log level (debug, info, error) (default "info")
--zap-encoder               Zap encoder (json or console) (default "json")
```

Example deployment with custom args:

```yaml
spec:
  template:
    spec:
      containers:
        - name: manager
          args:
            - --leader-elect
            - --zap-log-level=debug
            - --metrics-bind-address=:8443
```

### Resource Limits

Recommended resource limits:

**Development**:
```yaml
resources:
  limits:
    cpu: 100m
    memory: 128Mi
  requests:
    cpu: 50m
    memory: 64Mi
```

**Production**:
```yaml
resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 100m
    memory: 128Mi
```

### High Availability

For production deployments:

```yaml
spec:
  replicas: 2  # Or more
  template:
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchLabels:
                    control-plane: controller-manager
                topologyKey: kubernetes.io/hostname
```

## Monitoring

### Metrics

The operator exposes Prometheus metrics on port 8080 by default.

#### Service Monitor (for Prometheus Operator)

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: appbundle-operator-metrics
  namespace: appbundle-operator-system
spec:
  endpoints:
    - path: /metrics
      port: https
      scheme: https
      tlsConfig:
        insecureSkipVerify: true
  selector:
    matchLabels:
      control-plane: controller-manager
```

#### Key Metrics to Monitor

- `controller_runtime_reconcile_total`: Total reconciliations
- `controller_runtime_reconcile_errors_total`: Failed reconciliations
- `controller_runtime_reconcile_time_seconds`: Reconciliation duration
- `workqueue_depth`: Work queue depth

### Logging

View operator logs:

```bash
# Follow logs
kubectl logs -n appbundle-operator-system deployment/appbundle-operator-controller-manager -f

# Get logs for specific pod
kubectl logs -n appbundle-operator-system <pod-name> -c manager

# With timestamp
kubectl logs -n appbundle-operator-system deployment/appbundle-operator-controller-manager --timestamps
```

### Health Checks

The operator exposes health endpoints:

- `/healthz`: Liveness probe
- `/readyz`: Readiness probe

Test health:
```bash
kubectl port-forward -n appbundle-operator-system deployment/appbundle-operator-controller-manager 8081:8081

curl http://localhost:8081/healthz
curl http://localhost:8081/readyz
```

## RBAC Configuration

### Namespace-Scoped Deployment

To limit operator to specific namespaces, modify RBAC:

1. Change ClusterRole to Role
2. Change ClusterRoleBinding to RoleBinding
3. Set namespace in RoleBinding

Example `config/rbac/role.yaml`:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role  # Changed from ClusterRole
metadata:
  name: appbundle-operator-role
  namespace: my-namespace  # Add namespace
# ... rest of the role
```

### Scoped Permissions

For production, scope wildcard permissions:

```yaml
rules:
  # Instead of:
  # - apiGroups: ["*"]
  #   resources: ["*"]
  #   verbs: ["*"]
  
  # Use specific permissions:
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["services", "configmaps", "secrets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  # Add more as needed
```

## Security

### Run as Non-Root

Ensure the operator runs as non-root user:

```yaml
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
        - name: manager
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - ALL
```

### Network Policies

Restrict operator network access:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: appbundle-operator-netpol
  namespace: appbundle-operator-system
spec:
  podSelector:
    matchLabels:
      control-plane: controller-manager
  policyTypes:
    - Ingress
    - Egress
  egress:
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 443  # Kubernetes API
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 53  # DNS
        - protocol: UDP
          port: 53
```

## Upgrade

### Upgrade Operator

```bash
# Build new version
export IMG=<your-registry>/appbundle-operator:v0.2.0
make docker-build docker-push IMG=$IMG

# Apply new version
make deploy IMG=$IMG
```

### Upgrade CRDs

```bash
# Regenerate CRDs
make manifests

# Apply updated CRDs
kubectl apply -f config/crd/bases/
```

**Note**: CRD upgrades should be backward compatible. Test thoroughly before upgrading in production.

## Multi-Cluster Deployment

For managing AppBundles across multiple clusters:

### Hub-Spoke Model

1. Deploy operator in hub cluster
2. Configure kubeconfig for spoke clusters
3. Modify controller to support multi-cluster reconciliation

*(Detailed implementation to be added)*

## Troubleshooting

### Operator Not Starting

```bash
# Check pod events
kubectl describe pod -n appbundle-operator-system <pod-name>

# Check logs
kubectl logs -n appbundle-operator-system <pod-name>

# Common issues:
# 1. Image pull errors - verify IMG and registry access
# 2. RBAC issues - verify ServiceAccount permissions
# 3. CRD not installed - run make install
```

### AppBundle Not Reconciling

```bash
# Check AppBundle status
kubectl describe appbundle <name>

# Check operator logs
kubectl logs -n appbundle-operator-system deployment/appbundle-operator-controller-manager

# Common issues:
# 1. Invalid template - check syntax
# 2. RBAC - operator can't create resources
# 3. Resource conflicts - existing resources
```

### Resource Creation Failures

```bash
# Check component status
kubectl get appbundle <name> -o jsonpath='{.status.groupStatuses}'

# Check for events
kubectl get events -n <namespace> --sort-by='.lastTimestamp'

# Verify RBAC
kubectl auth can-i create deployments --as=system:serviceaccount:appbundle-operator-system:appbundle-operator-controller-manager
```

## Best Practices

1. **Use specific image tags** - Avoid `:latest`
2. **Set resource limits** - Prevent resource exhaustion
3. **Enable leader election** - For HA deployments
4. **Monitor metrics** - Track reconciliation performance
5. **Scope RBAC** - Limit permissions in production
6. **Test upgrades** - Always test in non-prod first
7. **Backup CRs** - Export AppBundles before upgrades
8. **Use namespaces** - Isolate AppBundles by environment

## Next Steps

- [Quickstart Guide](QUICKSTART.md) - Get started quickly
- [Development Guide](DEVELOPMENT.md) - Contribute to the project
- [Architecture](ARCHITECTURE.md) - Understand the internals
- [Main README](../README.md) - Project overview

## Getting Help

- Check operator logs for errors
- Review AppBundle status and conditions
- Consult documentation
- Review GitHub issues (when available)

