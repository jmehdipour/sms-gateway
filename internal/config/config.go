package config

import (
	"bytes"
	_ "embed"
	"time"

	"github.com/spf13/viper"
)

//go:embed defaults.yaml
var defaults []byte

// ---- Root ----

type Config struct {
	HTTP       HTTPConfig       `mapstructure:"http"`
	MySQL      DatabaseConfig   `mapstructure:"mysql"`
	ClickHouse DatabaseConfig   `mapstructure:"clickhouse"`
	Redis      RedisConfig      `mapstructure:"redis"`
	Kafka      KafkaConfig      `mapstructure:"kafka"`
	Dispatcher DispatcherConfig `mapstructure:"dispatcher"`
	RateLimit  RateLimitConfig  `mapstructure:"rate_limit"`
	Providers  []ProviderConfig `mapstructure:"providers"`
	Pricing    PricingConfig    `mapstructure:"pricing"`
}

// ---- Leaf structs ----

type HTTPConfig struct {
	Addr string `mapstructure:"addr"`
}

type DatabaseConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idletime"`
	PingTimeout     time.Duration `mapstructure:"ping_timeout"`
}

type RedisConfig struct {
	Addr        string        `mapstructure:"addr"`
	Password    string        `mapstructure:"password"`
	DB          int           `mapstructure:"db"`
	DialTimeout time.Duration `mapstructure:"dial_timeout"`
}

type KafkaConfig struct {
	Brokers        []string `mapstructure:"brokers"`
	GroupID        string   `mapstructure:"group_id"`
	MinBytes       int      `mapstructure:"min_bytes"`
	MaxBytes       int      `mapstructure:"max_bytes"`
	CommitInterval int      `mapstructure:"commit_interval_ms"`
}

type DispatcherConfig struct {
	WorkerCount      int              `mapstructure:"worker_count"`
	BatchSize        int              `mapstructure:"batch_size"`
	BatchWait        time.Duration    `mapstructure:"batch_wait"`
	MaxRetryAttempts MaxRetryAttempts `mapstructure:"max_retry_attempts"`
}

type MaxRetryAttempts struct {
	Normal  int `mapstructure:"normal"`
	Express int `mapstructure:"express"`
}

type RateLimitConfig struct {
	RPS   int `mapstructure:"rps"`
	Burst int `mapstructure:"burst"`
}

type BreakerConfig struct {
	FailThreshold int `mapstructure:"fail_threshold" yaml:"fail_threshold"`
	OpenForMs     int `mapstructure:"open_for_ms"    yaml:"open_for_ms"`
}

type ProviderConfig struct {
	Name        string        `mapstructure:"name"`
	Enabled     bool          `mapstructure:"enabled"`
	BaseURL     string        `mapstructure:"base_url"`
	NormalPath  string        `mapstructure:"normal_path"`
	ExpressPath string        `mapstructure:"express_path"`
	TimeoutMs   int           `mapstructure:"timeout_ms"`
	Breaker     BreakerConfig `mapstructure:"breaker"`
}

type PricingConfig struct {
	Normal  int64 `mapstructure:"normal"`
	Express int64 `mapstructure:"express"`
}

// Load reads embedded defaults, merges user YAML (if provided), and applies env overrides (SMSGW_*).
func Load(path string) (Config, error) {
	v := viper.New()

	// embedded defaults
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewReader(defaults)); err != nil {
		return Config{}, err
	}

	if path != "" {
		v.SetConfigFile(path)
		_ = v.MergeInConfig()
	}

	// env override (SMSGW_*)
	v.SetEnvPrefix("SMSGW")
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
