package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

//nolint:gochecknoglobals
var (
	PreconditionFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricPreconditionFailuresTotal,
			Help: "Total precondition failures by reason",
		},
		[]string{LabelModule, LabelPrerequisite},
	)

	BuildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: MetricBuildInfo,
			Help: "Build info for the running module",
		},
		[]string{LabelModule, LabelVersion, LabelRepo},
	)

	ComponentRelease = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: MetricComponentRelease,
			Help: "Last successfully deployed component version",
		},
		[]string{LabelModule, LabelVersion, LabelRepo},
	)

	// ReconcilePhaseDurationSeconds complements controller_runtime_reconcile_time_seconds
	// (total reconcile duration) by measuring per-phase time (render/deploy/gc).
	ReconcilePhaseDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    MetricReconcilePhaseDurationSeconds,
			Help:    "Duration of each reconcile action phase in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{LabelModule, LabelPhase},
	)

	ManagedResources = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: MetricManagedResources,
			Help: "Number of resources currently managed by the module",
		},
		[]string{LabelModule, LabelGroupVersionKind},
	)

	ConditionTransitionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricConditionTransitionsTotal,
			Help: "Total condition status transitions",
		},
		[]string{LabelModule, LabelConditionType, LabelStatus},
	)
)

//nolint:gochecknoinits
func init() {
	metrics.Registry.MustRegister(
		PreconditionFailuresTotal,
		BuildInfo,
		ComponentRelease,
		ReconcilePhaseDurationSeconds,
		ManagedResources,
		ConditionTransitionsTotal,
	)
}
