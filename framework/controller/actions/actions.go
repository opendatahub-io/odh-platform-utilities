package actions

import (
	"context"
	"reflect"
	"runtime"

	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
)

const (
	ActionGroup = "action"
)

// Fn runs one apply or finalizer action. During apply, a plain error stops the
// pipeline, marks provisioning as failed, and uses normal controller-runtime
// backoff. During finalization, a plain error stops finalization, retains the
// finalizer, and uses normal controller-runtime backoff. Use
// ActionError.NonBlocking() to continue after a non-blocking error, or
// ActionError.Advisory() for a tolerated outcome. Deprecated StopError and
// RequeueAfterError remain compatibility adapters.
type Fn func(ctx context.Context, rr *types.ReconciliationRequest) error

type Getter[T any] func(context.Context, *types.ReconciliationRequest) (T, error)

func (f Fn) String() string {
	fn := runtime.FuncForPC(reflect.ValueOf(f).Pointer())
	return fn.Name()
}
