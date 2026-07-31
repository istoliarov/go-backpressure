# Go Backpressure

[![Go Reference](https://pkg.go.dev/badge/github.com/istoliarov/go-backpressure.svg)](https://pkg.go.dev/github.com/istoliarov/go-backpressure)
[![Go](https://img.shields.io/badge/go-%3E%3D1.22-00ADD8.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Unified client-side backpressure for Go services.

`go-backpressure` helps a client service stop overwhelming a degraded downstream.
Instead of choosing between "send 100% of traffic" and "turn the dependency off",
it continuously adjusts how much traffic is allowed through.

The core package is intentionally small and generic. It has no required metrics,
logging, tracing, Redis, HTTP, RPC, or framework dependency. Consumers decide how
to classify outcomes, expose observability, and connect it to their own systems.

## What You Get

- **Controlled degradation instead of all-or-nothing switches.** Keep sending a
  safe percentage of useful calls while a downstream is recovering.
- **Fast local decisions.** `Allow`, `Acquire`, and direct `Report` complete in
  roughly 170-230 ns/op in the current benchmark suite.
- **No hot-path heap allocations.** Direct decisions, reports, attrs, observers,
  and samplers stay at `0 allocs/op` in the measured paths.
- **Cheaper failure handling.** A local reject is hundreds of nanoseconds; a
  downstream timeout is usually milliseconds or seconds.
- **Unified integration model.** The same controller API works for Redis, HTTP,
  RPC, databases, queues, and custom operations.
- **Caller-owned semantics.** Your service decides whether cache miss, 404,
  validation errors, timeouts, overload signals, or partial results should affect
  backpressure.

## Why This Matters

Downstream systems rarely fail cleanly. Redis gets slower before it disappears.
An RPC service starts timing out on a subset of calls. An HTTP dependency begins
returning 429 or 503 under load. If every client keeps sending the same volume of
traffic, the degradation feeds itself:

1. The downstream slows down or errors more often.
2. Clients wait longer, retry more, and pile up goroutines.
3. The downstream spends more work on requests that will time out anyway.
4. Latency and error rate grow across the system.

A circuit breaker is useful when a dependency is plainly broken, but it is often
too binary for partial degradation. This library gives you a middle ground:
allow 100%, then 80%, then 50%, then 10%, and recover smoothly when the
downstream becomes healthy again.

Local rejection is cheaper than a network timeout. A controlled fallback is
better than a cascading outage.

## What Problem It Solves

`go-backpressure` protects any client-side operation:

- Redis and cache reads or writes
- HTTP clients
- RPC or gRPC calls
- database calls
- queue producers
- any custom operation where overload should reduce traffic

The core package is intentionally unified. It does not know what Redis, HTTP,
gRPC, queues, or databases are. Your code classifies each result as `Success`,
`Failure`, `Neutral`, or `Overload`, and the controller adapts from those generic
signals.

That design matters because `err != nil` is not always infrastructure failure:

- cache miss: usually `Neutral`
- HTTP 404: usually `Neutral`
- validation error: usually `Neutral`
- timeout: `Failure`
- connection refused: `Failure`
- HTTP 429/503: `Overload`

## Install

```bash
go get github.com/istoliarov/go-backpressure
```

Requires Go 1.22 or newer.

## Quick Start

```go
package cache

import (
	"context"
	"errors"

	"github.com/istoliarov/go-backpressure"
)

var cacheBP = backpressure.New("cache_read", backpressure.DefaultConfig())

func GetUser(ctx context.Context, id string) (User, error) {
	permit, decision := cacheBP.Acquire(
		ctx,
		backpressure.AttrKey("operation", "cache_read"),
		backpressure.AttrKey("key", id),
	)
	if !decision.Allowed {
		return loadUserFromPrimary(ctx, id)
	}

	user, err := redisGetUser(ctx, id)
	permit.Report(classifyCacheRead(user, err))
	return user, err
}

func classifyCacheRead(user User, err error) backpressure.Outcome {
	switch {
	case err == nil:
		return backpressure.Success()
	case errors.Is(err, ErrCacheMiss):
		return backpressure.Neutral()
	case errors.Is(err, context.DeadlineExceeded):
		return backpressure.Failure(err)
	default:
		return backpressure.Failure(err)
	}
}
```

## Safer Wrapper

Use `Do` when you want the library to guarantee that `Report` happens exactly
once. Panics are reported as failures and then re-thrown.

```go
user, err := backpressure.Do(
	ctx,
	cacheBP,
	func(ctx context.Context) (User, error) {
		return redisGetUser(ctx, id)
	},
	func(ctx context.Context, decision backpressure.Decision) (User, error) {
		return loadUserFromPrimary(ctx, id)
	},
	func(user User, err error) backpressure.Outcome {
		return classifyCacheRead(user, err)
	},
	backpressure.AttrKey("operation", "cache_read"),
	backpressure.AttrKey("key", id),
)
```

## Pseudocode

### Cache Read

```text
decision = backpressure.acquire("cache_read", key=user_id)

if decision.rejected:
    return primary_database.get(user_id)

value, error = redis.get(user_id)

if value.exists:
    decision.report(success)
elif error == cache_miss:
    decision.report(neutral)
elif error == timeout:
    decision.report(failure)
else:
    decision.report(failure)

return value, error
```

### HTTP Client

```text
decision = backpressure.acquire("http", host=api.example.com)

if decision.rejected:
    return cached_response_or_local_error()

response, error = http_client.do(request)

if error:
    decision.report(failure)
elif response.status in [429, 503]:
    decision.report(overload)
elif response.status >= 500:
    decision.report(failure)
elif response.status >= 400:
    decision.report(neutral)
else:
    decision.report(success)
```

### RPC Call

```text
decision = backpressure.acquire("rpc", method=GetProfile)

if decision.rejected:
    return fallback_profile()

reply, error = rpc.call(GetProfile)

if error.code in [Unavailable, DeadlineExceeded]:
    decision.report(failure)
elif error.code == ResourceExhausted:
    decision.report(overload)
elif error:
    decision.report(neutral)
else:
    decision.report(success)
```

## Unified By Design

The core API speaks in operational signals, not protocols.

```go
type OutcomeClass int

const (
	OutcomeSuccess OutcomeClass = iota
	OutcomeFailure
	OutcomeNeutral
	OutcomeOverload
)
```

Your application, wrapper, or optional adapter can translate HTTP status codes,
gRPC status codes, Redis errors, queue errors, database errors, or
business-specific rules into these outcomes. The adaptive controller stays small,
stable, and reusable.

## What The Core Does Not Do

The core package does not:

- import a metrics client;
- import a logging or tracing SDK;
- decide whether `err != nil` is always a failure;
- assume HTTP, gRPC, Redis, queues, or databases;
- start goroutines per request;
- coordinate state between service instances.

Those choices belong to the consuming service. The library provides the
controller, decisions, outcomes, snapshots, and observer hooks.

## Algorithms

### SRE Adaptive Throttling

The default strategy tracks requests and accepted responses in a rolling window.
When attempted requests grow too far beyond accepted requests, the controller
starts rejecting a controlled percentage locally.

```text
dropRatio = max(0, (requests - K * accepts) / (requests + 1))
```

This is useful for protecting dependencies that degrade gradually.

### Pressure Strategy

The pressure strategy keeps a pressure score:

- failures increase pressure
- overload signals increase pressure faster
- successes decrease pressure
- time decay lets the system recover even at low traffic

The pressure score maps linearly to a pass percentage between
`MaxPassPercent` and `MinPassPercent`.

### Custom Strategies

If the built-in algorithms are not the right fit, consumers can register their
own strategy while keeping the same public controller API.

```go
cfg := backpressure.DefaultConfig()
cfg.Strategy = "my_strategy"

bp := backpressure.New(
	"search",
	cfg,
	backpressure.WithStrategy("my_strategy", myStrategy),
)
```

The custom strategy still receives generic `Config`, `Attr`, and `Outcome`
values. The core package remains protocol agnostic.

Custom strategy support is experimental until v1. The stable integration path is
the built-in controller API with caller-owned outcome classification.

## Sampling

Built-in samplers cover the common generic cases:

- `SequenceSampler`: even distribution across the request stream;
- `KeySampler`: stable behavior for the same attr value, such as user, tenant,
  shard, or cache key;
- `RandomSampler`: pseudo-random distribution without global `math/rand`.

## Performance

The core hot path is designed to sit in front of high-volume client operations.
In practice, that means a controller can protect a Redis call, RPC call, HTTP
request, queue publish, or custom operation without adding a meaningful amount
of local CPU overhead compared with the cost of the downstream call itself.

The current benchmark suite measures both the ideal path and common real-world
paths: attrs, observers, rejected decisions, reports, wrappers, and samplers.

Example benchmark on AMD Ryzen 7 7800X3D, Go 1.24.4, Windows:

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Baseline atomic add | 1.6 | 0 | 0 |
| Baseline mutex lock/unlock | 3.7 | 0 | 0 |
| Allow allowed | 176.1 | 0 | 0 |
| Allow allowed with attrs | 168.2 | 0 | 0 |
| Acquire allowed | 178.1 | 2 | 0 |
| Acquire allowed with attrs | 179.7 | 5 | 0 |
| Acquire rejected | 183.2 | 0 | 0 |
| Acquire with observer | 195.5 | 2 | 0 |
| Direct report success | 215.3 | 0 | 0 |
| Direct report failure | 223.2 | 0 | 0 |
| Direct report with observer | 227.8 | 0 | 0 |
| Do wrapper | 359.1 | 48 | 1 |
| Sequence sampler | 1.6 | 0 | 0 |
| Key sampler | 7.9 | 0 | 0 |
| Random sampler | 4.7 | 0 | 0 |

### How To Read These Numbers

- `Allow` is the cheapest API when you only need a decision.
- `Acquire` returns a `Permit` and is still allocation-free on the measured hot
  path.
- `Acquire with attrs` remains allocation-free; attrs are only copied into the
  permit when the call is allowed so delayed `Report` sees stable values.
- `Acquire with observer` shows the cost of synchronous observer callbacks
  without doing any logging or metrics work inside the callback.
- Direct `Report` is an escape hatch that counts a standalone attempted
  operation. For normal request flow, prefer `Acquire` plus `Permit.Report`.
- `Do wrapper` is the ergonomic safety API. It costs more than manual
  `Acquire`/`Report`, but still stays below a microsecond in this benchmark.
- Samplers are effectively free compared with controller decisions.

The useful comparison is not against a single mutex or atomic operation. The
useful comparison is against what this avoids: expensive network timeouts,
retry storms, goroutine buildup, and cascading downstream degradation.

Run locally:

```bash
go test -run=^$ -bench=. -benchmem ./...
```

Exact numbers depend on hardware and Go version. The important property is the
shape: local allow/reject/report decisions are measured in hundreds of
nanoseconds, while avoided downstream timeouts are normally configured in
milliseconds or seconds.

For example, even a 10 ms timeout is roughly **50,000x** slower than a 200 ns
local decision. Backpressure does not make downstream calls faster; it helps you
avoid sending calls that are likely to waste time and amplify overload.

## Runtime Configuration

Controllers can read configuration from a provider on every `Acquire` and
`Report`, so you can tune behavior without recreating the controller.

Start from `DefaultConfig()` for normal use. A zero-value `Config{}` normalizes
to a disabled fail-open controller, so partially initialized config does not
accidentally enable shedding.

Explicit zero values are preserved where zero is meaningful. For example,
`MaxPassPercent = 0` can intentionally close traffic, and `MinSamples = 0`
can intentionally disable warm-up.

```go
cfg := backpressure.DefaultConfig()
cfg.MinPassPercent = 10
cfg.MaxPassPercent = 100
cfg.ShadowMode = true

bp := backpressure.New("cache_read", cfg)

// Later:
cfg.ShadowMode = false
cfg.MinPassPercent = 5
bp.UpdateConfig(cfg)
```

## Observability

The core package has an observer interface instead of depending on a specific
metrics library. Use it to connect decisions and reports to whatever your
service already uses.

Observer callbacks are synchronous and run on the caller's hot path. Keep them
fast and non-blocking; if you need buffering, batching, logging, or network I/O,
do that behind your own adapter.

Useful metrics to expose:

- `backpressure_decisions_total{controller,reason,allowed}`
- `backpressure_reports_total{controller,outcome}`
- `backpressure_local_rejects_total{controller,reason}`
- `backpressure_pass_percent{controller}`
- `backpressure_drop_ratio{controller}`
- `backpressure_pressure{controller}`
- `backpressure_window_requests{controller}`
- `backpressure_window_accepts{controller}`
- `backpressure_shadow_rejects{controller}`

Every controller also exposes a `Snapshot` for debug endpoints.

```go
snapshot := bp.Snapshot()
```

## Optional Packages

Optional packages may provide thin examples for common protocols, but the core
library does not require them. You can ignore every adapter and call
`Acquire`/`Report` directly around your own operation.

## Production Rollout

1. Start in shadow mode and observe what would have been rejected.
2. Enable with a conservative floor, for example `MinPassPercent = 80`.
3. Watch local rejects, pass percent, drop ratio, downstream latency, and errors.
4. Lower `MinPassPercent` gradually to production values.
5. Keep fallback behavior explicit and cheap.

## FAQ

### Is this a circuit breaker?

No. A circuit breaker is usually binary: closed, open, half-open. This library is
percentage-based. It can pass 92%, 70%, 35%, or 5% of traffic depending on the
health signals it receives.

### Should cache miss be a failure?

Usually no. A cache miss is a normal business result. Treating it as failure can
make the controller reduce cache traffic when the cache is actually behaving
correctly.

### When should I use key-based sampling?

Use key-based sampling when you want stable behavior for the same user, tenant,
or cache key. Use sequence sampling when you want an even distribution across
the request stream and do not want the same keys to be skipped repeatedly.

### What happens on bad config?

The library is fail-open by default. Invalid critical configuration allows
traffic and emits an observer event rather than breaking the caller.

## Status

This project is being built as a small, production-oriented core with optional
adapters around it. The public API is intended to stay compact and protocol
agnostic.
