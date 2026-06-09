package precondition_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/action"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/precondition"
)

// testPlatformObject is a minimal PlatformObject for testing.
type testPlatformObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status testStatus `json:"status,omitempty"`
}

type testStatus struct {
	common.ComponentReleaseStatus `json:",inline"`
	common.Status                 `json:",inline"`
}

func (t *testPlatformObject) GetStatus() *common.Status {
	return &t.Status.Status
}

func (t *testPlatformObject) GetConditions() []common.Condition {
	return t.Status.Conditions
}

func (t *testPlatformObject) SetConditions(c []common.Condition) {
	t.Status.Conditions = c
}

func (t *testPlatformObject) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &t.Status.ComponentReleaseStatus
}

func (t *testPlatformObject) SetReleaseStatus(s common.ComponentReleaseStatus) {
	t.Status.ComponentReleaseStatus = s
}

func (t *testPlatformObject) DeepCopyObject() runtime.Object {
	if t == nil {
		return nil
	}

	out := *t
	out.ObjectMeta = *t.DeepCopy()
	t.Status.Status.DeepCopyInto(&out.Status.Status)
	t.Status.ComponentReleaseStatus.DeepCopyInto(&out.Status.ComponentReleaseStatus)

	return &out
}

var (
	_ common.PlatformObject = (*testPlatformObject)(nil)

	errTest = errors.New("test error")
)

func passingCheck(_ context.Context, _ *action.ReconciliationRequest) (precondition.CheckResult, error) {
	return precondition.CheckResult{Pass: true}, nil
}

func failingCheck(msg string) precondition.CheckFunc {
	return func(_ context.Context, _ *action.ReconciliationRequest) (precondition.CheckResult, error) {
		return precondition.CheckResult{Pass: false, Message: msg}, nil
	}
}

func errorCheck(_ context.Context, _ *action.ReconciliationRequest) (precondition.CheckResult, error) {
	return precondition.CheckResult{}, errTest
}

func newRR(conditionTypes ...string) *action.ReconciliationRequest {
	instance := &testPlatformObject{ObjectMeta: metav1.ObjectMeta{Name: "test-obj"}}

	return &action.ReconciliationRequest{
		Instance:   instance,
		Conditions: conditions.NewManager(instance, string(common.ConditionTypeReady), conditionTypes...),
	}
}

func TestRunAll_EmptyList(t *testing.T) {
	t.Parallel()

	rr := newRR()
	shouldStop := precondition.RunAll(t.Context(), rr, "", nil)

	assert.False(t, shouldStop)
}

func TestRunAll(t *testing.T) { //nolint:funlen // Table-driven test with many cases.
	t.Parallel()

	tests := []struct { //nolint:govet // fieldalignment: test table struct
		name                   string
		preConditions          []precondition.PreCondition
		generation             int64
		expectedShouldStop     bool
		expectedStatus         metav1.ConditionStatus
		expectedReason         string
		expectedSeverity       common.ConditionSeverity
		expectedMsgContains    []string
		expectedMsgNotContains []string
	}{
		{
			name: "all pass",
			preConditions: []precondition.PreCondition{
				precondition.Custom(passingCheck),
				precondition.Custom(passingCheck),
			},
			expectedShouldStop: false,
			expectedStatus:     metav1.ConditionTrue,
		},
		{
			name: "one fails without stop",
			preConditions: []precondition.PreCondition{
				precondition.Custom(passingCheck),
				precondition.Custom(failingCheck("CRD missing")),
			},
			generation:          5,
			expectedShouldStop:  false,
			expectedStatus:      metav1.ConditionFalse,
			expectedReason:      precondition.PreConditionFailedReason,
			expectedMsgContains: []string{"CRD missing"},
		},
		{
			name: "one fails with stop",
			preConditions: []precondition.PreCondition{
				precondition.Custom(passingCheck),
				precondition.Custom(failingCheck("CRD missing"), precondition.WithStopReconciliation()),
			},
			expectedShouldStop: true,
			expectedStatus:     metav1.ConditionFalse,
		},
		{
			name: "check error yields Unknown",
			preConditions: []precondition.PreCondition{
				precondition.Custom(errorCheck),
			},
			expectedShouldStop:  false,
			expectedStatus:      metav1.ConditionUnknown,
			expectedReason:      precondition.PreConditionFailedReason,
			expectedSeverity:    common.ConditionSeverityError,
			expectedMsgContains: []string{"test error"},
		},
		{
			name: "check error with stop",
			preConditions: []precondition.PreCondition{
				precondition.Custom(errorCheck, precondition.WithStopReconciliation()),
			},
			expectedShouldStop: true,
			expectedStatus:     metav1.ConditionUnknown,
		},
		{
			name: "mixed Unknown and Failed, False wins",
			preConditions: []precondition.PreCondition{
				precondition.Custom(failingCheck("CRD missing")),
				precondition.Custom(errorCheck),
			},
			expectedShouldStop: false,
			expectedStatus:     metav1.ConditionFalse,
		},
		{
			name: "aggregates messages from multiple failures",
			preConditions: []precondition.PreCondition{
				precondition.Custom(failingCheck("CRD A missing")),
				precondition.Custom(failingCheck("CRD B missing")),
			},
			expectedShouldStop:  false,
			expectedStatus:      metav1.ConditionFalse,
			expectedMsgContains: []string{"CRD A missing", "CRD B missing"},
		},
		{
			name: "severity aggregation: Error if any Error",
			preConditions: []precondition.PreCondition{
				precondition.Custom(failingCheck("info dep"), precondition.WithSeverity(common.ConditionSeverityInfo)),
				precondition.Custom(failingCheck("error dep")),
			},
			expectedShouldStop: false,
			expectedStatus:     metav1.ConditionFalse,
			expectedSeverity:   common.ConditionSeverityError,
		},
		{
			name: "severity aggregation: Info if all Info",
			preConditions: []precondition.PreCondition{
				precondition.Custom(failingCheck("info dep 1"), precondition.WithSeverity(common.ConditionSeverityInfo)),
				precondition.Custom(failingCheck("info dep 2"), precondition.WithSeverity(common.ConditionSeverityInfo)),
			},
			expectedShouldStop: false,
			expectedStatus:     metav1.ConditionFalse,
			expectedSeverity:   common.ConditionSeverityInfo,
		},
		{
			name: "custom message overrides check result",
			preConditions: []precondition.PreCondition{
				precondition.Custom(failingCheck("original msg"), precondition.WithMessage("custom guidance")),
			},
			expectedShouldStop:     false,
			expectedStatus:         metav1.ConditionFalse,
			expectedMsgContains:    []string{"custom guidance"},
			expectedMsgNotContains: []string{"original msg"},
		},
		{
			name: "nil check honors severity and stop",
			preConditions: []precondition.PreCondition{
				precondition.Custom(nil,
					precondition.WithSeverity(common.ConditionSeverityError),
					precondition.WithStopReconciliation(),
				),
			},
			expectedShouldStop:  true,
			expectedStatus:      metav1.ConditionUnknown,
			expectedSeverity:    common.ConditionSeverityError,
			expectedMsgContains: []string{"precondition check function is nil"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rr := newRR(precondition.ConditionTypeDependenciesAvailable)
			if tt.generation > 0 {
				rr.Instance.SetGeneration(tt.generation)
			}

			shouldStop := precondition.RunAll(t.Context(), rr, "", tt.preConditions)
			assert.Equal(t, tt.expectedShouldStop, shouldStop)

			got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
			require.NotNil(t, got)
			assert.Equal(t, tt.expectedStatus, got.Status)

			if tt.expectedStatus == metav1.ConditionTrue {
				assert.Empty(t, got.Reason)
				assert.Empty(t, got.Message)
			}

			if tt.expectedReason != "" {
				assert.Equal(t, tt.expectedReason, got.Reason)
			}

			if tt.expectedSeverity != "" {
				assert.Equal(t, tt.expectedSeverity, got.Severity)
			}

			for _, s := range tt.expectedMsgContains {
				assert.Contains(t, got.Message, s)
			}

			for _, s := range tt.expectedMsgNotContains {
				assert.NotContains(t, got.Message, s)
			}

			if tt.generation > 0 {
				assert.Equal(t, tt.generation, got.ObservedGeneration)
			}
		})
	}
}

func TestRunAll_ClusterTypeFiltering(t *testing.T) {
	t.Parallel()

	rr := newRR(precondition.ConditionTypeDependenciesAvailable)
	pcs := []precondition.PreCondition{
		precondition.Custom(
			failingCheck("k8s only check"),
			precondition.WithClusterTypes(cluster.ClusterTypeKubernetes),
			precondition.WithStopReconciliation(),
		),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, cluster.ClusterTypeOpenShift, pcs)

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.NotEqual(t, metav1.ConditionFalse, got.Status)
}

func TestRunAll_ClusterTypeFilterRunsBeforeSkipFunc(t *testing.T) {
	t.Parallel()

	skipFuncCalled := false

	rr := newRR(precondition.ConditionTypeDependenciesAvailable)
	pcs := []precondition.PreCondition{
		precondition.Custom(
			failingCheck("should not run"),
			precondition.WithClusterTypes(cluster.ClusterTypeKubernetes),
			precondition.WithSkipFunc(func(_ context.Context, _ *action.ReconciliationRequest) (bool, error) {
				skipFuncCalled = true

				return false, nil
			}),
			precondition.WithStopReconciliation(),
		),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, cluster.ClusterTypeOpenShift, pcs)

	assert.False(t, shouldStop)
	assert.False(t, skipFuncCalled, "SkipFunc should not be called when cluster type filter already skipped")
}

func TestRunAll_SkipFunc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		skipFunc       precondition.SkipFunc
		expectedStatus metav1.ConditionStatus
		expectedMsg    string
	}{
		{
			name:           "returns true skips precondition",
			skipFunc:       func(_ context.Context, _ *action.ReconciliationRequest) (bool, error) { return true, nil },
			expectedStatus: metav1.ConditionUnknown,
		},
		{
			name:           "returns false runs precondition",
			skipFunc:       func(_ context.Context, _ *action.ReconciliationRequest) (bool, error) { return false, nil },
			expectedStatus: metav1.ConditionFalse,
			expectedMsg:    "skip func test",
		},
		{
			name:           "nil runs precondition",
			skipFunc:       nil,
			expectedStatus: metav1.ConditionFalse,
			expectedMsg:    "skip func test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rr := newRR(precondition.ConditionTypeDependenciesAvailable)

			var opts []precondition.Option
			if tt.skipFunc != nil {
				opts = append(opts, precondition.WithSkipFunc(tt.skipFunc))
			}

			pcs := []precondition.PreCondition{
				precondition.Custom(failingCheck("skip func test"), opts...),
			}

			precondition.RunAll(t.Context(), rr, "", pcs)

			got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
			require.NotNil(t, got)
			assert.Equal(t, tt.expectedStatus, got.Status)

			if tt.expectedMsg != "" {
				assert.Contains(t, got.Message, tt.expectedMsg)
			}
		})
	}
}

func TestRunAll_SkipFuncError(t *testing.T) {
	t.Parallel()

	rr := newRR(precondition.ConditionTypeDependenciesAvailable)
	pcs := []precondition.PreCondition{
		precondition.Custom(
			failingCheck("should not run"),
			precondition.WithSkipFunc(func(_ context.Context, _ *action.ReconciliationRequest) (bool, error) {
				return false, errTest
			}),
		),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, "", pcs)
	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionUnknown, got.Status)
	assert.Contains(t, got.Message, "test error")
}

func TestRunAll_MultipleConditionTypes(t *testing.T) {
	t.Parallel()

	customCondition := "CustomDeps"
	rr := newRR(precondition.ConditionTypeDependenciesAvailable, customCondition)

	pcs := []precondition.PreCondition{
		precondition.Custom(passingCheck),
		precondition.Custom(failingCheck("custom failed"), precondition.WithConditionType(customCondition)),
	}

	precondition.RunAll(t.Context(), rr, "", pcs)

	defaultCond := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, defaultCond)
	assert.Equal(t, metav1.ConditionTrue, defaultCond.Status)

	customCond := rr.Conditions.GetCondition(customCondition)
	require.NotNil(t, customCond)
	assert.Equal(t, metav1.ConditionFalse, customCond.Status)
	assert.Contains(t, customCond.Message, "custom failed")
}

func TestRunAll_AllPreconditionsRunEvenWhenSomeFail(t *testing.T) {
	t.Parallel()

	callCount := 0
	countingCheck := precondition.CheckFunc(
		func(_ context.Context, _ *action.ReconciliationRequest) (precondition.CheckResult, error) {
			callCount++

			return precondition.CheckResult{Pass: false, Message: "fail"}, nil
		},
	)

	rr := newRR(precondition.ConditionTypeDependenciesAvailable)
	pcs := []precondition.PreCondition{
		precondition.Custom(countingCheck, precondition.WithStopReconciliation()),
		precondition.Custom(countingCheck, precondition.WithStopReconciliation()),
		precondition.Custom(countingCheck, precondition.WithStopReconciliation()),
	}

	precondition.RunAll(t.Context(), rr, "", pcs)

	assert.Equal(t, 3, callCount)
}

func TestCustom_Defaults(t *testing.T) {
	t.Parallel()

	rr := newRR(precondition.ConditionTypeDependenciesAvailable)
	shouldStop := precondition.RunAll(t.Context(), rr, "", []precondition.PreCondition{
		precondition.Custom(passingCheck),
	})

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionTrue, got.Status)
}

func TestCustom_FailingCheck(t *testing.T) {
	t.Parallel()

	rr := newRR(precondition.ConditionTypeDependenciesAvailable)
	shouldStop := precondition.RunAll(t.Context(), rr, "", []precondition.PreCondition{
		precondition.Custom(failingCheck("component not ready")),
	})

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionFalse, got.Status)
	assert.Contains(t, got.Message, "component not ready")
}

func TestCustom_ErrorCheck(t *testing.T) {
	t.Parallel()

	rr := newRR(precondition.ConditionTypeDependenciesAvailable)
	shouldStop := precondition.RunAll(t.Context(), rr, "", []precondition.PreCondition{
		precondition.Custom(errorCheck),
	})

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionUnknown, got.Status)
}

func TestCustom_WithStopReconciliation(t *testing.T) {
	t.Parallel()

	rr := newRR(precondition.ConditionTypeDependenciesAvailable)
	shouldStop := precondition.RunAll(t.Context(), rr, "", []precondition.PreCondition{
		precondition.Custom(failingCheck("must stop"), precondition.WithStopReconciliation()),
	})

	assert.True(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionFalse, got.Status)
}

func TestCustom_AllOptions(t *testing.T) {
	t.Parallel()

	customCondition := "CustomCheck"
	rr := newRR(customCondition)
	shouldStop := precondition.RunAll(t.Context(), rr, "", []precondition.PreCondition{
		precondition.Custom(
			failingCheck("original"),
			precondition.WithConditionType(customCondition),
			precondition.WithSeverity(common.ConditionSeverityInfo),
			precondition.WithStopReconciliation(),
			precondition.WithMessage("overridden message"),
		),
	})

	assert.True(t, shouldStop)

	got := rr.Conditions.GetCondition(customCondition)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionFalse, got.Status)
	assert.Equal(t, common.ConditionSeverityInfo, got.Severity)
	assert.Contains(t, got.Message, "overridden message")
	assert.NotContains(t, got.Message, "original")
}

func TestCustom_AccessesInstance(t *testing.T) {
	t.Parallel()

	instanceCheck := func(_ context.Context, rr *action.ReconciliationRequest) (precondition.CheckResult, error) {
		if rr.Instance == nil {
			return precondition.CheckResult{}, errTest
		}

		if rr.Instance.GetName() == "" {
			return precondition.CheckResult{Pass: false, Message: "instance has no name"}, nil
		}

		return precondition.CheckResult{Pass: true}, nil
	}

	rr := newRR(precondition.ConditionTypeDependenciesAvailable)
	shouldStop := precondition.RunAll(t.Context(), rr, "", []precondition.PreCondition{
		precondition.Custom(instanceCheck),
	})

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionTrue, got.Status)
}

func TestCustom_NilCheck(t *testing.T) {
	t.Parallel()

	rr := newRR(precondition.ConditionTypeDependenciesAvailable)
	shouldStop := precondition.RunAll(t.Context(), rr, "", []precondition.PreCondition{
		precondition.Custom(nil),
	})

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionUnknown, got.Status)
	assert.Contains(t, got.Message, "precondition check function is nil")
}

func TestCustom_NilCheckWithStopReconciliation(t *testing.T) {
	t.Parallel()

	rr := newRR(precondition.ConditionTypeDependenciesAvailable)
	shouldStop := precondition.RunAll(t.Context(), rr, "", []precondition.PreCondition{
		precondition.Custom(nil, precondition.WithStopReconciliation()),
	})

	assert.True(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionUnknown, got.Status)
}

func TestCustom_SkippedOnClusterType(t *testing.T) {
	t.Parallel()

	rr := newRR(precondition.ConditionTypeDependenciesAvailable)
	shouldStop := precondition.RunAll(t.Context(), rr, cluster.ClusterTypeOpenShift, []precondition.PreCondition{
		precondition.Custom(failingCheck("should not run"), precondition.WithClusterTypes(cluster.ClusterTypeKubernetes)),
	})

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.NotEqual(t, metav1.ConditionFalse, got.Status)
}
