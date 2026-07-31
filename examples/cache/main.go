package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/istoliarov/go-backpressure"
)

var errCacheMiss = errors.New("cache miss")

type user struct {
	ID   string
	Name string
}

func main() {
	cfg := backpressure.DefaultConfig()
	cfg.ShadowMode = true

	cacheBP := backpressure.New("cache_read", cfg, backpressure.WithSampler(backpressure.NewKeySampler("key")))

	value, err := backpressure.Do(
		context.Background(),
		cacheBP,
		func(ctx context.Context) (user, error) {
			return readCache(ctx, "user:42")
		},
		func(ctx context.Context, decision backpressure.Decision) (user, error) {
			return loadPrimary(ctx, "user:42")
		},
		classifyCacheRead,
		backpressure.AttrKey("operation", "cache_read"),
		backpressure.AttrKey("key", "user:42"),
	)
	fmt.Println(value.Name, err)
	fmt.Printf("pass_percent=%.2f\n", cacheBP.Snapshot().PassPercent)
}

func classifyCacheRead(value user, err error) backpressure.Outcome {
	switch {
	case err == nil && value.ID != "":
		return backpressure.Success()
	case errors.Is(err, errCacheMiss):
		return backpressure.Neutral()
	case errors.Is(err, context.DeadlineExceeded):
		return backpressure.Failure(err).WithLatency(50 * time.Millisecond)
	default:
		return backpressure.Failure(err)
	}
}

func readCache(context.Context, string) (user, error) {
	return user{ID: "42", Name: "Ada"}, nil
}

func loadPrimary(context.Context, string) (user, error) {
	return user{ID: "42", Name: "Ada"}, nil
}
