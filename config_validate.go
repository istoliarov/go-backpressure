package backpressure

import (
	"errors"
	"strings"
)

// ConfigError describes non-fatal configuration normalization issues.
type ConfigError struct {
	Issues []string
}

func (e ConfigError) Error() string {
	return "backpressure config: " + strings.Join(e.Issues, "; ")
}

func normalizeConfig(cfg Config) (Config, error, bool) {
	return normalizeConfigWithStrategy(cfg, isBuiltInStrategy)
}

func normalizeConfigWithStrategy(cfg Config, isKnownStrategy func(StrategyType) bool) (Config, error, bool) {
	def := DefaultConfig()
	var issues []string

	if cfg.Strategy == "" {
		cfg.Strategy = def.Strategy
	}
	if !isKnownStrategy(cfg.Strategy) {
		return failOpenConfig(cfg), ConfigError{Issues: []string{"unknown strategy " + string(cfg.Strategy)}}, true
	}

	if cfg.Window <= 0 {
		cfg.Window = def.Window
		issues = append(issues, "Window <= 0, using default")
	}
	if cfg.BucketCount <= 0 {
		cfg.BucketCount = def.BucketCount
		issues = append(issues, "BucketCount <= 0, using default")
	}
	if cfg.MinSamples < 0 {
		cfg.MinSamples = def.MinSamples
		issues = append(issues, "MinSamples < 0, using default")
	}
	if cfg.SREK == 0 {
		cfg.SREK = def.SREK
	}
	if cfg.SREK < 1 {
		cfg.SREK = def.SREK
		issues = append(issues, "SREK < 1, using default")
	}

	cfg.MinPassPercent = clampFloat(cfg.MinPassPercent, 0, 100)
	cfg.MaxPassPercent = clampFloat(cfg.MaxPassPercent, 0, 100)
	if cfg.MinPassPercent > cfg.MaxPassPercent {
		cfg.MinPassPercent = cfg.MaxPassPercent
		issues = append(issues, "MinPassPercent > MaxPassPercent, clamped to MaxPassPercent")
	}

	if cfg.PressureLimit <= 0 && cfg.Strategy == StrategyPressure {
		cfg.PressureLimit = def.PressureLimit
		issues = append(issues, "PressureLimit <= 0 for pressure strategy, using default")
	}
	if cfg.NeutralPolicy == "" {
		cfg.NeutralPolicy = def.NeutralPolicy
	}
	switch cfg.NeutralPolicy {
	case NeutralDoesNotAffect, NeutralCountsSuccess, NeutralCountsFailure:
	default:
		cfg.NeutralPolicy = def.NeutralPolicy
		issues = append(issues, "unknown NeutralPolicy, using default")
	}
	if len(issues) == 0 {
		return cfg, nil, false
	}
	return cfg, ConfigError{Issues: issues}, false
}

func isBuiltInStrategy(strategy StrategyType) bool {
	switch strategy {
	case StrategySREAdaptive, StrategyPressure:
		return true
	default:
		return false
	}
}

func failOpenConfig(cfg Config) Config {
	cfg.Enabled = false
	return cfg
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func isConfigError(err error) bool {
	var cfgErr ConfigError
	return errors.As(err, &cfgErr)
}
