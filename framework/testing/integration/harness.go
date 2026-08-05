// framework/testing/integration/harness.go
package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/onsi/gomega"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/cluster/gvk"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
)

// scheme registers core Kubernetes types (including apps/v1 Deployment).
// Used for typed client operations.
var scheme = func() *runtime.Scheme {
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		panic(err)
	}
	return s
}()

// decoder is used to parse multi-document YAML manifests.
var decoder = clientgoscheme.Codecs.UniversalDeserializer()

// config holds validated integration test configuration.
// Unexported to prevent mutation after construction; use functional options.
type config struct {
	kubeconfig           string
	operatorManifest     resources.Source
	dscName              string
	dscSpec              *DSCSpec
	moduleGVK            schema.GroupVersionKind
	moduleName           string
	moduleNamespace      string
	moduleAlreadyEnabled bool
	deploymentName       string
	timeout              time.Duration
	pollInterval         time.Duration
}

const (
	// DefaultDSCName is used when [WithDSCName] is omitted.
	DefaultDSCName = "default-dsc"
	// DefaultModuleNamespace is used when [WithModuleNamespace] is omitted.
	DefaultModuleNamespace = "opendatahub"
	// DefaultTimeout is the per-assertion wait when [WithTimeout] is omitted.
	DefaultTimeout = 5 * time.Minute
	// DefaultPollInterval is used when [WithPollInterval] is omitted.
	DefaultPollInterval = 5 * time.Second
)

// Option configures optional aspects of an integration test.
type Option func(*config)

// WithKubeconfig overrides KUBECONFIG. Required if that env var is unset.
func WithKubeconfig(path string) Option {
	return func(c *config) { c.kubeconfig = path }
}

// WithOperatorManifest SSA-applies this YAML before creating the DSC.
// The source must be multi-document Kubernetes YAML (CRDs, RBAC,
// Deployment, …) — the same shape as `kustomize build config/default`
// from a checkout of opendatahub-operator. Accepts [resources.NewFileSource]
// (absolute path) or [resources.NewURLSource] (HTTPS URL) for disposable
// clusters. Do not pass a channel name, quay image, GitHub tag, or PR URL
// — operator releases do not attach an install.yaml.
//
// Omit this option on the PR-gate path: install the operator in CI (OLM),
// then call [Run] without this option. Teardown does not uninstall the
// operator, so this does not belong on a shared gate cluster.
func WithOperatorManifest(src resources.Source) Option {
	return func(c *config) { c.operatorManifest = src }
}

// WithDSCName overrides [DefaultDSCName].
func WithDSCName(name string) Option {
	return func(c *config) { c.dscName = name }
}

// WithDSCSpec sets the DataScienceCluster spec. Omit to skip DSC creation.
func WithDSCSpec(spec *DSCSpec) Option {
	return func(c *config) { c.dscSpec = spec }
}

// WithModuleCR asserts Ready=True and ProvisioningSucceeded=True on this
// cluster-scoped singleton CR. Omit to skip module CR checks.
// Requires [WithDSCSpec] unless [WithModuleAlreadyEnabled] is also set.
func WithModuleCR(gvk schema.GroupVersionKind, name string) Option {
	return func(c *config) {
		c.moduleGVK = gvk
		c.moduleName = name
	}
}

// WithTimeout overrides [DefaultTimeout].
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithPollInterval overrides [DefaultPollInterval].
func WithPollInterval(d time.Duration) Option {
	return func(c *config) { c.pollInterval = d }
}

// WithModuleNamespace overrides [DefaultModuleNamespace].
func WithModuleNamespace(ns string) Option {
	return func(c *config) { c.moduleNamespace = ns }
}

// WithModuleAlreadyEnabled declares that [WithDSCSpec] is not required even
// though [WithModuleCR] is set. Use this when:
//   - the module was enabled via DSC before this test run, or
//   - the module is not a DSC component (standalone operator with its own CR).
func WithModuleAlreadyEnabled() Option {
	return func(c *config) { c.moduleAlreadyEnabled = true }
}

// validate fails fast on missing required fields before any cluster interaction.
func (c *config) validate(t *testing.T) {
	t.Helper()
	if c.kubeconfig == "" {
		t.Fatal("kubeconfig required: set KUBECONFIG env or use WithKubeconfig")
	}
	if c.deploymentName == "" {
		t.Fatal("deploymentName is required (first argument to Run)")
	}
	if (c.moduleGVK.Kind != "") != (c.moduleName != "") {
		t.Fatal("WithModuleCR: both GVK (with non-empty Kind) and name must be set together")
	}
	if c.moduleGVK.Kind != "" && c.dscSpec == nil && !c.moduleAlreadyEnabled {
		t.Fatal("WithModuleCR: requires WithDSCSpec to enable the module via DSC, or WithModuleAlreadyEnabled if the module is pre-provisioned or not a DSC component")
	}
	if c.dscSpec != nil && len(c.dscSpec.components) == 0 {
		t.Fatal("WithDSCSpec: DSCSpec has no components; add at least one with .Component()")
	}
	if c.timeout <= 0 {
		t.Fatal("WithTimeout: timeout must be positive")
	}
	if c.pollInterval <= 0 {
		t.Fatal("WithPollInterval: poll interval must be positive")
	}
}

// buildClient constructs a controller-runtime client from cfg.Kubeconfig.
func buildClient(t *testing.T, kubeconfig string) client.Client {
	t.Helper()
	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("build kubeconfig: %v", err)
	}
	c, err := client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("build k8s client: %v", err)
	}
	return c
}

// applyManifests loads content from src and applies all YAML documents via server-side apply.
func applyManifests(ctx context.Context, c client.Client, src resources.Source) error {
	raw, err := src.Load(ctx)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	objs, err := resources.Decode(decoder, raw)
	if err != nil {
		return fmt.Errorf("decode manifests: %w", err)
	}

	for i := range objs {
		if err := resources.Apply(ctx, c, &objs[i],
			client.ForceOwnership, client.FieldOwner("integration-test")); err != nil {
			return fmt.Errorf("apply %s %s: %w", objs[i].GetKind(), objs[i].GetName(), err)
		}
	}
	return nil
}

// createDSC creates a DataScienceCluster CR with the given spec.
// Returns an error (including AlreadyExists) if a DSC with that name already
// exists — callers must not overwrite a pre-existing cluster-scoped resource.
func createDSC(ctx context.Context, c client.Client, cfg *config) error {
	obj := resources.GvkToUnstructured(gvk.DataScienceCluster)
	obj.SetName(cfg.dscName)
	obj.Object["spec"] = cfg.dscSpec.ToMap()
	if err := c.Create(ctx, obj); err != nil {
		return fmt.Errorf("create DSC %s: %w", cfg.dscName, err)
	}
	return nil
}

// assertModuleReady asserts (via g) that the module CR has Ready=True and
// ProvisioningSucceeded=True. Called inside gomega.Eventually.
func assertModuleReady(ctx context.Context, g gomega.Gomega, c client.Client, cfg *config) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(cfg.moduleGVK)
	g.Expect(c.Get(ctx, client.ObjectKey{Name: cfg.moduleName}, u)).To(gomega.Succeed())

	statusRaw, _, err := unstructured.NestedMap(u.Object, "status")
	g.Expect(err).NotTo(gomega.HaveOccurred(), "reading status from %s %s", cfg.moduleGVK.Kind, cfg.moduleName)

	var status api.Status
	g.Expect(runtime.DefaultUnstructuredConverter.FromUnstructured(statusRaw, &status)).To(gomega.Succeed())

	g.Expect(conditions.IsStatusConditionTrue(&status, string(common.ConditionTypeReady))).To(gomega.BeTrue(),
		"module %s/%s: Ready condition is not True", cfg.moduleGVK.Kind, cfg.moduleName)
	g.Expect(conditions.IsStatusConditionTrue(&status, string(common.ConditionTypeProvisioningSucceeded))).To(gomega.BeTrue(),
		"module %s/%s: ProvisioningSucceeded condition is not True", cfg.moduleGVK.Kind, cfg.moduleName)
}

// assertDeploymentReady asserts (via g) that cfg.deploymentName in cfg.moduleNamespace
// has readyReplicas >= 1. Called inside gomega.Eventually.
func assertDeploymentReady(ctx context.Context, g gomega.Gomega, c client.Client, cfg *config) {
	dep := &appsv1.Deployment{}
	g.Expect(c.Get(ctx, client.ObjectKey{Namespace: cfg.moduleNamespace, Name: cfg.deploymentName}, dep)).To(gomega.Succeed())
	g.Expect(dep.Status.ReadyReplicas).To(gomega.BeNumerically(">=", 1),
		"Deployment %s/%s: readyReplicas < 1", cfg.moduleNamespace, cfg.deploymentName)
}

// teardown sets the module CR to managementState=Removed, waits for the
// module deployment to scale down, then deletes the DSC.
// ctx carries a cleanup-specific deadline. Errors are logged, not fatal.
// dscCreated must be true for teardown to wait and delete — it is never
// deleted if this harness did not create it.
func teardown(ctx context.Context, t *testing.T, c client.Client, cfg *config, dscCreated bool) {
	t.Helper()

	// Patch module CR to Removed if one was configured.
	if cfg.moduleGVK.Kind != "" && cfg.moduleName != "" {
		patch := &unstructured.Unstructured{}
		patch.SetGroupVersionKind(cfg.moduleGVK)
		patch.SetName(cfg.moduleName)
		if err := c.Patch(ctx, patch,
			client.RawPatch(apimachinerytypes.MergePatchType, []byte(`{"spec":{"managementState":"Removed"}}`)),
		); err != nil && !k8serr.IsNotFound(err) {
			t.Logf("teardown: patch module CR: %v", err)
		}
	}

	if dscCreated {
		// Wait for the module deployment to scale down before deleting the DSC.
		// Without this wait, the next run can race with a still-terminating module.
		if cfg.moduleGVK.Kind != "" {
			t.Logf("teardown: waiting for Deployment %s/%s to scale down", cfg.moduleNamespace, cfg.deploymentName)
			g := gomega.NewWithT(t)
			g.Eventually(func(g gomega.Gomega) {
				dep := &appsv1.Deployment{}
				err := c.Get(ctx, client.ObjectKey{Namespace: cfg.moduleNamespace, Name: cfg.deploymentName}, dep)
				if k8serr.IsNotFound(err) {
					return
				}
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(dep.Status.ReadyReplicas).To(gomega.BeZero(),
					"Deployment %s/%s: still has ready replicas", cfg.moduleNamespace, cfg.deploymentName)
			}).WithContext(ctx).WithPolling(cfg.pollInterval).Should(gomega.Succeed())
		}

		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(gvk.DataScienceCluster)
		obj.SetName(cfg.dscName)
		if err := c.Delete(ctx, obj); err != nil && !k8serr.IsNotFound(err) {
			t.Logf("teardown: delete DSC: %v", err)
		}
	}
}

// Run executes the integration scenario against a real cluster.
//
// deploymentName is required (compiler-enforced): that Deployment must reach
// readyReplicas >= 1. All other inputs are [Option]s; omitted options use
// Default* values or skip their step (see each With* comment).
//
// Teardown (module CR managementState=Removed, delete DSC) runs via t.Cleanup
// even if assertions fail.
func Run(t *testing.T, deploymentName string, opts ...Option) {
	t.Helper()

	cfg := &config{
		kubeconfig:      os.Getenv("KUBECONFIG"),
		moduleNamespace: DefaultModuleNamespace,
		deploymentName:  deploymentName,
		dscName:         DefaultDSCName,
		timeout:         DefaultTimeout,
		pollInterval:    DefaultPollInterval,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	cfg.validate(t)

	c := buildClient(t, cfg.kubeconfig)

	dscCreated := false
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cfg.timeout)
		defer cleanupCancel()
		teardown(cleanupCtx, t, c, cfg, dscCreated)
	})

	if cfg.operatorManifest != nil {
		t.Log("phase: applying operator manifests — SSA with ForceOwnership; use only on disposable clusters")
		applyCtx, applyCancel := context.WithTimeout(context.Background(), cfg.timeout)
		err := applyManifests(applyCtx, c, cfg.operatorManifest)
		applyCancel()
		if err != nil {
			t.Fatalf("deploy operator: %v", err)
		}
	}

	if cfg.dscSpec != nil {
		t.Logf("phase: creating DataScienceCluster %s", cfg.dscName)
		dscCtx, dscCancel := context.WithTimeout(context.Background(), cfg.timeout)
		err := createDSC(dscCtx, c, cfg)
		dscCancel()
		if err != nil {
			t.Fatalf("create DSC: %v", err)
		}
		dscCreated = true
	}

	g := gomega.NewWithT(t)

	if cfg.moduleGVK.Kind != "" && cfg.moduleName != "" {
		t.Logf("phase: waiting for %s/%s Ready=True + ProvisioningSucceeded=True", cfg.moduleGVK.Kind, cfg.moduleName)
		moduleCtx, moduleCancel := context.WithTimeout(context.Background(), cfg.timeout)
		defer moduleCancel()
		g.Eventually(func(g gomega.Gomega) {
			assertModuleReady(moduleCtx, g, c, cfg)
		}).WithContext(moduleCtx).WithPolling(cfg.pollInterval).Should(gomega.Succeed())
	}

	t.Logf("phase: waiting for Deployment %s/%s readyReplicas >= 1", cfg.moduleNamespace, cfg.deploymentName)
	depCtx, depCancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer depCancel()
	g.Eventually(func(g gomega.Gomega) {
		assertDeploymentReady(depCtx, g, c, cfg)
	}).WithContext(depCtx).WithPolling(cfg.pollInterval).Should(gomega.Succeed())

	t.Log("phase: all assertions passed")
}
