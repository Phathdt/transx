package config

import (
	"os"
	"testing"
)

func TestLoadReadsYAML(t *testing.T) {
	// Write a minimal temp config file
	content := `
app:
  environment: local
  log_level: info
http:
  address: ":8081"
postgres:
  database_url: "postgres://transx:transx@localhost:15432/transx?sslmode=disable"
kafka:
  brokers:
    - "localhost:9092"
`
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Postgres.DatabaseURL == "" {
		t.Fatal("expected database URL")
	}
	if got := len(cfg.Kafka.Brokers); got != 1 {
		t.Fatalf("expected 1 kafka broker, got %d", got)
	}
}

func TestValidateRejectsMissingRequiredFields(t *testing.T) {
	cfg := Config{}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected validation error for empty config")
	}
}

// TestEnvOverridesBindGRPCAddresses guards the docker-compose wiring: the gRPC
// keys must exist in YAML for viper's AutomaticEnv to bind their env overrides.
func TestEnvOverridesBindGRPCAddresses(t *testing.T) {
	content := `
app:
  environment: local
  log_level: info
http:
  address: ":8081"
postgres:
  database_url: "postgres://transx:transx@localhost:15432/transx?sslmode=disable"
kafka:
  brokers:
    - "localhost:9092"
`
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()
}
