package resources_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"

	. "github.com/onsi/gomega"
)

func newDecoder() runtime.Decoder {
	scheme := runtime.NewScheme()

	return serializer.NewCodecFactory(scheme).UniversalDeserializer()
}

func TestDecodeSingleDocument(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	content := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
  namespace: default
data:
  key: value
`)
	result, err := resources.Decode(newDecoder(), content)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result).Should(HaveLen(1))
	g.Expect(result[0].GetKind()).Should(Equal("ConfigMap"))
	g.Expect(result[0].GetName()).Should(Equal("test-cm"))
	g.Expect(result[0].GetNamespace()).Should(Equal("default"))
}

func TestDecodeMultiDocument(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	content := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-one
---
apiVersion: v1
kind: Service
metadata:
  name: svc-one
`)
	result, err := resources.Decode(newDecoder(), content)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result).Should(HaveLen(2))
	g.Expect(result[0].GetKind()).Should(Equal("ConfigMap"))
	g.Expect(result[1].GetKind()).Should(Equal("Service"))
}

func TestDecodeSkipsEmptyDocuments(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	content := []byte(`---
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: after-empty
---
`)
	result, err := resources.Decode(newDecoder(), content)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result).Should(HaveLen(1))
	g.Expect(result[0].GetName()).Should(Equal("after-empty"))
}

func TestDecodeSkipsDocumentsWithoutKind(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	content := []byte(`apiVersion: v1
metadata:
  name: no-kind
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: has-kind
`)
	result, err := resources.Decode(newDecoder(), content)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result).Should(HaveLen(1))
	g.Expect(result[0].GetName()).Should(Equal("has-kind"))
}

func TestDecodeEmptyInput(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	result, err := resources.Decode(newDecoder(), []byte(""))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result).Should(BeEmpty())
}

func TestDecodeInvalidYAML(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	content := []byte(`{{{invalid yaml`)
	_, err := resources.Decode(newDecoder(), content)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("unable to decode resource"))
}

func TestToUnstructuredSuccess(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	src := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "test",
				"namespace": "ns",
			},
		},
	}

	result, err := resources.ToUnstructured(src)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.GetKind()).Should(Equal("ConfigMap"))
	g.Expect(result.GetName()).Should(Equal("test"))
}

func TestToUnstructuredError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	src := map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
	}

	_, err := resources.ToUnstructured(src)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("unable to convert object"))
}
