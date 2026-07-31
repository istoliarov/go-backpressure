package backpressure

import (
	"sync"
	"time"
)

type pressureStrategy struct {
	mu           sync.Mutex
	pressure     float64
	requests     int64
	accepts      int64
	failures     int64
	localRejects int64
	lastDecay    time.Time
	sampler      Sampler
}

func newPressureStrategy(sampler Sampler) *pressureStrategy {
	return &pressureStrategy{sampler: sampler}
}

func (s *pressureStrategy) Allow(now time.Time, cfg Config, attrs []Attr) Decision {
	s.mu.Lock()
	requests := s.recordAttemptLocked(now, cfg)
	pressure := s.pressure
	s.mu.Unlock()

	passPercent := pressurePassPercent(cfg, pressure)
	decision := Decision{
		Allowed:     true,
		Reason:      ReasonAllowed,
		PassPercent: passPercent,
		DropRatio:   1 - passPercent/100,
	}
	if requests < cfg.MinSamples {
		decision.PassPercent = cfg.MaxPassPercent
		decision.DropRatio = 0
		decision.Reason = ReasonMinSamples
		return decision
	}
	allowed := s.sampler.Allow(passPercent, attrs)
	decision.Allowed = allowed
	if !allowed {
		decision.Reason = ReasonPressure
	}
	return decision
}

func (s *pressureStrategy) RecordAttempt(now time.Time, cfg Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordAttemptLocked(now, cfg)
}

func (s *pressureStrategy) RecordLocalReject(_ time.Time, _ Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localRejects++
}

func (s *pressureStrategy) recordAttemptLocked(now time.Time, cfg Config) int64 {
	s.applyDecayLocked(now, cfg)
	s.requests++
	return s.requests
}

func (s *pressureStrategy) Report(now time.Time, cfg Config, outcome Outcome, _ []Attr) {
	outcome = outcome.normalized()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyDecayLocked(now, cfg)
	switch outcome.Class {
	case OutcomeFailure:
		s.pressure += cfg.ErrorIncrease * outcome.Weight
		s.failures++
	case OutcomeOverload:
		s.pressure += cfg.OverloadIncrease * outcome.Weight
		s.failures++
	case OutcomeSuccess:
		s.pressure -= cfg.SuccessDecrease * outcome.Weight
		s.accepts++
	case OutcomeNeutral:
		switch cfg.NeutralPolicy {
		case NeutralCountsSuccess:
			s.pressure -= cfg.SuccessDecrease * outcome.Weight
			s.accepts++
		case NeutralCountsFailure:
			s.pressure += cfg.ErrorIncrease * outcome.Weight
			s.failures++
		}
	}
	s.pressure = clampFloat(s.pressure, 0, cfg.PressureLimit)
}

func (s *pressureStrategy) Snapshot(now time.Time, cfg Config) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyDecayLocked(now, cfg)
	passPercent := pressurePassPercent(cfg, s.pressure)
	return Snapshot{
		Enabled:        cfg.Enabled,
		Strategy:       StrategyPressure,
		PassPercent:    passPercent,
		DropRatio:      1 - passPercent/100,
		Pressure:       s.pressure,
		WindowRequests: s.requests,
		WindowAccepts:  s.accepts,
		WindowFailures: s.failures,
		LocalRejects:   s.localRejects,
		Config:         cfg,
		UpdatedAt:      now,
	}
}

func (s *pressureStrategy) applyDecayLocked(now time.Time, cfg Config) {
	if s.lastDecay.IsZero() {
		s.lastDecay = now
		return
	}
	if cfg.DecayPerSecond <= 0 {
		s.lastDecay = now
		return
	}
	elapsed := now.Sub(s.lastDecay).Seconds()
	if elapsed <= 0 {
		return
	}
	s.pressure -= elapsed * cfg.DecayPerSecond
	if s.pressure < 0 {
		s.pressure = 0
	}
	s.lastDecay = now
}

func pressurePassPercent(cfg Config, pressure float64) float64 {
	if cfg.PressureLimit <= 0 {
		return cfg.MaxPassPercent
	}
	ratio := clampFloat(pressure/cfg.PressureLimit, 0, 1)
	pass := cfg.MaxPassPercent - ratio*(cfg.MaxPassPercent-cfg.MinPassPercent)
	return clampFloat(pass, cfg.MinPassPercent, cfg.MaxPassPercent)
}
