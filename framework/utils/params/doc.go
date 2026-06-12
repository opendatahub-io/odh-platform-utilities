// Package params provides utilities for reading and updating params.env files
// used by Kubernetes component deployments.
//
// Usage:
//
//	// Replace existing keys from environment variables, and merge extra values:
//	params.Apply(manifestPath, "params.env",
//	    params.Replacement(params.FromEnv(map[string]string{
//	        "controller_image": "RELATED_IMAGE_CONTROLLER",
//	    })),
//	    params.Values(map[string]string{
//	        "namespace": "my-namespace",
//	    }),
//	)
//
//	// Replace existing keys from a static map (no env var lookup):
//	params.Apply(path, "params.env",
//	    params.Replacement(map[string]string{
//	        "controller_image": "quay.io/org/controller:v2",
//	    }),
//	)
//
//	// Only add/update values freely:
//	params.Apply(path, "params.env",
//	    params.Values(map[string]string{
//	        "namespace": "custom-ns",
//	        "new_param": "new_value",
//	    }),
//	)
package params
