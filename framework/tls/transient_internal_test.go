package tls

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/url"
	"testing"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsTransientAPIError_TransportErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		transient bool
	}{
		{
			name:      "connection reset is transient",
			err:       &net.OpError{Op: "dial", Err: errors.New("connection reset by peer")},
			transient: true,
		},
		{
			name:      "DNS no such host is transient",
			err:       &net.DNSError{Err: "no such host", Name: "api.example.com"},
			transient: true,
		},
		{
			name:      "deadline exceeded is transient",
			err:       context.DeadlineExceeded,
			transient: true,
		},
		{
			name:      "forbidden is not transient",
			err:       k8serr.NewForbidden(schema.GroupResource{}, "test", errors.New("denied")),
			transient: false,
		},
		{
			name:      "nil error is not transient",
			err:       nil,
			transient: false,
		},
		{
			name:      "unexpected EOF from incomplete response body is transient",
			err:       io.ErrUnexpectedEOF,
			transient: true,
		},
		{
			name:      "certificate verification failure wrapped in url.Error is not transient",
			err:       &url.Error{Op: "Get", URL: "https://api.example.com", Err: x509.UnknownAuthorityError{}},
			transient: false,
		},
		{
			name:      "tls record header error is not transient",
			err:       &url.Error{Op: "Get", URL: "https://api.example.com", Err: errors.New("tls: bad record MAC")},
			transient: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isTransientAPIError(tt.err)
			if got != tt.transient {
				t.Errorf("isTransientAPIError(%v) = %v, want %v", tt.err, got, tt.transient)
			}
		})
	}
}
