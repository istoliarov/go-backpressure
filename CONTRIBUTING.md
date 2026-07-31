# Contributing

Thanks for helping improve `go-backpressure`.

## Design Principles

- Keep the core package protocol agnostic.
- Do not add required dependencies for metrics, logging, tracing, Redis, HTTP, or RPC.
- Let callers classify their own outcomes.
- Prefer small, stable public APIs over convenience that narrows the use case.
- Keep hot-path allocations at zero where practical.

## Development

```bash
go test ./...
go vet ./...
go test -run=^$ -bench=. -benchmem ./...
```

Race tests should pass in environments with cgo support:

```bash
go test -race ./...
```

## Tests

Unit tests should be table-driven. Add simulation tests for algorithm behavior
when a change affects adaptive decisions over time.

## Compatibility

The module targets Go 1.22 or newer. Avoid changes that require a newer Go
version unless the minimum version is intentionally raised.
