// framework/testing/integration/harness_test.go
package integration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/framework/testing/integration"
)

func TestDSCSpec_ToMap(t *testing.T) {
	t.Parallel()

	spec := integration.NewDSCSpec().
		Component(integration.Dashboard, integration.Managed).
		Component(integration.Kserve, integration.Managed,
			integration.Sub("nim", integration.Managed),
			integration.Set("rawDeploymentServiceConfig", "Headed"),
		)

	m := spec.ToMap()
	require.Contains(t, m, "components")

	components, ok := m["components"].(map[string]any)
	require.True(t, ok, "components must be map[string]any")

	assert.Contains(t, components, "dashboard")
	assert.Contains(t, components, "kserve")

	kserve, ok := components["kserve"].(map[string]any)
	require.True(t, ok, "kserve must be map[string]any")
	assert.Equal(t, "Managed", kserve["managementState"])
	assert.Equal(t, map[string]any{"managementState": "Managed"}, kserve["nim"])
	assert.Equal(t, "Headed", kserve["rawDeploymentServiceConfig"])
}

func TestDSCSpec_EmptyReturnsNil(t *testing.T) {
	t.Parallel()

	spec := integration.NewDSCSpec()
	assert.Nil(t, spec.ToMap())
}
