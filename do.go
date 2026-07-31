package backpressure

import (
	"context"
	"fmt"
)

// Do wraps an operation with Acquire, fallback, classification, and exactly-once
// reporting. If call panics, Do reports a failure and re-panics.
func Do[T any](
	ctx context.Context,
	controller *Controller,
	call func(context.Context) (T, error),
	fallback func(context.Context, Decision) (T, error),
	classify func(T, error) Outcome,
	attrs ...Attr,
) (value T, err error) {
	permit, decision := controller.Acquire(ctx, attrs...)
	if !decision.Allowed {
		if fallback == nil {
			return value, ErrRejected{Decision: decision}
		}
		return fallback(ctx, decision)
	}

	reported := false
	defer func() {
		if r := recover(); r != nil {
			permit.Report(Failure(fmt.Errorf("panic: %v", r)))
			panic(r)
		}
		if !reported {
			permit.Report(classifyOutcome(classify, value, err))
		}
	}()

	value, err = call(ctx)
	permit.Report(classifyOutcome(classify, value, err))
	reported = true
	return value, err
}

func classifyOutcome[T any](classify func(T, error) Outcome, value T, err error) Outcome {
	if classify != nil {
		return classify(value, err)
	}
	if err != nil {
		return Failure(err)
	}
	return Success()
}
