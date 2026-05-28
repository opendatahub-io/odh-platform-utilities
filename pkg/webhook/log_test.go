package webhook_test

import (
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/webhook"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestNewWebhookLogConstructorWithRequest(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	base := logr.New(&captureLogSink{})
	req := &admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Namespace: "test-ns",
			Name:      "test-name",
			Operation: admissionv1.Create,
			Kind: metav1.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			},
			Resource: metav1.GroupVersionResource{
				Group:    "apps",
				Version:  "v1",
				Resource: "deployments",
			},
			UID: "test-uid",
		},
	}

	logger := webhook.NewWebhookLogConstructor("my-webhook")(base, req)
	sink, ok := logger.GetSink().(*captureLogSink)
	g.Expect(ok).Should(BeTrue())

	values := sink.ValuesByKey()
	g.Expect(values).Should(HaveKeyWithValue("webhook", "my-webhook"))
	g.Expect(values).Should(HaveKeyWithValue("namespace", "test-ns"))
	g.Expect(values).Should(HaveKeyWithValue("name", "test-name"))
	g.Expect(values).Should(HaveKeyWithValue("operation", admissionv1.Create))
	g.Expect(values).Should(HaveKeyWithValue("kind", "Deployment"))
	g.Expect(values).Should(HaveKey("resource"))
	g.Expect(values).Should(HaveKeyWithValue("requestID", req.UID))
}

func TestNewWebhookLogConstructorWithoutRequest(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	base := logr.New(&captureLogSink{})

	logger := webhook.NewWebhookLogConstructor("my-webhook")(base, nil)
	sink, ok := logger.GetSink().(*captureLogSink)
	g.Expect(ok).Should(BeTrue())

	values := sink.ValuesByKey()
	g.Expect(values).Should(Equal(map[string]any{
		"webhook": "my-webhook",
	}))
}

type captureLogSink struct {
	values []any
}

func (s *captureLogSink) Init(logr.RuntimeInfo) {}

func (s *captureLogSink) Enabled(int) bool {
	return true
}

func (s *captureLogSink) Info(int, string, ...any) {}

func (s *captureLogSink) Error(error, string, ...any) {}

func (s *captureLogSink) WithValues(keysAndValues ...any) logr.LogSink {
	values := append([]any{}, s.values...)
	values = append(values, keysAndValues...)

	return &captureLogSink{values: values}
}

func (s *captureLogSink) WithName(string) logr.LogSink {
	return &captureLogSink{values: append([]any{}, s.values...)}
}

func (s *captureLogSink) ValuesByKey() map[string]any {
	valuesByKey := make(map[string]any, len(s.values)/2)
	for i := 0; i+1 < len(s.values); i += 2 {
		key, ok := s.values[i].(string)
		if !ok {
			continue
		}

		valuesByKey[key] = s.values[i+1]
	}

	return valuesByKey
}
