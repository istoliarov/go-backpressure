package backpressure

import (
	"testing"
	"time"
)

func TestRollingWindowTable(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "records request outcomes and local rejects",
			run: func(t *testing.T) {
				t.Helper()
				cfg := DefaultConfig()
				cfg.Window = time.Second
				cfg.BucketCount = 10
				window := newRollingWindow(cfg.BucketCount)
				now := time.Unix(10, 0)

				window.recordRequest(now, cfg)
				window.recordOutcome(now, cfg, Success())
				window.recordOutcome(now, cfg, Failure(nil))
				window.recordLocalReject(now, cfg)

				totals := window.totals(now, cfg)
				if totals.requests != 1 || totals.accepts != 1 || totals.failures != 1 || totals.localRejects != 1 {
					t.Fatalf("unexpected totals: %+v", totals)
				}
			},
		},
		{
			name: "expires old buckets",
			run: func(t *testing.T) {
				t.Helper()
				cfg := DefaultConfig()
				cfg.Window = time.Second
				cfg.BucketCount = 10
				window := newRollingWindow(cfg.BucketCount)
				now := time.Unix(10, 0)

				window.recordRequest(now, cfg)
				later := now.Add(2 * time.Second)
				if totals := window.totals(later, cfg); totals.requests != 0 {
					t.Fatalf("expected old bucket to expire, got %+v", totals)
				}
			},
		},
		{
			name: "resizes when bucket count changes",
			run: func(t *testing.T) {
				t.Helper()
				cfg := DefaultConfig()
				window := newRollingWindow(2)
				cfg.BucketCount = 4
				window.recordRequest(time.Unix(10, 0), cfg)
				if len(window.buckets) != 4 {
					t.Fatalf("expected resize, got %d buckets", len(window.buckets))
				}
			},
		},
		{
			name: "bucket width falls back for invalid config",
			run: func(t *testing.T) {
				t.Helper()
				cfg := DefaultConfig()
				cfg.BucketCount = 0
				if got := bucketWidth(cfg); got <= 0 {
					t.Fatalf("expected positive width, got %v", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}
