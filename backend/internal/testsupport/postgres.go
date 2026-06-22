//go:build integration

package testsupport

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	postgresImage         = "postgres:16-alpine"
	postgresContainerName = "transx-integration-postgres"
)

func NewPostgresPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := tcpostgres.Run(ctx,
		postgresImage,
		tcpostgres.WithDatabase("transx_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
		testcontainers.WithReuseByName(postgresContainerName),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	adminConnString, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}

	databaseName := testDatabaseName()
	createTestDatabase(ctx, t, adminConnString, databaseName)
	t.Cleanup(func() {
		dropTestDatabase(context.Background(), t, adminConnString, databaseName)
	})

	connString, err := databaseConnectionString(adminConnString, databaseName)
	if err != nil {
		t.Fatalf("postgres test database connection string: %v", err)
	}

	MigrateUp(ctx, t, connString)

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		t.Fatalf("open pgx pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

func testDatabaseName() string {
	return fmt.Sprintf("transx_test_%d_%d", os.Getpid(), time.Now().UnixNano())
}

func createTestDatabase(ctx context.Context, t *testing.T, adminURL, databaseName string) {
	t.Helper()

	withMigrationDB(ctx, t, adminURL, func(db *sql.DB) error {
		_, err := db.ExecContext(ctx, `CREATE DATABASE `+quoteIdentifier(databaseName))
		return err
	})
}

func dropTestDatabase(ctx context.Context, t *testing.T, adminURL, databaseName string) {
	t.Helper()

	withMigrationDB(ctx, t, adminURL, func(db *sql.DB) error {
		_, err := db.ExecContext(ctx, `DROP DATABASE IF EXISTS `+quoteIdentifier(databaseName)+` WITH (FORCE)`)
		return err
	})
}

func databaseConnectionString(adminURL, databaseName string) (string, error) {
	parsed, err := url.Parse(adminURL)
	if err != nil {
		return "", fmt.Errorf("parse postgres connection string: %w", err)
	}
	parsed.Path = "/" + databaseName
	return parsed.String(), nil
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func MigrateUp(ctx context.Context, t *testing.T, databaseURL string) {
	t.Helper()

	withMigrationDB(ctx, t, databaseURL, func(db *sql.DB) error {
		if err := prepareTestDatabase(ctx, db); err != nil {
			return err
		}
		return goose.UpContext(ctx, db, MigrationsDir(t))
	})
}

func MigrateReset(ctx context.Context, t *testing.T, databaseURL string) {
	t.Helper()

	withMigrationDB(ctx, t, databaseURL, func(db *sql.DB) error {
		if err := resetTestSchema(ctx, db); err != nil {
			return err
		}
		if err := prepareTestDatabase(ctx, db); err != nil {
			return err
		}
		return goose.UpContext(ctx, db, MigrationsDir(t))
	})
}

func MigrateDown(ctx context.Context, t *testing.T, databaseURL string) {
	t.Helper()

	withMigrationDB(ctx, t, databaseURL, func(db *sql.DB) error {
		return resetTestSchema(ctx, db)
	})
}

func resetTestSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		DROP SCHEMA IF EXISTS public CASCADE;
		CREATE SCHEMA public;
		GRANT ALL ON SCHEMA public TO public;
	`); err != nil {
		return fmt.Errorf("reset test schema: %w", err)
	}
	return nil
}

func prepareTestDatabase(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE EXTENSION IF NOT EXISTS pgcrypto;
		CREATE OR REPLACE FUNCTION uuidv7() RETURNS uuid AS $$
			SELECT gen_random_uuid();
		$$ LANGUAGE SQL;
	`); err != nil {
		return fmt.Errorf("prepare postgres uuidv7: %w", err)
	}

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	return nil
}

func withMigrationDB(ctx context.Context, t *testing.T, databaseURL string, fn func(*sql.DB) error) {
	t.Helper()

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open migration db: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("close migration db: %v", err)
		}
	}()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping migration db: %v", err)
	}
	if err := fn(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
}

func MigrationsDir(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve caller")
	}
	dir := filepath.Dir(file)
	for {
		candidate := filepath.Join(dir, "db", "migrations")
		if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
			return candidate
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatal(fmt.Errorf("db/migrations not found from %s", file))
		}
		dir = next
	}
}
