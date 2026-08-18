// Package tls resolves OpenShift TLS security profiles into values module
// controllers can apply to their own servers and operands.
//
// # Why this package imports OpenShift APIs
//
// Named profiles (Old, Intermediate, Modern) and their cipher lists live in
// configv1.TLSProfiles and are allowed to change as Mozilla/OpenShift
// guidelines evolve. This package is a scoped exception to the root-module
// rule that forbids github.com/openshift/api: using those types keeps
// resolution aligned with apiservers.config.openshift.io/cluster. Other root
// packages must not import OpenShift APIs.
//
// # Typical manager wiring
//
// Callers must register the OpenShift config API in the scheme so typed
// Get/watch of APIServer works:
//
//	configv1.Install(scheme)
//
// Then at startup:
//
//	result, err := tls.Load(ctx, bootstrapClient)
//	if err != nil {
//	    // unexpected error: refuse to start
//	}
//	tlsOpts, _ := tls.ConfigFromProfile(result.Spec)
//	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
//	    Scheme:  scheme,
//	    Metrics: metricsserver.Options{TLSOpts: []func(*cryptotls.Config){tlsOpts}},
//	    WebhookServer: webhook.NewServer(webhook.Options{TLSOpts: []func(*cryptotls.Config){tlsOpts}}),
//	})
//	if result.Watchable {
//	    mgrCtx, cancel := context.WithCancel(ctx)
//	    watcher := &tls.SecurityProfileWatcher{
//	        Client:                mgr.GetClient(),
//	        InitialTLSProfileSpec: result.Spec,
//	        OnProfileChange: func(context.Context, configv1.TLSProfileSpec, configv1.TLSProfileSpec) {
//	            cancel()
//	        },
//	    }
//	    _ = watcher.SetupWithManager(mgr)
//	    _ = mgr.Start(mgrCtx)
//	}
//
// Required RBAC: get, list, and watch on apiservers.config.openshift.io.
//
// On vanilla Kubernetes, Load falls back to the Intermediate profile and
// Watchable is false. FromAPIServer does the same for proxy flag strings.
package tls
