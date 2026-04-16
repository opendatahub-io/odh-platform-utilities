package common

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// --- Management ---

// ManagementState defines whether a component is actively managed by the
// operator or scheduled for removal.
type ManagementState string

const (
	// Managed indicates the operator should reconcile the component and
	// maintain its desired state.
	Managed ManagementState = "Managed"

	// Removed indicates the operator should remove the component's resources
	// from the cluster.
	Removed ManagementState = "Removed"
)

// ManagementSpec carries the user's management intent for a module.
// Module CRDs embed this struct in their spec.
//
// +kubebuilder:object:generate=true
type ManagementSpec struct {
	// ManagementState controls whether the operator actively manages the
	// component (Managed) or removes it (Removed).
	// +kubebuilder:validation:Enum=Managed;Removed
	// +kubebuilder:default=Managed
	ManagementState ManagementState `json:"managementState,omitempty"`
}

// --- Condition Severity ---

// ConditionSeverity expresses the severity of a Condition.
type ConditionSeverity string

const (
	// ConditionSeverityError indicates a condition that requires immediate
	// attention and likely blocks normal operation. This is the default
	// severity when none is specified.
	ConditionSeverityError ConditionSeverity = ""

	// ConditionSeverityInfo indicates a condition that is informational and
	// does not require immediate action.
	ConditionSeverityInfo ConditionSeverity = "Info"
)

// --- Condition Type Constants ---

// ConditionType is a typed string for condition type identifiers, providing
// compile-time safety against typos in condition lookups.
type ConditionType string

const (
	// ConditionTypeReady is the top-level aggregate condition. The
	// orchestrator checks this before advancing to the next runlevel.
	ConditionTypeReady ConditionType = "Ready"

	// ConditionTypeProvisioningSucceeded reflects the result of manifest
	// application. The orchestrator reads this for status aggregation.
	ConditionTypeProvisioningSucceeded ConditionType = "ProvisioningSucceeded"

	// ConditionTypeDegraded indicates the module is functioning but impaired.
	// The orchestrator reports this in cluster-wide health.
	ConditionTypeDegraded ConditionType = "Degraded"
)

// --- Phase Constants ---

// Phase represents the top-level lifecycle phase of a module.
type Phase string

const (
	// PhaseReady indicates the module is fully operational.
	PhaseReady Phase = "Ready"

	// PhaseNotReady indicates the module is not yet available.
	PhaseNotReady Phase = "Not Ready"
)

// --- Condition ---

// Condition represents an observation of a module's state at a point in time.
// Module controllers set conditions to communicate health signals to the
// orchestrator and to human operators.
//
// +kubebuilder:object:generate=true
type Condition struct {
	// LastTransitionTime is the last time the condition transitioned from one
	// status to another.
	// +required
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`

	// Type is the condition type, e.g. ConditionTypeReady, ConditionTypeDegraded.
	// +required
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`

	// Status is the status of the condition (True, False, Unknown).
	// +required
	Status metav1.ConditionStatus `json:"status"`

	// Reason is a programmatic, one-word CamelCase identifier for the
	// condition's last transition, suitable for machine consumption.
	// +optional
	// +kubebuilder:validation:Optional
	Reason string `json:"reason,omitempty"`

	// Message is a human-readable description providing details about the
	// condition's last transition.
	// +optional
	Message string `json:"message,omitempty"`

	// Severity indicates whether this condition represents an error or is
	// purely informational. Empty string (default) indicates error severity.
	// +optional
	Severity ConditionSeverity `json:"severity,omitempty"`

	// ObservedGeneration is the .metadata.generation of the resource that
	// the condition was set based upon. If this does not match the resource's
	// current generation, the condition is likely stale.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// --- Status ---

// Status is the common status block that every module CR embeds. The
// orchestrator reads these fields generically to assess module health.
//
// +kubebuilder:object:generate=true
type Status struct {
	// Phase is the top-level lifecycle phase of the module.
	Phase Phase `json:"phase,omitempty"`

	// Conditions is the set of condition observations for this module.
	// +listType=atomic
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent .metadata.generation observed
	// by the controller. It allows consumers to determine whether the
	// controller has processed the latest spec changes.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// --- Release Types ---

// ComponentRelease describes a single software component's release metadata.
//
// +kubebuilder:object:generate=true
type ComponentRelease struct {
	// Name is the component name, used as the merge key in lists.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Version is the semantic version of the component.
	Version string `json:"version,omitempty"`

	// RepoURL is the source repository URL for this component release.
	// +optional
	RepoURL string `json:"repoUrl,omitempty"`
}

// ComponentReleaseStatus holds the list of component releases deployed by a
// module. The orchestrator reads this to detect module versions for upgrade
// mode.
//
// +kubebuilder:object:generate=true
type ComponentReleaseStatus struct {
	// Releases is the list of deployed component releases.
	// +listType=map
	// +listMapKey=name
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	Releases []ComponentRelease `json:"releases,omitempty"`
}

// --- Accessor Interfaces ---

// WithStatus provides read access to the common Status block.
type WithStatus interface {
	// GetStatus returns a pointer to the module's common status.
	GetStatus() *Status
}

// ConditionsAccessor provides read/write access to the conditions slice.
type ConditionsAccessor interface {
	// GetConditions returns the current conditions.
	GetConditions() []Condition
	// SetConditions replaces the conditions slice.
	SetConditions(conditions []Condition)
}

// WithReleases provides read/write access to the component release status.
type WithReleases interface {
	// GetReleaseStatus returns a pointer to the release status.
	GetReleaseStatus() *ComponentReleaseStatus
	// SetReleaseStatus replaces the release status.
	SetReleaseStatus(status ComponentReleaseStatus)
}

// --- PlatformObject ---

// PlatformObject is the central interface the orchestrator uses to read any
// module CR. Every module CRD that participates in the ODH platform must
// implement this interface.
//
// It composes client.Object (Kubernetes metadata + runtime.Object) with the
// three accessor interfaces for status, conditions, and release tracking.
type PlatformObject interface {
	client.Object
	WithStatus
	ConditionsAccessor
	WithReleases
}
