#!/bin/bash

# AppBundle Operator Setup Verification Script
# This script verifies that the operator is properly set up and ready to use

set -e

echo "=========================================="
echo "AppBundle Operator Setup Verification"
echo "=========================================="
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print status
print_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✓${NC} $2"
    else
        echo -e "${RED}✗${NC} $2"
        exit 1
    fi
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

echo "1. Checking Go installation..."
if command -v go &> /dev/null; then
    GO_VERSION=$(go version | awk '{print $3}')
    print_status 0 "Go is installed: $GO_VERSION"
else
    print_status 1 "Go is not installed"
fi

echo ""
echo "2. Checking project structure..."
REQUIRED_DIRS=("api/v1alpha1" "internal/controller" "internal/porch" "config/crd" "config/samples" "docs")
for dir in "${REQUIRED_DIRS[@]}"; do
    if [ -d "$dir" ]; then
        print_status 0 "Directory exists: $dir"
    else
        print_status 1 "Missing directory: $dir"
    fi
done

echo ""
echo "3. Checking required files..."
REQUIRED_FILES=(
    "api/v1alpha1/appbundle_types.go"
    "internal/controller/appbundle_controller.go"
    "internal/porch/porch_client.go"
    "config/samples/app_v1alpha1_appbundle.yaml"
    "README.md"
    "docs/QUICKSTART.md"
    "docs/ARCHITECTURE.md"
    "docs/DEVELOPMENT.md"
    "Makefile"
    "go.mod"
)
for file in "${REQUIRED_FILES[@]}"; do
    if [ -f "$file" ]; then
        print_status 0 "File exists: $file"
    else
        print_status 1 "Missing file: $file"
    fi
done

echo ""
echo "4. Checking Go dependencies..."
go mod verify &> /dev/null
print_status $? "Go modules verified"

echo ""
echo "5. Running go vet..."
go vet ./... &> /dev/null
print_status $? "No vet issues found"

echo ""
echo "6. Building the operator..."
make build &> /dev/null
print_status $? "Operator builds successfully"

echo ""
echo "7. Checking generated files..."
if [ -f "api/v1alpha1/zz_generated.deepcopy.go" ]; then
    print_status 0 "DeepCopy methods generated"
else
    print_warning "DeepCopy methods not generated. Run: make generate"
fi

if [ -f "config/crd/bases/app.example.com_appbundles.yaml" ]; then
    print_status 0 "CRD manifest generated"
else
    print_warning "CRD manifest not generated. Run: make manifests"
fi

echo ""
echo "8. Validating sample AppBundles..."
for sample in config/samples/app_v1alpha1_appbundle*.yaml; do
    if [ -f "$sample" ]; then
        # Basic YAML validation
        if command -v yq &> /dev/null; then
            yq eval '.' "$sample" &> /dev/null
            print_status $? "Valid YAML: $(basename $sample)"
        else
            print_status 0 "Sample exists: $(basename $sample)"
        fi
    fi
done

echo ""
echo "=========================================="
echo "Verification Summary"
echo "=========================================="
echo ""
echo -e "${GREEN}✓${NC} All checks passed!"
echo ""
echo "Your AppBundle Operator is ready to use!"
echo ""
echo "Next steps:"
echo "  1. Install CRDs: make install"
echo "  2. Run locally: make run"
echo "  3. Apply sample: kubectl apply -f config/samples/app_v1alpha1_appbundle.yaml"
echo ""
echo "For more information, see:"
echo "  - README.md for overview and features"
echo "  - docs/QUICKSTART.md for getting started"
echo "  - docs/DEVELOPMENT.md for development guide"
echo "  - docs/ARCHITECTURE.md for architecture details"
echo ""

