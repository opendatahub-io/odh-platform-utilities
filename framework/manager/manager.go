package manager

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	opclient "github.com/opendatahub-io/odh-platform-utilities/framework/client"
)

var _ manager.Manager = (*Manager)(nil)

// Option configures a Manager.
type Option func(*Manager)

// WithClient sets a custom client to be returned by GetClient.
func WithClient(c client.Client) Option {
	return func(m *Manager) {
		m.wrappedClient = c
	}
}

// WithManifestsBasePath sets the base path for component manifests.
func WithManifestsBasePath(p string) Option {
	return func(m *Manager) {
		m.manifestsBasePath = p
	}
}

// WithChartsBasePath sets the base path for Helm charts.
func WithChartsBasePath(p string) Option {
	return func(m *Manager) {
		m.chartsBasePath = p
	}
}

// Manager wraps a controller-runtime manager, allowing callers to override
// the client returned by GetClient and to attach base paths for manifest
// and chart resolution.
type Manager struct {
	manager.Manager

	wrappedClient     client.Client
	manifestsBasePath string
	chartsBasePath    string
}

// New creates a Manager that wraps the given controller-runtime manager.
// By default, the inner manager's client is wrapped with an unstructured
// caching client. Use WithClient to override with a different client.
func New(mgr manager.Manager, opts ...Option) *Manager {
	m := &Manager{
		Manager:       mgr,
		wrappedClient: opclient.New(mgr.GetClient()),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// GetClient returns the wrapped client.
func (m *Manager) GetClient() client.Client {
	return m.wrappedClient
}

// GetManifestsBasePath returns the base path for component manifests.
func (m *Manager) GetManifestsBasePath() string {
	return m.manifestsBasePath
}

// GetChartsBasePath returns the base path for Helm charts.
func (m *Manager) GetChartsBasePath() string {
	return m.chartsBasePath
}
