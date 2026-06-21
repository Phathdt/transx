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
	Provider Provider `yaml:"provider" mapstructure:"provider"`
	FX       FX       `yaml:"fx"       mapstructure:"fx"`
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

// Provider configures the external-transfer payment provider. Name is stamped
// onto every EXTERNAL transfer (clients never send it); Mode drives the fake
// client (always_success | always_failure | always_timeout). BaseURL is where
// the consumer reaches the provider over HTTP; ListenAddress is where the
// stub-provider service serves POST /submit.
type Provider struct {
	Name          string `yaml:"name"           mapstructure:"name"`
	Mode          string `yaml:"mode"           mapstructure:"mode"`
	BaseURL       string `yaml:"base_url"       mapstructure:"base_url"`
	ListenAddress string `yaml:"listen_address" mapstructure:"listen_address"`
}

// FX configures static exchange-rate corridors and cross-currency fees for the
// local MVP adapter. Rate keys use FROM_TO (VND_USD = one VND converted to USD).
// Fee keys are a source currency code; the flat fee is charged in that currency
// when a transfer converts out of it.
type FX struct {
	Rates map[string]string `yaml:"rates" mapstructure:"rates"`
	Fees  map[string]string `yaml:"fees"  mapstructure:"fees"`
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

	// EXTERNAL transfers need a provider identity and mode; fall back to the
	// stub defaults so the wallet service runs without explicit provider config.
	if cfg.Provider.Name == "" {
		cfg.Provider.Name = "stub"
	}
	if cfg.Provider.Mode == "" {
		cfg.Provider.Mode = "always_success"
	}
	// HTTP provider transport defaults for local single-host runs; compose and
	// non-local envs override via PROVIDER__BASE_URL / PROVIDER__LISTEN_ADDRESS.
	if cfg.Provider.BaseURL == "" {
		cfg.Provider.BaseURL = "http://localhost:4100"
	}
	if cfg.Provider.ListenAddress == "" {
		cfg.Provider.ListenAddress = ":4100"
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
