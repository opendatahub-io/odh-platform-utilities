//nolint:testpackage
package reconciler

import (
	"context"
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/gomega"
)

type fakeSource struct{}

func mustNewAggregator(
	target api.ConditionType,
	dependents ...conditions.DependentDefinition,
) *conditions.Aggregator {
	aggregator, err := conditions.NewAggregator(target, dependents...)
	if err != nil {
		panic(err)
	}

	return aggregator
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
