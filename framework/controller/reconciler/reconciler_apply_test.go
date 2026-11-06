//nolint:testpackage
package reconciler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	odherrors "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	. "github.com/onsi/gomega"
)

type recordedEvent struct {
	eventType string
	reason    string
	action    string
	note      string
}

type mockRecorder struct {
	events []recordedEvent
}

func (m *mockRecorder) Eventf(
	_ runtime.Object,
	_ runtime.Object,
	eventType string,
	reason string,
	action string,
	note string,
	_ ...any,
) {
	m.events = append(m.events, recordedEvent{
		eventType: eventType,
		reason:    reason,
		action:    action,
		note:      note,
	})
}

func isStopError(err error) bool {
	var se odherrors.StopError
	return errors.As(err, &se)
}

func stopErrorRequeueAfter(err error) time.Duration {
	var se odherrors.StopError
	if errors.As(err, &se) {
		return se.RequeueAfter()
	}
	return 0
}

func newApplyTestReconciler(
	recorder *mockRecorder,
	actionFn func(context.Context, *types.ReconciliationRequest) error,
) *Reconciler {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)

	cli := fake.NewClientBuilder().
		WithScheme(s).
		Build()

	r := &Reconciler{
		Client:                    cli,
		Scheme:                    s,
		Recorder:                  recorder,
		name:                      "test-reconciler",
		provisioningConditionType: DefaultProvisioningConditionType,
		phaseReady:                DefaultPhaseReady,
		phaseNotReady:             DefaultPhaseNotReady,
		preApplyFailedReason:      "PreConditionFailed",
		preApplyRequeueAfter:      DefaultPreApplyRequeueAfter,
		skipConditionCleanup:      true,
		conditionsAggregator: mustNewAggregator(
			conditions.Dependent(DefaultProvisioningConditionType, conditions.HealthyWhenTrue),
		),
		gvks:                        make(map[schema.GroupVersionKind]gvkInfo),
		excludeFromDynamicOwnership: make(map[schema.GroupVersionKind]struct{}),
		instanceFactory: func() (api.PlatformObject, error) {
			return newTestPlatformObject(testGVKConfigMap), nil
		},
	}

	r.Actions = append(r.Actions, func(ctx context.Context, rr *types.ReconciliationRequest) error {
		return actionFn(ctx, rr)
	})

	return r
}

func newNamedTestObject() *testPlatformObject {
	return newTestPlatformObject(testGVKConfigMap, func(u *unstructured.Unstructured) {
		u.SetName("test-obj")
		u.SetNamespace("default")
	})
}

func TestApply(t *testing.T) { //nolint:funlen
	t.Parallel()

	tests := []struct {
		name             string
		actionErr        error
		wantErr          string
		wantStopError    bool
		wantRequeueAfter time.Duration
		wantEventCount   int
		wantEventType    string
		wantReason       string
		wantNote         string
	}{
		{
			name:           "action succeeds, no event emitted",
			actionErr:      nil,
			wantEventCount: 0,
		},
		{
			name:           "non-StopError emits ProvisioningError warning",
			actionErr:      errors.New("something broke"),
			wantErr:        "Provisioning failed",
			wantStopError:  false,
			wantEventCount: 1,
			wantEventType:  corev1.EventTypeWarning,
			wantReason:     eventReasonProvisioningError,
		},
		{
			name:           "StopError without requeueAfter emits ProvisioningError warning",
			actionErr:      odherrors.NewStopError("missing dependency"),
			wantErr:        "Provisioning failed",
			wantStopError:  true,
			wantEventCount: 1,
			wantEventType:  corev1.EventTypeWarning,
			wantReason:     eventReasonProvisioningError,
		},
		{
			name:             "StopError with requeueAfter emits ProvisioningPaused normal event",
			actionErr:        odherrors.NewStopError("waiting for configmap").WithRequeueAfter(30 * time.Second),
			wantRequeueAfter: 30 * time.Second,
			wantEventCount:   1,
			wantEventType:    corev1.EventTypeNormal,
			wantReason:       eventReasonProvisioningPaused,
			wantNote:         "requeue after 30s",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)
			ctx := t.Context()
			recorder := &mockRecorder{}

			r := newApplyTestReconciler(recorder, func(_ context.Context, _ *types.ReconciliationRequest) error {
				return tc.actionErr
			})

			obj := newNamedTestObject()
			result, err := r.apply(ctx, obj)

			if tc.wantErr != "" {
				g.Expect(err).Should(MatchError(ContainSubstring(tc.wantErr)))
				g.Expect(result.RequeueAfter).To(BeZero())

				if tc.wantStopError {
					g.Expect(err).To(MatchError(isStopError, "IsStopError"))
					g.Expect(stopErrorRequeueAfter(err)).To(Equal(tc.wantRequeueAfter))
				} else {
					g.Expect(err).ToNot(MatchError(isStopError, "IsStopError"))
				}
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(result.RequeueAfter).To(Equal(tc.wantRequeueAfter))
			}

			g.Expect(recorder.events).To(HaveLen(tc.wantEventCount))

			if tc.wantEventCount > 0 {
				g.Expect(recorder.events[0].eventType).To(Equal(tc.wantEventType))
				g.Expect(recorder.events[0].reason).To(Equal(tc.wantReason))

				if tc.wantNote != "" {
					g.Expect(recorder.events[0].note).To(ContainSubstring(tc.wantNote))
				}
			}
		})
	}
}

func TestApply_PreApplyPause(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		configured       bool
		configure        time.Duration
		wantErr          string
		wantRequeueAfter time.Duration
		wantEventType    string
		wantEventReason  string
	}{
		{
			name:             "uses the default pause interval",
			wantRequeueAfter: DefaultPreApplyRequeueAfter,
			wantEventType:    corev1.EventTypeNormal,
			wantEventReason:  eventReasonProvisioningPaused,
		},
		{
			name:             "uses the configured pause interval",
			configured:       true,
			configure:        45 * time.Second,
			wantRequeueAfter: 45 * time.Second,
			wantEventType:    corev1.EventTypeNormal,
			wantEventReason:  eventReasonProvisioningPaused,
		},
		{
			name:            "uses error backoff when the pause interval is disabled",
			configured:      true,
			configure:       0,
			wantErr:         "pre-apply check not met",
			wantEventType:   corev1.EventTypeWarning,
			wantEventReason: eventReasonProvisioningError,
		},
		{
			name:            "uses error backoff when the pause interval is negative",
			configured:      true,
			configure:       -time.Second,
			wantErr:         "pre-apply check not met",
			wantEventType:   corev1.EventTypeWarning,
			wantEventReason: eventReasonProvisioningError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			var calls int
			recorder := &mockRecorder{}
			r := newApplyTestReconciler(recorder, func(context.Context, *types.ReconciliationRequest) error {
				calls++
				return nil
			})
			r.preApplyFn = func(context.Context, *types.ReconciliationRequest) bool {
				return true
			}
			if tc.configured {
				WithPreApplyRequeueAfter(tc.configure)(r)
			}

			obj := newNamedTestObject()
			result, err := r.apply(t.Context(), obj)

			if tc.wantErr != "" {
				g.Expect(err).To(MatchError(ContainSubstring(tc.wantErr)))
				g.Expect(result).To(Equal(ctrl.Result{}))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(result.RequeueAfter).To(Equal(tc.wantRequeueAfter))
			}

			g.Expect(calls).To(BeZero())
			condition := conditions.FindStatusCondition(obj, string(DefaultProvisioningConditionType))
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(condition.Reason).To(Equal(r.preApplyFailedReason))
			g.Expect(recorder.events).To(HaveLen(1))
			g.Expect(recorder.events[0].eventType).To(Equal(tc.wantEventType))
			g.Expect(recorder.events[0].reason).To(Equal(tc.wantEventReason))

			if tc.wantRequeueAfter > 0 {
				g.Expect(recorder.events[0].note).To(ContainSubstring("requeue after " + tc.wantRequeueAfter.String()))
			}
		})
	}
}

func TestApply_StatusWriteErrorEmitsWarning(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	statusErr := errors.New("status write failed")
	recorder := &mockRecorder{}
	r := newApplyTestReconciler(recorder, func(context.Context, *types.ReconciliationRequest) error {
		return nil
	})
	r.Client = fake.NewClientBuilder().
		WithScheme(r.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceApply: func(context.Context, client.Client, string, runtime.ApplyConfiguration, ...client.SubResourceApplyOption) error {
				return statusErr
			},
		}).
		Build()

	result, err := r.apply(t.Context(), newNamedTestObject())

	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(err).To(MatchError(ContainSubstring(statusErr.Error())))
	g.Expect(errors.Is(err, statusErr)).To(BeTrue())
	g.Expect(recorder.events).To(HaveLen(1))
	g.Expect(recorder.events[0].eventType).To(Equal(corev1.EventTypeWarning))
	g.Expect(recorder.events[0].reason).To(Equal(eventReasonReconcileError))
	g.Expect(recorder.events[0].action).To(Equal(eventActionReconcile))
	g.Expect(recorder.events[0].note).To(ContainSubstring(statusErr.Error()))
}

func TestNewReconciler_DefaultAggregatorTracksProvisioning(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	r, err := NewReconciler[*conditionAwarePlatformObject](
		newTestManager(),
		"test-reconciler",
		newConditionAwareObject(),
	)
	g.Expect(err).ShouldNot(HaveOccurred())

	r.Actions = append(r.Actions, func(_ context.Context, _ *types.ReconciliationRequest) error {
		return errors.New("action failed")
	})

	obj := newConditionAwareObject()
	_, err = r.apply(t.Context(), obj)
	g.Expect(err).Should(MatchError(ContainSubstring("Provisioning failed")))

	ready := conditions.FindStatusCondition(obj, string(DefaultHappyCondition))
	g.Expect(ready).ToNot(BeNil())
	g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(obj.GetStatus().Phase).To(Equal(DefaultPhaseNotReady))
}

func TestReconcile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		actionErr        error
		wantErr          string
		wantStopError    bool
		wantRequeueAfter time.Duration
	}{
		{
			name:             "StopError with requeueAfter returns RequeueAfter result, no error",
			actionErr:        odherrors.NewStopError("dependency not ready").WithRequeueAfter(45 * time.Second),
			wantRequeueAfter: 45 * time.Second,
		},
		{
			name:          "StopError without requeueAfter returns error",
			actionErr:     odherrors.NewStopError("fatal stop"),
			wantErr:       "fatal stop",
			wantStopError: true,
		},
		{
			name:          "non-StopError returns error",
			actionErr:     fmt.Errorf("unexpected: %w", errors.New("failure")),
			wantErr:       "failure",
			wantStopError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)
			ctx := t.Context()
			recorder := &mockRecorder{}

			r := newApplyTestReconciler(recorder, func(_ context.Context, _ *types.ReconciliationRequest) error {
				return tc.actionErr
			})

			obj := newNamedTestObject()
			err := r.Client.Create(ctx, obj)
			g.Expect(err).ShouldNot(HaveOccurred())

			result, reconcileErr := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(obj),
			})

			if tc.wantErr != "" {
				g.Expect(reconcileErr).Should(MatchError(ContainSubstring(tc.wantErr)))

				if tc.wantStopError {
					g.Expect(reconcileErr).To(MatchError(isStopError, "IsStopError"))
				} else {
					g.Expect(reconcileErr).ToNot(MatchError(isStopError, "IsStopError"))
				}

				g.Expect(result.RequeueAfter).To(BeZero())
			} else {
				g.Expect(reconcileErr).ShouldNot(HaveOccurred())
				g.Expect(result.RequeueAfter).To(Equal(tc.wantRequeueAfter))
			}
		})
	}
}

func TestApply_PostStatusErrorPreservesActionError(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	actionErr := errors.New("render failed")
	hookErr := errors.New("post status failed")
	r := newApplyTestReconciler(&mockRecorder{}, func(_ context.Context, _ *types.ReconciliationRequest) error {
		return actionErr
	})
	r.postStatusFn = func(context.Context, *types.ReconciliationRequest, bool) error {
		return hookErr
	}

	_, err := r.apply(t.Context(), newNamedTestObject())

	g.Expect(err).To(MatchError(ContainSubstring(actionErr.Error())))
	g.Expect(err).To(MatchError(ContainSubstring(hookErr.Error())))
	g.Expect(errors.Is(err, actionErr)).To(BeTrue())
	g.Expect(errors.Is(err, hookErr)).To(BeTrue())
}
