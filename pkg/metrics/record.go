package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
)

// RecordPreconditionFailure increments the precondition failure counter for the given module and reason.
func RecordPreconditionFailure(
	module string,
	prerequisite PrerequisiteReason,
) {
	PreconditionFailuresTotal.WithLabelValues(module, string(prerequisite)).Inc()
}

// RecordBuildInfo sets the build info gauge for a module, retiring any previous version.
func RecordBuildInfo(
	module string,
	version string,
	repo string,
) {
	deleteMatchingLabels(BuildInfo, prometheus.Labels{LabelModule: module})
	BuildInfo.WithLabelValues(module, version, repo).Set(1)
}

// RecordComponentRelease sets the last successfully deployed component version, retiring any previous version.
func RecordComponentRelease(
	module string,
	version string,
	repo string,
) {
	deleteMatchingLabels(ComponentRelease, prometheus.Labels{LabelModule: module})
	ComponentRelease.WithLabelValues(module, version, repo).Set(1)
}

// RecordReconcilePhaseDuration observes the duration of a reconcile action phase.
func RecordReconcilePhaseDuration(
	module string,
	phase ReconcilePhase,
	duration time.Duration,
) {
	ReconcilePhaseDurationSeconds.WithLabelValues(module, string(phase)).Observe(duration.Seconds())
}

// SetManagedResources sets the gauge of managed resources per GVK for a module, retiring any GVKs no longer present.
func SetManagedResources(
	module string,
	counts map[string]int,
) {
	deleteMatchingLabels(ManagedResources, prometheus.Labels{LabelModule: module})

	for gvk, count := range counts {
		ManagedResources.WithLabelValues(module, gvk).Set(float64(count))
	}
}

// RecordConditionTransition increments the condition transition counter for a module, condition type, and status.
func RecordConditionTransition(
	module string,
	conditionType common.ConditionType,
	status ConditionStatus,
) {
	ConditionTransitionsTotal.WithLabelValues(module, string(conditionType), string(status)).Inc()
}

func deleteMatchingLabels(vec *prometheus.GaugeVec, match prometheus.Labels) {
	vec.DeletePartialMatch(match)
}
