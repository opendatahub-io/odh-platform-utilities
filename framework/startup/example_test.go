package startup_test

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-platform-utilities/framework/startup"
)

func ExampleWaitForDatabaseClaimProvisioned() {
	reader := claimReader{kind: "DatabaseClaim", get: func(obj *unstructured.Unstructured) error {
		setProvisioned(obj, "True")
		return nil
	}}

	err := startup.WaitForDatabaseClaimProvisioned(context.Background(), reader, "consumer", "app-claim", time.Minute)
	fmt.Println(err)
	// Output: <nil>
}

func ExampleWaitForSchemaClaimProvisioned() {
	reader := claimReader{kind: "SchemaClaim", get: func(obj *unstructured.Unstructured) error {
		setProvisioned(obj, "True")
		return nil
	}}

	err := startup.WaitForSchemaClaimProvisioned(context.Background(), reader, "consumer", "app-claim", time.Minute)
	fmt.Println(err)
	// Output: <nil>
}
