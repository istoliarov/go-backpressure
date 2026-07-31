// Package backpressure provides unified client-side adaptive throttling for Go services.
//
// A controller sits before a downstream operation, decides whether the operation
// should be attempted, and learns from caller-classified outcomes. The package is
// protocol agnostic: Redis, HTTP, RPC, database calls, queues, and custom
// operations all report the same generic Success, Failure, Neutral, and Overload
// signals.
package backpressure
