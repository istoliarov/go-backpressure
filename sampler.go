package backpressure

import (
	"hash/fnv"
	"sync/atomic"
	"time"
)

// Sampler decides whether a given pass percentage should allow an operation.
type Sampler interface {
	Allow(passPercent float64, attrs []Attr) bool
}

// SequenceSampler distributes decisions evenly across the request stream.
type SequenceSampler struct {
	seq atomic.Uint64
}

// NewSequenceSampler creates a stream-oriented sampler.
func NewSequenceSampler() *SequenceSampler {
	return &SequenceSampler{}
}

func (s *SequenceSampler) Allow(passPercent float64, _ []Attr) bool {
	return allowByPercent(s.seq.Add(1), passPercent)
}

// KeySampler makes stable decisions for the same attribute value.
type KeySampler struct {
	Key      string
	fallback SequenceSampler
}

// NewKeySampler creates a sampler keyed by the given attribute name.
func NewKeySampler(key string) *KeySampler {
	return &KeySampler{Key: key}
}

func (s *KeySampler) Allow(passPercent float64, attrs []Attr) bool {
	if s == nil {
		return allowByPercent(0, passPercent)
	}
	value, ok := AttrValue(attrs, s.Key)
	if !ok {
		return s.fallback.Allow(passPercent, attrs)
	}
	return allowByPercent(hashString(value), passPercent)
}

// RandomSampler makes pseudo-random decisions without using the global
// math/rand source.
type RandomSampler struct {
	state atomic.Uint64
}

// NewRandomSampler creates a sampler with the provided seed. A zero seed uses
// the current time.
func NewRandomSampler(seed uint64) *RandomSampler {
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	sampler := &RandomSampler{}
	sampler.state.Store(seed)
	return sampler
}

func (s *RandomSampler) Allow(passPercent float64, _ []Attr) bool {
	if s == nil {
		return allowByPercent(0, passPercent)
	}
	for {
		old := s.state.Load()
		next := xorshift64(old)
		if next == 0 {
			next = 0x9e3779b97f4a7c15
		}
		if s.state.CompareAndSwap(old, next) {
			return allowByPercent(next, passPercent)
		}
	}
}

func allowByPercent(seed uint64, passPercent float64) bool {
	if passPercent >= 100 {
		return true
	}
	if passPercent <= 0 {
		return false
	}
	const max = uint64(10_000)
	threshold := uint64(passPercent * 100)
	return mix64(seed)%max < threshold
}

func hashString(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

func mix64(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

func xorshift64(x uint64) uint64 {
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	return x
}
