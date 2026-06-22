package precondition

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/action"
)

// ErrEmptyGVKList is returned when MonitorCRDs is called with an empty
// GroupVersionKind slice.
var ErrEmptyGVKList = errors.New("MonitorCRDs called with empty GroupVersionKind list")

// MonitorCRD creates a PreCondition that checks for the presence of a
// single CRD via the client's RESTMapper (single-shot, no retry).
func MonitorCRD(gvk schema.GroupVersionKind, opts ...Option) PreCondition {
	return MonitorCRDs([]schema.GroupVersionKind{gvk}, opts...)
}

// MonitorCRDs creates a PreCondition that checks for the presence of
// multiple CRDs via the client's RESTMapper (single-shot, no retry).
// All CRDs are checked and absent ones are reported together in a
// single failure message.
func MonitorCRDs(gvks []schema.GroupVersionKind, opts ...Option) PreCondition {
	monitored := slices.Clone(gvks)

	return newPreCondition(func(_ context.Context, rr *action.ReconciliationRequest) (CheckResult, error) {
		if len(monitored) == 0 {
			return CheckResult{}, ErrEmptyGVKList
		}

		mapper := rr.Client.RESTMapper()

		var missing []string

		for _, gvk := range monitored {
			_, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
			if err != nil {
				if meta.IsNoMatchError(err) {
					missing = append(missing, fmt.Sprintf("%s.%s: CRD not found", gvk.Kind, gvk.Group))

					continue
				}

				return CheckResult{}, fmt.Errorf(
					"%s.%s: failed to check CRD presence: %w",
					gvk.Kind, gvk.Group, err,
				)
			}
		}

		if len(missing) > 0 {
			return CheckResult{
				Pass:    false,
				Message: strings.Join(missing, "; "),
			}, nil
		}

		return CheckResult{Pass: true}, nil
	}, opts...)
}
