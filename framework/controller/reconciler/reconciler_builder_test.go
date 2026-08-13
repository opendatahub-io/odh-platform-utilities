//nolint:testpackage
package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"

	. "github.com/onsi/gomega"
)

func TestReconcilerBuilder_ComposeWith(t *testing.T) {
	noop := func(_ context.Context, _ *types.ReconciliationRequest) error { return nil }

	t.Run("fn is called with the builder", func(t *testing.T) {
		g := NewWithT(t)
		called := false
		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.ComposeWith(func(b *ReconcilerBuilder[*testPlatformObject]) {
			called = true
		})
		g.Expect(called).To(BeTrue())
	})

	t.Run("returns the same builder", func(t *testing.T) {
		g := NewWithT(t)
		b := &ReconcilerBuilder[*testPlatformObject]{}
		result := b.ComposeWith(func(*ReconcilerBuilder[*testPlatformObject]) {})
		g.Expect(result).To(BeIdenticalTo(b))
	})

	t.Run("actions registered inside fn land at call position", func(t *testing.T) {
		g := NewWithT(t)
		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.WithAction(noop)
		b.ComposeWith(func(b *ReconcilerBuilder[*testPlatformObject]) {
			b.WithAction(noop)
			b.WithAction(noop)
		})
		b.WithAction(noop)
		g.Expect(b.actions).To(HaveLen(4))
	})

	t.Run("multiple ComposeWith calls compose correctly", func(t *testing.T) {
		g := NewWithT(t)
		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.ComposeWith(func(b *ReconcilerBuilder[*testPlatformObject]) {
			b.WithAction(noop)
		}).ComposeWith(func(b *ReconcilerBuilder[*testPlatformObject]) {
			b.WithAction(noop)
			b.WithAction(noop)
		})
		g.Expect(b.actions).To(HaveLen(3))
	})

	t.Run("nil fn panics immediately", func(t *testing.T) {
		g := NewWithT(t)
		b := &ReconcilerBuilder[*testPlatformObject]{}
		g.Expect(func() {
			b.ComposeWith(nil)
		}).To(Panic())
	})

	t.Run("errors from fn surface in b.errors and are returned by Build()", func(t *testing.T) {
		g := NewWithT(t)
		injected := errors.New("injected error")
		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.ComposeWith(func(b *ReconcilerBuilder[*testPlatformObject]) {
			b.errors = injected
		})
		_, buildErr := b.Build(context.Background())
		g.Expect(buildErr).To(MatchError(ContainSubstring(injected.Error())))
	})
}

func TestReconcilerBuilder_WithActionE(t *testing.T) {
	t.Run("adds action when no error", func(t *testing.T) {
		g := NewWithT(t)
		noop := func(_ context.Context, _ *types.ReconciliationRequest) error { return nil }
		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.WithActionE(noop, nil)
		g.Expect(b.actions).To(HaveLen(1))
		g.Expect(b.errors).ToNot(HaveOccurred())
	})

	t.Run("accumulates error and skips action", func(t *testing.T) {
		g := NewWithT(t)
		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.WithActionE(nil, errors.New("action init failed"))
		g.Expect(b.actions).To(BeEmpty())
		g.Expect(b.errors).To(HaveOccurred())
	})

	t.Run("error surfaces in Build()", func(t *testing.T) {
		g := NewWithT(t)
		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.WithActionE(nil, errors.New("action init failed"))
		_, buildErr := b.Build(context.Background())
		g.Expect(buildErr).To(MatchError(ContainSubstring("action init failed")))
	})

	t.Run("invalid conditions configuration surfaces in Build()", func(t *testing.T) {
		g := NewWithT(t)
		b := &ReconcilerBuilder[*testPlatformObject]{
			input: forInput{
				object: newTestPlatformObject(testGVKDashboard),
				gvk:    testGVKDashboard,
			},
			happyCondition: api.ConditionTypeReady,
			dependentConditions: []conditions.DependentDefinition{
				conditions.Dependent("DependencyReady", conditions.HealthyWhenTrue),
				conditions.Dependent("DependencyReady", conditions.HealthyWhenFalse),
			},
		}

		_, buildErr := b.Build(context.Background())
		g.Expect(buildErr).To(MatchError(ContainSubstring("invalid conditions manager configuration")))
	})
}

func TestWithConditionAggregation(t *testing.T) {
	t.Parallel()

	t.Run("stores configuration without panicking", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		opt := WithConditionAggregation(
			api.ConditionTypeReady,
			conditions.Dependent("DependencyReady", conditions.HealthyWhenTrue),
			conditions.Dependent("DependencyReady", conditions.HealthyWhenFalse),
		)

		reconciler := &Reconciler{}
		g.Expect(func() {
			opt(reconciler)
		}).ToNot(Panic())
		g.Expect(reconciler.conditionsManagerHappy).To(Equal(api.ConditionTypeReady))
		g.Expect(reconciler.conditionsManagerDependents).To(HaveLen(2))
	})

	t.Run("stores valid configuration", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		opt := WithConditionAggregation(
			api.ConditionTypeReady,
			conditions.Dependent("DependencyReady", conditions.HealthyWhenTrue),
		)

		reconciler := &Reconciler{}
		opt(reconciler)
		g.Expect(reconciler.conditionsManagerHappy).To(Equal(api.ConditionTypeReady))
		g.Expect(reconciler.conditionsManagerDependents).To(ConsistOf(
			conditions.Dependent("DependencyReady", conditions.HealthyWhenTrue),
		))
	})
}
