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
