package render

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash"
)

// Hash computes a SHA-256 hash of the ReconciliationRequest inputs that
// influence rendering output. It considers the Instance UID and generation,
// manifest paths, template paths (including per-template labels and
// annotations), and Helm chart identity+values.
// This is the default CachingKeyFn for action-pipeline renderers.
//
// ctx is passed to Helm chart Values loaders; use it for cancellation and deadlines.
func Hash(ctx context.Context, rr *ReconciliationRequest) ([]byte, error) {
	h := sha256.New()

	instanceGeneration := make([]byte, binary.MaxVarintLen64)
	binary.PutVarint(instanceGeneration, rr.Instance.GetGeneration())

	err := hashWrite(h, []byte(rr.Instance.GetUID()))
	if err != nil {
		return nil, fmt.Errorf("failed to hash instance: %w", err)
	}

	err = hashWrite(h, instanceGeneration)
	if err != nil {
		return nil, fmt.Errorf("failed to hash instance generation: %w", err)
	}

	err = hashManifests(h, rr)
	if err != nil {
		return nil, err
	}

	err = hashHelmCharts(ctx, h, rr)
	if err != nil {
		return nil, err
	}

	return h.Sum(nil), nil
}

func hashWrite(h hash.Hash, data []byte) error {
	_, err := h.Write(data)

	return err
}

func hashStringMap(h hash.Hash, m map[string]string) error {
	if len(m) == 0 {
		return hashWrite(h, []byte("{}"))
	}

	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to hash string map: %w", err)
	}

	return hashWrite(h, b)
}

func hashManifests(h hash.Hash, rr *ReconciliationRequest) error {
	for i := range rr.Manifests {
		err := hashWrite(h, []byte(rr.Manifests[i].String()))
		if err != nil {
			return fmt.Errorf("failed to hash manifest: %w", err)
		}
	}

	for i := range rr.Templates {
		err := hashWrite(h, []byte(rr.Templates[i].Path))
		if err != nil {
			return fmt.Errorf("failed to hash template path: %w", err)
		}

		err = hashStringMap(h, rr.Templates[i].Labels)
		if err != nil {
			return fmt.Errorf("failed to hash template labels: %w", err)
		}

		err = hashStringMap(h, rr.Templates[i].Annotations)
		if err != nil {
			return fmt.Errorf("failed to hash template annotations: %w", err)
		}
	}

	return nil
}

func hashHelmCharts(ctx context.Context, h hash.Hash, rr *ReconciliationRequest) error {
	for i := range rr.HelmCharts {
		err := hashWrite(h, []byte(rr.HelmCharts[i].Chart))
		if err != nil {
			return fmt.Errorf("failed to hash helm chart: %w", err)
		}

		err = hashWrite(h, []byte(rr.HelmCharts[i].ReleaseName))
		if err != nil {
			return fmt.Errorf("failed to hash helm chart release name: %w", err)
		}

		if rr.HelmCharts[i].Values == nil {
			continue
		}

		err = hashHelmValues(ctx, h, rr, i)
		if err != nil {
			return err
		}
	}

	return nil
}

func hashHelmValues(ctx context.Context, h hash.Hash, rr *ReconciliationRequest, idx int) error {
	values, err := rr.HelmCharts[idx].Values(ctx)
	if err != nil {
		return fmt.Errorf("failed to get helm chart values: %w", err)
	}

	b, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("failed to hash helm chart values: %w", err)
	}

	err = hashWrite(h, b)
	if err != nil {
		return fmt.Errorf("failed to hash helm chart values: %w", err)
	}

	return nil
}
