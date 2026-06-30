package metrics

// Metric name constants (used as the __name__ label value).
const (
	MetricReconcileTotal            = "module_reconcile_total"
	MetricReconcileDurationSeconds  = "module_reconcile_duration_seconds"
	MetricPreconditionFailuresTotal = "module_precondition_failures_total"
	MetricBuildInfo                 = "module_build_info"
	MetricComponentRelease          = "module_component_release"
)

// Label name constants.
const (
	LabelModule       = "module"
	LabelResult       = "result"
	LabelPrerequisite = "prerequisite"
	LabelVersion      = "version"
	LabelRepo         = "repo"
)

// ReconcileResult represents the outcome of a reconcile invocation.
type ReconcileResult string

// Typed result values.
const (
	ResultSuccess ReconcileResult = "success"
	ResultError   ReconcileResult = "error"
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
