package tls_test

import (
	"context"
	"errors"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	pkgtls "github.com/opendatahub-io/odh-platform-utilities/pkg/tls"
)

var errDenied = errors.New("denied")

func newTLSScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()
	require.NoError(t, configv1.Install(s))

	return s
}

func newClusterAPIServer(profile *configv1.TLSSecurityProfile) *configv1.APIServer {
	return &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: cluster.ClusterAPIServerObj},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: profile,
		},
	}
}

type erroringClient struct {
	client.Client

	getErr error
}

func (c *erroringClient) Get(
	ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption,
) error {
	if c.getErr != nil && key.Name == cluster.ClusterAPIServerObj {
		return c.getErr
	}

	return c.Client.Get(ctx, key, obj, opts...)
}

func TestFromAPIServer(t *testing.T) { //nolint:funlen // Table-driven APIServer fetch cases.
	t.Parallel()

	scheme := newTLSScheme(t)
	ctx := context.Background()

	t.Run("missing APIServer uses intermediate defaults", func(t *testing.T) {
		t.Parallel()

		cli := fake.NewClientBuilder().WithScheme(scheme).Build()

		minVersion, cipherSuites, err := pkgtls.FromAPIServer(ctx, cli, pkgtls.FormatShort)
		require.NoError(t, err)
		assert.Equal(t, "TLS1.2", minVersion)
		assert.Equal(t, intermediateIANACiphers, cipherSuites)
	})

	t.Run("NoMatch uses intermediate defaults", func(t *testing.T) {
		t.Parallel()

		cli := &erroringClient{
			Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
			getErr: &meta.NoKindMatchError{
				GroupKind: schema.GroupKind{Group: "config.openshift.io", Kind: "APIServer"},
			},
		}

		minVersion, cipherSuites, err := pkgtls.FromAPIServer(ctx, cli, pkgtls.FormatGo)
		require.NoError(t, err)
		assert.Equal(t, "VersionTLS12", minVersion)
		assert.Equal(t, intermediateIANACiphers, cipherSuites)
	})

	t.Run("APIServer without tlsSecurityProfile uses intermediate defaults", func(t *testing.T) {
		t.Parallel()

		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(newClusterAPIServer(nil)).Build()

		minVersion, cipherSuites, err := pkgtls.FromAPIServer(ctx, cli, pkgtls.FormatShort)
		require.NoError(t, err)
		assert.Equal(t, "TLS1.2", minVersion)
		assert.Equal(t, intermediateIANACiphers, cipherSuites)
	})

	t.Run("APIServer with old profile floors to intermediate flags", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{Type: configv1.TLSProfileOldType})
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		minVersion, cipherSuites, err := pkgtls.FromAPIServer(ctx, cli, pkgtls.FormatShort)
		require.NoError(t, err)
		assert.Equal(t, "TLS1.2", minVersion)
		assert.Equal(t, intermediateIANACiphers, cipherSuites)
	})

	t.Run("APIServer with custom profile that has unsupported TLS version floors ciphers too", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileCustomType,
			Custom: &configv1.CustomTLSProfile{
				TLSProfileSpec: configv1.TLSProfileSpec{
					Ciphers:       []string{"ECDHE-RSA-AES256-GCM-SHA384", "DES-CBC3-SHA"},
					MinTLSVersion: configv1.VersionTLS11,
				},
			},
		})
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		minVersion, cipherSuites, err := pkgtls.FromAPIServer(ctx, cli, pkgtls.FormatShort)
		require.NoError(t, err)
		assert.Equal(t, "TLS1.2", minVersion)
		assert.Equal(t, intermediateIANACiphers, cipherSuites)
	})

	t.Run("APIServer with custom profile", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileCustomType,
			Custom: &configv1.CustomTLSProfile{
				TLSProfileSpec: configv1.TLSProfileSpec{
					Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
					MinTLSVersion: configv1.VersionTLS12,
				},
			},
		})
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		minVersion, cipherSuites, err := pkgtls.FromAPIServer(ctx, cli, pkgtls.FormatShort)
		require.NoError(t, err)
		assert.Equal(t, "TLS1.2", minVersion)
		assert.Equal(t, "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", cipherSuites)
	})

	t.Run("forbidden is returned", func(t *testing.T) {
		t.Parallel()

		cli := &erroringClient{
			Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
			getErr: k8serr.NewForbidden(
				schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"},
				cluster.ClusterAPIServerObj,
				errDenied,
			),
		}

		_, _, err := pkgtls.FromAPIServer(ctx, cli, pkgtls.FormatShort)
		require.Error(t, err)
		assert.True(t, k8serr.IsForbidden(err))
	})
}

func TestFetchAPIServerTLSProfile(t *testing.T) {
	t.Parallel()

	scheme := newTLSScheme(t)
	ctx := context.Background()

	t.Run("missing APIServer is an error", func(t *testing.T) {
		t.Parallel()

		cli := fake.NewClientBuilder().WithScheme(scheme).Build()

		_, err := pkgtls.FetchAPIServerTLSProfile(ctx, cli)
		require.Error(t, err)
		assert.True(t, k8serr.IsNotFound(err))
	})

	t.Run("modern profile", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType})
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		spec, err := pkgtls.FetchAPIServerTLSProfile(ctx, cli)
		require.NoError(t, err)
		assert.Equal(t, configv1.VersionTLS13, spec.MinTLSVersion)
		assert.Equal(t, configv1.TLSProfiles[configv1.TLSProfileModernType].Ciphers, spec.Ciphers)
	})
}

func TestLoad(t *testing.T) { //nolint:funlen // Fallback-policy cases for manager startup.
	t.Parallel()

	scheme := newTLSScheme(t)
	ctx := context.Background()
	intermediate := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]

	t.Run("success is watchable", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType})
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		result, err := pkgtls.Load(ctx, cli)
		require.NoError(t, err)
		assert.True(t, result.Watchable)
		assert.Equal(t, configv1.VersionTLS13, result.Spec.MinTLSVersion)
	})

	t.Run("not found is not watchable", func(t *testing.T) {
		t.Parallel()

		cli := fake.NewClientBuilder().WithScheme(scheme).Build()

		result, err := pkgtls.Load(ctx, cli)
		require.NoError(t, err)
		assert.False(t, result.Watchable)
		assert.Equal(t, intermediate.MinTLSVersion, result.Spec.MinTLSVersion)
		assert.Equal(t, intermediate.Ciphers, result.Spec.Ciphers)
	})

	t.Run("no match is not watchable", func(t *testing.T) {
		t.Parallel()

		cli := &erroringClient{
			Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
			getErr: &meta.NoKindMatchError{
				GroupKind: schema.GroupKind{Group: "config.openshift.io", Kind: "APIServer"},
			},
		}

		result, err := pkgtls.Load(ctx, cli)
		require.NoError(t, err)
		assert.False(t, result.Watchable)
		assert.Equal(t, intermediate.MinTLSVersion, result.Spec.MinTLSVersion)
	})

	t.Run("transient error is watchable", func(t *testing.T) {
		t.Parallel()

		transients := []error{
			k8serr.NewServiceUnavailable("unavailable"),
			k8serr.NewTimeoutError("timed out", 0),
			k8serr.NewServerTimeout(schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"}, "get", 0),
			k8serr.NewTooManyRequests("rate limited", 1),
		}
		for _, getErr := range transients {
			cli := &erroringClient{
				Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
				getErr: getErr,
			}

			result, err := pkgtls.Load(ctx, cli)
			require.NoError(t, err)
			assert.True(t, result.Watchable)
			assert.Equal(t, intermediate.MinTLSVersion, result.Spec.MinTLSVersion)
		}
	})

	t.Run("forbidden is returned", func(t *testing.T) {
		t.Parallel()

		cli := &erroringClient{
			Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
			getErr: k8serr.NewForbidden(
				schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"},
				cluster.ClusterAPIServerObj,
				errDenied,
			),
		}

		_, err := pkgtls.Load(ctx, cli)
		require.Error(t, err)
		assert.True(t, k8serr.IsForbidden(err))
	})
}
