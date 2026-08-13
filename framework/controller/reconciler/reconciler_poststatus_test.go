//nolint:testpackage
package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/onsi/gomega"
)

type conditionAwarePlatformObject struct {
	unstructured.Unstructured

	status api.Status
}

func (o *conditionAwarePlatformObject) GetStatus() *api.Status {
	return &o.status
}

func (o *conditionAwarePlatformObject) GetConditions() []api.Condition {
	return o.status.Conditions
}

func (o *conditionAwarePlatformObject) SetConditions(conds []api.Condition) {
	o.status.Conditions = conds
}

func newConditionAwareObject() *conditionAwarePlatformObject {
	obj := &conditionAwarePlatformObject{}
	obj.SetGroupVersionKind(testGVKConfigMap)
	obj.SetName("test-obj")
	obj.SetNamespace("default")
	return obj
}

func newPostStatusTestReconciler(
	recorder *mockRecorder,
	actionFn func(context.Context, *types.ReconciliationRequest) error,
	postStatusFn PostStatusFn,
) *Reconciler {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)

	cli := fake.NewClientBuilder().
		WithScheme(s).
		Build()

	return &Reconciler{
		Client:                    cli,
		Scheme:                    s,
		Recorder:                  recorder,
		name:                      "test-reconciler",
		provisioningConditionType: DefaultProvisioningConditionType,
		phaseReady:                DefaultPhaseReady,
		phaseNotReady:             DefaultPhaseNotReady,
		skipConditionCleanup:      true,
		postStatusFn:              postStatusFn,
		Actions: []actions.Fn{
			actionFn,
		},
		conditionsAggregator: mustNewAggregator(
			DefaultHappyCondition,
			conditions.Dependent(DefaultProvisioningConditionType, conditions.HealthyWhenTrue),
		),
		gvks:                        make(map[schema.GroupVersionKind]gvkInfo),
		excludeFromDynamicOwnership: make(map[schema.GroupVersionKind]struct{}),
		instanceFactory: func() (api.PlatformObject, error) {
			return newConditionAwareObject(), nil
		},
	}
}

func TestPostStatusFn(t *testing.T) {
	t.Parallel()

	t.Run("hook called with isHappy=true when all conditions met", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ctx := t.Context()

		var capturedHappy bool
		r := newPostStatusTestReconciler(
			&mockRecorder{},
			func(_ context.Context, _ *types.ReconciliationRequest) error { return nil },
			func(_ context.Context, _ *types.ReconciliationRequest, isHappy bool) error {
				capturedHappy = isHappy
				return nil
			},
		)

		obj := newConditionAwareObject()
		_, err := r.apply(ctx, obj)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(capturedHappy).To(BeTrue())
		g.Expect(obj.GetStatus().Phase).To(Equal(DefaultPhaseReady))
	})

	t.Run("hook called with isHappy=false when provisioning fails", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ctx := t.Context()

		var capturedHappy bool
		r := newPostStatusTestReconciler(
			&mockRecorder{},
			func(_ context.Context, _ *types.ReconciliationRequest) error {
				return errors.New("action failed")
			},
			func(_ context.Context, _ *types.ReconciliationRequest, isHappy bool) error {
				capturedHappy = isHappy
				return nil
			},
		)

		obj := newConditionAwareObject()
		_, err := r.apply(ctx, obj)
		g.Expect(err).Should(MatchError(ContainSubstring("provisioning failed")))
		g.Expect(capturedHappy).To(BeFalse())
	})

	t.Run("hook mutations are visible on instance", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ctx := t.Context()

		r := newPostStatusTestReconciler(
			&mockRecorder{},
			func(_ context.Context, _ *types.ReconciliationRequest) error { return nil },
			func(_ context.Context, rr *types.ReconciliationRequest, isHappy bool) error {
				if isHappy {
					rr.Instance.GetStatus().Phase = "CustomPhase"
				}
				return nil
			},
		)

		obj := newConditionAwareObject()
		_, err := r.apply(ctx, obj)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(obj.GetStatus().Phase).To(Equal("CustomPhase"))
	})

	t.Run("hook error is propagated", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ctx := t.Context()

		r := newPostStatusTestReconciler(
			&mockRecorder{},
			func(_ context.Context, _ *types.ReconciliationRequest) error { return nil },
			func(_ context.Context, _ *types.ReconciliationRequest, _ bool) error {
				return errors.New("hook broke")
			},
		)

		obj := newConditionAwareObject()
		_, err := r.apply(ctx, obj)
		g.Expect(err).Should(MatchError(ContainSubstring("post status hook: hook broke")))
	})

	t.Run("skipStatusConditionsFn clears phase set by hook", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ctx := t.Context()

		r := newPostStatusTestReconciler(
			&mockRecorder{},
			func(_ context.Context, _ *types.ReconciliationRequest) error { return nil },
			func(_ context.Context, rr *types.ReconciliationRequest, _ bool) error {
				rr.Instance.GetStatus().Phase = "CustomPhase"
				return nil
			},
		)
		r.skipStatusConditionsFn = func() bool { return true }

		obj := newConditionAwareObject()
		_, err := r.apply(ctx, obj)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(obj.GetStatus().Phase).To(Equal(""))
	})

	t.Run("nil hook does not panic", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ctx := t.Context()

		r := newPostStatusTestReconciler(
			&mockRecorder{},
			func(_ context.Context, _ *types.ReconciliationRequest) error { return nil },
			nil,
		)

		obj := newConditionAwareObject()
		_, err := r.apply(ctx, obj)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(obj.GetStatus().Phase).To(Equal(DefaultPhaseReady))
	})
}
