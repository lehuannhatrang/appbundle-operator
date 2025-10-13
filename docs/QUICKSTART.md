# AppBundle Operator - Quick Start Guide

This guide will help you get started with the AppBundle Operator in 5 minutes.

## Prerequisites

- Kubernetes cluster (v1.19+)
- kubectl configured to access your cluster
- (Optional) Argo CD installed for sync wave testing

## Step 1: Install the Operator

### Option A: Install from Source

```bash
# Clone the repository
git clone <repository-url>
cd appbundle-operator

# Install CRDs
make install

# Deploy the operator
make deploy IMG=<your-registry>/appbundle-operator:latest
```

### Option B: Run Locally (Development)

```bash
# Install CRDs
make install

# Run the operator locally
make run
```

## Step 2: Create Your First AppBundle

Create a file named `my-first-appbundle.yaml`:

```yaml
apiVersion: app.example.com/v1alpha1
kind: AppBundle
metadata:
  name: my-first-app
  namespace: default
spec:
  groups:
    # Group 0: Create namespace first
    - name: setup
      order: 0
      components:
        - name: app-namespace
          order: 0
          template:
            apiVersion: v1
            kind: Namespace
            metadata:
              name: demo-app
              labels:
                managed-by: appbundle-operator
    
    # Group 1: Deploy application
    - name: application
      order: 1
      components:
        - name: nginx-deployment
          order: 0
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: nginx
              namespace: demo-app
            spec:
              replicas: 2
              selector:
                matchLabels:
                  app: nginx
              template:
                metadata:
                  labels:
                    app: nginx
                spec:
                  containers:
                    - name: nginx
                      image: nginx:latest
                      ports:
                        - containerPort: 80
        
        - name: nginx-service
          order: 1
          template:
            apiVersion: v1
            kind: Service
            metadata:
              name: nginx
              namespace: demo-app
            spec:
              selector:
                app: nginx
              ports:
                - port: 80
                  targetPort: 80
              type: LoadBalancer
```

Apply it:

```bash
kubectl apply -f my-first-appbundle.yaml
```

## Step 3: Check the Status

Watch the deployment progress:

```bash
# Watch AppBundle status
kubectl get appbundle my-first-app -w

# Get detailed status
kubectl describe appbundle my-first-app

# Check deployed resources
kubectl get all -n demo-app
```

Expected output:

```
NAME                          PHASE      MESSAGE
appbundle/my-first-app        Deployed   All groups deployed successfully
```

## Step 4: Verify Argo CD Integration

If you have Argo CD installed, check the sync wave annotations:

```bash
# Check namespace (should have sync-wave: "0")
kubectl get namespace demo-app -o jsonpath='{.metadata.annotations}'

# Check deployment (should have sync-wave: "100")
kubectl get deployment nginx -n demo-app -o jsonpath='{.metadata.annotations}'

# Check service (should have sync-wave: "101")
kubectl get service nginx -n demo-app -o jsonpath='{.metadata.annotations}'
```

## Step 5: Update the AppBundle

Let's scale up the deployment by modifying the AppBundle:

```yaml
# Edit my-first-appbundle.yaml
# Change replicas from 2 to 5 in the nginx-deployment component
spec:
  groups:
    - name: application
      order: 1
      components:
        - name: nginx-deployment
          order: 0
          template:
            # ... other fields ...
            spec:
              replicas: 5  # Changed from 2
```

Apply the changes:

```bash
kubectl apply -f my-first-appbundle.yaml

# Watch the update
kubectl get deployment nginx -n demo-app -w
```

## Step 6: Clean Up

Delete the AppBundle (this will cascade delete all managed resources):

```bash
kubectl delete appbundle my-first-app

# Verify resources are deleted
kubectl get all -n demo-app
```

## Next Steps

### Try a More Complex Example

Deploy a three-tier application:

```bash
kubectl apply -f config/samples/app_v1alpha1_appbundle.yaml
```

This example includes:
- Infrastructure setup (namespace, configmap)
- Database tier (secrets, service, deployment)
- Application tier (service, deployment)

### Explore Porch Integration

If you have Porch installed:

```bash
kubectl apply -f config/samples/app_v1alpha1_appbundle_with_porch.yaml
```

### Deploy a Microservices Application

```bash
kubectl apply -f config/samples/app_v1alpha1_appbundle_microservices.yaml
```

## Understanding Deployment Order

The AppBundle Operator deploys resources in a specific order:

1. **Groups are sorted by their `order` field** (ascending)
2. **Components within each group are sorted by their `order` field**
3. **Each group completes before the next group starts**
4. **Argo CD sync waves are automatically calculated**:
   - Sync Wave = (Group Order × 100) + Component Order

Example from above:
- Group 0 (setup), Component 0 (namespace) → Sync Wave 0
- Group 1 (application), Component 0 (deployment) → Sync Wave 100
- Group 1 (application), Component 1 (service) → Sync Wave 101

This ensures:
- Namespace is created first
- Deployment is created before Service
- Argo CD syncs in the same order

## Troubleshooting

### AppBundle stuck in "Deploying" phase

```bash
# Check controller logs
kubectl logs -n appbundle-operator-system deployment/appbundle-operator-controller-manager

# Check AppBundle status for errors
kubectl get appbundle <name> -o yaml | grep -A 10 status
```

### Resources not being created

1. Check RBAC permissions:
```bash
kubectl get clusterrolebinding | grep appbundle
```

2. Verify CRDs are installed:
```bash
kubectl get crd appbundles.app.example.com
```

3. Check for validation errors:
```bash
kubectl describe appbundle <name>
```

### Operator not running

```bash
# Check operator deployment
kubectl get deployment -n appbundle-operator-system

# Check operator logs
kubectl logs -n appbundle-operator-system deployment/appbundle-operator-controller-manager -f
```

## Common Patterns

### Database-First Pattern

Always deploy databases and stateful services before applications:

```yaml
groups:
  - name: stateful
    order: 0
    components:
      - name: database
        order: 0
  - name: stateless
    order: 1
    components:
      - name: api-server
        order: 0
```

### Infrastructure-Application-Routing Pattern

Common three-tier pattern:

```yaml
groups:
  - name: infrastructure
    order: 0  # Namespaces, ConfigMaps, Secrets
  - name: application
    order: 1  # Deployments, StatefulSets
  - name: networking
    order: 2  # Services, Ingresses
```

### Canary Deployment Pattern

Deploy stable version first, then canary:

```yaml
groups:
  - name: stable
    order: 0
    components:
      - name: app-v1
        order: 0
  - name: canary
    order: 1
    components:
      - name: app-v2
        order: 0
```

## Further Reading

- [Main README](../README.md) - Complete documentation
- [API Reference](../README.md#api-reference) - Detailed API documentation
- [Examples](../config/samples/) - More example AppBundles
- [Porch Integration](../internal/porch/porch_client.go) - Porch integration guide

## Getting Help

- Check the [README](../README.md) for detailed documentation
- Review [examples](../config/samples/) for common patterns
- Check operator logs for errors
- Review Kubernetes events: `kubectl get events -n <namespace>`

Happy bundling! 🚀

