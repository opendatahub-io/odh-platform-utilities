//nolint:testpackage
package reconciler

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type deletionTestInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Status api.Status `json:"status"`
}

func (d *deletionTestInstance) GetStatus() *api.Status {
	return &d.Status
}

func (d *deletionTestInstance) GetConditions() []api.Condition {
	return d.Status.Conditions
}

func (d *deletionTestInstance) SetConditions(c []api.Condition) {
	d.Status.Conditions = c
}
func (d *deletionTestInstance) DeepCopyObject() runtime.Object {
	cp := *d
	cp.Finalizers = append([]string(nil), d.Finalizers...)
	if d.DeletionTimestamp != nil {
		ts := *d.DeletionTimestamp
		cp.DeletionTimestamp = &ts
	}
	return &cp
}

func deletionTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	s.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "test", Version: "v1", Kind: "Fake"},
		&deletionTestInstance{},
	)
	s.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "test", Version: "v1", Kind: "FakeList"},
		&unstructured.UnstructuredList{},
	)
	metav1.AddToGroupVersion(s, schema.GroupVersion{Group: "test", Version: "v1"})
	return s
}

// TestReconcile_DeletionViaAPIReader verifies that the reconciler reads from the
// API server (apiReader), not the informer cache (Client). The cached client
// returns the object WITHOUT deletionTimestamp (stale cache), while the API
// reader returns it WITH deletionTimestamp. The delete path must run.
func TestReconcile_DeletionViaAPIReader(t *testing.T) {
	g := NewWithT(t)

	const finalizerName = "test.io/cleanup"
	now := metav1.Now()

	obj := &deletionTestInstance{
		TypeMeta: metav1.TypeMeta{APIVersion: "test/v1", Kind: "Fake"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-instance",
			UID:               k8stypes.UID("uid-1234"),
			Generation:        1,
			Finalizers:        []string{finalizerName},
			DeletionTimestamp: &now,
		},
	}

	scheme := deletionTestScheme()

	// Backing client has the real object (with deletionTimestamp + finalizer).
	// Used as the apiReader and for Update (finalizer removal).
	backingClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obj).
		WithStatusSubresource(obj).
		Build()

	// Cached client wraps the backing client but strips deletionTimestamp
	// on Get, simulating a stale informer cache.
	cachedClient := interceptor.NewClient(backingClient, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if err := c.Get(ctx, key, obj, opts...); err != nil {
				return err
			}
			if m, ok := obj.(metav1.Object); ok {
				m.SetDeletionTimestamp(nil)
			}
			return nil
		},
	})

	deleteCalled := false
	applyCalled := false

	r := &Reconciler{
		Client:    cachedClient,
		apiReader: backingClient,
		Scheme:    scheme,
		Log:       ctrl.Log.WithName("test"),
		Recorder:  &events.FakeRecorder{},

		name:                      "test",
		finalizerName:             finalizerName,
		provisioningConditionType: DefaultProvisioningConditionType,
		phaseReady:                DefaultPhaseReady,
		phaseNotReady:             DefaultPhaseNotReady,
		instanceFactory: func() (api.PlatformObject, error) {
			return &deletionTestInstance{
				TypeMeta: metav1.TypeMeta{APIVersion: "test/v1", Kind: "Fake"},
			}, nil
		},
		conditionsManagerFactory: func(accessor api.ConditionsAccessor) *conditions.Manager {
			return conditions.NewManager(accessor, DefaultHappyCondition)
		},
		gvks:                        make(map[schema.GroupVersionKind]gvkInfo),
		excludeFromDynamicOwnership: make(map[schema.GroupVersionKind]struct{}),
	}

	r.Finalizer = []actions.Fn{
		func(_ context.Context, _ *types.ReconciliationRequest) error {
			deleteCalled = true
			return nil
		},
	}
	r.Actions = []actions.Fn{
		func(_ context.Context, _ *types.ReconciliationRequest) error {
			applyCalled = true
			return nil
		},
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "test-instance"},
	})

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(deleteCalled).To(BeTrue(), "delete path should run when API server has deletionTimestamp")
	g.Expect(applyCalled).To(BeFalse(), "apply path should not run when object is being deleted")
}
