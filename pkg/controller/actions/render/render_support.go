package render

import (
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/types"
)

type CachingKeyFn func(rr *types.ReconciliationRequest) ([]byte, error)
