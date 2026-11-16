package matchers

import (
	api "github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/types"
)

func ExtractStatusCondition(conditionType string) func(in types.ResourceObject) api.Condition {
	return func(in types.ResourceObject) api.Condition {
		c := conditions.FindStatusCondition(in.GetStatus(), conditionType)
		if c == nil {
			return api.Condition{}
		}

		return *c
	}
}
