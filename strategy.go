package backpressure

import (
	"time"
)

// Strategy is the algorithm contract implemented by built-in strategies.
//
// Custom strategies are supported for advanced consumers, but this interface is
// experimental until v1 and may change based on real integrations.
type Strategy interface {
	Allow(now time.Time, cfg Config, attrs []Attr) Decision
	Report(now time.Time, cfg Config, outcome Outcome, attrs []Attr)
	Snapshot(now time.Time, cfg Config) Snapshot
}
