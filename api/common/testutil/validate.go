package testutil

import (
	"errors"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
)

var (
	errNilStatus        = errors.New("GetStatus() returned nil")
	errCondRoundTrip    = errors.New("conditions round-trip failed")
	errMandatoryConds   = errors.New("mandatory condition types")
	errNilReleaseStatus = errors.New("GetReleaseStatus() returned nil")
	errRelRoundTrip     = errors.New("release-status round-trip failed")
	errPhaseField       = errors.New("phase field check failed")
)

// ValidatePlatformObject verifies that obj satisfies the PlatformObject
// contract expected by the ODH orchestrator. It checks:
//   - GetStatus() returns non-nil
//   - GetConditions()/SetConditions() round-trip correctly
//   - GetReleaseStatus()/SetReleaseStatus() round-trip correctly
//   - Mandatory condition types (Ready, ProvisioningSucceeded, Degraded)
//     can be set and retrieved
//   - Phase field accepts Ready and Not Ready values
//
// Each sub-check is run as a named subtest so failures pinpoint the exact
// contract requirement that is violated.
func ValidatePlatformObject(t *testing.T, obj common.PlatformObject) {
	t.Helper()

	t.Run("GetStatus", func(t *testing.T) {
		t.Helper()

		err := checkGetStatus(obj)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ConditionsRoundTrip", func(t *testing.T) {
		t.Helper()

		err := checkConditionsRoundTrip(obj)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("MandatoryConditionTypes", func(t *testing.T) {
		t.Helper()

		err := checkMandatoryConditionTypes(obj)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ReleaseStatusRoundTrip", func(t *testing.T) {
		t.Helper()

		err := checkReleaseStatusRoundTrip(obj)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("PhaseValues", func(t *testing.T) {
		t.Helper()

		err := checkPhaseValues(obj)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func checkGetStatus(obj common.PlatformObject) error {
	if obj.GetStatus() == nil {
		return errNilStatus
	}

	return nil
}

func checkConditionsRoundTrip(obj common.PlatformObject) error {
	original := obj.GetConditions()

	defer obj.SetConditions(original)

	conditions := []common.Condition{
		{
			Type:               "testutil-roundtrip",
			Status:             metav1.ConditionTrue,
			Reason:             "Testing",
			LastTransitionTime: metav1.Now(),
		},
	}

	obj.SetConditions(conditions)

	got := obj.GetConditions()
	if len(got) != 1 {
		return fmt.Errorf(
			"%w: set 1 condition, got %d back",
			errCondRoundTrip, len(got),
		)
	}

	if got[0].Type != "testutil-roundtrip" {
		return fmt.Errorf(
			"%w: expected type %q, got %q",
			errCondRoundTrip, "testutil-roundtrip", got[0].Type,
		)
	}

	if got[0].Status != metav1.ConditionTrue {
		return fmt.Errorf(
			"%w: expected status %q, got %q",
			errCondRoundTrip, metav1.ConditionTrue, got[0].Status,
		)
	}

	return nil
}

func checkMandatoryConditionTypes(obj common.PlatformObject) error {
	original := obj.GetConditions()

	defer obj.SetConditions(original)

	mandatoryTypes := []common.ConditionType{
		common.ConditionTypeReady,
		common.ConditionTypeProvisioningSucceeded,
		common.ConditionTypeDegraded,
	}

	conditions := make([]common.Condition, 0, len(mandatoryTypes))
	for _, ct := range mandatoryTypes {
		conditions = append(conditions, common.Condition{
			Type:               string(ct),
			Status:             metav1.ConditionUnknown,
			Reason:             "ContractTest",
			LastTransitionTime: metav1.Now(),
		})
	}

	obj.SetConditions(conditions)

	got := obj.GetConditions()
	if len(got) != len(mandatoryTypes) {
		return fmt.Errorf(
			"%w: set %d conditions, got %d back",
			errMandatoryConds, len(mandatoryTypes), len(got),
		)
	}

	gotTypes := make(map[string]bool, len(got))
	for _, c := range got {
		gotTypes[c.Type] = true
	}

	for _, ct := range mandatoryTypes {
		if !gotTypes[string(ct)] {
			return fmt.Errorf(
				"%w: type %q was set but not found",
				errMandatoryConds, ct,
			)
		}
	}

	return nil
}

func checkReleaseStatusRoundTrip(obj common.PlatformObject) error {
	original := obj.GetReleaseStatus()
	if original == nil {
		return errNilReleaseStatus
	}

	defer obj.SetReleaseStatus(*original)

	releases := common.ComponentReleaseStatus{
		Releases: []common.ComponentRelease{
			{Name: "testutil-component", Version: "v0.0.1"},
		},
	}

	obj.SetReleaseStatus(releases)

	got := obj.GetReleaseStatus()
	if got == nil {
		return fmt.Errorf(
			"%w: returned nil after SetReleaseStatus()",
			errRelRoundTrip,
		)
	}

	if len(got.Releases) != 1 {
		return fmt.Errorf(
			"%w: set 1 release, got %d back",
			errRelRoundTrip, len(got.Releases),
		)
	}

	if got.Releases[0].Name != "testutil-component" {
		return fmt.Errorf(
			"%w: expected name %q, got %q",
			errRelRoundTrip, "testutil-component",
			got.Releases[0].Name,
		)
	}

	if got.Releases[0].Version != "v0.0.1" {
		return fmt.Errorf(
			"%w: expected version %q, got %q",
			errRelRoundTrip, "v0.0.1",
			got.Releases[0].Version,
		)
	}

	return nil
}

func checkPhaseValues(obj common.PlatformObject) error {
	status := obj.GetStatus()
	if status == nil {
		return errNilStatus
	}

	originalPhase := status.Phase

	defer func() { status.Phase = originalPhase }()

	status.Phase = common.PhaseReady
	if obj.GetStatus().Phase != common.PhaseReady {
		return fmt.Errorf(
			"%w: does not accept %q, got %q",
			errPhaseField, common.PhaseReady,
			obj.GetStatus().Phase,
		)
	}

	status.Phase = common.PhaseNotReady
	if obj.GetStatus().Phase != common.PhaseNotReady {
		return fmt.Errorf(
			"%w: does not accept %q, got %q",
			errPhaseField, common.PhaseNotReady,
			obj.GetStatus().Phase,
		)
	}

	return nil
}
