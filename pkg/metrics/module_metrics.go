package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ReconcileDuration measures reconcile loop duration in seconds.
//
//nolint:gochecknoglobals
var ReconcileDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: "odh",
		Subsystem: "module",
		Name:      "reconcile_duration_seconds",
		Help:      "Duration of reconcile loop per module operator",
		Buckets:   prometheus.DefBuckets,
	},
	[]string{"module", "result"},
)

// ReconcileTotal counts total reconcile invocations.
//
//nolint:gochecknoglobals
var ReconcileTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "odh",
		Subsystem: "module",
		Name:      "reconcile_total",
		Help:      "Total number of reconcile invocations per module operator",
	},
	[]string{"module", "result"},
)

// Register registers all module metrics with the controller-runtime registry.
func Register() {
	ctrlmetrics.Registry.MustRegister(ReconcileDuration, ReconcileTotal)
}
