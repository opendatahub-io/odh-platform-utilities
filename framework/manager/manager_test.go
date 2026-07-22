package manager_test

import (
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"

	uclient "github.com/opendatahub-io/odh-platform-utilities/framework/client"
	"github.com/opendatahub-io/odh-platform-utilities/framework/manager"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientFake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/onsi/gomega"
)

type mockManager struct {
	ctrlmanager.Manager

	client client.Client
}

func (m *mockManager) GetClient() client.Client {
	return m.client
}

func newMockManager() *mockManager {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)

	return &mockManager{
		client: clientFake.NewClientBuilder().
			WithScheme(s).
			Build(),
	}
}

func TestNew_CreatesManager(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	wrappedMgr := manager.New(newMockManager())

	g.Expect(wrappedMgr).ShouldNot(BeNil())
}

func TestGetClient_ReturnsWrappedClient(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	wrappedMgr := manager.New(newMockManager())
	returnedClient := wrappedMgr.GetClient()

	_, ok := returnedClient.(*uclient.Client)
	g.Expect(ok).Should(BeTrue(), "GetClient should return *uclient.Client")
}

func TestManager_ManifestsBasePath(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	wrappedMgr := manager.New(newMockManager(), manager.WithManifestsBasePath("/opt/manifests"))

	g.Expect(wrappedMgr.GetManifestsBasePath()).Should(Equal("/opt/manifests"))
}

func TestManager_ChartsBasePath(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	wrappedMgr := manager.New(newMockManager(), manager.WithChartsBasePath("/opt/charts"))

	g.Expect(wrappedMgr.GetChartsBasePath()).Should(Equal("/opt/charts"))
}

func TestManager_DefaultPaths(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	wrappedMgr := manager.New(newMockManager())

	g.Expect(wrappedMgr.GetManifestsBasePath()).Should(BeEmpty())
	g.Expect(wrappedMgr.GetChartsBasePath()).Should(BeEmpty())
}
