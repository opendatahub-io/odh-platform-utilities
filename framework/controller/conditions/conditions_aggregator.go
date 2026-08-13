package conditions

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ConditionPolarity string

const (
	HealthyWhenTrue  ConditionPolarity = "HealthyWhenTrue"
	HealthyWhenFalse ConditionPolarity = "HealthyWhenFalse"
)

type DependentDefinition struct {
	Type     api.ConditionType
	Polarity ConditionPolarity
}

func Dependent(
	name api.ConditionType,
	polarity ConditionPolarity,
) DependentDefinition {
	return DependentDefinition{
		Type:     name,
		Polarity: polarity,
	}
}

type Aggregator struct {
	target     api.ConditionType
	dependents map[api.ConditionType]ConditionPolarity
}

func NewAggregator(
	target api.ConditionType,
	dependentDefinitions ...DependentDefinition,
) (*Aggregator, error) {
	dependents := make(map[api.ConditionType]ConditionPolarity)

	for _, dependent := range dependentDefinitions {
		if dependent.Type == "" {
			return nil, errors.New("dependent type must not be empty")
		}

		if dependent.Type == target {
			return nil, fmt.Errorf("dependent %q must not match target", dependent.Type)
		}

		if existing, exists := dependents[dependent.Type]; exists {
			if existing != dependent.Polarity {
				return nil, fmt.Errorf(
					"conflicting polarity for dependent %q: %q vs %q",
					dependent.Type,
					existing,
					dependent.Polarity,
				)
			}

			continue
		}

		dependents[dependent.Type] = dependent.Polarity
	}

	return &Aggregator{
		target:     target,
		dependents: dependents,
	}, nil
}

func (a *Aggregator) Target() api.ConditionType {
	if a == nil {
		return ""
	}

	return a.target
}

func (a *Aggregator) Dependents() []DependentDefinition {
	if a == nil {
		return nil
	}

	definitions := make([]DependentDefinition, 0, len(a.dependents))
	for conditionType, polarity := range a.dependents {
		definitions = append(definitions, DependentDefinition{
			Type:     conditionType,
			Polarity: polarity,
		})
	}

	slices.SortFunc(definitions, func(a DependentDefinition, b DependentDefinition) int {
		return cmp.Compare(string(a.Type), string(b.Type))
	})

	return definitions
}

func (a *Aggregator) Aggregate(accessor api.ConditionsAccessor) *api.Condition {
	if a == nil || accessor == nil {
		return nil
	}

	selected := a.firstUnhealthyCondition(slices.Clone(accessor.GetConditions()))
	if selected == nil {
		return &api.Condition{
			Type:   string(a.target),
			Status: metav1.ConditionTrue,
		}
	}

	status := metav1.ConditionFalse
	if selected.Status == metav1.ConditionUnknown {
		status = metav1.ConditionUnknown
	}

	return &api.Condition{
		Type:    string(a.target),
		Status:  status,
		Reason:  selected.Reason,
		Message: selected.Message,
	}
}

func (a *Aggregator) firstUnhealthyCondition(conditions []api.Condition) *api.Condition {
	slices.SortStableFunc(conditions, a.compareConditions)

	var explicitUnhealthy *api.Condition
	var unknownUnhealthy *api.Condition

	for _, condition := range conditions {
		if !a.shouldConsider(condition) || a.isHealthy(condition) {
			continue
		}

		if condition.Status == metav1.ConditionUnknown {
			if unknownUnhealthy == nil {
				conditionCopy := condition
				unknownUnhealthy = &conditionCopy
			}
			continue
		}

		if explicitUnhealthy == nil {
			conditionCopy := condition
			explicitUnhealthy = &conditionCopy
		}
	}

	if explicitUnhealthy != nil {
		return explicitUnhealthy
	}

	return unknownUnhealthy
}

func (a *Aggregator) hasDependent(conditionType api.ConditionType) bool {
	if a == nil {
		return false
	}

	_, found := a.dependents[conditionType]
	return found
}

func (a *Aggregator) polarityFor(conditionType api.ConditionType) ConditionPolarity {
	if a == nil {
		return HealthyWhenTrue
	}

	if polarity, ok := a.dependents[conditionType]; ok {
		return polarity
	}

	return HealthyWhenTrue
}

func (a *Aggregator) shouldConsider(condition api.Condition) bool {
	conditionType := api.ConditionType(condition.Type)

	switch {
	case conditionType == a.target:
		return false
	case condition.Severity == api.ConditionSeverityInfo:
		return false
	case len(a.dependents) == 0:
		return true
	default:
		return a.hasDependent(conditionType)
	}
}

func (a *Aggregator) isHealthy(condition api.Condition) bool {
	if condition.Status == metav1.ConditionUnknown {
		return false
	}

	switch a.polarityFor(api.ConditionType(condition.Type)) {
	case HealthyWhenFalse:
		return condition.Status == metav1.ConditionFalse
	default:
		return condition.Status == metav1.ConditionTrue
	}
}

// compareConditions keeps aggregation deterministic by ordering the target
// first, registered dependents next, and all remaining conditions
// lexicographically by type.
func (a *Aggregator) compareConditions(left api.Condition, right api.Condition) int {
	priority := func(conditionType string) int {
		switch {
		case conditionType == string(a.Target()):
			return 0
		case a.hasDependent(api.ConditionType(conditionType)):
			return 1
		default:
			return 2
		}
	}

	result := cmp.Compare(priority(left.Type), priority(right.Type))
	if result != 0 {
		return result
	}

	return strings.Compare(left.Type, right.Type)
}
