package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/istoliarov/go-backpressure"
)

func main() {
	controller := backpressure.New("queue_publish", backpressure.DefaultConfig())

	permit, decision := controller.Acquire(context.Background(), backpressure.AttrKey("operation", "queue_publish"))
	if !decision.Allowed {
		fmt.Println("write skipped locally")
		return
	}

	err := publish(context.Background())
	if err != nil {
		permit.Report(backpressure.Failure(err))
		return
	}
	permit.Report(backpressure.Success())
}

func publish(context.Context) error {
	return errors.New("broker unavailable")
}
