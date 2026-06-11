package imagestreams_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/onsi/gomega/gstruct"
	imagev1 "github.com/openshift/api/image/v1"
	"github.com/rs/xid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/imagestreams"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"github.com/opendatahub-io/odh-platform-utilities/framework/utils/test/matchers"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"

	. "github.com/onsi/gomega"
)

type fakeInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	status        fwapi.Status
	releaseStatus common.ComponentReleaseStatus
}

func (f *fakeInstance) GetStatus() *fwapi.Status {
	return &f.status
}

func (f *fakeInstance) GetConditions() []fwapi.Condition {
	return f.status.Conditions
}

func (f *fakeInstance) SetConditions(c []fwapi.Condition) {
	f.status.Conditions = c
}

func (f *fakeInstance) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &f.releaseStatus
}

func (f *fakeInstance) SetReleaseStatus(status common.ComponentReleaseStatus) {
	f.releaseStatus = status
}

func (f *fakeInstance) DeepCopyObject() runtime.Object {
	o := *f
	return &o
}

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = imagev1.Install(s)
	_ = corev1.AddToScheme(s)
	return s
}

func newFakeClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(newScheme()).
		WithObjects(objs...).
		Build()
}

func newFakeClientWithInterceptor(funcs interceptor.Funcs, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(newScheme()).
		WithObjects(objs...).
		WithInterceptorFuncs(funcs).
		Build()
}

func newImageStream(name string, ns string, partOf string, tagStatuses []imagev1.NamedTagEventList) *imagev1.ImageStream {
	return &imagev1.ImageStream{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				labels.PlatformPartOf: partOf,
			},
		},
		Status: imagev1.ImageStreamStatus{
			Tags: tagStatuses,
		},
	}
}

func newInstance() *fakeInstance {
	return &fakeInstance{
		TypeMeta:   metav1.TypeMeta{APIVersion: "test/v1", Kind: "FakeComponent"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-instance"},
	}
}

func TestImageStreamsNoImageStreams(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	ns := xid.New().String()

	cl := newFakeClient()
	instance := newInstance()

	action := imagestreams.NewAction(
		imagestreams.InNamespace(ns),
		imagestreams.WithSelectorLabel(labels.PlatformPartOf, "fakecomponent"),
	)

	rr := types.ReconciliationRequest{
		Client:   cl,
		Instance: instance,
	}
	rr.Conditions = conditions.NewManager(rr.Instance, string(fwapi.ConditionTypeReady))

	err := action(ctx, &rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Instance).Should(
		WithTransform(
			matchers.ExtractStatusCondition(imagestreams.DefaultConditionType),
			gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Status": Equal(metav1.ConditionTrue),
			}),
		),
	)
}

func TestImageStreamsNoMatchErrorVanillaK8s(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	ns := xid.New().String()

	interceptorFuncs := interceptor.Funcs{
		List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if _, ok := list.(*imagev1.ImageStreamList); ok {
				return &meta.NoKindMatchError{
					GroupKind: schema.GroupKind{
						Group: "image.openshift.io",
						Kind:  "ImageStream",
					},
					SearchedVersions: []string{"v1"},
				}
			}
			return cl.List(ctx, list, opts...)
		},
	}

	cl := newFakeClientWithInterceptor(interceptorFuncs)
	instance := newInstance()

	action := imagestreams.NewAction(
		imagestreams.InNamespace(ns),
		imagestreams.WithSelectorLabel(labels.PlatformPartOf, "fakecomponent"),
	)

	rr := types.ReconciliationRequest{
		Client:   cl,
		Instance: instance,
	}
	rr.Conditions = conditions.NewManager(rr.Instance, string(fwapi.ConditionTypeReady))

	err := action(ctx, &rr)
	g.Expect(err).ShouldNot(HaveOccurred())

	cond := conditions.FindStatusCondition(rr.Instance.GetStatus(), imagestreams.DefaultConditionType)
	g.Expect(cond).Should(BeNil(), "no condition should be set when ImageStream CRD is missing")
}

func TestImageStreamsAllHealthy(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	ns := xid.New().String()

	is := newImageStream("jupyter-datascience", ns, "fakecomponent", []imagev1.NamedTagEventList{
		{
			Tag:   "latest",
			Items: []imagev1.TagEvent{{Image: "sha256:abc123"}},
		},
	})

	cl := newFakeClient(is)
	instance := newInstance()

	action := imagestreams.NewAction(
		imagestreams.InNamespace(ns),
		imagestreams.WithSelectorLabel(labels.PlatformPartOf, "fakecomponent"),
	)

	rr := types.ReconciliationRequest{
		Client:   cl,
		Instance: instance,
	}
	rr.Conditions = conditions.NewManager(rr.Instance, string(fwapi.ConditionTypeReady))

	err := action(ctx, &rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Instance).Should(
		WithTransform(
			matchers.ExtractStatusCondition(imagestreams.DefaultConditionType),
			gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Status": Equal(metav1.ConditionTrue),
			}),
		),
	)
}

func TestImageStreamsAllFailed(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	ns := xid.New().String()

	is := newImageStream("jupyter-cuda", ns, "fakecomponent", []imagev1.NamedTagEventList{
		{
			Tag:   "cuda-12",
			Items: []imagev1.TagEvent{},
			Conditions: []imagev1.TagEventCondition{{
				Type:    imagev1.ImportSuccess,
				Status:  corev1.ConditionFalse,
				Message: "image not found",
			}},
		},
	})

	cl := newFakeClient(is)
	instance := newInstance()

	action := imagestreams.NewAction(
		imagestreams.InNamespace(ns),
		imagestreams.WithSelectorLabel(labels.PlatformPartOf, "fakecomponent"),
	)

	rr := types.ReconciliationRequest{
		Client:   cl,
		Instance: instance,
	}
	rr.Conditions = conditions.NewManager(rr.Instance, string(fwapi.ConditionTypeReady))

	err := action(ctx, &rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Instance).Should(
		WithTransform(
			matchers.ExtractStatusCondition(imagestreams.DefaultConditionType),
			gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Status":  Equal(metav1.ConditionFalse),
				"Reason":  Equal(imagestreams.DefaultNotAvailableReason),
				"Message": ContainSubstring("Warning:"),
			}),
		),
	)
}

func TestImageStreamsMixedHealth(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	ns := xid.New().String()

	is := newImageStream("jupyter-mixed", ns, "fakecomponent", []imagev1.NamedTagEventList{
		{
			Tag:   "cpu",
			Items: []imagev1.TagEvent{{Image: "sha256:abc"}},
		},
		{
			Tag:   "cuda",
			Items: []imagev1.TagEvent{},
			Conditions: []imagev1.TagEventCondition{{
				Type:    imagev1.ImportSuccess,
				Status:  corev1.ConditionFalse,
				Message: "not found",
			}},
		},
	})

	cl := newFakeClient(is)
	instance := newInstance()

	action := imagestreams.NewAction(
		imagestreams.InNamespace(ns),
		imagestreams.WithSelectorLabel(labels.PlatformPartOf, "fakecomponent"),
	)

	rr := types.ReconciliationRequest{
		Client:   cl,
		Instance: instance,
	}
	rr.Conditions = conditions.NewManager(rr.Instance, string(fwapi.ConditionTypeReady))

	err := action(ctx, &rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Instance).Should(
		WithTransform(
			matchers.ExtractStatusCondition(imagestreams.DefaultConditionType),
			gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Status":  Equal(metav1.ConditionFalse),
				"Message": And(ContainSubstring("1 ImageStream tag(s)"), ContainSubstring("jupyter-mixed:cuda")),
			}),
		),
	)
}

func TestImageStreamsFreshDeploy(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	ns := xid.New().String()

	is := newImageStream("jupyter-new", ns, "fakecomponent", []imagev1.NamedTagEventList{
		{
			Tag:   "latest",
			Items: []imagev1.TagEvent{},
		},
	})

	cl := newFakeClient(is)
	instance := newInstance()

	action := imagestreams.NewAction(
		imagestreams.InNamespace(ns),
		imagestreams.WithSelectorLabel(labels.PlatformPartOf, "fakecomponent"),
	)

	rr := types.ReconciliationRequest{
		Client:   cl,
		Instance: instance,
	}
	rr.Conditions = conditions.NewManager(rr.Instance, string(fwapi.ConditionTypeReady))

	err := action(ctx, &rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Instance).Should(
		WithTransform(
			matchers.ExtractStatusCondition(imagestreams.DefaultConditionType),
			gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Status": Equal(metav1.ConditionTrue),
			}),
		),
	)
}

func TestImageStreamsImportSuccessTrue(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	ns := xid.New().String()

	is := newImageStream("jupyter-importing", ns, "fakecomponent", []imagev1.NamedTagEventList{
		{
			Tag:   "latest",
			Items: []imagev1.TagEvent{},
			Conditions: []imagev1.TagEventCondition{{
				Type:   imagev1.ImportSuccess,
				Status: corev1.ConditionTrue,
			}},
		},
	})

	cl := newFakeClient(is)
	instance := newInstance()

	action := imagestreams.NewAction(
		imagestreams.InNamespace(ns),
		imagestreams.WithSelectorLabel(labels.PlatformPartOf, "fakecomponent"),
	)

	rr := types.ReconciliationRequest{
		Client:   cl,
		Instance: instance,
	}
	rr.Conditions = conditions.NewManager(rr.Instance, string(fwapi.ConditionTypeReady))

	err := action(ctx, &rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Instance).Should(
		WithTransform(
			matchers.ExtractStatusCondition(imagestreams.DefaultConditionType),
			gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Status": Equal(metav1.ConditionTrue),
			}),
		),
	)
}

func TestImageStreamsMultipleImageStreams(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	ns := xid.New().String()

	partOf := "fakecomponent"
	is1 := newImageStream("jupyter-cpu", ns, partOf, []imagev1.NamedTagEventList{
		{Tag: "latest", Items: []imagev1.TagEvent{{Image: "sha256:ok"}}},
	})
	is2 := newImageStream("jupyter-cuda", ns, partOf, []imagev1.NamedTagEventList{
		{
			Tag: "cuda-12", Items: []imagev1.TagEvent{},
			Conditions: []imagev1.TagEventCondition{{
				Type: imagev1.ImportSuccess, Status: corev1.ConditionFalse, Message: "not found",
			}},
		},
	})

	cl := newFakeClient(is1, is2)
	instance := newInstance()

	action := imagestreams.NewAction(
		imagestreams.InNamespace(ns),
		imagestreams.WithSelectorLabel(labels.PlatformPartOf, partOf),
	)

	rr := types.ReconciliationRequest{
		Client:   cl,
		Instance: instance,
	}
	rr.Conditions = conditions.NewManager(rr.Instance, string(fwapi.ConditionTypeReady))

	err := action(ctx, &rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Instance).Should(
		WithTransform(
			matchers.ExtractStatusCondition(imagestreams.DefaultConditionType),
			gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Status":  Equal(metav1.ConditionFalse),
				"Message": ContainSubstring("jupyter-cuda:cuda-12"),
			}),
		),
	)
}

func TestImageStreamsIgnoresDifferentLabels(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	ns := xid.New().String()

	is := newImageStream("other-component", ns, "dashboard", []imagev1.NamedTagEventList{
		{
			Tag: "bad", Items: []imagev1.TagEvent{},
			Conditions: []imagev1.TagEventCondition{{
				Type: imagev1.ImportSuccess, Status: corev1.ConditionFalse, Message: "fail",
			}},
		},
	})

	cl := newFakeClient(is)
	instance := newInstance()

	action := imagestreams.NewAction(
		imagestreams.InNamespace(ns),
		imagestreams.WithSelectorLabel(labels.PlatformPartOf, "fakecomponent"),
	)

	rr := types.ReconciliationRequest{
		Client:   cl,
		Instance: instance,
	}
	rr.Conditions = conditions.NewManager(rr.Instance, string(fwapi.ConditionTypeReady))

	err := action(ctx, &rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Instance).Should(
		WithTransform(
			matchers.ExtractStatusCondition(imagestreams.DefaultConditionType),
			gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Status": Equal(metav1.ConditionTrue),
			}),
		),
	)
}

func TestImageStreamsInNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	ns := xid.New().String()
	otherNs := xid.New().String()

	partOf := "fakecomponent"
	isTarget := newImageStream("jupyter-target", ns, partOf, []imagev1.NamedTagEventList{
		{
			Tag: "bad", Items: []imagev1.TagEvent{},
			Conditions: []imagev1.TagEventCondition{{
				Type: imagev1.ImportSuccess, Status: corev1.ConditionFalse, Message: "fail",
			}},
		},
	})
	isOther := newImageStream("jupyter-other", otherNs, partOf, []imagev1.NamedTagEventList{
		{
			Tag: "bad", Items: []imagev1.TagEvent{},
			Conditions: []imagev1.TagEventCondition{{
				Type: imagev1.ImportSuccess, Status: corev1.ConditionFalse, Message: "fail",
			}},
		},
	})

	cl := newFakeClient(isTarget, isOther)
	instance := newInstance()

	action := imagestreams.NewAction(
		imagestreams.InNamespace(ns),
		imagestreams.WithSelectorLabel(labels.PlatformPartOf, partOf),
	)

	rr := types.ReconciliationRequest{
		Client:   cl,
		Instance: instance,
	}
	rr.Conditions = conditions.NewManager(rr.Instance, string(fwapi.ConditionTypeReady))

	err := action(ctx, &rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Instance).Should(
		WithTransform(
			matchers.ExtractStatusCondition(imagestreams.DefaultConditionType),
			gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Status":  Equal(metav1.ConditionFalse),
				"Message": And(ContainSubstring("jupyter-target:bad"), Not(ContainSubstring("jupyter-other"))),
			}),
		),
	)
}

func TestImageStreamsMessageTruncation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	ns := xid.New().String()

	longMsg := strings.Repeat("x", 200)
	is := newImageStream("jupyter-long", ns, "fakecomponent", []imagev1.NamedTagEventList{
		{
			Tag: "tag1", Items: []imagev1.TagEvent{},
			Conditions: []imagev1.TagEventCondition{{
				Type: imagev1.ImportSuccess, Status: corev1.ConditionFalse, Message: longMsg,
			}},
		},
	})

	cl := newFakeClient(is)
	instance := newInstance()

	action := imagestreams.NewAction(
		imagestreams.InNamespace(ns),
		imagestreams.WithSelectorLabel(labels.PlatformPartOf, "fakecomponent"),
	)

	rr := types.ReconciliationRequest{
		Client:   cl,
		Instance: instance,
	}
	rr.Conditions = conditions.NewManager(rr.Instance, string(fwapi.ConditionTypeReady))

	err := action(ctx, &rr)
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(rr.Instance).Should(
		WithTransform(
			matchers.ExtractStatusCondition(imagestreams.DefaultConditionType),
			gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Status":  Equal(metav1.ConditionFalse),
				"Message": And(ContainSubstring("..."), Not(ContainSubstring(longMsg))),
			}),
		),
	)
}

func TestImageStreamsMaxFailedTagsCap(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	ns := xid.New().String()

	tags := make([]imagev1.NamedTagEventList, 0, 15)
	for i := range 15 {
		tags = append(tags, imagev1.NamedTagEventList{
			Tag: fmt.Sprintf("tag-%d", i), Items: []imagev1.TagEvent{},
			Conditions: []imagev1.TagEventCondition{{
				Type: imagev1.ImportSuccess, Status: corev1.ConditionFalse, Message: "not found",
			}},
		})
	}

	is := newImageStream("jupyter-many-tags", ns, "fakecomponent", tags)

	cl := newFakeClient(is)
	instance := newInstance()

	action := imagestreams.NewAction(
		imagestreams.InNamespace(ns),
		imagestreams.WithSelectorLabel(labels.PlatformPartOf, "fakecomponent"),
	)

	rr := types.ReconciliationRequest{
		Client:   cl,
		Instance: instance,
	}
	rr.Conditions = conditions.NewManager(rr.Instance, string(fwapi.ConditionTypeReady))

	err := action(ctx, &rr)
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(rr.Instance).Should(
		WithTransform(
			matchers.ExtractStatusCondition(imagestreams.DefaultConditionType),
			gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Status": Equal(metav1.ConditionFalse),
				"Message": And(
					ContainSubstring("Warning: 15 ImageStream tag(s) failed to import"),
					ContainSubstring("... and 5 more"),
					Not(ContainSubstring("tag-14")),
				),
			}),
		),
	)
}

func TestImageStreamsMissingNamespaceFn(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	cl := newFakeClient()
	instance := newInstance()

	action := imagestreams.NewAction(
		imagestreams.WithSelectorLabel(labels.PlatformPartOf, "fakecomponent"),
	)

	rr := types.ReconciliationRequest{
		Client:   cl,
		Instance: instance,
	}
	rr.Conditions = conditions.NewManager(rr.Instance, string(fwapi.ConditionTypeReady))

	err := action(ctx, &rr)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("namespace function is not configured"))
}
