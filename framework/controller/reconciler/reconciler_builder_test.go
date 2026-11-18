//nolint:testpackage
package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/cluster/gvk"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// applyReconcilerOpts applies the given option groups to a bare Reconciler
// the way NewReconciler would, so builder wiring can be tested without a
// manager.
func applyReconcilerOpts(optGroups ...[]ReconcilerOpt) *Reconciler {
	r := &Reconciler{
		excludeFromDynamicOwnership: map[schema.GroupVersionKind]struct{}{},
		unownableGVKs:               map[schema.GroupVersionKind]struct{}{},
	}
	for _, group := range optGroups {
		for _, opt := range group {
			opt(r)
		}
	}
	return r
}

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

func TestWithConditionAggregator(t *testing.T) {
	t.Parallel()

	t.Run("stores nil aggregator without panicking", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		opt := WithConditionAggregator(nil)

		reconciler := &Reconciler{}
		g.Expect(func() {
			opt(reconciler)
		}).ToNot(Panic())
		g.Expect(reconciler.conditionsAggregator).To(BeNil())
	})

	t.Run("stores provided aggregator", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		aggregator := mustNewAggregator(
			conditions.Dependent("DependencyReady", conditions.HealthyWhenTrue),
		)
		opt := WithConditionAggregator(aggregator)

		reconciler := &Reconciler{}
		opt(reconciler)
		g.Expect(reconciler.conditionsAggregator).To(BeIdenticalTo(aggregator))
	})
}

func TestReconcilerBuilder_OwnsUnownableGVKs(t *testing.T) {
	t.Parallel()

	t.Run("Owns on a Namespace surfaces a build error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.Owns(resources.GvkToUnstructured(gvk.Namespace))

		g.Expect(b.errors).To(HaveOccurred())
		g.Expect(b.errors.Error()).To(ContainSubstring("Namespace"))
		g.Expect(b.errors.Error()).To(ContainSubstring("Watches"))
		_, err := b.Build(context.Background())
		g.Expect(err).To(MatchError(ContainSubstring("must not be registered as an owned type")))
	})

	t.Run("OwnsGVK on a CustomResourceDefinition surfaces a build error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.OwnsGVK(gvk.CustomResourceDefinition)

		g.Expect(b.errors).To(HaveOccurred())
		g.Expect(b.errors.Error()).To(ContainSubstring("CustomResourceDefinition"))
		_, err := b.Build(context.Background())
		g.Expect(err).To(MatchError(ContainSubstring("must not be registered as an owned type")))
	})

	t.Run("Owns on an ordinary GVK records no error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.Owns(resources.GvkToUnstructured(gvk.Deployment), WithEventHandler(handler.Funcs{}))

		g.Expect(b.errors).ToNot(HaveOccurred())
	})

	t.Run("WithUnownableGVKs extends the rejected set", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.WithUnownableGVKs(gvk.Secret)
		b.Owns(resources.GvkToUnstructured(gvk.Secret), WithEventHandler(handler.Funcs{}))

		g.Expect(b.errors).To(HaveOccurred())
		g.Expect(b.errors.Error()).To(ContainSubstring("Secret"))
	})

	t.Run("defaults remain rejected alongside custom unownable GVKs", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		// Extend-only: registering custom unownable GVKs must not
		// shadow the defaults — Namespace and CustomResourceDefinition
		// stay rejected however the option is used.
		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.WithUnownableGVKs(gvk.Secret)
		b.Owns(resources.GvkToUnstructured(gvk.Namespace), WithEventHandler(handler.Funcs{}))
		b.Owns(resources.GvkToUnstructured(gvk.CustomResourceDefinition), WithEventHandler(handler.Funcs{}))
		b.Owns(resources.GvkToUnstructured(gvk.Secret), WithEventHandler(handler.Funcs{}))

		g.Expect(b.errors).To(HaveOccurred())
		g.Expect(b.errors.Error()).To(ContainSubstring("Namespace"))
		g.Expect(b.errors.Error()).To(ContainSubstring("CustomResourceDefinition"))
		g.Expect(b.errors.Error()).To(ContainSubstring("Secret"))
	})

	t.Run("WithUnownableGVKs after Owns still fails the build", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		// Order reversal: the watch is recorded while the GVK is still
		// allowed, and only afterwards declared unownable. The Owns()-time
		// check cannot catch this; Build() must.
		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.OwnsGVK(gvk.Secret, WithEventHandler(handler.Funcs{}))
		b.WithUnownableGVKs(gvk.Secret)

		_, err := b.Build(context.Background())
		g.Expect(err).To(MatchError(ContainSubstring("must not be registered as an owned type")))
	})

	t.Run("WithUnownableGVKs excludes the GVK from dynamic ownership enabled via the builder", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		// Apply the exact options Build() assembles, minus the manager
		// wiring it cannot run without: the ownership decisions under
		// test live entirely in these options.
		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.WithUnownableGVKs(gvk.Secret)
		b.WithDynamicOwnership(ExcludeGVKs(gvk.ConfigMap))

		r := applyReconcilerOpts(b.ownershipExclusionOpts(), b.reconcilerOpts)
		g.Expect(r.IsDynamicOwnershipEnabled()).To(BeTrue())
		g.Expect(r.IsExcludedFromDynamicOwnership(gvk.Secret)).To(BeTrue())
		g.Expect(r.IsExcludedFromDynamicOwnership(gvk.ConfigMap)).To(BeTrue())
	})

	t.Run("WithUnownableGVKs excludes the GVK from dynamic ownership enabled via WithReconcilerOpts", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		// Dynamic ownership can also be enabled through the runtime
		// option instead of the builder method; the unownable set must
		// reach the runtime ownership decision either way.
		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.WithUnownableGVKs(gvk.Secret)
		b.WithReconcilerOpts(WithDynamicOwnership())

		r := applyReconcilerOpts(b.ownershipExclusionOpts(), b.reconcilerOpts)
		g.Expect(r.IsDynamicOwnershipEnabled()).To(BeTrue())
		g.Expect(r.IsExcludedFromDynamicOwnership(gvk.Secret)).To(BeTrue())
	})

	t.Run("rejection message explains the event mapper for kinds without the instance annotation", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.OwnsGVK(gvk.CustomResourceDefinition)

		g.Expect(b.errors).To(HaveOccurred())
		g.Expect(b.errors.Error()).To(ContainSubstring("event mapper"))
		g.Expect(b.errors.Error()).To(ContainSubstring("WithPredicates"))
	})

	t.Run("WithUnownableGVKs marks the GVK unownable on the runtime controller", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.WithUnownableGVKs(gvk.Secret)

		r := applyReconcilerOpts(b.ownershipExclusionOpts(), b.reconcilerOpts)
		g.Expect(r.IsUnownableGVK(gvk.Secret)).To(BeTrue())
		g.Expect(r.IsUnownableGVK(gvk.ConfigMap)).To(BeFalse())
	})

	t.Run("ExcludeGVKs does not mark the GVK unownable", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		// Excluding a GVK from dynamic ownership is not the same contract
		// as declaring it unownable: static Owns() keeps its documented
		// precedence over the exclusion, so the two must stay separate at
		// the runtime controller too.
		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.WithDynamicOwnership(ExcludeGVKs(gvk.Secret))

		r := applyReconcilerOpts(b.ownershipExclusionOpts(), b.reconcilerOpts)
		g.Expect(r.IsExcludedFromDynamicOwnership(gvk.Secret)).To(BeTrue())
		g.Expect(r.IsUnownableGVK(gvk.Secret)).To(BeFalse())
	})

	t.Run("restating a default unownable GVK does not exclude it from the runtime set", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		// The defaults' ownership enforcement does not come from the
		// exclusion set (the shouldOwn Namespace guard and the CRD
		// deploy dispatch handle it); forwarding a restated default
		// would only suppress the dynamic CRD by-name watch through
		// the exclusion check in the dynamic ownership action.
		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.WithUnownableGVKs(gvk.CustomResourceDefinition)

		r := applyReconcilerOpts(b.ownershipExclusionOpts(), b.reconcilerOpts)
		g.Expect(r.IsExcludedFromDynamicOwnership(gvk.CustomResourceDefinition)).To(BeFalse())
	})

	t.Run("Watches on a Namespace records no error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		b := &ReconcilerBuilder[*testPlatformObject]{}
		b.Watches(resources.GvkToUnstructured(gvk.Namespace), WithEventHandler(handler.Funcs{}))

		g.Expect(b.errors).ToNot(HaveOccurred())
	})
}
