package gc

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
)

const (
	kubernetesRBACGroup         = "rbac.authorization.k8s.io"
	openShiftAuthorizationGroup = "authorization.openshift.io"

	kindRole               = "Role"
	kindRoleBinding        = "RoleBinding"
	kindClusterRole        = "ClusterRole"
	kindClusterRoleBinding = "ClusterRoleBinding"
)

// openShiftRBACAliasKinds: kinds shared by rbac.authorization.k8s.io and
// authorization.openshift.io (same etcd object). RoleBindingRestriction is not an alias.
//
//nolint:gochecknoglobals // Immutable kind set.
var openShiftRBACAliasKinds = map[string]struct{}{
	kindRole:               {},
	kindRoleBinding:        {},
	kindClusterRole:        {},
	kindClusterRoleBinding: {},
}

func isOpenShiftRBACAlias(gvk schema.GroupVersionKind) bool {
	if gvk.Group != openShiftAuthorizationGroup {
		return false
	}

	_, ok := openShiftRBACAliasKinds[gvk.Kind]

	return ok
}

func isKubernetesRBACAlias(gvk schema.GroupVersionKind) bool {
	if gvk.Group != kubernetesRBACGroup {
		return false
	}

	_, ok := openShiftRBACAliasKinds[gvk.Kind]

	return ok
}

func aliasKindsInGroup(items []resources.Resource, group string) map[string]struct{} {
	kinds := make(map[string]struct{})

	for i := range items {
		gvk := items[i].GroupVersionKind()
		if gvk.Group != group {
			continue
		}

		if _, ok := openShiftRBACAliasKinds[gvk.Kind]; ok {
			kinds[gvk.Kind] = struct{}{}
		}
	}

	return kinds
}

// preferKubernetesRBACResources picks one API view per aliased RBAC kind:
//   - Kubernetes deletable+listable → drop OpenShift alias
//   - Kubernetes deletable but not listable, OpenShift deletable+listable →
//     drop Kubernetes (avoid Forbidden list noise) and keep OpenShift
//   - OpenShift-only kinds are never removed
//
//nolint:cyclop // Preference matrix needs the branch set above; splitting obscures it.
func preferKubernetesRBACResources(deletable, listable []resources.Resource) []resources.Resource {
	deletableKubernetes := aliasKindsInGroup(deletable, kubernetesRBACGroup)
	if len(deletableKubernetes) == 0 {
		return deletable
	}

	listableKubernetes := aliasKindsInGroup(listable, kubernetesRBACGroup)
	deletableOpenShift := aliasKindsInGroup(deletable, openShiftAuthorizationGroup)
	listableOpenShift := aliasKindsInGroup(listable, openShiftAuthorizationGroup)

	preferKubernetes := make(map[string]struct{})
	preferOpenShift := make(map[string]struct{})

	for kind := range deletableKubernetes {
		if _, ok := listableKubernetes[kind]; ok {
			preferKubernetes[kind] = struct{}{}
			continue
		}

		_, openShiftDeletable := deletableOpenShift[kind]
		_, openShiftListable := listableOpenShift[kind]

		if openShiftDeletable && openShiftListable {
			preferOpenShift[kind] = struct{}{}
		}
	}

	if len(preferKubernetes) == 0 && len(preferOpenShift) == 0 {
		return deletable
	}

	filtered := make([]resources.Resource, 0, len(deletable))

	for i := range deletable {
		gvk := deletable[i].GroupVersionKind()
		if isOpenShiftRBACAlias(gvk) {
			if _, drop := preferKubernetes[gvk.Kind]; drop {
				continue
			}
		}

		if isKubernetesRBACAlias(gvk) {
			if _, drop := preferOpenShift[gvk.Kind]; drop {
				continue
			}
		}

		filtered = append(filtered, deletable[i])
	}

	return filtered
}
