package connpool

import "time"

var (
	defaultMaxIdle     uint = 10
	defaultMaxIdleTime      = time.Minute
	defaultMaxCap      uint = 20
	defaultInitialCap  uint = 5
)

type Config struct {
	MaxIdle     uint          // 最大空闲连接数
	MaxIdleTime time.Duration // 最大空闲时间
	MaxCap      uint          // 最大并发存活数
	InitialCap  uint          // 初始连接数
}

func DefaultConfig() *Config {
	return &Config{
		MaxIdle:     defaultMaxIdle,     // 默认最大空闲连接数
		MaxIdleTime: defaultMaxIdleTime, // 默认最大空闲时间为1分钟
		MaxCap:      defaultMaxCap,      // 默认最大并发存活数
		InitialCap:  defaultInitialCap,  // 默认初始连接数
	}
}

func LoadConfig(options ...Option) *Config {
	config := DefaultConfig()
	for _, option := range options {
		option(config)
	}
	return config
}

func (c *Config) Validate() error {
	if !(c.InitialCap <= c.MaxIdle && c.MaxCap >= c.MaxIdle && c.InitialCap >= 0) {
		return ErrCapacitySetting
	}
	return nil
}
