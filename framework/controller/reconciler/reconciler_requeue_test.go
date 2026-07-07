//nolint:testpackage
package reconciler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	. "github.com/onsi/gomega"
)

type requeueTestInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Status api.Status `json:"status"`
}

func (f *requeueTestInstance) GetStatus() *api.Status {
	return &f.Status
}

func (f *requeueTestInstance) GetConditions() []api.Condition {
	return f.Status.Conditions
}

func (f *requeueTestInstance) SetConditions(c []api.Condition) {
	f.Status.Conditions = c
}

func (f *requeueTestInstance) DeepCopyObject() runtime.Object {
	o := *f
	return &o
}

func newRequeueTestReconciler(t *testing.T, defaultRequeueAfter time.Duration, action actions.Fn) (*Reconciler, api.PlatformObject) {
	t.Helper()

	gvk := schema.GroupVersionKind{Group: "test", Version: "v1", Kind: "Fake"}

	instance := &requeueTestInstance{
		TypeMeta:   metav1.TypeMeta{APIVersion: "test/v1", Kind: "Fake"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-instance", UID: k8stypes.UID("uid-1234"), Generation: 1},
	}

	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(gvk, &requeueTestInstance{})

	err := scheme.AddConversionFunc(&requeueTestInstance{}, &requeueTestInstance{}, func(a, b any, _ conversion.Scope) error {
		in, ok := a.(*requeueTestInstance)
		if !ok {
			return fmt.Errorf("unexpected source type %T", a)
		}

		out, ok := b.(*requeueTestInstance)
		if !ok {
			return fmt.Errorf("unexpected destination type %T", b)
		}

		*out = *in

		return nil
	})
	if err != nil {
		t.Fatalf("failed to register conversion func: %v", err)
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceApply: func(context.Context, client.Client, string, runtime.ApplyConfiguration, ...client.SubResourceApplyOption) error {
				return nil
			},
		}).
		Build()

	r := &Reconciler{
		Client:                    cli,
		Recorder:                  events.NewFakeRecorder(10),
		name:                      "test",
		provisioningConditionType: DefaultProvisioningConditionType,
		phaseReady:                DefaultPhaseReady,
		phaseNotReady:             DefaultPhaseNotReady,
		preApplyFailedReason:      "PreConditionFailed",
		conditionsManagerFactory: func(accessor api.ConditionsAccessor) *conditions.Manager {
			return conditions.NewManager(accessor, DefaultHappyCondition)
		},
		defaultRequeueAfter: defaultRequeueAfter,
	}

	if action != nil {
		r.AddAction(action)
	}

	return r, instance
}

func TestApply_DefaultRequeueAfter(t *testing.T) {
	t.Parallel()

	const defaultRequeue = 5 * time.Minute

	tests := []struct {
		name                 string
		defaultRequeueAfter  time.Duration
		action               actions.Fn
		expectedRequeueAfter time.Duration
	}{
		{
			name:                 "falls back to default on plain success",
			defaultRequeueAfter:  defaultRequeue,
			action:               nil,
			expectedRequeueAfter: defaultRequeue,
		},
		{
			name:                "action-requested requeue wins over default",
			defaultRequeueAfter: defaultRequeue,
			action: func(_ context.Context, _ *types.ReconciliationRequest) error {
				return errors.NewRequeueAfterError(30 * time.Second)
			},
			expectedRequeueAfter: 30 * time.Second,
		},
		{
			name:                 "no default configured means no requeue",
			defaultRequeueAfter:  0,
			action:               nil,
			expectedRequeueAfter: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			r, instance := newRequeueTestReconciler(t, tc.defaultRequeueAfter, tc.action)

			requeueAfter, err := r.apply(context.Background(), instance)

			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(requeueAfter).To(Equal(tc.expectedRequeueAfter))
		})
	}
}
