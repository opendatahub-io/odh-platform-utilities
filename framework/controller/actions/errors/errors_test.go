package errors_test

import (
	"errors"
	stderrors "errors"
	"fmt"
	"testing"
	"time"

	odherrors "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"

	. "github.com/onsi/gomega"
)

func TestNewStopError(t *testing.T) {
	g := NewWithT(t)

	se := odherrors.NewStopError("something went %s", "wrong")

	g.Expect(se.Error()).To(Equal("something went wrong"))
	g.Expect(se.RequeueAfter()).To(BeZero())
}

func TestNewStopErrorW(t *testing.T) {
	g := NewWithT(t)

	cause := stderrors.New("root cause")
	se := odherrors.NewStopErrorW(cause)

	g.Expect(se.Error()).To(Equal("root cause"))
	g.Expect(se.RequeueAfter()).To(BeZero())
	g.Expect(errors.Is(se, cause)).To(BeTrue())
}

func TestNewStopErrorWithRequeueAfter(t *testing.T) {
	g := NewWithT(t)

	se := odherrors.NewStopErrorWithRequeueAfter(30*time.Second, "waiting for %s", "configmap")

	g.Expect(se.Error()).To(Equal("waiting for configmap"))
	g.Expect(se.RequeueAfter()).To(Equal(30 * time.Second))
}

func TestNewStopErrorWithRequeueAfterW(t *testing.T) {
	g := NewWithT(t)

	cause := stderrors.New("dependency missing")
	se := odherrors.NewStopErrorWithRequeueAfterW(1*time.Minute, cause)

	g.Expect(se.Error()).To(Equal("dependency missing"))
	g.Expect(se.RequeueAfter()).To(Equal(1 * time.Minute))
	g.Expect(errors.Is(se, cause)).To(BeTrue())
}

func TestStopErrorSatisfiesErrorInterface(t *testing.T) {
	g := NewWithT(t)

	var err error = odherrors.NewStopError("test")

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(Equal("test"))
}

func TestStopErrorAsFromWrappedChain(t *testing.T) {
	g := NewWithT(t)

	se := odherrors.NewStopError("inner stop")
	wrapped := fmt.Errorf("outer: %w", se)

	var extracted odherrors.StopError
	g.Expect(errors.As(wrapped, &extracted)).To(BeTrue())
	g.Expect(extracted.Error()).To(Equal("inner stop"))
	g.Expect(extracted.RequeueAfter()).To(BeZero())
}

func TestStopErrorWithRequeueAfterAsFromWrappedChain(t *testing.T) {
	g := NewWithT(t)

	se := odherrors.NewStopErrorWithRequeueAfter(45*time.Second, "waiting")
	wrapped := fmt.Errorf("provisioning paused: %w", se)

	var extracted odherrors.StopError
	g.Expect(errors.As(wrapped, &extracted)).To(BeTrue())
	g.Expect(extracted.Error()).To(Equal("waiting"))
	g.Expect(extracted.RequeueAfter()).To(Equal(45 * time.Second))
}
