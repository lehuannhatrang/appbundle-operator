#!/bin/bash
# Watch AppBundle Deployment Script
# This script helps you observe the ordered deployment with visual feedback

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

APPBUNDLE_NAME="appbundle-sample"
NAMESPACE="sample-app"

echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║         AppBundle Ordered Deployment Observer                 ║${NC}"
echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
echo ""

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}Error: kubectl not found${NC}"
    exit 1
fi

# Function to print timestamp
timestamp() {
    date +"%T"
}

# Function to get pod status
get_pod_status() {
    kubectl get pods -n "$NAMESPACE" 2>/dev/null | grep -v NAME || echo "No pods yet"
}

# Function to get appbundle status
get_appbundle_status() {
    kubectl get appbundle "$APPBUNDLE_NAME" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Not found"
}

echo -e "${YELLOW}[$(timestamp)]${NC} Checking if AppBundle already exists..."
if kubectl get appbundle "$APPBUNDLE_NAME" &>/dev/null; then
    echo -e "${YELLOW}[$(timestamp)]${NC} AppBundle exists. Deleting for clean test..."
    kubectl delete appbundle "$APPBUNDLE_NAME" --wait=false
    echo -e "${YELLOW}[$(timestamp)]${NC} Waiting for cleanup..."
    sleep 5
fi

echo ""
echo -e "${GREEN}[$(timestamp)]${NC} Applying AppBundle..."
kubectl apply -f config/samples/app_v1alpha1_appbundle.yaml

echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  Watching Deployment (Press Ctrl+C to stop)${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Track phases
LAST_PHASE=""
ITERATION=0

while true; do
    ITERATION=$((ITERATION + 1))
    CURRENT_PHASE=$(get_appbundle_status)
    
    # Only print when phase changes or every 5 iterations
    if [ "$CURRENT_PHASE" != "$LAST_PHASE" ] || [ $((ITERATION % 5)) -eq 0 ]; then
        echo -e "${GREEN}[$(timestamp)]${NC} AppBundle Phase: ${YELLOW}${CURRENT_PHASE}${NC}"
        
        # Show pods
        echo -e "${BLUE}Pods in namespace ${NAMESPACE}:${NC}"
        get_pod_status | while IFS= read -r line; do
            if [[ $line == *"Init:"* ]]; then
                echo -e "  ${YELLOW}$line${NC} (waiting...)"
            elif [[ $line == *"Running"* ]]; then
                echo -e "  ${GREEN}$line${NC}"
            elif [[ $line == *"No pods"* ]]; then
                echo -e "  ${YELLOW}$line${NC}"
            else
                echo -e "  $line"
            fi
        done
        echo ""
        
        LAST_PHASE="$CURRENT_PHASE"
    fi
    
    # Check if deployment is complete
    if [ "$CURRENT_PHASE" = "Deployed" ]; then
        echo -e "${GREEN}[$(timestamp)]${NC} ✓ Deployment Complete!"
        break
    fi
    
    # Check if deployment failed
    if [ "$CURRENT_PHASE" = "Failed" ]; then
        echo -e "${RED}[$(timestamp)]${NC} ✗ Deployment Failed!"
        echo ""
        echo "AppBundle status:"
        kubectl get appbundle "$APPBUNDLE_NAME" -o yaml | grep -A 20 "status:"
        exit 1
    fi
    
    # Timeout after 2 minutes
    if [ $ITERATION -gt 120 ]; then
        echo -e "${RED}[$(timestamp)]${NC} Timeout waiting for deployment"
        exit 1
    fi
    
    sleep 1
done

echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  Final Status${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

echo -e "${GREEN}AppBundle:${NC}"
kubectl get appbundle "$APPBUNDLE_NAME"
echo ""

echo -e "${GREEN}Resources in namespace ${NAMESPACE}:${NC}"
kubectl get all -n "$NAMESPACE"
echo ""

echo -e "${GREEN}Deployment Timeline (check init container logs):${NC}"
echo ""
echo -e "${YELLOW}Database init container:${NC}"
kubectl logs -n "$NAMESPACE" -l app=database -c wait-for-dependencies 2>/dev/null || echo "  (not available)"
echo ""
echo -e "${YELLOW}Web-app init containers (first pod):${NC}"
POD=$(kubectl get pods -n "$NAMESPACE" -l app=web-app -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
if [ -n "$POD" ]; then
    kubectl logs -n "$NAMESPACE" "$POD" -c wait-for-dependencies 2>/dev/null || echo "  (not available)"
else
    echo "  (no pods found)"
fi
echo ""

echo -e "${GREEN}[$(timestamp)]${NC} ✓ Observation complete!"
echo ""
echo -e "To see operator logs: ${BLUE}kubectl logs -n appbundle-system deployment/appbundle-controller-manager${NC}"
echo -e "To clean up: ${BLUE}kubectl delete appbundle $APPBUNDLE_NAME${NC}"

