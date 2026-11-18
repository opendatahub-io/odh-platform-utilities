package conditions_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	kcond "github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/database/conditions"
)

// Package-level error sentinels.
var (
	errDialRefused = &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: syscall.ECONNREFUSED,
	}

	errDNSNotFound = &net.DNSError{
		Err:        "no such host",
		Name:       "db.example.com",
		IsNotFound: true,
	}

	// errAuthRejectedWithDSN mimics a driver error whose message embeds the
	// DSN together with its password.
	errAuthRejectedWithDSN = errors.New("failed to connect to " +
		"`host=db.example.com user=modelregistry password=s3cr3t dbname=registry`")

	errAuthRejected    = errors.New(`pq: password authentication failed for user "modelregistry"`)
	errUnclassified    = errors.New("server closed the connection unexpectedly")
	errFirstFailure    = errors.New("first failure")
	errSecondFailure   = errors.New("second failure")
	errDuringReconcile = errors.New("during reconcile")

	// errQuotedPassword mimics a driver error whose message embeds a
	// quoted password with escapes.
	errQuotedPassword = errors.New("failed to connect to " +
		"`host=db.example.com user=modelregistry password='p@''ss w0rd' dbname=registry`")

	// errSeparatorPassword mimics an error whose unquoted password begins
	// with a separator character.
	errSeparatorPassword = errors.New("connect: password=&s3cr3t dbname=registry")

	// errEscapedWhitespace mimics an error whose unquoted password
	// contains a backslash-escaped space, which libpq accepts.
	errEscapedWhitespace = errors.New(`connect: password=prefix\ suffix dbname=registry`)

	// errUnterminatedQuote mimics a malformed DSN whose quoted password
	// never closes.
	errUnterminatedQuote = errors.New("connect: password='prefix suffix dbname=registry")

	// errURIPasswordFragment mimics an error whose URI userinfo password
	// contains a password= fragment.
	errURIPasswordFragment = errors.New("failed to connect to postgres://u:prefix-password=xxx@host/db")

	// errSchemePassword mimics an error whose password value contains an
	// embedded scheme.
	errSchemePassword = errors.New("failed to connect: password='ab://cd' dbname=x")
)

// componentCR is a minimal component CR: embedding common.Status provides
// the ConditionsAccessor interface the helpers operate on.
type componentCR struct {
	common.Status
}

var _ common.ConditionsAccessor = (*componentCR)(nil)

// findCondition returns the named condition or fails the test.
func findCondition(
	t *testing.T,
	accessor common.ConditionsAccessor,
	conditionType common.ConditionType,
) common.Condition {
	t.Helper()

	for _, cond := range accessor.GetConditions() {
		if cond.Type == string(conditionType) {
			return cond
		}
	}

	t.Fatalf("condition %s not found", conditionType)

	return common.Condition{}
}

func TestSetConnectedUpsertsDefaultReason(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}

	changed := conditions.SetConnected(cr)

	assert.True(t, changed)

	got := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.Equal(t, metav1.ConditionTrue, got.Status)
	assert.Equal(t, common.ReasonConnectionSucceeded, got.Reason)
	assert.Empty(t, got.Message)
	assert.Empty(t, got.Severity)
}

func TestSetConnectedReplacesExistingCondition(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}
	conditions.SetFailed(cr, errDialRefused)

	changed := conditions.SetConnected(cr)

	assert.True(t, changed)

	got := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.Equal(t, metav1.ConditionTrue, got.Status)
	assert.Equal(t, common.ReasonConnectionSucceeded, got.Reason)
	assert.Empty(t, got.Message)
}

func TestSetConnectedLeavesSiblingsUntouched(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}
	kcond.SetStatusCondition(cr, common.Condition{
		Type:   string(common.ConditionTypeReady),
		Status: metav1.ConditionTrue,
		Reason: "AllDependentsHealthy",
	})

	conditions.SetConnected(cr)

	assert.Len(t, cr.GetConditions(), 2)

	ready := findCondition(t, cr, common.ConditionTypeReady)
	assert.Equal(t, metav1.ConditionTrue, ready.Status)
}

func TestSetConnectedAppliesOptions(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}

	conditions.SetConnected(cr,
		kcond.WithMessage("connected to db.example.com:5432/model_registry"),
		kcond.WithSeverity(common.ConditionSeverityInfo),
		kcond.WithObservedGeneration(4),
	)

	got := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.Equal(t, "connected to db.example.com:5432/model_registry", got.Message)
	assert.Equal(t, common.ConditionSeverityInfo, got.Severity)
	assert.Equal(t, int64(4), got.ObservedGeneration)
}

func TestSetConnectedIdempotent(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}

	assert.True(t, conditions.SetConnected(cr))
	assert.False(t, conditions.SetConnected(cr))
}

func TestSetConnectedNilAccessor(t *testing.T) {
	t.Parallel()

	assert.False(t, conditions.SetConnected(nil))
}

func TestSetFailedClassifiesReasonFromError(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}

	changed := conditions.SetFailed(cr, errDialRefused)

	assert.True(t, changed)

	got := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.Equal(t, metav1.ConditionFalse, got.Status)
	assert.Equal(t, common.ReasonHostUnreachable, got.Reason)
	assert.Equal(t, "dial tcp: connection refused", got.Message)
}

func TestSetFailedRedactsMessage(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}

	conditions.SetFailed(cr, errAuthRejectedWithDSN)

	got := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.NotContains(t, got.Message, "s3cr3t")
	assert.Contains(t, got.Message, "password=REDACTED")
}

func TestSetFailedRedactsEscapedAndPrefixedPasswords(t *testing.T) {
	t.Parallel()

	for _, err := range []error{
		errQuotedPassword,
		errSeparatorPassword,
		errEscapedWhitespace,
		errUnterminatedQuote,
	} {
		cr := &componentCR{}

		conditions.SetFailed(cr, err)

		got := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
		assert.NotContains(t, got.Message, "s3cr3t")
		assert.NotContains(t, got.Message, "w0rd")
		assert.NotContains(t, got.Message, "suffix")
		assert.Contains(t, got.Message, "password=REDACTED")
	}
}

func TestSetConnectedRedactsMessageOption(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}

	conditions.SetConnected(cr, kcond.WithMessage(`connected with password=prefix\ suffix`))

	got := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.NotContains(t, got.Message, "suffix")
	assert.Contains(t, got.Message, "password=REDACTED")
}

func TestSetFailedRedactsURIPasswordFragment(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}

	conditions.SetFailed(cr, errURIPasswordFragment)

	got := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.NotContains(t, got.Message, "prefix-password")
	assert.NotContains(t, got.Message, "xxx")
	assert.Contains(t, got.Message, "u:REDACTED@")
}

func TestSetFailedRedactsSchemeInPassword(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}

	conditions.SetFailed(cr, errSchemePassword)

	got := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.NotContains(t, got.Message, "ab://cd")
	assert.Contains(t, got.Message, "password=REDACTED")
}

// The type and status are the setters' contract: a custom option must not
// be able to upsert a sibling condition or invert the reported outcome.
func TestSettersFixTypeAndStatus(t *testing.T) {
	t.Parallel()

	rogue := func(c *common.Condition) {
		c.Type = string(common.ConditionTypeReady)
		c.Status = metav1.ConditionTrue
	}

	cr := &componentCR{}
	conditions.SetFailed(cr, errDialRefused, rogue)

	assert.Len(t, cr.GetConditions(), 1)

	got := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.Equal(t, metav1.ConditionFalse, got.Status)
	assert.Equal(t, common.ReasonHostUnreachable, got.Reason)

	rogueFalse := func(c *common.Condition) {
		c.Type = string(common.ConditionTypeDegraded)
		c.Status = metav1.ConditionFalse
	}

	cr = &componentCR{}
	conditions.SetConnected(cr, rogueFalse)

	assert.Len(t, cr.GetConditions(), 1)

	got = findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.Equal(t, metav1.ConditionTrue, got.Status)
	assert.Equal(t, common.ReasonConnectionSucceeded, got.Reason)
}

func TestSetFailedRedactsMessagesSuppliedViaOptions(t *testing.T) {
	t.Parallel()

	// The message set by WithError must still be redacted.
	cr := &componentCR{}
	conditions.SetFailed(cr, errAuthRejectedWithDSN, kcond.WithError(errAuthRejectedWithDSN))

	got := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.NotContains(t, got.Message, "s3cr3t")
	assert.Contains(t, got.Message, "password=REDACTED")

	// A custom message supplied via WithMessage is redacted too.
	cr = &componentCR{}
	conditions.SetFailed(cr, errUnclassified, kcond.WithMessage("connected with password=abc123"))

	got = findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.NotContains(t, got.Message, "abc123")
	assert.Contains(t, got.Message, "password=REDACTED")
}

func TestSetFailedIdempotent(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}

	assert.True(t, conditions.SetFailed(cr, errDialRefused))
	assert.False(t, conditions.SetFailed(cr, errDialRefused))
}

func TestSetConnectedObservedGenerationRequiresOptionEachCall(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}

	conditions.SetConnected(cr, kcond.WithObservedGeneration(7))
	conditions.SetConnected(cr)

	// The upsert replaces the condition wholesale: observedGeneration must
	// be passed on every call.
	got := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.Equal(t, int64(0), got.ObservedGeneration)
}

func TestSetFailedReasonOverride(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}

	conditions.SetFailed(cr, errAuthRejected, kcond.WithReason(common.ReasonCredentialsRejected))

	got := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.Equal(t, common.ReasonCredentialsRejected, got.Reason)

	// The authentication message itself contains no secret material and
	// must not be mangled.
	assert.Equal(t, errAuthRejected.Error(), got.Message)
}

func TestSetFailedNilError(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}

	conditions.SetFailed(cr, nil)

	got := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.Equal(t, common.ReasonConnectionFailed, got.Reason)
	assert.Empty(t, got.Message)
}

func TestSetFailedNilAccessor(t *testing.T) {
	t.Parallel()

	assert.False(t, conditions.SetFailed(nil, errUnclassified))
}

func TestReasonForErrorDefaultsToConnectionFailed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  error
		name string
	}{
		{err: nil, name: "nil error"},
		{err: errUnclassified, name: "unclassified error"},
		{err: context.DeadlineExceeded, name: "caller context deadline exceeded"},
		{err: context.Canceled, name: "caller context canceled"},
		{
			// Cancellation is always the caller's, even when it
			// surfaced through a network operation.
			err:  &net.OpError{Op: "dial", Net: "tcp", Err: context.Canceled},
			name: "network-wrapped context cancellation",
		},
		{
			err:  fmt.Errorf("ping: %w", context.DeadlineExceeded),
			name: "wrapped caller context deadline",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, common.ReasonConnectionFailed, conditions.ReasonForError(tc.err))
		})
	}
}

func TestReasonForErrorClassifiesNetworkErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  error
		name string
	}{
		{err: errDialRefused, name: "connection refused"},
		{err: errDNSNotFound, name: "DNS resolution failure"},
		{
			err:  fmt.Errorf("opening connection: %w", &net.OpError{Op: "dial", Net: "tcp"}),
			name: "wrapped network error",
		},
		{
			// A dialer's own Timeout expiry, however its deadline sentinel
			// is wrapped on the running Go version.
			err:  &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrDeadlineExceeded},
			name: "dialer deadline",
		},
		{
			// A network-wrapped context deadline: the shape a dialer
			// timeout produces on Go versions whose net timeout error
			// matches the context sentinel.
			err:  &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded},
			name: "network-wrapped context deadline",
		}, {
			err:  errors.Join(errDialRefused, errDuringReconcile),
			name: "joined network error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, common.ReasonHostUnreachable, conditions.ReasonForError(tc.err))
		})
	}
}

// A dial whose deadline is already in the past returns before any network
// I/O with the same shape as a dialer timeout: a *net.OpError wrapping the
// context deadline sentinel. The errors.Is assertion pins that the guarded
// branch is exercised on every run, independent of the network
// environment.
func TestReasonForErrorClassifiesDialerTimeout(t *testing.T) {
	t.Parallel()

	d := &net.Dialer{Deadline: time.Now().Add(-time.Second)}

	_, err := d.Dial("tcp", "127.0.0.1:1")
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, common.ReasonHostUnreachable, conditions.ReasonForError(err))
}

// A dial canceled by the caller: the error surfaces through a
// *net.OpError, but cancellation is unambiguously the caller's, so it must
// classify as ConnectionFailed. The pre-canceled context makes the dial
// return before any network I/O.
func TestReasonForErrorClassifiesCanceledDial(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&net.Dialer{}).DialContext(ctx, "tcp", "127.0.0.1:1")
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, common.ReasonConnectionFailed, conditions.ReasonForError(err))
}

func TestReasonForErrorClassifiesTLSErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  error
		name string
	}{
		{err: x509.UnknownAuthorityError{}, name: "unknown authority"},
		{
			err:  fmt.Errorf("tls: failed to verify certificate: %w", x509.UnknownAuthorityError{}),
			name: "wrapped certificate error",
		},
		{
			err:  x509.CertificateInvalidError{Reason: x509.Expired, Detail: "certificate is expired"},
			name: "certificate invalid",
		},
		{
			err:  fmt.Errorf("tls: %w", x509.HostnameError{Host: "db.example.com"}),
			name: "hostname mismatch",
		},
		{
			err:  tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"},
			name: "tls record header error",
		},
		{
			// 42 is bad_certificate; the alert values are unexported.
			err:  tls.AlertError(42),
			name: "tls peer alert",
		},
		{
			err:  fmt.Errorf("handshake: %w", tls.AlertError(40)), // 40 is handshake_failure.
			name: "wrapped tls peer alert",
		},
		{
			err:  fmt.Errorf("handshake: %w", tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}),
			name: "wrapped tls record header error",
		},
		{
			// *url.Error implements net.Error but wraps a certificate
			// error; certificate checks must win over net.Error.
			err:  &url.Error{Op: "Get", URL: "https://db.example.com", Err: x509.UnknownAuthorityError{}},
			name: "url error wrapping certificate error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, common.ReasonTLSVerificationFailed, conditions.ReasonForError(tc.err))
		})
	}
}

func TestRedactMasksPasswordMaterial(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
		want string
	}{
		{
			name: "libpq-style DSN",
			msg: "failed to connect to " +
				"`host=db.example.com user=modelregistry password=s3cr3t dbname=registry`",
			want: "failed to connect to " +
				"`host=db.example.com user=modelregistry password=REDACTED dbname=registry`",
		},
		{
			name: "unquoted password containing a semicolon",
			msg:  "connect: password=abc;def dbname=registry",
			want: "connect: password=REDACTED dbname=registry",
		},
		{
			name: "unquoted password prefixed with an ampersand",
			msg:  "connect: password=&s3cr3t dbname=registry",
			want: "connect: password=REDACTED dbname=registry",
		},
		{
			name: "unquoted password containing an ampersand",
			msg:  "connect: password=abc&def dbname=registry",
			want: "connect: password=REDACTED dbname=registry",
		},
		{
			name: "unquoted password with escaped whitespace",
			msg:  `connect: password=prefix\ suffix dbname=registry`,
			want: "connect: password=REDACTED dbname=registry",
		},
		{
			name: "sslpassword key",
			msg:  "connect: sslpassword=tlssecret dbname=registry",
			want: "connect: sslpassword=REDACTED dbname=registry",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, conditions.Redact(tc.msg))
		})
	}
}

func TestRedactMasksQuotedPasswordValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
		want string
	}{
		{
			name: "single-quoted with spaces",
			msg:  "connect: password='p@ss w0rd' dbname=registry",
			want: "connect: password=REDACTED dbname=registry",
		},
		{
			name: "single-quoted with doubled-quote escape",
			msg:  "connect: password='p@''ss w0rd' dbname=registry",
			want: "connect: password=REDACTED dbname=registry",
		},
		{
			name: "single-quoted with backslash escape",
			msg:  `connect: password='p@\'ss w0rd' dbname=registry`,
			want: "connect: password=REDACTED dbname=registry",
		},
		{
			name: "double-quoted with spaces",
			msg:  `connect: password="p@ss w0rd" dbname=registry`,
			want: `connect: password=REDACTED dbname=registry`,
		},
		{
			name: "double-quoted with escape",
			msg:  `connect: password="p@\"ss w0rd" dbname=registry`,
			want: `connect: password=REDACTED dbname=registry`,
		},
		{
			// An unterminated quote cannot reveal where the value ends,
			// so the rest of the string is masked.
			name: "unterminated single quote masks the rest",
			msg:  "invalid DSN: password='prefix suffix dbname=registry",
			want: "invalid DSN: password=REDACTED",
		},
		{
			name: "unterminated double quote masks the rest",
			msg:  `invalid DSN: password="prefix suffix dbname=registry`,
			want: "invalid DSN: password=REDACTED",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, conditions.Redact(tc.msg))
		})
	}
}

func TestRedactMasksURICredentials(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
		want string
	}{
		{ //nolint:gosec // fixture credential, not a real secret
			name: "URI with embedded password",
			msg:  "postgres://modelregistry:hunter2@db.example.com:5432/registry",
			want: "postgres://modelregistry:REDACTED@db.example.com:5432/registry",
		},
		{
			name: "URI with empty user",
			msg:  "postgres://:s3cr3t@db.example.com/registry",
			want: "postgres://:REDACTED@db.example.com/registry",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "URI password containing slash",
			msg:  "postgres://modelregistry:pa/ss@db.example.com/registry",
			want: "postgres://modelregistry:REDACTED@db.example.com/registry",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "URI whose password contains a password parameter",
			msg:  "postgres://u:prefix-password=xxx@host/db",
			want: "postgres://u:REDACTED@host/db",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "URI password containing an at sign",
			msg:  "postgres://user:p@ss@db.example.com/registry",
			want: "postgres://user:REDACTED@db.example.com/registry",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "password containing a question mark",
			msg:  "postgres://u:pa?ss@db.example.com/app",
			want: "postgres://u:REDACTED@db.example.com/app",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, conditions.Redact(tc.msg))
		})
	}
}

func TestRedactMasksURIQueryCredentials(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
		want string
	}{
		{
			// Non-secret query fields are preserved; only the password
			// field's value is replaced.
			name: "query parameter",
			msg:  "postgres://db.example.com/registry?password=abc123&sslmode=require",
			want: "postgres://db.example.com/registry?password=REDACTED&sslmode=require",
		},
		{
			// libpq percent-decodes query keys before lookup.
			name: "percent-encoded key",
			msg:  "postgres://db.example.com/registry?%70assword=abc123&sslmode=require",
			want: "postgres://db.example.com/registry?%70assword=REDACTED&sslmode=require",
		},
		{
			// DSN quoting rules do not apply to query values.
			name: "quoted-looking value",
			msg:  "postgres://db.example.com/registry?password='abc'def",
			want: "postgres://db.example.com/registry?password=REDACTED",
		},
		{
			// A key that cannot be decoded masks its value: redaction
			// fails closed.
			name: "undecodable key",
			msg:  "postgres://db.example.com/registry?%zz=abc&password=def",
			want: "postgres://db.example.com/registry?%zz=REDACTED&password=REDACTED",
		},
		{
			name: "at sign in query is not userinfo",
			msg:  "postgres://db.example.com:5432/registry?user=u@x",
			want: "postgres://db.example.com:5432/registry?user=u@x",
		},
		{
			name: "at sign in query value after a path",
			msg:  "postgres://db/app?x=a@b",
			want: "postgres://db/app?x=a@b",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, conditions.Redact(tc.msg))
		})
	}
}

func TestRedactLeavesCleanMessagesUnchanged(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
	}{
		{"authentication message", errAuthRejected.Error()},
		{"URI without password", "postgres://modelregistry@db.example.com:5432/registry"},
		{"message without secrets", "dial tcp 10.0.0.5:5432: connect: connection refused"},
		{"empty message", ""},
		{"path at sign is not userinfo", "postgres://db:5432/team@east"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.msg, conditions.Redact(tc.msg))
		})
	}
}

// libpq consumes a backslash followed by a newline as part of the value,
// so escaped newlines are password material, not terminators.
func TestRedactMasksEscapedNewlines(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
		want string
	}{
		{
			name: "single-quoted with escaped newline",
			msg:  "connect: password='p@''ss \\\nw0rd' dbname=registry",
			want: "connect: password=REDACTED dbname=registry",
		},
		{
			name: "double-quoted with escaped newline",
			msg:  "connect: password=\"p@ss \\\nw0rd\" dbname=registry",
			want: "connect: password=REDACTED dbname=registry",
		},
		{
			name: "unquoted with escaped newline",
			msg:  "connect: password=p@ss\\\nw0rd dbname=registry",
			want: "connect: password=REDACTED dbname=registry",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, conditions.Redact(tc.msg))
		})
	}
}

// A password value containing an embedded scheme must be consumed whole:
// the scanner must not treat a scheme inside the value as a new URI.
func TestRedactMasksSchemeInPasswordValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
		want string
	}{
		{
			name: "quoted DSN value",
			msg:  "connect: password='ab://cd' dbname=x",
			want: "connect: password=REDACTED dbname=x",
		},
		{
			name: "unquoted DSN value",
			msg:  "connect: password=ab://cd dbname=x",
			want: "connect: password=REDACTED dbname=x",
		},
		{
			name: "URI query value",
			msg:  "postgres://h/db?password=ab://cd&sslmode=disable",
			want: "postgres://h/db?password=REDACTED&sslmode=disable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, conditions.Redact(tc.msg))
		})
	}
}

// Multiple URIs can share one whitespace-delimited token, separated by
// commas or semicolons: each URI in the token is masked.
func TestRedactMasksMultipleURIs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
		want string
	}{
		{ //nolint:gosec // fixture credential, not a real secret
			name: "comma-separated URIs",
			msg:  "postgres://u:one@host,postgres://v:two@host",
			want: "postgres://u:REDACTED@host,postgres://v:REDACTED@host",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "semicolon-separated URIs",
			msg:  "postgres://u:one@host;postgres://v:two@host",
			want: "postgres://u:REDACTED@host;postgres://v:REDACTED@host",
		},
		{
			name: "comma-separated URIs with query on the last",
			msg:  "postgres://u:one@host,postgres://host2/db?password=abc",
			want: "postgres://u:REDACTED@host,postgres://host2/db?password=REDACTED",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, conditions.Redact(tc.msg))
		})
	}
}

// A password prefix that reads as a port must not exempt the authority
// from userinfo recovery: the terminating @ a separator inside the
// password hid is still found, whatever separator it hid behind.
func TestRedactMasksPortShapedPasswordPrefixes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
		want string
	}{
		{ //nolint:gosec // fixture credential, not a real secret
			name: "question mark in password with a numeric prefix",
			msg:  "postgres://u:12345?ss@host/db",
			want: "postgres://u:REDACTED@host/db",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "question mark in password with a numeric prefix, pathless",
			msg:  "postgres://u:12345?ss@host",
			want: "postgres://u:REDACTED@host",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "hash in password with a numeric prefix, pathless",
			msg:  "postgres://u:12#34@host",
			want: "postgres://u:REDACTED@host",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "slash in password with a numeric prefix",
			msg:  "postgres://u:12/34@host/db",
			want: "postgres://u:REDACTED@host/db",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "hash in password with a numeric prefix",
			msg:  "postgres://u:12#34@host/db",
			want: "postgres://u:REDACTED@host/db",
		},
		{
			name: "question mark in password with an empty user",
			msg:  "postgres://:5432?x@host/db",
			want: "postgres://:REDACTED@host/db",
		},
		{
			name: "slash in password with an empty user",
			msg:  "postgres://:5432/x@host/db",
			want: "postgres://:REDACTED@host/db",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "later at sign in the query does not decoy the recovery",
			msg:  "postgres://u:12345?ss@host/db?x=a@b",
			want: "postgres://u:REDACTED@host/db?x=a@b",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, conditions.Redact(tc.msg))
		})
	}
}

// A password containing an @ makes an earlier candidate look like the
// userinfo terminator: candidates are examined from the last @ backwards,
// so the password material after an in-password @ is masked too.
func TestRedactMasksAtSignInRecoveredPasswords(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
		want string
	}{
		{ //nolint:gosec // fixture credential, not a real secret
			name: "slash and at sign in password",
			msg:  "postgres://u:12/ab@cd@host/db",
			want: "postgres://u:REDACTED@host/db",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "question mark and at sign in password, pathless",
			msg:  "postgres://u:12?ab@cd@host",
			want: "postgres://u:REDACTED@host",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "hash and at sign in password",
			msg:  "postgres://u:12#ab@cd@host/db",
			want: "postgres://u:REDACTED@host/db",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, conditions.Redact(tc.msg))
		})
	}
}

// A host:port-shaped authority stays trusted when the later @ sits in
// path or query material: nothing that a URI delimiter terminates
// follows it, so the @ cannot mark a second host.
func TestRedactPreservesHostPortWithLaterAtSign(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
	}{
		{"at sign in query after a path", "postgres://db.example.com:5432/registry?user=u@x"},
		{"at sign in path at the end", "postgres://db:5432/team@east"},
		{"at sign in a query value", "postgres://db/app?x=a@b"},
		{"query field with an at sign after a port", "postgres://h:42?user=u@x"},
		{"fragment holding a field and an at sign after a port", "postgres://db:5432#user=u@x"},
		{"at sign in a query after an IPv6 authority", "postgres://[::1]:5432/db?user=u@x"},
		{"IPv6 authority with no userinfo", "postgres://[::1]:5432/db"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.msg, conditions.Redact(tc.msg))
		})
	}
}

// A scheme inside a non-secret value is skipped when the span boundary
// is searched, not terminal: a later separator-preceded URI in the same
// whitespace token is still masked.
func TestRedactMasksLaterURIsAfterSchemeInValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
		want string
	}{
		{ //nolint:gosec // fixture credential, not a real secret
			name: "URL-valued query parameter before a comma-separated URI",
			msg:  "postgres://u:one@host/db?application_name=http://example,postgres://v:two@host",
			want: "postgres://u:REDACTED@host/db?application_name=http://example,postgres://v:REDACTED@host",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "URL-valued query parameter and a parameter before the URI",
			msg:  "postgres://u:one@host/db?app=http://x,a=1,postgres://v:two@host",
			want: "postgres://u:REDACTED@host/db?app=http://x,a=1,postgres://v:REDACTED@host",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "semicolon-separated URI after a URL-valued parameter",
			msg:  "postgres://u:one@host/db?app=http://x;postgres://v:two@host",
			want: "postgres://u:REDACTED@host/db?app=http://x;postgres://v:REDACTED@host",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "scheme in a path value before a comma-separated URI",
			msg:  "postgres://u:one@host/path://x,postgres://v:two@host",
			want: "postgres://u:REDACTED@host/path://x,postgres://v:REDACTED@host",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, conditions.Redact(tc.msg))
		})
	}
}

// Credentials nested in the value of a non-secret query field — a URI
// holding userinfo or a password parameter — are masked recursively;
// values without password material are preserved.
func TestRedactMasksNestedCredentialsInQueryValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
		want string
	}{
		{
			name: "password parameter in a nested URI value",
			msg:  "postgres://h/db?appname=http://x?password=secret",
			want: "postgres://h/db?appname=http://x?password=REDACTED",
		},
		{ //nolint:gosec // fixture credential, not a real secret
			name: "userinfo in a nested URI value",
			msg:  "postgres://h/db?appname=http://u:p@x",
			want: "postgres://h/db?appname=http://u:REDACTED@x",
		},
		{
			name: "non-secret URL value is preserved",
			msg:  "postgres://h/db?appname=http://example.com/x",
			want: "postgres://h/db?appname=http://example.com/x",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, conditions.Redact(tc.msg))
		})
	}
}

// A bare query field — no key/value separator — that follows a password
// field is password material the value's own separator split: it is
// absorbed and masked, failing closed. Bare fields elsewhere are
// preserved.
func TestRedactMasksPasswordSplitByBareField(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
		want string
	}{
		{
			name: "password value split by an ampersand",
			msg:  "postgres://h/db?password=se&cret",
			want: "postgres://h/db?password=REDACTED&REDACTED",
		},
		{
			name: "absorbed split followed by a regular field",
			msg:  "postgres://h/db?password=se&cret&app=1",
			want: "postgres://h/db?password=REDACTED&REDACTED&app=1",
		},
		{
			name: "bare flag after a non-password field is preserved",
			msg:  "postgres://h/db?sslmode=require&flag",
			want: "postgres://h/db?sslmode=require&flag",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, conditions.Redact(tc.msg))
		})
	}
}

func TestRedactMultipleOccurrences(t *testing.T) {
	t.Parallel()

	got := conditions.Redact("password=first then postgres://u:second@h/db")

	assert.Equal(t, "password=REDACTED then postgres://u:REDACTED@h/db", got)
}

func TestRedactIdempotent(t *testing.T) {
	t.Parallel()

	msg := "connect postgres://modelregistry:hunter2@db.example.com/registry " +
		"with password=s3cr3t"

	once := conditions.Redact(msg)

	assert.Equal(t, once, conditions.Redact(once))

	// The recovered, split and nested shapes are idempotent too.
	nested := "postgres://u:12345?ss@host/db?appname=http://u:p@x&password=se&cret"

	nestedOnce := conditions.Redact(nested)

	assert.Equal(t, nestedOnce, conditions.Redact(nestedOnce))
}

func TestLastTransitionTimeSemantics(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}
	conditions.SetFailed(cr, errFirstFailure)

	first := findCondition(t, cr, common.ConditionTypeDatabaseConnected)

	// A reason- or message-only change keeps the transition time.
	time.Sleep(10 * time.Millisecond)

	conditions.SetFailed(cr, errSecondFailure)

	second := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.Equal(t, first.LastTransitionTime, second.LastTransitionTime)
	assert.Equal(t, errSecondFailure.Error(), second.Message)

	// A status change updates it.
	time.Sleep(10 * time.Millisecond)

	conditions.SetConnected(cr)

	third := findCondition(t, cr, common.ConditionTypeDatabaseConnected)
	assert.True(t, third.LastTransitionTime.After(first.LastTransitionTime.Time))
}

func TestManagerAggregatesFalseDependentIntoReady(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}
	mgr := kcond.NewManager(cr,
		string(common.ConditionTypeReady),
		string(common.ConditionTypeDatabaseConnected),
	)

	mgr.MarkFalse(
		string(common.ConditionTypeDatabaseConnected),
		kcond.WithReason(common.ReasonHostUnreachable),
		kcond.WithMessage("dial tcp: connection refused"),
	)

	ready := findCondition(t, cr, common.ConditionTypeReady)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, common.ReasonHostUnreachable, ready.Reason)
}

func TestManagerInfoSeverityDoesNotAffectReady(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}
	mgr := kcond.NewManager(cr,
		string(common.ConditionTypeReady),
		string(common.ConditionTypeDatabaseConnected),
	)

	mgr.MarkFalse(
		string(common.ConditionTypeDatabaseConnected),
		kcond.WithReason(common.ReasonHostUnreachable),
		kcond.WithSeverity(common.ConditionSeverityInfo),
	)

	ready := findCondition(t, cr, common.ConditionTypeReady)
	assert.Equal(t, metav1.ConditionTrue, ready.Status)
}

func TestManagerRawHelperSkipsRecomputation(t *testing.T) {
	t.Parallel()

	cr := &componentCR{}
	mgr := kcond.NewManager(cr,
		string(common.ConditionTypeReady),
		string(common.ConditionTypeDatabaseConnected),
	)

	// SetFailed upserts through SetStatusCondition directly, not through
	// the Manager, so Ready is not recomputed.
	changed := conditions.SetFailed(cr, errDialRefused)

	assert.True(t, changed)

	dependent := mgr.GetCondition(string(common.ConditionTypeDatabaseConnected))
	require.NotNil(t, dependent)
	assert.Equal(t, metav1.ConditionFalse, dependent.Status)

	ready := findCondition(t, cr, common.ConditionTypeReady)
	assert.Equal(t, metav1.ConditionUnknown, ready.Status)
}
