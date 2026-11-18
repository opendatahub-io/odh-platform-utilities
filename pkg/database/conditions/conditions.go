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
	"net/url"
	"regexp"
	"strings"

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
			`('(?:[^'\\]|(?s:\\.)|'')*'?|"(?:[^"\\]|(?s:\\.))*"?|(?:[^\\\s]|(?s:\\.))*)`)

	// uriStartPattern locates candidate URIs in a message; each span from
	// the scheme to the next whitespace is then parsed structurally.
	uriStartPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.\-]*://`)
)

// SetConnected upserts a DatabaseConnected=True condition on accessor,
// reporting a successful database connection.
//
// The reason defaults to common.ReasonConnectionSucceeded. WithReason
// overrides it; the generic reason set by WithError is replaced by the
// default. opts can otherwise refine the condition with the options from
// pkg/controller/conditions, for example WithMessage to record what was
// connected to, or WithSeverity to report connectivity as informational.
// The type and status are fixed: an option cannot retarget the condition
// or invert the outcome. The final message is always passed through
// Redact. Returns true if the condition changed.
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

	condition.Type = string(common.ConditionTypeDatabaseConnected)
	condition.Status = metav1.ConditionTrue

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
// classification, though the Error severity it sets is not. The default
// message is err.Error(), and the final message is always passed through
// Redact, including messages supplied through options such as WithMessage
// or WithError. The type and status are fixed: an option cannot retarget
// the condition or invert the outcome.
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

	condition.Type = string(common.ConditionTypeDatabaseConnected)
	condition.Status = metav1.ConditionFalse

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
//     tls.RecordHeaderError, tls.AlertError) map to
//     common.ReasonTLSVerificationFailed;
//   - errors implementing net.Error (DNS failures, refused connections,
//     network and dialer timeouts) map to common.ReasonHostUnreachable;
//   - caller cancellation (context.Canceled) maps to
//     common.ReasonConnectionFailed;
//   - a caller deadline (context.DeadlineExceeded) that did not surface
//     through a network operation maps to common.ReasonConnectionFailed;
//   - everything else, including a nil error, maps to
//     common.ReasonConnectionFailed.
//
// Cancellation is unambiguous — nothing in the network stack cancels a
// context on its own — so it always maps to the caller, even when the
// error surfaced through a network operation. Deadlines are classified by
// how they surface: a dialer's own timeout surfaces as a *net.OpError
// wrapping the same sentinel as context.DeadlineExceeded on some Go
// versions, and the context sentinels themselves satisfy net.Error on
// others, so neither the sentinel nor the interface alone can discriminate
// the two. A deadline counts as the caller's budget only when no
// *net.OpError is in its chain.
//
// Driver-specific classification, such as PostgreSQL SQLSTATE 28P01
// (password authentication failed), cannot be detected without importing a
// driver. Callers that know the failure mode should pass the reason
// explicitly.
func ReasonForError(err error) string {
	if err == nil {
		return common.ReasonConnectionFailed
	}

	// Cancellation is always the caller's: nothing in the network stack
	// cancels a context on its own, so the match needs no *net.OpError
	// guard even when the error surfaced through one.
	if errors.Is(err, context.Canceled) {
		return common.ReasonConnectionFailed
	}

	// A deadline is ambiguous: a dialer's own timeout wraps the same
	// sentinel as context.DeadlineExceeded, so only a deadline that did
	// not surface through a network operation is the caller's budget.
	if errors.Is(err, context.DeadlineExceeded) {
		var opErr *net.OpError
		if !errors.As(err, &opErr) {
			return common.ReasonConnectionFailed
		}
	}

	if isTLSError(err) {
		return common.ReasonTLSVerificationFailed
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return common.ReasonHostUnreachable
	}

	return common.ReasonConnectionFailed
}

// isTLSError reports whether the error is a TLS-layer failure: a
// certificate or hostname error, a malformed handshake record, or a
// peer-sent alert. TLS errors are checked before net.Error because
// wrapper types such as *url.Error also implement net.Error.
func isTLSError(err error) bool {
	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return true
	}

	var unknownAuthorityErr x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthorityErr) {
		return true
	}

	var certErr x509.CertificateInvalidError
	if errors.As(err, &certErr) {
		return true
	}

	var recordHeaderErr tls.RecordHeaderError
	if errors.As(err, &recordHeaderErr) {
		return true
	}

	var alertErr tls.AlertError

	return errors.As(err, &alertErr)
}

// Redact masks password material in msg: password key/value parameters in
// libpq-style DSNs (password=secret, sslpassword=secret, quoted values
// with escapes) and credentials embedded in URIs
// (scheme://user:password@host and password query parameters). The two
// syntaxes are scanned left to right: a DSN password match consumes its
// complete value — including any embedded scheme — before the scanner
// looks for the next URI, and URI spans are parsed structurally: userinfo
// is determined before the query is split off — including userinfo a
// password-embedded separator hid, whatever port-like shape the password
// prefix takes — query keys are percent-decoded before comparison, and
// non-secret query fields are preserved, though a field value that itself
// embeds a URI or a DSN password parameter is redacted recursively.
// Redact is idempotent and leaves messages without password material
// unchanged.
func Redact(msg string) string {
	if !uriStartPattern.MatchString(msg) && !passwordParamPattern.MatchString(msg) {
		return msg
	}

	var b strings.Builder

	pos := 0

	for {
		var done bool

		pos, done = maskNext(msg, pos, &b)
		if done {
			break
		}
	}

	b.WriteString(msg[pos:])

	return b.String()
}

// maskNext masks the next DSN password or URI span in msg from pos,
// appending the masked output to b, and returns the new scan position
// and whether scanning is complete.
func maskNext(msg string, pos int, b *strings.Builder) (int, bool) {
	u := uriStartPattern.FindStringIndex(msg[pos:])
	d := passwordParamPattern.FindStringIndex(msg[pos:])

	if u == nil && d == nil {
		return pos, true
	}

	// A DSN match consumes its complete value before the scanner
	// searches for another URI, so a password containing an embedded
	// scheme is masked in full.
	if d != nil && (u == nil || d[0] <= u[0]) {
		b.WriteString(msg[pos : pos+d[0]])
		b.WriteString(maskDSN(msg[pos+d[0] : pos+d[1]]))

		return pos + d[1], false
	}

	start := pos + u[0]
	end := uriSpanEnd(msg, start, pos+u[1])

	b.WriteString(msg[pos:start])
	b.WriteString(maskURISpan(msg[start:end]))

	return end, false
}

// uriSpanEnd returns the end of the URI span starting at start: the next
// whitespace, or an embedded scheme preceded by a URI separator.
func uriSpanEnd(msg string, start, markerEnd int) int {
	end := start + strings.IndexAny(msg[start:], " \t\n\r\v\f")
	if end < start {
		end = len(msg)
	}

	// Multiple URIs can share one whitespace-delimited token when
	// separated by commas or semicolons: an embedded scheme starts a
	// new URI only there, so a scheme inside a password value does not
	// split it. Every candidate is examined — a scheme inside a
	// non-secret value, such as a query parameter holding a URL, is
	// skipped, not terminal — so a later separator-preceded scheme
	// still ends the span. The search starts after the current scheme
	// prefix, which would otherwise match itself.
	for from := markerEnd; from < end; {
		next := uriStartPattern.FindStringIndex(msg[from:end])
		if next == nil {
			break
		}

		at := from + next[0]
		if msg[at-1] == ',' || msg[at-1] == ';' {
			end = at

			break
		}

		// next is relative to the searched slice: resume after the
		// matched scheme, not after its start offset added twice.
		from += next[1]
	}

	return end
}

// maskDSN masks password key/value parameters in plain, non-URI text.
func maskDSN(s string) string {
	return passwordParamPattern.ReplaceAllString(s, "${1}="+redactedPlaceholder)
}

// maskURISpan masks the userinfo password and the password query
// parameters of a single whitespace-delimited URI span. The userinfo is
// determined before the query is split off, so a password containing ?
// stays intact; the path and fragment are plain text as far as redaction
// is concerned and are passed through maskDSN.
func maskURISpan(span string) string {
	prefixLen := strings.Index(span, "://") + 3

	masked := span[:prefixLen] + maskUserinfo(span[prefixLen:])

	beforeQuery, query, hasQuery := strings.Cut(masked, "?")

	var b strings.Builder

	b.WriteString(maskDSN(beforeQuery))

	if hasQuery {
		b.WriteByte('?')
		b.WriteString(maskQuery(query))
	}

	return b.String()
}

// maskUserinfo masks the password in the userinfo of a URI. The authority
// ends at the first /, ? or #; when it holds no @ but is
// user:password-shaped, the password itself contained the separator, and
// the rest of the span is searched for the terminating @ so a password
// containing /, ? or # is masked in full — including when the password
// prefix reads as a port (postgres://u:12345?ss@host/db). See
// truncatedUserinfoEnd for how that shape is told apart from a real
// host:port.
func maskUserinfo(rest string) string {
	authorityEnd := strings.IndexAny(rest, "/?#")
	if authorityEnd < 0 {
		authorityEnd = len(rest)
	}

	authority := rest[:authorityEnd]

	userinfoEnd := strings.LastIndexByte(authority, '@')
	if userinfoEnd < 0 {
		userinfoEnd = truncatedUserinfoEnd(rest, authority, authorityEnd)
	}

	if userinfoEnd < 0 {
		return rest
	}

	user, password, hasColon := strings.Cut(rest[:userinfoEnd], ":")
	if !hasColon || password == "" {
		return rest
	}

	return user + ":" + redactedPlaceholder + rest[userinfoEnd:]
}

// truncatedUserinfoEnd locates the @ that terminates userinfo material a
// password-embedded separator hid from the authority: the authority holds
// no @, yet one follows later in the span.
//
// A user:password-shaped authority trusts any later @. A host:port-shaped
// one — the shape a numeric password prefix shares — recovers only on
// structural evidence that the @ cannot be path or query material: an
// empty host is not host:port, and an @ whose following segment a URI
// delimiter terminates marks a second host, which is how the truncated
// user:password@host reading presents. A query or fragment separator that
// cut the authority also recovers when no key=value field precedes the @:
// a query field carries an =, so a bare one is a password that swallowed
// the separator, pathless host or not. That = check is also what leaves
// one shape unrecovered on the keep side: a # -cut segment holding both
// = and a later @ (u:12#x=y@host) is indistinguishable from fragment
// material (db:5432#user=u@x), so the check prefers the non-secret
// reading there. Candidates are examined from the last @ backwards: a
// password containing an @ gives an earlier candidate the same shape as
// the terminator, so the rightmost @ carrying evidence wins. A path
// separator that cut the authority does not recover on a bare host:
// u:12/34@host is indistinguishable from the path @ in
// db:5432/team@east, so the checks keep
// postgres://db.example.com:5432/registry?user=u@x and
// postgres://db:5432/team@east intact. Redaction fails closed: the
// ambiguous cases lean toward masking.
func truncatedUserinfoEnd(rest, authority string, authorityEnd int) int {
	if looksLikeUserPassword(authority) {
		return strings.LastIndexByte(rest, '@')
	}

	host, _, hasColon := strings.Cut(authority, ":")
	if !hasColon {
		return -1
	}

	for at := strings.LastIndexByte(rest, '@'); at > authorityEnd; at = strings.LastIndexByte(rest[:at], '@') {
		if host == "" {
			return at
		}

		if afterAt := rest[at+1:]; strings.IndexAny(afterAt, "/?#") > 0 {
			return at
		}

		if cut := rest[authorityEnd:at]; (strings.HasPrefix(cut, "?") || strings.HasPrefix(cut, "#")) &&
			!strings.Contains(cut, "=") {
			return at
		}
	}

	return -1
}

// looksLikeUserPassword reports whether an authority segment has the
// shape of user:password rather than host:port: a colon whose tail is not
// a short run of digits, which is how ports appear. A bracketed authority
// is an IPv6 host literal, never a username.
func looksLikeUserPassword(authority string) bool {
	if strings.HasPrefix(authority, "[") {
		return false
	}

	_, tail, hasColon := strings.Cut(authority, ":")
	if !hasColon {
		return false
	}

	if tail != "" && len(tail) <= 5 && strings.Trim(tail, "0123456789") == "" {
		return false
	}

	return true
}

// maskQuery masks the value of every password field in a URI query.
// Field keys are percent-decoded before comparison; DSN quoting rules do
// not apply to query values, so the complete raw value is replaced. A key
// that cannot be decoded masks its value too: redaction fails closed.
// A value that itself embeds a URI or a DSN password parameter is
// redacted recursively, so credentials nested in the value of another
// field are masked as well. A bare field — no key/value separator — that
// follows a password field is password material the value's own separator
// split: it is masked too, failing closed.
func maskQuery(query string) string {
	if query == "" {
		return query
	}

	fields := strings.Split(query, "&")

	absorbing := false

	for i, field := range fields {
		key, value, hasValue := strings.Cut(field, "=")
		if !hasValue {
			if absorbing {
				fields[i] = redactedPlaceholder
			}

			continue
		}

		decoded, err := url.PathUnescape(key)
		if err != nil || strings.EqualFold(decoded, "password") ||
			strings.EqualFold(decoded, "sslpassword") {
			fields[i] = key + "=" + redactedPlaceholder
			absorbing = true

			continue
		}

		absorbing = false

		if uriStartPattern.MatchString(value) || passwordParamPattern.MatchString(value) {
			fields[i] = key + "=" + Redact(value)
		}
	}

	return strings.Join(fields, "&")
}
