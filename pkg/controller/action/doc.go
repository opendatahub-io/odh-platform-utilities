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
// Build a pipeline as a slice of [Fn] and execute them sequentially:
//
//	rr := &action.ReconciliationRequest{
//	    Client:     cli,
//	    Instance:   myCR,
//	    Conditions: conditions.NewManager(myCR,
//	        string(common.ConditionTypeReady),
//	        string(common.ConditionTypeProvisioningSucceeded),
//	    ),
//	}
//
//	pipeline := []action.Fn{renderAction, deployAction, gcAction}
//
//	for _, step := range pipeline {
//	    if err := step(ctx, rr); err != nil {
//	        return err
//	    }
//	}
//
// # Standalone Usage
//
// Teams that prefer not to use the pipeline can instantiate
// [ReconciliationRequest] directly and pass fields to standalone functions:
//
//	rr := &action.ReconciliationRequest{
//	    Client:   cli,
//	    Instance: myCR,
//	}
//
//	resources, err := kustomize.Render(path, engineOpts)
//	rr.Resources = resources
//
//	err = deployer.Deploy(ctx, deploy.DeployInput{
//	    Client:    rr.Client,
//	    Owner:     rr.Instance,
//	    Resources: rr.Resources,
//	})
//
// The types are intentionally minimal. They do not import the ODH operator
// and have no dependencies beyond the platform contract ([common.PlatformObject]),
// conditions manager ([conditions.Manager]), and standard Kubernetes libraries.
package action
