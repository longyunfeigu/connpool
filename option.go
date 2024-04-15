package connpool

import "time"

type Option func(config *Config)

func WithMaxIdle(maxIdle uint) Option {
	return func(config *Config) {
		config.MaxIdle = maxIdle
	}
}

func WithMaxIdleTime(maxIdleTime time.Duration) Option {
	return func(config *Config) {
		config.MaxIdleTime = maxIdleTime
	}
}

func WithMaxCap(maxCap uint) Option {
	return func(config *Config) {
		config.MaxCap = maxCap
	}
}

func WithInitialCap(initialCap uint) Option {
	return func(config *Config) {
		config.InitialCap = initialCap
	}
}
