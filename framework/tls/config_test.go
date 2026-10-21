package tls_test

import (
	"crypto/tls"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgtls "github.com/opendatahub-io/odh-platform-utilities/framework/tls"
)

func TestConfigFromProfile_TLS12SetsCipherSuites(t *testing.T) {
	t.Parallel()

	spec := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	fn, unsupported := pkgtls.ConfigFromProfile(spec)
	require.NotNil(t, fn)
	assert.Equal(t, []string{
		ianaTLS13AES128GCM,
		ianaTLS13AES256GCM,
		ianaTLS13CHACHA20,
	}, unsupported)

	cfg := &tls.Config{}
	fn(cfg)

	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.Len(t, cfg.CipherSuites, 6)
	assert.Contains(t, cfg.CurvePreferences, tls.X25519)
	assert.Contains(t, cfg.CurvePreferences, tls.CurveP256)
}

func TestConfigFromProfile_TLS12DropsTLS13Ciphers(t *testing.T) {
	t.Parallel()

	spec := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers: []string{
			ianaTLS13AES128GCM,
			openSSLECDHERSAAES128GCM,
		},
	}
	fn, unsupported := pkgtls.ConfigFromProfile(spec)
	cfg := &tls.Config{}
	fn(cfg)

	assert.Equal(t, []string{ianaTLS13AES128GCM}, unsupported)
	assert.Equal(t, []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}, cfg.CipherSuites)
}

func TestConfigFromProfile_TLS13LeavesCipherSuitesUnset(t *testing.T) {
	t.Parallel()

	spec := *configv1.TLSProfiles[configv1.TLSProfileModernType]
	fn, unsupported := pkgtls.ConfigFromProfile(spec)
	require.NotNil(t, fn)
	assert.Empty(t, unsupported)

	cfg := &tls.Config{}
	fn(cfg)

	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	assert.Empty(t, cfg.CipherSuites)
}

func TestConfigFromProfile_UnsupportedCiphersReported(t *testing.T) {
	t.Parallel()

	spec := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{openSSLECDHERSAAES128GCM, openSSLDHERSAAES128GCM},
	}
	fn, unsupported := pkgtls.ConfigFromProfile(spec)
	require.NotNil(t, fn)
	assert.Equal(t, []string{openSSLDHERSAAES128GCM}, unsupported)

	cfg := &tls.Config{}
	fn(cfg)
	assert.Len(t, cfg.CipherSuites, 1)
}

func TestConfigFromProfile_DropsRC4Ciphers(t *testing.T) {
	t.Parallel()

	spec := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"TLS_RSA_WITH_RC4_128_SHA", openSSLECDHERSAAES128GCM},
	}
	fn, unsupported := pkgtls.ConfigFromProfile(spec)
	require.NotNil(t, fn)
	assert.ElementsMatch(t, []string{"TLS_RSA_WITH_RC4_128_SHA"}, unsupported)

	cfg := &tls.Config{}
	fn(cfg)
	assert.Equal(t, []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}, cfg.CipherSuites)
}

func TestConfigFromProfile_DropsUnnameableInsecureCiphers(t *testing.T) {
	t.Parallel()

	// The ECDHE RSA 3DES suite is implemented by this runtime but no
	// OpenShift profile can name it (Old carries only the RSA key-exchange
	// 3DES variant), so the profile cipher vocabulary rejects it.
	spec := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA", openSSLECDHERSAAES128GCM},
	}
	fn, unsupported := pkgtls.ConfigFromProfile(spec)
	require.NotNil(t, fn)
	assert.ElementsMatch(t, []string{"TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA"}, unsupported)

	cfg := &tls.Config{}
	fn(cfg)
	assert.Equal(t, []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}, cfg.CipherSuites)
}

func TestValidateStrictTLSProfile_RejectsRC4OnlyLists(t *testing.T) {
	t.Parallel()

	err := pkgtls.ValidateStrictTLSProfile(configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"TLS_RSA_WITH_RC4_128_SHA"},
	})
	assert.ErrorIs(t, err, pkgtls.ErrTLSProfileHasNoSupportedCipherSuites)
}

func TestConfigFromProfileStrict_RejectsInvalidMinimum(t *testing.T) {
	t.Parallel()

	fn, unsupported, err := pkgtls.ConfigFromProfileStrict(configv1.TLSProfileSpec{
		MinTLSVersion: "VersionTLS99",
	})
	assert.Nil(t, fn)
	assert.Nil(t, unsupported)
	assert.ErrorIs(t, err, pkgtls.ErrUnsupportedTLSProfileVersion)
}

func TestConfigFromProfileStrict_AcceptsValidProfile(t *testing.T) {
	t.Parallel()

	fn, unsupported, err := pkgtls.ConfigFromProfileStrict(*configv1.TLSProfiles[configv1.TLSProfileIntermediateType])
	require.NoError(t, err)
	require.NotNil(t, fn)
	assert.NotEmpty(t, unsupported)
}

func TestConfigFromProfile_ReportsMappedButUnimplementedCipher(t *testing.T) {
	t.Parallel()

	fn, unsupported := pkgtls.ConfigFromProfile(configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"DHE-RSA-AES128-GCM-SHA256"},
	})
	require.NotNil(t, fn)
	assert.Equal(t, []string{"DHE-RSA-AES128-GCM-SHA256"}, unsupported)

	_, unsupported = pkgtls.ConfigFromProfile(configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"ECDHE-RSA-AES256-SHA384"},
	})
	assert.Equal(t, []string{"ECDHE-RSA-AES256-SHA384"}, unsupported)
}

func TestConfigFromProfile_TLS10HonorsOldProfile(t *testing.T) {
	t.Parallel()

	spec := *configv1.TLSProfiles[configv1.TLSProfileOldType]
	fn, unsupported := pkgtls.ConfigFromProfile(spec)
	require.NotNil(t, fn)
	// The TLS 1.3 suites are not configurable below TLS 1.3; the CBC SHA384/
	// SHA256 suites are pruned by library-go's ciphersUnsupportedByGo map.
	assert.ElementsMatch(t, []string{
		ianaTLS13AES128GCM,
		ianaTLS13AES256GCM,
		ianaTLS13CHACHA20,
		openSSLECDHEECDSAAES256SHA,
		openSSLECDHERSAAES256SHA384,
		openSSLAES256SHA256,
	}, unsupported)

	cfg := &tls.Config{}
	fn(cfg)

	assert.Equal(t, uint16(tls.VersionTLS10), cfg.MinVersion)
	assert.NotEmpty(t, cfg.CipherSuites)
	for _, code := range cfg.CipherSuites {
		assert.NotEqual(t, tls.TLS_AES_128_GCM_SHA256, code)
	}
}

func TestConfigFromProfile_UnknownMinVersionFloorsToTLS12(t *testing.T) {
	t.Parallel()

	spec := configv1.TLSProfileSpec{
		MinTLSVersion: "VersionTLS99",
		Ciphers:       []string{openSSLECDHERSAAES128GCM},
	}
	fn, _ := pkgtls.ConfigFromProfile(spec)
	cfg := &tls.Config{}
	fn(cfg)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
}

func TestConfigFromProfile_SkipsUnknownGroups(t *testing.T) {
	t.Parallel()

	spec := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{openSSLECDHERSAAES128GCM},
		Groups: []configv1.TLSGroup{
			configv1.TLSGroupX25519,
			"not-a-group",
			configv1.TLSGroupSecP256r1MLKEM768,
			configv1.TLSGroupSecP384r1MLKEM1024,
		},
	}
	fn, unsupported := pkgtls.ConfigFromProfile(spec)
	cfg := &tls.Config{}
	fn(cfg)
	assert.Equal(t, []tls.CurveID{tls.X25519, tls.SecP256r1MLKEM768, tls.SecP384r1MLKEM1024}, cfg.CurvePreferences)
	assert.Equal(t, []string{"not-a-group"}, unsupported)
}

func TestConfigFromProfile_EmptyGroupsLeaveCurvePreferencesUnset(t *testing.T) {
	t.Parallel()

	spec := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{openSSLECDHERSAAES128GCM},
	}
	fn, _ := pkgtls.ConfigFromProfile(spec)
	cfg := &tls.Config{}
	fn(cfg)
	assert.Nil(t, cfg.CurvePreferences)
}
