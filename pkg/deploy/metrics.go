package deploy

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// DeployedResourcesTotal tracks the number of resources applied per
// controller. The "controller" label is the lowercased Kind name of the
// reconciling CR.
//
//nolint:gochecknoglobals
var DeployedResourcesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "deploy_resources_total",
		Help: "Number of deployed resources",
	},
	[]string{
		"controller",
	},
)

//nolint:gochecknoinits
func init() {
	metrics.Registry.MustRegister(DeployedResourcesTotal)
}
