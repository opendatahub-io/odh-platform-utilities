package conditions_test

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"

	. "github.com/onsi/gomega"
)

const (
	readyCondition        = api.ConditionType("Ready")
	dependency1Condition  = api.ConditionType("Dependency1")
	dependency2Condition  = api.ConditionType("Dependency2")
	deploymentsAvailable  = api.ConditionType("DeploymentsAvailable")
	dependenciesAvailable = api.ConditionType("DependenciesAvailable")
)

type fakeAccessor struct {
	conditions []api.Condition
}

func (f *fakeAccessor) GetConditions() []api.Condition {
	return f.conditions
}

func (f *fakeAccessor) SetConditions(values []api.Condition) {
	f.conditions = values
}

type copyingAccessor struct {
	conditions []api.Condition
}

func (c *copyingAccessor) GetConditions() []api.Condition {
	return append([]api.Condition(nil), c.conditions...)
}

func (c *copyingAccessor) SetConditions(values []api.Condition) {
	c.conditions = append([]api.Condition(nil), values...)
}

func newManager(
	accessor *fakeAccessor,
	happy api.ConditionType,
	dependents ...api.ConditionType,
) *conditions.Manager {
	specs := make([]conditions.DependentDefinition, 0, len(dependents))
	for _, dependent := range dependents {
		specs = append(specs, conditions.Dependent(dependent, conditions.HealthyWhenTrue))
	}

	aggregator, err := conditions.NewAggregator(happy, specs...)
	if err != nil {
		panic(err)
	}

	return conditions.NewManager(accessor, aggregator)
}

func TestManager_InitializeConditions(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	manager := newManager(accessor, readyCondition, dependency1Condition, dependency2Condition)

	g.Expect(accessor.GetConditions()).To(HaveLen(3))
	g.Expect(manager.GetCondition(readyCondition)).NotTo(BeNil())
	g.Expect(manager.GetCondition(readyCondition).Status).To(Equal(metav1.ConditionUnknown))
	g.Expect(manager.GetCondition(dependency1Condition)).NotTo(BeNil())
	g.Expect(manager.GetCondition(dependency2Condition)).NotTo(BeNil())
}

func TestManager_IsHappy(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	manager := newManager(accessor, readyCondition, dependency1Condition, dependency2Condition)

	g.Expect(manager.IsHappy()).To(BeFalse())

	manager.MarkFalse(dependency1Condition)
	manager.MarkFalse(dependency2Condition)

	g.Expect(manager.IsHappy()).To(BeFalse())

	manager.MarkTrue(dependency1Condition)
	g.Expect(manager.IsHappy()).To(BeFalse())

	manager.MarkTrue(dependency2Condition)
	g.Expect(manager.IsHappy()).To(BeTrue())
}

func TestManager_IsHappy_NoDependents(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	accessor.SetConditions([]api.Condition{
		{Type: string(dependency1Condition), Status: metav1.ConditionUnknown},
		{Type: string(dependency2Condition), Status: metav1.ConditionUnknown},
	})

	manager := newManager(accessor, readyCondition)
	g.Expect(manager.IsHappy()).To(BeFalse())

	manager.MarkFalse(dependency1Condition)
	g.Expect(manager.IsHappy()).To(BeFalse())

	manager.MarkTrue(dependency1Condition)
	g.Expect(manager.IsHappy()).To(BeFalse())

	manager.MarkFalse(dependency2Condition)
	g.Expect(manager.IsHappy()).To(BeFalse())

	manager.MarkTrue(dependency2Condition)
	g.Expect(manager.IsHappy()).To(BeTrue())
}

func TestManager_SetAndClearCondition(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	manager := newManager(accessor, readyCondition, dependency1Condition)

	manager.MarkTrue(dependency1Condition)
	g.Expect(manager.GetCondition(dependency1Condition)).NotTo(BeNil())
	g.Expect(manager.GetCondition(dependency1Condition).Status).To(Equal(metav1.ConditionTrue))

	manager.ClearCondition(dependency1Condition)
	g.Expect(manager.GetCondition(dependency1Condition)).To(BeNil())
}

func TestManager_RecomputeHappiness(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	manager := newManager(accessor, readyCondition, dependency1Condition, dependency2Condition)

	manager.MarkTrue(dependency1Condition)
	manager.MarkFalse(dependency2Condition, conditions.WithSeverity(api.ConditionSeverityError))
	g.Expect(manager.IsHappy()).To(BeFalse())
	g.Expect(manager.GetTopLevelCondition().Status).To(Equal(metav1.ConditionFalse))

	manager.MarkTrue(dependency2Condition)
	g.Expect(manager.IsHappy()).To(BeTrue())
}

func TestManager_ManualHappyConditionWriteCanBeOverriddenByRecompute(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	manager := newManager(accessor, readyCondition, dependency1Condition)

	manager.MarkFalse(
		dependency1Condition,
		conditions.WithReason("Broken"),
		conditions.WithMessage("dependency is unhealthy"),
	)
	g.Expect(manager.GetCondition(readyCondition).Status).To(Equal(metav1.ConditionFalse))

	manager.MarkTrue(
		readyCondition,
		conditions.WithReason("ForcedReady"),
		conditions.WithMessage("manual override"),
	)
	g.Expect(manager.GetCondition(readyCondition).Status).To(Equal(metav1.ConditionTrue))
	g.Expect(manager.GetCondition(readyCondition).Reason).To(Equal("ForcedReady"))

	manager.RecomputeHappiness()
	g.Expect(manager.GetCondition(readyCondition).Status).To(Equal(metav1.ConditionFalse))
	g.Expect(manager.GetCondition(readyCondition).Reason).To(Equal("Broken"))
}

func TestManager_HealthyWhenFalse(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	manager := conditions.NewManager(
		accessor,
		mustNewAggregator(
			readyCondition,
			conditions.Dependent(dependency1Condition, conditions.HealthyWhenFalse),
		),
	)

	manager.MarkHealthy(dependency1Condition)
	g.Expect(manager.GetCondition(dependency1Condition).Status).To(Equal(metav1.ConditionFalse))
	g.Expect(manager.IsHappy()).To(BeTrue())

	manager.MarkUnhealthy(
		dependency1Condition,
		conditions.WithReason("Degraded"),
		conditions.WithMessage("dependency is degraded"),
	)
	g.Expect(manager.GetCondition(dependency1Condition).Status).To(Equal(metav1.ConditionTrue))
	g.Expect(manager.GetTopLevelCondition().Reason).To(Equal("Degraded"))
	g.Expect(manager.IsHappy()).To(BeFalse())
}

func TestManager_ResetPreservesConditions(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	manager := newManager(accessor, readyCondition, dependency1Condition, dependency2Condition)

	manager.MarkTrue(dependency1Condition)
	manager.MarkTrue(dependency2Condition)
	g.Expect(accessor.GetConditions()).To(HaveLen(3))

	manager.Reset()

	g.Expect(accessor.GetConditions()).To(HaveLen(3))
	g.Expect(manager.GetCondition(readyCondition)).NotTo(BeNil())
	g.Expect(manager.GetCondition(dependency1Condition)).NotTo(BeNil())
	g.Expect(manager.GetCondition(dependency2Condition)).NotTo(BeNil())
}

func TestManager_CleanupStaleConditions(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	accessor.SetConditions([]api.Condition{
		{Type: string(dependency1Condition), Status: metav1.ConditionTrue},
		{Type: string(dependency2Condition), Status: metav1.ConditionTrue},
	})

	manager := newManager(accessor, readyCondition, dependency1Condition)
	manager.Reset()
	manager.MarkTrue(dependency1Condition)
	manager.CleanupStaleConditions()

	g.Expect(manager.GetCondition(dependency1Condition)).NotTo(BeNil())
	g.Expect(manager.GetCondition(dependency2Condition)).To(BeNil())
	g.Expect(manager.GetCondition(readyCondition)).NotTo(BeNil())
}

func TestManager_CleanupStaleConditionsPreservesHappy(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	manager := newManager(accessor, readyCondition, dependency1Condition)

	manager.MarkTrue(dependency1Condition)
	g.Expect(manager.IsHappy()).To(BeTrue())

	manager.Reset()
	manager.MarkTrue(dependency1Condition)
	manager.CleanupStaleConditions()

	g.Expect(manager.GetCondition(readyCondition)).NotTo(BeNil())
	g.Expect(manager.IsHappy()).To(BeTrue())
}

func TestManager_TimestampPreservedWhenConditionUnchanged(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	manager := newManager(accessor, readyCondition, dependency1Condition)

	manager.MarkTrue(dependency1Condition, conditions.WithReason("TestReason"), conditions.WithMessage("test message"))

	originalCondition := manager.GetCondition(dependency1Condition)
	g.Expect(originalCondition).NotTo(BeNil())
	originalTime := originalCondition.LastTransitionTime

	time.Sleep(time.Millisecond)

	manager.Reset()

	manager.MarkTrue(dependency1Condition, conditions.WithReason("TestReason"), conditions.WithMessage("test message"))

	updatedCondition := manager.GetCondition(dependency1Condition)
	g.Expect(updatedCondition).NotTo(BeNil())
	g.Expect(updatedCondition.LastTransitionTime).To(Equal(originalTime))
}

func TestManager_CleanupStaleConditionsRecomputesHappiness(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	accessor.SetConditions([]api.Condition{
		{Type: string(dependency1Condition), Status: metav1.ConditionTrue},
		{
			Type:     string(dependency2Condition),
			Status:   metav1.ConditionFalse,
			Reason:   "Broken",
			Message:  "something failed",
			Severity: api.ConditionSeverityError,
		},
	})

	manager := newManager(accessor, readyCondition, dependency1Condition)
	g.Expect(manager.IsHappy()).To(BeFalse())

	manager.Reset()
	manager.MarkTrue(dependency1Condition)
	manager.CleanupStaleConditions()

	g.Expect(manager.GetCondition(dependency2Condition)).To(BeNil())
	g.Expect(manager.GetCondition(readyCondition)).NotTo(BeNil())
	g.Expect(manager.IsHappy()).To(BeTrue())
}

func TestManager_CleanupStaleConditionsNoopWithoutReset(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	manager := newManager(accessor, readyCondition, dependency1Condition, dependency2Condition)

	manager.MarkTrue(dependency1Condition)
	manager.MarkTrue(dependency2Condition)
	g.Expect(accessor.GetConditions()).To(HaveLen(3))

	manager.CleanupStaleConditions()

	g.Expect(accessor.GetConditions()).To(HaveLen(3))
}

func TestManager_UnsetDependentsBlockHappiness(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	manager := newManager(accessor, readyCondition, deploymentsAvailable, dependenciesAvailable)

	manager.Reset()

	manager.MarkTrue(deploymentsAvailable)

	manager.CleanupStaleConditions()
	manager.RecomputeHappiness()

	g.Expect(manager.IsHappy()).To(BeFalse(), "Ready must be False when a declared dependent was not set")

	cond := manager.GetCondition(dependenciesAvailable)
	g.Expect(cond).NotTo(BeNil(), "unset dependent must not be removed")
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(conditions.ConditionReasonNotSet))
}

func TestManager_AllDependentsSetAllowsHappiness(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	manager := newManager(accessor, readyCondition, deploymentsAvailable, dependenciesAvailable)

	manager.Reset()

	manager.MarkTrue(deploymentsAvailable)
	manager.MarkTrue(dependenciesAvailable)

	manager.CleanupStaleConditions()
	manager.RecomputeHappiness()

	g.Expect(manager.IsHappy()).To(BeTrue(), "Ready should be True when all dependents are set")
}

func TestManager_NonDependentStaleConditionRemoved(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	accessor.SetConditions([]api.Condition{
		{Type: string(dependency1Condition), Status: metav1.ConditionTrue},
		{Type: "OrphanedCondition", Status: metav1.ConditionTrue},
	})

	manager := newManager(accessor, readyCondition, dependency1Condition)

	manager.Reset()
	manager.MarkTrue(dependency1Condition)

	manager.CleanupStaleConditions()

	g.Expect(manager.GetCondition(dependency1Condition)).NotTo(BeNil())
	g.Expect(manager.GetCondition(api.ConditionType("OrphanedCondition"))).To(BeNil(), "non-dependent stale condition should be removed")
	g.Expect(manager.IsHappy()).To(BeTrue())
}

func TestManager_UnsetDependentRecoversOnNextCycle(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	manager := newManager(accessor, readyCondition, deploymentsAvailable)

	manager.Reset()
	manager.CleanupStaleConditions()
	manager.RecomputeHappiness()

	g.Expect(manager.IsHappy()).To(BeFalse())
	cond := manager.GetCondition(deploymentsAvailable)
	g.Expect(cond).NotTo(BeNil())
	g.Expect(cond.Reason).To(Equal(conditions.ConditionReasonNotSet))

	manager2 := newManager(accessor, readyCondition, deploymentsAvailable)
	manager2.Reset()
	manager2.MarkTrue(deploymentsAvailable)

	manager2.CleanupStaleConditions()
	manager2.RecomputeHappiness()

	g.Expect(manager2.IsHappy()).To(BeTrue(), "should recover when dependent is set on next cycle")
}

func TestManager_MultipleDependentsPartiallySet(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	condA := api.ConditionType("CondA")
	condB := api.ConditionType("CondB")
	condC := api.ConditionType("CondC")

	accessor := &fakeAccessor{}
	manager := newManager(accessor, readyCondition, condA, condB, condC)

	manager.Reset()

	manager.MarkTrue(condA)
	manager.MarkTrue(condC)

	manager.CleanupStaleConditions()
	manager.RecomputeHappiness()

	g.Expect(manager.IsHappy()).To(BeFalse(), "should be unhappy when any dependent is missing")

	g.Expect(manager.GetCondition(condA).Status).To(Equal(metav1.ConditionTrue))
	g.Expect(manager.GetCondition(condB).Status).To(Equal(metav1.ConditionFalse))
	g.Expect(manager.GetCondition(condB).Reason).To(Equal(conditions.ConditionReasonNotSet))
	g.Expect(manager.GetCondition(condC).Status).To(Equal(metav1.ConditionTrue))
}

func TestManager_UnsetHealthyWhenFalseDependentIsMarkedExplicitlyUnhealthy(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{}
	manager := conditions.NewManager(
		accessor,
		mustNewAggregator(
			readyCondition,
			conditions.Dependent(dependency1Condition, conditions.HealthyWhenFalse),
		),
	)

	manager.Reset()
	manager.CleanupStaleConditions()
	manager.RecomputeHappiness()

	cond := manager.GetCondition(dependency1Condition)
	g.Expect(cond).NotTo(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(cond.Reason).To(Equal(conditions.ConditionReasonNotSet))
	g.Expect(manager.GetCondition(readyCondition).Status).To(Equal(metav1.ConditionFalse))
}

func TestManager_Sort(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &fakeAccessor{conditions: make([]api.Condition, 0)}

	manager := newManager(
		accessor,
		api.ConditionType("Z"),
		api.ConditionType("A"),
		api.ConditionType("C"),
	)
	manager.MarkTrue(api.ConditionType("B"))
	manager.MarkTrue(api.ConditionType("D"))
	manager.MarkTrue(api.ConditionType("E"))
	manager.Sort()

	result := make([]string, 0, len(accessor.conditions))
	for _, c := range accessor.conditions {
		result = append(result, c.Type)
	}

	g.Expect(result).To(Equal([]string{
		"Z",
		"A",
		"C",
		"B",
		"D",
		"E",
	}))
}

func TestManager_SortPersistsViaAccessor(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	accessor := &copyingAccessor{
		conditions: []api.Condition{
			{Type: "B", Status: metav1.ConditionTrue},
			{Type: "Z", Status: metav1.ConditionTrue},
			{Type: "C", Status: metav1.ConditionTrue},
			{Type: "A", Status: metav1.ConditionTrue},
		},
	}
	manager := conditions.NewManager(
		accessor,
		mustNewAggregator(
			api.ConditionType("Z"),
			conditions.Dependent(api.ConditionType("A"), conditions.HealthyWhenTrue),
			conditions.Dependent(api.ConditionType("C"), conditions.HealthyWhenTrue),
		),
	)

	manager.Sort()

	result := make([]string, 0, len(accessor.conditions))
	for _, condition := range accessor.conditions {
		result = append(result, condition.Type)
	}

	g.Expect(result).To(Equal([]string{
		"Z",
		"A",
		"C",
		"B",
	}))
}

func TestNewAggregatorRejectsConflictingDuplicateDependents(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	aggregator, err := conditions.NewAggregator(
		readyCondition,
		conditions.Dependent(dependency1Condition, conditions.HealthyWhenTrue),
		conditions.Dependent(dependency1Condition, conditions.HealthyWhenFalse),
	)

	g.Expect(aggregator).To(BeNil())
	g.Expect(err).To(HaveOccurred())
}

func TestNewAggregatorRejectsEmptyDependentType(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	aggregator, err := conditions.NewAggregator(
		readyCondition,
		conditions.Dependent("", conditions.HealthyWhenTrue),
	)

	g.Expect(aggregator).To(BeNil())
	g.Expect(err).To(HaveOccurred())
}

func TestNewAggregatorRejectsTargetAsDependent(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	aggregator, err := conditions.NewAggregator(
		readyCondition,
		conditions.Dependent(readyCondition, conditions.HealthyWhenTrue),
	)

	g.Expect(aggregator).To(BeNil())
	g.Expect(err).To(HaveOccurred())
}

func TestNewAggregatorIgnoresDuplicateDependentsWithSamePolarity(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	aggregator, err := conditions.NewAggregator(
		readyCondition,
		conditions.Dependent(dependency1Condition, conditions.HealthyWhenTrue),
		conditions.Dependent(dependency1Condition, conditions.HealthyWhenTrue),
	)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(aggregator.Dependents()).To(HaveLen(1))
	g.Expect(aggregator.Dependents()[0]).To(Equal(conditions.Dependent(dependency1Condition, conditions.HealthyWhenTrue)))
}

func TestAggregator_AggregateUsesStableDependentThenLexicalOrder(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	aggregator := mustNewAggregator(
		readyCondition,
		conditions.Dependent(api.ConditionType("CondB"), conditions.HealthyWhenTrue),
		conditions.Dependent(api.ConditionType("CondA"), conditions.HealthyWhenTrue),
	)
	accessor := &fakeAccessor{
		conditions: []api.Condition{
			{Type: "OtherCondition", Status: metav1.ConditionFalse, Reason: "Other"},
			{Type: "CondB", Status: metav1.ConditionFalse, Reason: "B"},
			{Type: "CondA", Status: metav1.ConditionFalse, Reason: "A"},
		},
	}

	condition := aggregator.Aggregate(accessor)
	g.Expect(condition).NotTo(BeNil())
	g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(condition.Reason).To(Equal("A"))
}

func mustNewAggregator(
	target api.ConditionType,
	dependents ...conditions.DependentDefinition,
) *conditions.Aggregator {
	aggregator, err := conditions.NewAggregator(target, dependents...)
	if err != nil {
		panic(err)
	}

	return aggregator
}
