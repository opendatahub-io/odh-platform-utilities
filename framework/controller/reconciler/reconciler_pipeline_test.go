//nolint:testpackage
package reconciler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	odherrors "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	. "github.com/onsi/gomega"
)

func TestApply_ActionErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("plain errors stop later actions", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		firstErr := errors.New("first failure")
		var calls []string
		recorder := &mockRecorder{}
		r := newApplyTestReconciler(recorder, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "first")
				return firstErr
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "second")
				return nil
			},
		}

		_, err := r.apply(t.Context(), newNamedTestObject())

		g.Expect(err).To(MatchError(ContainSubstring("first failure")))
		g.Expect(errors.Is(err, firstErr)).To(BeTrue())
		g.Expect(calls).To(Equal([]string{"first"}))
		g.Expect(recorder.events).To(HaveLen(1))
		g.Expect(recorder.events[0].eventType).To(Equal(corev1.EventTypeWarning))
		g.Expect(recorder.events[0].reason).To(Equal(eventReasonProvisioningError))
	})

	t.Run("StopError stops later actions", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		r := newApplyTestReconciler(&mockRecorder{}, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "stop")
				return odherrors.NewStopError("stop now")
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-stop")
				return nil
			},
		}

		_, err := r.apply(t.Context(), newNamedTestObject())

		g.Expect(err).To(MatchError(isStopError, "IsStopError"))
		g.Expect(calls).To(Equal([]string{"stop"}))
	})

	t.Run("terminal errors retain earlier non-terminal diagnostics", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ordinaryErr := errors.New("render failed")
		var calls []string
		recorder := &mockRecorder{}
		r := newApplyTestReconciler(recorder, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "failure")
				return odherrors.NewActionErrorW(ordinaryErr).NonBlocking()
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "stop")
				return odherrors.NewStopError("dependency pending").WithRequeueAfter(time.Minute)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-stop")
				return nil
			},
		}

		obj := newNamedTestObject()
		result, err := r.apply(t.Context(), obj)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(time.Minute))
		g.Expect(calls).To(Equal([]string{"failure", "stop"}))
		condition := conditions.FindStatusCondition(obj, string(DefaultProvisioningConditionType))
		g.Expect(condition.Message).To(ContainSubstring(ordinaryErr.Error()))
		g.Expect(condition.Message).To(ContainSubstring("dependency pending"))
		g.Expect(recorder.events).To(HaveLen(1))
		g.Expect(recorder.events[0].eventType).To(Equal(corev1.EventTypeNormal))
		g.Expect(recorder.events[0].reason).To(Equal(eventReasonProvisioningPaused))
	})

	t.Run("RequeueAfterError continues actions and uses the earliest delay", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		recorder := &mockRecorder{}
		r := newApplyTestReconciler(recorder, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "first")
				return odherrors.NewRequeueAfterError(45 * time.Second)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "second")
				return odherrors.NewRequeueAfterError(15 * time.Second)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "third")
				return nil
			},
		}

		result, err := r.apply(t.Context(), newNamedTestObject())

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(15 * time.Second))
		g.Expect(calls).To(Equal([]string{"first", "second", "third"}))
		g.Expect(recorder.events).To(BeEmpty())
	})

	t.Run("a terminal error ignores an earlier legacy requeue marker", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		r := newApplyTestReconciler(&mockRecorder{}, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "requeue")
				return odherrors.NewRequeueAfterError(15 * time.Second)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "stop")
				return odherrors.NewStopError("waiting").WithRequeueAfter(time.Minute)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-stop")
				return nil
			},
		}

		result, err := r.apply(t.Context(), newNamedTestObject())

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(time.Minute))
		g.Expect(calls).To(Equal([]string{"requeue", "stop"}))
	})

	t.Run("ordinary errors joined with a requeue marker are reported", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ordinaryErr := errors.New("render failed")
		var calls []string
		r := newApplyTestReconciler(&mockRecorder{}, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "mixed")
				return errors.Join(ordinaryErr, odherrors.NewRequeueAfterError(time.Minute))
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-mixed")
				return nil
			},
		}

		result, err := r.apply(t.Context(), newNamedTestObject())

		g.Expect(err).To(MatchError(ContainSubstring(ordinaryErr.Error())))
		g.Expect(errors.Is(err, ordinaryErr)).To(BeTrue())
		g.Expect(result.RequeueAfter).To(BeZero())
		g.Expect(calls).To(Equal([]string{"mixed"}))
	})

	t.Run("an ordinary error joined with a delayed terminal uses error backoff", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ordinaryErr := errors.New("render failed")
		var calls []string
		r := newApplyTestReconciler(&mockRecorder{}, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "mixed")
				return errors.Join(
					ordinaryErr,
					odherrors.NewStopError("waiting").WithRequeueAfter(time.Minute),
				)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-stop")
				return nil
			},
		}

		result, err := r.apply(t.Context(), newNamedTestObject())

		g.Expect(err).To(MatchError(ContainSubstring(ordinaryErr.Error())))
		g.Expect(errors.Is(err, ordinaryErr)).To(BeTrue())
		g.Expect(result.RequeueAfter).To(BeZero())
		g.Expect(calls).To(Equal([]string{"mixed"}))
	})

	t.Run("plain errors stop after progress requeues", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ordinaryErr := errors.New("deployment failed")
		var calls []string
		r := newApplyTestReconciler(&mockRecorder{}, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "progress")
				return odherrors.NewRequeueAfterError(time.Minute)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "failure")
				return ordinaryErr
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-failure")
				return nil
			},
		}

		result, err := r.apply(t.Context(), newNamedTestObject())

		g.Expect(err).To(MatchError(ContainSubstring(ordinaryErr.Error())))
		g.Expect(result.RequeueAfter).To(BeZero())
		g.Expect(calls).To(Equal([]string{"progress", "failure"}))
	})
}

func TestApply_ActionErrorClassifiers(t *testing.T) {
	t.Parallel()

	t.Run("default ActionError stops actions, marks provisioning false, and honors its delay", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		recorder := &mockRecorder{}
		r := newApplyTestReconciler(recorder, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "terminal")
				return odherrors.NewActionError("dependency unavailable").
					WithRequeueAfter(30 * time.Second)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-terminal")
				return nil
			},
		}
		obj := newNamedTestObject()

		result, err := r.apply(t.Context(), obj)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		g.Expect(calls).To(Equal([]string{"terminal"}))
		condition := conditions.FindStatusCondition(obj, string(DefaultProvisioningConditionType))
		g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		g.Expect(condition.Message).To(ContainSubstring("dependency unavailable"))
		g.Expect(recorder.events).To(HaveLen(1))
		g.Expect(recorder.events[0].reason).To(Equal(eventReasonProvisioningPaused))
	})

	t.Run("failure continues actions, marks provisioning false, and honors its delay", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		recorder := &mockRecorder{}
		r := newApplyTestReconciler(recorder, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "failure")
				return odherrors.NewActionError("resource not ready").
					NonBlocking().WithRequeueAfter(30 * time.Second)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-failure")
				return nil
			},
		}
		obj := newNamedTestObject()

		result, err := r.apply(t.Context(), obj)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		g.Expect(calls).To(Equal([]string{"failure", "after-failure"}))
		condition := conditions.FindStatusCondition(obj, string(DefaultProvisioningConditionType))
		g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		g.Expect(condition.Message).To(ContainSubstring("resource not ready"))
		g.Expect(recorder.events).To(HaveLen(1))
		g.Expect(recorder.events[0].eventType).To(Equal(corev1.EventTypeWarning))
		g.Expect(recorder.events[0].reason).To(Equal(eventReasonProvisioningError))
	})

	t.Run("advisory continues actions, keeps provisioning true, and adds context", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		r := newApplyTestReconciler(&mockRecorder{}, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "advisory")
				return odherrors.NewActionError("waiting for rollout").Advisory().
					WithRequeueAfter(30 * time.Second)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-advisory")
				return nil
			},
		}
		obj := newNamedTestObject()

		result, err := r.apply(t.Context(), obj)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		g.Expect(calls).To(Equal([]string{"advisory", "after-advisory"}))
		condition := conditions.FindStatusCondition(obj, string(DefaultProvisioningConditionType))
		g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		g.Expect(condition.Reason).To(Equal(conditionReasonAdvisory))
		g.Expect(condition.Message).To(ContainSubstring("waiting for rollout"))
	})

	t.Run("terminal without a delay stops actions and returns an error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		recorder := &mockRecorder{}
		r := newApplyTestReconciler(recorder, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "terminal")
				return odherrors.NewActionError("dependency unavailable").Terminal()
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-terminal")
				return nil
			},
		}
		obj := newNamedTestObject()

		result, err := r.apply(t.Context(), obj)

		g.Expect(err).To(MatchError(ContainSubstring("dependency unavailable")))
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(calls).To(Equal([]string{"terminal"}))
		condition := conditions.FindStatusCondition(obj, string(DefaultProvisioningConditionType))
		g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		g.Expect(recorder.events).To(HaveLen(1))
		g.Expect(recorder.events[0].eventType).To(Equal(corev1.EventTypeWarning))
		g.Expect(recorder.events[0].reason).To(Equal(eventReasonProvisioningError))
	})

	t.Run("failure without a delay continues actions and returns an error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		recorder := &mockRecorder{}
		r := newApplyTestReconciler(recorder, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "failure")
				return odherrors.NewActionError("invalid configuration").NonBlocking()
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-failure")
				return nil
			},
		}
		obj := newNamedTestObject()

		result, err := r.apply(t.Context(), obj)

		g.Expect(err).To(MatchError(ContainSubstring("invalid configuration")))
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(calls).To(Equal([]string{"failure", "after-failure"}))
		condition := conditions.FindStatusCondition(obj, string(DefaultProvisioningConditionType))
		g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		g.Expect(recorder.events).To(HaveLen(1))
		g.Expect(recorder.events[0].eventType).To(Equal(corev1.EventTypeWarning))
		g.Expect(recorder.events[0].reason).To(Equal(eventReasonProvisioningError))
	})

	t.Run("advisory without a delay uses the default requeue", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		recorder := &mockRecorder{}
		r := newApplyTestReconciler(recorder, nil)
		r.defaultRequeueAfter = time.Minute
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "advisory")
				return odherrors.NewActionError("rollout pending").Advisory()
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-advisory")
				return nil
			},
		}
		obj := newNamedTestObject()

		result, err := r.apply(t.Context(), obj)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(time.Minute))
		g.Expect(calls).To(Equal([]string{"advisory", "after-advisory"}))
		condition := conditions.FindStatusCondition(obj, string(DefaultProvisioningConditionType))
		g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		g.Expect(condition.Reason).To(Equal(conditionReasonAdvisory))
		g.Expect(recorder.events).To(BeEmpty())
	})

	t.Run("plain errors override advisory delays and mark provisioning false", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		plainErr := errors.New("API request failed")
		recorder := &mockRecorder{}
		r := newApplyTestReconciler(recorder, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				return odherrors.NewActionError("rollout pending").Advisory().
					WithRequeueAfter(time.Minute)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				return plainErr
			},
		}
		obj := newNamedTestObject()

		result, err := r.apply(t.Context(), obj)

		g.Expect(err).To(MatchError(ContainSubstring(plainErr.Error())))
		g.Expect(err).To(MatchError(ContainSubstring("rollout pending")))
		g.Expect(result).To(Equal(ctrl.Result{}))
		condition := conditions.FindStatusCondition(obj, string(DefaultProvisioningConditionType))
		g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		g.Expect(condition.Message).To(ContainSubstring(plainErr.Error()))
		g.Expect(condition.Message).To(ContainSubstring("rollout pending"))
		g.Expect(recorder.events).To(HaveLen(1))
		g.Expect(recorder.events[0].reason).To(Equal(eventReasonProvisioningError))
	})

	t.Run("failures outrank advisories and retain advisory context", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		r := newApplyTestReconciler(&mockRecorder{}, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				return odherrors.NewActionError("rollout pending").Advisory()
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				return odherrors.NewActionError("invalid configuration").NonBlocking()
			},
		}
		obj := newNamedTestObject()

		_, err := r.apply(t.Context(), obj)

		g.Expect(err).To(MatchError(ContainSubstring("rollout pending")))
		g.Expect(err).To(MatchError(ContainSubstring("invalid configuration")))
		condition := conditions.FindStatusCondition(obj, string(DefaultProvisioningConditionType))
		g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		g.Expect(condition.Message).To(ContainSubstring("rollout pending"))
		g.Expect(condition.Message).To(ContainSubstring("invalid configuration"))
	})
}
func TestApply_WrappedActionMarkers(t *testing.T) {
	t.Parallel()

	t.Run("wrapped requeue marker continues actions", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		r := newApplyTestReconciler(&mockRecorder{}, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "requeue")
				return fmt.Errorf("waiting for dependency: %w", odherrors.NewRequeueAfterError(time.Minute))
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-requeue")
				return nil
			},
		}

		result, err := r.apply(t.Context(), newNamedTestObject())

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(time.Minute))
		g.Expect(calls).To(Equal([]string{"requeue", "after-requeue"}))
	})

	t.Run("wrapped stop marker stops actions", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		r := newApplyTestReconciler(&mockRecorder{}, nil)
		r.Actions = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "stop")
				return fmt.Errorf("waiting for dependency: %w", odherrors.NewStopError("not ready"))
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-stop")
				return nil
			},
		}

		_, err := r.apply(t.Context(), newNamedTestObject())

		g.Expect(err).To(MatchError(isStopError, "IsStopError"))
		g.Expect(calls).To(Equal([]string{"stop"}))
	})
}

func TestDelete_ActionErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("terminal finalizer errors override earlier ordinary errors", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ordinaryErr := errors.New("cleanup failed")
		var calls []string
		r := newApplyTestReconciler(&mockRecorder{}, nil)
		r.Actions = nil
		r.Finalizer = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "failure")
				return odherrors.NewActionErrorW(ordinaryErr).NonBlocking()
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-failure")
				return nil
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "stop")
				return odherrors.NewStopError("cleanup pending").WithRequeueAfter(time.Minute)
			},
		}

		obj := newNamedTestObject()
		controllerutil.AddFinalizer(obj, r.finalizerName)
		g.Expect(r.Client.Create(t.Context(), obj)).To(Succeed())

		result, err := r.delete(t.Context(), obj)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(time.Minute))
		g.Expect(calls).To(Equal([]string{"failure", "after-failure", "stop"}))
		g.Expect(controllerutil.ContainsFinalizer(obj, r.finalizerName)).To(BeTrue())
	})

	t.Run("StopError without requeue stops finalizers and preserves the finalizer", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		recorder := &mockRecorder{}
		r := newApplyTestReconciler(recorder, nil)
		r.Actions = nil
		r.Finalizer = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "stop")
				return odherrors.NewStopError("cleanup complete")
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-stop")
				return nil
			},
		}

		obj := newNamedTestObject()
		controllerutil.AddFinalizer(obj, r.finalizerName)
		g.Expect(r.Client.Create(t.Context(), obj)).To(Succeed())

		result, err := r.delete(t.Context(), obj)

		g.Expect(err).To(MatchError(ContainSubstring("cleanup complete")))
		g.Expect(err).To(MatchError(isStopError, "IsStopError"))
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(calls).To(Equal([]string{"stop"}))
		g.Expect(controllerutil.ContainsFinalizer(obj, r.finalizerName)).To(BeTrue())
		g.Expect(recorder.events).To(HaveLen(1))
		g.Expect(recorder.events[0].eventType).To(Equal(corev1.EventTypeWarning))
		g.Expect(recorder.events[0].reason).To(Equal(eventReasonFinalizationError))
	})

	t.Run("StopError with requeue stops finalizers and retains the finalizer", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		recorder := &mockRecorder{}
		r := newApplyTestReconciler(recorder, nil)
		r.Actions = nil
		r.Finalizer = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "stop")
				return odherrors.NewStopError("waiting for cleanup").WithRequeueAfter(30 * time.Second)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-stop")
				return nil
			},
		}

		obj := newNamedTestObject()
		controllerutil.AddFinalizer(obj, r.finalizerName)
		g.Expect(r.Client.Create(t.Context(), obj)).To(Succeed())

		result, err := r.delete(t.Context(), obj)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		g.Expect(calls).To(Equal([]string{"stop"}))
		g.Expect(controllerutil.ContainsFinalizer(obj, r.finalizerName)).To(BeTrue())
		g.Expect(recorder.events).To(HaveLen(1))
		g.Expect(recorder.events[0].eventType).To(Equal(corev1.EventTypeNormal))
		g.Expect(recorder.events[0].reason).To(Equal(eventReasonFinalizationPaused))
	})

	t.Run("RequeueAfterError requeues deletion and retains the finalizer", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		r := newApplyTestReconciler(&mockRecorder{}, nil)
		r.Actions = nil
		r.Finalizer = []actions.Fn{
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "requeue")
				return odherrors.NewRequeueAfterError(time.Minute)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-requeue")
				return nil
			},
		}

		obj := newNamedTestObject()
		controllerutil.AddFinalizer(obj, r.finalizerName)
		g.Expect(r.Client.Create(t.Context(), obj)).To(Succeed())

		result, err := r.delete(t.Context(), obj)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(time.Minute))
		g.Expect(calls).To(Equal([]string{"requeue", "after-requeue"}))
		g.Expect(controllerutil.ContainsFinalizer(obj, r.finalizerName)).To(BeTrue())
	})
}
