package backpressure

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func BenchmarkBaselineAtomicAdd(b *testing.B) {
	var counter atomic.Uint64
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		counter.Add(1)
	}
}

func BenchmarkBaselineMutexLockUnlock(b *testing.B) {
	var mu sync.Mutex
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		mu.Lock()
		mu.Unlock()
	}
}

func BenchmarkAllowAllowed(b *testing.B) {
	bp := New("bench", DefaultConfig())
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = bp.Allow(ctx)
	}
}

func BenchmarkAllowAllowedWithAttrs(b *testing.B) {
	bp := New("bench", DefaultConfig())
	ctx := context.Background()
	attrs := []Attr{
		AttrKey("operation", "cache_read"),
		AttrKey("key", "user:42"),
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = bp.Allow(ctx, attrs...)
	}
}

func BenchmarkAcquireAllowed(b *testing.B) {
	bp := New("bench", DefaultConfig())
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = bp.Acquire(ctx)
	}
}

func BenchmarkAcquireAllowedWithAttrs(b *testing.B) {
	bp := New("bench", DefaultConfig())
	ctx := context.Background()
	attrs := []Attr{
		AttrKey("operation", "cache_read"),
		AttrKey("key", "user:42"),
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = bp.Acquire(ctx, attrs...)
	}
}

func BenchmarkAcquireRejected(b *testing.B) {
	cfg := DefaultConfig()
	cfg.MinSamples = 1
	cfg.MinPassPercent = 0
	bp := New("bench", cfg)
	for i := 0; i < 1000; i++ {
		permit, decision := bp.Acquire(context.Background())
		if decision.Allowed {
			permit.Report(Failure(errors.New("timeout")))
		}
	}
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = bp.Acquire(ctx)
	}
}

func BenchmarkAcquireWithObserver(b *testing.B) {
	bp := New("bench", DefaultConfig(), WithObserver(ObserverFunc{
		Decision: func(string, Decision, []Attr) {},
	}))
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = bp.Acquire(ctx)
	}
}

func BenchmarkReportSuccess(b *testing.B) {
	bp := New("bench", DefaultConfig())
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		bp.Report(Success())
	}
}

func BenchmarkReportFailure(b *testing.B) {
	bp := New("bench", DefaultConfig())
	err := errors.New("timeout")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		bp.Report(Failure(err))
	}
}

func BenchmarkReportWithObserver(b *testing.B) {
	bp := New("bench", DefaultConfig(), WithObserver(ObserverFunc{
		Report: func(string, Outcome, Snapshot, []Attr) {},
	}))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		bp.Report(Success())
	}
}

func BenchmarkDoWrapper(b *testing.B) {
	bp := New("bench", DefaultConfig())
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Do(
			ctx,
			bp,
			func(context.Context) (int, error) { return 1, nil },
			nil,
			func(int, error) Outcome { return Success() },
		)
	}
}

func BenchmarkSequenceSampler(b *testing.B) {
	sampler := NewSequenceSampler()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = sampler.Allow(50, nil)
	}
}

func BenchmarkKeySampler(b *testing.B) {
	sampler := NewKeySampler("key")
	attrs := []Attr{AttrKey("key", "user:42")}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = sampler.Allow(50, attrs)
	}
}

func BenchmarkRandomSampler(b *testing.B) {
	sampler := NewRandomSampler(42)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = sampler.Allow(50, nil)
	}
}
