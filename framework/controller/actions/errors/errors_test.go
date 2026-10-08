package errors_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	odherrors "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"

	. "github.com/onsi/gomega"
)

func TestNewStopError(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	se := odherrors.NewStopError("something went %s", "wrong")

	g.Expect(se.Error()).To(Equal("something went wrong"))
	g.Expect(se.RequeueAfter()).To(BeZero())
}

func TestNewStopErrorW(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	cause := errors.New("root cause")
	se := odherrors.NewStopErrorW(cause)

	g.Expect(se.Error()).To(Equal("root cause"))
	g.Expect(se.RequeueAfter()).To(BeZero())
	g.Expect(errors.Is(se, cause)).To(BeTrue())
}

func TestStopErrorSatisfiesErrorInterface(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	var err error = odherrors.NewStopError("test")

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(Equal("test"))
}

func TestStopErrorAsFromWrappedChain(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	se := odherrors.NewStopError("inner stop")
	wrapped := fmt.Errorf("outer: %w", se)

	var extracted odherrors.StopError
	g.Expect(errors.As(wrapped, &extracted)).To(BeTrue())
	g.Expect(extracted.Error()).To(Equal("inner stop"))
	g.Expect(extracted.RequeueAfter()).To(BeZero())
}

func TestStopErrorWithRequeueAfterAsFromWrappedChain(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	se := odherrors.NewStopError("waiting").WithRequeueAfter(45 * time.Second)
	wrapped := fmt.Errorf("provisioning paused: %w", se)

	var extracted odherrors.StopError
	g.Expect(errors.As(wrapped, &extracted)).To(BeTrue())
	g.Expect(extracted.Error()).To(Equal("waiting"))
	g.Expect(extracted.RequeueAfter()).To(Equal(45 * time.Second))
}

func TestStopErrorWithRequeueAfter(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	se := odherrors.NewStopError("waiting for %s", "dependency").WithRequeueAfter(30 * time.Second)

	g.Expect(se.Error()).To(Equal("waiting for dependency"))
	g.Expect(se.RequeueAfter()).To(Equal(30 * time.Second))
}

func TestStopErrorWWithRequeueAfter(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	cause := errors.New("dependency missing")
	se := odherrors.NewStopErrorW(cause).WithRequeueAfter(1 * time.Minute)

	g.Expect(se.Error()).To(Equal("dependency missing"))
	g.Expect(se.RequeueAfter()).To(Equal(1 * time.Minute))
	g.Expect(errors.Is(se, cause)).To(BeTrue())
}

func TestActionError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		errorType odherrors.ActionErrorType
		wantType  odherrors.ActionErrorType
		wantAfter time.Duration
	}{
		{
			name:      "terminal",
			errorType: odherrors.ActionErrorTerminal,
			wantType:  odherrors.ActionErrorTerminal,
		},
		{
			name:      "non-blocking error with requeue",
			errorType: odherrors.ActionErrorNonBlocking,
			wantType:  odherrors.ActionErrorNonBlocking,
			wantAfter: time.Minute,
		},
		{
			name:      "advisory",
			errorType: odherrors.ActionErrorAdvisory,
			wantType:  odherrors.ActionErrorAdvisory,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			cause := errors.New("root cause")

			actionErr := odherrors.NewActionErrorW(cause)
			switch tc.errorType {
			case odherrors.ActionErrorTerminal:
				actionErr = actionErr.Terminal()
			case odherrors.ActionErrorNonBlocking:
				actionErr = actionErr.NonBlocking()
			case odherrors.ActionErrorAdvisory:
				actionErr = actionErr.Advisory()
			}
			if tc.wantAfter > 0 {
				actionErr = actionErr.WithRequeueAfter(tc.wantAfter)
			}

			g.Expect(actionErr.Type()).To(Equal(tc.wantType))
			g.Expect(actionErr.RequeueAfter()).To(Equal(tc.wantAfter))
			g.Expect(actionErr.Error()).To(Equal(cause.Error()))
			g.Expect(actionErr.Err()).To(MatchError(cause.Error()))
			g.Expect(errors.Is(actionErr, cause)).To(BeTrue())
		})
	}
}

func TestNewActionErrorf(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	actionErr := odherrors.NewActionErrorf("waiting for %s", "dependency")

	g.Expect(actionErr.Type()).To(Equal(odherrors.ActionErrorTerminal))
	g.Expect(actionErr.Error()).To(Equal("waiting for dependency"))
}

func TestActionErrorAdd(t *testing.T) {
	t.Parallel()

	t.Run("terminal stops, retains earlier diagnostics, and ignores earlier delays", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		result, stop := odherrors.ActionError{}.Add("first", odherrors.NewActionError("rollout pending").
			Advisory().WithRequeueAfter(15*time.Second))
		g.Expect(stop).To(BeFalse())

		terminal := odherrors.NewActionError("dependency unavailable").Terminal().
			WithRequeueAfter(time.Minute)
		result, stop = result.Add("second", terminal)

		g.Expect(stop).To(BeTrue())
		g.Expect(result.Type()).To(Equal(odherrors.ActionErrorTerminal))
		g.Expect(result.RequeueAfter()).To(Equal(time.Minute))
		g.Expect(result.Error()).To(ContainSubstring("dependency unavailable"))
		g.Expect(result.Error()).To(ContainSubstring("rollout pending"))
	})

	t.Run("plain errors stop and discard earlier delays", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		advisory := odherrors.NewActionError("waiting").Advisory().
			WithRequeueAfter(time.Minute)
		result, stop := odherrors.ActionError{}.Add("advice", advisory)
		g.Expect(stop).To(BeFalse())

		result, stop = result.Add("failure", errors.New("temporary API failure"))
		g.Expect(stop).To(BeTrue())
		g.Expect(result.Type()).To(Equal(odherrors.ActionErrorTerminal))
		g.Expect(result.RequeueAfter()).To(BeZero())
		g.Expect(result.Error()).To(ContainSubstring("waiting"))
		g.Expect(result.Error()).To(ContainSubstring("temporary API failure"))
	})

	t.Run("plain errors override delayed terminals in a joined return", func(t *testing.T) {
		t.Parallel()
		plainErr := errors.New("render failed")
		stopErr := odherrors.NewStopError("dependency pending").WithRequeueAfter(time.Minute)

		tests := []struct {
			name string
			err  error
		}{
			{name: "plain error first", err: errors.Join(plainErr, stopErr)},
			{name: "plain error last", err: errors.Join(stopErr, plainErr)},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				g := NewWithT(t)

				result, stop := odherrors.ActionError{}.Add("render", tc.err)

				g.Expect(stop).To(BeTrue())
				g.Expect(result.Type()).To(Equal(odherrors.ActionErrorTerminal))
				g.Expect(result.RequeueAfter()).To(BeZero())
				g.Expect(result.Error()).To(ContainSubstring(plainErr.Error()))
				g.Expect(result.Error()).To(ContainSubstring(stopErr.Error()))
			})
		}
	})

	t.Run("an outer delayed terminal explicitly schedules joined plain errors", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		plainErr := errors.New("render failed")
		joined := errors.Join(plainErr, errors.New("dependency pending"))
		delayed := odherrors.NewActionErrorW(joined).WithRequeueAfter(time.Minute)

		result, stop := odherrors.ActionError{}.Add("render", delayed)

		g.Expect(stop).To(BeTrue())
		g.Expect(result.Type()).To(Equal(odherrors.ActionErrorTerminal))
		g.Expect(result.RequeueAfter()).To(Equal(time.Minute))
		g.Expect(errors.Is(result, plainErr)).To(BeTrue())
	})

	t.Run("joined terminal errors retain all diagnostics and use the earliest delay", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		first := odherrors.NewActionError("first dependency pending").WithRequeueAfter(time.Minute)
		second := odherrors.NewActionError("second dependency pending").WithRequeueAfter(15 * time.Second)

		result, stop := odherrors.ActionError{}.Add("render", errors.Join(first, second))

		g.Expect(stop).To(BeTrue())
		g.Expect(result.Type()).To(Equal(odherrors.ActionErrorTerminal))
		g.Expect(result.RequeueAfter()).To(Equal(15 * time.Second))
		g.Expect(result.Error()).To(ContainSubstring(first.Error()))
		g.Expect(result.Error()).To(ContainSubstring(second.Error()))
	})

	t.Run("a delayed terminal wins over an undelayed joined terminal", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		undelayed := odherrors.NewActionError("configuration invalid")
		delayed := odherrors.NewStopError("dependency pending").WithRequeueAfter(45 * time.Second)

		result, stop := odherrors.ActionError{}.Add("render", errors.Join(undelayed, delayed))

		g.Expect(stop).To(BeTrue())
		g.Expect(result.Type()).To(Equal(odherrors.ActionErrorTerminal))
		g.Expect(result.RequeueAfter()).To(Equal(45 * time.Second))
		g.Expect(result.Error()).To(ContainSubstring(undelayed.Error()))
		g.Expect(result.Error()).To(ContainSubstring(delayed.Error()))
	})

	t.Run("uses the earliest semantic requeue", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		first := odherrors.NewActionError("first").Advisory().
			WithRequeueAfter(time.Minute)
		second := odherrors.NewActionError("second").
			NonBlocking().WithRequeueAfter(15 * time.Second)

		result, _ := odherrors.ActionError{}.Add("first", first)
		result, _ = result.Add("second", second)

		g.Expect(result.Type()).To(Equal(odherrors.ActionErrorNonBlocking))
		g.Expect(result.RequeueAfter()).To(Equal(15 * time.Second))
	})

	t.Run("rejects a manually constructed zero-value action error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		result, stop := odherrors.ActionError{}.Add("invalid", odherrors.ActionError{})

		g.Expect(stop).To(BeTrue())
		g.Expect(result.Type()).To(Equal(odherrors.ActionErrorTerminal))
		g.Expect(result.Error()).To(ContainSubstring("invalid ActionError type 0"))
	})

	t.Run("preserves a wrapped action error's classifier and delay", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		wrapped := fmt.Errorf("waiting for rollout: %w", odherrors.NewActionError("rollout pending").
			Advisory().WithRequeueAfter(time.Minute))
		result, stop := odherrors.ActionError{}.Add("wait", wrapped)

		g.Expect(stop).To(BeFalse())
		g.Expect(result.Type()).To(Equal(odherrors.ActionErrorAdvisory))
		g.Expect(result.RequeueAfter()).To(Equal(time.Minute))
		g.Expect(result.Error()).To(ContainSubstring("waiting for rollout"))
		g.Expect(result.Error()).To(ContainSubstring("rollout pending"))
	})

	t.Run("preserves a wrapped plain error's context", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		cause := errors.New("connection refused")
		wrapped := fmt.Errorf("deploying ConfigMap: %w", cause)

		result, stop := odherrors.ActionError{}.Add("deploy", wrapped)

		g.Expect(stop).To(BeTrue())
		g.Expect(result.Type()).To(Equal(odherrors.ActionErrorTerminal))
		g.Expect(result.Error()).To(ContainSubstring("deploying ConfigMap: connection refused"))
		g.Expect(errors.Is(result, cause)).To(BeTrue())
	})

	t.Run("preserves outer context for wrapped joined plain errors", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		first := errors.New("secret missing")
		second := errors.New("config invalid")
		wrapped := fmt.Errorf("rendering manifests: %w", errors.Join(first, second))

		result, stop := odherrors.ActionError{}.Add("render", wrapped)

		g.Expect(stop).To(BeTrue())
		g.Expect(result.Type()).To(Equal(odherrors.ActionErrorTerminal))
		g.Expect(result.Error()).To(ContainSubstring("rendering manifests"))
		g.Expect(result.Error()).To(ContainSubstring(first.Error()))
		g.Expect(result.Error()).To(ContainSubstring(second.Error()))
		g.Expect(errors.Is(result, first)).To(BeTrue())
		g.Expect(errors.Is(result, second)).To(BeTrue())
	})

	t.Run("joined action errors select failure and retain all messages", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		advisory := odherrors.NewActionError("rollout pending").Advisory().
			WithRequeueAfter(time.Minute)
		failure := odherrors.NewActionError("invalid configuration").NonBlocking()
		result, stop := odherrors.ActionError{}.Add("validate", errors.Join(advisory, failure))

		g.Expect(stop).To(BeFalse())
		g.Expect(result.Type()).To(Equal(odherrors.ActionErrorNonBlocking))
		g.Expect(result.RequeueAfter()).To(Equal(time.Minute))
		g.Expect(result.Error()).To(ContainSubstring("rollout pending"))
		g.Expect(result.Error()).To(ContainSubstring("invalid configuration"))
	})
}
