package manager

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	uclient "github.com/opendatahub-io/odh-platform-utilities/framework/client"
)

type Option func(*Manager)

func WithManifestsBasePath(p string) Option {
	return func(m *Manager) {
		m.manifestsBasePath = p
	}
}

func WithChartsBasePath(p string) Option {
	return func(m *Manager) {
		m.chartsBasePath = p
	}
}

// Manager wraps a controller-runtime manager to return the cache-coherent
// Client from GetClient(). It also carries manifest and chart base path
// configuration that the reconciler reads via duck-typed interfaces.
type Manager struct {
	manager.Manager

	wrappedClient     *uclient.Client
	manifestsBasePath string
	chartsBasePath    string
}

func New(mgr manager.Manager, opts ...Option) *Manager {
	wrappedClient := uclient.New(mgr.GetClient())

	m := &Manager{
		Manager:       mgr,
		wrappedClient: wrappedClient,
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

func (m *Manager) GetClient() client.Client {
	return m.wrappedClient
}

func (m *Manager) GetManifestsBasePath() string {
	return m.manifestsBasePath
}

func (m *Manager) GetChartsBasePath() string {
	return m.chartsBasePath
}
