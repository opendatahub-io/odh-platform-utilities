package tls

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
)

// SecurityProfileWatcher watches the cluster APIServer object for TLS profile
// changes. The usual OnProfileChange implementation cancels the manager
// context so the process restarts and picks up the new profile.
//
// Register the watcher only when [Load] reports Watchable. The scheme must
// include configv1.APIServer. Required RBAC: get, list, and watch on
// apiservers.config.openshift.io.
type SecurityProfileWatcher struct {
	client.Client

	// OnProfileChange is called when the cluster TLS profile changes.
	OnProfileChange func(ctx context.Context, oldTLSProfileSpec, newTLSProfileSpec configv1.TLSProfileSpec)

	// InitialTLSProfileSpec is the TLS profile spec applied when the process started.
	InitialTLSProfileSpec configv1.TLSProfileSpec
}

// SetupWithManager registers the watcher with the manager. Leader election is
// disabled so every replica observes profile changes.
func (r *SecurityProfileWatcher) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		Named("tlssecurityprofilewatcher").
		WithOptions(controller.Options{NeedLeaderElection: ptr.To(false)}).
		For(&configv1.APIServer{}, builder.WithPredicates(
			predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return e.Object.GetName() == cluster.ClusterAPIServerObj
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return e.ObjectNew.GetName() == cluster.ClusterAPIServerObj
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return e.Object.GetName() == cluster.ClusterAPIServerObj
				},
				GenericFunc: func(e event.GenericEvent) bool {
					return e.Object.GetName() == cluster.ClusterAPIServerObj
				},
			},
		)).
		WithLogConstructor(func(_ *reconcile.Request) logr.Logger {
			return mgr.GetLogger().WithValues(
				"controller", "tlssecurityprofilewatcher",
			)
		}).
		Complete(r)
	if err != nil {
		return fmt.Errorf("could not set up controller for TLS security profile watcher: %w", err)
	}

	return nil
}

// Reconcile compares the current APIServer TLS profile to InitialTLSProfileSpec
// and invokes OnProfileChange when they differ.
func (r *SecurityProfileWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx, "name", req.Name)

	logger.V(1).Info("Reconciling APIServer TLS profile")
	defer logger.V(1).Info("Finished reconciling APIServer TLS profile")

	apiServer := &configv1.APIServer{}

	err := r.Get(ctx, req.NamespacedName, apiServer)
	if err != nil {
		if k8serr.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to get APIServer %s: %w", req.String(), err)
	}

	currentTLSProfileSpec := *ProfileSpecFromSecurityProfile(apiServer.Spec.TLSSecurityProfile)

	if !reflect.DeepEqual(r.InitialTLSProfileSpec, currentTLSProfileSpec) {
		if r.OnProfileChange != nil {
			r.OnProfileChange(ctx, r.InitialTLSProfileSpec, currentTLSProfileSpec)
		}

		r.InitialTLSProfileSpec = currentTLSProfileSpec
	}

	return ctrl.Result{}, nil
}
