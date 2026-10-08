package webhook_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/gomega"
	"github.com/opendatahub-io/odh-platform-utilities/framework/webhook"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type testServer struct {
	ctrlwebhook.Server

	path string
	hook http.Handler
}

func (s *testServer) Register(path string, hook http.Handler) {
	s.path, s.hook = path, hook
}

type testManager struct {
	ctrl.Manager

	scheme *runtime.Scheme
	reader client.Reader
	server *testServer
}

func (m *testManager) GetScheme() *runtime.Scheme           { return m.scheme }
func (m *testManager) GetAPIReader() client.Reader          { return m.reader }
func (m *testManager) GetWebhookServer() ctrlwebhook.Server { return m.server }

func newManager(t *testing.T) *testManager {
	t.Helper()
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	return &testManager{
		scheme: scheme,
		reader: fake.NewClientBuilder().WithScheme(scheme).Build(),
		server: &testServer{},
	}
}

func registeredHook(t *testing.T, mgr *testManager, path string) *admission.Webhook {
	t.Helper()
	g := NewWithT(t)
	g.Expect(mgr.server.path).To(Equal(path))
	hook, ok := mgr.server.hook.(*admission.Webhook)
	g.Expect(ok).To(BeTrue())
	return hook
}

func request(t *testing.T, operation admissionv1.Operation) admission.Request {
	t.Helper()
	g := NewWithT(t)
	obj, err := json.Marshal(&corev1.ConfigMap{})
	g.Expect(err).To(Succeed())
	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID: types.UID("test"), Operation: operation,
		Kind:   metav1.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
		Object: runtime.RawExtension{Raw: obj}, OldObject: runtime.RawExtension{Raw: obj},
	}}
}

func TestRawHandler(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	mgr := newManager(t)
	called := false
	err := webhook.For(mgr, "/validate-example", corev1.SchemeGroupVersion.WithKind("ConfigMap")).WithName("example-validator").
		WithHandler(func(_ context.Context, _ admission.Request, reader client.Reader, decoder admission.Decoder) admission.Response {
			g.Expect(reader).To(BeIdenticalTo(mgr.reader))
			g.Expect(decoder).NotTo(BeNil())
			called = true
			return admission.Allowed("")
		}).Build()
	g.Expect(err).To(Succeed())
	hook := registeredHook(t, mgr, "/validate-example")
	g.Expect(hook.LogConstructor).NotTo(BeNil())
	g.Expect(hook.Handle(t.Context(), request(t, admissionv1.Create)).Allowed).To(BeTrue())
	g.Expect(called).To(BeTrue())
}

func TestWebhookLogName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		override string
		want     string
	}{
		{name: "path by default", want: "/validate-example"},
		{name: "override", override: "example-validator", want: "example-validator"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			mgr := newManager(t)
			builder := webhook.For(mgr, "/validate-example", corev1.SchemeGroupVersion.WithKind("ConfigMap")).OnCreate(func(context.Context, admission.Request, client.Reader) admission.Response {
				return admission.Allowed("")
			})
			if tc.override != "" {
				builder.WithName(tc.override)
			}
			g.Expect(builder.Build()).To(Succeed())
			var output string
			logger := funcr.NewJSON(func(line string) { output = line }, funcr.Options{})
			req := request(t, admissionv1.Create)
			req.UserInfo.Username = "sensitive-user"
			registeredHook(t, mgr, "/validate-example").LogConstructor(logger, &req).Info("test")
			var fields map[string]any
			g.Expect(json.Unmarshal([]byte(output), &fields)).To(Succeed())
			g.Expect(fields["webhook"]).To(Equal(tc.want))
			g.Expect(fields).NotTo(HaveKey("user"))
			g.Expect(output).NotTo(ContainSubstring("sensitive-user"))
		})
	}
}

func TestOnCreate(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	mgr := newManager(t)
	called := 0
	err := webhook.For(mgr, "/create-only", corev1.SchemeGroupVersion.WithKind("ConfigMap")).OnCreate(func(_ context.Context, req admission.Request, reader client.Reader) admission.Response {
		g.Expect(req.Operation).To(Equal(admissionv1.Create))
		g.Expect(reader).To(BeIdenticalTo(mgr.reader))
		called++
		return admission.Denied("already exists")
	}).Build()
	g.Expect(err).To(Succeed())
	hook := registeredHook(t, mgr, "/create-only")
	g.Expect(hook.Handle(t.Context(), request(t, admissionv1.Create)).Allowed).To(BeFalse())
	g.Expect(hook.Handle(t.Context(), request(t, admissionv1.Update)).Allowed).To(BeTrue())
	g.Expect(called).To(Equal(1))
}

func TestOperationCallbacks(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	mgr := newManager(t)
	var called []admissionv1.Operation
	handle := func(_ context.Context, req admission.Request, reader client.Reader) admission.Response {
		g.Expect(reader).To(BeIdenticalTo(mgr.reader))
		called = append(called, req.Operation)
		return admission.Denied(string(req.Operation))
	}
	err := webhook.For(mgr, "/operations", corev1.SchemeGroupVersion.WithKind("ConfigMap")).
		OnCreate(handle).OnUpdate(handle).OnDelete(handle).OnConnect(handle).Build()
	g.Expect(err).To(Succeed())
	hook := registeredHook(t, mgr, "/operations")
	operations := []admissionv1.Operation{admissionv1.Create, admissionv1.Update, admissionv1.Delete, admissionv1.Connect}
	for _, operation := range operations {
		g.Expect(hook.Handle(t.Context(), request(t, operation)).Allowed).To(BeFalse())
	}
	g.Expect(called).To(Equal(operations))
}

func TestRepeatedOperationCallbacks(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	mgr := newManager(t)
	called := 0
	first := func(context.Context, admission.Request, client.Reader) admission.Response {
		called++
		return admission.PatchResponseFromRaw([]byte(`{}`), []byte(`{"first":true}`)).WithWarnings("first")
	}
	second := func(context.Context, admission.Request, client.Reader) admission.Response {
		g.Expect(called).To(Equal(1))
		called++
		return admission.PatchResponseFromRaw([]byte(`{}`), []byte(`{"second":true}`)).WithWarnings("second")
	}
	g.Expect(webhook.For(mgr, "/multi", corev1.SchemeGroupVersion.WithKind("ConfigMap")).OnCreate(first).OnCreate(second).Build()).To(Succeed())
	resp := registeredHook(t, mgr, "/multi").Handle(t.Context(), request(t, admissionv1.Create))
	g.Expect(resp.Allowed).To(BeTrue())
	g.Expect(resp.Warnings).To(Equal([]string{"first", "second"}))
	g.Expect(resp.PatchType).NotTo(BeNil())
	g.Expect(*resp.PatchType).To(Equal(admissionv1.PatchTypeJSONPatch))
	var patches []map[string]any
	g.Expect(json.Unmarshal(resp.Patch, &patches)).To(Succeed())
	g.Expect(patches).To(Equal([]map[string]any{
		{"op": "add", "path": "/first", "value": true},
		{"op": "add", "path": "/second", "value": true},
	}))
	g.Expect(called).To(Equal(2))

	mgr = newManager(t)
	g.Expect(webhook.For(mgr, "/no-patch", corev1.SchemeGroupVersion.WithKind("ConfigMap")).
		OnCreate(func(context.Context, admission.Request, client.Reader) admission.Response {
			return admission.Allowed("")
		}).
		OnCreate(func(context.Context, admission.Request, client.Reader) admission.Response {
			return admission.Allowed("")
		}).Build()).To(Succeed())
	resp = registeredHook(t, mgr, "/no-patch").Handle(t.Context(), request(t, admissionv1.Create))
	g.Expect(resp.Allowed).To(BeTrue())
	g.Expect(resp.Patch).To(BeEmpty())
	g.Expect(resp.PatchType).To(BeNil())

	mgr = newManager(t)
	called = 0
	g.Expect(webhook.For(mgr, "/deny", corev1.SchemeGroupVersion.WithKind("ConfigMap")).
		OnCreate(func(context.Context, admission.Request, client.Reader) admission.Response {
			called++
			return admission.Allowed("")
		}).
		OnCreate(func(context.Context, admission.Request, client.Reader) admission.Response {
			called++
			return admission.Denied("stop")
		}).
		OnCreate(func(context.Context, admission.Request, client.Reader) admission.Response {
			called++
			return admission.Allowed("")
		}).Build()).To(Succeed())
	resp = registeredHook(t, mgr, "/deny").Handle(t.Context(), request(t, admissionv1.Create))
	g.Expect(resp.Allowed).To(BeFalse())
	g.Expect(resp.Result.Message).To(Equal("stop"))
	g.Expect(called).To(Equal(2))
}

func TestTypedCallbacks(t *testing.T) {
	t.Parallel()

	t.Run("validation and omitted operation", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		mgr := newManager(t)
		called := false
		err := webhook.Validating[*corev1.ConfigMap](mgr, "/validate-configmap").
			OnCreate(func(_ context.Context, obj *corev1.ConfigMap, reader client.Reader) (admission.Warnings, error) {
				g.Expect(obj).NotTo(BeNil())
				g.Expect(reader).To(BeIdenticalTo(mgr.reader))
				called = true
				return admission.Warnings{"checked"}, nil
			}).Build()
		g.Expect(err).To(Succeed())
		hook := registeredHook(t, mgr, "/validate-configmap")
		resp := hook.Handle(t.Context(), request(t, admissionv1.Create))
		g.Expect(resp.Allowed).To(BeTrue())
		g.Expect(resp.Warnings).To(Equal([]string{"checked"}))
		g.Expect(called).To(BeTrue())
		called = false
		g.Expect(hook.Handle(t.Context(), request(t, admissionv1.Update)).Allowed).To(BeTrue())
		g.Expect(called).To(BeFalse())
	})

	t.Run("defaulting", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		mgr := newManager(t)
		called := false
		err := webhook.Defaulting[*corev1.ConfigMap](mgr, "/mutate-configmap").
			WithDefault(func(_ context.Context, obj *corev1.ConfigMap, reader client.Reader) error {
				g.Expect(reader).To(BeIdenticalTo(mgr.reader))
				obj.Data = map[string]string{"defaulted": "yes"}
				called = true
				return nil
			}).Build()
		g.Expect(err).To(Succeed())
		hook := registeredHook(t, mgr, "/mutate-configmap")
		resp := hook.Handle(t.Context(), request(t, admissionv1.Create))
		g.Expect(resp.Allowed).To(BeTrue())
		g.Expect(called).To(BeTrue())
		g.Expect(resp.Patches).NotTo(BeEmpty())
	})

	t.Run("update and delete", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		mgr := newManager(t)
		updated, deleted := false, false
		err := webhook.Validating[*corev1.ConfigMap](mgr, "/validate-changes").
			OnUpdate(func(_ context.Context, oldObj, newObj *corev1.ConfigMap, reader client.Reader) (admission.Warnings, error) {
				g.Expect(oldObj).NotTo(BeNil())
				g.Expect(newObj).NotTo(BeNil())
				g.Expect(reader).To(BeIdenticalTo(mgr.reader))
				updated = true
				return nil, nil
			}).
			OnDelete(func(_ context.Context, obj *corev1.ConfigMap, reader client.Reader) (admission.Warnings, error) {
				g.Expect(obj).NotTo(BeNil())
				g.Expect(reader).To(BeIdenticalTo(mgr.reader))
				deleted = true
				return nil, nil
			}).Build()
		g.Expect(err).To(Succeed())
		hook := registeredHook(t, mgr, "/validate-changes")
		g.Expect(hook.Handle(t.Context(), request(t, admissionv1.Update)).Allowed).To(BeTrue())
		g.Expect(hook.Handle(t.Context(), request(t, admissionv1.Delete)).Allowed).To(BeTrue())
		g.Expect(updated).To(BeTrue())
		g.Expect(deleted).To(BeTrue())
	})
}

func TestRepeatedTypedValidation(t *testing.T) {
	t.Parallel()
	for _, operation := range []admissionv1.Operation{admissionv1.Create, admissionv1.Update, admissionv1.Delete} {
		t.Run(string(operation), func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			mgr := newManager(t)
			builder := webhook.Validating[*corev1.ConfigMap](mgr, "/multi")
			var called []int
			for i := 1; i <= 3; i++ {
				validate := func() (admission.Warnings, error) {
					called = append(called, i)
					if i == 2 {
						return admission.Warnings{"second"}, errors.New("stop")
					}
					return admission.Warnings{"first"}, nil
				}
				switch operation {
				case admissionv1.Create:
					builder.OnCreate(func(context.Context, *corev1.ConfigMap, client.Reader) (admission.Warnings, error) { return validate() })
				case admissionv1.Update:
					builder.OnUpdate(func(context.Context, *corev1.ConfigMap, *corev1.ConfigMap, client.Reader) (admission.Warnings, error) {
						return validate()
					})
				case admissionv1.Delete:
					builder.OnDelete(func(context.Context, *corev1.ConfigMap, client.Reader) (admission.Warnings, error) { return validate() })
				default:
					t.Fatalf("unsupported operation %q", operation)
				}
			}
			g.Expect(builder.Build()).To(Succeed())
			resp := registeredHook(t, mgr, "/multi").Handle(t.Context(), request(t, operation))
			g.Expect(resp.Allowed).To(BeFalse())
			g.Expect(resp.Result.Message).To(ContainSubstring("stop"))
			g.Expect(resp.Warnings).To(Equal([]string{"first", "second"}))
			g.Expect(called).To(Equal([]int{1, 2}))
		})
	}
}

func TestRepeatedDefaulting(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	mgr := newManager(t)
	g.Expect(webhook.Defaulting[*corev1.ConfigMap](mgr, "/multi").
		WithDefault(func(_ context.Context, obj *corev1.ConfigMap, _ client.Reader) error {
			obj.Data = map[string]string{"first": "yes"}
			return nil
		}).
		WithDefault(func(_ context.Context, obj *corev1.ConfigMap, _ client.Reader) error {
			g.Expect(obj.Data).To(HaveKeyWithValue("first", "yes"))
			obj.Data["second"] = "yes"
			return nil
		}).Build()).To(Succeed())
	resp := registeredHook(t, mgr, "/multi").Handle(t.Context(), request(t, admissionv1.Create))
	g.Expect(resp.Allowed).To(BeTrue())
	g.Expect(string(resp.Patch)).To(ContainSubstring("first"))
	g.Expect(string(resp.Patch)).To(ContainSubstring("second"))

	mgr = newManager(t)
	called := false
	g.Expect(webhook.Defaulting[*corev1.ConfigMap](mgr, "/stop").
		WithDefault(func(context.Context, *corev1.ConfigMap, client.Reader) error { return errors.New("stop") }).
		WithDefault(func(context.Context, *corev1.ConfigMap, client.Reader) error { called = true; return nil }).Build()).To(Succeed())
	resp = registeredHook(t, mgr, "/stop").Handle(t.Context(), request(t, admissionv1.Create))
	g.Expect(resp.Allowed).To(BeFalse())
	g.Expect(called).To(BeFalse())
}

func TestTypedHandlerErrors(t *testing.T) {
	t.Parallel()

	t.Run("validator preserves warning and error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		mgr := newManager(t)
		g.Expect(webhook.Validating[*corev1.ConfigMap](mgr, "/validate-error").
			OnCreate(func(context.Context, *corev1.ConfigMap, client.Reader) (admission.Warnings, error) {
				return admission.Warnings{"check this setting"}, errors.New("invalid configmap")
			}).Build()).To(Succeed())
		resp := registeredHook(t, mgr, "/validate-error").Handle(t.Context(), request(t, admissionv1.Create))
		g.Expect(resp.Allowed).To(BeFalse())
		g.Expect(resp.Result.Message).To(ContainSubstring("invalid configmap"))
		g.Expect(resp.Warnings).To(Equal([]string{"check this setting"}))
	})

	t.Run("defaulter returns error without patch", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		mgr := newManager(t)
		g.Expect(webhook.Defaulting[*corev1.ConfigMap](mgr, "/default-error").
			WithDefault(func(context.Context, *corev1.ConfigMap, client.Reader) error {
				return errors.New("cannot default configmap")
			}).Build()).To(Succeed())
		resp := registeredHook(t, mgr, "/default-error").Handle(t.Context(), request(t, admissionv1.Create))
		g.Expect(resp.Allowed).To(BeFalse())
		g.Expect(resp.Result.Message).To(ContainSubstring("cannot default configmap"))
		g.Expect(resp.Patches).To(BeEmpty())
	})
}

func TestMalformedTypedRequests(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		operation admissionv1.Operation
		oldObject bool
	}{
		{name: "create object", operation: admissionv1.Create},
		{name: "update object", operation: admissionv1.Update},
		{name: "update old object", operation: admissionv1.Update, oldObject: true},
		{name: "delete old object", operation: admissionv1.Delete, oldObject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			mgr := newManager(t)
			called := false
			g.Expect(webhook.Validating[*corev1.ConfigMap](mgr, "/validate-malformed").
				OnCreate(func(context.Context, *corev1.ConfigMap, client.Reader) (admission.Warnings, error) {
					called = true
					return nil, nil
				}).OnUpdate(func(context.Context, *corev1.ConfigMap, *corev1.ConfigMap, client.Reader) (admission.Warnings, error) {
				called = true
				return nil, nil
			}).OnDelete(func(context.Context, *corev1.ConfigMap, client.Reader) (admission.Warnings, error) {
				called = true
				return nil, nil
			}).Build()).To(Succeed())
			req := request(t, tc.operation)
			if tc.oldObject {
				req.OldObject.Raw = []byte("{")
			} else {
				req.Object.Raw = []byte("{")
			}
			resp := registeredHook(t, mgr, "/validate-malformed").Handle(t.Context(), req)
			g.Expect(resp.Allowed).To(BeFalse())
			g.Expect(resp.Result.Code).To(Equal(int32(http.StatusBadRequest)))
			g.Expect(called).To(BeFalse())
		})
	}

	t.Run("defaulter object", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		mgr := newManager(t)
		called := false
		g.Expect(webhook.Defaulting[*corev1.ConfigMap](mgr, "/default-malformed").
			WithDefault(func(context.Context, *corev1.ConfigMap, client.Reader) error {
				called = true
				return nil
			}).Build()).To(Succeed())
		req := request(t, admissionv1.Create)
		req.Object.Raw = []byte("{")
		resp := registeredHook(t, mgr, "/default-malformed").Handle(t.Context(), req)
		g.Expect(resp.Allowed).To(BeFalse())
		g.Expect(resp.Result.Code).To(Equal(int32(http.StatusBadRequest)))
		g.Expect(called).To(BeFalse())
	})
}

func TestWithAdmissionOverridesLogConstructor(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	mgr := newManager(t)
	called := false
	g.Expect(webhook.For(mgr, "/custom-log", corev1.SchemeGroupVersion.WithKind("ConfigMap")).OnCreate(func(context.Context, admission.Request, client.Reader) admission.Response {
		return admission.Allowed("")
	}).WithAdmission(func(hook *admission.Webhook) {
		hook.LogConstructor = func(base logr.Logger, _ *admission.Request) logr.Logger {
			called = true
			return base
		}
	}).Build()).To(Succeed())
	g.Expect(registeredHook(t, mgr, "/custom-log").Handle(t.Context(), request(t, admissionv1.Create)).Allowed).To(BeTrue())
	g.Expect(called).To(BeTrue())
}

func TestPrebuiltWebhookCustomization(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	mgr := newManager(t)
	customLogCalled := false
	prebuilt := &admission.Webhook{
		Handler: admission.HandlerFunc(func(context.Context, admission.Request) admission.Response {
			return admission.Allowed("")
		}),
		LogConstructor: func(base logr.Logger, _ *admission.Request) logr.Logger {
			customLogCalled = true
			return base
		},
	}
	err := webhook.For(mgr, "/custom", corev1.SchemeGroupVersion.WithKind("ConfigMap")).WithWebhook(prebuilt).WithAdmission(func(hook *admission.Webhook) {
		hook.WithRecoverPanic(false)
	}).Build()
	g.Expect(err).To(Succeed())
	hook := registeredHook(t, mgr, "/custom")
	g.Expect(hook).NotTo(BeIdenticalTo(prebuilt))
	hook.LogConstructor(logr.Discard(), nil)
	g.Expect(customLogCalled).To(BeTrue())
	g.Expect(hook.RecoverPanic).NotTo(BeNil())
	g.Expect(*hook.RecoverPanic).To(BeFalse())
	g.Expect(prebuilt.RecoverPanic).To(BeNil())
}

func TestPrebuiltWebhookCanBeReused(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	mgr := newManager(t)
	prebuilt := &admission.Webhook{Handler: admission.HandlerFunc(func(context.Context, admission.Request) admission.Response {
		return admission.Allowed("")
	})}
	prebuilt.WithRecoverPanic(true)
	g.Expect(webhook.For(mgr, "/configmap", corev1.SchemeGroupVersion.WithKind("ConfigMap")).WithWebhook(prebuilt).Build()).To(Succeed())
	first := registeredHook(t, mgr, "/configmap")
	g.Expect(first.Handle(t.Context(), request(t, admissionv1.Create)).Allowed).To(BeTrue())
	g.Expect(webhook.For(mgr, "/secret", corev1.SchemeGroupVersion.WithKind("Secret")).WithWebhook(prebuilt).Build()).To(Succeed())
	second := registeredHook(t, mgr, "/secret")
	secretReq := request(t, admissionv1.Create)
	secretReq.Kind.Kind = "Secret"
	g.Expect(first.Handle(t.Context(), request(t, admissionv1.Create)).Allowed).To(BeTrue())
	g.Expect(second.Handle(t.Context(), secretReq).Allowed).To(BeTrue())
	g.Expect(first.Handle(t.Context(), secretReq).Allowed).To(BeFalse())
	g.Expect(prebuilt.LogConstructor).To(BeNil())
	g.Expect(first.RecoverPanic).NotTo(BeIdenticalTo(prebuilt.RecoverPanic))
	*prebuilt.RecoverPanic = false
	g.Expect(*first.RecoverPanic).To(BeTrue())
	g.Expect(*second.RecoverPanic).To(BeTrue())
}

func TestBuildRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	handler := func(context.Context, admission.Request, client.Reader, admission.Decoder) admission.Response {
		return admission.Allowed("")
	}
	cases := []struct {
		name  string
		build func(*testManager) error
	}{
		{"manager", func(*testManager) error {
			return webhook.For(nil, "/missing-manager", corev1.SchemeGroupVersion.WithKind("ConfigMap")).WithHandler(handler).Build()
		}},
		{"path", func(m *testManager) error {
			return webhook.For(m, "relative", corev1.SchemeGroupVersion.WithKind("ConfigMap")).WithHandler(handler).Build()
		}},
		{"handler", func(m *testManager) error {
			return webhook.For(m, "/missing", corev1.SchemeGroupVersion.WithKind("ConfigMap")).Build()
		}},
		{"nil handler", func(m *testManager) error {
			return webhook.For(m, "/nil", corev1.SchemeGroupVersion.WithKind("ConfigMap")).WithHandler(nil).Build()
		}},
		{"nil create handler", func(m *testManager) error {
			return webhook.For(m, "/nil-create", corev1.SchemeGroupVersion.WithKind("ConfigMap")).OnCreate(nil).Build()
		}},
		{"multiple handlers", func(m *testManager) error {
			return webhook.For(m, "/multiple", corev1.SchemeGroupVersion.WithKind("ConfigMap")).WithHandler(handler).WithWebhook(&admission.Webhook{}).Build()
		}},
		{"empty validator", func(m *testManager) error {
			return webhook.Validating[*corev1.ConfigMap](m, "/validate").Build()
		}},
		{"nil typed create handler", func(m *testManager) error {
			return webhook.Validating[*corev1.ConfigMap](m, "/validate").OnCreate(nil).
				OnUpdate(func(context.Context, *corev1.ConfigMap, *corev1.ConfigMap, client.Reader) (admission.Warnings, error) {
					return nil, nil
				}).Build()
		}},
		{"nil typed update handler", func(m *testManager) error {
			return webhook.Validating[*corev1.ConfigMap](m, "/validate").OnUpdate(nil).
				OnCreate(func(context.Context, *corev1.ConfigMap, client.Reader) (admission.Warnings, error) {
					return nil, nil
				}).Build()
		}},
		{"nil typed delete handler", func(m *testManager) error {
			return webhook.Validating[*corev1.ConfigMap](m, "/validate").OnDelete(nil).
				OnCreate(func(context.Context, *corev1.ConfigMap, client.Reader) (admission.Warnings, error) {
					return nil, nil
				}).Build()
		}},
		{"empty defaulter", func(m *testManager) error {
			return webhook.Defaulting[*corev1.ConfigMap](m, "/mutate").Build()
		}},
		{"nil defaulter", func(m *testManager) error {
			return webhook.Defaulting[*corev1.ConfigMap](m, "/mutate").WithDefault(nil).Build()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			mgr := newManager(t)
			g.Expect(tc.build(mgr)).To(HaveOccurred())
			g.Expect(mgr.server.hook).To(BeNil())
		})
	}
}

func TestRequestKindGuard(t *testing.T) {
	t.Parallel()
	gvk := corev1.SchemeGroupVersion.WithKind("ConfigMap")
	for _, style := range []string{"raw", "operation", "prebuilt", "customized", "validating", "defaulting"} {
		t.Run(style, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			mgr := newManager(t)
			called := false
			handler := admission.HandlerFunc(func(context.Context, admission.Request) admission.Response {
				called = true
				return admission.Allowed("")
			})
			builder := webhook.For(mgr, "/guard", gvk)
			var err error
			switch style {
			case "raw":
				err = builder.WithHandler(func(ctx context.Context, req admission.Request, _ client.Reader, _ admission.Decoder) admission.Response {
					return handler.Handle(ctx, req)
				}).Build()
			case "operation":
				err = builder.OnCreate(func(ctx context.Context, req admission.Request, _ client.Reader) admission.Response {
					return handler.Handle(ctx, req)
				}).Build()
			case "prebuilt":
				err = builder.WithWebhook(&admission.Webhook{Handler: handler}).Build()
			case "customized":
				err = builder.WithWebhook(&admission.Webhook{Handler: admission.HandlerFunc(func(context.Context, admission.Request) admission.Response {
					t.Fatal("original handler should have been replaced")
					return admission.Denied("")
				})}).WithAdmission(func(hook *admission.Webhook) { hook.Handler = handler }).Build()
			case "validating":
				err = webhook.Validating[*corev1.ConfigMap](mgr, "/guard").OnCreate(func(context.Context, *corev1.ConfigMap, client.Reader) (admission.Warnings, error) {
					called = true
					return nil, nil
				}).Build()
			case "defaulting":
				err = webhook.Defaulting[*corev1.ConfigMap](mgr, "/guard").WithDefault(func(context.Context, *corev1.ConfigMap, client.Reader) error {
					called = true
					return nil
				}).Build()
			}
			g.Expect(err).To(Succeed())
			hook := registeredHook(t, mgr, "/guard")
			req := request(t, admissionv1.Create)
			req.RequestKind = &metav1.GroupVersionKind{Group: "original.example.com", Version: "v2", Kind: "Original"}
			g.Expect(hook.Handle(t.Context(), req).Allowed).To(BeTrue())
			g.Expect(called).To(BeTrue())
			called = false
			for _, kind := range []metav1.GroupVersionKind{
				{Group: "wrong.example.com", Version: gvk.Version, Kind: gvk.Kind},
				{Version: "v2", Kind: gvk.Kind},
				{Version: gvk.Version, Kind: "Secret"},
				{},
			} {
				for _, operation := range []admissionv1.Operation{admissionv1.Create, admissionv1.Update, admissionv1.Delete, admissionv1.Connect} {
					req := request(t, operation)
					req.Kind = kind
					// The GVK guard must reject before the typed decoder sees malformed content.
					req.Object.Raw, req.OldObject.Raw = []byte("{"), []byte("{")
					resp := hook.Handle(t.Context(), req)
					g.Expect(resp.Allowed).To(BeFalse())
					g.Expect(resp.Result.Code).To(Equal(int32(http.StatusBadRequest)))
					g.Expect(resp.Result.Message).To(ContainSubstring("expected GVK " + gvk.String()))
					g.Expect(resp.Result.Message).To(ContainSubstring("got " + schema.GroupVersionKind(kind).String()))
					g.Expect(resp.Patches).To(BeEmpty())
					g.Expect(called).To(BeFalse())
				}
			}
			for _, payload := range []string{
				`{"apiVersion":"wrong.example.com/v1","kind":"ConfigMap"}`,
				`{"apiVersion":"v2","kind":"ConfigMap"}`,
				`{"apiVersion":"v1","kind":"Secret"}`,
			} {
				for _, tc := range []struct {
					operation admissionv1.Operation
					oldObject bool
				}{
					{operation: admissionv1.Create},
					{operation: admissionv1.Update},
					{operation: admissionv1.Update, oldObject: true},
					{operation: admissionv1.Delete, oldObject: true},
				} {
					req := request(t, tc.operation)
					if tc.oldObject {
						req.OldObject.Raw = []byte(payload)
					} else {
						req.Object.Raw = []byte(payload)
					}
					resp := hook.Handle(t.Context(), req)
					g.Expect(resp.Allowed).To(BeFalse())
					g.Expect(resp.Result.Code).To(Equal(int32(http.StatusBadRequest)))
					g.Expect(resp.Result.Message).To(ContainSubstring("expected object GVK " + gvk.String()))
					g.Expect(resp.Patches).To(BeEmpty())
					g.Expect(called).To(BeFalse())
				}
			}
			deleteReq := request(t, admissionv1.Delete)
			deleteReq.Object.Raw = []byte(`{"apiVersion":"v1","kind":"DeleteOptions"}`)
			g.Expect(hook.Handle(t.Context(), deleteReq).Allowed).To(BeTrue())
			if style != "defaulting" {
				connectReq := request(t, admissionv1.Connect)
				connectReq.Object.Raw = []byte(`{"apiVersion":"v1","kind":"PodExecOptions"}`)
				g.Expect(hook.Handle(t.Context(), connectReq).Allowed).To(BeTrue())
			}
		})
	}
}

func TestBuildRejectsInvalidGVK(t *testing.T) {
	t.Parallel()
	for _, gvk := range []schema.GroupVersionKind{{}, {Version: "v1"}, {Kind: "ConfigMap"}} {
		t.Run(gvk.String(), func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			mgr := newManager(t)
			err := webhook.For(mgr, "/invalid-gvk", gvk).OnCreate(func(context.Context, admission.Request, client.Reader) admission.Response {
				return admission.Allowed("")
			}).Build()
			g.Expect(err).To(MatchError(ContainSubstring("requires a GVK with version and kind")))
			g.Expect(mgr.server.hook).To(BeNil())
		})
	}
}

func TestTypedGVKRegistration(t *testing.T) {
	t.Parallel()
	for _, registration := range []string{"missing", "multiple"} {
		for _, style := range []string{"validating", "defaulting"} {
			t.Run(registration+"/"+style, func(t *testing.T) {
				t.Parallel()
				g := NewWithT(t)
				mgr := newManager(t)
				if registration == "missing" {
					mgr.scheme = runtime.NewScheme()
				} else {
					mgr.scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "example.com", Version: "v2", Kind: "ConfigMap"}, &corev1.ConfigMap{})
				}
				var err error
				if style == "validating" {
					err = webhook.Validating[*corev1.ConfigMap](mgr, "/resolution").OnCreate(func(context.Context, *corev1.ConfigMap, client.Reader) (admission.Warnings, error) {
						return nil, nil
					}).Build()
				} else {
					err = webhook.Defaulting[*corev1.ConfigMap](mgr, "/resolution").WithDefault(func(context.Context, *corev1.ConfigMap, client.Reader) error { return nil }).Build()
				}
				g.Expect(err).To(HaveOccurred())
				if registration == "multiple" {
					g.Expect(err).To(MatchError(ContainSubstring("multiple GroupVersionKinds")))
				}
				g.Expect(mgr.server.hook).To(BeNil())
			})
		}
	}
}
