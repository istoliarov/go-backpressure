package backpressure

// Observer receives synchronous best-effort callbacks for decisions, reports,
// and config normalization errors. Observer panics are recovered by Controller,
// but slow callbacks still add latency to the caller's hot path.
type Observer interface {
	OnDecision(name string, decision Decision, attrs []Attr)
	OnReport(name string, outcome Outcome, snapshot Snapshot, attrs []Attr)
	OnConfigError(name string, err error)
}

// ObserverFunc adapts individual functions into an Observer.
type ObserverFunc struct {
	Decision    func(name string, decision Decision, attrs []Attr)
	Report      func(name string, outcome Outcome, snapshot Snapshot, attrs []Attr)
	ConfigError func(name string, err error)
}

func (o ObserverFunc) OnDecision(name string, decision Decision, attrs []Attr) {
	if o.Decision != nil {
		o.Decision(name, decision, attrs)
	}
}

func (o ObserverFunc) OnReport(name string, outcome Outcome, snapshot Snapshot, attrs []Attr) {
	if o.Report != nil {
		o.Report(name, outcome, snapshot, attrs)
	}
}

func (o ObserverFunc) OnConfigError(name string, err error) {
	if o.ConfigError != nil {
		o.ConfigError(name, err)
	}
}

// MultiObserver fans callbacks out to several observers.
type MultiObserver []Observer

func (m MultiObserver) OnDecision(name string, decision Decision, attrs []Attr) {
	for _, observer := range m {
		if observer != nil {
			observer.OnDecision(name, decision, attrs)
		}
	}
}

func (m MultiObserver) OnReport(name string, outcome Outcome, snapshot Snapshot, attrs []Attr) {
	for _, observer := range m {
		if observer != nil {
			observer.OnReport(name, outcome, snapshot, attrs)
		}
	}
}

func (m MultiObserver) OnConfigError(name string, err error) {
	for _, observer := range m {
		if observer != nil {
			observer.OnConfigError(name, err)
		}
	}
}
