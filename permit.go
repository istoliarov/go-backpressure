package backpressure

import "sync"

// Permit reports the result of an allowed operation. Implementations are safe to
// call more than once; only the first report is applied.
type Permit interface {
	Report(outcome Outcome)
}

type controllerPermit struct {
	once       sync.Once
	controller *Controller
	attrs      []Attr
}

func (p *controllerPermit) Report(outcome Outcome) {
	if p == nil || p.controller == nil {
		return
	}
	p.once.Do(func() {
		p.controller.report(outcome, p.attrs, false)
	})
}

type noopPermit struct{}

func (noopPermit) Report(Outcome) {}
