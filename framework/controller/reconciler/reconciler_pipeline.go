package reconciler

import (
	"context"
	"fmt"
	"time"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	odherrors "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *Reconciler) handleActionOutcome(
	ctx context.Context,
	res api.PlatformObject,
	result odherrors.ActionError,
	config actionOutcomeConfig,
) (ctrl.Result, error) {
	switch result.Type() {
	case odherrors.ActionErrorTerminal:
		return r.handleTerminalActionOutcome(res, result, config)
	case odherrors.ActionErrorNonBlocking:
		return r.handleNonBlockingActionOutcome(res, result, config)
	default:
		return handleSuccessfulActionOutcome(ctx, result, config)
	}
}

func (r *Reconciler) handleTerminalActionOutcome(
	res api.PlatformObject,
	result odherrors.ActionError,
	config actionOutcomeConfig,
) (ctrl.Result, error) {
	requeueAfter := result.RequeueAfter()
	if requeueAfter > 0 {
		r.Recorder.Eventf(
			res,
			nil,
			corev1.EventTypeNormal,
			config.eventReasonPaused,
			config.eventAction,
			fmt.Sprintf("requeue after %s: %s", requeueAfter, result.Error()),
		)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	r.Recorder.Eventf(
		res,
		nil,
		corev1.EventTypeWarning,
		config.eventReasonError,
		config.eventAction,
		result.Error(),
	)

	return ctrl.Result{}, fmt.Errorf("%s failed: %w", config.errorPrefix, result)
}

func (r *Reconciler) handleNonBlockingActionOutcome(
	res api.PlatformObject,
	result odherrors.ActionError,
	config actionOutcomeConfig,
) (ctrl.Result, error) {
	r.Recorder.Eventf(
		res,
		nil,
		corev1.EventTypeWarning,
		config.eventReasonError,
		config.eventAction,
		result.Error(),
	)

	requeueAfter := result.RequeueAfter()
	if requeueAfter > 0 {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	return ctrl.Result{}, fmt.Errorf("%s failed: %w", config.errorPrefix, result)
}

func handleSuccessfulActionOutcome(
	ctx context.Context,
	result odherrors.ActionError,
	config actionOutcomeConfig,
) (ctrl.Result, error) {
	requeueAfter := result.RequeueAfter()
	if requeueAfter == 0 {
		requeueAfter = config.defaultRequeueAfter
	}
	if requeueAfter > 0 {
		log.FromContext(ctx).V(1).Info("scheduling requeue", "after", requeueAfter.Truncate(time.Second))
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) runActions(
	ctx context.Context,
	rr *types.ReconciliationRequest,
	actionsToRun []actions.Fn,
) odherrors.ActionError {
	l := log.FromContext(ctx)

	var result odherrors.ActionError

	for _, action := range actionsToRun {
		actionName := action.String()
		l.Info("Executing action", "action", actionName)

		err := action(log.IntoContext(
			ctx,
			l.WithName(actions.ActionGroup).WithName(actionName),
		), rr)

		var stop bool
		result, stop = result.Add(actionName, err)
		if stop {
			l.Info("action stopped the pipeline", "action", actionName)
			break
		}
	}

	return result
}
