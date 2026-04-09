package placeholder_test

import (
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/placeholder"
)

func TestVersion(t *testing.T) {
	t.Parallel()

	got := placeholder.Version()
	if got == "" {
		t.Error("Version() returned empty string")
	}
}
