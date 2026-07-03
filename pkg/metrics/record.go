package metrics

import (
	"time"
)

// RecordPreconditionFailure increments [PreconditionFailuresTotal] when a
// module detects a missing prerequisite.
func RecordPreconditionFailure(
	module string,
	prerequisite PrerequisiteReason,
) {
	PreconditionFailuresTotal.WithLabelValues(module, string(prerequisite)).Inc()
}

// RecordBuildInfo sets [BuildInfo] to 1 with the module's version and source
// repository. Typically called once at startup.
func RecordBuildInfo(
	module string,
	version string,
	repo string,
) {
	BuildInfo.WithLabelValues(module, version, repo).Set(1)
}

// RecordComponentRelease sets [ComponentRelease] to 1 tracking the last
// successfully deployed component version.
func RecordComponentRelease(
	module string,
	version string,
	repo string,
) {
	ComponentRelease.WithLabelValues(module, version, repo).Set(1)
}

// RecordReconcilePhaseDuration observes the duration of a single reconcile
// action phase (render, deploy, gc) in [ReconcilePhaseDurationSeconds].
func RecordReconcilePhaseDuration(
	module string,
	phase ReconcilePhase,
	duration time.Duration,
) {
	ReconcilePhaseDurationSeconds.WithLabelValues(module, string(phase)).Observe(duration.Seconds())
}

// SetManagedResources sets [ManagedResources] to the current count of
// resources the module manages for a given GVK.
func SetManagedResources(
	module string,
	groupVersionKind string,
	count int,
) {
	ManagedResources.WithLabelValues(module, groupVersionKind).Set(float64(count))
}

// RecordConditionTransition increments [ConditionTransitionsTotal] when a
// condition changes status.
func RecordConditionTransition(
	module string,
	conditionType string,
	status ConditionStatus,
) {
	ConditionTransitionsTotal.WithLabelValues(module, conditionType, string(status)).Inc()
}
