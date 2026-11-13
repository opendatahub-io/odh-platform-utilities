package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestPreserveDeploymentUserFields covers the fragment construction the
// patch-mode release uses to keep the Deployment user-tuned fields alive
// (see preserveDeploymentUserFields): skeletons are created only for the
// containers both the desired state carries and that exist live,
// MergeDeployments fills them with the live resources and the live replica
// count, and containers the desired state dropped or added are left out of
// the fragment entirely. The release-body path through the migration is
// covered end to end by the patch-mode migration tests; this test pins the
// helper's own contract.
func TestPreserveDeploymentUserFields(t *testing.T) {
	t.Parallel()

	newProbe := func(kind string, containers any, replicas any) *unstructured.Unstructured {
		u := &unstructured.Unstructured{}
		u.SetAPIVersion("apps/v1")
		u.SetKind(kind)
		u.SetName("probe")
		u.SetNamespace("test")

		spec := map[string]any{
			"template": map[string]any{
				"spec": map[string]any{"containers": containers},
			},
		}
		if replicas != nil {
			spec["replicas"] = replicas
		}
		u.Object["spec"] = spec

		return u
	}

	newFragment := func() *unstructured.Unstructured {
		f := &unstructured.Unstructured{}
		f.SetAPIVersion("apps/v1")
		f.SetKind("Deployment")
		f.SetName("probe")
		f.SetNamespace("test")
		return f
	}

	container := func(name string, resources any) map[string]any {
		c := map[string]any{"name": name}
		if resources != nil {
			c["resources"] = resources
		}
		return c
	}

	resources := map[string]any{"requests": map[string]any{"cpu": "500m"}}

	fragmentContainers := func(t *testing.T, fragment *unstructured.Unstructured) []any {
		t.Helper()
		containers, found, err := unstructured.NestedSlice(fragment.Object, "spec", "template", "spec", "containers")
		require.NoError(t, err)
		if !found {
			return nil
		}
		return containers
	}

	t.Run("preserves the live replicas and resources of carried containers", func(t *testing.T) {
		t.Parallel()

		desired := newProbe("Deployment", []any{container("manager", nil)}, nil)
		live := newProbe("Deployment", []any{container("manager", resources)}, int64(3))
		fragment := newFragment()

		require.NoError(t, preserveDeploymentUserFields(desired, live, fragment))

		replicas, found, err := unstructured.NestedInt64(fragment.Object, "spec", "replicas")
		require.NoError(t, err)
		require.True(t, found, "the fragment must carry the live replica count")
		assert.Equal(t, int64(3), replicas)

		containers := fragmentContainers(t, fragment)
		require.Len(t, containers, 1)
		assert.Equal(t, map[string]any{"name": "manager", "resources": resources}, containers[0])
	})

	t.Run("excludes containers the desired state adds", func(t *testing.T) {
		t.Parallel()

		desired := newProbe("Deployment", []any{container("manager", nil), container("sidecar", nil)}, nil)
		live := newProbe("Deployment", []any{container("manager", resources)}, nil)
		fragment := newFragment()

		require.NoError(t, preserveDeploymentUserFields(desired, live, fragment))

		containers := fragmentContainers(t, fragment)
		require.Len(t, containers, 1, "a container the desired state adds has no live counterpart and must not be claimed")
		name, ok := containers[0].(map[string]any)["name"].(string)
		require.True(t, ok)
		assert.Equal(t, "manager", name)
	})

	t.Run("excludes containers the desired state drops", func(t *testing.T) {
		t.Parallel()

		desired := newProbe("Deployment", []any{container("manager", nil)}, nil)
		live := newProbe("Deployment", []any{container("manager", resources), container("old-sidecar", resources)}, nil)
		fragment := newFragment()

		require.NoError(t, preserveDeploymentUserFields(desired, live, fragment))

		containers := fragmentContainers(t, fragment)
		require.Len(t, containers, 1, "a container the desired state drops must not be retained by the fragment")
		name, ok := containers[0].(map[string]any)["name"].(string)
		require.True(t, ok)
		assert.Equal(t, "manager", name)
	})

	t.Run("carries only the replica count when no container survives", func(t *testing.T) {
		t.Parallel()

		desired := newProbe("Deployment", []any{container("sidecar", nil)}, nil)
		live := newProbe("Deployment", []any{container("manager", resources)}, int64(2))
		fragment := newFragment()

		require.NoError(t, preserveDeploymentUserFields(desired, live, fragment))

		assert.Empty(t, fragmentContainers(t, fragment), "no container is carried by both states; the fragment must not claim any")

		replicas, found, err := unstructured.NestedInt64(fragment.Object, "spec", "replicas")
		require.NoError(t, err)
		require.True(t, found, "the live replica count is preserved regardless of the containers")
		assert.Equal(t, int64(2), replicas)
	})

	t.Run("reduces carried skeletons to their merge key when live has no resources", func(t *testing.T) {
		t.Parallel()

		desired := newProbe("Deployment", []any{container("manager", resources)}, nil)
		live := newProbe("Deployment", []any{container("manager", nil)}, nil)
		fragment := newFragment()

		require.NoError(t, preserveDeploymentUserFields(desired, live, fragment))

		containers := fragmentContainers(t, fragment)
		require.Len(t, containers, 1)
		assert.Equal(t, map[string]any{"name": "manager"}, containers[0], "an absent live resources map must not carry desired resources into the fragment")
	})

	t.Run("fails loudly on malformed containers", func(t *testing.T) {
		t.Parallel()

		desired := newProbe("Deployment", "not-a-slice", nil)
		live := newProbe("Deployment", []any{container("manager", nil)}, nil)
		fragment := newFragment()

		require.ErrorContains(t, preserveDeploymentUserFields(desired, live, fragment), "failed to read the containers")
	})
}
