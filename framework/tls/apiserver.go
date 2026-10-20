package tls

import (
	"context"
	"errors"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	commonTLS "github.com/openshift/controller-runtime-common/pkg/tls"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LoadResult is the cluster TLS profile to apply at manager startup.
type LoadResult struct {
	// Spec is the resolved APIServer TLS profile. On fallback it is Intermediate.
	// Under a non-adhering policy, callers select their legacy defaults instead.
	Spec configv1.TLSProfileSpec
	// AdherencePolicy is read from the APIServer by LoadWithAdherence. Load sets
	// it to NoOpinion because it does not fetch the policy.
	AdherencePolicy configv1.TLSAdherencePolicy
	// UsedFallback reports whether a missing or transient API error selected the
	// Intermediate profile instead of an APIServer profile.
	UsedFallback bool
	// Watchable is true when the OpenShift APIServer API is present (or is
	// expected to recover from a transient error). Register SecurityProfileWatcher
	// only when this is true.
	Watchable bool
}

// FetchAPIServerTLSProfile fetches the TLS profile spec configured on the
// cluster APIServer. Callers must have configv1.APIServer in the scheme.
//
// Get failures are returned as errors. Use [Load], [LoadWithAdherence], or
// [FromAPIServer] when missing-API fallback is required.
func FetchAPIServerTLSProfile(ctx context.Context, cli client.Reader) (configv1.TLSProfileSpec, error) {
	apiServer, err := getAPIServer(ctx, cli)
	if err != nil {
		return configv1.TLSProfileSpec{}, err
	}

	return *ProfileSpecFromSecurityProfile(apiServer.Spec.TLSSecurityProfile), nil
}

func getAPIServer(ctx context.Context, cli client.Reader) (*configv1.APIServer, error) {
	apiServer := &configv1.APIServer{}
	key := client.ObjectKey{Name: commonTLS.APIServerName}

	err := cli.Get(ctx, key, apiServer)
	if err != nil {
		return nil, fmt.Errorf("failed to get APIServer %q: %w", commonTLS.APIServerName, err)
	}

	return apiServer, nil
}

// FromAPIServer fetches the cluster TLS profile and returns version and cipher
// strings for operand/proxy flags. NotFound and NoMatch (vanilla Kubernetes)
// fall back to Intermediate defaults with a nil error. Other errors are returned.
func FromAPIServer(ctx context.Context, cli client.Reader, format VersionFormat) (string, string, error) {
	apiServer := &configv1.APIServer{}

	err := cli.Get(ctx, client.ObjectKey{Name: commonTLS.APIServerName}, apiServer)
	if err != nil {
		if k8serr.IsNotFound(err) || meta.IsNoMatchError(err) {
			minVersion, cipherSuites := FromProfile(ctx, nil, format)

			return minVersion, cipherSuites, nil
		}

		return "", "", fmt.Errorf("failed to get APIServer %q: %w", commonTLS.APIServerName, err)
	}

	minVersion, cipherSuites := FromProfile(ctx, apiServer.Spec.TLSSecurityProfile, format)

	return minVersion, cipherSuites, nil
}

// Load resolves only the cluster TLS profile for manager startup; it does not
// read TLS adherence and sets LoadResult.AdherencePolicy to NoOpinion. Use
// LoadWithAdherence when configuring SecurityProfileWatcher.OnAdherencePolicyChange.
//
//   - success: cluster spec, Watchable true
//   - API absent (NotFound / NoMatch): Intermediate, Watchable false
//   - transient API error: Intermediate, Watchable true (watcher can self-heal)
//   - unexpected error: returned so the caller can refuse to start
func Load(ctx context.Context, cli client.Reader) (LoadResult, error) {
	spec, err := FetchAPIServerTLSProfile(ctx, cli)
	if err == nil {
		return LoadResult{Spec: spec, AdherencePolicy: configv1.TLSAdherencePolicyNoOpinion, Watchable: true}, nil
	}

	result, ok := loadFallback(err)
	if !ok {
		return LoadResult{}, err
	}

	return result, nil
}

// LoadWithAdherence resolves the APIServer profile and adherence policy for
// manager startup. It validates profiles when adherence is Strict or unknown;
// callers still select their effective TLS options with ShouldHonorClusterTLSProfile.
func LoadWithAdherence(ctx context.Context, cli client.Reader) (LoadResult, error) {
	apiServer, err := getAPIServer(ctx, cli)
	if err != nil {
		result, ok := loadFallback(err)
		if !ok {
			return LoadResult{}, err
		}

		return result, nil
	}

	adherence := apiServer.Spec.TLSAdherence

	spec := ProfileSpecFromSecurityProfile(apiServer.Spec.TLSSecurityProfile)
	if ShouldHonorClusterTLSProfile(adherence) {
		specValue, specErr := GetStrictTLSProfileSpec(apiServer.Spec.TLSSecurityProfile)
		if specErr != nil {
			return LoadResult{}, fmt.Errorf("invalid strict APIServer TLS profile: %w", specErr)
		}
		spec = &specValue

		err = ValidateStrictTLSProfile(*spec)
		if err != nil {
			return LoadResult{}, fmt.Errorf("invalid strict APIServer TLS profile: %w", err)
		}
	}

	return LoadResult{Spec: *spec, AdherencePolicy: adherence, Watchable: true}, nil
}

func loadFallback(err error) (LoadResult, bool) {
	switch {
	case k8serr.IsNotFound(err) || meta.IsNoMatchError(err):
		return LoadResult{
			Spec:            intermediateSpec(),
			AdherencePolicy: configv1.TLSAdherencePolicyNoOpinion,
			UsedFallback:    true,
			Watchable:       false,
		}, true
	case isTransientAPIError(err):
		return LoadResult{
			Spec:            intermediateSpec(),
			AdherencePolicy: configv1.TLSAdherencePolicyNoOpinion,
			UsedFallback:    true,
			Watchable:       true,
		}, true
	default:
		return LoadResult{}, false
	}
}

// FromAPIServerWithAdherence resolves the cluster profile according to the
// APIServer TLS adherence policy. Missing APIs and non-adhering policies use
// Intermediate defaults; Strict and unknown policies fail on unusable profiles.
func FromAPIServerWithAdherence(
	ctx context.Context,
	cli client.Reader,
	format VersionFormat,
) (string, string, error) {
	profile, useClusterProfile, err := strictProfileFromAPIServer(ctx, cli)
	if err != nil {
		return "", "", err
	}

	if !useClusterProfile {
		minVersion, cipherSuites := FromProfile(ctx, nil, format)
		return minVersion, cipherSuites, nil
	}

	return FromProfileStrict(ctx, profile, format)
}

// FromAPIServerWithCurvePreferences resolves the cluster profile according to
// its adherence policy and returns version, cipher, and curve preference strings.
func FromAPIServerWithCurvePreferences(
	ctx context.Context,
	cli client.Reader,
	format VersionFormat,
) (string, string, string, error) {
	profile, useClusterProfile, err := strictProfileFromAPIServer(ctx, cli)
	if err != nil {
		return "", "", "", err
	}

	if !useClusterProfile {
		return FromProfileWithCurvePreferences(ctx, nil, format)
	}

	return FromProfileStrictWithCurvePreferences(ctx, profile, format)
}

func strictProfileFromAPIServer(
	ctx context.Context,
	cli client.Reader,
) (*configv1.TLSSecurityProfile, bool, error) {
	apiServer, err := getAPIServer(ctx, cli)
	if err != nil {
		if k8serr.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil, false, nil
		}

		return nil, false, err
	}

	if !ShouldHonorClusterTLSProfile(apiServer.Spec.TLSAdherence) {
		return nil, false, nil
	}

	return apiServer.Spec.TLSSecurityProfile, true, nil
}

func isTransientAPIError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) ||
		k8serr.IsServiceUnavailable(err) ||
		k8serr.IsTimeout(err) ||
		k8serr.IsServerTimeout(err) ||
		k8serr.IsTooManyRequests(err)
}
