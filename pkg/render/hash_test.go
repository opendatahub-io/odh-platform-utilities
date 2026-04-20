package render_test

import (
	"testing"

	"github.com/rs/xid"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/render"

	. "github.com/onsi/gomega"
)

func baseInstance(name, uid string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": name,
				"uid":  uid,
			},
		},
	}
}

//nolint:paralleltest
func TestHashChangesWithTemplateLabels(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	uid := xid.New().String()
	inst := baseInstance("a", uid)

	rr1 := &render.ReconciliationRequest{
		Instance: inst,
		Templates: []render.TemplateInfo{{
			Path:   "x.tmpl.yaml",
			Labels: map[string]string{"a": "1"},
		}},
	}

	rr2 := &render.ReconciliationRequest{
		Instance: inst,
		Templates: []render.TemplateInfo{{
			Path:   "x.tmpl.yaml",
			Labels: map[string]string{"a": "2"},
		}},
	}

	h1, err := render.Hash(ctx, rr1)
	g.Expect(err).ShouldNot(HaveOccurred())

	h2, err := render.Hash(ctx, rr2)
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(h1).ShouldNot(BeEquivalentTo(h2))
}

//nolint:paralleltest
func TestHashStableForSameInputs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	uid := xid.New().String()
	inst := baseInstance("a", uid)

	rr := &render.ReconciliationRequest{
		Instance: inst,
		Templates: []render.TemplateInfo{{
			Path:   "x.tmpl.yaml",
			Labels: map[string]string{"k": "v"},
		}},
	}

	h1, err := render.Hash(ctx, rr)
	g.Expect(err).ShouldNot(HaveOccurred())

	h2, err := render.Hash(ctx, rr)
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(h1).Should(Equal(h2))
}
