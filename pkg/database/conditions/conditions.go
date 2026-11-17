// Package conditions provides helpers for reporting database connectivity on
// a component's own custom resource through the standard condition type
// ConditionTypeDatabaseConnected and its reason vocabulary from api/common.
//
// The helpers are independent of the condition Manager in
// pkg/controller/conditions. They upsert conditions directly on any
// common.ConditionsAccessor — typically a module CR embedding common.Status
// — and return whether the condition changed. Components that use a Manager
// should instead set the condition through the Manager's MarkTrue and
// MarkFalse with the same constants from api/common, so that aggregation
// into the Ready condition happens; see the worked example in
// examples/database-connection-condition.
//
// Persistence is the caller's responsibility: mutate the object in memory
// with these helpers, then write the status subresource with pkg/status.
package conditions

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	kcond "github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
)

// redactedPlaceholder replaces masked password material in redacted
// messages.
const redactedPlaceholder = "REDACTED"

var (
	// passwordParamPattern matches password key/value parameters as they
	// appear in libpq-style DSNs and driver error messages, including
	// quoted values with escapes and the sslpassword key
	// (password=secret, password='tw0 words', password='tw0\'s words',
	// sslpassword=secret). The key itself is captured so the replacement
	// preserves it. Unquoted values consume the complete non-whitespace
	// token, honouring backslash escapes, so characters such as ; and &
	// are password material, not separators. A value with an opening
	// quote but no closing quote masks the rest of the string: when the
	// end of the value cannot be determined, redaction fails closed.
	passwordParamPattern = regexp.MustCompile(
		`(?i)\b((?:ssl)?password)\s*=\s*` +
			`('(?:[^'\\]|\\.|'')*'?|"(?:[^"\\]|\\.)*"?|(?:[^\\\s]|\\.)*)`)

	// uriPasswordPattern matches credentials embedded in a URI
	// (scheme://user:password@host), including an empty user
	// (scheme://:password@host) and passwords containing a slash. A
	// password containing an unencoded @ cannot be fully masked.
	uriPasswordPattern = regexp.MustCompile(`(?i)([a-z][a-z0-9+.\-]*)://([^:/@\s]*):([^@\s]+)@`)
)

// SetConnected upserts a DatabaseConnected=True condition on accessor,
// reporting a successful database connection.
//
// The reason defaults to common.ReasonConnectionSucceeded. WithReason
// overrides it; the generic reason set by WithError is replaced by the
// default. opts can otherwise refine the condition with the options from
// pkg/controller/conditions, for example WithMessage to record what was
// connected to, or WithSeverity to report connectivity as informational.
// The final message is always passed through Redact. Returns true if the
// condition changed.
//
// The upsert replaces the condition wholesale: pass WithObservedGeneration
// on every call, or a previously stamped value is cleared.
func SetConnected(accessor common.ConditionsAccessor, opts ...kcond.Option) bool {
	condition := common.Condition{
		Type:   string(common.ConditionTypeDatabaseConnected),
		Status: metav1.ConditionTrue,
	}

	for _, opt := range opts {
		opt(&condition)
	}

	if condition.Reason == "" || condition.Reason == kcond.ConditionReasonError {
		condition.Reason = common.ReasonConnectionSucceeded
	}

	condition.Message = Redact(condition.Message)

	return kcond.SetStatusCondition(accessor, condition)
}

// SetFailed upserts a DatabaseConnected=False condition on accessor,
// reporting a failed database connection.
//
// The reason defaults to the classification from ReasonForError. WithReason
// overrides it; the generic reason set by WithError is replaced by the
// classification. The default message is err.Error(), and the final message
// is always passed through Redact, including messages supplied through
// options such as WithMessage or WithError.
//
// Authentication failures cannot be detected through standard library
// inspection: when the caller knows the server rejected the credentials,
// pass common.ReasonCredentialsRejected via WithReason.
//
// The upsert replaces the condition wholesale: pass WithObservedGeneration
// on every call, or a previously stamped value is cleared.
func SetFailed(accessor common.ConditionsAccessor, err error, opts ...kcond.Option) bool {
	condition := common.Condition{
		Type:   string(common.ConditionTypeDatabaseConnected),
		Status: metav1.ConditionFalse,
	}

	if err != nil {
		condition.Message = err.Error()
	}

	for _, opt := range opts {
		opt(&condition)
	}

	if condition.Reason == "" || condition.Reason == kcond.ConditionReasonError {
		condition.Reason = ReasonForError(err)
	}

	condition.Message = Redact(condition.Message)

	return kcond.SetStatusCondition(accessor, condition)
}

// ReasonForError maps a database connection error to a standard reason from
// api/common using standard library inspection only:
//
//   - TLS errors (x509 certificate and hostname errors,
//     tls.RecordHeaderError) map to common.ReasonTLSVerificationFailed;
//   - errors implementing net.Error (DNS failures, refused connections,
//     network and dialer timeouts) map to common.ReasonHostUnreachable;
//   - caller context errors (context.DeadlineExceeded, context.Canceled)
//     that did not surface through a network operation map to
//     common.ReasonConnectionFailed;
//   - everything else, including a nil error, maps to
//     common.ReasonConnectionFailed.
//
// Deadlines are classified by how they surface: a dialer's own timeout
// surfaces as a *net.OpError wrapping the same sentinel as
// context.DeadlineExceeded on some Go versions, and the context sentinels
// themselves satisfy net.Error on others, so neither the sentinel nor the
// interface alone can discriminate the two. A context error counts as the
// caller's budget only when no *net.OpError is in its chain.
//
// Driver-specific classification, such as PostgreSQL SQLSTATE 28P01
// (password authentication failed), cannot be detected without importing a
// driver. Callers that know the failure mode should pass the reason
// explicitly.
func ReasonForError(err error) string {
	if err == nil {
		return common.ReasonConnectionFailed
	}

	// Caller context errors that did not surface through a network
	// operation. The *net.OpError guard is needed on both ends: a dialer's
	// own timeout wraps the same sentinel as context.DeadlineExceeded,
	// and the context sentinels themselves satisfy net.Error on some Go
	// versions.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		var opErr *net.OpError
		if !errors.As(err, &opErr) {
			return common.ReasonConnectionFailed
		}
	}

	// TLS errors are checked before net.Error: wrapper types such as
	// *url.Error also implement net.Error.
	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return common.ReasonTLSVerificationFailed
	}

	var unknownAuthorityErr x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthorityErr) {
		return common.ReasonTLSVerificationFailed
	}

	var certErr x509.CertificateInvalidError
	if errors.As(err, &certErr) {
		return common.ReasonTLSVerificationFailed
	}

	var recordHeaderErr tls.RecordHeaderError
	if errors.As(err, &recordHeaderErr) {
		return common.ReasonTLSVerificationFailed
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return common.ReasonHostUnreachable
	}

	return common.ReasonConnectionFailed
}

// Redact masks password material in msg: password key/value parameters
// (password=secret, sslpassword=secret, quoted values) and passwords
// embedded in URIs (scheme://user:password@host) are replaced with
// REDACTED. Redact is idempotent and leaves messages without password
// material unchanged.
func Redact(msg string) string {
	msg = passwordParamPattern.ReplaceAllString(msg, "${1}="+redactedPlaceholder)
	msg = uriPasswordPattern.ReplaceAllString(msg, "${1}://${2}:"+redactedPlaceholder+"@")

	return msg
}
