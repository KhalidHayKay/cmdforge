// Package goose provides optional PostgreSQL migration commands for cmdforge.
// The application owns the database connection and migration filesystem.
package goose

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log"

	"github.com/khalidhaykay/cmdforge"
	pressly "github.com/pressly/goose/v3"
)

// Migrator registers commands backed by a Goose Provider.
// Construct it with New. It never closes the caller's database.
type Migrator struct {
	provider *pressly.Provider
}

var _ cmdforge.Extension = (*Migrator)(nil)

// New creates a PostgreSQL migration extension using SQL files at the root of
// migrations. Use fs.Sub to select a directory within an embedded filesystem.
// Construction discovers migrations; database access and SQL parsing occur when
// commands run. Goose's global Go-migration registry is disabled.
func New(db *sql.DB, migrations fs.FS) (*Migrator, error) {
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
	_, err := m.provider.Up(ctx)
	return err
}

func (m *Migrator) down(ctx context.Context) error {
	_, err := m.provider.Down(ctx)
	return err
}

func (m *Migrator) rollback(ctx context.Context) error {
	_, err := m.provider.DownTo(ctx, 0)
	return err
}

func (m *Migrator) reset(ctx context.Context) error {
	if err := m.rollback(ctx); err != nil {
		return err
	}
	return m.up(ctx)
}

func (m *Migrator) status(ctx context.Context) error {
	statuses, err := m.provider.Status(ctx)
	if err != nil {
		return err
	}
	for _, status := range statuses {
		fmt.Printf("%s\t%s\n", status.State, status.Source.Path)
	}
	return nil
}
