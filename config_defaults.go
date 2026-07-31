package backpressure

import "time"

// DefaultConfig returns conservative production defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:          true,
		Strategy:         StrategySREAdaptive,
		MinPassPercent:   5,
		MaxPassPercent:   100,
		Window:           2 * time.Minute,
		BucketCount:      20,
		MinSamples:       100,
		SREK:             1.5,
		PressureLimit:    10_000,
		ErrorIncrease:    100,
		OverloadIncrease: 300,
		SuccessDecrease:  1,
		DecayPerSecond:   10,
		NeutralPolicy:    NeutralDoesNotAffect,
	}
}
