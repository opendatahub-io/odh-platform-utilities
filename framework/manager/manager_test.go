package manager_test

import (
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"

	opclient "github.com/opendatahub-io/odh-platform-utilities/framework/client"
	"github.com/opendatahub-io/odh-platform-utilities/framework/manager"

	. "github.com/onsi/gomega"
)

type mockManager struct {
	ctrlmanager.Manager

	client client.Client
}

func (m *mockManager) GetClient() client.Client {
	return m.client
}

func TestNew_CreatesManager(t *testing.T) {
	g := NewWithT(t)

	mockMgr := &mockManager{client: fake.NewFakeClient()}
	mgr := manager.New(mockMgr)

	g.Expect(mgr).ShouldNot(BeNil())
}

func TestGetClient_WithoutOption_ReturnsWrappedClient(t *testing.T) {
	g := NewWithT(t)

	inner := fake.NewFakeClient()
	mockMgr := &mockManager{client: inner}
	mgr := manager.New(mockMgr)

	_, ok := mgr.GetClient().(*opclient.Client)
	g.Expect(ok).Should(BeTrue(), "default client should be *opclient.Client")
}

func TestGetClient_WithClient_ReturnsProvidedClient(t *testing.T) {
	g := NewWithT(t)

	inner := fake.NewFakeClient()
	custom := fake.NewFakeClient()
	mockMgr := &mockManager{client: inner}
	mgr := manager.New(mockMgr, manager.WithClient(custom))

	g.Expect(mgr.GetClient()).Should(BeIdenticalTo(custom))
}

func TestGetManifestsBasePath_Default(t *testing.T) {
	g := NewWithT(t)

	mockMgr := &mockManager{client: fake.NewFakeClient()}
	mgr := manager.New(mockMgr)

	g.Expect(mgr.GetManifestsBasePath()).Should(BeEmpty())
}

func TestGetManifestsBasePath_WithOption(t *testing.T) {
	g := NewWithT(t)

	mockMgr := &mockManager{client: fake.NewFakeClient()}
	mgr := manager.New(mockMgr, manager.WithManifestsBasePath("/opt/manifests"))

	g.Expect(mgr.GetManifestsBasePath()).Should(Equal("/opt/manifests"))
}

func TestGetChartsBasePath_Default(t *testing.T) {
	g := NewWithT(t)

	mockMgr := &mockManager{client: fake.NewFakeClient()}
	mgr := manager.New(mockMgr)

	g.Expect(mgr.GetChartsBasePath()).Should(BeEmpty())
}

func TestGetChartsBasePath_WithOption(t *testing.T) {
	g := NewWithT(t)

	mockMgr := &mockManager{client: fake.NewFakeClient()}
	mgr := manager.New(mockMgr, manager.WithChartsBasePath("/opt/charts"))

	g.Expect(mgr.GetChartsBasePath()).Should(Equal("/opt/charts"))
}

func TestManager_ImplementsManagerInterface(t *testing.T) {
	var _ ctrlmanager.Manager = (*manager.Manager)(nil)
}
