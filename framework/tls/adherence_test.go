package tls_test

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pkgtls "github.com/opendatahub-io/odh-platform-utilities/framework/tls"
)

func TestShouldHonorClusterTLSProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy configv1.TLSAdherencePolicy
		want   bool
	}{
		{name: "NoOpinion empty value", policy: configv1.TLSAdherencePolicyNoOpinion, want: false},
		{name: "LegacyAdheringComponentsOnly", policy: configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly, want: false},
		{name: "StrictAllComponents", policy: configv1.TLSAdherencePolicyStrictAllComponents, want: true},
		{name: "unknown value", policy: "FuturePolicy", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, pkgtls.ShouldHonorClusterTLSProfile(tt.policy))
		})
	}
}

func TestValidateStrictTLSProfile(t *testing.T) { //nolint:funlen // Table-driven profile validation cases.
	t.Parallel()

	tests := []struct {
		name    string
		spec    configv1.TLSProfileSpec
		wantErr string
	}{
		{name: "intermediate profile", spec: *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]},
		{name: "modern profile", spec: *configv1.TLSProfiles[configv1.TLSProfileModernType]},
		{
			name: "old profile minimum is honored",
			spec: *configv1.TLSProfiles[configv1.TLSProfileOldType],
		},
		{
			name: "custom TLS 1.1 minimum is honored",
			spec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS11,
				Ciphers:       []string{openSSLECDHERSAAES128GCM},
			},
		},
		{
			name: "unknown version is unsupported",
			spec: configv1.TLSProfileSpec{
				MinTLSVersion: "VersionTLS99",
				Ciphers:       []string{openSSLECDHERSAAES128GCM},
			},
			wantErr: "minimum version",
		},
		{
			name: "empty TLS 1.2 cipher list",
			spec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS12,
			},
			wantErr: "no cipher suites",
		},
		{
			name: "all TLS 1.2 ciphers unsupported",
			spec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS12,
				Ciphers:       []string{openSSLDHERSAAES128GCM},
			},
			wantErr: "no cipher suites supported",
		},
		{
			name: "TLS 1.3-only ciphers do not satisfy TLS 1.2",
			spec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS12,
				Ciphers:       []string{ianaTLS13AES128GCM},
			},
			wantErr: "no cipher suites supported",
		},
		{
			name: "mixed TLS 1.2 and TLS 1.3 ciphers retain TLS 1.2 suites",
			spec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS12,
				Ciphers: []string{
					ianaTLS13AES128GCM,
					openSSLECDHERSAAES128GCM,
				},
			},
		},
		{
			name: "TLS 1.3 cipher list remains informational",
			spec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS13,
				Ciphers:       []string{ianaTLS13AES128GCM},
			},
		},
		{
			name: "TLS 1.3 permits an empty cipher list",
			spec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS13,
			},
		},
		{
			name: "all groups unsupported",
			spec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS12,
				Ciphers:       []string{openSSLECDHERSAAES128GCM},
				Groups:        []configv1.TLSGroup{"unknown-group"},
			},
			wantErr: "no TLS groups supported",
		},
		{
			name: "partially supported groups",
			spec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS12,
				Ciphers:       []string{openSSLECDHERSAAES128GCM},
				Groups:        []configv1.TLSGroup{"unknown-group", configv1.TLSGroupX25519},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := pkgtls.ValidateStrictTLSProfile(tt.spec)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestCurvePreferencesFromSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    *configv1.TLSProfileSpec
		want    string
		wantErr bool
	}{
		{name: "nil spec", want: ""},
		{name: "no groups", spec: &configv1.TLSProfileSpec{}, want: ""},
		{
			name:    "all groups unsupported",
			spec:    &configv1.TLSProfileSpec{Groups: []configv1.TLSGroup{"unknown-group"}},
			wantErr: true,
		},
		{
			name: "supported groups",
			spec: &configv1.TLSProfileSpec{Groups: []configv1.TLSGroup{
				configv1.TLSGroupX25519,
				configv1.TLSGroupSecP256r1,
				configv1.TLSGroupSecP256r1MLKEM768,
			}},
			want: "29,23,4587",
		},
		{
			name: "unsupported groups are dropped",
			spec: &configv1.TLSProfileSpec{Groups: []configv1.TLSGroup{
				"unknown-group",
				configv1.TLSGroupX25519,
			}},
			want: "29",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := pkgtls.CurvePreferencesFromSpec(context.Background(), tt.spec)
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, got)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFromProfileStrict(t *testing.T) {
	t.Parallel()

	profile := &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileCustomType,
		Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
			Ciphers:       []string{openSSLECDHERSAAES128GCM},
			MinTLSVersion: configv1.VersionTLS12,
		}},
	}
	minVersion, ciphers, err := pkgtls.FromProfileStrict(context.Background(), profile, pkgtls.FormatGo)
	require.NoError(t, err)
	assert.Equal(t, "VersionTLS12", minVersion)
	assert.Equal(t, ianaECDHERSAAES128GCM, ciphers)

	minVersion, ciphers, err = pkgtls.FromProfileStrict(context.Background(), &configv1.TLSSecurityProfile{}, pkgtls.FormatShort)
	require.NoError(t, err)
	assert.Equal(t, "TLS1.2", minVersion)
	assert.Equal(t, intermediateIANACiphers, ciphers)

	unsupported := &configv1.TLSSecurityProfile{Type: "Unknown"}
	_, _, err = pkgtls.FromProfileStrict(context.Background(), unsupported, pkgtls.FormatShort)
	require.ErrorContains(t, err, "unsupported TLS profile type")

	for _, tt := range []struct {
		version   configv1.TLSProtocolVersion
		want      string
		wantShort string
	}{
		{version: configv1.VersionTLS10, want: "VersionTLS10", wantShort: "TLS1.0"},
		{version: configv1.VersionTLS11, want: "VersionTLS11", wantShort: "TLS1.1"},
	} {
		profile := &configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileCustomType,
			Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
				MinTLSVersion: tt.version,
				Ciphers:       []string{openSSLECDHERSAAES128GCM},
			}},
		}

		// Both formats express all four version names: kube-auth-proxy
		// (short) and kube-rbac-proxy (Go) both parse them.
		shortMinVersion, shortCiphers, shortErr := pkgtls.FromProfileStrict(context.Background(), profile, pkgtls.FormatShort)
		require.NoError(t, shortErr)
		assert.Equal(t, tt.wantShort, shortMinVersion)
		assert.Equal(t, ianaECDHERSAAES128GCM, shortCiphers)

		minVersion, ciphers, goErr := pkgtls.FromProfileStrict(context.Background(), profile, pkgtls.FormatGo)
		require.NoError(t, goErr)
		assert.Equal(t, tt.want, minVersion)
		assert.Equal(t, ianaECDHERSAAES128GCM, ciphers)
	}

	profile = &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileCustomType,
		Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
			MinTLSVersion: "VersionTLS99",
			Ciphers:       []string{openSSLECDHERSAAES128GCM},
		}},
	}
	_, _, err = pkgtls.FromProfileStrict(context.Background(), profile, pkgtls.FormatShort)
	require.ErrorContains(t, err, "minimum version")
}

func TestFromProfileStrictTLS13CipherFiltering(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		minVersion  configv1.TLSProtocolVersion
		ciphers     []string
		wantCiphers string
		wantErr     string
	}{
		{
			name:       "TLS 1.3-only suites cannot satisfy TLS 1.2",
			minVersion: configv1.VersionTLS12,
			ciphers:    []string{ianaTLS13AES128GCM},
			wantErr:    "no cipher suites supported",
		},
		{
			name:        "mixed TLS 1.2 and TLS 1.3 suites keep only TLS 1.2",
			minVersion:  configv1.VersionTLS12,
			ciphers:     []string{ianaTLS13AES128GCM, openSSLECDHERSAAES128GCM},
			wantCiphers: ianaECDHERSAAES128GCM,
		},
		{
			name:        "TLS 1.3 cipher subset is informational",
			minVersion:  configv1.VersionTLS13,
			ciphers:     []string{ianaTLS13AES128GCM},
			wantCiphers: ianaTLS13AES128GCM,
		},
		{
			name:       "unmappable TLS 1.3 ciphers are informational",
			minVersion: configv1.VersionTLS13,
			ciphers:    []string{openSSLDHERSAAES128GCM},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			profile := &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
					MinTLSVersion: tt.minVersion,
					Ciphers:       tt.ciphers,
				}},
			}
			version, ciphers, err := pkgtls.FromProfileStrict(context.Background(), profile, pkgtls.FormatShort)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Empty(t, version)
				assert.Empty(t, ciphers)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, map[configv1.TLSProtocolVersion]string{
				configv1.VersionTLS12: "TLS1.2",
				configv1.VersionTLS13: "TLS1.3",
			}[tt.minVersion], version)
			assert.Equal(t, tt.wantCiphers, ciphers)
		})
	}
}

func TestFromAPIServerWithCurvePreferences(t *testing.T) { //nolint:funlen // Table-driven adherence resolution cases.
	t.Parallel()

	scheme := newTLSScheme(t)
	ctx := context.Background()
	strictProfile := &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileCustomType,
		Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
			Ciphers:       []string{openSSLECDHERSAAES128GCM},
			MinTLSVersion: configv1.VersionTLS12,
			Groups:        []configv1.TLSGroup{configv1.TLSGroupX25519},
		}},
	}
	apiServer := func(adherence configv1.TLSAdherencePolicy) *configv1.APIServer {
		object := newClusterAPIServer(strictProfile)
		object.Spec.TLSAdherence = adherence

		return object
	}
	apiServerWithProfile := func(profile *configv1.TLSSecurityProfile, adherence configv1.TLSAdherencePolicy) *configv1.APIServer {
		object := newClusterAPIServer(profile)
		object.Spec.TLSAdherence = adherence

		return object
	}
	emptyTypeAPIServer := newClusterAPIServer(&configv1.TLSSecurityProfile{})
	emptyTypeAPIServer.Spec.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents

	tests := []struct {
		getErr      error
		name        string
		wantVer     string
		wantCiphers string
		wantCurves  string
		objects     []client.Object
		wantErr     bool
	}{
		{
			name:        "no APIServer uses intermediate defaults",
			wantVer:     "TLS1.2",
			wantCiphers: intermediateIANACiphers,
			wantCurves:  "4588,29,23,24",
		},
		{
			name:        "NoOpinion uses intermediate defaults",
			objects:     []client.Object{apiServer(configv1.TLSAdherencePolicyNoOpinion)},
			wantVer:     "TLS1.2",
			wantCiphers: intermediateIANACiphers,
			wantCurves:  "4588,29,23,24",
		},
		{
			name:        "Strict applies the profile",
			objects:     []client.Object{apiServer(configv1.TLSAdherencePolicyStrictAllComponents)},
			wantVer:     "TLS1.2",
			wantCiphers: ianaECDHERSAAES128GCM,
			wantCurves:  "29",
		},
		{
			name:        "Strict empty profile type defaults to Intermediate",
			objects:     []client.Object{emptyTypeAPIServer},
			wantVer:     "TLS1.2",
			wantCiphers: intermediateIANACiphers,
			wantCurves:  "4588,29,23,24",
		},
		{
			name: "Strict TLS 1.2 rejects TLS 1.3-only ciphers",
			objects: []client.Object{apiServerWithProfile(
				&configv1.TLSSecurityProfile{
					Type: configv1.TLSProfileCustomType,
					Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers:       []string{ianaTLS13AES128GCM},
						Groups:        []configv1.TLSGroup{configv1.TLSGroupX25519},
					}},
				},
				configv1.TLSAdherencePolicyStrictAllComponents,
			)},
			wantErr: true,
		},
		{
			name: "Strict TLS 1.2 keeps supported suites from mixed list",
			objects: []client.Object{apiServerWithProfile(
				&configv1.TLSSecurityProfile{
					Type: configv1.TLSProfileCustomType,
					Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers:       []string{ianaTLS13AES128GCM, openSSLECDHERSAAES128GCM},
						Groups:        []configv1.TLSGroup{configv1.TLSGroupX25519},
					}},
				},
				configv1.TLSAdherencePolicyStrictAllComponents,
			)},
			wantVer:     "TLS1.2",
			wantCiphers: ianaECDHERSAAES128GCM,
			wantCurves:  "29",
		},
		{
			name: "Strict TLS 1.2 rejects profiles with no supported groups",
			objects: []client.Object{apiServerWithProfile(
				&configv1.TLSSecurityProfile{
					Type: configv1.TLSProfileCustomType,
					Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers:       []string{openSSLECDHERSAAES128GCM},
						Groups:        []configv1.TLSGroup{"unsupported-group"},
					}},
				},
				configv1.TLSAdherencePolicyStrictAllComponents,
			)},
			wantErr: true,
		},
		{
			name: "Strict Modern retains TLS 1.3 cipher names",
			objects: []client.Object{apiServerWithProfile(
				&configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
				configv1.TLSAdherencePolicyStrictAllComponents,
			)},
			wantVer:     "TLS1.3",
			wantCiphers: tls13IANACiphers,
			wantCurves:  "4588,29,23,24",
		},
		{
			name:        "unknown adherence fails closed by applying the profile",
			objects:     []client.Object{apiServer("FuturePolicy")},
			wantVer:     "TLS1.2",
			wantCiphers: ianaECDHERSAAES128GCM,
			wantCurves:  "29",
		},
		{
			name: "forbidden read returns an error",
			getErr: k8serr.NewForbidden(
				schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"},
				"cluster",
				nil,
			),
			wantErr: true,
		},
		{
			name:    "NoMatch uses intermediate defaults",
			getErr:  &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "config.openshift.io", Kind: "APIServer"}},
			wantVer: "TLS1.2", wantCiphers: intermediateIANACiphers, wantCurves: "4588,29,23,24",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build()

			var reader client.Reader = fakeClient
			if tt.getErr != nil {
				reader = &erroringClient{Client: fakeClient, getErr: tt.getErr}
			}

			version, ciphers, curves, err := pkgtls.FromAPIServerWithCurvePreferences(ctx, reader, pkgtls.FormatShort)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantVer, version)
			assert.Equal(t, tt.wantCiphers, ciphers)
			assert.Equal(t, tt.wantCurves, curves)
		})
	}
}

func TestFromAPIServerWithAdherence(t *testing.T) { //nolint:funlen // Covers direct non-curve resolver edge cases.
	t.Parallel()

	scheme := newTLSScheme(t)
	modern := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}
	apiServer := func(profile *configv1.TLSSecurityProfile, policy configv1.TLSAdherencePolicy) *configv1.APIServer {
		object := newClusterAPIServer(profile)
		object.Spec.TLSAdherence = policy
		return object
	}

	tests := []struct {
		objects     []client.Object
		getErr      error
		name        string
		wantVersion string
		wantCiphers string
		wantErr     string
	}{
		{
			name:        "missing APIServer uses intermediate",
			wantVersion: "VersionTLS12",
			wantCiphers: intermediateIANACiphers,
		},
		{
			name:        "NoOpinion uses intermediate despite configured profile",
			objects:     []client.Object{apiServer(modern, configv1.TLSAdherencePolicyNoOpinion)},
			wantVersion: "VersionTLS12",
			wantCiphers: intermediateIANACiphers,
		},
		{
			name:        "Strict applies TLS 1.3 profile",
			objects:     []client.Object{apiServer(modern, configv1.TLSAdherencePolicyStrictAllComponents)},
			wantVersion: "VersionTLS13",
			wantCiphers: tls13IANACiphers,
		},
		{
			name: "Strict rejects TLS 1.3-only ciphers with TLS 1.2 minimum",
			objects: []client.Object{apiServer(&configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
					MinTLSVersion: configv1.VersionTLS12,
					Ciphers:       []string{ianaTLS13AES128GCM},
				}},
			}, configv1.TLSAdherencePolicyStrictAllComponents)},
			wantErr: "no cipher suites supported",
		},
		{
			name: "Strict rejects profiles with no supported groups",
			objects: []client.Object{apiServer(&configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
					MinTLSVersion: configv1.VersionTLS12,
					Ciphers:       []string{openSSLECDHERSAAES128GCM},
					Groups:        []configv1.TLSGroup{"unsupported-group"},
				}},
			}, configv1.TLSAdherencePolicyStrictAllComponents)},
			wantErr: "no TLS groups supported",
		},
		{
			name:    "unauthorized APIServer read fails closed",
			getErr:  k8serr.NewUnauthorized("authentication required"),
			wantErr: "failed to get APIServer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build()
			var reader client.Reader = fakeClient
			if tt.getErr != nil {
				reader = &erroringClient{Client: fakeClient, getErr: tt.getErr}
			}
			version, ciphers, err := pkgtls.FromAPIServerWithAdherence(context.Background(), reader, pkgtls.FormatGo)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantVersion, version)
			assert.Equal(t, tt.wantCiphers, ciphers)
		})
	}
}

func TestLoadWithAdherence(t *testing.T) { //nolint:funlen // Startup fallback and validation cases.
	t.Parallel()

	scheme := newTLSScheme(t)
	intermediate := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]

	t.Run("strict policy returns profile and watchable state", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType})
		apiServer.Spec.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		result, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.NoError(t, err)
		assert.True(t, result.Watchable)
		assert.False(t, result.UsedFallback)
		assert.Equal(t, configv1.TLSAdherencePolicyStrictAllComponents, result.AdherencePolicy)
		assert.Equal(t, configv1.VersionTLS13, result.Spec.MinTLSVersion)
	})

	t.Run("strict old profile is honored", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{Type: configv1.TLSProfileOldType})
		apiServer.Spec.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		result, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.NoError(t, err)
		assert.Equal(t, configv1.TLSAdherencePolicyStrictAllComponents, result.AdherencePolicy)
		assert.Equal(t, configv1.VersionTLS10, result.Spec.MinTLSVersion)
		assert.Equal(t, configv1.TLSProfiles[configv1.TLSProfileOldType].Ciphers, result.Spec.Ciphers)
	})

	t.Run("strict empty profile type defaults to Intermediate", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{})
		apiServer.Spec.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		result, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.NoError(t, err)
		assert.Equal(t, configv1.TLSAdherencePolicyStrictAllComponents, result.AdherencePolicy)
		assert.Equal(t, intermediate.MinTLSVersion, result.Spec.MinTLSVersion)
	})

	t.Run("strict unknown profile type fails closed", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{Type: "Unknown"})
		apiServer.Spec.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		_, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.ErrorContains(t, err, "unsupported TLS profile type")
	})

	t.Run("NoOpinion unknown profile type falls back to Intermediate", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{Type: "Unknown"})
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		result, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.NoError(t, err)
		assert.Equal(t, configv1.VersionTLS12, result.Spec.MinTLSVersion)
	})

	t.Run("Legacy unknown profile type falls back to Intermediate", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{Type: "Unknown"})
		apiServer.Spec.TLSAdherence = configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		result, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.NoError(t, err)
		assert.Equal(t, configv1.VersionTLS12, result.Spec.MinTLSVersion)
	})

	t.Run("strict custom profile without spec fails closed", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{Type: configv1.TLSProfileCustomType})
		apiServer.Spec.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		_, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.ErrorContains(t, err, "custom TLS profile has no custom specification")
	})

	t.Run("NoOpinion preserves the resolved profile for change detection", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType})
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		result, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.NoError(t, err)
		assert.Equal(t, configv1.TLSAdherencePolicyNoOpinion, result.AdherencePolicy)
		assert.Equal(t, configv1.VersionTLS13, result.Spec.MinTLSVersion)
	})

	t.Run("invalid strict profile fails closed", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileCustomType,
			Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS12,
			}},
		})
		apiServer.Spec.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		_, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.ErrorContains(t, err, "no cipher suites")
	})

	t.Run("TLS 1.3-only ciphers do not satisfy strict TLS 1.2", func(t *testing.T) {
		t.Parallel()

		apiServer := newClusterAPIServer(&configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileCustomType,
			Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS12,
				Ciphers:       []string{ianaTLS13AES128GCM},
			}},
		})
		apiServer.Spec.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

		_, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.ErrorContains(t, err, "no cipher suites supported")
	})

	t.Run("missing APIServer uses intermediate defaults and is not watchable", func(t *testing.T) {
		t.Parallel()

		cli := fake.NewClientBuilder().WithScheme(scheme).Build()

		result, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.NoError(t, err)
		assert.False(t, result.Watchable)
		assert.True(t, result.UsedFallback)
		assert.Equal(t, configv1.TLSAdherencePolicyNoOpinion, result.AdherencePolicy)
		assert.Equal(t, intermediate.MinTLSVersion, result.Spec.MinTLSVersion)
	})

	t.Run("NoMatch uses intermediate defaults and is not watchable", func(t *testing.T) {
		t.Parallel()

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		cli := &erroringClient{
			Client: fakeClient,
			getErr: &meta.NoKindMatchError{
				GroupKind: schema.GroupKind{Group: "config.openshift.io", Kind: "APIServer"},
			},
		}

		result, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.NoError(t, err)
		assert.False(t, result.Watchable)
		assert.True(t, result.UsedFallback)
		assert.Equal(t, intermediate.MinTLSVersion, result.Spec.MinTLSVersion)
	})

	t.Run("transient error uses intermediate and remains watchable", func(t *testing.T) {
		t.Parallel()

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		cli := &erroringClient{Client: fakeClient, getErr: k8serr.NewServiceUnavailable("unavailable")}

		result, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.NoError(t, err)
		assert.True(t, result.Watchable)
		assert.True(t, result.UsedFallback)
		assert.Equal(t, configv1.TLSAdherencePolicyNoOpinion, result.AdherencePolicy)
		assert.Equal(t, intermediate.MinTLSVersion, result.Spec.MinTLSVersion)
	})

	t.Run("forbidden error is returned", func(t *testing.T) {
		t.Parallel()

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		cli := &erroringClient{
			Client: fakeClient,
			getErr: k8serr.NewForbidden(
				schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"},
				"cluster",
				nil,
			),
		}

		_, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.Error(t, err)
		assert.True(t, k8serr.IsForbidden(err))
	})

	t.Run("unauthorized error is returned", func(t *testing.T) {
		t.Parallel()

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		cli := &erroringClient{Client: fakeClient, getErr: k8serr.NewUnauthorized("authentication required")}

		_, err := pkgtls.LoadWithAdherence(context.Background(), cli)
		require.Error(t, err)
		assert.True(t, k8serr.IsUnauthorized(err))
	})
}

func TestStrictAPIServerProfileMissingCustomFailsClosed(t *testing.T) {
	t.Parallel()

	scheme := newTLSScheme(t)
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSAdherence:       configv1.TLSAdherencePolicyStrictAllComponents,
			TLSSecurityProfile: &configv1.TLSSecurityProfile{Type: configv1.TLSProfileCustomType},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

	_, _, err := pkgtls.FromAPIServerWithAdherence(context.Background(), cli, pkgtls.FormatShort)
	require.ErrorContains(t, err, "custom TLS profile has no custom specification")
}
