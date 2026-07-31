package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/istoliarov/go-backpressure"
)

var (
	errQueueFull       = errors.New("queue full")
	errBrokerDown      = errors.New("broker down")
	errDuplicateRecord = errors.New("duplicate record")
)

type event struct {
	Key  string
	Body string
}

type producer interface {
	Publish(context.Context, event) error
}

func main() {
	cfg := backpressure.DefaultConfig()
	cfg.MinPassPercent = 20

	queueBP := backpressure.New("queue_publish", cfg, backpressure.WithSampler(backpressure.NewKeySampler("partition_key")))
	producer := fakeProducer{}
	ev := event{Key: "user:42", Body: "profile.updated"}

	permit, decision := queueBP.Acquire(
		context.Background(),
		backpressure.AttrKey("topic", "user-events"),
		backpressure.AttrKey("partition_key", ev.Key),
	)
	if !decision.Allowed {
		bufferLocally(ev)
		fmt.Printf("buffered locally reason=%s\n", decision.Reason)
		return
	}

	err := producer.Publish(context.Background(), ev)
	permit.Report(classifyPublish(err))

	fmt.Printf("published err=%v pass_percent=%.2f\n", err, queueBP.Snapshot().PassPercent)
}

func classifyPublish(err error) backpressure.Outcome {
	switch {
	case err == nil:
		return backpressure.Success()
	case errors.Is(err, errQueueFull):
		return backpressure.Overload(err).WithLatency(30 * time.Millisecond)
	case errors.Is(err, errBrokerDown), errors.Is(err, context.DeadlineExceeded):
		return backpressure.Failure(err).WithLatency(120 * time.Millisecond)
	case errors.Is(err, errDuplicateRecord):
		return backpressure.Neutral()
	default:
		return backpressure.Failure(err)
	}
}

type fakeProducer struct{}

func (fakeProducer) Publish(context.Context, event) error {
	return nil
}

func bufferLocally(event) {}
