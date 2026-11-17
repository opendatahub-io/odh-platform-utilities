package common_test

import (
	"encoding/json"
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/api/common/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDatabaseConditionTypeConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, common.ConditionTypeDatabaseConnected, common.ConditionType("DatabaseConnected"))

	// The optional type must not collide with the mandatory and shared
	// types the orchestrator evaluates.
	for _, existing := range []common.ConditionType{
		common.ConditionTypeReady,
		common.ConditionTypeProvisioningSucceeded,
		common.ConditionTypeDegraded,
	} {
		assert.NotEqual(t, common.ConditionTypeDatabaseConnected, existing)
	}
}

func TestDatabaseReasonConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		reason string
		want   string
	}{
		{common.ReasonConnectionSucceeded, "ConnectionSucceeded"},
		{common.ReasonConnectionFailed, "ConnectionFailed"},
		{common.ReasonCredentialsUnavailable, "CredentialsUnavailable"},
		{common.ReasonCredentialsRejected, "CredentialsRejected"},
		{common.ReasonHostUnreachable, "HostUnreachable"},
		{common.ReasonTLSVerificationFailed, "TLSVerificationFailed"},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			// Exact values are contract.
			assert.Equal(t, tc.want, tc.reason)

			// Reason is documented as a CamelCase programmatic identifier.
			assert.Regexp(t, `^[A-Z][A-Za-z0-9]*$`, tc.reason)
		})
	}
}

func TestDatabaseConnectedConditionJSONRoundTrip(t *testing.T) {
	t.Parallel()

	status := common.Status{
		Conditions: []common.Condition{
			{
				Type:    string(common.ConditionTypeDatabaseConnected),
				Status:  metav1.ConditionFalse,
				Reason:  common.ReasonHostUnreachable,
				Message: "dial tcp 10.0.0.5:5432: connect: connection refused",
			},
		},
	}

	data, err := json.Marshal(status)
	require.NoError(t, err)

	var got common.Status
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, status, got)
}

func TestDatabaseConnectedConditionPlatformObjectContract(t *testing.T) {
	t.Parallel()

	obj := &testComponent{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "model-registry",
			Generation: 1,
		},
	}

	obj.SetConditions([]common.Condition{
		{
			Type:   string(common.ConditionTypeDatabaseConnected),
			Status: metav1.ConditionTrue,
			Reason: common.ReasonConnectionSucceeded,
		},
	})

	// The optional condition type must not interfere with the mandatory
	// PlatformObject contract checks.
	validation.ValidatePlatformObject(t, obj)

	conditions := obj.GetConditions()
	require.Len(t, conditions, 1)
	assert.Equal(t, string(common.ConditionTypeDatabaseConnected), conditions[0].Type)
	assert.Equal(t, common.ReasonConnectionSucceeded, conditions[0].Reason)
}
