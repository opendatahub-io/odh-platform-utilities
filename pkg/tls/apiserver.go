package tls

import (
	"context"
	"errors"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
)

// LoadResult is the cluster TLS profile to apply at manager startup.
type LoadResult struct {
	// Spec is the resolved TLS profile. On fallback it is the Intermediate profile.
	Spec configv1.TLSProfileSpec
	// Watchable is true when the OpenShift APIServer API is present (or is
	// expected to recover from a transient error). Register SecurityProfileWatcher
	// only when this is true.
	Watchable bool
}

// FetchAPIServerTLSProfile fetches the TLS profile spec configured on the
// cluster APIServer. Callers must have configv1.APIServer in the scheme.
//
// Get failures are returned as errors. Use [Load] or [FromAPIServer] when
// missing-API fallback is required.
func FetchAPIServerTLSProfile(ctx context.Context, cli client.Reader) (configv1.TLSProfileSpec, error) {
	apiServer := &configv1.APIServer{}

	err := cli.Get(ctx, client.ObjectKey{Name: cluster.ClusterAPIServerObj}, apiServer)
	if err != nil {
		return configv1.TLSProfileSpec{}, fmt.Errorf("failed to get APIServer %q: %w", cluster.ClusterAPIServerObj, err)
	}

	return *ProfileSpecFromSecurityProfile(apiServer.Spec.TLSSecurityProfile), nil
}

// FromAPIServer fetches the cluster TLS profile and returns version and cipher
// strings for operand/proxy flags. NotFound and NoMatch (vanilla Kubernetes)
// fall back to Intermediate defaults with a nil error. Other errors are returned.
func FromAPIServer(ctx context.Context, cli client.Reader, format VersionFormat) (string, string, error) {
	apiServer := &configv1.APIServer{}

	err := cli.Get(ctx, client.ObjectKey{Name: cluster.ClusterAPIServerObj}, apiServer)
	if err != nil {
		if k8serr.IsNotFound(err) || meta.IsNoMatchError(err) {
			minVersion, cipherSuites := FromProfile(ctx, nil, format)

			return minVersion, cipherSuites, nil
		}

		return "", "", fmt.Errorf("failed to get APIServer %q: %w", cluster.ClusterAPIServerObj, err)
	}

	minVersion, cipherSuites := FromProfile(ctx, apiServer.Spec.TLSSecurityProfile, format)

	return minVersion, cipherSuites, nil
}

// Load resolves the cluster TLS profile for manager startup.
//
//   - success: cluster spec, Watchable true
//   - API absent (NotFound / NoMatch): Intermediate, Watchable false
//   - transient API error: Intermediate, Watchable true (watcher can self-heal)
//   - unexpected error: returned so the caller can refuse to start
func Load(ctx context.Context, cli client.Reader) (LoadResult, error) {
	spec, err := FetchAPIServerTLSProfile(ctx, cli)
	if err == nil {
		return LoadResult{Spec: spec, Watchable: true}, nil
	}

	switch {
	case k8serr.IsNotFound(err) || meta.IsNoMatchError(err):
		return LoadResult{Spec: intermediateSpec(), Watchable: false}, nil
	case isTransientAPIError(err):
		return LoadResult{Spec: intermediateSpec(), Watchable: true}, nil
	default:
		return LoadResult{}, err
	}
}

func isTransientAPIError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) ||
		k8serr.IsServiceUnavailable(err) ||
		k8serr.IsTimeout(err) ||
		k8serr.IsServerTimeout(err) ||
		k8serr.IsTooManyRequests(err)
}
