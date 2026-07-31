package httpbp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/istoliarov/go-backpressure"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type denySampler struct{}

func (denySampler) Allow(float64, []backpressure.Attr) bool { return false }

func TestDefaultClassifierTable(t *testing.T) {
	tests := []struct {
		name string
		resp *http.Response
		err  error
		want backpressure.OutcomeClass
	}{
		{
			name: "network error is failure",
			err:  errors.New("dial"),
			want: backpressure.OutcomeFailure,
		},
		{
			name: "nil response is failure",
			want: backpressure.OutcomeFailure,
		},
		{
			name: "429 is overload",
			resp: &http.Response{StatusCode: http.StatusTooManyRequests},
			want: backpressure.OutcomeOverload,
		},
		{
			name: "503 is overload",
			resp: &http.Response{StatusCode: http.StatusServiceUnavailable},
			want: backpressure.OutcomeOverload,
		},
		{
			name: "500 is failure",
			resp: &http.Response{StatusCode: http.StatusInternalServerError},
			want: backpressure.OutcomeFailure,
		},
		{
			name: "404 is neutral",
			resp: &http.Response{StatusCode: http.StatusNotFound},
			want: backpressure.OutcomeNeutral,
		},
		{
			name: "200 is success",
			resp: &http.Response{StatusCode: http.StatusOK},
			want: backpressure.OutcomeSuccess,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultClassifier(tt.resp, tt.err, time.Millisecond)
			if got.Class != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got.Class)
			}
			if got.Latency != time.Millisecond {
				t.Fatalf("expected latency to be preserved, got %v", got.Latency)
			}
		})
	}
}

func TestRoundTripperTable(t *testing.T) {
	tests := []struct {
		name string
		rt   func(t *testing.T) RoundTripper
		want func(t *testing.T, resp *http.Response, err error)
	}{
		{
			name: "nil controller delegates to base",
			rt: func(t *testing.T) RoundTripper {
				t.Helper()
				return RoundTripper{
					Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
						return response(http.StatusAccepted), nil
					}),
				}
			},
			want: func(t *testing.T, resp *http.Response, err error) {
				t.Helper()
				if err != nil || resp.StatusCode != http.StatusAccepted {
					t.Fatalf("unexpected result: resp=%+v err=%v", resp, err)
				}
			},
		},
		{
			name: "allowed request reports success",
			rt: func(t *testing.T) RoundTripper {
				t.Helper()
				bp := backpressure.New("http", backpressure.DefaultConfig())
				t.Cleanup(func() {
					if snapshot := bp.Snapshot(); snapshot.WindowAccepts != 1 {
						t.Fatalf("expected success report, got %+v", snapshot)
					}
				})
				return RoundTripper{
					Controller: bp,
					Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
						return response(http.StatusOK), nil
					}),
				}
			},
			want: func(t *testing.T, resp *http.Response, err error) {
				t.Helper()
				if err != nil || resp.StatusCode != http.StatusOK {
					t.Fatalf("unexpected result: resp=%+v err=%v", resp, err)
				}
			},
		},
		{
			name: "local reject calls fallback",
			rt: func(t *testing.T) RoundTripper {
				t.Helper()
				cfg := backpressure.DefaultConfig()
				cfg.MinSamples = 1
				return RoundTripper{
					Controller: backpressure.New("http", cfg, backpressure.WithSampler(denySampler{})),
					Fallback: func(*http.Request, backpressure.Decision) (*http.Response, error) {
						return response(http.StatusTeapot), nil
					},
				}
			},
			want: func(t *testing.T, resp *http.Response, err error) {
				t.Helper()
				if err != nil || resp.StatusCode != http.StatusTeapot {
					t.Fatalf("unexpected fallback result: resp=%+v err=%v", resp, err)
				}
			},
		},
		{
			name: "local reject without fallback returns ErrRejected",
			rt: func(t *testing.T) RoundTripper {
				t.Helper()
				cfg := backpressure.DefaultConfig()
				cfg.MinSamples = 1
				return RoundTripper{
					Controller: backpressure.New("http", cfg, backpressure.WithSampler(denySampler{})),
				}
			},
			want: func(t *testing.T, _ *http.Response, err error) {
				t.Helper()
				var rejected backpressure.ErrRejected
				if !errors.As(err, &rejected) {
					t.Fatalf("expected ErrRejected, got %T %v", err, err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/users", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := tt.rt(t).RoundTrip(req)
			tt.want(t, resp, err)
		})
	}
}

func response(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}
}
