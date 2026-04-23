package cluster

// Platform identifies the product distribution that is deploying the
// controller. Module controllers use this to select manifest overlays,
// branding text, namespace defaults, and other platform-specific behavior.
//
// In the fully-realized module architecture, modules should prefer reading
// platform info from their projected CR config (set by the orchestrator)
// rather than calling detection directly. The detection API is a transitional
// necessity and fallback mechanism.
type Platform string

const (
	// OpenDataHub is the community Open Data Hub distribution.
	OpenDataHub Platform = "Open Data Hub"

	// SelfManagedRhoai is the self-managed OpenShift AI distribution.
	SelfManagedRhoai Platform = "OpenShift AI Self-Managed"

	// ManagedRhoai is the cloud-service (addon) OpenShift AI distribution.
	ManagedRhoai Platform = "OpenShift AI Cloud Service"

	// XKS is the platform type for non-OpenShift Kubernetes deployments
	// (AKS, CoreWeave, EKS, and similar). Set via the ODH_PLATFORM_TYPE
	// environment variable by the CCM Helm chart.
	XKS Platform = "XKS"
)

// ClusterType identifies the Kubernetes distribution at the infrastructure level.
type ClusterType string

const (
	// ClusterTypeOpenShift identifies an OpenShift cluster (ClusterVersion CRD is present).
	ClusterTypeOpenShift ClusterType = "OpenShift"

	// ClusterTypeKubernetes identifies a plain/vanilla Kubernetes cluster.
	ClusterTypeKubernetes ClusterType = "Kubernetes"
)

// ClusterInfo aggregates infrastructure-layer facts about the cluster.
type ClusterInfo struct {
	// Type is the infrastructure type (OpenShift or Kubernetes).
	Type ClusterType

	// Version is the OpenShift semantic version when Type is ClusterTypeOpenShift.
	// Zero value when the cluster is not OpenShift or version detection fails.
	Version string

	// FipsEnabled is true when the cluster was installed in FIPS mode.
	// Always false on vanilla Kubernetes (the ConfigMap is absent).
	FipsEnabled bool
}

// AuthenticationMode represents the cluster authentication mode.
// Only meaningful on OpenShift clusters.
type AuthenticationMode string

const (
	// AuthModeIntegratedOAuth is the default OpenShift authentication mode.
	AuthModeIntegratedOAuth AuthenticationMode = "IntegratedOAuth"

	// AuthModeOIDC indicates external OIDC authentication.
	AuthModeOIDC AuthenticationMode = "OIDC"

	// AuthModeNone indicates no authentication or an unknown/custom type.
	AuthModeNone AuthenticationMode = "None"
)

// OperatorInfo holds metadata about an installed OLM operator.
type OperatorInfo struct {
	// Version is the operator version extracted from the OperatorCondition
	// name (e.g. "v1.2.3"). May be empty if the version suffix is absent.
	Version string
}

const (
	// ClusterAuthenticationObj is the well-known name of the cluster-scope
	// Authentication CR on OpenShift.
	ClusterAuthenticationObj = "cluster"

	// OpenShiftVersionObj is the well-known name of the ClusterVersion CR.
	OpenShiftVersionObj = "version"
)
