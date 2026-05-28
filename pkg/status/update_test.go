package status_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/status"
)

var (
	errConflictCause = errors.New("modified by another actor")
	errGetFailed     = errors.New("simulated get failure")
)

func newConflictError(name string) error {
	return apierrors.NewConflict(
		schema.GroupResource{Resource: "pods"},
		name,
		errConflictCause,
	)
}

func TestUpdate_Success(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().
		WithObjects(pod).
		WithStatusSubresource(pod).
		Build()

	err := status.Update(context.Background(), c, pod, func(p *corev1.Pod) {
		p.Status.Phase = corev1.PodRunning
	})

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning))
}

func TestUpdate_ConflictThenSuccess(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	callCount := 0
	c := fake.NewClientBuilder().
		WithObjects(pod).
		WithStatusSubresource(pod).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(
				ctx context.Context,
				cl client.Client,
				subResourceName string,
				obj client.Object,
				opts ...client.SubResourceUpdateOption,
			) error {
				callCount++
				if callCount <= 2 {
					return newConflictError(obj.GetName())
				}

				return cl.SubResource(subResourceName).Update(ctx, obj, opts...)
			},
		}).
		Build()

	err := status.Update(context.Background(), c, pod, func(p *corev1.Pod) {
		p.Status.Phase = corev1.PodRunning
	})

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning))
	g.Expect(callCount).To(Equal(3))
}

func TestUpdate_ExhaustedRetries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxRetries int
	}{
		{"multiple retries", 3},
		{"zero retries", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
			}

			c := fake.NewClientBuilder().
				WithObjects(pod).
				WithStatusSubresource(pod).
				WithInterceptorFuncs(interceptor.Funcs{
					SubResourceUpdate: func(
						_ context.Context,
						_ client.Client,
						_ string,
						obj client.Object,
						_ ...client.SubResourceUpdateOption,
					) error {
						return newConflictError(obj.GetName())
					},
				}).
				Build()

			err := status.Update(context.Background(), c, pod, func(p *corev1.Pod) {
				p.Status.Phase = corev1.PodRunning
			}, status.WithMaxRetries(tt.maxRetries))

			g.Expect(err).Should(HaveOccurred())
			g.Expect(errors.Is(err, status.ErrRetriesExhausted)).To(BeTrue())
			g.Expect(apierrors.IsConflict(err)).To(BeTrue())
		})
	}
}

func TestUpdate_NilMutateFn(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().
		WithObjects(pod).
		WithStatusSubresource(pod).
		Build()

	err := status.Update[*corev1.Pod](context.Background(), c, pod, nil)

	g.Expect(err).Should(HaveOccurred())
	g.Expect(errors.Is(err, status.ErrNilMutateFn)).To(BeTrue())
}

func TestUpdate_GetFailureDuringRetry(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().
		WithObjects(pod).
		WithStatusSubresource(pod).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(
				_ context.Context,
				_ client.Client,
				_ string,
				obj client.Object,
				_ ...client.SubResourceUpdateOption,
			) error {
				return newConflictError(obj.GetName())
			},
			Get: func(
				_ context.Context,
				_ client.WithWatch,
				_ client.ObjectKey,
				_ client.Object,
				_ ...client.GetOption,
			) error {
				return errGetFailed
			},
		}).
		Build()

	err := status.Update(context.Background(), c, pod, func(p *corev1.Pod) {
		p.Status.Phase = corev1.PodRunning
	})

	g.Expect(err).Should(HaveOccurred())
	g.Expect(errors.Is(err, errGetFailed)).To(BeTrue())
	g.Expect(errors.Is(err, status.ErrRetriesExhausted)).To(BeFalse())
}

func TestUpdate_NonConflictError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().
		WithObjects(pod).
		WithStatusSubresource(pod).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(
				_ context.Context,
				_ client.Client,
				_ string,
				_ client.Object,
				_ ...client.SubResourceUpdateOption,
			) error {
				return apierrors.NewNotFound(
					schema.GroupResource{Resource: "pods"},
					"test-pod",
				)
			},
		}).
		Build()

	err := status.Update(context.Background(), c, pod, func(p *corev1.Pod) {
		p.Status.Phase = corev1.PodRunning
	})

	g.Expect(err).Should(HaveOccurred())
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	g.Expect(errors.Is(err, status.ErrRetriesExhausted)).To(BeFalse())
}
