package validation

import (
	"errors"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
)

var (
	ErrNilStatus = errors.New(
		"status pointer must not be nil:" +
			" ensure it returns a pointer to the actual Status field",
	)
	ErrCondRoundTrip = errors.New(
		"conditions not persisted:" +
			" ensure set stores conditions so get returns them",
	)
	ErrMandatoryConds = errors.New(
		"mandatory condition type missing:" +
			" Ready and ProvisioningSucceeded must both be storable",
	)
	ErrNilReleaseStatus = errors.New(
		"release status pointer must not be nil:" +
			" ensure it returns a pointer to the actual ComponentReleaseStatus field",
	)
	ErrRelRoundTrip = errors.New(
		"release status not persisted:" +
			" ensure set stores releases so get returns them",
	)
	ErrPhaseField = errors.New(
		"phase not writable through status pointer:" +
			" ensure the status accessor returns a pointer to the actual field, not a copy",
	)
	ErrNilObject = errors.New(
		"PlatformObject must not be nil",
	)
)

// Validate checks whether obj satisfies the PlatformObject behavioral
// contract. Returns a combined error describing all violations, or nil
// if the contract is satisfied.
func Validate(obj common.PlatformObject) error {
	if obj == nil {
		return ErrNilObject
	}

	checks := []func(common.PlatformObject) error{
		checkGetStatus,
		checkConditionsRoundTrip,
		checkMandatoryConditionTypes,
		checkReleaseStatusRoundTrip,
		checkPhaseValues,
	}

	var errs []error

	for _, check := range checks {
		err := check(obj)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// ValidatePlatformObject is a test helper that runs [Validate] and fails
// the test if any contract violation is found.
func ValidatePlatformObject(t *testing.T, obj common.PlatformObject) {
	t.Helper()

	err := Validate(obj)
	if err != nil {
		t.Fatal(err)
	}
}

func checkGetStatus(obj common.PlatformObject) error {
	if obj.GetStatus() == nil {
		return ErrNilStatus
	}

	return nil
}

// Verifies that conditions are actually persisted, not silently dropped.
func checkConditionsRoundTrip(obj common.PlatformObject) error {
	original := obj.GetConditions()

	// Restore original state so the validator doesn't leave side effects.
	defer obj.SetConditions(original)

	conditions := []common.Condition{
		{
			Type:               "validation-roundtrip",
			Status:             metav1.ConditionTrue,
			Reason:             "Validation",
			LastTransitionTime: metav1.Now(),
		},
	}

	obj.SetConditions(conditions)

	got := obj.GetConditions()
	if len(got) != 1 {
		return fmt.Errorf(
			"%w: set 1 condition, got %d back",
			ErrCondRoundTrip, len(got),
		)
	}

	if got[0].Type != "validation-roundtrip" {
		return fmt.Errorf(
			"%w: expected type %q, got %q",
			ErrCondRoundTrip, "validation-roundtrip", got[0].Type,
		)
	}

	if got[0].Status != metav1.ConditionTrue {
		return fmt.Errorf(
			"%w: expected status %q, got %q",
			ErrCondRoundTrip, metav1.ConditionTrue, got[0].Status,
		)
	}

	return nil
}

// Verifies that all mandatory condition types can be stored and retrieved.
func checkMandatoryConditionTypes(obj common.PlatformObject) error {
	original := obj.GetConditions()

	// Restore original state so the validator doesn't leave side effects.
	defer obj.SetConditions(original)

	mandatoryTypes := []common.ConditionType{
		common.ConditionTypeReady,
		common.ConditionTypeProvisioningSucceeded,
	}

	conditions := make([]common.Condition, 0, len(mandatoryTypes))
	for _, ct := range mandatoryTypes {
		conditions = append(conditions, common.Condition{
			Type:               string(ct),
			Status:             metav1.ConditionUnknown,
			Reason:             "ContractValidation",
			LastTransitionTime: metav1.Now(),
		})
	}

	obj.SetConditions(conditions)

	got := obj.GetConditions()
	if len(got) != len(mandatoryTypes) {
		return fmt.Errorf(
			"%w: set %d conditions, got %d back",
			ErrMandatoryConds, len(mandatoryTypes), len(got),
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
				ErrMandatoryConds, ct,
			)
		}
	}

	return nil
}

// Verifies that release status is actually persisted, not silently dropped.
func checkReleaseStatusRoundTrip(obj common.PlatformObject) error {
	originalPtr := obj.GetReleaseStatus()
	if originalPtr == nil {
		return ErrNilReleaseStatus
	}

	// Snapshot the value before mutation to avoid restoring mutated data.
	original := *originalPtr

	// Restore original state so the validator doesn't leave side effects.
	defer obj.SetReleaseStatus(original)

	releases := common.ComponentReleaseStatus{
		Releases: []common.ComponentRelease{
			{Name: "validation-component", Version: "v0.0.1"},
		},
	}

	obj.SetReleaseStatus(releases)

	got := obj.GetReleaseStatus()
	if got == nil {
		return fmt.Errorf(
			"%w: returned nil after storing releases",
			ErrRelRoundTrip,
		)
	}

	if len(got.Releases) != 1 {
		return fmt.Errorf(
			"%w: set 1 release, got %d back",
			ErrRelRoundTrip, len(got.Releases),
		)
	}

	if got.Releases[0].Name != "validation-component" {
		return fmt.Errorf(
			"%w: expected name %q, got %q",
			ErrRelRoundTrip, "validation-component",
			got.Releases[0].Name,
		)
	}

	if got.Releases[0].Version != "v0.0.1" {
		return fmt.Errorf(
			"%w: expected version %q, got %q",
			ErrRelRoundTrip, "v0.0.1",
			got.Releases[0].Version,
		)
	}

	return nil
}

// Verifies that the status accessor returns a pointer, not a copy.
func checkPhaseValues(obj common.PlatformObject) error {
	status := obj.GetStatus()
	if status == nil {
		return ErrNilStatus
	}

	originalPhase := status.Phase

	// Restore original state so the validator doesn't leave side effects.
	defer func() { status.Phase = originalPhase }()

	status.Phase = common.PhaseReady
	if obj.GetStatus().Phase != common.PhaseReady {
		return fmt.Errorf(
			"%w: does not accept %q, got %q",
			ErrPhaseField, common.PhaseReady,
			obj.GetStatus().Phase,
		)
	}

	status.Phase = common.PhaseNotReady
	if obj.GetStatus().Phase != common.PhaseNotReady {
		return fmt.Errorf(
			"%w: does not accept %q, got %q",
			ErrPhaseField, common.PhaseNotReady,
			obj.GetStatus().Phase,
		)
	}

	return nil
}
