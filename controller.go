package backpressure

import (
	"context"
	"sync/atomic"
	"time"
)

// Option customizes a Controller.
type Option func(*Controller)

// WithConfigProvider makes the controller read runtime config from provider.
func WithConfigProvider(provider ConfigProvider) Option {
	return func(c *Controller) {
		if provider != nil {
			c.provider = provider
			c.ownsProvider = false
		}
	}
}

// WithObserver installs synchronous observer callbacks. Observer panics are
// recovered, but callbacks still run on the caller's hot path and should be
// fast and non-blocking.
func WithObserver(observer Observer) Option {
	return func(c *Controller) {
		c.observer = observer
	}
}

// WithSampler installs the sampler used by all built-in strategies.
func WithSampler(sampler Sampler) Option {
	return func(c *Controller) {
		if sampler != nil {
			c.sampler = sampler
		}
	}
}

// WithStrategy registers a custom strategy type. The controller will use it
// when Config.Strategy matches strategyType.
func WithStrategy(strategyType StrategyType, strategy Strategy) Option {
	return func(c *Controller) {
		if strategyType == "" || strategy == nil {
			return
		}
		if c.custom == nil {
			c.custom = make(map[StrategyType]Strategy)
		}
		c.custom[strategyType] = strategy
	}
}

// Controller decides whether client-side operations should be attempted and
// adapts from caller-reported outcomes.
type Controller struct {
	name         string
	provider     ConfigProvider
	ownsProvider bool
	observer     Observer
	sampler      Sampler

	sre      *sreStrategy
	pressure *pressureStrategy
	custom   map[StrategyType]Strategy

	shadowRejects atomic.Int64
}

// New creates a controller. Pass DefaultConfig() unless you intentionally use a
// custom configuration.
func New(name string, cfg Config, opts ...Option) *Controller {
	if name == "" {
		name = "default"
	}
	controller := &Controller{
		name:         name,
		provider:     NewStaticConfig(cfg),
		ownsProvider: true,
		sampler:      NewSequenceSampler(),
	}
	for _, opt := range opts {
		opt(controller)
	}
	controller.sre = newSREStrategy(controller.sampler)
	controller.pressure = newPressureStrategy(controller.sampler)
	return controller
}

// Name returns the controller name used in snapshots and observer callbacks.
func (c *Controller) Name() string {
	if c == nil {
		return ""
	}
	return c.name
}

// UpdateConfig updates the controller config when it uses the default
// StaticConfig provider. Controllers created with WithConfigProvider should be
// updated through that provider instead.
func (c *Controller) UpdateConfig(cfg Config) {
	if c == nil {
		return
	}
	if !c.ownsProvider {
		return
	}
	if static, ok := c.provider.(*StaticConfig); ok {
		static.Store(cfg)
	}
}

// Allow records an attempted operation and returns only a decision. Prefer
// Acquire when the caller can report a result through the returned Permit.
func (c *Controller) Allow(ctx context.Context, attrs ...Attr) Decision {
	_ = ctx
	if c == nil {
		return Decision{Allowed: true, Reason: ReasonDisabled, PassPercent: 100}
	}
	return c.decide(attrs)
}

// Acquire records an attempted operation, decides whether it should run, and
// returns a Permit for reporting the operation outcome when it is allowed.
func (c *Controller) Acquire(ctx context.Context, attrs ...Attr) (Permit, Decision) {
	_ = ctx
	if c == nil {
		return noopPermit{}, Decision{Allowed: true, Reason: ReasonDisabled, PassPercent: 100}
	}
	decision := c.decide(attrs)
	if !decision.Allowed {
		return noopPermit{}, decision
	}
	return &controllerPermit{controller: c, attrs: cloneAttrs(attrs)}, decision
}

func (c *Controller) decide(attrs []Attr) Decision {
	cfg, cfgErr, failOpen := c.config()
	if cfgErr != nil {
		c.notifyConfigError(cfgErr)
	}
	if failOpen || !cfg.Enabled {
		decision := Decision{Allowed: true, Reason: ReasonDisabled, PassPercent: 100}
		if failOpen {
			decision.Reason = ReasonInvalidConfig
		}
		c.notifyDecision(decision, attrs)
		return decision
	}

	strategy := c.strategy(cfg)
	decision := strategy.Allow(time.Now(), cfg, attrs)
	if cfg.ShadowMode && !decision.Allowed {
		decision.WouldReject = true
		decision.Allowed = true
		c.shadowRejects.Add(1)
	}
	if !decision.Allowed {
		c.recordLocalReject(strategy, cfg)
	}
	c.notifyDecision(decision, attrs)
	return decision
}

// Report reports an outcome without a Permit and counts it as an attempted
// operation for built-in strategies. Prefer Permit.Report when the result
// belongs to a specific Acquire call.
func (c *Controller) Report(outcome Outcome) {
	if c == nil {
		return
	}
	c.report(outcome, nil, true)
}

// Snapshot returns the current controller state for debug endpoints and
// observability adapters.
func (c *Controller) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{Enabled: false, PassPercent: 100, UpdatedAt: time.Now()}
	}
	cfg, cfgErr, failOpen := c.config()
	if cfgErr != nil {
		c.notifyConfigError(cfgErr)
	}
	if failOpen || !cfg.Enabled {
		return Snapshot{
			Name:          c.name,
			Enabled:       false,
			Strategy:      cfg.Strategy,
			PassPercent:   100,
			ShadowRejects: c.shadowRejects.Load(),
			Config:        cfg,
			UpdatedAt:     time.Now(),
		}
	}
	snapshot := c.strategy(cfg).Snapshot(time.Now(), cfg)
	snapshot.Name = c.name
	snapshot.ShadowRejects = c.shadowRejects.Load()
	return snapshot
}

func (c *Controller) report(outcome Outcome, attrs []Attr, direct bool) {
	cfg, cfgErr, failOpen := c.config()
	if cfgErr != nil {
		c.notifyConfigError(cfgErr)
	}
	if failOpen || !cfg.Enabled {
		return
	}
	outcome = outcome.normalized()
	strategy := c.strategy(cfg)
	if direct {
		c.recordAttempt(strategy, cfg)
	}
	strategy.Report(time.Now(), cfg, outcome, attrs)
	snapshot := strategy.Snapshot(time.Now(), cfg)
	snapshot.Name = c.name
	snapshot.ShadowRejects = c.shadowRejects.Load()
	c.notifyReport(outcome, snapshot, attrs)
}

func (c *Controller) config() (Config, error, bool) {
	if c.provider == nil {
		return DefaultConfig(), nil, false
	}
	return normalizeConfigWithStrategy(c.provider.Snapshot(), c.isKnownStrategy)
}

func (c *Controller) strategy(cfg Config) Strategy {
	if strategy, ok := c.custom[cfg.Strategy]; ok {
		return strategy
	}
	switch cfg.Strategy {
	case StrategyPressure:
		return c.pressure
	default:
		return c.sre
	}
}

func (c *Controller) isKnownStrategy(strategy StrategyType) bool {
	if isBuiltInStrategy(strategy) {
		return true
	}
	_, ok := c.custom[strategy]
	return ok
}

func (c *Controller) recordAttempt(strategy Strategy, cfg Config) {
	recorder, ok := strategy.(interface {
		RecordAttempt(now time.Time, cfg Config)
	})
	if ok {
		recorder.RecordAttempt(time.Now(), cfg)
	}
}

func (c *Controller) recordLocalReject(strategy Strategy, cfg Config) {
	recorder, ok := strategy.(interface {
		RecordLocalReject(now time.Time, cfg Config)
	})
	if ok {
		recorder.RecordLocalReject(time.Now(), cfg)
	}
}

func (c *Controller) notifyDecision(decision Decision, attrs []Attr) {
	if c.observer == nil {
		return
	}
	defer recoverObserver()
	c.observer.OnDecision(c.name, decision, attrs)
}

func (c *Controller) notifyReport(outcome Outcome, snapshot Snapshot, attrs []Attr) {
	if c.observer == nil {
		return
	}
	defer recoverObserver()
	c.observer.OnReport(c.name, outcome, snapshot, attrs)
}

func (c *Controller) notifyConfigError(err error) {
	if c.observer == nil || err == nil || !isConfigError(err) {
		return
	}
	defer recoverObserver()
	c.observer.OnConfigError(c.name, err)
}

func recoverObserver() {
	_ = recover()
}
