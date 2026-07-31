package backpressure

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type allowSampler struct{}

func (allowSampler) Allow(float64, []Attr) bool { return true }

type denySampler struct{}

func (denySampler) Allow(float64, []Attr) bool { return false }

type staticStrategy struct {
	decision Decision
	reports  int
}

func (s *staticStrategy) Allow(time.Time, Config, []Attr) Decision {
	return s.decision
}

func (s *staticStrategy) Report(time.Time, Config, Outcome, []Attr) {
	s.reports++
}

func (s *staticStrategy) Snapshot(now time.Time, cfg Config) Snapshot {
	return Snapshot{
		Enabled:     cfg.Enabled,
		Strategy:    cfg.Strategy,
		PassPercent: s.decision.PassPercent,
		Config:      cfg,
		UpdatedAt:   now,
	}
}

func TestConfigNormalizationTable(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		want     func(Config) bool
		wantErr  bool
		wantFail bool
	}{
		{
			name: "zero config keeps disabled but fills safe defaults",
			cfg:  Config{},
			want: func(cfg Config) bool {
				return !cfg.Enabled && cfg.Strategy == StrategySREAdaptive && cfg.MinPassPercent == 0 && cfg.MaxPassPercent == 0
			},
			wantErr: true,
		},
		{
			name: "min and max pass clamp",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.MinPassPercent = 120
				cfg.MaxPassPercent = 60
				return cfg
			}(),
			want: func(cfg Config) bool {
				return cfg.MinPassPercent == 60 && cfg.MaxPassPercent == 60
			},
			wantErr: true,
		},
		{
			name: "invalid window bucket and k use defaults",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.Window = -time.Second
				cfg.BucketCount = -1
				cfg.SREK = 0.5
				return cfg
			}(),
			want: func(cfg Config) bool {
				def := DefaultConfig()
				return cfg.Window == def.Window && cfg.BucketCount == def.BucketCount && cfg.SREK == def.SREK
			},
			wantErr: true,
		},
		{
			name: "unknown strategy fail opens",
			cfg: Config{
				Enabled:  true,
				Strategy: "unknown",
			},
			want: func(cfg Config) bool {
				return !cfg.Enabled
			},
			wantErr:  true,
			wantFail: true,
		},
		{
			name: "pressure strategy with negative pressure limit uses default",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.Strategy = StrategyPressure
				cfg.PressureLimit = -1
				return cfg
			}(),
			want: func(cfg Config) bool {
				return cfg.PressureLimit == DefaultConfig().PressureLimit
			},
			wantErr: true,
		},
		{
			name: "unknown neutral policy uses default",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.NeutralPolicy = "custom"
				return cfg
			}(),
			want: func(cfg Config) bool {
				return cfg.NeutralPolicy == DefaultConfig().NeutralPolicy
			},
			wantErr: true,
		},
		{
			name: "explicit zero pressure knobs are preserved for sre strategy",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.PressureLimit = 0
				cfg.ErrorIncrease = 0
				cfg.OverloadIncrease = 0
				cfg.SuccessDecrease = 0
				cfg.DecayPerSecond = 0
				return cfg
			}(),
			want: func(cfg Config) bool {
				return cfg.PressureLimit == 0 &&
					cfg.ErrorIncrease == 0 &&
					cfg.OverloadIncrease == 0 &&
					cfg.SuccessDecrease == 0 &&
					cfg.DecayPerSecond == 0
			},
		},
		{
			name: "pressure strategy with zero pressure limit uses default but preserves explicit zero weights",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.Strategy = StrategyPressure
				cfg.PressureLimit = 0
				cfg.ErrorIncrease = 0
				cfg.OverloadIncrease = 0
				cfg.SuccessDecrease = 0
				cfg.DecayPerSecond = 0
				return cfg
			}(),
			want: func(cfg Config) bool {
				return cfg.PressureLimit == DefaultConfig().PressureLimit &&
					cfg.ErrorIncrease == 0 &&
					cfg.OverloadIncrease == 0 &&
					cfg.SuccessDecrease == 0 &&
					cfg.DecayPerSecond == 0
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err, failOpen := normalizeConfig(tt.cfg)
			if tt.wantErr != (err != nil) {
				t.Fatalf("error mismatch: err=%v", err)
			}
			if tt.wantFail != failOpen {
				t.Fatalf("fail-open mismatch: got %v", failOpen)
			}
			if !tt.want(got) {
				t.Fatalf("unexpected normalized config: %+v", got)
			}
		})
	}
}

func TestControllerAcquireTable(t *testing.T) {
	tests := []struct {
		name string
		new  func(t *testing.T) (*Controller, *atomic.Bool)
		want func(t *testing.T, decision Decision, observed *atomic.Bool)
	}{
		{
			name: "empty name becomes default",
			new: func(t *testing.T) (*Controller, *atomic.Bool) {
				t.Helper()
				return New("", DefaultConfig()), nil
			},
			want: func(t *testing.T, decision Decision, _ *atomic.Bool) {
				t.Helper()
				if !decision.Allowed {
					t.Fatalf("unexpected decision: %+v", decision)
				}
			},
		},
		{
			name: "disabled controller always allows",
			new: func(t *testing.T) (*Controller, *atomic.Bool) {
				t.Helper()
				cfg := DefaultConfig()
				cfg.Enabled = false
				return New("test", cfg), nil
			},
			want: func(t *testing.T, decision Decision, _ *atomic.Bool) {
				t.Helper()
				if !decision.Allowed || decision.Reason != ReasonDisabled {
					t.Fatalf("unexpected decision: %+v", decision)
				}
			},
		},
		{
			name: "invalid config fail opens and notifies observer",
			new: func(t *testing.T) (*Controller, *atomic.Bool) {
				t.Helper()
				var observed atomic.Bool
				bp := New("test", Config{Enabled: true, Strategy: "unknown"}, WithObserver(ObserverFunc{
					ConfigError: func(_ string, _ error) { observed.Store(true) },
				}))
				return bp, &observed
			},
			want: func(t *testing.T, decision Decision, observed *atomic.Bool) {
				t.Helper()
				if !decision.Allowed || decision.Reason != ReasonInvalidConfig {
					t.Fatalf("expected fail-open invalid config, got %+v", decision)
				}
				if observed == nil || !observed.Load() {
					t.Fatal("expected config error observer call")
				}
			},
		},
		{
			name: "shadow mode reports would reject but allows caller",
			new: func(t *testing.T) (*Controller, *atomic.Bool) {
				t.Helper()
				cfg := DefaultConfig()
				cfg.ShadowMode = true
				cfg.MinSamples = 1
				cfg.MinPassPercent = 0
				return New("test", cfg, WithSampler(denySampler{})), nil
			},
			want: func(t *testing.T, decision Decision, _ *atomic.Bool) {
				t.Helper()
				if !decision.Allowed || !decision.WouldReject || decision.Reason != ReasonAdaptiveDrop {
					t.Fatalf("unexpected shadow decision: %+v", decision)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp, observed := tt.new(t)
			_, decision := bp.Acquire(context.Background())
			if tt.name == "empty name becomes default" && bp.Name() != "default" {
				t.Fatalf("expected default name, got %q", bp.Name())
			}
			tt.want(t, decision, observed)
		})
	}
}

func TestShadowModeMetricsTable(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want func(t *testing.T, snapshot Snapshot)
	}{
		{
			name: "shadow reject is not actual local reject",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.ShadowMode = true
				cfg.MinSamples = 1
				cfg.MinPassPercent = 0
				return cfg
			}(),
			want: func(t *testing.T, snapshot Snapshot) {
				t.Helper()
				if snapshot.ShadowRejects != 1 || snapshot.LocalRejects != 0 {
					t.Fatalf("unexpected shadow snapshot: %+v", snapshot)
				}
			},
		},
		{
			name: "real reject is actual local reject",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.MinSamples = 1
				cfg.MinPassPercent = 0
				return cfg
			}(),
			want: func(t *testing.T, snapshot Snapshot) {
				t.Helper()
				if snapshot.ShadowRejects != 0 || snapshot.LocalRejects != 1 {
					t.Fatalf("unexpected real reject snapshot: %+v", snapshot)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := New("test", tt.cfg, WithSampler(denySampler{}))
			_, _ = bp.Acquire(context.Background())
			tt.want(t, bp.Snapshot())
		})
	}
}

func TestRuntimeConfigTable(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "UpdateConfig works with default provider",
			run: func(t *testing.T) {
				t.Helper()
				bp := New("test", DefaultConfig())
				cfg := DefaultConfig()
				cfg.Enabled = false
				bp.UpdateConfig(cfg)
				if decision := bp.Allow(context.Background()); decision.Reason != ReasonDisabled {
					t.Fatalf("expected disabled decision, got %+v", decision)
				}
			},
		},
		{
			name: "external provider controls config",
			run: func(t *testing.T) {
				t.Helper()
				provider := NewStaticConfig(DefaultConfig())
				bp := New("test", DefaultConfig(), WithConfigProvider(provider))
				cfg := DefaultConfig()
				cfg.MaxPassPercent = 70
				provider.Store(cfg)
				if snapshot := bp.Snapshot(); snapshot.PassPercent != 70 {
					t.Fatalf("expected provider config, got %+v", snapshot)
				}
			},
		},
		{
			name: "UpdateConfig does not mutate external provider",
			run: func(t *testing.T) {
				t.Helper()
				provider := NewStaticConfig(DefaultConfig())
				bp := New("test", DefaultConfig(), WithConfigProvider(provider))
				cfg := DefaultConfig()
				cfg.Enabled = false
				bp.UpdateConfig(cfg)
				if decision := bp.Allow(context.Background()); decision.Reason == ReasonDisabled {
					t.Fatalf("external provider should remain enabled: %+v", decision)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestCustomStrategyTable(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "unknown custom strategy without registration fail opens",
			run: func(t *testing.T) {
				t.Helper()
				cfg := DefaultConfig()
				cfg.Strategy = "tenant_policy"
				bp := New("test", cfg)
				if decision := bp.Allow(context.Background()); decision.Reason != ReasonInvalidConfig {
					t.Fatalf("expected invalid config, got %+v", decision)
				}
			},
		},
		{
			name: "registered custom strategy is used",
			run: func(t *testing.T) {
				t.Helper()
				strategy := &staticStrategy{decision: Decision{
					Allowed:     true,
					Reason:      ReasonAllowed,
					PassPercent: 42,
				}}
				cfg := DefaultConfig()
				cfg.Strategy = "tenant_policy"
				bp := New("test", cfg, WithStrategy("tenant_policy", strategy))
				permit, decision := bp.Acquire(context.Background())
				if !decision.Allowed || decision.PassPercent != 42 {
					t.Fatalf("unexpected decision: %+v", decision)
				}
				permit.Report(Success())
				if strategy.reports != 1 {
					t.Fatalf("expected one report, got %d", strategy.reports)
				}
				if snapshot := bp.Snapshot(); snapshot.Strategy != "tenant_policy" || snapshot.PassPercent != 42 {
					t.Fatalf("unexpected snapshot: %+v", snapshot)
				}
			},
		},
		{
			name: "invalid custom strategy option is ignored",
			run: func(t *testing.T) {
				t.Helper()
				cfg := DefaultConfig()
				cfg.Strategy = "tenant_policy"
				bp := New("test", cfg, WithStrategy("", &staticStrategy{}), WithStrategy("tenant_policy", nil))
				if decision := bp.Allow(context.Background()); decision.Reason != ReasonInvalidConfig {
					t.Fatalf("expected invalid config, got %+v", decision)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestSREStrategyTable(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		run  func(*Controller)
		want func(t *testing.T, snapshot Snapshot)
	}{
		{
			name: "failures reduce pass percent without violating min",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.MinSamples = 1
				cfg.MinPassPercent = 10
				return cfg
			}(),
			run: func(bp *Controller) {
				for i := 0; i < 200; i++ {
					permit, decision := bp.Acquire(context.Background())
					if decision.Allowed {
						permit.Report(Failure(errors.New("timeout")))
					}
				}
			},
			want: func(t *testing.T, snapshot Snapshot) {
				t.Helper()
				if snapshot.PassPercent >= 100 {
					t.Fatalf("expected reduced pass percent, got %+v", snapshot)
				}
				if snapshot.PassPercent < 10 {
					t.Fatalf("min pass violated: %+v", snapshot)
				}
			},
		},
		{
			name: "max pass caps healthy downstream",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.MinSamples = 1
				cfg.MaxPassPercent = 80
				return cfg
			}(),
			run: func(bp *Controller) {
				permit, decision := bp.Acquire(context.Background())
				if decision.Allowed {
					permit.Report(Success())
				}
			},
			want: func(t *testing.T, snapshot Snapshot) {
				t.Helper()
				if snapshot.PassPercent != 80 {
					t.Fatalf("expected max pass cap, got %+v", snapshot)
				}
			},
		},
		{
			name: "recovery grows pass rate after successes",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.MinSamples = 1
				cfg.MinPassPercent = 5
				return cfg
			}(),
			run: func(bp *Controller) {
				for i := 0; i < 100; i++ {
					permit, decision := bp.Acquire(context.Background())
					if decision.Allowed {
						permit.Report(Failure(errors.New("timeout")))
					}
				}
				before := bp.Snapshot()
				for i := 0; i < 400; i++ {
					permit, decision := bp.Acquire(context.Background())
					if decision.Allowed {
						permit.Report(Success())
					}
				}
				after := bp.Snapshot()
				if after.PassPercent <= before.PassPercent {
					t.Fatalf("expected recovery: before=%+v after=%+v", before, after)
				}
			},
			want: func(t *testing.T, snapshot Snapshot) {
				t.Helper()
				if snapshot.PassPercent <= 5 {
					t.Fatalf("expected recovered pass percent, got %+v", snapshot)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := New("test", tt.cfg, WithSampler(allowSampler{}))
			tt.run(bp)
			tt.want(t, bp.Snapshot())
		})
	}
}

func TestOutcomePolicyTable(t *testing.T) {
	tests := []struct {
		name         string
		policy       NeutralPolicy
		outcome      Outcome
		wantAccepts  int64
		wantFailures int64
	}{
		{
			name:    "neutral ignored by default",
			policy:  NeutralDoesNotAffect,
			outcome: Neutral(),
		},
		{
			name:        "neutral can count as success",
			policy:      NeutralCountsSuccess,
			outcome:     Neutral(),
			wantAccepts: 1,
		},
		{
			name:         "neutral can count as failure",
			policy:       NeutralCountsFailure,
			outcome:      Neutral(),
			wantFailures: 1,
		},
		{
			name:         "overload counts as failure",
			policy:       NeutralDoesNotAffect,
			outcome:      Overload(errors.New("busy")),
			wantFailures: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.MinSamples = 1
			cfg.NeutralPolicy = tt.policy
			bp := New("test", cfg)
			bp.Report(tt.outcome)
			snapshot := bp.Snapshot()
			if snapshot.WindowAccepts != tt.wantAccepts || snapshot.WindowFailures != tt.wantFailures {
				t.Fatalf("unexpected snapshot: %+v", snapshot)
			}
		})
	}
}

func TestPressureStrategyTable(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Controller)
		want func(t *testing.T, snapshot Snapshot)
	}{
		{
			name: "failure increases pressure",
			run: func(bp *Controller) {
				bp.Report(Failure(errors.New("timeout")))
			},
			want: func(t *testing.T, snapshot Snapshot) {
				t.Helper()
				if snapshot.Pressure != 100 || snapshot.PassPercent >= 100 {
					t.Fatalf("unexpected failure pressure: %+v", snapshot)
				}
			},
		},
		{
			name: "overload has heavier weight",
			run: func(bp *Controller) {
				bp.Report(Overload(errors.New("busy")))
			},
			want: func(t *testing.T, snapshot Snapshot) {
				t.Helper()
				if snapshot.Pressure != 300 {
					t.Fatalf("unexpected overload pressure: %+v", snapshot)
				}
			},
		},
		{
			name: "success decreases pressure",
			run: func(bp *Controller) {
				bp.Report(Failure(errors.New("timeout")))
				bp.Report(Success())
			},
			want: func(t *testing.T, snapshot Snapshot) {
				t.Helper()
				if snapshot.Pressure != 90 || snapshot.WindowAccepts != 1 {
					t.Fatalf("unexpected success decrease: %+v", snapshot)
				}
			},
		},
		{
			name: "decay lowers pressure over time",
			run: func(bp *Controller) {
				bp.Report(Failure(errors.New("timeout")))
				time.Sleep(20 * time.Millisecond)
			},
			want: func(t *testing.T, snapshot Snapshot) {
				t.Helper()
				if snapshot.Pressure >= 100 {
					t.Fatalf("expected decay, got %+v", snapshot)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Strategy = StrategyPressure
			cfg.MinSamples = 1
			cfg.PressureLimit = 1_000
			cfg.ErrorIncrease = 100
			cfg.OverloadIncrease = 300
			cfg.SuccessDecrease = 10
			cfg.DecayPerSecond = 1_000
			bp := New("test", cfg)
			tt.run(bp)
			tt.want(t, bp.Snapshot())
		})
	}
}

func TestDoTable(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "success reports once",
			run: func(t *testing.T) {
				t.Helper()
				bp := New("test", DefaultConfig())
				value, err := Do(
					context.Background(),
					bp,
					func(context.Context) (string, error) { return "value", nil },
					nil,
					func(string, error) Outcome { return Success() },
				)
				if err != nil || value != "value" {
					t.Fatalf("unexpected result: value=%q err=%v", value, err)
				}
				if snapshot := bp.Snapshot(); snapshot.WindowAccepts != 1 {
					t.Fatalf("expected success report, got %+v", snapshot)
				}
			},
		},
		{
			name: "fallback handles local reject",
			run: func(t *testing.T) {
				t.Helper()
				cfg := DefaultConfig()
				cfg.MinSamples = 1
				bp := New("test", cfg, WithSampler(denySampler{}))
				value, err := Do(
					context.Background(),
					bp,
					func(context.Context) (string, error) { return "call", nil },
					func(context.Context, Decision) (string, error) { return "fallback", nil },
					func(string, error) Outcome { return Success() },
				)
				if err != nil || value != "fallback" {
					t.Fatalf("unexpected fallback result: value=%q err=%v", value, err)
				}
			},
		},
		{
			name: "missing fallback returns ErrRejected",
			run: func(t *testing.T) {
				t.Helper()
				cfg := DefaultConfig()
				cfg.MinSamples = 1
				bp := New("test", cfg, WithSampler(denySampler{}))
				_, err := Do(
					context.Background(),
					bp,
					func(context.Context) (string, error) { return "call", nil },
					nil,
					func(string, error) Outcome { return Success() },
				)
				var rejected ErrRejected
				if !errors.As(err, &rejected) {
					t.Fatalf("expected ErrRejected, got %T %v", err, err)
				}
			},
		},
		{
			name: "panic reports failure and repanics",
			run: func(t *testing.T) {
				t.Helper()
				cfg := DefaultConfig()
				cfg.MinSamples = 100
				bp := New("test", cfg)
				defer func() {
					if recover() == nil {
						t.Fatal("expected panic")
					}
					if snapshot := bp.Snapshot(); snapshot.WindowFailures != 1 {
						t.Fatalf("expected panic to report failure, got %+v", snapshot)
					}
				}()
				_, _ = Do(
					context.Background(),
					bp,
					func(context.Context) (string, error) { panic("boom") },
					nil,
					nil,
				)
			},
		},
		{
			name: "nil classifier maps nil error to success",
			run: func(t *testing.T) {
				t.Helper()
				bp := New("test", DefaultConfig())
				_, err := Do(
					context.Background(),
					bp,
					func(context.Context) (string, error) { return "ok", nil },
					nil,
					nil,
				)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if snapshot := bp.Snapshot(); snapshot.WindowAccepts != 1 {
					t.Fatalf("expected success, got %+v", snapshot)
				}
			},
		},
		{
			name: "nil classifier maps error to failure",
			run: func(t *testing.T) {
				t.Helper()
				bp := New("test", DefaultConfig())
				callErr := errors.New("timeout")
				_, err := Do(
					context.Background(),
					bp,
					func(context.Context) (string, error) { return "", callErr },
					nil,
					nil,
				)
				if !errors.Is(err, callErr) {
					t.Fatalf("unexpected error: %v", err)
				}
				if snapshot := bp.Snapshot(); snapshot.WindowFailures != 1 {
					t.Fatalf("expected failure, got %+v", snapshot)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestPermitTable(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "controller permit reports once",
			run: func(t *testing.T) {
				t.Helper()
				bp := New("test", DefaultConfig())
				permit, decision := bp.Acquire(context.Background())
				if !decision.Allowed {
					t.Fatal("expected allow")
				}
				permit.Report(Success())
				permit.Report(Success())
				if snapshot := bp.Snapshot(); snapshot.WindowAccepts != 1 {
					t.Fatalf("expected one report, got %+v", snapshot)
				}
			},
		},
		{
			name: "noop permit is safe",
			run: func(t *testing.T) {
				t.Helper()
				noopPermit{}.Report(Failure(errors.New("ignored")))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestObserverSafetyTable(t *testing.T) {
	tests := []struct {
		name string
		bp   func() *Controller
		run  func(*Controller)
	}{
		{
			name: "decision panic is recovered",
			bp: func() *Controller {
				return New("test", DefaultConfig(), WithObserver(ObserverFunc{
					Decision: func(string, Decision, []Attr) { panic("observer") },
				}))
			},
			run: func(bp *Controller) {
				_, decision := bp.Acquire(context.Background())
				if !decision.Allowed {
					t.Fatalf("expected allow, got %+v", decision)
				}
			},
		},
		{
			name: "report panic is recovered",
			bp: func() *Controller {
				return New("test", DefaultConfig(), WithObserver(ObserverFunc{
					Report: func(string, Outcome, Snapshot, []Attr) { panic("observer") },
				}))
			},
			run: func(bp *Controller) {
				bp.Report(Success())
			},
		},
		{
			name: "config panic is recovered",
			bp: func() *Controller {
				return New("test", Config{Enabled: true, Strategy: "unknown"}, WithObserver(ObserverFunc{
					ConfigError: func(string, error) { panic("observer") },
				}))
			},
			run: func(bp *Controller) {
				_, decision := bp.Acquire(context.Background())
				if !decision.Allowed {
					t.Fatalf("expected fail-open allow, got %+v", decision)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(tt.bp())
		})
	}
}

func TestParallelAcquireReportTable(t *testing.T) {
	tests := []struct {
		name       string
		goroutines int
		iterations int
	}{
		{
			name:       "parallel acquire and report",
			goroutines: 32,
			iterations: 1000,
		},
		{
			name:       "parallel runtime config update",
			goroutines: 8,
			iterations: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewStaticConfig(DefaultConfig())
			bp := New("test", DefaultConfig(), WithConfigProvider(provider))
			var wg sync.WaitGroup
			for i := 0; i < tt.goroutines; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					for j := 0; j < tt.iterations; j++ {
						if tt.name == "parallel runtime config update" && id == 0 {
							cfg := DefaultConfig()
							cfg.MinPassPercent = float64(j % 50)
							provider.Store(cfg)
						}
						permit, decision := bp.Acquire(context.Background())
						if decision.Allowed {
							permit.Report(Success())
						}
					}
				}(i)
			}
			wg.Wait()
			if snapshot := bp.Snapshot(); snapshot.WindowAccepts == 0 {
				t.Fatalf("expected reports, got %+v", snapshot)
			}
		})
	}
}
