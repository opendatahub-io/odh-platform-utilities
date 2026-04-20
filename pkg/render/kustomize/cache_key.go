package kustomize

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/render"
)

func (a *action) cacheKey(ctx context.Context, rr *render.ReconciliationRequest) ([]byte, error) {
	base, err := render.Hash(ctx, rr)
	if err != nil {
		return nil, err
	}

	ns := a.namespace

	if a.nsFn != nil {
		ns, err = a.nsFn(ctx)
		if err != nil {
			return nil, fmt.Errorf("kustomize cache key: namespace fn: %w", err)
		}
	}

	h := sha256.New()

	_, err = h.Write(base)
	if err != nil {
		return nil, fmt.Errorf("kustomize cache key: %w", err)
	}

	_, err = h.Write([]byte(ns))
	if err != nil {
		return nil, fmt.Errorf("kustomize cache key: %w", err)
	}

	return h.Sum(nil), nil
}
