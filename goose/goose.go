// Package goose provides optional PostgreSQL migration commands for cmdforge.
// Applications supply configuration and migration files. New owns its database;
// NewWithDB borrows a caller-owned database.
package goose

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/khalidhaykay/cmdforge"
	pressly "github.com/pressly/goose/v3"
)

// Migrator registers commands backed by a Goose Provider.
// Construct it with New or NewWithDB.
type Migrator struct {
	provider     *pressly.Provider
	openProvider func() (*pressly.Provider, error)
}

var _ cmdforge.Extension = (*Migrator)(nil)

// New creates a PostgreSQL extension from a connection string and migration files.
// It validates configuration and discovers migrations without contacting the server.
// Each command opens its own pool and closes it before returning, including on
// failure. Listing commands and declining confirmation open no pool. No Close
// call is required. SQL files must be at the root of migrations.
func New(databaseURL string, migrations fs.FS) (*Migrator, error) {
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	return newOwned(func() (*sql.DB, error) { return stdlib.OpenDB(*config.Copy()), nil }, migrations)
}

func newOwned(openDB func() (*sql.DB, error), migrations fs.FS) (*Migrator, error) {
	openProvider := func() (*pressly.Provider, error) {
		db, err := openDB()
		if err != nil {
			return nil, err
		}
		m, err := NewWithDB(db, migrations)
		if err != nil {
			return nil, errors.Join(err, db.Close())
		}
		return m.provider, nil
	}
	// Validate eagerly, but retain no idle pool or background database goroutine.
	provider, err := openProvider()
	if err != nil {
		return nil, err
	}
	if err := provider.Close(); err != nil {
		return nil, err
	}
	return &Migrator{openProvider: openProvider}, nil
}

// withProvider completes cleanup before command reports fatal errors (os.Exit
// does not run defers). Refresh shares one provider for rollback and reapply.
func (m *Migrator) withProvider(run func(*Migrator) error) (err error) {
	if m.provider != nil {
		return run(m)
	}
	provider, err := m.openProvider()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, provider.Close()) }()
	return run(&Migrator{provider: provider})
}

// NewWithDB creates a PostgreSQL migration extension using SQL files at the root of
// migrations. Use fs.Sub to select a directory within an embedded filesystem.
// Construction discovers migrations; database access and SQL parsing occur when
// commands run. Goose's global Go-migration registry is disabled.
// The caller owns db; this extension never closes it.
func NewWithDB(db *sql.DB, migrations fs.FS) (*Migrator, error) {
	provider, err := pressly.NewProvider(pressly.DialectPostgres, db, migrations,
		pressly.WithDisableGlobalRegistry(true))
	if err != nil {
		return nil, err
	}
	return &Migrator{provider: provider}, nil
}

// Register adds migration commands to cli. Rollback and reset commands use
// cmdforge's destructive confirmation. Execution errors are fatal, consistent
// with cmdforge's command-line error handling.
func (m *Migrator) Register(cli *cmdforge.CLI) {
	cli.Register("db:migrate up", command(m.up), false)
	cli.Register("db:migrate down", command(m.down), true)
	cli.Register("db:migrate status", command(m.status), false)
	cli.Register("db:migrate reset", command(m.rollback), true)
	cli.Register("db:migrate refresh", command(m.reset), true)
}

func command(run func(context.Context) error) func(context.Context) {
	return func(ctx context.Context) {
		if err := run(ctx); err != nil {
			log.Fatal(err)
		}
	}
}

func (m *Migrator) up(ctx context.Context) error {
	return m.withProvider(func(m *Migrator) error {
		_, err := m.provider.Up(ctx)
		return err
	})
}

func (m *Migrator) down(ctx context.Context) error {
	return m.withProvider(func(m *Migrator) error {
		_, err := m.provider.Down(ctx)
		return err
	})
}

func (m *Migrator) rollback(ctx context.Context) error {
	return m.withProvider(func(m *Migrator) error {
		_, err := m.provider.DownTo(ctx, 0)
		return err
	})
}

func (m *Migrator) reset(ctx context.Context) error {
	return m.withProvider(func(m *Migrator) error {
		if err := m.rollback(ctx); err != nil {
			return err
		}
		return m.up(ctx)
	})
}

func (m *Migrator) status(ctx context.Context) error {
	return m.withProvider(func(m *Migrator) error {
		statuses, err := m.provider.Status(ctx)
		if err != nil {
			return err
		}
		for _, status := range statuses {
			fmt.Printf("%s\t%s\n", status.State, status.Source.Path)
		}
		return nil
	})
}
