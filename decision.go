package backpressure

import "time"

// Decision describes the result of an Acquire or Allow call.
type Decision struct {
	Allowed     bool
	Reason      Reason
	PassPercent float64
	DropRatio   float64
	RetryAfter  time.Duration
	WouldReject bool
}

// Reason explains why a decision was made.
type Reason string

const (
	ReasonAllowed       Reason = "allowed"
	ReasonDisabled      Reason = "disabled"
	ReasonMinSamples    Reason = "min_samples"
	ReasonMaxPass       Reason = "max_pass"
	ReasonPressure      Reason = "pressure"
	ReasonAdaptiveDrop  Reason = "adaptive_drop"
	ReasonInvalidConfig Reason = "invalid_config"
)
