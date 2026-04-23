package deploy_test

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/deploy"

	. "github.com/onsi/gomega"
)

func TestCacheAddAndHas(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	c := deploy.NewCache()

	original := newCacheObj("100")
	modified := newCacheObj("")

	err := c.Add(original, modified)
	g.Expect(err).ShouldNot(HaveOccurred())

	found, err := c.Has(original, modified)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(found).Should(BeTrue())
}

func TestCacheHasMissOnDifferentContent(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	c := deploy.NewCache()

	original := newCacheObj("100")
	modified1 := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{"name": "cm-1", "namespace": "default"},
		"data":     map[string]any{"key": "value-a"},
	}}
	modified2 := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{"name": "cm-1", "namespace": "default"},
		"data":     map[string]any{"key": "value-b"},
	}}

	err := c.Add(original, modified1)
	g.Expect(err).ShouldNot(HaveOccurred())

	found, err := c.Has(original, modified2)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(found).Should(BeFalse())
}

func TestCacheHasMissOnDifferentResourceVersion(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	c := deploy.NewCache()

	original := newCacheObj("100")
	modified := newCacheObj("")

	err := c.Add(original, modified)
	g.Expect(err).ShouldNot(HaveOccurred())

	differentRV := newCacheObj("200")
	found, err := c.Has(differentRV, modified)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(found).Should(BeFalse())
}

func TestCacheDelete(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	c := deploy.NewCache()

	original := newCacheObj("100")
	modified := newCacheObj("")

	err := c.Add(original, modified)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = c.Delete(original, modified)
	g.Expect(err).ShouldNot(HaveOccurred())

	found, err := c.Has(original, modified)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(found).Should(BeFalse())
}

func TestCacheNilInputs(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	c := deploy.NewCache()

	found, err := c.Has(nil, nil)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(found).Should(BeFalse())

	err = c.Add(nil, nil)
	g.Expect(err).Should(HaveOccurred())

	err = c.Delete(nil, nil)
	g.Expect(err).ShouldNot(HaveOccurred())
}

func TestCacheTTLExpiry(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	c := deploy.NewCache(deploy.WithTTL(100 * time.Millisecond))

	original := newCacheObj("100")
	modified := newCacheObj("")

	err := c.Add(original, modified)
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Eventually(func() (bool, error) {
		c.Sync()

		return c.Has(original, modified)
	}).WithTimeout(2 * time.Second).WithPolling(200 * time.Millisecond).Should(BeFalse())
}

func newCacheObj(resourceVersion string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm-1", "namespace": "default"},
	}}

	if resourceVersion != "" {
		obj.SetResourceVersion(resourceVersion)
	}

	return obj
}
