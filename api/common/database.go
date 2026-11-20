package common

// --- Database Connection Condition ---

// ConditionTypeDatabaseConnected reports a component's connectivity to its
// database. A component that consumes a database claim Secret sets this
// condition on its own CR.
//
// This is an optional, recommended condition type. The orchestrator does
// not require it and does not evaluate it as part of the mandatory
// PlatformObject contract; only ConditionTypeReady and
// ConditionTypeProvisioningSucceeded are mandatory.
const ConditionTypeDatabaseConnected ConditionType = "DatabaseConnected"

// --- Database Connection Condition Reasons ---

// Reasons for ConditionTypeDatabaseConnected, matching the untyped Reason
// field of Condition.
const (
	// ReasonConnectionSucceeded indicates the connection attempt succeeded.
	ReasonConnectionSucceeded = "ConnectionSucceeded"

	// ReasonConnectionFailed indicates the connection attempt failed for a
	// reason that could not be classified further.
	ReasonConnectionFailed = "ConnectionFailed"

	// ReasonCredentialsUnavailable indicates the claim Secret is missing or
	// does not carry the required keys, so no connection was attempted.
	ReasonCredentialsUnavailable = "CredentialsUnavailable"

	// ReasonCredentialsRejected indicates the server rejected the
	// credentials, for example PostgreSQL SQLSTATE 28P01. Standard library
	// error inspection cannot detect this, so callers set it explicitly.
	ReasonCredentialsRejected = "CredentialsRejected"

	// ReasonHostUnreachable indicates a network-level failure reaching the
	// database host, such as DNS resolution failure, connection refused, or
	// a network-level timeout.
	ReasonHostUnreachable = "HostUnreachable"

	// ReasonTLSVerificationFailed indicates the TLS handshake or
	// certificate verification failed.
	ReasonTLSVerificationFailed = "TLSVerificationFailed"
)
