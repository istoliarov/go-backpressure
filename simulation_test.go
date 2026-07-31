package backpressure

import (
	"errors"
	"math"
	"testing"
)

func TestAlgorithmSimulationTable(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		run  func(*Controller)
		want func(t *testing.T, snapshot Snapshot)
	}{
		{
			name: "sre fixed failures drives pass percent to min",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.MinSamples = 0
				cfg.MinPassPercent = 15
				return cfg
			}(),
			run: func(bp *Controller) {
				for i := 0; i < 500; i++ {
					permit, decision := bp.Acquire(nil)
					if decision.Allowed {
						permit.Report(Failure(errors.New("timeout")))
					}
				}
			},
			want: func(t *testing.T, snapshot Snapshot) {
				t.Helper()
				if math.Abs(snapshot.PassPercent-15) > 0.000001 {
					t.Fatalf("expected min pass percent, got %+v", snapshot)
				}
			},
		},
		{
			name: "sre direct reports count attempts",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.MinSamples = 0
				return cfg
			}(),
			run: func(bp *Controller) {
				bp.Report(Failure(errors.New("timeout")))
			},
			want: func(t *testing.T, snapshot Snapshot) {
				t.Helper()
				if snapshot.WindowRequests != 1 || snapshot.WindowFailures != 1 {
					t.Fatalf("expected direct report to count attempt and failure, got %+v", snapshot)
				}
			},
		},
		{
			name: "pressure fixed overload reaches min pass",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.Strategy = StrategyPressure
				cfg.MinSamples = 0
				cfg.MinPassPercent = 20
				cfg.PressureLimit = 100
				cfg.OverloadIncrease = 100
				cfg.DecayPerSecond = 0
				return cfg
			}(),
			run: func(bp *Controller) {
				for i := 0; i < 5; i++ {
					bp.Report(Overload(errors.New("busy")))
				}
			},
			want: func(t *testing.T, snapshot Snapshot) {
				t.Helper()
				if snapshot.Pressure != 100 || snapshot.PassPercent != 20 || snapshot.WindowRequests != 5 {
					t.Fatalf("unexpected pressure snapshot: %+v", snapshot)
				}
			},
		},
		{
			name: "pressure success recovery lowers pressure",
			cfg: func() Config {
				cfg := DefaultConfig()
				cfg.Strategy = StrategyPressure
				cfg.MinSamples = 0
				cfg.PressureLimit = 100
				cfg.ErrorIncrease = 100
				cfg.SuccessDecrease = 25
				cfg.DecayPerSecond = 0
				return cfg
			}(),
			run: func(bp *Controller) {
				bp.Report(Failure(errors.New("timeout")))
				bp.Report(Success())
				bp.Report(Success())
			},
			want: func(t *testing.T, snapshot Snapshot) {
				t.Helper()
				if snapshot.Pressure != 50 || snapshot.WindowAccepts != 2 || snapshot.WindowRequests != 3 {
					t.Fatalf("unexpected recovery snapshot: %+v", snapshot)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := New("simulation", tt.cfg)
			tt.run(bp)
			tt.want(t, bp.Snapshot())
		})
	}
}
