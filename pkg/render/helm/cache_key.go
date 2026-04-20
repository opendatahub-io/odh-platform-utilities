package helm

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	engineTypes "github.com/k8s-manifest-kit/engine/pkg/types"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/render"
)

func (a *action) cacheKey(ctx context.Context, rr *render.ReconciliationRequest) ([]byte, error) {
	base, err := render.Hash(ctx, rr)
	if err != nil {
		return nil, err
	}

	fp, err := digestRenderOpts(a.opts)
	if err != nil {
		return nil, err
	}

	h := sha256.New()

	_, err = h.Write(base)
	if err != nil {
		return nil, fmt.Errorf("helm cache key: %w", err)
	}

	_, err = h.Write(fp)
	if err != nil {
		return nil, fmt.Errorf("helm cache key: %w", err)
	}

	return h.Sum(nil), nil
}

// digestRenderOpts fingerprints label/annotation injection options and
// transformer identity. Helm's engine uses function-typed transformers; those
// are distinguished by reflect.Value.Pointer on the func value. Non-func values
// fall back to list index when pointer identity is not available.
func digestRenderOpts(opts []Option) ([]byte, error) {
	o := &options{}

	for _, opt := range opts {
		opt(o)
	}

	h := sha256.New()

	lb, err := json.Marshal(o.labels)
	if err != nil {
		return nil, fmt.Errorf("helm cache key: marshal labels: %w", err)
	}

	ab, err := json.Marshal(o.annotations)
	if err != nil {
		return nil, fmt.Errorf("helm cache key: marshal annotations: %w", err)
	}

	_, err = h.Write(lb)
	if err != nil {
		return nil, err
	}

	_, err = h.Write(ab)
	if err != nil {
		return nil, err
	}

	err = binary.Write(h, binary.BigEndian, uint64(len(o.transformers)))
	if err != nil {
		return nil, err
	}

	for i := range o.transformers {
		err = writeHelmTransformerID(h, i, o.transformers[i])
		if err != nil {
			return nil, err
		}
	}

	return h.Sum(nil), nil
}

func writeHelmTransformerID(w io.Writer, i int, tr engineTypes.Transformer) error {
	var err error

	_, err = fmt.Fprintf(w, "%T|", tr)
	if err != nil {
		return err
	}

	rv := reflect.ValueOf(tr)

	switch rv.Kind() {
	case reflect.Func:
		_, err = fmt.Fprintf(w, "0x%x|", rv.Pointer())
		if err != nil {
			return err
		}
	case reflect.Ptr:
		if !rv.IsNil() {
			_, err = fmt.Fprintf(w, "0x%x|", rv.Pointer())
			if err != nil {
				return err
			}
		}
	default:
		_, err = fmt.Fprintf(w, "idx:%d|", i)
		if err != nil {
			return err
		}
	}

	return nil
}
