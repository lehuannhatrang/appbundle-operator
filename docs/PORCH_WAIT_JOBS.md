# Porch Wait Jobs for Sequential Deployment

## Overview

When using Porch packages with the AppBundle Operator, the controller automatically injects **wait Jobs** into each package using Starlark mutators. These Jobs use Argo CD hooks to ensure that resources in one group are fully ready before the next group starts deploying.

This solves the critical problem of **group-level synchronization** where groups have proper sync wave ordering, but Argo CD doesn't wait for resources within a group to be ready before proceeding to the next group.

## The Problem

Without wait Jobs:
```
Group 0 (infrastructure): Creates namespace
  ↓
Group 1 (database): Deploys MongoDB StatefulSet
  ↓ (Argo CD immediately proceeds without waiting)
Group 2 (application): Tries to connect to MongoDB... ❌ FAILS (MongoDB not ready yet)
```

With wait Jobs:
```
Group 0 (infrastructure): Creates namespace
  ↓
Group 1 (database): Deploys MongoDB StatefulSet
  ↓ Wait Job checks: kubectl rollout status statefulset/mongodb ✅
  ↓ (Proceeds only after MongoDB is ready)
Group 2 (application): Connects to MongoDB... ✅ SUCCESS
```

## How It Works

### 1. Automatic Injection

For every component with `porchPackageRef`, the controller adds a Starlark mutator to the PackageVariant that:

1. **Scans all resources** in the package
2. **Identifies workload resources** (Deployments, StatefulSets, DaemonSets, Jobs)
3. **Generates wait commands** for each resource
4. **Injects a wait Job** into the package with appropriate Argo CD hooks

### 2. Starlark Mutator

The controller adds this mutator to the PackageVariant pipeline:

```yaml
- image: gcr.io/kpt-fn/starlark:v0.5.3
  configMap:
    source: |
      load("kpt", "ResourceList")
      def transform(resource_list: ResourceList):
          wait_commands = []
          
          # Scan resources and generate wait commands
          for resource in resource_list["items"]:
              kind = resource.get("kind", "")
              metadata = resource.get("metadata", {})
              name = metadata.get("name", "")
              ns = metadata.get("namespace", "")
              
              if kind == "Deployment":
                  wait_commands.append(f"kubectl rollout status deployment/{name} -n {ns} --timeout=15m")
              elif kind == "StatefulSet":
                  wait_commands.append(f"kubectl rollout status statefulset/{name} -n {ns} --timeout=15m")
              # ... etc
          
          # Create wait Job with Argo CD hooks
          job_yaml = {
              "metadata": {
                  "annotations": {
                      "argocd.argoproj.io/hook": "Sync",
                      "argocd.argoproj.io/hook-delete-policy": "HookSucceeded"
                  }
              },
              "spec": {
                  "template": {
                      "spec": {
                          "containers": [{
                              "args": [" && ".join(wait_commands)]
                          }]
                      }
                  }
              }
          }
          resource_list["items"].append(job_yaml)
          return resource_list
```

### 3. Generated Wait Job

The Starlark mutator creates a Job like this:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: wait-database-mongodb
  namespace: free5gc
  annotations:
    # KEY: This makes Argo CD execute the Job during sync and wait for it
    argocd.argoproj.io/hook: "Sync"
    # Clean up the Job after it succeeds
    argocd.argoproj.io/hook-delete-policy: "HookSucceeded"
    # Sync wave between groups (e.g., 150 is between group 1 and group 2)
    argocd.argoproj.io/sync-wave: "150"
  labels:
    app.example.com/appbundle: my-app
    app.example.com/group: database
    app.example.com/component: mongodb
    app.example.com/wait-job: "true"
spec:
  ttlSecondsAfterFinished: 300  # Clean up 5 minutes after completion
  backoffLimit: 3
  template:
    spec:
      restartPolicy: Never
      serviceAccountName: appbundle-wait-reader
      containers:
        - name: wait
          image: bitnami/kubectl:latest
          command: ["sh", "-c"]
          args:
            # Waits for all workloads to be ready
            - kubectl rollout status statefulset/mongodb -n free5gc --timeout=15m && 
              kubectl rollout status deployment/mongo-express -n free5gc --timeout=15m
```

## Sync Wave Placement

The wait Job's sync wave is strategically placed **between groups**:

```
Group Order | Component Sync Waves | Wait Job Sync Wave
------------|---------------------|-------------------
Group 0     | 0-49                | 50 (at end of group)
Group 1     | 100-149             | 150 (at end of group)
Group 2     | 200-249             | 250 (at end of group)
Group 3     | 300-349             | 350 (at end of group)
```

Formula: `waitSyncWave = (groupOrder * 100) + 50`

This ensures the wait Job runs **after all resources in the group** but **before the next group starts**.

## Example Scenario

### AppBundle Definition

```yaml
apiVersion: app.example.com/v1alpha1
kind: AppBundle
metadata:
  name: free5gc-deployment
spec:
  porchIntegration:
    enabled: true
    repository: mgmt
  
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
              name: free5gc
    
    - name: database
      order: 1
      components:
        - name: mongodb
          order: 0
          porchPackageRef:
            packageName: mongodb
            repository: catalog-databases
    
    - name: core-network
      order: 2
      components:
        - name: amf
          order: 0
          porchPackageRef:
            packageName: free5gc-amf
            repository: catalog-5g
```

### Deployment Flow

1. **Group 0 (infrastructure)**: Sync wave 0
   - Namespace created
   - No wait Job (no workload resources)

2. **Group 1 (database)**: Sync wave 100
   - MongoDB StatefulSet deployed (sync wave 100)
   - **Wait Job injected** (sync wave 150)
     - Waits for: `kubectl rollout status statefulset/mongodb -n free5gc`
     - ✅ Job succeeds when MongoDB is ready

3. **Group 2 (core-network)**: Sync wave 200
   - AMF Deployment deployed (sync wave 200)
   - Only starts after wait Job from Group 1 succeeds
   - **Wait Job injected** (sync wave 250)
     - Waits for: `kubectl rollout status deployment/amf -n free5gc`

## Supported Resource Types

The wait Job automatically handles these resource types:

| Resource Type | Wait Command |
|--------------|-------------|
| Deployment | `kubectl rollout status deployment/<name> -n <ns> --timeout=15m` |
| StatefulSet | `kubectl rollout status statefulset/<name> -n <ns> --timeout=15m` |
| DaemonSet | `kubectl rollout status daemonset/<name> -n <ns> --timeout=15m` |
| Job | `kubectl wait --for=condition=complete job/<name> -n <ns> --timeout=15m` |

Resources not in this list (Services, ConfigMaps, etc.) are ignored since they don't need readiness checks.

## RBAC Requirements

The wait Jobs use a ServiceAccount called `appbundle-wait-reader` that needs permissions to check resource status:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: appbundle-wait-reader
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: appbundle-wait-reader
rules:
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets", "daemonsets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["batch"]
    resources: ["jobs"]
    verbs: ["get", "list", "watch"]
```

**Important**: This ServiceAccount must be created in each namespace where wait Jobs will run, or use a ClusterRoleBinding for cluster-wide access.

The RBAC resources are automatically deployed with the operator in `config/rbac/wait_job_rbac.yaml`.

## Verification

### Check Generated Wait Job

After deploying an AppBundle with Porch components:

```bash
# List all wait Jobs
kubectl get job -l app.example.com/wait-job=true

# Check wait Job for a specific component
kubectl get job wait-database-mongodb -o yaml

# View wait Job logs
kubectl logs job/wait-database-mongodb
```

### Check PackageVariant Mutators

```bash
# View the PackageVariant to see the Starlark mutator
kubectl get packagevariant appbundle-mongodb -o yaml

# Look for the pipeline.mutators section with starlark image
```

### Monitor Argo CD Sync

In Argo CD UI:
1. Go to your AppBundle application
2. Click "Sync"
3. Watch the sync waves progress
4. You'll see wait Jobs appear between groups
5. Observe Argo CD waiting for each Job to complete

## Troubleshooting

### Wait Job Fails

If a wait Job fails:

```bash
# Check the Job status
kubectl describe job wait-database-mongodb

# View the logs to see which resource failed
kubectl logs job/wait-database-mongodb

# Common issues:
# - Resource not ready within timeout (15 minutes)
# - Resource in CrashLoopBackOff
# - ServiceAccount lacks permissions
```

### Wait Job Not Created

If no wait Job appears:

1. **Check PackageVariant mutators**:
   ```bash
   kubectl get packagevariant appbundle-mongodb -o jsonpath='{.spec.pipeline.mutators}'
   ```
   Should show the Starlark mutator.

2. **Check package resources**:
   No wait Job is created if the package contains only infrastructure resources (ConfigMaps, Services, etc.).

3. **Check Starlark function logs** (if available):
   Porch executes mutators as containers - check for execution errors.

### ServiceAccount Missing

If wait Jobs fail with permission errors:

```bash
# Create the ServiceAccount in the target namespace
kubectl create serviceaccount appbundle-wait-reader -n free5gc

# Bind it to the ClusterRole
kubectl create clusterrolebinding appbundle-wait-reader-free5gc \
  --clusterrole=appbundle-wait-reader \
  --serviceaccount=free5gc:appbundle-wait-reader
```

## Benefits

✅ **True Sequential Deployment** - Groups wait for each other  
✅ **Automatic Discovery** - No manual wait command specification  
✅ **Argo CD Integration** - Uses native hooks for reliability  
✅ **Self-Cleaning** - Jobs deleted after success  
✅ **Resource-Aware** - Only waits for workload resources  
✅ **Namespace-Aware** - Correctly handles multi-namespace deployments  

## Advanced Configuration

### Custom Timeout

The default timeout is 15 minutes. To customize, you would need to modify the Starlark script in the controller.

### Multiple Resources

The wait Job waits for **all** workload resources in sequence using `&&`:

```bash
kubectl rollout status deployment/app1 -n ns --timeout=15m && \
kubectl rollout status deployment/app2 -n ns --timeout=15m && \
kubectl rollout status statefulset/db -n ns --timeout=15m
```

If any resource fails, the entire Job fails, and Argo CD halts the sync.

### Skip Wait Job

Currently, wait Jobs are automatically added to all Porch components. Future enhancements could add:
- Annotation to disable: `app.example.com/skip-wait-job: "true"`
- Custom wait logic per component
- Parallel waiting instead of sequential

## Comparison with Alternatives

| Approach | Pros | Cons |
|----------|------|------|
| **Wait Jobs (This Implementation)** | ✅ Automatic<br>✅ Reliable<br>✅ Argo CD native | ❌ Requires RBAC setup |
| **Argo CD Sync Waves Only** | ✅ Simple | ❌ Doesn't wait for readiness<br>❌ Race conditions |
| **Argo CD Health Checks** | ✅ Built-in | ❌ Limited resource types<br>❌ No cross-group waiting |
| **Manual kubectl wait** | ✅ Direct control | ❌ Not GitOps<br>❌ Hard to maintain |

## References

- [Argo CD Sync Hooks](https://argo-cd.readthedocs.io/en/stable/user-guide/resource_hooks/)
- [Argo CD Sync Waves](https://argo-cd.readthedocs.io/en/stable/user-guide/sync-waves/)
- [KPT Starlark Function](https://catalog.kpt.dev/starlark/v0.5/)
- [kubectl wait Command](https://kubernetes.io/docs/reference/generated/kubectl/kubectl-commands#wait)

