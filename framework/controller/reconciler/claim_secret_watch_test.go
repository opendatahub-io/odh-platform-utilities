//nolint:testpackage
package reconciler

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/opendatahub-io/odh-platform-utilities/framework/cluster/gvk"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestClaimSecretWatchByName(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	b := &ReconcilerBuilder[*testPlatformObject]{}
	g.Expect(b.WatchesClaimSecretByName("app", "claim-db", "component")).To(BeIdenticalTo(b))
	g.Expect(b.watches).To(HaveLen(1))
	watch := b.watches[0]
	g.Expect(watch.object.GetObjectKind().GroupVersionKind()).To(Equal(gvk.Secret))
	g.Expect(watch.predicates).To(HaveLen(1))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "claim-db", Namespace: "app"},
		Data:       map[string][]byte{"password": []byte("old")},
	}
	repaired := secret.DeepCopy()
	repaired.Data["password"] = []byte("new")
	changed := event.UpdateEvent{ObjectOld: secret, ObjectNew: repaired}
	g.Expect(watch.predicates[0].Update(changed)).To(BeTrue())

	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer queue.ShutDown()
	watch.eventHandler.Update(context.Background(), changed, queue)
	g.Expect(queue.Len()).To(Equal(1))
	request, shutdown := queue.Get()
	g.Expect(shutdown).To(BeFalse())
	g.Expect(request.NamespacedName).To(Equal(types.NamespacedName{Name: "component"}))
	queue.Done(request)

	g.Expect(watch.predicates[0].Update(event.UpdateEvent{ObjectOld: repaired, ObjectNew: repaired.DeepCopy()})).To(BeFalse())
	g.Expect(watch.predicates[0].Delete(event.DeleteEvent{Object: repaired})).To(BeTrue())
	watch.eventHandler.Delete(context.Background(), event.DeleteEvent{Object: repaired}, queue)
	g.Expect(queue.Len()).To(Equal(1))
	request, shutdown = queue.Get()
	g.Expect(shutdown).To(BeFalse())
	g.Expect(request.NamespacedName).To(Equal(types.NamespacedName{Name: "component"}))
	queue.Done(request)

	g.Expect(watch.predicates[0].Create(event.CreateEvent{Object: repaired})).To(BeTrue())
	watch.eventHandler.Create(context.Background(), event.CreateEvent{Object: repaired}, queue)
	g.Expect(queue.Len()).To(Equal(1))
	request, shutdown = queue.Get()
	g.Expect(shutdown).To(BeFalse())
	g.Expect(request.NamespacedName).To(Equal(types.NamespacedName{Name: "component"}))
	queue.Done(request)

	other := repaired.DeepCopy()
	other.Namespace = "another-app"
	g.Expect(watch.predicates[0].Update(event.UpdateEvent{ObjectOld: secret, ObjectNew: other})).To(BeFalse())
	other.Namespace = "app"
	other.Name = "other-secret"
	g.Expect(watch.predicates[0].Create(event.CreateEvent{Object: other})).To(BeFalse())
}
