package postgres

import (
	"context"
	"os"
	"testing"

	"transx/internal/platform/config"
)

func TestConnectWithDatabaseURL(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	pool, err := Connect(context.Background(), config.Postgres{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer pool.Close()
}
