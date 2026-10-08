package webhook_test

import (
	"bytes"
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/opendatahub-io/odh-platform-utilities/framework/webhook"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestEnvtestAdmission(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	const path = "/validate-configmaps"
	webhookPath, mutatePath := path, "/mutate-configmaps"
	fail, none := admissionregv1.Fail, admissionregv1.SideEffectClassNone
	configMapRule := admissionregv1.Rule{
		APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"configmaps"},
	}
	assetsDir, err := filepath.Abs(filepath.Join("..", "bin", "envtest"))
	g.Expect(err).To(Succeed())
	env := &envtest.Environment{DownloadBinaryAssets: true, DownloadBinaryAssetsVersion: "1.35.0", BinaryAssetsDirectory: assetsDir, WebhookInstallOptions: envtest.WebhookInstallOptions{
		MutatingWebhooks: []*admissionregv1.MutatingWebhookConfiguration{{
			ObjectMeta: metav1.ObjectMeta{Name: "configmap-mutation-test"},
			Webhooks: []admissionregv1.MutatingWebhook{{
				Name: "configmap-mutation.test.example.com",
				ClientConfig: admissionregv1.WebhookClientConfig{Service: &admissionregv1.ServiceReference{
					Name: "unused", Namespace: "default", Path: &mutatePath,
				}},
				Rules: []admissionregv1.RuleWithOperations{{
					Operations: []admissionregv1.OperationType{admissionregv1.Create}, Rule: configMapRule,
				}},
				FailurePolicy: &fail, SideEffects: &none, AdmissionReviewVersions: []string{"v1"},
			}},
		}},
		ValidatingWebhooks: []*admissionregv1.ValidatingWebhookConfiguration{{
			ObjectMeta: metav1.ObjectMeta{Name: "configmap-validation-test"},
			Webhooks: []admissionregv1.ValidatingWebhook{{
				Name: "configmap-validation.test.example.com",
				ClientConfig: admissionregv1.WebhookClientConfig{Service: &admissionregv1.ServiceReference{
					Name: "unused", Namespace: "default", Path: &webhookPath,
				}},
				Rules: []admissionregv1.RuleWithOperations{{
					Operations: []admissionregv1.OperationType{admissionregv1.Create, admissionregv1.Update, admissionregv1.Delete},
					Rule:       configMapRule,
				}},
				FailurePolicy: &fail, SideEffects: &none, AdmissionReviewVersions: []string{"v1"},
			}},
		}},
	}}
	cfg, err := env.Start()
	g.Expect(err).To(Succeed())
	t.Cleanup(func() { g.Expect(env.Stop()).To(Succeed()) })

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Host:    env.WebhookInstallOptions.LocalServingHost,
			Port:    env.WebhookInstallOptions.LocalServingPort,
			CertDir: env.WebhookInstallOptions.LocalServingCertDir,
		}),
	})
	g.Expect(err).To(Succeed())
	g.Expect(webhook.For(mgr, path, corev1.SchemeGroupVersion.WithKind("ConfigMap")).OnCreate(func(ctx context.Context, req admission.Request, reader client.Reader) admission.Response {
		if req.Name == "blocked" {
			return admission.Denied("blocked by test webhook")
		}
		if err := reader.Get(ctx, client.ObjectKey{Name: "default"}, &corev1.Namespace{}); err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		return admission.Allowed("").WithWarnings("allowed by test webhook")
	}).OnCreate(func(context.Context, admission.Request, client.Reader) admission.Response {
		return admission.Allowed("").WithWarnings("second check passed")
	}).OnUpdate(func(context.Context, admission.Request, client.Reader) admission.Response {
		return admission.Denied("update blocked by test webhook")
	}).OnDelete(func(context.Context, admission.Request, client.Reader) admission.Response {
		return admission.Denied("delete blocked by test webhook")
	}).Build()).To(Succeed())
	g.Expect(webhook.Defaulting[*corev1.ConfigMap](mgr, mutatePath).WithDefault(func(_ context.Context, obj *corev1.ConfigMap, _ client.Reader) error {
		if obj.Name == "mutated" {
			obj.Data = map[string]string{"injected": "yes"}
		}
		return nil
	}).Build()).To(Succeed())

	ctx, cancel := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	go func() { serverDone <- mgr.GetWebhookServer().Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		g.Expect(<-serverDone).To(Succeed())
	})
	started := mgr.GetWebhookServer().StartedChecker()
	g.Eventually(func() bool {
		return started(&http.Request{}) == nil
	}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

	var warnings bytes.Buffer
	clientConfig := *cfg
	clientConfig.WarningHandler = rest.NewWarningWriter(&warnings, rest.WarningWriterOptions{})
	k8sClient, err := client.New(&clientConfig, client.Options{Scheme: scheme})
	g.Expect(err).To(Succeed())
	allowed := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "allowed", Namespace: "default"}}
	g.Expect(k8sClient.Create(ctx, allowed)).To(Succeed())
	g.Expect(warnings.String()).To(ContainSubstring("allowed by test webhook"))
	g.Expect(warnings.String()).To(ContainSubstring("second check passed"))
	mutated := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "mutated", Namespace: "default"}}
	g.Expect(k8sClient.Create(ctx, mutated)).To(Succeed())
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mutated), mutated)).To(Succeed())
	g.Expect(mutated.Data).To(HaveKeyWithValue("injected", "yes"))
	err = k8sClient.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "blocked", Namespace: "default"}})
	g.Expect(err).To(HaveOccurred())
	g.Expect(apierrors.IsForbidden(err)).To(BeTrue())
	g.Expect(err.Error()).To(ContainSubstring("blocked by test webhook"))
	allowed.Data = map[string]string{"change": "blocked"}
	err = k8sClient.Update(ctx, allowed)
	g.Expect(err).To(HaveOccurred())
	g.Expect(apierrors.IsForbidden(err)).To(BeTrue())
	g.Expect(err.Error()).To(ContainSubstring("update blocked by test webhook"))
	err = k8sClient.Delete(ctx, allowed)
	g.Expect(err).To(HaveOccurred())
	g.Expect(apierrors.IsForbidden(err)).To(BeTrue())
	g.Expect(err.Error()).To(ContainSubstring("delete blocked by test webhook"))
	stored := &corev1.ConfigMap{}
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(allowed), stored)).To(Succeed())
	g.Expect(stored.Data).NotTo(HaveKey("change"))
}
