package render

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// RenderedResourcesTotal is a Prometheus counter that tracks the total
// number of resources rendered per controller and rendering engine.
// Labels: "controller" (lowercase Kind), "engine" (helm/kustomize/template).
//
//nolint:gochecknoglobals
var RenderedResourcesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "action_renderer_manifests_total",
		Help: "Number of rendered resources",
	},
	[]string{
		"controller",
		"engine",
	},
)

//nolint:gochecknoinits
func init() {
	metrics.Registry.MustRegister(RenderedResourcesTotal)
}
