package conditions

import (
	"fmt"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
)

type Option func(*api.Condition)

func WithReason(value string) Option {
	return func(c *api.Condition) {
		c.Reason = value
	}
}

func WithMessage(msg string) Option {
	return func(c *api.Condition) {
		c.Message = msg
	}
}

func WithMessagef(format string, args ...any) Option {
	return func(c *api.Condition) {
		c.Message = fmt.Sprintf(format, args...)
	}
}

func WithObservedGeneration(value int64) Option {
	return func(c *api.Condition) {
		c.ObservedGeneration = value
	}
}

func WithSeverity(value api.ConditionSeverity) Option {
	return func(c *api.Condition) {
		c.Severity = value
	}
}

func WithError(err error) Option {
	return func(c *api.Condition) {
		c.Severity = api.ConditionSeverityError
		c.Reason = api.ConditionReasonError
		if err != nil {
			c.Message = err.Error()
		}
	}
}
