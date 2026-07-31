package backpressure

import (
	"sync"
	"time"
)

type rollingTotals struct {
	requests     int64
	accepts      int64
	failures     int64
	localRejects int64
}

type rollingBucket struct {
	start        int64
	requests     int64
	accepts      int64
	failures     int64
	localRejects int64
}

type rollingWindow struct {
	mu      sync.Mutex
	buckets []rollingBucket
}

func newRollingWindow(bucketCount int) *rollingWindow {
	return &rollingWindow{buckets: make([]rollingBucket, bucketCount)}
}

func (w *rollingWindow) recordRequest(now time.Time, cfg Config) rollingTotals {
	w.mu.Lock()
	defer w.mu.Unlock()
	bucket := w.bucket(now, cfg)
	bucket.requests++
	return w.totalsLocked(now, cfg)
}

func (w *rollingWindow) recordLocalReject(now time.Time, cfg Config) rollingTotals {
	w.mu.Lock()
	defer w.mu.Unlock()
	bucket := w.bucket(now, cfg)
	bucket.localRejects++
	return w.totalsLocked(now, cfg)
}

func (w *rollingWindow) recordOutcome(now time.Time, cfg Config, outcome Outcome) rollingTotals {
	w.mu.Lock()
	defer w.mu.Unlock()
	bucket := w.bucket(now, cfg)
	switch outcome.Class {
	case OutcomeSuccess:
		bucket.accepts++
	case OutcomeFailure, OutcomeOverload:
		bucket.failures++
	case OutcomeNeutral:
		switch cfg.NeutralPolicy {
		case NeutralCountsSuccess:
			bucket.accepts++
		case NeutralCountsFailure:
			bucket.failures++
		}
	}
	return w.totalsLocked(now, cfg)
}

func (w *rollingWindow) totals(now time.Time, cfg Config) rollingTotals {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.totalsLocked(now, cfg)
}

func (w *rollingWindow) bucket(now time.Time, cfg Config) *rollingBucket {
	w.ensureBucketCount(cfg.BucketCount)
	width := bucketWidth(cfg)
	start := now.UnixNano() / int64(width)
	idx := int(start % int64(len(w.buckets)))
	bucket := &w.buckets[idx]
	if bucket.start != start {
		*bucket = rollingBucket{start: start}
	}
	return bucket
}

func (w *rollingWindow) ensureBucketCount(bucketCount int) {
	if bucketCount <= 0 {
		bucketCount = DefaultConfig().BucketCount
	}
	if len(w.buckets) == bucketCount {
		return
	}
	w.buckets = make([]rollingBucket, bucketCount)
}

func (w *rollingWindow) totalsLocked(now time.Time, cfg Config) rollingTotals {
	width := bucketWidth(cfg)
	current := now.UnixNano() / int64(width)
	span := int64(cfg.BucketCount)
	var totals rollingTotals
	for _, bucket := range w.buckets {
		if bucket.start == 0 || current-bucket.start >= span {
			continue
		}
		totals.requests += bucket.requests
		totals.accepts += bucket.accepts
		totals.failures += bucket.failures
		totals.localRejects += bucket.localRejects
	}
	return totals
}

func bucketWidth(cfg Config) time.Duration {
	if cfg.BucketCount <= 0 {
		return DefaultConfig().Window / time.Duration(DefaultConfig().BucketCount)
	}
	width := cfg.Window / time.Duration(cfg.BucketCount)
	if width <= 0 {
		return time.Nanosecond
	}
	return width
}
