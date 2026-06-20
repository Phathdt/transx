package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	App      App      `yaml:"app"      mapstructure:"app"`
	HTTP     HTTP     `yaml:"http"     mapstructure:"http"`
	Postgres Postgres `yaml:"postgres" mapstructure:"postgres"`
	Kafka    Kafka    `yaml:"kafka"    mapstructure:"kafka"`
	Auth     Auth     `yaml:"auth"     mapstructure:"auth"`
}

type App struct {
	Environment string `yaml:"environment" mapstructure:"environment"`
	LogLevel    string `yaml:"log_level"   mapstructure:"log_level"`
}

// Auth configures the auth service: JWT signing (HS256) and token lifetime.
type Auth struct {
	JWTSecret string        `yaml:"jwt_secret" mapstructure:"jwt_secret"`
	JWTTTL    time.Duration `yaml:"jwt_ttl"    mapstructure:"jwt_ttl"` // e.g. 24h
}

type HTTP struct {
	Address            string   `yaml:"address"              mapstructure:"address"`
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins" mapstructure:"cors_allowed_origins"`
}

type Postgres struct {
	DatabaseURL string `yaml:"database_url" mapstructure:"database_url"`
}

type Kafka struct {
	Brokers []string `yaml:"brokers" mapstructure:"brokers"`
}

// Load reads config from configPath YAML file with env var overrides.
// Env override format: APP__LOG_LEVEL overrides app.log_level
func Load(configPath string) (Config, error) {
	_ = godotenv.Load(".env", "../.env")

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("failed to read config file %q: %w", configPath, err)
	}

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) validate() error {
	var problems []string

	if c.Postgres.DatabaseURL == "" {
		problems = append(problems, "postgres.database_url is required")
	}
	if len(c.Kafka.Brokers) == 0 {
		problems = append(problems, "kafka.brokers must include at least one broker")
	}
	if c.HTTP.Address == "" {
		problems = append(problems, "http.address is required")
	}

	if len(problems) > 0 {
		return fmt.Errorf("config validation failed: %s", strings.Join(problems, "; "))
	}
	return nil
}
