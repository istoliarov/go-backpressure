package backpressure

import "sync/atomic"

type atomicConfig struct {
	value atomic.Value
}

func (c *atomicConfig) Load() Config {
	cfg, ok := c.value.Load().(Config)
	if !ok {
		return DefaultConfig()
	}
	return cfg
}

func (c *atomicConfig) Store(cfg Config) {
	c.value.Store(cfg)
}
