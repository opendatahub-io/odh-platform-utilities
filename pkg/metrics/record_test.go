package metrics_test

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metrics"
)

//nolint:paralleltest
func TestRecordPreconditionFailure(t *testing.T) {
	metrics.PreconditionFailuresTotal.Reset()

	metrics.RecordPreconditionFailure("monitoring", metrics.PrerequisiteMissingDependency)

	val := testutil.ToFloat64(
		metrics.PreconditionFailuresTotal.WithLabelValues("monitoring", "missing_dependency"),
	)
	assert.InDelta(t, 1.0, val, 0.001)
}

//nolint:paralleltest
func TestRecordPreconditionFailure_AllReasons(t *testing.T) {
	reasons := []struct {
		reason metrics.PrerequisiteReason
		label  string
	}{
		{metrics.PrerequisiteMissingDependency, "missing_dependency"},
		{metrics.PrerequisiteMissingConfiguration, "missing_configuration"},
		{metrics.PrerequisiteAPIUnavailable, "api_unavailable"},
		{metrics.PrerequisiteInsufficientRBAC, "insufficient_rbac"},
		{metrics.PrerequisiteCRDNotFound, "crd_not_found"},
		{metrics.PrerequisiteComponentNotReady, "component_not_ready"},
	}

	for _, r := range reasons {
		t.Run(r.label, func(t *testing.T) {
			metrics.PreconditionFailuresTotal.Reset()

			metrics.RecordPreconditionFailure("test-module", r.reason)

			val := testutil.ToFloat64(
				metrics.PreconditionFailuresTotal.WithLabelValues("test-module", r.label),
			)
			assert.InDelta(t, 1.0, val, 0.001)
		})
	}
}

//nolint:paralleltest
func TestRecordBuildInfo(t *testing.T) {
	metrics.BuildInfo.Reset()

	metrics.RecordBuildInfo("monitoring", "v0.3.1", "odh-observability")

	val := testutil.ToFloat64(
		metrics.BuildInfo.WithLabelValues("monitoring", "v0.3.1", "odh-observability"),
	)
	assert.InDelta(t, 1.0, val, 0.001)
}

//nolint:paralleltest
func TestRecordBuildInfo_RetiresOldVersion(t *testing.T) {
	metrics.BuildInfo.Reset()

	metrics.RecordBuildInfo("monitoring", "v0.3.1", "odh-observability")
	metrics.RecordBuildInfo("monitoring", "v0.4.0", "odh-observability")

	count := testutil.CollectAndCount(metrics.BuildInfo)
	assert.Equal(t, 1, count, "old version series must be retired")

	val := testutil.ToFloat64(
		metrics.BuildInfo.WithLabelValues("monitoring", "v0.4.0", "odh-observability"),
	)
	assert.InDelta(t, 1.0, val, 0.001)
}

//nolint:paralleltest
func TestRecordComponentRelease(t *testing.T) {
	metrics.ComponentRelease.Reset()

	metrics.RecordComponentRelease("monitoring", "v0.4.0", "odh-observability")

	val := testutil.ToFloat64(
		metrics.ComponentRelease.WithLabelValues("monitoring", "v0.4.0", "odh-observability"),
	)
	assert.InDelta(t, 1.0, val, 0.001)
}

//nolint:paralleltest
func TestRecordComponentRelease_RetiresOldVersion(t *testing.T) {
	metrics.ComponentRelease.Reset()

	metrics.RecordComponentRelease("monitoring", "v0.3.0", "odh-observability")
	metrics.RecordComponentRelease("monitoring", "v0.4.0", "odh-observability")

	count := testutil.CollectAndCount(metrics.ComponentRelease)
	assert.Equal(t, 1, count, "old version series must be retired")

	val := testutil.ToFloat64(
		metrics.ComponentRelease.WithLabelValues("monitoring", "v0.4.0", "odh-observability"),
	)
	assert.InDelta(t, 1.0, val, 0.001)
}

//nolint:paralleltest
func TestRecordReconcilePhaseDuration(t *testing.T) {
	metrics.ReconcilePhaseDurationSeconds.Reset()

	metrics.RecordReconcilePhaseDuration("dashboard", metrics.PhaseRender, 100*time.Millisecond)
	metrics.RecordReconcilePhaseDuration("dashboard", metrics.PhaseDeploy, 500*time.Millisecond)
	metrics.RecordReconcilePhaseDuration("dashboard", metrics.PhaseGC, 50*time.Millisecond)

	count := testutil.CollectAndCount(metrics.ReconcilePhaseDurationSeconds)
	assert.Equal(t, 3, count, "should have observations for 3 phases")
}

//nolint:paralleltest
func TestRecordReconcilePhaseDuration_MultipleObservations(t *testing.T) {
	metrics.ReconcilePhaseDurationSeconds.Reset()

	metrics.RecordReconcilePhaseDuration("dashboard", metrics.PhaseRender, 200*time.Millisecond)
	metrics.RecordReconcilePhaseDuration("dashboard", metrics.PhaseRender, 300*time.Millisecond)

	count := testutil.CollectAndCount(metrics.ReconcilePhaseDurationSeconds)
	assert.Positive(t, count)
}

//nolint:paralleltest
func TestSetManagedResources(t *testing.T) {
	metrics.ManagedResources.Reset()

	metrics.SetManagedResources("dashboard", map[string]int{
		"apps/v1/Deployment": 5,
		"v1/ConfigMap":       12,
	})

	deployments := testutil.ToFloat64(
		metrics.ManagedResources.WithLabelValues("dashboard", "apps/v1/Deployment"),
	)
	assert.InDelta(t, 5.0, deployments, 0.001)

	configmaps := testutil.ToFloat64(
		metrics.ManagedResources.WithLabelValues("dashboard", "v1/ConfigMap"),
	)
	assert.InDelta(t, 12.0, configmaps, 0.001)
}

//nolint:paralleltest
func TestSetManagedResources_RetiresOldGVKs(t *testing.T) {
	metrics.ManagedResources.Reset()

	metrics.SetManagedResources("dashboard", map[string]int{
		"apps/v1/Deployment": 5,
		"v1/ConfigMap":       12,
		"v1/Secret":          3,
	})

	// Second call without v1/Secret — it should be retired
	metrics.SetManagedResources("dashboard", map[string]int{
		"apps/v1/Deployment": 4,
		"v1/ConfigMap":       10,
	})

	count := testutil.CollectAndCount(metrics.ManagedResources)
	assert.Equal(t, 2, count, "v1/Secret series must be retired")

	deployments := testutil.ToFloat64(
		metrics.ManagedResources.WithLabelValues("dashboard", "apps/v1/Deployment"),
	)
	assert.InDelta(t, 4.0, deployments, 0.001)
}

//nolint:paralleltest
func TestRecordConditionTransition(t *testing.T) {
	metrics.ConditionTransitionsTotal.Reset()

	metrics.RecordConditionTransition("dashboard", common.ConditionTypeReady, metrics.ConditionTrue)
	metrics.RecordConditionTransition("dashboard", common.ConditionTypeReady, metrics.ConditionFalse)
	metrics.RecordConditionTransition("dashboard", common.ConditionTypeReady, metrics.ConditionTrue)

	trueVal := testutil.ToFloat64(
		metrics.ConditionTransitionsTotal.WithLabelValues("dashboard", "Ready", "True"),
	)
	assert.InDelta(t, 2.0, trueVal, 0.001)

	falseVal := testutil.ToFloat64(
		metrics.ConditionTransitionsTotal.WithLabelValues("dashboard", "Ready", "False"),
	)
	assert.InDelta(t, 1.0, falseVal, 0.001)
}

//nolint:paralleltest
func TestRecordConditionTransition_AllStatuses(t *testing.T) {
	statuses := []struct {
		status metrics.ConditionStatus
		label  string
	}{
		{metrics.ConditionTrue, "True"},
		{metrics.ConditionFalse, "False"},
		{metrics.ConditionUnknown, "Unknown"},
	}

	for _, s := range statuses {
		t.Run(s.label, func(t *testing.T) {
			metrics.ConditionTransitionsTotal.Reset()

			metrics.RecordConditionTransition("test-module", common.ConditionTypeReady, s.status)

			val := testutil.ToFloat64(
				metrics.ConditionTransitionsTotal.WithLabelValues("test-module", "Ready", s.label),
			)
			assert.InDelta(t, 1.0, val, 0.001)
		})
	}
}
