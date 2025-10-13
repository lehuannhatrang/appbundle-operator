# Development Guide

## Prerequisites

- **Go**: Version 1.21 or higher
- **Kubebuilder**: Version 3.0 or higher
- **Docker**: For building images
- **Kubernetes Cluster**: For testing (kind, minikube, or remote cluster)
- **kubectl**: Configured to access your cluster

## Project Structure

```
appbundle-operator/
├── api/
│   └── v1alpha1/
│       ├── appbundle_types.go      # AppBundle CRD definition
│       ├── groupversion_info.go    # API group version info
│       └── zz_generated.deepcopy.go # Generated DeepCopy methods
├── cmd/
│   └── main.go                      # Operator entry point
├── config/
│   ├── crd/                         # CRD manifests
│   ├── default/                     # Default deployment config
│   ├── manager/                     # Manager deployment
│   ├── rbac/                        # RBAC manifests
│   └── samples/                     # Sample AppBundle CRs
├── internal/
│   ├── controller/
│   │   ├── appbundle_controller.go  # Main reconciler
│   │   └── suite_test.go            # Test suite
│   └── porch/
│       └── porch_client.go          # Porch integration
├── docs/
│   ├── ARCHITECTURE.md              # Architecture documentation
│   ├── DEVELOPMENT.md               # This file
│   └── QUICKSTART.md                # Quick start guide
├── Makefile                         # Build automation
├── go.mod                           # Go dependencies
├── go.sum                           # Go dependency checksums
├── Dockerfile                       # Container image
└── README.md                        # Main documentation
```

## Setup Development Environment

### 1. Clone the Repository

```bash
git clone <repository-url>
cd appbundle-operator
```

### 2. Install Dependencies

```bash
# Download Go dependencies
go mod download

# Install kubebuilder (if not already installed)
# Linux/macOS:
curl -L -o kubebuilder https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)
chmod +x kubebuilder && sudo mv kubebuilder /usr/local/bin/

# Verify installation
kubebuilder version
```

### 3. Setup Kubernetes Cluster

Using kind:
```bash
kind create cluster --name appbundle-dev
kubectl cluster-info --context kind-appbundle-dev
```

Using minikube:
```bash
minikube start --profile appbundle-dev
kubectl cluster-info
```

## Development Workflow

### 1. Make Code Changes

Edit files in `api/v1alpha1/` or `internal/controller/`

### 2. Generate Code

```bash
# Generate DeepCopy methods
make generate

# Generate CRD manifests
make manifests
```

### 3. Run Tests

```bash
# Run unit tests
make test

# Run tests with coverage
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### 4. Install CRDs

```bash
make install
```

### 5. Run Locally

```bash
# Run against the configured Kubernetes cluster
make run
```

The operator will:
- Connect to your Kubernetes cluster
- Install CRDs if needed
- Start the controller manager
- Watch for AppBundle resources

### 6. Test Your Changes

In another terminal:

```bash
# Apply a sample AppBundle
kubectl apply -f config/samples/app_v1alpha1_appbundle.yaml

# Watch the operator logs in the first terminal

# Check the status
kubectl get appbundle -w

# Clean up
kubectl delete appbundle appbundle-sample
```

## Build and Deploy

### Build Docker Image

```bash
# Build the image
make docker-build IMG=<your-registry>/appbundle-operator:v0.1.0

# Push to registry
make docker-push IMG=<your-registry>/appbundle-operator:v0.1.0
```

### Deploy to Cluster

```bash
# Deploy the operator
make deploy IMG=<your-registry>/appbundle-operator:v0.1.0

# Check deployment
kubectl get deployment -n appbundle-operator-system

# View logs
kubectl logs -n appbundle-operator-system deployment/appbundle-operator-controller-manager -f
```

### Undeploy

```bash
make undeploy
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make help` | Show all available targets |
| `make generate` | Generate code (DeepCopy, etc.) |
| `make manifests` | Generate CRD and RBAC manifests |
| `make fmt` | Format code |
| `make vet` | Run go vet |
| `make test` | Run unit tests |
| `make build` | Build manager binary |
| `make run` | Run against configured cluster |
| `make install` | Install CRDs |
| `make uninstall` | Uninstall CRDs |
| `make deploy` | Deploy operator to cluster |
| `make undeploy` | Remove operator from cluster |
| `make docker-build` | Build Docker image |
| `make docker-push` | Push Docker image |

## Testing

### Unit Tests

```bash
# Run all tests
make test

# Run specific package tests
go test ./internal/controller/...

# Run with verbose output
go test -v ./...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Integration Tests

Create integration tests in `internal/controller/appbundle_controller_test.go`:

```go
var _ = Describe("AppBundle Controller", func() {
    Context("When reconciling a resource", func() {
        It("should successfully deploy groups in order", func() {
            // Test implementation
        })
    })
})
```

Run with:
```bash
make test
```

### E2E Tests

```bash
# Start the operator
make run &

# Apply test cases
kubectl apply -f config/samples/app_v1alpha1_appbundle.yaml

# Verify deployment
kubectl get appbundle appbundle-sample -o yaml

# Check deployed resources
kubectl get all -n sample-app

# Clean up
kubectl delete appbundle appbundle-sample
```

## Debugging

### Local Debugging with Delve

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Run with debugger
dlv debug cmd/main.go -- 
```

### VSCode Debug Configuration

Create `.vscode/launch.json`:

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Debug Operator",
            "type": "go",
            "request": "launch",
            "mode": "auto",
            "program": "${workspaceFolder}/cmd/main.go",
            "env": {
                "KUBECONFIG": "${env:HOME}/.kube/config"
            },
            "args": []
        }
    ]
}
```

### Remote Debugging

```bash
# Build debug image
docker build -t appbundle-operator:debug --target debug .

# Deploy with debug port exposed
kubectl port-forward -n appbundle-operator-system deployment/appbundle-operator-controller-manager 2345:2345

# Connect with delve
dlv connect localhost:2345
```

### Logging

Increase log verbosity:

```bash
# Run with verbose logging
make run ARGS="--zap-log-level=debug"

# In deployed operator, edit manager args:
kubectl edit deployment -n appbundle-operator-system appbundle-operator-controller-manager
# Add: --zap-log-level=debug
```

## Common Development Tasks

### Adding a New Field to AppBundle

1. Edit `api/v1alpha1/appbundle_types.go`
2. Add the field with proper JSON tags and comments
3. Run `make generate manifests`
4. Update the controller logic in `internal/controller/appbundle_controller.go`
5. Update samples in `config/samples/`
6. Run tests

Example:
```go
type AppBundleSpec struct {
    // ... existing fields ...
    
    // NewField is a new configuration option
    // +optional
    NewField string `json:"newField,omitempty"`
}
```

### Adding a New Status Field

1. Edit `api/v1alpha1/appbundle_types.go`
2. Add to `AppBundleStatus`
3. Run `make generate manifests`
4. Update controller to populate the field
5. Update status in reconcile loop

### Modifying Controller Logic

1. Edit `internal/controller/appbundle_controller.go`
2. Add/modify reconciliation logic
3. Run `make fmt vet`
4. Run `make test`
5. Test locally with `make run`

### Adding RBAC Permissions

1. Add kubebuilder RBAC markers in controller:
```go
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update
```

2. Run `make manifests`
3. Verify in `config/rbac/role.yaml`

### Creating a New Sample

1. Create YAML file in `config/samples/`
2. Follow naming: `app_v1alpha1_appbundle_<description>.yaml`
3. Add comprehensive comments
4. Test with `kubectl apply -f config/samples/<file>.yaml`
5. Document in README

## Code Style and Best Practices

### Go Style

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `go fmt` (automatic with `make fmt`)
- Run `go vet` (automatic with `make vet`)
- Keep functions small and focused
- Add godoc comments to exported types and functions

### Kubebuilder Markers

Use markers for code generation:

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:validation:Required
// +kubebuilder:validation:Minimum=0
// +optional
```

### Error Handling

```go
// Good
if err != nil {
    logger.Error(err, "Failed to create resource", "name", obj.GetName())
    return ctrl.Result{}, err
}

// Bad
if err != nil {
    fmt.Println("Error:", err)
    return ctrl.Result{}, nil  // Swallowing errors
}
```

### Logging

```go
// Use structured logging
logger := log.FromContext(ctx)
logger.Info("Reconciling AppBundle", "name", appBundle.Name, "namespace", appBundle.Namespace)
logger.Error(err, "Failed to reconcile", "component", component.Name)

// Avoid
fmt.Println("Reconciling...")  // Bad
```

## Updating Dependencies

```bash
# Update all dependencies
go get -u ./...
go mod tidy

# Update specific dependency
go get -u github.com/example/package@version

# Verify
go mod verify
```

## Release Process

1. **Update Version**
   ```bash
   # Update version in relevant files
   # Tag the release
   git tag v0.1.0
   git push origin v0.1.0
   ```

2. **Build and Push Image**
   ```bash
   make docker-build IMG=<registry>/appbundle-operator:v0.1.0
   make docker-push IMG=<registry>/appbundle-operator:v0.1.0
   ```

3. **Generate Release Manifests**
   ```bash
   make build-installer IMG=<registry>/appbundle-operator:v0.1.0
   ```

4. **Create GitHub Release**
   - Upload manifests
   - Write release notes
   - Attach binaries

## Troubleshooting Development Issues

### CRD Changes Not Reflecting

```bash
# Regenerate and reinstall
make manifests
make install

# Or restart operator
kubectl rollout restart deployment -n appbundle-operator-system appbundle-operator-controller-manager
```

### Import Errors

```bash
# Clean and rebuild
go clean -cache
go mod tidy
go mod download
```

### Test Failures

```bash
# Run with verbose output
go test -v ./... -run TestName

# Run specific test
go test -v ./internal/controller -run TestReconcile
```

### Controller Not Starting

```bash
# Check CRDs
kubectl get crd appbundles.app.example.com

# Check RBAC
kubectl auth can-i create appbundles --as=system:serviceaccount:appbundle-operator-system:appbundle-operator-controller-manager

# Check logs
kubectl logs -n appbundle-operator-system deployment/appbundle-operator-controller-manager
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Run linters: `make fmt vet`
6. Commit with clear messages
7. Push and create a pull request

## Resources

- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Controller Runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

