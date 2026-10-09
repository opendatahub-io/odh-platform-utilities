//nolint:testpackage
package reconciler

import (
	"context"
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	ctrlrecorder "sigs.k8s.io/controller-runtime/pkg/recorder"

	. "github.com/onsi/gomega"
)

type fakeSource struct{}

func TestDependentConditions(t *testing.T) {
	type conditionName string

	got := DependentConditions(conditionName("ModulesReady"), conditionName("ProvisioningProgress"))
	want := []conditions.DependentDefinition{
		{Type: api.ConditionType("ModulesReady"), Polarity: conditions.HealthyWhenTrue},
		{Type: api.ConditionType("ProvisioningProgress"), Polarity: conditions.HealthyWhenTrue},
	}

	NewWithT(t).Expect(got).To(Equal(want))
}

type testManager struct {
	ctrlmanager.Manager

	client   client.Client
	scheme   *runtime.Scheme
	recorder ctrlrecorder.EventRecorder
	config   *rest.Config
}

func mustNewAggregator(
	dependents ...conditions.DependentDefinition,
) *conditions.Aggregator {
	aggregator, err := conditions.NewAggregator(api.ConditionTypeReady, dependents...)
	if err != nil {
		panic(err)
	}

	return aggregator
}

func (m *testManager) GetClient() client.Client {
	return m.client
}

func (m *testManager) GetScheme() *runtime.Scheme {
	return m.scheme
}

func (m *testManager) GetEventRecorder(string) ctrlrecorder.EventRecorder {
	return m.recorder
}

func (m *testManager) GetConfig() *rest.Config {
	return m.config
}

func newTestManager() *testManager {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)

	return &testManager{
		client: fake.NewClientBuilder().
			WithScheme(s).
			Build(),
		scheme:   s,
		recorder: events.NewFakeRecorder(10),
		config: &rest.Config{
			Host: "https://127.0.0.1",
		},
	}
}

func (fakeSource) Start(context.Context, workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
	return nil
}

func TestReconcilerBuilder_WatchesRawSource(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	b := &ReconcilerBuilder[*testPlatformObject]{}

	src := fakeSource{}
	result := b.WatchesRawSource(src)

	g.Expect(result).To(BeIdenticalTo(b))
	g.Expect(b.rawSources).To(ConsistOf(src))
}
