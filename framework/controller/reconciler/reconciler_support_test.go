//nolint:testpackage
package reconciler

import (
	"context"
	"testing"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/gomega"
)

type fakeSource struct{}

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
