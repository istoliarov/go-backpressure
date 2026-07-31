package backpressure

import "testing"

func TestSamplerTable(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "sequence approximate distribution",
			run: func(t *testing.T) {
				t.Helper()
				sampler := NewSequenceSampler()
				const total = 10_000
				allowed := 0
				for i := 0; i < total; i++ {
					if sampler.Allow(25, nil) {
						allowed++
					}
				}
				if allowed < 2300 || allowed > 2700 {
					t.Fatalf("unexpected distribution: %d/%d", allowed, total)
				}
			},
		},
		{
			name: "sequence boundaries",
			run: func(t *testing.T) {
				t.Helper()
				sampler := NewSequenceSampler()
				if !sampler.Allow(100, nil) {
					t.Fatal("100 percent should always allow")
				}
				if sampler.Allow(0, nil) {
					t.Fatal("0 percent should always reject")
				}
			},
		},
		{
			name: "key sampler stable for same key",
			run: func(t *testing.T) {
				t.Helper()
				sampler := NewKeySampler("key")
				attrs := []Attr{AttrKey("key", "user-42")}
				first := sampler.Allow(50, attrs)
				for i := 0; i < 100; i++ {
					if got := sampler.Allow(50, attrs); got != first {
						t.Fatal("key sampler is not stable")
					}
				}
			},
		},
		{
			name: "key sampler falls back when key is missing",
			run: func(t *testing.T) {
				t.Helper()
				sampler := NewKeySampler("key")
				const total = 10_000
				allowed := 0
				for i := 0; i < total; i++ {
					if sampler.Allow(25, nil) {
						allowed++
					}
				}
				if allowed < 2300 || allowed > 2700 {
					t.Fatalf("unexpected fallback distribution: %d/%d", allowed, total)
				}
			},
		},
		{
			name: "random sampler approximate distribution",
			run: func(t *testing.T) {
				t.Helper()
				sampler := NewRandomSampler(42)
				const total = 10_000
				allowed := 0
				for i := 0; i < total; i++ {
					if sampler.Allow(25, nil) {
						allowed++
					}
				}
				if allowed < 2300 || allowed > 2700 {
					t.Fatalf("unexpected random distribution: %d/%d", allowed, total)
				}
			},
		},
		{
			name: "random sampler zero seed and nil sampler are safe",
			run: func(t *testing.T) {
				t.Helper()
				if !NewRandomSampler(0).Allow(100, nil) {
					t.Fatal("100 percent should allow")
				}
				var sampler *RandomSampler
				if !sampler.Allow(100, nil) {
					t.Fatal("nil random sampler should allow at 100 percent")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}
