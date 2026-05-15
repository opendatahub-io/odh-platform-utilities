// Package testutil provides a test utility that module teams can call in
// their unit tests to verify their CRD implementation satisfies the
// PlatformObject contract. This catches integration issues at development
// time rather than during orchestrator integration.
//
// Usage is opt-in. Teams that prefer manual validation are free to skip it.
//
// # Example
//
//	func TestMyComponent_PlatformObject(t *testing.T) {
//	    obj := &v1alpha1.MyComponent{
//	        ObjectMeta: metav1.ObjectMeta{Name: "default"},
//	    }
//	    testutil.ValidatePlatformObject(t, obj)
//	}
package testutil
