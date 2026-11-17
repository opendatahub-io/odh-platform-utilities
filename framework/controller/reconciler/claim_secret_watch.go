package reconciler

import (
	"github.com/opendatahub-io/odh-platform-utilities/framework/cluster/gvk"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/handlers"
	resourcepredicates "github.com/opendatahub-io/odh-platform-utilities/framework/controller/predicates/resources"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// WatchesClaimSecretByName watches a known claim Secret and enqueues the
// cluster-scoped component CR. It also catches a Secret recreated without any
// consumer-owned label or annotation.
func (b *ReconcilerBuilder[T]) WatchesClaimSecretByName(namespace, secretName, componentName string) *ReconcilerBuilder[T] {
	return b.WatchesGVK(gvk.Secret,
		WithEventHandler(handlers.ToNamed(componentName)),
		WithPredicates(predicate.And(
			resourcepredicates.CreatedOrUpdatedOrDeletedNamedInNamespace(secretName, namespace),
			resourcepredicates.SecretContentChangedPredicate,
		)),
	)
}
