package tls_test

import (
	"context"
	"sync/atomic"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	pkgtls "github.com/opendatahub-io/odh-platform-utilities/pkg/tls"
)

func TestSecurityProfileWatcher_Reconcile(t *testing.T) { //nolint:funlen // Watcher no-op vs change vs missing CR.
	t.Parallel()

	scheme := newTLSScheme(t)
	ctx := context.Background()
	intermediate := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	modern := *configv1.TLSProfiles[configv1.TLSProfileModernType]
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: cluster.ClusterAPIServerObj}}

	t.Run("no callback when profile is unchanged", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType})
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		var called atomic.Bool

		watcher := &pkgtls.SecurityProfileWatcher{
			Client:                cli,
			InitialTLSProfileSpec: intermediate,
			OnProfileChange: func(context.Context, configv1.TLSProfileSpec, configv1.TLSProfileSpec) {
				called.Store(true)
			},
		}

		_, err := watcher.Reconcile(ctx, req)
		require.NoError(t, err)
		assert.False(t, called.Load())
		assert.Equal(t, intermediate.MinTLSVersion, watcher.InitialTLSProfileSpec.MinTLSVersion)
	})

	t.Run("callback when profile changes", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType})
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		var called atomic.Bool

		var gotOld, gotNew configv1.TLSProfileSpec

		watcher := &pkgtls.SecurityProfileWatcher{
			Client:                cli,
			InitialTLSProfileSpec: intermediate,
			OnProfileChange: func(_ context.Context, oldSpec, newSpec configv1.TLSProfileSpec) {
				called.Store(true)

				gotOld = oldSpec
				gotNew = newSpec
			},
		}

		_, err := watcher.Reconcile(ctx, req)
		require.NoError(t, err)
		assert.True(t, called.Load())
		assert.Equal(t, intermediate.MinTLSVersion, gotOld.MinTLSVersion)
		assert.Equal(t, modern.MinTLSVersion, gotNew.MinTLSVersion)
		assert.Equal(t, modern.MinTLSVersion, watcher.InitialTLSProfileSpec.MinTLSVersion)
	})

	t.Run("missing APIServer is a no-op", func(t *testing.T) {
		t.Parallel()

		cli := fake.NewClientBuilder().WithScheme(scheme).Build()

		var called atomic.Bool

		watcher := &pkgtls.SecurityProfileWatcher{
			Client:                cli,
			InitialTLSProfileSpec: intermediate,
			OnProfileChange: func(context.Context, configv1.TLSProfileSpec, configv1.TLSProfileSpec) {
				called.Store(true)
			},
		}

		_, err := watcher.Reconcile(ctx, req)
		require.NoError(t, err)
		assert.False(t, called.Load())
	})
}
