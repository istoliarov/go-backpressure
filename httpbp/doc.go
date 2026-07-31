// Package httpbp provides an optional net/http RoundTripper adapter for
// backpressure controllers.
//
// The adapter is intentionally thin: it translates HTTP responses into generic
// backpressure outcomes and delegates all policy decisions to the core package.
// Consumers can ignore this package and wrap HTTP calls themselves.
package httpbp
