package httpbp

import (
	"errors"
	"net/http"
	"time"

	"github.com/istoliarov/go-backpressure"
)

type Classifier func(*http.Response, error, time.Duration) backpressure.Outcome
type Fallback func(*http.Request, backpressure.Decision) (*http.Response, error)
type Attrs func(*http.Request) []backpressure.Attr

type RoundTripper struct {
	Controller *backpressure.Controller
	Base       http.RoundTripper
	Fallback   Fallback
	Classify   Classifier
	Attrs      Attrs
}

func (rt RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := rt.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if rt.Controller == nil {
		return base.RoundTrip(req)
	}

	attrs := defaultAttrs(req)
	if rt.Attrs != nil {
		attrs = rt.Attrs(req)
	}
	permit, decision := rt.Controller.Acquire(req.Context(), attrs...)
	if !decision.Allowed {
		if rt.Fallback != nil {
			return rt.Fallback(req, decision)
		}
		return nil, backpressure.ErrRejected{Decision: decision}
	}

	start := time.Now()
	resp, err := base.RoundTrip(req)
	latency := time.Since(start)
	classify := rt.Classify
	if classify == nil {
		classify = DefaultClassifier
	}
	permit.Report(classify(resp, err, latency))
	return resp, err
}

func DefaultClassifier(resp *http.Response, err error, latency time.Duration) backpressure.Outcome {
	if err != nil {
		return backpressure.Failure(err).WithLatency(latency)
	}
	if resp == nil {
		return backpressure.Failure(errors.New("nil http response")).WithLatency(latency)
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable:
		return backpressure.Overload(nil).WithLatency(latency)
	case resp.StatusCode >= 500:
		return backpressure.Failure(nil).WithLatency(latency)
	case resp.StatusCode >= 400:
		return backpressure.Neutral().WithLatency(latency)
	default:
		return backpressure.Success().WithLatency(latency)
	}
}

func defaultAttrs(req *http.Request) []backpressure.Attr {
	if req == nil || req.URL == nil {
		return nil
	}
	return []backpressure.Attr{
		backpressure.AttrKey("operation", "http_client"),
		backpressure.AttrKey("method", req.Method),
		backpressure.AttrKey("host", req.URL.Host),
	}
}
