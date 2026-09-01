//nolint:testpackage
package reconciler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	odherrors "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	. "github.com/onsi/gomega"
)

func newDeleteTestReconciler(recorder *mockRecorder, finalizers ...actions.Fn) *Reconciler {
	r := newApplyTestReconciler(recorder, nil)
	r.Actions = nil
	r.Finalizer = finalizers
	return r
}

func newFinalizerTestObject(t *testing.T, r *Reconciler) *testPlatformObject {
	t.Helper()

	obj := newNamedTestObject()
	controllerutil.AddFinalizer(obj, r.finalizerName)
	if err := r.Client.Create(t.Context(), obj); err != nil {
		t.Fatal(err)
	}

	return obj
}

func TestDelete_RequeueAfterErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("uses the earliest positive delay and retains the finalizer", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		r := newDeleteTestReconciler(&mockRecorder{},
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
		)
		obj := newFinalizerTestObject(t, r)

		result, err := r.delete(t.Context(), obj)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(15 * time.Second))
		g.Expect(calls).To(Equal([]string{"first", "second", "third"}))
		g.Expect(controllerutil.ContainsFinalizer(obj, r.finalizerName)).To(BeTrue())
	})

	t.Run("ignores non-positive delays and removes the finalizer", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		r := newDeleteTestReconciler(&mockRecorder{},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "zero")
				return odherrors.NewRequeueAfterError(0)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "negative")
				return odherrors.NewRequeueAfterError(-time.Second)
			},
		)
		obj := newFinalizerTestObject(t, r)

		result, err := r.delete(t.Context(), obj)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(BeZero())
		g.Expect(calls).To(Equal([]string{"zero", "negative"}))
		g.Expect(controllerutil.ContainsFinalizer(obj, r.finalizerName)).To(BeFalse())
	})

	t.Run("ignores a legacy marker when a terminal error follows", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		r := newDeleteTestReconciler(&mockRecorder{},
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
		)
		obj := newFinalizerTestObject(t, r)

		result, err := r.delete(t.Context(), obj)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(time.Minute))
		g.Expect(calls).To(Equal([]string{"requeue", "stop"}))
		g.Expect(controllerutil.ContainsFinalizer(obj, r.finalizerName)).To(BeTrue())
	})
}

func TestDelete_PlainErrorStopsAfterRequeueAfter(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	ordinaryErr := errors.New("cleanup failed")
	var calls []string
	recorder := &mockRecorder{}
	r := newDeleteTestReconciler(recorder,
		func(_ context.Context, _ *types.ReconciliationRequest) error {
			calls = append(calls, "requeue")
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
	)
	obj := newFinalizerTestObject(t, r)

	result, err := r.delete(t.Context(), obj)

	g.Expect(err).To(MatchError(ContainSubstring(ordinaryErr.Error())))
	g.Expect(errors.Is(err, ordinaryErr)).To(BeTrue())
	g.Expect(result.RequeueAfter).To(BeZero())
	g.Expect(calls).To(Equal([]string{"requeue", "failure"}))
	g.Expect(controllerutil.ContainsFinalizer(obj, r.finalizerName)).To(BeTrue())
	g.Expect(recorder.events).To(HaveLen(1))
	g.Expect(recorder.events[0].eventType).To(Equal(corev1.EventTypeWarning))
	g.Expect(recorder.events[0].reason).To(Equal(eventReasonFinalizationError))
}

func TestDelete_ActionErrorClassifiers(t *testing.T) {
	t.Parallel()

	t.Run("non-blocking error with a delay retains the finalizer and schedules requeue", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		recorder := &mockRecorder{}
		r := newDeleteTestReconciler(recorder,
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "failure")
				return odherrors.NewActionError("cleanup incomplete").
					NonBlocking().WithRequeueAfter(30 * time.Second)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-failure")
				return nil
			},
		)
		obj := newFinalizerTestObject(t, r)

		result, err := r.delete(t.Context(), obj)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		g.Expect(calls).To(Equal([]string{"failure", "after-failure"}))
		g.Expect(controllerutil.ContainsFinalizer(obj, r.finalizerName)).To(BeTrue())
		g.Expect(recorder.events).To(HaveLen(1))
		g.Expect(recorder.events[0].eventType).To(Equal(corev1.EventTypeWarning))
		g.Expect(recorder.events[0].reason).To(Equal(eventReasonFinalizationError))
	})

	t.Run("non-blocking error without a delay retains the finalizer and returns an error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		recorder := &mockRecorder{}
		r := newDeleteTestReconciler(recorder,
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				return odherrors.NewActionError("cleanup failed").NonBlocking()
			},
		)
		obj := newFinalizerTestObject(t, r)

		result, err := r.delete(t.Context(), obj)

		g.Expect(err).To(MatchError(ContainSubstring("cleanup failed")))
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(controllerutil.ContainsFinalizer(obj, r.finalizerName)).To(BeTrue())
		g.Expect(recorder.events).To(HaveLen(1))
		g.Expect(recorder.events[0].eventType).To(Equal(corev1.EventTypeWarning))
		g.Expect(recorder.events[0].reason).To(Equal(eventReasonFinalizationError))
	})

	t.Run("default ActionError stops finalizers and returns an error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		recorder := &mockRecorder{}
		r := newDeleteTestReconciler(recorder,
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "terminal")
				return odherrors.NewActionError("cleanup blocked")
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-terminal")
				return nil
			},
		)
		obj := newFinalizerTestObject(t, r)

		result, err := r.delete(t.Context(), obj)

		g.Expect(err).To(MatchError(ContainSubstring("cleanup blocked")))
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(calls).To(Equal([]string{"terminal"}))
		g.Expect(controllerutil.ContainsFinalizer(obj, r.finalizerName)).To(BeTrue())
		g.Expect(recorder.events).To(HaveLen(1))
		g.Expect(recorder.events[0].eventType).To(Equal(corev1.EventTypeWarning))
		g.Expect(recorder.events[0].reason).To(Equal(eventReasonFinalizationError))
	})

	t.Run("advisory without a delay permits finalizer removal", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		r := newDeleteTestReconciler(&mockRecorder{},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				return odherrors.NewActionError("cleanup note").Advisory()
			},
		)
		obj := newFinalizerTestObject(t, r)

		result, err := r.delete(t.Context(), obj)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(controllerutil.ContainsFinalizer(obj, r.finalizerName)).To(BeFalse())
	})

	t.Run("advisory with a delay continues finalizers and retains the finalizer", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		var calls []string
		r := newDeleteTestReconciler(&mockRecorder{},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "advisory")
				return odherrors.NewActionError("cleanup still running").Advisory().
					WithRequeueAfter(30 * time.Second)
			},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				calls = append(calls, "after-advisory")
				return nil
			},
		)
		obj := newFinalizerTestObject(t, r)

		result, err := r.delete(t.Context(), obj)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		g.Expect(calls).To(Equal([]string{"advisory", "after-advisory"}))
		g.Expect(controllerutil.ContainsFinalizer(obj, r.finalizerName)).To(BeTrue())
	})
}
