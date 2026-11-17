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

// A real dial with the dialer's own timeout: on the Go versions this
// module targets, the resulting network error wraps the same sentinel as
// context.DeadlineExceeded, which the synthetic cases above cannot
// reproduce. The classification must be HostUnreachable whether the
// environment yields a timeout or an unreachable-route error; the dial is
// bounded by the dialer timeout.
func TestReasonForErrorClassifiesDialerTimeout(t *testing.T) {
	t.Parallel()

	d := &net.Dialer{Timeout: 500 * time.Millisecond}

	_, err := d.Dial("tcp", "192.0.2.1:5432") // TEST-NET-1, RFC 5737.
	require.Error(t, err)
	assert.Equal(t, common.ReasonHostUnreachable, conditions.ReasonForError(err))
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
		{
			// Conservative tokenization: the query parameters following
			// the password are consumed with it.
			name: "URI query parameter",
			msg:  "postgres://db.example.com/registry?password=abc123&sslmode=require",
			want: "postgres://db.example.com/registry?password=REDACTED",
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.msg, conditions.Redact(tc.msg))
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
