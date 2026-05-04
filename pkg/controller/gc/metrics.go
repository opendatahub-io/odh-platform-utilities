package gc

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// DeletedTotal counts the number of resources deleted by GC per controller.
var DeletedTotal = prometheus.NewCounterVec( //nolint:gochecknoglobals
	prometheus.CounterOpts{
		Name: "action_gc_deleted_total",
		Help: "Number of GCed resources",
	},
	[]string{
		"controller",
	},
)

// CyclesTotal counts the number of GC cycles per controller.
var CyclesTotal = prometheus.NewCounterVec( //nolint:gochecknoglobals
	prometheus.CounterOpts{
		Name: "action_gc_cycles_total",
		Help: "Number of GC cycles",
	},
	[]string{
		"controller",
	},
)

var registerOnce sync.Once //nolint:gochecknoglobals

// RegisterMetrics registers GC metrics with the controller-runtime metrics
// registry. Safe to call multiple times; registration happens only once.
func RegisterMetrics() {
	registerOnce.Do(func() {
		metrics.Registry.MustRegister(DeletedTotal)
		metrics.Registry.MustRegister(CyclesTotal)
	})
}
