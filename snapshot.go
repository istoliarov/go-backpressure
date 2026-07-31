package backpressure

import "time"

// Snapshot is a point-in-time view of controller state.
type Snapshot struct {
	Name     string
	Enabled  bool
	Strategy StrategyType

	PassPercent float64
	DropRatio   float64
	Pressure    float64

	WindowRequests int64
	WindowAccepts  int64
	WindowFailures int64
	LocalRejects   int64
	// ShadowRejects counts requests that would have been rejected while
	// ShadowMode allowed them through.
	ShadowRejects int64

	Config    Config
	UpdatedAt time.Time
}
