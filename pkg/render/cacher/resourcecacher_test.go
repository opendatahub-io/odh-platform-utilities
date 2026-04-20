package cacher_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/xid"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/render"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/render/cacher"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"

	. "github.com/onsi/gomega"
)

var (
	errHashing = errors.New("hashing error")
	errRender  = errors.New("render error")
)

type testCacher struct {
	mock.Mock

	c         *cacher.ResourceCacher
	rr        *render.ReconciliationRequest
	ctx       context.Context //nolint:containedctx
	r         resources.UnstructuredList
	doubleRes resources.UnstructuredList
}

func newHash() []byte {
	return xid.New().Bytes()
}

func newResources() []unstructured.Unstructured {
	return []unstructured.Unstructured{
		{
			Object: map[string]any{
				xid.New().String(): xid.New().String(),
			},
		},
	}
}

func newTestCacher() *testCacher {
	c := &testCacher{
		ctx: context.Background(),
		r:   newResources(),
		rr:  &render.ReconciliationRequest{},
	}

	rc := cacher.NewResourceCacher("test")
	rc.SetKeyFn(c.hashFn)
	c.c = &rc
	c.rr.Instance = &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "test"},
		},
	}
	c.doubleRes = append(c.doubleRes, c.r[0], c.r[0])

	return c
}

func (s *testCacher) resetGenerated() {
	s.rr.Generated = false
}

func (s *testCacher) setResources(r []unstructured.Unstructured) {
	s.rr.Resources = r
}

func (s *testCacher) hashFn(
	ctx context.Context, rr *render.ReconciliationRequest,
) ([]byte, error) {
	args := s.Called(ctx, rr)

	return args.Get(0).([]byte), args.Error(1) //nolint:errcheck,forcetypeassert
}

func (s *testCacher) doRender(
	ctx context.Context, rr *render.ReconciliationRequest,
) (resources.UnstructuredList, error) {
	args := s.Called(ctx, rr)

	return args.Get(0).(resources.UnstructuredList), args.Error(1) //nolint:errcheck,forcetypeassert
}

//nolint:paralleltest
func TestCacherShouldRenderFirstRun(t *testing.T) {
	g := NewWithT(t)
	m := newTestCacher()

	m.On("hashFn", m.ctx, m.rr).Return(newHash(), nil).Once()
	m.On("doRender", m.ctx, m.rr).Return(m.r, nil).Once()

	err := m.c.Render(m.ctx, m.rr, m.doRender)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(m.rr.Resources).Should(BeEquivalentTo(m.r))
	g.Expect(m.rr.Generated).Should(BeTrue())

	m.AssertExpectations(t)
}

//nolint:paralleltest
func TestCacherShouldNotRenderSecondRun(t *testing.T) {
	g := NewWithT(t)
	m := newTestCacher()

	m.On("hashFn", m.ctx, m.rr).Return(newHash(), nil).Twice()
	m.On("doRender", m.ctx, m.rr).Return(m.r, nil).Once()

	_ = m.c.Render(m.ctx, m.rr, m.doRender)
	m.resetGenerated()

	err := m.c.Render(m.ctx, m.rr, m.doRender)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(m.rr.Resources).Should(BeEquivalentTo(m.doubleRes))
	g.Expect(m.rr.Generated).Should(BeFalse())

	m.AssertExpectations(t)
}

//nolint:paralleltest
func TestCacherShouldRenderDifferentKey(t *testing.T) {
	g := NewWithT(t)
	m := newTestCacher()

	m.
		On("hashFn", m.ctx, m.rr).Return(newHash(), nil).Once().
		On("hashFn", m.ctx, m.rr).Return(newHash(), nil).Once()
	m.On("doRender", m.ctx, m.rr).Return(m.r, nil).Twice()

	_ = m.c.Render(m.ctx, m.rr, m.doRender)
	m.resetGenerated()

	err := m.c.Render(m.ctx, m.rr, m.doRender)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(m.rr.Resources).Should(BeEquivalentTo(m.doubleRes))
	g.Expect(m.rr.Generated).Should(BeTrue())

	m.AssertExpectations(t)
}

//nolint:paralleltest
func TestCacherShouldRenderIfKeyUnset(t *testing.T) {
	g := NewWithT(t)
	m := newTestCacher()

	m.c.SetKeyFn(nil)

	m.On("doRender", m.ctx, m.rr).Return(m.r, nil).Twice()

	_ = m.c.Render(m.ctx, m.rr, m.doRender)
	m.resetGenerated()

	err := m.c.Render(m.ctx, m.rr, m.doRender)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(m.rr.Resources).Should(BeEquivalentTo(m.doubleRes))
	g.Expect(m.rr.Generated).Should(BeTrue())

	m.AssertExpectations(t)
}

//nolint:paralleltest
func TestCacherShouldErrorIfKeyError(t *testing.T) {
	g := NewWithT(t)
	m := newTestCacher()

	m.On("hashFn", m.ctx, m.rr).Return(newHash(), errHashing).Once()

	err := m.c.Render(m.ctx, m.rr, m.doRender)

	g.Expect(err).Should(HaveOccurred())
	g.Expect(m.rr.Resources).Should(BeEmpty())
	g.Expect(m.rr.Generated).Should(BeFalse())

	m.AssertExpectations(t)
}

//nolint:paralleltest
func TestCacherShouldErrorIfRenderError(t *testing.T) {
	g := NewWithT(t)
	m := newTestCacher()

	m.On("hashFn", m.ctx, m.rr).Return(newHash(), nil).Once()
	m.On("doRender", m.ctx, m.rr).Return(cacher.Zero[resources.UnstructuredList](), errRender).Once()

	err := m.c.Render(m.ctx, m.rr, m.doRender)

	g.Expect(err).Should(HaveOccurred())
	g.Expect(m.rr.Resources).Should(BeEmpty())
	g.Expect(m.rr.Generated).Should(BeFalse())

	m.AssertExpectations(t)
}

//nolint:paralleltest
func TestCacherShouldAddResources(t *testing.T) {
	g := NewWithT(t)
	m := newTestCacher()

	orig := append(newResources(), newResources()...)
	m.setResources(orig)

	expected := append([]unstructured.Unstructured{}, orig...)
	expected = append(expected, m.r...)

	m.On("hashFn", m.ctx, m.rr).Return(newHash(), nil).Once()
	m.On("doRender", m.ctx, m.rr).Return(m.r, nil).Once()

	err := m.c.Render(m.ctx, m.rr, m.doRender)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(m.rr.Resources).Should(BeEquivalentTo(expected))

	m.AssertExpectations(t)
}
