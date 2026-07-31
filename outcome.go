package backpressure

import "time"

// OutcomeClass classifies an operation result for backpressure purposes.
type OutcomeClass int

const (
	OutcomeSuccess OutcomeClass = iota
	OutcomeFailure
	OutcomeNeutral
	OutcomeOverload
)

func (c OutcomeClass) String() string {
	switch c {
	case OutcomeSuccess:
		return "success"
	case OutcomeFailure:
		return "failure"
	case OutcomeNeutral:
		return "neutral"
	case OutcomeOverload:
		return "overload"
	default:
		return "unknown"
	}
}

// Outcome is the caller-classified result of an attempted operation.
type Outcome struct {
	Class   OutcomeClass
	Err     error
	Latency time.Duration
	Weight  float64
}

// Success reports that the downstream handled the operation well.
func Success() Outcome {
	return Outcome{Class: OutcomeSuccess, Weight: 1}
}

// Failure reports infrastructure-like failure such as timeout or unavailable.
func Failure(err error) Outcome {
	return Outcome{Class: OutcomeFailure, Err: err, Weight: 1}
}

// Neutral reports a valid result that should not worsen backpressure state.
func Neutral() Outcome {
	return Outcome{Class: OutcomeNeutral, Weight: 1}
}

// Overload reports an explicit overload signal such as 429, 503, or resource
// exhausted.
func Overload(err error) Outcome {
	return Outcome{Class: OutcomeOverload, Err: err, Weight: 1}
}

// WithLatency attaches downstream latency to an outcome.
func (o Outcome) WithLatency(latency time.Duration) Outcome {
	o.Latency = latency
	return o
}

// WithWeight adjusts the outcome impact for strategies that use weights.
func (o Outcome) WithWeight(weight float64) Outcome {
	o.Weight = weight
	return o
}

func (o Outcome) normalized() Outcome {
	if o.Weight <= 0 {
		o.Weight = 1
	}
	return o
}
