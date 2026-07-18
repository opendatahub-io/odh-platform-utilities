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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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
		skipConditionCleanup:      true,
		conditionsManagerFactory: func(accessor api.ConditionsAccessor) *conditions.Manager {
			return conditions.NewManager(accessor, DefaultHappyCondition)
		},
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
			wantErr:        "provisioning failed",
			wantStopError:  false,
			wantEventCount: 1,
			wantEventType:  corev1.EventTypeWarning,
			wantReason:     "ProvisioningError",
		},
		{
			name:           "StopError without requeueAfter emits ProvisioningError warning",
			actionErr:      odherrors.NewStopError("missing dependency"),
			wantErr:        "provisioning failed",
			wantStopError:  true,
			wantEventCount: 1,
			wantEventType:  corev1.EventTypeWarning,
			wantReason:     "ProvisioningError",
		},
		{
			name:             "StopError with requeueAfter emits ProvisioningPaused normal event",
			actionErr:        odherrors.NewStopError("waiting for configmap").WithRequeueAfter(30 * time.Second),
			wantErr:          "provisioning paused",
			wantStopError:    true,
			wantRequeueAfter: 30 * time.Second,
			wantEventCount:   1,
			wantEventType:    corev1.EventTypeNormal,
			wantReason:       "ProvisioningPaused",
			wantNote:         "requeue after 30s",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()
			recorder := &mockRecorder{}

			r := newApplyTestReconciler(recorder, func(_ context.Context, _ *types.ReconciliationRequest) error {
				return tc.actionErr
			})

			obj := newNamedTestObject()
			err := r.apply(ctx, obj)

			if tc.wantErr != "" {
				g.Expect(err).Should(MatchError(ContainSubstring(tc.wantErr)))

				if tc.wantStopError {
					g.Expect(err).To(MatchError(isStopError, "IsStopError"))
					g.Expect(stopErrorRequeueAfter(err)).To(Equal(tc.wantRequeueAfter))
				} else {
					g.Expect(err).ToNot(MatchError(isStopError, "IsStopError"))
				}
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
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

func TestReconcile(t *testing.T) {
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
