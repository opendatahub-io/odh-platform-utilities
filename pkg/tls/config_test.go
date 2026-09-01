package tls_test

import (
	"crypto/tls"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgtls "github.com/opendatahub-io/odh-platform-utilities/pkg/tls"
)

func TestConfigFromProfile_TLS12SetsCipherSuites(t *testing.T) {
	t.Parallel()

	spec := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	fn, unsupported := pkgtls.ConfigFromProfile(spec)
	require.NotNil(t, fn)
	assert.Empty(t, unsupported)

	cfg := &tls.Config{}
	fn(cfg)

	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.NotEmpty(t, cfg.CipherSuites)
	assert.Contains(t, cfg.CurvePreferences, tls.X25519)
	assert.Contains(t, cfg.CurvePreferences, tls.CurveP256)
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
		Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", "DHE-RSA-AES128-GCM-SHA256"},
	}
	fn, unsupported := pkgtls.ConfigFromProfile(spec)
	require.NotNil(t, fn)
	assert.Equal(t, []string{"DHE-RSA-AES128-GCM-SHA256"}, unsupported)

	cfg := &tls.Config{}
	fn(cfg)
	assert.Len(t, cfg.CipherSuites, 1)
}

func TestConfigFromProfile_UnknownMinVersionFloorsToTLS12(t *testing.T) {
	t.Parallel()

	spec := configv1.TLSProfileSpec{
		MinTLSVersion: "VersionTLS99",
		Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
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
		Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
		Groups:        []configv1.TLSGroup{configv1.TLSGroupX25519, "not-a-group", configv1.TLSGroupSecP256r1MLKEM768},
	}
	fn, _ := pkgtls.ConfigFromProfile(spec)
	cfg := &tls.Config{}
	fn(cfg)
	assert.Equal(t, []tls.CurveID{tls.X25519}, cfg.CurvePreferences)
}

func TestConfigFromProfile_EmptyGroupsLeaveCurvePreferencesUnset(t *testing.T) {
	t.Parallel()

	spec := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
	}
	fn, _ := pkgtls.ConfigFromProfile(spec)
	cfg := &tls.Config{}
	fn(cfg)
	assert.Nil(t, cfg.CurvePreferences)
}
