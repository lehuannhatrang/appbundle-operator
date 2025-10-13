# AppBundle Operator Architecture

## Overview

The AppBundle Operator is a Kubernetes operator that manages the deployment of complex applications with ordered resource creation and Argo CD sync wave integration.

## Components

### 1. Custom Resource Definition (CRD)

**AppBundle**: The core custom resource that defines an application bundle.

```
AppBundle
├── Metadata (name, namespace, labels, annotations)
├── Spec
│   ├── Groups[]
│   │   ├── Name
│   │   ├── Order (deployment priority)
│   │   └── Components[]
│   │       ├── Name
│   │       ├── Order (within group)
│   │       ├── Template (K8s resource)
│   │       └── PorchPackageRef (optional)
│   └── PorchIntegration
│       ├── Enabled
│       └── Repository
└── Status
    ├── Phase (Pending|Deploying|Deployed|Failed)
    ├── Message
    ├── GroupStatuses[]
    │   ├── Name
    │   ├── Phase
    │   ├── Message
    │   └── ComponentStatuses[]
    │       ├── Name
    │       ├── Phase
    │       ├── Message
    │       └── ResourceRef
    ├── ObservedGeneration
    └── Conditions[]
```

### 2. Controller

**AppBundleReconciler**: The main controller that watches and reconciles AppBundle resources.

**Responsibilities**:
- Watch AppBundle custom resources
- Manage finalizers for cleanup
- Sort and deploy groups in order
- Sort and deploy components within groups
- Add Argo CD sync wave annotations
- Set owner references for garbage collection
- Update status and conditions
- Integrate with Porch (when enabled)

### 3. Porch Integration

**Location**: `internal/porch/porch_client.go`

**Purpose**: Interface with Porch for package lifecycle management.

**Capabilities** (to be implemented):
- Fetch package revisions
- List packages from repositories
- Get package contents
- Watch for package changes

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                         User/GitOps                          │
│                    (applies AppBundle CR)                    │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes API Server                     │
│              (stores AppBundle custom resource)              │
└────────┬────────────────────────────────────┬───────────────┘
         │                                    │
         │ watches                            │ reads/writes
         ▼                                    ▼
┌──────────────────────┐         ┌─────────────────────────────┐
│  AppBundle Controller│         │   Porch API (optional)      │
│                      │         │  (package management)       │
│  Reconciliation Loop │◄────────┤                             │
│  ├─ Get AppBundle    │ queries │  - PackageRevision CRD      │
│  ├─ Check Finalizers │         │  - Repository CRD           │
│  ├─ Porch Integration│         │  - Package contents         │
│  ├─ Sort Groups      │         └─────────────────────────────┘
│  ├─ Deploy Resources │
│  └─ Update Status    │
└────────┬─────────────┘
         │
         │ creates/updates
         ▼
┌─────────────────────────────────────────────────────────────┐
│                  Kubernetes Resources                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │Namespace │  │ConfigMap │  │  Secret  │  │ Service  │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘   │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                  │
│  │Deployment│  │StatefulSet│ │  Ingress │  ...             │
│  └──────────┘  └──────────┘  └──────────┘                  │
│                                                              │
│  (with sync-wave annotations & owner references)            │
└─────────────────────────────────────────────────────────────┘
         │
         │ syncs (if Argo CD installed)
         ▼
┌─────────────────────────────────────────────────────────────┐
│                        Argo CD (optional)                    │
│  - Reads sync-wave annotations                              │
│  - Syncs resources in order                                 │
│  - Reports sync status                                      │
└─────────────────────────────────────────────────────────────┘
```

## Reconciliation Flow

### Main Reconciliation Loop

```
┌─────────────────────────────────────────────────────────────┐
│                    Reconcile Triggered                       │
│         (AppBundle created/updated/deleted/resync)          │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
                  ┌──────────────┐
                  │ Fetch AppBundle│
                  └──────┬─────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │ Resource exists?    │
              └──────┬──────────────┘
                     │
        ┌────────────┴────────────┐
        │ No                      │ Yes
        │                         ▼
        │              ┌──────────────────┐
        │              │ Deletion pending?│
        │              └──────┬───────────┘
        │                     │
        │           ┌─────────┴─────────┐
        │           │ Yes               │ No
        │           ▼                   ▼
        │  ┌────────────────┐   ┌──────────────┐
        │  │  Run Finalizer │   │Add Finalizer │
        │  │  Clean up      │   │(if needed)   │
        │  │  Remove Finalizer│ └──────┬───────┘
        │  └────────────────┘          │
        │                              ▼
        │                   ┌──────────────────┐
        │                   │Initialize Status │
        │                   │(if needed)       │
        │                   └──────┬───────────┘
        │                          │
        │                          ▼
        │                   ┌──────────────────┐
        │                   │Porch Integration?│
        │                   └──────┬───────────┘
        │                          │
        │                ┌─────────┴─────────┐
        │                │ Yes               │ No
        │                ▼                   │
        │         ┌──────────────┐           │
        │         │Reconcile Porch│          │
        │         │   Packages    │          │
        │         └──────┬────────┘          │
        │                │                   │
        │                └─────────┬─────────┘
        │                          │
        │                          ▼
        │                   ┌──────────────┐
        │                   │ Sort Groups  │
        │                   │  by order    │
        │                   └──────┬───────┘
        │                          │
        │                          ▼
        │                   ┌──────────────────┐
        │                   │For each Group:   │
        │                   │- Sort Components │
        │                   │- Deploy in order │
        │                   │- Update status   │
        │                   └──────┬───────────┘
        │                          │
        │                          ▼
        │                   ┌──────────────────┐
        │                   │ Update Overall   │
        │                   │     Status       │
        │                   └──────────────────┘
        │                          
        └─────────────┬────────────┘
                      │
                      ▼
              ┌──────────────┐
              │Return Result │
              └──────────────┘
```

### Component Deployment Flow

```
┌─────────────────────────────────────────────────────────────┐
│                   Deploy Component                           │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
                  ┌──────────────┐
                  │Parse Template│
                  │  (JSON)      │
                  └──────┬───────┘
                         │
                         ▼
              ┌──────────────────────┐
              │Calculate Sync Wave   │
              │(group.order*100 +    │
              │ component.order)     │
              └──────┬───────────────┘
                     │
                     ▼
              ┌──────────────────────┐
              │Add Annotations:      │
              │- argocd sync-wave    │
              └──────┬───────────────┘
                     │
                     ▼
              ┌──────────────────────┐
              │Add Labels:           │
              │- appbundle name      │
              │- group name          │
              │- component name      │
              └──────┬───────────────┘
                     │
                     ▼
              ┌──────────────────────┐
              │Set Namespace         │
              │(if not specified)    │
              └──────┬───────────────┘
                     │
                     ▼
              ┌──────────────────────┐
              │Set Owner Reference   │
              │(for garbage collect) │
              └──────┬───────────────┘
                     │
                     ▼
              ┌──────────────────────┐
              │Resource exists?      │
              └──────┬───────────────┘
                     │
        ┌────────────┴────────────┐
        │ No                      │ Yes
        │                         │
        ▼                         ▼
┌──────────────┐         ┌────────────────┐
│Create Resource│        │Update Resource │
└──────┬───────┘         └───────┬────────┘
       │                         │
       └──────────┬──────────────┘
                  │
                  ▼
        ┌──────────────────┐
        │Update Component  │
        │    Status        │
        └──────────────────┘
```

## Sync Wave Calculation

The operator calculates Argo CD sync waves to ensure ordered deployment:

```
Sync Wave = (Group Order × 100) + Component Order
```

**Examples**:

| Group | Group Order | Component | Component Order | Sync Wave |
|-------|-------------|-----------|-----------------|-----------|
| infra | 0           | namespace | 0               | 0         |
| infra | 0           | configmap | 1               | 1         |
| db    | 1           | secret    | 0               | 100       |
| db    | 1           | service   | 1               | 101       |
| db    | 1           | deployment| 2               | 102       |
| app   | 2           | deployment| 0               | 200       |
| app   | 2           | service   | 1               | 201       |

**Benefits**:
- Simple and predictable ordering
- Up to 100 components per group without conflicts
- Clear separation between groups
- Compatible with Argo CD's sync wave mechanism

## Status Management

The operator maintains detailed status information:

### Status Phases

1. **Pending**: AppBundle created, deployment not started
2. **Deploying**: Resources are being deployed
3. **Deployed**: All resources successfully deployed
4. **Failed**: Deployment encountered an error

### Status Updates

```
AppBundle Status
├── Phase: Overall deployment phase
├── Message: Human-readable status message
├── GroupStatuses[]: Per-group status
│   ├── Name: Group name
│   ├── Phase: Group deployment phase
│   ├── Message: Group status message
│   └── ComponentStatuses[]: Per-component status
│       ├── Name: Component name
│       ├── Phase: Component deployment phase
│       ├── Message: Component status message
│       └── ResourceRef: Reference to deployed resource
│           ├── APIVersion
│           ├── Kind
│           ├── Name
│           └── Namespace
├── ObservedGeneration: Last processed spec generation
└── Conditions[]: Standard K8s conditions
    └── Ready: Overall readiness condition
```

## RBAC

The controller requires the following permissions:

### AppBundle Resources

```yaml
- apiGroups: [app.example.com]
  resources: [appbundles]
  verbs: [get, list, watch, create, update, patch, delete]

- apiGroups: [app.example.com]
  resources: [appbundles/status]
  verbs: [get, update, patch]

- apiGroups: [app.example.com]
  resources: [appbundles/finalizers]
  verbs: [update]
```

### Managed Resources

```yaml
- apiGroups: ["*"]
  resources: ["*"]
  verbs: [get, list, watch, create, update, patch, delete]
```

**Note**: In production, you should scope these permissions to specific resource types needed by your AppBundles.

## Garbage Collection

Resources are automatically deleted when an AppBundle is deleted through:

1. **Owner References**: All deployed resources have owner references to the AppBundle
2. **Kubernetes GC**: K8s garbage collector automatically deletes owned resources
3. **Finalizers**: Custom cleanup logic can be added in the finalizer

## Error Handling

### Reconciliation Errors

- Errors during deployment stop the current group
- Status is updated with error information
- Failed phase is set
- Kubernetes will retry reconciliation

### Retry Logic

- Built-in controller-runtime retry with exponential backoff
- Manual requeue can be triggered by updating the AppBundle

### Status Reporting

All errors are reported in:
1. AppBundle status message
2. Group status (if group-specific)
3. Component status (if component-specific)
4. Kubernetes conditions
5. Controller logs

## Performance Considerations

### Sequential vs Parallel Deployment

- **Groups**: Deployed sequentially (by design)
- **Components**: Deployed sequentially within a group (by design)
- **Reason**: Ensures ordering and dependency satisfaction

### Resource Limits

- No hard limits on groups or components
- Practical limits depend on Kubernetes API server capacity
- Large AppBundles may have longer reconciliation times

### Optimization Opportunities

Future enhancements could include:
- Parallel deployment within a group (for independent components)
- Batch resource creation
- Caching of resource status
- Event-driven reconciliation triggers

## Integration Points

### Argo CD

- **Sync Waves**: Automatic annotation injection
- **Health Checks**: Argo CD monitors deployed resources
- **GitOps**: AppBundles can be managed in Git repositories

### Porch

- **Package Management**: Fetch resources from Porch packages
- **Version Control**: Track package revisions
- **Repository**: Centralized package storage

### Future Integrations

- **Helm**: Support Helm charts as component templates
- **Kustomize**: Apply Kustomize transformations
- **OPA/Gatekeeper**: Policy validation
- **Prometheus**: Metrics and monitoring

## Security Considerations

1. **RBAC**: Limit operator permissions in production
2. **Network Policies**: Restrict operator network access
3. **Resource Validation**: Add webhook validation (future)
4. **Secrets**: Use sealed secrets or external secret managers
5. **Multi-tenancy**: Namespace isolation for AppBundles

## Testing Strategy

### Unit Tests

- Controller reconciliation logic
- Status update functions
- Sync wave calculation
- Group and component sorting

### Integration Tests

- Full reconciliation flow
- Resource creation and updates
- Finalizer cleanup
- Error handling

### E2E Tests

- Deploy real AppBundles
- Verify resource ordering
- Test Argo CD integration
- Validate status reporting

## Deployment Patterns

### Single Namespace

AppBundle and all resources in one namespace:
```yaml
metadata:
  namespace: my-app
```

### Multi-Namespace

AppBundle in one namespace, resources in others:
```yaml
spec:
  groups:
    - name: group1
      components:
        - template:
            metadata:
              namespace: namespace1
```

### Cluster-Scoped

Deploy cluster-scoped resources:
```yaml
spec:
  groups:
    - name: cluster-resources
      components:
        - template:
            kind: ClusterRole
            # no namespace
```

## Troubleshooting

See [QUICKSTART.md](QUICKSTART.md#troubleshooting) for common issues and solutions.

## References

- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime)
- [Argo CD Sync Waves](https://argo-cd.readthedocs.io/en/stable/user-guide/sync-waves/)
- [Kubernetes Operators](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

