package precondition

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/action"
)

// ErrEmptyGroupKindList is returned when MonitorCRDs is called with an empty GroupKind slice.
var ErrEmptyGroupKindList = errors.New("MonitorCRDs called with empty GroupKind list")

// MonitorCRD creates a PreCondition that checks for the presence of a single CRD.
// It delegates to [cluster.HasCRD], which retries with backoff (~5 s per absent CRD).
// When checking many CRDs, prefer [MonitorCRDs] — it reports all absent CRDs in
// one message but checks them sequentially.
func MonitorCRD(gk schema.GroupKind, opts ...Option) PreCondition {
	return MonitorCRDs([]schema.GroupKind{gk}, opts...)
}

// MonitorCRDs creates a PreCondition that checks for the presence of multiple CRDs.
// All CRDs are checked and absent ones are reported together in a single failure message.
// The first API error encountered is returned immediately.
//
// Each CRD check uses [cluster.HasCRD] which retries with backoff (~5 s per absent CRD).
// Checks run sequentially, so N absent CRDs add ~5·N seconds of latency.
func MonitorCRDs(gks []schema.GroupKind, opts ...Option) PreCondition {
	monitoredGKs := slices.Clone(gks)

	return newPreCondition(func(ctx context.Context, rr *action.ReconciliationRequest) (CheckResult, error) {
		if len(monitoredGKs) == 0 {
			return CheckResult{}, ErrEmptyGroupKindList
		}

		var missing []string

		for _, gk := range monitoredGKs {
			has, err := cluster.HasCRD(ctx, rr.Client, gk)
			if err != nil {
				return CheckResult{}, fmt.Errorf("%s: failed to check CRD presence: %w", gk.Kind, err)
			}

			if !has {
				missing = append(missing, gk.Kind+": CRD not found")
			}
		}

		if len(missing) > 0 {
			return CheckResult{Pass: false, Message: strings.Join(missing, "; ")}, nil
		}

		return CheckResult{Pass: true}, nil
	}, opts...)
}
