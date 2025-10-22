/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appv1alpha1 "github.com/example/appbundle-operator/api/v1alpha1"
)

const (
	appBundleFinalizer = "app.example.com/finalizer"
	// Argo CD sync wave annotation
	argoSyncWaveAnnotation = "argocd.argoproj.io/sync-wave"
)

// AppBundleReconciler reconciles a AppBundle object
type AppBundleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=app.example.com,resources=appbundles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=app.example.com,resources=appbundles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=app.example.com,resources=appbundles/finalizers,verbs=update
// +kubebuilder:rbac:groups=config.porch.kpt.dev,resources=packagevariants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.porch.kpt.dev,resources=packagevariants/status,verbs=get
// +kubebuilder:rbac:groups=config.porch.kpt.dev,resources=repositories,verbs=get;list;watch
// +kubebuilder:rbac:groups=config.porch.kpt.dev,resources=packagerevisions,verbs=get;list;watch
// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *AppBundleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the AppBundle instance
	appBundle := &appv1alpha1.AppBundle{}
	if err := r.Get(ctx, req.NamespacedName, appBundle); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, could have been deleted after reconcile request
			logger.Info("AppBundle resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get AppBundle")
		return ctrl.Result{}, err
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(appBundle, appBundleFinalizer) {
		controllerutil.AddFinalizer(appBundle, appBundleFinalizer)
		if err := r.Update(ctx, appBundle); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Check if the AppBundle is being deleted
	if !appBundle.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(appBundle, appBundleFinalizer) {
			// Run finalization logic
			if err := r.finalizeAppBundle(ctx, appBundle); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(appBundle, appBundleFinalizer)
			if err := r.Update(ctx, appBundle); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Initialize status if needed
	if appBundle.Status.Phase == "" {
		appBundle.Status.Phase = appv1alpha1.PhasePending
		if err := r.Status().Update(ctx, appBundle); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile Porch packages if integration is enabled
	if appBundle.Spec.PorchIntegration != nil && appBundle.Spec.PorchIntegration.Enabled {
		if err := r.reconcilePorchPackages(ctx, appBundle); err != nil {
			logger.Error(err, "Failed to reconcile Porch packages")
			return r.updateStatusWithError(ctx, appBundle, err)
		}
	}

	// Sort groups by order
	sortedGroups := make([]appv1alpha1.Group, len(appBundle.Spec.Groups))
	copy(sortedGroups, appBundle.Spec.Groups)
	sort.Slice(sortedGroups, func(i, j int) bool {
		return sortedGroups[i].Order < sortedGroups[j].Order
	})

	// Deploy resources group by group
	appBundle.Status.Phase = appv1alpha1.PhaseDeploying
	appBundle.Status.GroupStatuses = make([]appv1alpha1.GroupStatus, 0, len(sortedGroups))

	for _, group := range sortedGroups {
		groupStatus, err := r.reconcileGroup(ctx, appBundle, group)
		if err != nil {
			logger.Error(err, "Failed to reconcile group", "group", group.Name)
			appBundle.Status.GroupStatuses = append(appBundle.Status.GroupStatuses, groupStatus)
			return r.updateStatusWithError(ctx, appBundle, err)
		}
		appBundle.Status.GroupStatuses = append(appBundle.Status.GroupStatuses, groupStatus)
	}

	// All groups deployed successfully
	appBundle.Status.Phase = appv1alpha1.PhaseDeployed
	appBundle.Status.Message = "All groups deployed successfully"
	appBundle.Status.ObservedGeneration = appBundle.Generation

	// Update condition
	meta.SetStatusCondition(&appBundle.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "DeploymentComplete",
		Message:            "All resources deployed successfully",
		ObservedGeneration: appBundle.Generation,
	})

	if err := r.Status().Update(ctx, appBundle); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileGroup reconciles a single group of components
func (r *AppBundleReconciler) reconcileGroup(ctx context.Context, appBundle *appv1alpha1.AppBundle, group appv1alpha1.Group) (appv1alpha1.GroupStatus, error) {
	logger := log.FromContext(ctx)

	groupStatus := appv1alpha1.GroupStatus{
		Name:              group.Name,
		Phase:             appv1alpha1.PhaseDeploying,
		ComponentStatuses: make([]appv1alpha1.ComponentStatus, 0, len(group.Components)),
	}

	// Sort components by order
	sortedComponents := make([]appv1alpha1.Component, len(group.Components))
	copy(sortedComponents, group.Components)
	sort.Slice(sortedComponents, func(i, j int) bool {
		return sortedComponents[i].Order < sortedComponents[j].Order
	})

	// Calculate base sync wave for this group
	baseSyncWave := group.Order * 100

	for _, component := range sortedComponents {
		componentStatus, err := r.reconcileComponent(ctx, appBundle, group, component, baseSyncWave)
		if err != nil {
			logger.Error(err, "Failed to reconcile component", "group", group.Name, "component", component.Name)
			groupStatus.Phase = appv1alpha1.PhaseFailed
			groupStatus.Message = fmt.Sprintf("Failed to deploy component %s: %v", component.Name, err)
			groupStatus.ComponentStatuses = append(groupStatus.ComponentStatuses, componentStatus)
			return groupStatus, err
		}
		groupStatus.ComponentStatuses = append(groupStatus.ComponentStatuses, componentStatus)
	}

	groupStatus.Phase = appv1alpha1.PhaseDeployed
	groupStatus.Message = "All components deployed successfully"
	return groupStatus, nil
}

// reconcileComponent reconciles a single component
func (r *AppBundleReconciler) reconcileComponent(ctx context.Context, appBundle *appv1alpha1.AppBundle, group appv1alpha1.Group, component appv1alpha1.Component, baseSyncWave int) (appv1alpha1.ComponentStatus, error) {
	logger := log.FromContext(ctx)

	componentStatus := appv1alpha1.ComponentStatus{
		Name:  component.Name,
		Phase: appv1alpha1.PhaseDeploying,
	}

	// If component has a Porch package reference, create PackageVariant
	if component.PorchPackageRef != nil {
		return r.reconcileComponentWithPorch(ctx, appBundle, group, component, baseSyncWave)
	}

	// Validate that template is provided for non-Porch components
	if len(component.Template.Raw) == 0 {
		componentStatus.Phase = appv1alpha1.PhaseFailed
		componentStatus.Message = "Component must have either template or porchPackageRef specified"
		return componentStatus, fmt.Errorf("component %s has neither template nor porchPackageRef", component.Name)
	}

	// Parse the template into an unstructured object
	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal(component.Template.Raw, obj); err != nil {
		componentStatus.Phase = appv1alpha1.PhaseFailed
		componentStatus.Message = fmt.Sprintf("Failed to parse template: %v", err)
		return componentStatus, err
	}

	// Calculate sync wave for this component
	syncWave := baseSyncWave + component.Order

	// Add Argo CD sync wave annotation
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[argoSyncWaveAnnotation] = strconv.Itoa(syncWave)
	obj.SetAnnotations(annotations)

	// Add labels for tracking
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["app.example.com/appbundle"] = appBundle.Name
	labels["app.example.com/group"] = group.Name
	labels["app.example.com/component"] = component.Name
	obj.SetLabels(labels)

	// Set namespace if not specified
	if obj.GetNamespace() == "" && appBundle.Namespace != "" {
		obj.SetNamespace(appBundle.Namespace)
	}

	// Set owner reference only if the resource is in the same namespace as the AppBundle
	// Kubernetes doesn't allow cross-namespace owner references for security reasons
	// Also skip for cluster-scoped resources (they have no namespace)
	if obj.GetNamespace() != "" && obj.GetNamespace() == appBundle.Namespace {
		if err := controllerutil.SetControllerReference(appBundle, obj, r.Scheme); err != nil {
			logger.Info("Warning: Failed to set owner reference, continuing without it",
				"error", err,
				"resource", obj.GetKind(),
				"name", obj.GetName(),
				"namespace", obj.GetNamespace())
			// Don't return error - continue without owner reference
			// The resource will still be created but won't be garbage collected automatically
		}
	} else {
		logger.Info("Skipping owner reference for cross-namespace or cluster-scoped resource",
			"resource", obj.GetKind(),
			"name", obj.GetName(),
			"namespace", obj.GetNamespace(),
			"appBundleNamespace", appBundle.Namespace)
	}

	// Create or update the resource
	existingObj := &unstructured.Unstructured{}
	existingObj.SetGroupVersionKind(obj.GroupVersionKind())
	err := r.Get(ctx, types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}, existingObj)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create the resource
			logger.Info("Creating resource", "group", group.Name, "component", component.Name, "kind", obj.GetKind(), "name", obj.GetName())
			if err := r.Create(ctx, obj); err != nil {
				componentStatus.Phase = appv1alpha1.PhaseFailed
				componentStatus.Message = fmt.Sprintf("Failed to create resource: %v", err)
				return componentStatus, err
			}
		} else {
			componentStatus.Phase = appv1alpha1.PhaseFailed
			componentStatus.Message = fmt.Sprintf("Failed to get existing resource: %v", err)
			return componentStatus, err
		}
	} else {
		// Update the resource
		logger.Info("Updating resource", "group", group.Name, "component", component.Name, "kind", obj.GetKind(), "name", obj.GetName())
		obj.SetResourceVersion(existingObj.GetResourceVersion())
		if err := r.Update(ctx, obj); err != nil {
			componentStatus.Phase = appv1alpha1.PhaseFailed
			componentStatus.Message = fmt.Sprintf("Failed to update resource: %v", err)
			return componentStatus, err
		}
	}

	// Wait for the resource to become ready
	logger.Info("Waiting for resource to become ready", "kind", obj.GetKind(), "name", obj.GetName())
	if err := r.waitForResourceReady(ctx, obj); err != nil {
		componentStatus.Phase = appv1alpha1.PhaseFailed
		componentStatus.Message = fmt.Sprintf("Resource not ready: %v", err)
		logger.Error(err, "Resource did not become ready", "kind", obj.GetKind(), "name", obj.GetName())
		return componentStatus, err
	}

	componentStatus.Phase = appv1alpha1.PhaseDeployed
	componentStatus.Message = "Resource deployed successfully"
	componentStatus.ResourceRef = &appv1alpha1.ResourceReference{
		APIVersion: obj.GetAPIVersion(),
		Kind:       obj.GetKind(),
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
	}

	logger.Info("Resource is ready", "kind", obj.GetKind(), "name", obj.GetName())
	return componentStatus, nil
}

// waitForResourceReady waits for a resource to become ready based on its kind
func (r *AppBundleReconciler) waitForResourceReady(ctx context.Context, obj *unstructured.Unstructured) error {
	logger := log.FromContext(ctx)
	kind := obj.GetKind()

	// Define timeout and poll interval
	timeout := 5 * time.Minute
	pollInterval := 2 * time.Second

	// Resources that don't need readiness checks
	immediatelyReadyKinds := map[string]bool{
		"Namespace":             true,
		"ConfigMap":             true,
		"Secret":                true,
		"Service":               true,
		"PersistentVolumeClaim": true,
		"ServiceAccount":        true,
		"Role":                  true,
		"RoleBinding":           true,
		"ClusterRole":           true,
		"ClusterRoleBinding":    true,
		"Ingress":               true,
		"NetworkPolicy":         true,
	}

	// If it's an immediately ready resource, return success
	if immediatelyReadyKinds[kind] {
		logger.Info("Resource is immediately ready", "kind", kind, "name", obj.GetName())
		return nil
	}

	// For resources that need readiness checks
	return wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		// Fetch the latest version of the resource
		current := &unstructured.Unstructured{}
		current.SetGroupVersionKind(obj.GroupVersionKind())
		err := r.Get(ctx, types.NamespacedName{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}, current)

		if err != nil {
			if errors.IsNotFound(err) {
				logger.V(1).Info("Resource not found yet, waiting...", "kind", kind, "name", obj.GetName())
				return false, nil
			}
			return false, err
		}

		// Check readiness based on resource kind
		switch kind {
		case "Deployment":
			return r.isDeploymentReady(current)
		case "StatefulSet":
			return r.isStatefulSetReady(current)
		case "DaemonSet":
			return r.isDaemonSetReady(current)
		case "Job":
			return r.isJobComplete(current)
		case "Pod":
			return r.isPodReady(current)
		default:
			// For unknown types, just check if they exist
			logger.Info("Unknown resource type, considering ready", "kind", kind, "name", obj.GetName())
			return true, nil
		}
	})
}

// isDeploymentReady checks if a Deployment is ready
func (r *AppBundleReconciler) isDeploymentReady(obj *unstructured.Unstructured) (bool, error) {
	// Get desired replicas
	replicas, found, err := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	if err != nil {
		return false, err
	}
	if !found {
		replicas = 1 // Default to 1 if not specified
	}

	// Get status
	readyReplicas, found, err := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
	if err != nil {
		return false, err
	}
	if !found {
		readyReplicas = 0
	}

	updatedReplicas, found, err := unstructured.NestedInt64(obj.Object, "status", "updatedReplicas")
	if err != nil {
		return false, err
	}
	if !found {
		updatedReplicas = 0
	}

	availableReplicas, found, err := unstructured.NestedInt64(obj.Object, "status", "availableReplicas")
	if err != nil {
		return false, err
	}
	if !found {
		availableReplicas = 0
	}

	ready := readyReplicas == replicas &&
		updatedReplicas == replicas &&
		availableReplicas == replicas

	return ready, nil
}

// isStatefulSetReady checks if a StatefulSet is ready
func (r *AppBundleReconciler) isStatefulSetReady(obj *unstructured.Unstructured) (bool, error) {
	replicas, found, err := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	if err != nil {
		return false, err
	}
	if !found {
		replicas = 1
	}

	readyReplicas, found, err := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
	if err != nil {
		return false, err
	}
	if !found {
		readyReplicas = 0
	}

	return readyReplicas == replicas, nil
}

// isDaemonSetReady checks if a DaemonSet is ready
func (r *AppBundleReconciler) isDaemonSetReady(obj *unstructured.Unstructured) (bool, error) {
	desiredNumberScheduled, found, err := unstructured.NestedInt64(obj.Object, "status", "desiredNumberScheduled")
	if err != nil {
		return false, err
	}
	if !found || desiredNumberScheduled == 0 {
		return false, nil
	}

	numberReady, found, err := unstructured.NestedInt64(obj.Object, "status", "numberReady")
	if err != nil {
		return false, err
	}
	if !found {
		numberReady = 0
	}

	return numberReady == desiredNumberScheduled, nil
}

// isJobComplete checks if a Job is complete
func (r *AppBundleReconciler) isJobComplete(obj *unstructured.Unstructured) (bool, error) {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}

	for _, condition := range conditions {
		condMap, ok := condition.(map[string]interface{})
		if !ok {
			continue
		}
		condType, found, err := unstructured.NestedString(condMap, "type")
		if err != nil || !found {
			continue
		}
		status, found, err := unstructured.NestedString(condMap, "status")
		if err != nil || !found {
			continue
		}
		if condType == "Complete" && status == "True" {
			return true, nil
		}
		if condType == "Failed" && status == "True" {
			return false, fmt.Errorf("job failed")
		}
	}

	return false, nil
}

// isPodReady checks if a Pod is ready
func (r *AppBundleReconciler) isPodReady(obj *unstructured.Unstructured) (bool, error) {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}

	for _, condition := range conditions {
		condMap, ok := condition.(map[string]interface{})
		if !ok {
			continue
		}
		condType, found, err := unstructured.NestedString(condMap, "type")
		if err != nil || !found {
			continue
		}
		status, found, err := unstructured.NestedString(condMap, "status")
		if err != nil || !found {
			continue
		}
		if condType == "Ready" && status == "True" {
			return true, nil
		}
	}

	return false, nil
}

// reconcileComponentWithPorch reconciles a component that uses a Porch package
func (r *AppBundleReconciler) reconcileComponentWithPorch(ctx context.Context, appBundle *appv1alpha1.AppBundle, group appv1alpha1.Group, component appv1alpha1.Component, baseSyncWave int) (appv1alpha1.ComponentStatus, error) {
	logger := log.FromContext(ctx)

	componentStatus := appv1alpha1.ComponentStatus{
		Name:  component.Name,
		Phase: appv1alpha1.PhaseDeploying,
	}

	if component.PorchPackageRef == nil {
		componentStatus.Phase = appv1alpha1.PhaseFailed
		componentStatus.Message = "PorchPackageRef is nil"
		return componentStatus, fmt.Errorf("porchPackageRef is nil")
	}

	// Create PackageVariant name (custom format: appbundle-<package>)
	packageVariantName := fmt.Sprintf("appbundle-%s", component.PorchPackageRef.PackageName)

	// Determine namespace for PackageVariant (default to "default")
	pvNamespace := "default"
	if component.PorchPackageRef.Namespace != "" {
		pvNamespace = component.PorchPackageRef.Namespace
	}

	// Determine downstream repo (default to "mgmt" or use from spec)
	downstreamRepo := "mgmt"
	if appBundle.Spec.PorchIntegration != nil && appBundle.Spec.PorchIntegration.Repository != "" {
		downstreamRepo = appBundle.Spec.PorchIntegration.Repository
	}

	// Determine revision (default to "main")
	revision := "main"
	if component.PorchPackageRef.Revision != "" {
		revision = component.PorchPackageRef.Revision
	}

	// Create PackageVariant CRD
	packageVariant := &unstructured.Unstructured{}
	packageVariant.SetAPIVersion("config.porch.kpt.dev/v1alpha1")
	packageVariant.SetKind("PackageVariant")
	packageVariant.SetName(packageVariantName)
	packageVariant.SetNamespace(pvNamespace)

	// Calculate sync wave
	syncWave := baseSyncWave + component.Order
	syncWaveStr := strconv.Itoa(syncWave)

	// Calculate wait job sync wave (between current and next group)
	// Place it at baseSyncWave + 50 (middle of the group's range)
	waitSyncWave := baseSyncWave + 50
	waitSyncWaveStr := strconv.Itoa(waitSyncWave)

	// Build mutators list
	mutators := []interface{}{
		// Mutator 1: Set sync wave annotations on all resources
		map[string]interface{}{
			"image": "gcr.io/kpt-fn/set-annotations:v0.1.4",
			"configMap": map[string]interface{}{
				argoSyncWaveAnnotation: syncWaveStr,
			},
		},
		// Mutator 2: Add AppBundle tracking labels to all resources
		map[string]interface{}{
			"image": "gcr.io/kpt-fn/set-labels:v0.2.0",
			"configMap": map[string]interface{}{
				"app.example.com/appbundle": appBundle.Name,
				"app.example.com/group":     group.Name,
				"app.example.com/component": component.Name,
			},
		},
		// Mutator 3: Inject wait Job using Starlark
		// This Job waits for all workload resources to be ready before proceeding
		r.buildWaitJobMutator(appBundle, group, component, packageVariantName, waitSyncWaveStr),
	}

	// Set PackageVariant spec with pipeline mutators
	spec := map[string]interface{}{
		"upstream": map[string]interface{}{
			"repo":     component.PorchPackageRef.Repository,
			"package":  component.PorchPackageRef.PackageName,
			"revision": revision,
		},
		"downstream": map[string]interface{}{
			"repo":    downstreamRepo,
			"package": packageVariantName,
		},
		"annotations": map[string]interface{}{
			"approval.nephio.org/policy": "initial",
		},
		"adoptionPolicy": "adoptExisting",
		"deletionPolicy": "delete",
		// Pipeline mutators to inject annotations and wait Job
		"pipeline": map[string]interface{}{
			"mutators": mutators,
		},
	}

	if err := unstructured.SetNestedMap(packageVariant.Object, spec, "spec"); err != nil {
		componentStatus.Phase = appv1alpha1.PhaseFailed
		componentStatus.Message = fmt.Sprintf("Failed to set PackageVariant spec: %v", err)
		return componentStatus, err
	}

	// Add annotations to metadata
	annotations := map[string]string{
		argoSyncWaveAnnotation: strconv.Itoa(syncWave),
	}
	packageVariant.SetAnnotations(annotations)

	// Add labels
	labels := map[string]string{
		"app.example.com/appbundle": appBundle.Name,
		"app.example.com/group":     group.Name,
		"app.example.com/component": component.Name,
	}
	packageVariant.SetLabels(labels)

	// Create or update PackageVariant
	existingPV := &unstructured.Unstructured{}
	existingPV.SetGroupVersionKind(packageVariant.GroupVersionKind())
	err := r.Get(ctx, types.NamespacedName{
		Name:      packageVariantName,
		Namespace: pvNamespace,
	}, existingPV)

	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Creating PackageVariant", "name", packageVariantName, "package", component.PorchPackageRef.PackageName)
			if err := r.Create(ctx, packageVariant); err != nil {
				componentStatus.Phase = appv1alpha1.PhaseFailed
				componentStatus.Message = fmt.Sprintf("Failed to create PackageVariant: %v", err)
				return componentStatus, err
			}
		} else {
			componentStatus.Phase = appv1alpha1.PhaseFailed
			componentStatus.Message = fmt.Sprintf("Failed to get PackageVariant: %v", err)
			return componentStatus, err
		}
	} else {
		logger.Info("PackageVariant already exists", "name", packageVariantName)
		// Update if needed (for now, skip update to avoid conflicts with Porch)
	}

	// Wait for PackageVariant to be ready
	logger.Info("Waiting for PackageVariant to be ready", "name", packageVariantName)
	if err := r.waitForPackageVariantReady(ctx, packageVariantName, pvNamespace); err != nil {
		componentStatus.Phase = appv1alpha1.PhaseFailed
		componentStatus.Message = fmt.Sprintf("PackageVariant not ready: %v", err)
		return componentStatus, err
	}

	// Auto-discover and monitor resources from the deployed package
	var resourcesToMonitor []*unstructured.Unstructured

	// If template is provided, use it (backward compatibility)
	if len(component.Template.Raw) > 0 {
		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(component.Template.Raw, obj); err == nil {
			// Set the correct namespace (default to AppBundle namespace)
			if obj.GetNamespace() == "" {
				obj.SetNamespace(appBundle.Namespace)
			}
			resourcesToMonitor = append(resourcesToMonitor, obj)
			logger.Info("Using provided template for resource monitoring", "kind", obj.GetKind(), "name", obj.GetName())
		}
	} else {
		// Auto-discover resources from the PackageVariant
		logger.Info("No template provided, auto-discovering resources from Porch package")
		discoveredResources, err := r.discoverPackageVariantResources(ctx, packageVariantName, pvNamespace, downstreamRepo)
		if err != nil {
			logger.Info("Failed to discover resources from package, will continue", "error", err)
			// Don't fail - the PackageVariant is ready, resource discovery might take time
		} else if len(discoveredResources) > 0 {
			resourcesToMonitor = discoveredResources
			logger.Info("Auto-discovered resources from package", "count", len(discoveredResources))
		}
	}

	// Wait for discovered/specified resources to be ready
	for _, obj := range resourcesToMonitor {
		logger.Info("Waiting for Porch-deployed resource to be ready", "kind", obj.GetKind(), "name", obj.GetName(), "namespace", obj.GetNamespace())
		if err := r.waitForResourceReady(ctx, obj); err != nil {
			logger.Error(err, "Porch-deployed resource not ready yet", "kind", obj.GetKind(), "name", obj.GetName())
			// Don't fail - the PackageVariant is ready, resources might take longer to become ready
		} else {
			logger.Info("Resource is ready", "kind", obj.GetKind(), "name", obj.GetName())
		}
	}

	componentStatus.Phase = appv1alpha1.PhaseDeployed
	componentStatus.Message = fmt.Sprintf("PackageVariant deployed successfully via Porch (%d resources monitored)", len(resourcesToMonitor))
	componentStatus.ResourceRef = &appv1alpha1.ResourceReference{
		APIVersion: "config.porch.kpt.dev/v1alpha1",
		Kind:       "PackageVariant",
		Name:       packageVariantName,
		Namespace:  component.PorchPackageRef.Namespace,
	}

	logger.Info("PackageVariant deployed and ready", "name", packageVariantName)
	return componentStatus, nil
}

// waitForPackageVariantReady waits for a PackageVariant to become ready
func (r *AppBundleReconciler) waitForPackageVariantReady(ctx context.Context, name, namespace string) error {
	logger := log.FromContext(ctx)
	timeout := 5 * time.Minute
	pollInterval := 2 * time.Second

	return wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		pv := &unstructured.Unstructured{}
		pv.SetAPIVersion("config.porch.kpt.dev/v1alpha1")
		pv.SetKind("PackageVariant")

		err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, pv)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.V(1).Info("PackageVariant not found yet", "name", name)
				return false, nil
			}
			return false, err
		}

		// Check if PackageVariant is ready
		conditions, found, err := unstructured.NestedSlice(pv.Object, "status", "conditions")
		if err != nil || !found {
			logger.V(1).Info("PackageVariant has no conditions yet", "name", name)
			return false, nil
		}

		for _, condition := range conditions {
			condMap, ok := condition.(map[string]interface{})
			if !ok {
				continue
			}
			condType, found, err := unstructured.NestedString(condMap, "type")
			if err != nil || !found {
				continue
			}
			status, found, err := unstructured.NestedString(condMap, "status")
			if err != nil || !found {
				continue
			}
			if condType == "Ready" && status == "True" {
				logger.Info("PackageVariant is ready", "name", name)
				return true, nil
			}
		}

		logger.V(1).Info("PackageVariant not ready yet", "name", name)
		return false, nil
	})
}

// discoverPackageVariantResources discovers resources deployed by a PackageVariant
// It queries the downstream PackageRevision created by the PackageVariant to find
// all resources that were deployed
func (r *AppBundleReconciler) discoverPackageVariantResources(ctx context.Context, packageVariantName, pvNamespace, downstreamRepo string) ([]*unstructured.Unstructured, error) {
	logger := log.FromContext(ctx)

	// Get the PackageVariant to find the downstream PackageRevision
	pv := &unstructured.Unstructured{}
	pv.SetAPIVersion("config.porch.kpt.dev/v1alpha1")
	pv.SetKind("PackageVariant")

	if err := r.Get(ctx, types.NamespacedName{Name: packageVariantName, Namespace: pvNamespace}, pv); err != nil {
		return nil, fmt.Errorf("failed to get PackageVariant: %w", err)
	}

	// Get the downstream package name from PackageVariant status
	downstreamPkgName, found, err := unstructured.NestedString(pv.Object, "status", "downstreamTargets", "0", "name")
	if err != nil || !found {
		logger.Info("Downstream package not yet created in PackageVariant status, will retry")
		return nil, fmt.Errorf("downstream package not found in status")
	}

	logger.Info("Found downstream package", "name", downstreamPkgName, "namespace", pvNamespace)

	// Query the PackageRevision to get the deployed resources
	// The PackageRevision contains the actual resource manifests
	prList := &unstructured.UnstructuredList{}
	prList.SetAPIVersion("porch.kpt.dev/v1alpha1")
	prList.SetKind("PackageRevisionList")

	// List PackageRevisions matching our downstream package
	listOpts := []client.ListOption{
		client.InNamespace(pvNamespace),
	}

	if err := r.List(ctx, prList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list PackageRevisions: %w", err)
	}

	var targetPR *unstructured.Unstructured
	for i := range prList.Items {
		pr := &prList.Items[i]
		prName := pr.GetName()

		// Match PackageRevisions that belong to our downstream package
		// PackageRevision names typically follow pattern: <repo>-<package>-<revision>
		// We need to check the spec to match correctly
		specPackage, found, err := unstructured.NestedString(pr.Object, "spec", "packageName")
		if err != nil || !found {
			continue
		}

		specRepo, found, err := unstructured.NestedString(pr.Object, "spec", "repositoryName")
		if err != nil || !found {
			continue
		}

		if specPackage == packageVariantName && specRepo == downstreamRepo {
			targetPR = pr
			logger.Info("Found matching PackageRevision", "name", prName)
			break
		}
	}

	if targetPR == nil {
		logger.Info("PackageRevision not found yet for package", "package", packageVariantName)
		return nil, fmt.Errorf("packageRevision not found for downstream package")
	}

	// Get the resources from the PackageRevision
	// Resources are stored in spec.resources
	resources, found, err := unstructured.NestedSlice(targetPR.Object, "spec", "resources")
	if err != nil {
		return nil, fmt.Errorf("failed to get resources from PackageRevision: %w", err)
	}
	if !found {
		logger.Info("No resources found in PackageRevision yet")
		return nil, nil
	}

	logger.Info("Found resources in PackageRevision", "count", len(resources))

	// Parse each resource into an unstructured object
	var discoveredResources []*unstructured.Unstructured
	for _, res := range resources {
		resMap, ok := res.(map[string]interface{})
		if !ok {
			continue
		}

		// Get the resource data/content
		content, found, err := unstructured.NestedString(resMap, "data")
		if err != nil || !found {
			continue
		}

		// Parse the YAML/JSON content into unstructured
		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal([]byte(content), obj); err != nil {
			// Try as YAML if JSON fails
			logger.V(1).Info("Failed to parse resource as JSON, might be YAML", "error", err)
			continue
		}

		// Filter out non-workload resources (we want to monitor actual workloads)
		kind := obj.GetKind()
		if kind != "" && !isInfrastructureResource(kind) {
			discoveredResources = append(discoveredResources, obj)
			logger.Info("Discovered resource from package", "kind", kind, "name", obj.GetName())
		}
	}

	return discoveredResources, nil
}

// isInfrastructureResource checks if a resource kind is infrastructure (non-workload)
// Infrastructure resources don't need readiness monitoring
func isInfrastructureResource(kind string) bool {
	infraKinds := map[string]bool{
		"Namespace":                true,
		"ConfigMap":                true,
		"Secret":                   true,
		"ServiceAccount":           true,
		"Role":                     true,
		"RoleBinding":              true,
		"ClusterRole":              true,
		"ClusterRoleBinding":       true,
		"CustomResourceDefinition": true,
		"ResourceQuota":            true,
		"LimitRange":               true,
	}
	return infraKinds[kind]
}

// buildWaitJobMutator creates a Starlark mutator that injects a wait Job
// The wait Job uses Argo CD hooks to pause deployment until resources are ready
func (r *AppBundleReconciler) buildWaitJobMutator(appBundle *appv1alpha1.AppBundle, group appv1alpha1.Group, component appv1alpha1.Component, packageName, syncWave string) map[string]interface{} {
	// Determine namespace for the wait job
	namespace := appBundle.Namespace
	if namespace == "" {
		namespace = "default"
	}

	// Generate a random suffix for the job name to avoid conflicts
	randomSuffix := fmt.Sprintf("%d", time.Now().Unix()%100000)

	// Build the Starlark script that injects the wait Job
	// Note: KPT Starlark doesn't need imports - resource_list is passed directly
	starlarkScript := fmt.Sprintf(`def transform(resource_list):
    # Determine the target namespace from resources in the package
    target_namespace = None
    specific_wait_commands = []
    
    # Scan all resources in the package to determine namespace and specific resources
    for resource in resource_list["items"]:
        kind = resource.get("kind", "")
        metadata = resource.get("metadata", {})
        name = metadata.get("name", "")
        ns = metadata.get("namespace", "")
        
        # Store the target namespace from the first namespaced resource
        if ns and not target_namespace:
            target_namespace = ns
        
        # Collect specific wait commands for known workload resources
        # Note: Starlark doesn't support f-strings, use string concatenation
        if kind == "Deployment" and name and ns:
            specific_wait_commands.append("kubectl rollout status deployment/" + name + " -n " + ns + " --timeout=15m")
        elif kind == "StatefulSet" and name and ns:
            specific_wait_commands.append("kubectl rollout status statefulset/" + name + " -n " + ns + " --timeout=15m")
        elif kind == "DaemonSet" and name and ns:
            specific_wait_commands.append("kubectl rollout status daemonset/" + name + " -n " + ns + " --timeout=15m")
    
    # If no specific namespace found, use default
    if not target_namespace:
        target_namespace = "%s"
    
    # Build wait script - always create a wait job
    # If we found specific resources, wait for them. Otherwise, use a simple delay
    if specific_wait_commands:
        wait_script = " && ".join(specific_wait_commands)
    else:
        # Generic wait: just add a delay to ensure resources have time to deploy
        # This is a fallback when we can't detect specific resources in the package
        wait_script = "echo 'Waiting for resources to be created in namespace " + target_namespace + "...' && sleep 10 && echo 'Proceeding to next group'"
    
    # Always create wait job to ensure sequential deployment
    # First, create ServiceAccount
    sa_yaml = {
        "apiVersion": "v1",
        "kind": "ServiceAccount",
        "metadata": {
            "name": "appbundle-wait-reader",
            "namespace": target_namespace,
            "annotations": {
                "argocd.argoproj.io/sync-wave": "%s"
            },
            "labels": {
                "app.example.com/appbundle": "%s",
                "app.example.com/wait-job": "true"
            }
        }
    }
    resource_list["items"].append(sa_yaml)
    
    # Create ClusterRole for reading resource status
    clusterrole_yaml = {
        "apiVersion": "rbac.authorization.k8s.io/v1",
        "kind": "ClusterRole",
        "metadata": {
            "name": "appbundle-wait-reader-" + target_namespace,
            "annotations": {
                "argocd.argoproj.io/sync-wave": "%s"
            },
            "labels": {
                "app.example.com/appbundle": "%s",
                "app.example.com/wait-job": "true"
            }
        },
        "rules": [
            {
                "apiGroups": ["apps"],
                "resources": ["deployments", "deployments/status", "statefulsets", "statefulsets/status", "daemonsets", "daemonsets/status"],
                "verbs": ["get", "list", "watch"]
            },
            {
                "apiGroups": ["batch"],
                "resources": ["jobs", "jobs/status"],
                "verbs": ["get", "list", "watch"]
            },
            {
                "apiGroups": [""],
                "resources": ["pods", "pods/status", "services"],
                "verbs": ["get", "list", "watch"]
            }
        ]
    }
    resource_list["items"].append(clusterrole_yaml)
    
    # Create ClusterRoleBinding
    clusterrolebinding_yaml = {
        "apiVersion": "rbac.authorization.k8s.io/v1",
        "kind": "ClusterRoleBinding",
        "metadata": {
            "name": "appbundle-wait-reader-" + target_namespace,
            "annotations": {
                "argocd.argoproj.io/sync-wave": "%s"
            },
            "labels": {
                "app.example.com/appbundle": "%s",
                "app.example.com/wait-job": "true"
            }
        },
        "roleRef": {
            "apiGroup": "rbac.authorization.k8s.io",
            "kind": "ClusterRole",
            "name": "appbundle-wait-reader-" + target_namespace
        },
        "subjects": [
            {
                "kind": "ServiceAccount",
                "name": "appbundle-wait-reader",
                "namespace": target_namespace
            }
        ]
    }
    resource_list["items"].append(clusterrolebinding_yaml)
    
    # Finally, create the wait Job
    job_yaml = {
        "apiVersion": "batch/v1",
        "kind": "Job",
        "metadata": {
            "name": "wait-%s-%s-%s",
            "namespace": target_namespace,
            "annotations": {
                "argocd.argoproj.io/hook": "Sync",
                "argocd.argoproj.io/sync-wave": "%s"
            },
            "labels": {
                "app.example.com/appbundle": "%s",
                "app.example.com/group": "%s",
                "app.example.com/component": "%s",
                "app.example.com/wait-job": "true"
            }
        },
        "spec": {
            "ttlSecondsAfterFinished": 300,
            "backoffLimit": 3,
            "template": {
                "spec": {
                    "restartPolicy": "Never",
                    "serviceAccountName": "appbundle-wait-reader",
                    "containers": [{
                        "name": "wait",
                        "image": "bitnami/kubectl:latest",
                        "command": ["sh", "-c"],
                        "args": [wait_script]
                    }]
                }
            }
        }
    }
    resource_list["items"].append(job_yaml)
    
    return resource_list

# Call the transform function
transform(resource_list)
`, namespace,
		syncWave, appBundle.Name, // ServiceAccount sync-wave and labels
		syncWave, appBundle.Name, // ClusterRole sync-wave and labels
		syncWave, appBundle.Name, // ClusterRoleBinding sync-wave and labels
		group.Name, component.Name, randomSuffix, syncWave, appBundle.Name, group.Name, component.Name) // Job with random suffix

	return map[string]interface{}{
		"image": "gcr.io/kpt-fn/starlark:v0.4.3",
		"configMap": map[string]interface{}{
			"source": starlarkScript,
		},
	}
}

// reconcilePorchPackages handles integration with Porch for package lifecycle management
// nolint:unparam // This function currently always returns nil as it's a placeholder
func (r *AppBundleReconciler) reconcilePorchPackages(ctx context.Context, appBundle *appv1alpha1.AppBundle) error {
	logger := log.FromContext(ctx)

	// Validate that required repositories are registered in Porch
	// This is a pre-flight check before creating PackageVariants
	logger.Info("Porch integration enabled", "repository", appBundle.Spec.PorchIntegration.Repository)

	// In a real implementation, you might want to:
	// 1. Query Porch to ensure repositories are registered
	// 2. Validate package availability
	// 3. Pre-fetch package metadata for validation

	return nil
}

// finalizeAppBundle handles cleanup when AppBundle is deleted
// nolint:unparam // This function currently always returns nil as cleanup is handled by K8s GC
func (r *AppBundleReconciler) finalizeAppBundle(ctx context.Context, appBundle *appv1alpha1.AppBundle) error {
	logger := log.FromContext(ctx)
	logger.Info("Finalizing AppBundle", "name", appBundle.Name)

	// Cleanup logic here
	// Resources with owner references will be automatically deleted by K8s
	// We need to manually clean up resources without owner references:
	// - Cross-namespace resources
	// - Cluster-scoped resources (Namespaces, ClusterRoles, etc.)

	// Delete resources in reverse order (reverse of groups and components)
	sortedGroups := make([]appv1alpha1.Group, len(appBundle.Spec.Groups))
	copy(sortedGroups, appBundle.Spec.Groups)
	sort.Slice(sortedGroups, func(i, j int) bool {
		return sortedGroups[i].Order > sortedGroups[j].Order // Reverse order
	})

	for _, group := range sortedGroups {
		// Sort components in reverse order
		sortedComponents := make([]appv1alpha1.Component, len(group.Components))
		copy(sortedComponents, group.Components)
		sort.Slice(sortedComponents, func(i, j int) bool {
			return sortedComponents[i].Order > sortedComponents[j].Order // Reverse order
		})

		for _, component := range sortedComponents {
			// If component uses Porch, delete the PackageVariant
			// The PackageVariant deletion will cascade to deployed resources
			if component.PorchPackageRef != nil {
				pvName := fmt.Sprintf("appbundle-%s", component.PorchPackageRef.PackageName)
				pvNamespace := "default"
				if component.PorchPackageRef.Namespace != "" {
					pvNamespace = component.PorchPackageRef.Namespace
				}

				pv := &unstructured.Unstructured{}
				pv.SetAPIVersion("config.porch.kpt.dev/v1alpha1")
				pv.SetKind("PackageVariant")
				pv.SetName(pvName)
				pv.SetNamespace(pvNamespace)

				logger.Info("Deleting PackageVariant", "name", pvName, "namespace", pvNamespace)
				if err := r.Delete(ctx, pv); err != nil && !errors.IsNotFound(err) {
					logger.Error(err, "Failed to delete PackageVariant", "name", pvName)
				}
				continue
			}

			// For non-Porch components, parse template and delete resource
			if len(component.Template.Raw) > 0 {
				obj := &unstructured.Unstructured{}
				if err := json.Unmarshal(component.Template.Raw, obj); err != nil {
					logger.Error(err, "Failed to parse template during cleanup", "component", component.Name)
					continue
				}

				// Set namespace if not specified in template
				if obj.GetNamespace() == "" && appBundle.Namespace != "" {
					obj.SetNamespace(appBundle.Namespace)
				}

				// Try to delete the resource (ignore NotFound errors)
				logger.Info("Deleting resource", "kind", obj.GetKind(), "name", obj.GetName(), "namespace", obj.GetNamespace())
				if err := r.Delete(ctx, obj); err != nil && !errors.IsNotFound(err) {
					logger.Error(err, "Failed to delete resource", "kind", obj.GetKind(), "name", obj.GetName())
					// Continue with other resources even if one fails
				}
			}
		}
	}

	logger.Info("AppBundle finalization complete", "name", appBundle.Name)
	return nil
}

// updateStatusWithError updates the AppBundle status with error information
func (r *AppBundleReconciler) updateStatusWithError(ctx context.Context, appBundle *appv1alpha1.AppBundle, err error) (ctrl.Result, error) {
	appBundle.Status.Phase = appv1alpha1.PhaseFailed
	appBundle.Status.Message = err.Error()

	meta.SetStatusCondition(&appBundle.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "DeploymentFailed",
		Message:            err.Error(),
		ObservedGeneration: appBundle.Generation,
	})

	if statusErr := r.Status().Update(ctx, appBundle); statusErr != nil {
		return ctrl.Result{}, statusErr
	}

	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppBundleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1alpha1.AppBundle{}).
		Named("appbundle").
		Complete(r)
}
