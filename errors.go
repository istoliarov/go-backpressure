package backpressure

import "fmt"

// ErrRejected is returned by Do and optional adapters when a request is locally
// rejected and no fallback is provided.
type ErrRejected struct {
	Decision Decision
}

func (e ErrRejected) Error() string {
	return fmt.Sprintf("backpressure rejected request: reason=%s pass_percent=%.2f", e.Decision.Reason, e.Decision.PassPercent)
}
