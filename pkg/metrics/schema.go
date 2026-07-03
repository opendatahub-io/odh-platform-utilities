package metrics

// Metric name constants (used as the __name__ label value).
const (
	MetricPreconditionFailuresTotal     = "module_precondition_failures_total"
	MetricBuildInfo                     = "module_build_info"
	MetricComponentRelease              = "module_component_release"
	MetricReconcilePhaseDurationSeconds = "module_reconcile_phase_duration_seconds"
	MetricManagedResources              = "module_managed_resources"
	MetricConditionTransitionsTotal     = "module_condition_transitions_total"
)

// Label name constants.
const (
	LabelModule           = "module"
	LabelPrerequisite     = "prerequisite"
	LabelVersion          = "version"
	LabelRepo             = "repo"
	LabelPhase            = "phase"
	LabelGroupVersionKind = "group_version_kind"
	LabelConditionType    = "condition_type"
	LabelStatus           = "status"
)

// PrerequisiteReason represents the reason for a precondition failure.
type PrerequisiteReason string

const (
	PrerequisiteMissingDependency    PrerequisiteReason = "missing_dependency"
	PrerequisiteMissingConfiguration PrerequisiteReason = "missing_configuration"
	PrerequisiteAPIUnavailable       PrerequisiteReason = "api_unavailable"
	PrerequisiteInsufficientRBAC     PrerequisiteReason = "insufficient_rbac"
	PrerequisiteCRDNotFound          PrerequisiteReason = "crd_not_found"
	PrerequisiteComponentNotReady    PrerequisiteReason = "component_not_ready"
)

// ReconcilePhase represents a phase of the action pipeline.
type ReconcilePhase string

const (
	PhaseRender ReconcilePhase = "render"
	PhaseDeploy ReconcilePhase = "deploy"
	PhaseGC     ReconcilePhase = "gc"
)

// ConditionStatus represents a Kubernetes condition status value.
type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)
