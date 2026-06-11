package imagestreams

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	imagev1 "github.com/openshift/api/image/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
)

const (
	DefaultConditionType      = "ImageStreamsAvailable"
	DefaultNotAvailableReason = "ImageStreamsNotReady"
	DefaultPartOfLabelKey     = "platform.opendatahub.io/part-of"

	// maxConditionMessageLen caps the per-tag error message length to avoid
	// leaking verbose registry errors (CWE-209).
	maxConditionMessageLen = 100

	// maxFailedTags caps the number of failed tags reported in the condition
	// message to keep it readable in oc get / dashboard views.
	maxFailedTags = 10
)

type Action struct {
	partOfLabelKey                string
	labels                        map[string]string
	namespaceFn                   actions.Getter[string]
	conditionType                 string
	notAvailableReason            string
	disableAutomaticPartOfDefault bool
}

type ActionOpts func(*Action)

func WithSelectorLabel(k string, v string) ActionOpts {
	return func(action *Action) {
		action.labels[k] = v
	}
}

func WithSelectorLabels(values map[string]string) ActionOpts {
	return func(action *Action) {
		maps.Copy(action.labels, values)
	}
}

func WithPartOfLabel(key string) ActionOpts {
	return func(action *Action) {
		action.partOfLabelKey = key
	}
}

func WithConditionType(conditionType string) ActionOpts {
	return func(action *Action) {
		action.conditionType = conditionType
	}
}

func WithNotAvailableReason(reason string) ActionOpts {
	return func(action *Action) {
		action.notAvailableReason = reason
	}
}

func WithoutAutomaticPartOfDefault() ActionOpts {
	return func(action *Action) {
		action.disableAutomaticPartOfDefault = true
	}
}

func InNamespace(ns string) ActionOpts {
	return func(action *Action) {
		action.namespaceFn = func(_ context.Context, _ *types.ReconciliationRequest) (string, error) {
			return ns, nil
		}
	}
}

func InNamespaceFn(fn actions.Getter[string]) ActionOpts {
	return func(action *Action) {
		if fn == nil {
			return
		}
		action.namespaceFn = fn
	}
}

func (a *Action) run(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(types.ResourceObject)
	if !ok {
		return fmt.Errorf("resource instance %v is not a ResourceObject", rr.Instance)
	}

	l := make(map[string]string, len(a.labels))
	maps.Copy(l, a.labels)

	if !a.disableAutomaticPartOfDefault && l[a.partOfLabelKey] == "" {
		kind, err := resources.KindForObject(rr.Client.Scheme(), rr.Instance)
		if err != nil {
			return err
		}

		l[a.partOfLabelKey] = strings.ToLower(kind)
	}

	if a.namespaceFn == nil {
		return errors.New("namespace function is not configured for imagestream status action")
	}

	ns, err := a.namespaceFn(ctx, rr)
	if err != nil {
		return fmt.Errorf("unable to compute namespace: %w", err)
	}

	imageStreams := &imagev1.ImageStreamList{}

	err = rr.Client.List(
		ctx,
		imageStreams,
		client.InNamespace(ns),
		client.MatchingLabels(l),
	)

	if meta.IsNoMatchError(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("error fetching list of ImageStreams: %w", err)
	}

	s := obj.GetStatus()

	rr.Conditions.MarkTrue(a.conditionType, conditions.WithObservedGeneration(s.ObservedGeneration))

	if len(imageStreams.Items) == 0 {
		return nil
	}

	var failedTags []string

	for i := range imageStreams.Items {
		is := &imageStreams.Items[i]
		for _, tagStatus := range is.Status.Tags {
			if len(tagStatus.Items) > 0 {
				continue
			}
			for _, cond := range tagStatus.Conditions {
				if cond.Type == imagev1.ImportSuccess && cond.Status == corev1.ConditionFalse {
					msg := cond.Message
					if len(msg) > maxConditionMessageLen {
						msg = msg[:maxConditionMessageLen] + "..."
					}
					failedTags = append(failedTags, fmt.Sprintf("%s:%s (%s)", is.Name, tagStatus.Tag, msg))
				}
			}
		}
	}

	if len(failedTags) > 0 {
		reported := failedTags
		suffix := ""
		if len(reported) > maxFailedTags {
			suffix = fmt.Sprintf("; ... and %d more", len(reported)-maxFailedTags)
			reported = reported[:maxFailedTags]
		}

		rr.Conditions.MarkFalse(
			a.conditionType,
			conditions.WithObservedGeneration(s.ObservedGeneration),
			conditions.WithReason(a.notAvailableReason),
			conditions.WithMessage("Warning: %d ImageStream tag(s) failed to import: %s%s", len(failedTags), strings.Join(reported, "; "), suffix),
		)
	}

	return nil
}

func NewAction(opts ...ActionOpts) actions.Fn {
	action := Action{
		partOfLabelKey:     DefaultPartOfLabelKey,
		labels:             map[string]string{},
		conditionType:      DefaultConditionType,
		notAvailableReason: DefaultNotAvailableReason,
	}

	for _, opt := range opts {
		opt(&action)
	}

	return action.run
}
