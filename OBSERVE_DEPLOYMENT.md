# Observing Ordered Deployment with Delays

This guide shows you how to observe the AppBundle operator's ordered deployment feature using the sample AppBundle with 10-second init container delays.

## Overview

The sample AppBundle now includes init containers that sleep for 10 seconds before starting the main containers. This makes the ordered deployment very visible and easy to observe.

## Deployment Order

```
Group 0 (order: 0) - Infrastructure
├─ Component 0: namespace (sync-wave: 0)
└─ Component 1: configmap (sync-wave: 1)

Group 1 (order: 1) - Database
├─ Component 0: db-secret (sync-wave: 100)
├─ Component 1: db-service (sync-wave: 101)
└─ Component 2: db-deployment (sync-wave: 102) ⏱️ +10s delay

Group 2 (order: 2) - Application
├─ Component 0: app-service (sync-wave: 200)
└─ Component 1: app-deployment (sync-wave: 201) ⏱️ +10s delay
```

## Watch the Deployment in Action

### Method 1: Watch Pods (Recommended)

Open **3 terminals** for the best view:

**Terminal 1: Watch AppBundle Status**
```bash
watch -n 1 'kubectl get appbundle appbundle-sample -o jsonpath="{.status.phase}" && echo && kubectl get appbundle appbundle-sample -o jsonpath="{.status.message}"'
```

**Terminal 2: Watch Pods**
```bash
watch -n 1 'kubectl get pods -n sample-app'
```

**Terminal 3: Watch Operator Logs**
```bash
kubectl logs -n appbundle-system deployment/appbundle-controller-manager -f
```

Then in a **4th terminal**, apply the AppBundle:
```bash
kubectl apply -f config/samples/app_v1alpha1_appbundle.yaml
```

### Method 2: Watch with kubectl

Single command to watch everything:

```bash
# Apply the AppBundle
kubectl apply -f config/samples/app_v1alpha1_appbundle.yaml

# Watch pods being created
kubectl get pods -n sample-app -w
```

You'll see:
1. **No pods initially** (namespace just created)
2. **Database pod appears** → Init container starts (10s wait)
3. **Database pod running** → Main container starts
4. **Web-app pods appear** → Init containers start (10s wait each)
5. **Web-app pods running** → Main containers start

### Method 3: Timeline View

Track the deployment timeline:

```bash
# Apply
kubectl apply -f config/samples/app_v1alpha1_appbundle.yaml

# Watch events in real-time
kubectl get events -n sample-app --watch --sort-by='.lastTimestamp'
```

You'll see events like:
- `0s` - Namespace created
- `0s` - ConfigMap created
- `1s` - Secret created
- `1s` - Services created
- `2s` - Database deployment created
- `2s` - Database pod scheduled
- `2s` - Database init container started
- `12s` - Database init container completed (after 10s)
- `12s` - Database main container started
- `13s` - Web-app deployment created
- `13s` - Web-app pods scheduled
- `13s` - Web-app init containers started
- `23s` - Web-app init containers completed (after 10s)
- `23s` - Web-app main containers started

## Detailed Observation Commands

### 1. Check AppBundle Status in Detail

```bash
# Overall status
kubectl get appbundle appbundle-sample

# Detailed status (including group and component statuses)
kubectl get appbundle appbundle-sample -o yaml | grep -A 50 "status:"

# Just the phase
kubectl get appbundle appbundle-sample -o jsonpath='{.status.phase}'
```

### 2. Watch Component Deployment Progress

```bash
# See which components are being deployed
kubectl get appbundle appbundle-sample -o jsonpath='{.status.groupStatuses[*].componentStatuses[*].name}' | tr ' ' '\n'

# See component phases
kubectl get appbundle appbundle-sample -o jsonpath='{.status.groupStatuses[*].componentStatuses[*].phase}' | tr ' ' '\n'
```

### 3. Watch Init Containers

```bash
# See init container status
kubectl get pods -n sample-app -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.initContainerStatuses[0].state}{"\n"}{end}'

# Watch init container logs (database)
kubectl logs -n sample-app -l app=database -c wait-for-dependencies -f

# Watch init container logs (web-app)
kubectl logs -n sample-app -l app=web-app -c wait-for-dependencies -f
```

### 4. Track Sync Waves

Check the sync wave annotations on created resources:

```bash
# View sync waves
kubectl get all -n sample-app -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.argocd\.argoproj\.io/sync-wave}{"\n"}{end}' | sort -k2 -n
```

Example output:
```
database        101
database        102
web-app         200
web-app         201
```

## Understanding the Timing

### Expected Timeline

| Time | Group | Component | Action |
|------|-------|-----------|--------|
| 0s | Infrastructure | namespace | Created |
| 0s | Infrastructure | configmap | Created |
| ~1s | Database | db-secret | Created |
| ~1s | Database | db-service | Created |
| ~2s | Database | db-deployment | Pod scheduled |
| ~2s | Database | db-deployment | Init container starts |
| **12s** | Database | db-deployment | **Init container done (10s wait)** |
| 12s | Database | db-deployment | Main container starts |
| ~13s | Application | app-service | Created |
| ~14s | Application | app-deployment | Pods scheduled |
| ~14s | Application | app-deployment | Init containers start (3 replicas) |
| **24s** | Application | app-deployment | **Init containers done (10s wait)** |
| 24s | Application | app-deployment | Main containers start |

**Total deployment time**: ~30 seconds

### Why Init Containers?

Init containers are perfect for demonstrating ordered deployment because:

1. ✅ **Visible Delay**: The 10s sleep makes the ordering easy to see
2. ✅ **Pod Status**: Shows as "Init:0/1" while waiting
3. ✅ **Logs**: Can view init container logs to see the wait message
4. ✅ **Kubernetes Native**: Uses standard Kubernetes feature
5. ✅ **Non-Intrusive**: Doesn't affect the main application

## Verification Steps

After deployment completes:

### 1. Verify All Resources

```bash
# Check all resources in sample-app namespace
kubectl get all -n sample-app

# Expected output:
# - 1 database pod (Running)
# - 3 web-app pods (Running)
# - 2 services (database, web-app)
# - 2 deployments (database, web-app)
# - 1 replicaset per deployment
```

### 2. Check AppBundle Final Status

```bash
kubectl get appbundle appbundle-sample -o jsonpath='{.status.phase}'
# Should output: Deployed

kubectl get appbundle appbundle-sample -o jsonpath='{.status.message}'
# Should output: All groups deployed successfully
```

### 3. Verify Init Containers Ran

```bash
# Check init container logs (should show the wait message)
kubectl logs -n sample-app -l app=database -c wait-for-dependencies
# Expected: "Waiting 10s before starting database..."
#          "Starting database now"

kubectl logs -n sample-app -l app=web-app -c wait-for-dependencies --tail=3
# Expected: "Waiting 10s before starting web app..."
#          "Starting web app now"
```

## Cleanup and Re-test

To see the deployment again:

```bash
# Delete the AppBundle
kubectl delete appbundle appbundle-sample

# Wait for cleanup (should take ~5s)
kubectl get namespace sample-app
# Should return: Error from server (NotFound)

# Deploy again to see the ordering
kubectl apply -f config/samples/app_v1alpha1_appbundle.yaml
```

## Advanced: Custom Wait Times

You can modify the wait time in the sample YAML:

```yaml
initContainers:
  - name: wait-for-dependencies
    image: busybox:1.36
    command: ['sh', '-c', 'echo "Waiting 20s..."; sleep 20; echo "Done"']
    #                                           ^^^ Change this value
```

**Suggested wait times**:
- **5s**: Quick demo
- **10s**: Good for watching (default)
- **30s**: Very visible for presentations
- **60s**: Exaggerated for teaching

## Troubleshooting

### Init Container Stuck

If an init container doesn't complete:

```bash
# Check init container status
kubectl describe pod -n sample-app <pod-name>

# Check init container logs
kubectl logs -n sample-app <pod-name> -c wait-for-dependencies
```

### Pods Not Appearing

If pods don't appear in sample-app namespace:

```bash
# Check operator logs for errors
kubectl logs -n appbundle-system deployment/appbundle-controller-manager --tail=50

# Check AppBundle status
kubectl describe appbundle appbundle-sample
```

### Image Pull Issues

If busybox image fails to pull:

```bash
# Check pod events
kubectl describe pod -n sample-app <pod-name> | grep -A 10 Events

# Try a different image
# Replace busybox:1.36 with alpine:3.18 in the sample YAML
```

## Script: Automated Observation

Save this as `watch-deployment.sh`:

```bash
#!/bin/bash
echo "Starting AppBundle deployment watch..."
echo "Press Ctrl+C to stop"
echo ""

# Start watching in background
kubectl get pods -n sample-app -w &
WATCH_PID=$!

# Apply the AppBundle
echo "Applying AppBundle..."
kubectl apply -f config/samples/app_v1alpha1_appbundle.yaml

# Wait for completion
echo ""
echo "Waiting for deployment to complete..."
kubectl wait --for=condition=Ready --timeout=60s appbundle/appbundle-sample 2>/dev/null || true

# Stop watching
kill $WATCH_PID 2>/dev/null

echo ""
echo "Final status:"
kubectl get appbundle appbundle-sample
kubectl get pods -n sample-app
```

Run it:
```bash
chmod +x watch-deployment.sh
./watch-deployment.sh
```

## Summary

The 10-second init container delays make the AppBundle operator's ordered deployment feature **highly visible and easy to demonstrate**. This is perfect for:

- 🎓 **Learning**: Understanding how sync waves work
- 🧪 **Testing**: Verifying ordered deployment
- 📊 **Demos**: Showing the feature to others
- 🐛 **Debugging**: Seeing each step clearly

Happy watching! 🎉

