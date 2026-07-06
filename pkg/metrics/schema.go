package metrics

const (
	MetricPreconditionFailuresTotal     = "module_precondition_failures_total"
	MetricBuildInfo                     = "module_build_info"
	MetricComponentRelease              = "module_component_release"
	MetricReconcilePhaseDurationSeconds = "module_reconcile_phase_duration_seconds"
	MetricManagedResources              = "module_managed_resources"
	MetricConditionTransitionsTotal     = "module_condition_transitions_total"
)

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

type PrerequisiteReason string

const (
	PrerequisiteMissingDependency    PrerequisiteReason = "missing_dependency"
	PrerequisiteMissingConfiguration PrerequisiteReason = "missing_configuration"
	PrerequisiteAPIUnavailable       PrerequisiteReason = "api_unavailable"
	PrerequisiteInsufficientRBAC     PrerequisiteReason = "insufficient_rbac"
	PrerequisiteCRDNotFound          PrerequisiteReason = "crd_not_found"
	PrerequisiteComponentNotReady    PrerequisiteReason = "component_not_ready"
)

type ReconcilePhase string

const (
	PhaseRender ReconcilePhase = "render"
	PhaseDeploy ReconcilePhase = "deploy"
	PhaseGC     ReconcilePhase = "gc"
)

type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

// Note: the condition_type label is bounded by common.ConditionType, a typed
// string defined in api/common with a fixed set of values (Ready,
// ProvisioningSucceeded, Degraded). RecordConditionTransition enforces this
// at compile time by accepting common.ConditionType rather than a raw string.
