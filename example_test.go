package backpressure_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/istoliarov/go-backpressure"
)

func ExampleController_Acquire() {
	bp := backpressure.New("cache_read", backpressure.DefaultConfig())

	permit, decision := bp.Acquire(context.Background(), backpressure.AttrKey("operation", "cache_read"))
	if !decision.Allowed {
		fmt.Println("fallback")
		return
	}

	value, err := readCache(context.Background(), "item:42")
	permit.Report(classifyCacheRead(value, err))
	fmt.Println("done")

	// Output: done
}

func ExampleDo() {
	bp := backpressure.New("rpc", backpressure.DefaultConfig())

	value, err := backpressure.Do(
		context.Background(),
		bp,
		func(ctx context.Context) (string, error) {
			return "value", nil
		},
		func(ctx context.Context, decision backpressure.Decision) (string, error) {
			return "fallback", nil
		},
		func(value string, err error) backpressure.Outcome {
			if err != nil {
				return backpressure.Failure(err)
			}
			return backpressure.Success()
		},
		backpressure.AttrKey("operation", "lookup"),
	)
	fmt.Println(value, err)

	// Output: value <nil>
}

func readCache(_ context.Context, key string) (string, error) {
	if key == "" {
		return "", errors.New("empty key")
	}
	return "cached", nil
}

func classifyCacheRead(value string, err error) backpressure.Outcome {
	switch {
	case value != "":
		return backpressure.Success()
	case err == nil:
		return backpressure.Neutral()
	case errors.Is(err, context.DeadlineExceeded):
		return backpressure.Failure(err).WithLatency(50 * time.Millisecond)
	default:
		return backpressure.Failure(err)
	}
}
