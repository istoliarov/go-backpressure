package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/istoliarov/go-backpressure"
)

var (
	errNoRows             = errors.New("no rows")
	errTooManyConnections = errors.New("too many connections")
	errSerializationRetry = errors.New("serialization retry")
)

type user struct {
	ID   string
	Name string
}

type userStore interface {
	FindUser(context.Context, string) (user, error)
}

func main() {
	dbBP := backpressure.New("db_find_user", backpressure.DefaultConfig())
	store := fakeStore{}

	value, err := backpressure.Do(
		context.Background(),
		dbBP,
		func(ctx context.Context) (user, error) {
			return store.FindUser(ctx, "42")
		},
		func(ctx context.Context, decision backpressure.Decision) (user, error) {
			return user{}, backpressure.ErrRejected{Decision: decision}
		},
		classifyDatabaseRead,
		backpressure.AttrKey("operation", "find_user"),
		backpressure.AttrKey("table", "users"),
	)

	fmt.Printf("user=%s err=%v rejects=%d\n", value.Name, err, dbBP.Snapshot().LocalRejects)
}

func classifyDatabaseRead(value user, err error) backpressure.Outcome {
	switch {
	case err == nil && value.ID != "":
		return backpressure.Success()
	case errors.Is(err, errNoRows):
		return backpressure.Neutral()
	case errors.Is(err, errTooManyConnections):
		return backpressure.Overload(err).WithLatency(25 * time.Millisecond)
	case errors.Is(err, context.DeadlineExceeded):
		return backpressure.Failure(err).WithLatency(100 * time.Millisecond)
	case errors.Is(err, errSerializationRetry):
		return backpressure.Neutral()
	default:
		return backpressure.Failure(err)
	}
}

type fakeStore struct{}

func (fakeStore) FindUser(context.Context, string) (user, error) {
	return user{ID: "42", Name: "Ada"}, nil
}
