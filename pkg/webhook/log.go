package webhook

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NewWebhookLogConstructor returns a log constructor that preserves the default
// admission request fields and adds the webhook name to each logger.
func NewWebhookLogConstructor(name string) func(logr.Logger, *admission.Request) logr.Logger {
	return func(base logr.Logger, req *admission.Request) logr.Logger {
		l := admission.DefaultLogConstructor(base, req)
		if req == nil {
			return l.WithValues("webhook", name)
		}

		return l.WithValues(
			"webhook", name,
			"namespace", req.Namespace,
			"name", req.Name,
			"operation", req.Operation,
			"kind", req.Kind.Kind,
		)
	}
}
