package reconciler

import (
	"github.com/opendatahub-io/odh-platform-utilities/framework/cluster/gvk"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/handlers"
	resourcepredicates "github.com/opendatahub-io/odh-platform-utilities/framework/controller/predicates/resources"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// WatchesSecretByName watches a known Secret and enqueues the cluster-scoped
// component CR when it is created, deleted, or its data changes. For a database
// claim, use status.connection.secretRef.name and the claim's namespace.
func (b *ReconcilerBuilder[T]) WatchesSecretByName(namespace, secretName, componentName string) *ReconcilerBuilder[T] {
	return b.WatchesGVK(gvk.Secret,
		WithEventHandler(handlers.ToNamed(componentName)),
		WithPredicates(predicate.And(
			resourcepredicates.CreatedOrUpdatedOrDeletedNamedInNamespace(secretName, namespace),
			resourcepredicates.SecretContentChangedPredicate,
		)),
	)
}
