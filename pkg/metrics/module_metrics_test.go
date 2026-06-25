package metrics_test

import (
	"errors"
	"testing"
	"time"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/metrics"
)

var errReconcileFailed = errors.New("reconcile failed")

func TestReconcileTimer_Success(t *testing.T) {
	t.Parallel()

	var err error

	stop := metrics.ReconcileTimer("test-module", &err)

	time.Sleep(1 * time.Millisecond)
	stop()
}

func TestReconcileTimer_Error(t *testing.T) {
	t.Parallel()

	err := errReconcileFailed

	stop := metrics.ReconcileTimer("test-module", &err)
	stop()
}
