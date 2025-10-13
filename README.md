# AppBundle Operator

A Kubernetes operator built with Kubebuilder for managing application bundles with ordered deployment and Argo CD sync wave integration.

## Overview

The AppBundle Operator enables you to define complex application deployments as a single custom resource, with support for:

- **Ordered Deployment**: Deploy resources in a specific order using groups and components
- **Argo CD Integration**: Automatic sync wave annotations for GitOps workflows
- **Porch Integration**: Package lifecycle management with Porch (Kubernetes Package Orchestration)
- **Comprehensive Status Tracking**: Monitor deployment progress at group and component levels

## Features

### Ordered Resource Deployment

Define application components in groups with explicit ordering:
- Groups are deployed sequentially based on their `order` field
- Components within a group are also deployed in order
- Ensures dependencies are deployed before dependent resources

### Argo CD Sync Wave Integration

The operator automatically adds `argocd.argoproj.io/sync-wave` annotations to deployed resources:
- Group order × 100 + Component order = Sync Wave
- Example: Group 2, Component 3 → Sync Wave 203
- Seamless integration with Argo CD's phased sync

### Porch Package Management

Integration with Porch for package lifecycle management:
- Reference Porch packages in component definitions
- Track package revisions and updates
- Centralized package repository management

### Status Tracking

Comprehensive status reporting:
- Overall deployment phase (Pending, Deploying, Deployed, Failed)
- Per-group status tracking
- Per-component status with resource references
- Kubernetes conditions for integration with other tools

## Architecture

```
AppBundle CR
├── Spec
│   ├── Groups (ordered)
│   │   ├── Components (ordered)
│   │   │   ├── Template (K8s resource)
│   │   │   └── PorchPackageRef (optional)
│   │   └── ...
│   └── ...
└── Status
    ├── Phase
    ├── GroupStatuses
    └── Conditions
```

## Installation

### Prerequisites

- Kubernetes cluster (v1.19+)
- kubectl configured
- (Optional) Argo CD for GitOps workflows
- (Optional) Porch for package management

### Deploy the Operator

```bash
# Install CRDs
make install

# Deploy the operator
make deploy IMG=<your-registry>/appbundle-operator:tag

# Or run locally for development
make run
```

## Usage

### Basic Example

Create a simple AppBundle with ordered deployment:

```yaml
apiVersion: app.example.com/v1alpha1
kind: AppBundle
metadata:
  name: my-app
  namespace: default
spec:
  groups:
    - name: infrastructure
      order: 0
      components:
        - name: namespace
          order: 0
          template:
            apiVersion: v1
            kind: Namespace
            metadata:
              name: my-app
        - name: configmap
          order: 1
          template:
            apiVersion: v1
            kind: ConfigMap
            metadata:
              name: app-config
              namespace: my-app
            data:
              key: value
    
    - name: application
      order: 1
      components:
        - name: deployment
          order: 0
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: my-app
              namespace: my-app
            spec:
              replicas: 3
              selector:
                matchLabels:
                  app: my-app
              template:
                metadata:
                  labels:
                    app: my-app
                spec:
                  containers:
                    - name: app
                      image: nginx:latest
```

### With Porch Integration

```yaml
apiVersion: app.example.com/v1alpha1
kind: AppBundle
metadata:
  name: app-with-porch
spec:
  porchIntegration:
    enabled: true
    repository: https://github.com/my-org/packages
  groups:
    - name: base
      order: 0
      components:
        - name: base-package
          order: 0
          porchPackageRef:
            name: base-infrastructure
            namespace: porch-packages
            revision: v1.0.0
          template:
            apiVersion: v1
            kind: Namespace
            metadata:
              name: my-app
```

### Check Status

```bash
# Get AppBundle status
kubectl get appbundle my-app -o yaml

# Watch deployment progress
kubectl get appbundle my-app -w

# View detailed status
kubectl describe appbundle my-app
```

## API Reference

### AppBundle Spec

| Field | Type | Description |
|-------|------|-------------|
| `groups` | `[]Group` | List of component groups (required) |
| `porchIntegration` | `PorchIntegrationSpec` | Porch integration configuration (optional) |

### Group

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | Unique identifier for the group |
| `order` | `int` | Deployment order (lower = earlier) |
| `components` | `[]Component` | List of components in the group |

### Component

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | Unique identifier for the component |
| `order` | `int` | Deployment order within the group |
| `template` | `runtime.RawExtension` | Kubernetes resource template |
| `porchPackageRef` | `PorchPackageReference` | Reference to Porch package (optional) |

### AppBundle Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | `DeploymentPhase` | Current deployment phase |
| `message` | `string` | Human-readable status message |
| `groupStatuses` | `[]GroupStatus` | Status for each group |
| `observedGeneration` | `int64` | Last observed generation |
| `conditions` | `[]metav1.Condition` | Standard Kubernetes conditions |

## Examples

See the `config/samples/` directory for complete examples:

- **[app_v1alpha1_appbundle.yaml](config/samples/app_v1alpha1_appbundle.yaml)**: Basic three-tier application
- **[app_v1alpha1_appbundle_with_porch.yaml](config/samples/app_v1alpha1_appbundle_with_porch.yaml)**: Using Porch packages
- **[app_v1alpha1_appbundle_microservices.yaml](config/samples/app_v1alpha1_appbundle_microservices.yaml)**: Microservices deployment

## Argo CD Integration

When deployed in an Argo CD-managed cluster, the operator automatically adds sync wave annotations:

```yaml
# Deployed resource will have:
metadata:
  annotations:
    argocd.argoproj.io/sync-wave: "103"  # Group 1, Component 3
  labels:
    app.example.com/appbundle: my-app
    app.example.com/group: database
    app.example.com/component: db-deployment
```

This ensures:
1. Resources are synced in the correct order
2. Dependencies are satisfied before dependent resources
3. Rollback and sync operations maintain ordering

## Porch Integration

### Overview

Porch (Package Orchestration for Resource Configuration Handling) is a Kubernetes-native package management system. The AppBundle operator integrates with Porch for:

- **Package Lifecycle Management**: Track package versions and revisions
- **Centralized Repository**: Store and manage application packages
- **Version Control**: Rollback and upgrade packages easily

### Setup

1. Install Porch in your cluster
2. Create a Porch repository
3. Publish packages to the repository
4. Reference packages in AppBundle components

### Implementation Status

The Porch integration framework is in place with placeholder methods. To complete the integration:

1. Add Porch API dependencies:
   ```bash
   go get github.com/GoogleContainerTools/kpt/porch/api/porch/v1alpha1
   go get github.com/GoogleContainerTools/kpt/porch/api/porchconfig/v1alpha1
   ```

2. Implement the methods in `internal/porch/porch_client.go`

3. Update the controller to fetch package contents from Porch

See `internal/porch/porch_client.go` for detailed integration notes.

## Development

### Prerequisites

- Go 1.21+
- Kubernetes cluster (kind, minikube, or remote)
- kubectl
- kubebuilder 3.0+

### Build and Run Locally

```bash
# Install dependencies
go mod download

# Generate code and manifests
make generate
make manifests

# Install CRDs
make install

# Run locally
make run
```

### Run Tests

```bash
# Run unit tests
make test

# Run e2e tests (requires cluster)
make test-e2e
```

### Build Docker Image

```bash
make docker-build IMG=<your-registry>/appbundle-operator:tag
make docker-push IMG=<your-registry>/appbundle-operator:tag
```

## Architecture Details

### Controller Logic

1. **Reconciliation Loop**:
   - Fetch AppBundle CR
   - Validate and initialize status
   - Process Porch packages (if enabled)
   - Sort groups by order
   - Deploy groups sequentially
   - Update status

2. **Group Processing**:
   - Sort components by order
   - Calculate base sync wave (group.order × 100)
   - Deploy components sequentially
   - Track group status

3. **Component Processing**:
   - Parse component template
   - Add Argo CD sync wave annotation
   - Add tracking labels
   - Set owner references
   - Create or update resource
   - Track component status

### Sync Wave Calculation

```
syncWave = (group.order × 100) + component.order
```

Examples:
- Group 0, Component 0 → Sync Wave 0
- Group 0, Component 5 → Sync Wave 5
- Group 2, Component 3 → Sync Wave 203
- Group 10, Component 1 → Sync Wave 1001

This ensures:
- All components in Group 0 deploy before Group 1
- Components within a group deploy in order
- Up to 100 components per group without conflicts

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Resources

- [Kubebuilder Documentation](https://book.kubebuilder.io/)
- [Argo CD Sync Waves](https://argo-cd.readthedocs.io/en/stable/user-guide/sync-waves/)
- [Porch Documentation](https://github.com/GoogleContainerTools/kpt/tree/main/porch)
- [Kubernetes Operator Pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

## Roadmap

- [ ] Complete Porch integration implementation
- [ ] Add webhook validation for AppBundle resources
- [ ] Implement rollback functionality
- [ ] Add metrics and monitoring
- [ ] Support for health checks and readiness gates
- [ ] Helm chart for operator deployment
- [ ] Enhanced status reporting with events
- [ ] Multi-cluster deployment support
