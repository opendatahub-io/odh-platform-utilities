package template

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"

	gt "text/template"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/render"
	templateutils "github.com/opendatahub-io/odh-platform-utilities/pkg/template"
)

// errTemplateCacheDataMissing is returned when cacheKey runs without run()
// having set templateData on the action (internal invariant).
var errTemplateCacheDataMissing = errors.New("missing precomputed template data")

func (a *action) cacheKey(ctx context.Context, rr *render.ReconciliationRequest) ([]byte, error) {
	base, err := render.Hash(ctx, rr)
	if err != nil {
		return nil, err
	}

	if a.templateData == nil {
		return nil, fmt.Errorf("template cache key: %w", errTemplateCacheDataMissing)
	}

	hashData := maps.Clone(a.templateData)
	delete(hashData, ComponentKey)

	dataBytes, err := json.Marshal(hashData)
	if err != nil {
		return nil, fmt.Errorf("template cache key: marshal data: %w", err)
	}

	renderOptsDigest, err := digestTemplateRenderOpts(a.opts.renderOpts)
	if err != nil {
		return nil, err
	}

	h := sha256.New()

	_, err = h.Write(base)
	if err != nil {
		return nil, fmt.Errorf("template cache key: %w", err)
	}

	_, err = h.Write(dataBytes)
	if err != nil {
		return nil, fmt.Errorf("template cache key: %w", err)
	}

	_, err = h.Write(renderOptsDigest)
	if err != nil {
		return nil, fmt.Errorf("template cache key: %w", err)
	}

	return h.Sum(nil), nil
}

// digestTemplateRenderOpts fingerprints label/annotation injection and the
// text/template.FuncMap (each function's identity via reflect).
func digestTemplateRenderOpts(opts []Option) ([]byte, error) {
	o := options{
		labels:      make(map[string]string),
		annotations: make(map[string]string),
		funcMap:     templateutils.TextTemplateFuncMap(),
	}

	for _, opt := range opts {
		opt(&o)
	}

	h := sha256.New()

	lb, err := json.Marshal(o.labels)
	if err != nil {
		return nil, fmt.Errorf("template render opts: marshal labels: %w", err)
	}

	ab, err := json.Marshal(o.annotations)
	if err != nil {
		return nil, fmt.Errorf("template render opts: marshal annotations: %w", err)
	}

	_, err = h.Write(lb)
	if err != nil {
		return nil, err
	}

	_, err = h.Write(ab)
	if err != nil {
		return nil, err
	}

	err = writeTemplateFuncMapDigest(h, o.funcMap)
	if err != nil {
		return nil, err
	}

	return h.Sum(nil), nil
}

func writeTemplateFuncMapDigest(w io.Writer, funcMap gt.FuncMap) error {
	keys := slices.Sorted(maps.Keys(funcMap))

	for _, name := range keys {
		var err error

		_, err = fmt.Fprintf(w, "%s:", name)
		if err != nil {
			return err
		}

		fn := funcMap[name]
		rv := reflect.ValueOf(fn)

		var ptr uint64

		if rv.Kind() == reflect.Func {
			ptr = uint64(rv.Pointer())
		}

		err = binary.Write(w, binary.BigEndian, ptr)
		if err != nil {
			return err
		}

		_, err = w.Write([]byte("|"))
		if err != nil {
			return err
		}
	}

	return nil
}
