package api

import (
	"github.com/operator-framework/api/pkg/lib/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConditionSeverity expresses the severity of a Condition Type failing.
type ConditionSeverity string

const (
	ConditionSeverityError ConditionSeverity = ""
	ConditionSeverityInfo  ConditionSeverity = "Info"

	ConditionReasonError = "Error"
)

// Condition represents an observation of an object's state.
// +kubebuilder:object:generate=true
type Condition struct {
	// type of condition in CamelCase or in foo.example.com/CamelCase.
	//
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
	// +kubebuilder:validation:MaxLength=316
	Type string `json:"type"`

	// status of the condition, one of True, False, Unknown.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=True;False;Unknown
	Status metav1.ConditionStatus `json:"status"`

	// observedGeneration represents the .metadata.generation that the condition was set based upon.
	// +optional
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// lastTransitionTime is the last time the condition transitioned from one status to another.
	// +optional
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=date-time
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`

	// reason contains a programmatic identifier indicating the reason for the condition's last transition.
	// +optional
	// +kubebuilder:validation:Optional
	Reason string `json:"reason,omitempty"`

	// message is a human-readable message indicating details about the transition.
	// +optional
	// +kubebuilder:validation:Optional
	Message string `json:"message,omitempty"`

	// Severity with which to treat failures of this type of condition.
	// When this is not specified, it defaults to Error.
	// +optional
	// +kubebuilder:validation:Optional
	Severity ConditionSeverity `json:"severity,omitempty"`

	// The last time we got an update on a given condition, this should not be set and is
	// present only for backward compatibility reasons.
	// +optional
	// +kubebuilder:validation:Optional
	LastHeartbeatTime *metav1.Time `json:"lastHeartbeatTime,omitempty"`
}

// Status is the common status block shared by all platform objects.
// +kubebuilder:object:generate=true
type Status struct {
	Phase string `json:"phase,omitempty"`

	// The generation observed by the resource controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +listType=atomic
	Conditions []Condition `json:"conditions,omitempty"`
}

func (s *Status) GetConditions() []Condition {
	return s.Conditions
}

func (s *Status) SetConditions(conditions []Condition) {
	s.Conditions = append(s.Conditions[:0:0], conditions...)
}

type WithStatus interface {
	GetStatus() *Status
}

type ConditionsAccessor interface {
	GetConditions() []Condition
	SetConditions(conditions []Condition)
}

type PlatformObject interface {
	client.Object
	WithStatus
	ConditionsAccessor
}

type Platform string

// Release includes information on operator version and platform.
// +kubebuilder:object:generate=true
type Release struct {
	Name    Platform                `json:"name,omitempty"`
	Version version.OperatorVersion `json:"version,omitempty"`
}

// ComponentRelease represents the detailed status of a component release.
// +kubebuilder:object:generate=true
type ComponentRelease struct {
	// +required
	// +kubebuilder:validation:Required
	Name    string `json:"name"              yaml:"name"`
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
	RepoURL string `json:"repoUrl,omitempty" yaml:"repoUrl,omitempty"`
}

// ComponentReleaseStatus tracks the list of component releases, including their name, version, and repository URL.
// +kubebuilder:object:generate=true
type ComponentReleaseStatus struct {
	// +patchMergeKey=name
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=name
	Releases []ComponentRelease `json:"releases,omitempty" yaml:"releases,omitempty"`
}

// WithReleases is implemented by resources that track per-component release metadata.
type WithReleases interface {
	GetReleaseStatus() *[]ComponentRelease
	SetReleaseStatus(status []ComponentRelease)
}
