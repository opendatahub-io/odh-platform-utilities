package flakiness

import (
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

// SampleAppender is a minimal subset of storage.Appender exposing only the
// Append method. Any storage.Appender satisfies this interface.
type SampleAppender interface {
	Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error)
}
