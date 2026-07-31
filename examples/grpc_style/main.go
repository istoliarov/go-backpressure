package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/istoliarov/go-backpressure"
)

type statusCode string

const (
	codeOK                statusCode = "OK"
	codeCanceled          statusCode = "Canceled"
	codeInvalidArgument   statusCode = "InvalidArgument"
	codeDeadlineExceeded  statusCode = "DeadlineExceeded"
	codeUnavailable       statusCode = "Unavailable"
	codeResourceExhausted statusCode = "ResourceExhausted"
)

type rpcError struct {
	code statusCode
	msg  string
}

func (e rpcError) Error() string {
	return string(e.code) + ": " + e.msg
}

type profile struct {
	ID   string
	Name string
}

func main() {
	cfg := backpressure.DefaultConfig()
	cfg.Strategy = backpressure.StrategyPressure

	rpcBP := backpressure.New("grpc_profile", cfg, backpressure.WithSampler(backpressure.NewKeySampler("tenant")))

	value, err := backpressure.Do(
		context.Background(),
		rpcBP,
		func(ctx context.Context) (profile, error) {
			return getProfile(ctx, "42")
		},
		func(ctx context.Context, decision backpressure.Decision) (profile, error) {
			return cachedProfile("42"), backpressure.ErrRejected{Decision: decision}
		},
		classifyRPC,
		backpressure.AttrKey("service", "profile"),
		backpressure.AttrKey("method", "GetProfile"),
		backpressure.AttrKey("tenant", "acme"),
	)

	fmt.Printf("profile=%s err=%v pass_percent=%.2f\n", value.Name, err, rpcBP.Snapshot().PassPercent)
}

func classifyRPC(_ profile, err error) backpressure.Outcome {
	if err == nil {
		return backpressure.Success()
	}

	var rpcErr rpcError
	if !errors.As(err, &rpcErr) {
		return backpressure.Failure(err)
	}

	switch rpcErr.code {
	case codeOK:
		return backpressure.Success()
	case codeResourceExhausted:
		return backpressure.Overload(err).WithLatency(40 * time.Millisecond)
	case codeUnavailable, codeDeadlineExceeded:
		return backpressure.Failure(err).WithLatency(80 * time.Millisecond)
	case codeCanceled, codeInvalidArgument:
		return backpressure.Neutral()
	default:
		return backpressure.Failure(err)
	}
}

func getProfile(context.Context, string) (profile, error) {
	return profile{ID: "42", Name: "Ada"}, nil
}

func cachedProfile(id string) profile {
	return profile{ID: id, Name: "cached"}
}
