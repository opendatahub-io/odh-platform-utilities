package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
)

func RecordPreconditionFailure(
	module string,
	prerequisite PrerequisiteReason,
) {
	PreconditionFailuresTotal.WithLabelValues(module, string(prerequisite)).Inc()
}

func RecordBuildInfo(
	module string,
	version string,
	repo string,
) {
	deleteMatchingLabels(BuildInfo, prometheus.Labels{LabelModule: module})
	BuildInfo.WithLabelValues(module, version, repo).Set(1)
}

func RecordComponentRelease(
	module string,
	version string,
	repo string,
) {
	deleteMatchingLabels(ComponentRelease, prometheus.Labels{LabelModule: module})
	ComponentRelease.WithLabelValues(module, version, repo).Set(1)
}

func RecordReconcilePhaseDuration(
	module string,
	phase ReconcilePhase,
	duration time.Duration,
) {
	ReconcilePhaseDurationSeconds.WithLabelValues(module, string(phase)).Observe(duration.Seconds())
}

func SetManagedResources(
	module string,
	counts map[string]int,
) {
	deleteMatchingLabels(ManagedResources, prometheus.Labels{LabelModule: module})

	for gvk, count := range counts {
		ManagedResources.WithLabelValues(module, gvk).Set(float64(count))
	}
}

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
