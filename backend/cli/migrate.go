package cli

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"transx/internal/platform/config"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/urfave/cli/v2"
)

func MigrateUp(c *cli.Context) error {
	db, err := openMigrateDB(c)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := goose.Up(db, filepath.Join("db", "migrations")); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	fmt.Println("Migrations applied successfully")
	return nil
}

func MigrateDown(c *cli.Context) error {
	db, err := openMigrateDB(c)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := goose.Down(db, filepath.Join("db", "migrations")); err != nil {
		return fmt.Errorf("migrate down: %w", err)
	}
	fmt.Println("Last migration rolled back")
	return nil
}

func MigrateStatus(c *cli.Context) error {
	db, err := openMigrateDB(c)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return goose.Status(db, filepath.Join("db", "migrations"))
}

func openMigrateDB(c *cli.Context) (*sql.DB, error) {
	cfg, err := config.Load(c.String("config"))
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("pgx", cfg.Postgres.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return db, nil
}
