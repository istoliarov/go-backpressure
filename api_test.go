package backpressure

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAttrTable(t *testing.T) {
	tests := []struct {
		name  string
		attrs []Attr
		key   string
		want  string
		ok    bool
	}{
		{
			name:  "finds first matching attr",
			attrs: []Attr{AttrKey("key", "first"), AttrKey("key", "second")},
			key:   "key",
			want:  "first",
			ok:    true,
		},
		{
			name:  "missing attr",
			attrs: []Attr{AttrKey("operation", "read")},
			key:   "key",
		},
		{
			name:  "clone attrs is independent",
			attrs: []Attr{AttrKey("key", "original")},
			key:   "key",
			want:  "original",
			ok:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := AttrValue(tt.attrs, tt.key)
			if tt.name == "clone attrs is independent" {
				cloned := cloneAttrs(tt.attrs)
				tt.attrs[0].Value = "mutated"
				got, ok = AttrValue(cloned, tt.key)
			}
			if got != tt.want || ok != tt.ok {
				t.Fatalf("AttrValue() = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestOutcomeTable(t *testing.T) {
	errBoom := errors.New("boom")
	tests := []struct {
		name string
		in   Outcome
		want Outcome
	}{
		{
			name: "success default",
			in:   Success(),
			want: Outcome{Class: OutcomeSuccess, Weight: 1},
		},
		{
			name: "failure preserves error",
			in:   Failure(errBoom),
			want: Outcome{Class: OutcomeFailure, Err: errBoom, Weight: 1},
		},
		{
			name: "latency and weight are chainable",
			in:   Overload(errBoom).WithLatency(time.Second).WithWeight(2),
			want: Outcome{Class: OutcomeOverload, Err: errBoom, Latency: time.Second, Weight: 2},
		},
		{
			name: "non-positive weight normalizes",
			in:   Outcome{Class: OutcomeSuccess}.normalized(),
			want: Outcome{Class: OutcomeSuccess, Weight: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.in != tt.want {
				t.Fatalf("unexpected outcome: got %+v want %+v", tt.in, tt.want)
			}
		})
	}
}

func TestOutcomeClassStringTable(t *testing.T) {
	tests := []struct {
		class OutcomeClass
		want  string
	}{
		{OutcomeSuccess, "success"},
		{OutcomeFailure, "failure"},
		{OutcomeNeutral, "neutral"},
		{OutcomeOverload, "overload"},
		{OutcomeClass(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.class.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNilControllerTable(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, bp *Controller)
	}{
		{
			name: "name is empty",
			run: func(t *testing.T, bp *Controller) {
				t.Helper()
				if bp.Name() != "" {
					t.Fatal("expected empty name")
				}
			},
		},
		{
			name: "acquire fail opens",
			run: func(t *testing.T, bp *Controller) {
				t.Helper()
				permit, decision := bp.Acquire(context.Background())
				if !decision.Allowed || decision.PassPercent != 100 {
					t.Fatalf("unexpected decision: %+v", decision)
				}
				permit.Report(Success())
			},
		},
		{
			name: "snapshot fail opens",
			run: func(t *testing.T, bp *Controller) {
				t.Helper()
				if snapshot := bp.Snapshot(); snapshot.Enabled || snapshot.PassPercent != 100 {
					t.Fatalf("unexpected snapshot: %+v", snapshot)
				}
			},
		},
		{
			name: "update and report are no-op",
			run: func(t *testing.T, bp *Controller) {
				t.Helper()
				bp.UpdateConfig(DefaultConfig())
				bp.Report(Success())
			},
		},
	}

	var bp *Controller
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t, bp)
		})
	}
}

func TestObserverTable(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "observer func nil callbacks are safe",
			run: func(t *testing.T) {
				t.Helper()
				var observer Observer = ObserverFunc{}
				observer.OnDecision("test", Decision{}, nil)
				observer.OnReport("test", Success(), Snapshot{}, nil)
				observer.OnConfigError("test", errors.New("bad"))
			},
		},
		{
			name: "multi observer fans out",
			run: func(t *testing.T) {
				t.Helper()
				calls := 0
				observer := MultiObserver{
					ObserverFunc{Decision: func(string, Decision, []Attr) { calls++ }},
					nil,
					ObserverFunc{Decision: func(string, Decision, []Attr) { calls++ }},
				}
				observer.OnDecision("test", Decision{}, nil)
				if calls != 2 {
					t.Fatalf("expected two calls, got %d", calls)
				}
			},
		},
		{
			name: "multi observer report and config error are safe",
			run: func(t *testing.T) {
				t.Helper()
				observer := MultiObserver{
					ObserverFunc{
						Report:      func(string, Outcome, Snapshot, []Attr) {},
						ConfigError: func(string, error) {},
					},
				}
				observer.OnReport("test", Success(), Snapshot{}, nil)
				observer.OnConfigError("test", errors.New("bad"))
			},
		},
		{
			name: "config error formats issues",
			run: func(t *testing.T) {
				t.Helper()
				err := ConfigError{Issues: []string{"one", "two"}}
				if got := err.Error(); got != "backpressure config: one; two" {
					t.Fatalf("unexpected error: %q", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestErrRejectedTable(t *testing.T) {
	tests := []struct {
		name     string
		decision Decision
		want     string
	}{
		{
			name:     "formats reason and pass percent",
			decision: Decision{Reason: ReasonPressure, PassPercent: 12.345},
			want:     "backpressure rejected request: reason=pressure pass_percent=12.35",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (ErrRejected{Decision: tt.decision}).Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}
