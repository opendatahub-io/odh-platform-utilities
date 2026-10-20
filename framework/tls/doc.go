// Package tls resolves OpenShift TLS security profiles into values module
// controllers can apply to their own servers and operands.
//
// # Why this package imports OpenShift APIs
//
// Named profiles (Old, Intermediate, Modern) and their cipher lists live in
// configv1.TLSProfiles and are allowed to change as Mozilla/OpenShift
// guidelines evolve. This framework package keeps resolution aligned with
// apiservers.config.openshift.io/cluster. Other framework packages must not
// import OpenShift TLS profile APIs or library-go crypto helpers.
//
// Generic TLS profile parsing, TLSConfig mapping, and APIServer watching reuse
// controller-runtime-common. This package adds adherence-aware loading and the
// stricter validation needed by ODH's TLS policy.
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
//	result, err := tls.LoadWithAdherence(ctx, bootstrapClient)
//	if err != nil {
//	    return fmt.Errorf("load TLS profile: %w", err)
//	}
//	profile := result.Spec
//	if !tls.ShouldHonorClusterTLSProfile(result.AdherencePolicy) {
//	    profile = *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
//	}
//	tlsOpts, unsupported, err := tls.ConfigFromProfileStrict(profile)
//	if err != nil {
//	    return fmt.Errorf("validate TLS profile: %w", err)
//	}
//	if len(unsupported) > 0 {
//	    setupLog.Info("dropping cipher names unsupported by Go", "ciphers", unsupported)
//	}
//	mgrCtx := ctx
//	var cancel context.CancelFunc
//	if result.Watchable {
//	    mgrCtx, cancel = context.WithCancel(ctx)
//	    defer cancel()
//	}
//	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
//	    Scheme:  scheme,
//	    Metrics: metricsserver.Options{TLSOpts: []func(*cryptotls.Config){tlsOpts}},
//	    WebhookServer: webhook.NewServer(webhook.Options{TLSOpts: []func(*cryptotls.Config){tlsOpts}}),
//	})
//	if err != nil {
//	    return fmt.Errorf("create manager: %w", err)
//	}
//	if result.Watchable {
//	    watcher := &tls.SecurityProfileWatcher{
//	        Client:                    mgr.GetClient(),
//	        InitialTLSProfileSpec:     result.Spec,
//	        InitialTLSAdherencePolicy: result.AdherencePolicy,
//	        OnProfileChange: func(context.Context, configv1.TLSProfileSpec, configv1.TLSProfileSpec) {
//	            cancel()
//	        },
//	        OnAdherencePolicyChange: func(context.Context, configv1.TLSAdherencePolicy, configv1.TLSAdherencePolicy) {
//	            cancel()
//	        },
//	    }
//	    if err := watcher.SetupWithManager(mgr); err != nil {
//	        return fmt.Errorf("register TLS profile watcher: %w", err)
//	    }
//	}
//	if err := mgr.Start(mgrCtx); err != nil {
//	    return fmt.Errorf("start manager: %w", err)
//	}
//
// Required RBAC: get on apiservers.config.openshift.io. list and watch are
// required only when registering SecurityProfileWatcher.
//
// On vanilla Kubernetes, LoadWithAdherence falls back to the Intermediate
// profile with NoOpinion adherence and Watchable false. An absent profile is
// valid and resolves to Intermediate. Under NoOpinion or Legacy adherence,
// unknown profile types use the documented Intermediate compatibility fallback;
// Strict adherence rejects them. ConfigFromProfileStrict should be used by
// callers that convert profile specs directly and need fail-closed validation.
// The legacy ConfigFromProfile helper remains for compatibility.
//
// See ../../docs/module-tls.md for the expected module wiring.
package tls
