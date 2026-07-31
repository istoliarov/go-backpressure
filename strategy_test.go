package backpressure

import "testing"

func TestStrategyHelpersTable(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "sre below min samples passes max",
			run: func(t *testing.T) {
				t.Helper()
				cfg := DefaultConfig()
				cfg.MaxPassPercent = 80
				cfg.MinSamples = 100
				pass, drop, reason := srePassPercent(cfg, rollingTotals{requests: 1})
				if pass != 80 || drop != 0 || reason != ReasonMinSamples {
					t.Fatalf("unexpected values: pass=%v drop=%v reason=%v", pass, drop, reason)
				}
			},
		},
		{
			name: "sre no drop but max cap reports max pass",
			run: func(t *testing.T) {
				t.Helper()
				cfg := DefaultConfig()
				cfg.MaxPassPercent = 70
				cfg.MinSamples = 1
				pass, drop, reason := srePassPercent(cfg, rollingTotals{requests: 100, accepts: 100})
				if pass != 70 || drop != 0 || reason != ReasonMaxPass {
					t.Fatalf("unexpected values: pass=%v drop=%v reason=%v", pass, drop, reason)
				}
			},
		},
		{
			name: "sre clamps to min pass",
			run: func(t *testing.T) {
				t.Helper()
				cfg := DefaultConfig()
				cfg.MinPassPercent = 25
				cfg.MinSamples = 1
				pass, drop, reason := srePassPercent(cfg, rollingTotals{requests: 1000, accepts: 0})
				if pass != 25 || drop != 0.75 || reason != ReasonAdaptiveDrop {
					t.Fatalf("unexpected values: pass=%v drop=%v reason=%v", pass, drop, reason)
				}
			},
		},
		{
			name: "pressure invalid limit returns max pass",
			run: func(t *testing.T) {
				t.Helper()
				cfg := DefaultConfig()
				cfg.MaxPassPercent = 60
				cfg.PressureLimit = 0
				if got := pressurePassPercent(cfg, 100); got != 60 {
					t.Fatalf("unexpected pass percent: %v", got)
				}
			},
		},
		{
			name: "clamp float boundaries",
			run: func(t *testing.T) {
				t.Helper()
				if clampFloat(-1, 0, 10) != 0 || clampFloat(11, 0, 10) != 10 || clampFloat(5, 0, 10) != 5 {
					t.Fatal("unexpected clamp behavior")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}
