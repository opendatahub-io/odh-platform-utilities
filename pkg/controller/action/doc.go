// Package action defines the core types for the optional action pipeline
// pattern used in ODH module controllers.
//
// The pipeline pattern composes reconciliation steps as a sequence of [Fn]
// functions that share state through a [ReconciliationRequest]. This enables
// consistent error handling, condition management, and resource flow between
// render, deploy, and garbage collection steps.
//
// # Pipeline Usage
//
// Build a pipeline as a slice of [Fn] and execute them sequentially.
// Each step is an [Fn] closure that calls standalone render, deploy, or GC
// functions internally. The render package has its own [render.Fn] type
// operating on [render.ReconciliationRequest]; wrap render calls in an
// [Fn] to bridge the two:
//
//	rr := &action.ReconciliationRequest{
//	    Client:     cli,
//	    Instance:   myCR,
//	    Deployer:   deployer,
//	    Conditions: conditions.NewManager(myCR,
//	        string(common.ConditionTypeReady),
//	        string(common.ConditionTypeProvisioningSucceeded),
//	    ),
//	}
//
//	renderStep := action.Fn(func(ctx context.Context, rr *action.ReconciliationRequest) error {
//	    resources, err := kustomize.Render(manifestPath, engineOpts)
//	    if err != nil {
//	        return err
//	    }
//	    rr.Resources = append(rr.Resources, resources...)
//	    return nil
//	})
//
//	deployStep := action.Fn(func(ctx context.Context, rr *action.ReconciliationRequest) error {
//	    return rr.Deployer.Deploy(ctx, deploy.DeployInput{
//	        Client:    rr.Client,
//	        Owner:     rr.Instance,
//	        Resources: rr.Resources,
//	    })
//	})
//
//	pipeline := []action.Fn{renderStep, deployStep}
//
//	for _, step := range pipeline {
//	    if err := step(ctx, rr); err != nil {
//	        return err
//	    }
//	}
//
// # Standalone Usage
//
// Teams that prefer not to use the pipeline can call render, deploy, and
// GC functions directly without constructing a [ReconciliationRequest]:
//
//	resources, err := kustomize.Render(path, engineOpts)
//
//	err = deployer.Deploy(ctx, deploy.DeployInput{
//	    Client:    cli,
//	    Owner:     myCR,
//	    Resources: resources,
//	})
//
// # Design Note
//
// [ReconciliationRequest] is intentionally minimal — it carries only the
// fields shared across pipeline steps (Client, Instance, Deployer,
// Conditions, Resources). Render-specific fields (Manifests, Templates,
// HelmCharts) live in [render.ReconciliationRequest], keeping the action
// pipeline decoupled from rendering concerns. The two request types are
// not interchangeable; use closures to bridge them as shown above.
//
// These types do not import the ODH operator and have no dependencies
// beyond the platform contract ([common.PlatformObject]), conditions
// manager ([conditions.Manager]), and standard Kubernetes libraries.
package action
