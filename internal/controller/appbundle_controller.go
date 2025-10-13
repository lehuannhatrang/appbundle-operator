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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

	// Set owner reference
	if err := controllerutil.SetControllerReference(appBundle, obj, r.Scheme); err != nil {
		componentStatus.Phase = appv1alpha1.PhaseFailed
		componentStatus.Message = fmt.Sprintf("Failed to set owner reference: %v", err)
		return componentStatus, err
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

	componentStatus.Phase = appv1alpha1.PhaseDeployed
	componentStatus.Message = "Resource deployed successfully"
	componentStatus.ResourceRef = &appv1alpha1.ResourceReference{
		APIVersion: obj.GetAPIVersion(),
		Kind:       obj.GetKind(),
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
	}

	return componentStatus, nil
}

// reconcilePorchPackages handles integration with Porch for package lifecycle management
// nolint:unparam // This function currently always returns nil as it's a placeholder
func (r *AppBundleReconciler) reconcilePorchPackages(ctx context.Context, appBundle *appv1alpha1.AppBundle) error {
	logger := log.FromContext(ctx)

	// TODO: Implement Porch integration
	// This is a placeholder for Porch package lifecycle management
	// Integration points:
	// 1. Fetch package revisions from Porch
	// 2. Validate package availability
	// 3. Track package lifecycle events
	// 4. Update component templates from Porch packages

	logger.Info("Porch integration enabled", "repository", appBundle.Spec.PorchIntegration.Repository)

	// For now, this is a no-op. In a real implementation:
	// - Query Porch API for package status
	// - Fetch package contents if needed
	// - Update AppBundle status with package information

	return nil
}

// finalizeAppBundle handles cleanup when AppBundle is deleted
// nolint:unparam // This function currently always returns nil as cleanup is handled by K8s GC
func (r *AppBundleReconciler) finalizeAppBundle(ctx context.Context, appBundle *appv1alpha1.AppBundle) error {
	logger := log.FromContext(ctx)
	logger.Info("Finalizing AppBundle", "name", appBundle.Name)

	// Cleanup logic here
	// Resources with owner references will be automatically deleted by K8s
	// Additional cleanup for external resources or Porch integration can be added here

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
