package backpressure

import "time"

type sreStrategy struct {
	window  *rollingWindow
	sampler Sampler
}

func newSREStrategy(sampler Sampler) *sreStrategy {
	return &sreStrategy{
		window:  newRollingWindow(DefaultConfig().BucketCount),
		sampler: sampler,
	}
}

func (s *sreStrategy) Allow(now time.Time, cfg Config, attrs []Attr) Decision {
	totals := s.window.recordRequest(now, cfg)
	passPercent, dropRatio, reason := srePassPercent(cfg, totals)
	allowed := s.sampler.Allow(passPercent, attrs)
	decision := Decision{
		Allowed:     allowed,
		Reason:      ReasonAllowed,
		PassPercent: passPercent,
		DropRatio:   dropRatio,
	}
	if !allowed {
		decision.Reason = reason
	}
	if totals.requests < cfg.MinSamples && allowed {
		decision.Reason = ReasonMinSamples
	}
	return decision
}

func (s *sreStrategy) RecordAttempt(now time.Time, cfg Config) {
	s.window.recordRequest(now, cfg)
}

func (s *sreStrategy) RecordLocalReject(now time.Time, cfg Config) {
	s.window.recordLocalReject(now, cfg)
}

func (s *sreStrategy) Report(now time.Time, cfg Config, outcome Outcome, _ []Attr) {
	s.window.recordOutcome(now, cfg, outcome)
}

func (s *sreStrategy) Snapshot(now time.Time, cfg Config) Snapshot {
	totals := s.window.totals(now, cfg)
	passPercent, dropRatio, _ := srePassPercent(cfg, totals)
	return Snapshot{
		Enabled:        cfg.Enabled,
		Strategy:       StrategySREAdaptive,
		PassPercent:    passPercent,
		DropRatio:      dropRatio,
		WindowRequests: totals.requests,
		WindowAccepts:  totals.accepts,
		WindowFailures: totals.failures,
		LocalRejects:   totals.localRejects,
		Config:         cfg,
		UpdatedAt:      now,
	}
}

func srePassPercent(cfg Config, totals rollingTotals) (float64, float64, Reason) {
	if totals.requests < cfg.MinSamples {
		return cfg.MaxPassPercent, 0, ReasonMinSamples
	}
	dropRatio := 0.0
	if totals.requests > 0 {
		dropRatio = (float64(totals.requests) - cfg.SREK*float64(totals.accepts)) / (float64(totals.requests) + 1)
	}
	if dropRatio < 0 {
		dropRatio = 0
	}
	maxDropRatio := 1 - cfg.MinPassPercent/100
	if dropRatio > maxDropRatio {
		dropRatio = maxDropRatio
	}
	passPercent := 100 * (1 - dropRatio)
	passPercent = clampFloat(passPercent, cfg.MinPassPercent, cfg.MaxPassPercent)
	reason := ReasonAdaptiveDrop
	if dropRatio == 0 && passPercent < 100 {
		reason = ReasonMaxPass
	}
	return passPercent, dropRatio, reason
}
