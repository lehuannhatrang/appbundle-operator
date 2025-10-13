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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Component represents a Kubernetes resource template within a group
type Component struct {
	// Name is the unique identifier for the component within a group
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Order determines the deployment order of the component within its group
	// Components with lower order numbers are deployed first
	// +kubebuilder:validation:Minimum=0
	// +optional
	Order int `json:"order,omitempty"`

	// Template is the Kubernetes resource template to be deployed
	// This can be any valid Kubernetes resource (Deployment, Service, ConfigMap, etc.)
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Template runtime.RawExtension `json:"template"`

	// PorchPackageRef references a Porch package for this component
	// +optional
	PorchPackageRef *PorchPackageReference `json:"porchPackageRef,omitempty"`
}

// Group represents a collection of related components
type Group struct {
	// Name is the unique identifier for the group
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Order determines the deployment order of the group
	// Groups with lower order numbers are deployed first
	// +kubebuilder:validation:Minimum=0
	// +optional
	Order int `json:"order,omitempty"`

	// Components is the list of components in this group
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:Required
	Components []Component `json:"components"`
}

// PorchPackageReference contains information to reference a Porch package
type PorchPackageReference struct {
	// Name of the Porch package
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the Porch package
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Revision of the Porch package
	// +optional
	Revision string `json:"revision,omitempty"`
}

// AppBundleSpec defines the desired state of AppBundle
type AppBundleSpec struct {
	// Groups is the list of component groups to be deployed
	// Groups are deployed in order based on their Order field
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:Required
	Groups []Group `json:"groups"`

	// PorchIntegration enables integration with Porch for package lifecycle management
	// +optional
	PorchIntegration *PorchIntegrationSpec `json:"porchIntegration,omitempty"`
}

// PorchIntegrationSpec defines configuration for Porch integration
type PorchIntegrationSpec struct {
	// Enabled determines if Porch integration is active
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Repository is the Porch repository to use
	// +optional
	Repository string `json:"repository,omitempty"`
}

// DeploymentPhase represents the current phase of deployment
type DeploymentPhase string

const (
	// PhasePending means the AppBundle has been accepted but deployment has not started
	PhasePending DeploymentPhase = "Pending"
	// PhaseDeploying means the AppBundle is currently being deployed
	PhaseDeploying DeploymentPhase = "Deploying"
	// PhaseDeployed means all resources have been successfully deployed
	PhaseDeployed DeploymentPhase = "Deployed"
	// PhaseFailed means deployment encountered an error
	PhaseFailed DeploymentPhase = "Failed"
)

// GroupStatus represents the status of a group
type GroupStatus struct {
	// Name of the group
	Name string `json:"name"`

	// Phase is the current deployment phase of the group
	Phase DeploymentPhase `json:"phase"`

	// Message provides additional details about the current phase
	// +optional
	Message string `json:"message,omitempty"`

	// ComponentStatuses contains status for each component
	// +optional
	ComponentStatuses []ComponentStatus `json:"componentStatuses,omitempty"`
}

// ComponentStatus represents the status of a component
type ComponentStatus struct {
	// Name of the component
	Name string `json:"name"`

	// Phase is the current deployment phase of the component
	Phase DeploymentPhase `json:"phase"`

	// Message provides additional details about the current phase
	// +optional
	Message string `json:"message,omitempty"`

	// ResourceRef references the deployed resource
	// +optional
	ResourceRef *ResourceReference `json:"resourceRef,omitempty"`
}

// ResourceReference contains information about a deployed resource
type ResourceReference struct {
	// APIVersion of the resource
	APIVersion string `json:"apiVersion"`

	// Kind of the resource
	Kind string `json:"kind"`

	// Name of the resource
	Name string `json:"name"`

	// Namespace of the resource (if applicable)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// AppBundleStatus defines the observed state of AppBundle.
type AppBundleStatus struct {
	// Phase is the current overall deployment phase
	// +optional
	Phase DeploymentPhase `json:"phase,omitempty"`

	// Message provides additional details about the current phase
	// +optional
	Message string `json:"message,omitempty"`

	// GroupStatuses contains status for each group
	// +optional
	GroupStatuses []GroupStatus `json:"groupStatuses,omitempty"`

	// ObservedGeneration is the last generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the AppBundle's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AppBundle is the Schema for the appbundles API
type AppBundle struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of AppBundle
	// +required
	Spec AppBundleSpec `json:"spec"`

	// status defines the observed state of AppBundle
	// +optional
	Status AppBundleStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// AppBundleList contains a list of AppBundle
type AppBundleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppBundle `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AppBundle{}, &AppBundleList{})
}
