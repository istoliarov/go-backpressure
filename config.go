package backpressure

import (
	"time"
)

type StrategyType string

const (
	StrategySREAdaptive StrategyType = "sre_adaptive"
	StrategyPressure    StrategyType = "pressure"
)

type NeutralPolicy string

const (
	NeutralDoesNotAffect NeutralPolicy = "does_not_affect"
	NeutralCountsSuccess NeutralPolicy = "counts_success"
	NeutralCountsFailure NeutralPolicy = "counts_failure"
)

// Config controls a controller. Start from DefaultConfig for normal use.
//
// The zero value is fail-open disabled after normalization. This avoids
// accidentally enabling traffic shedding from a partially initialized config.
// Explicit zero values are preserved for fields where zero is meaningful, such
// as MinSamples, MinPassPercent, MaxPassPercent, and pressure weights.
type Config struct {
	Enabled    bool
	ShadowMode bool

	Strategy StrategyType

	MinPassPercent float64
	MaxPassPercent float64

	Window      time.Duration
	BucketCount int
	MinSamples  int64

	SREK float64

	PressureLimit    float64
	ErrorIncrease    float64
	OverloadIncrease float64
	SuccessDecrease  float64
	DecayPerSecond   float64

	NeutralPolicy NeutralPolicy
}

// ConfigProvider supplies runtime configuration snapshots.
type ConfigProvider interface {
	Snapshot() Config
}

type StaticConfig struct {
	cfg atomicConfig
}

// NewStaticConfig creates a concurrency-safe in-memory ConfigProvider.
func NewStaticConfig(cfg Config) *StaticConfig {
	provider := &StaticConfig{}
	provider.Store(cfg)
	return provider
}

// Snapshot returns the latest stored configuration.
func (c *StaticConfig) Snapshot() Config {
	return c.cfg.Load()
}

// Store replaces the current configuration.
func (c *StaticConfig) Store(cfg Config) {
	c.cfg.Store(cfg)
}
