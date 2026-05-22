package cmdforge

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CLI is the public handle the caller gets from New(). It wraps the command
// router and the migration handler, exposing only what the caller needs:
// Register() to add custom commands, and Start() to run.
type CLI struct {
	cmd *CMD
}

// New creates a CLI instance wired with the built-in db migration commands.
// db is a live pgxpool.Pool — the caller creates and owns it.
// migrations is the caller's own slice of Migration; the package ships none.
func New(db *pgxpool.Pool, migrations []Migration) *CLI {
	h := newHandler(db, migrations)
	cmd := newCMD()
	// Built-in db commands.
	cmd.add("db:migrate up", h.migrateUp, false)
	cmd.add("db:migrate down", h.migrateDown, true)
	cmd.add("db:migrate reset", h.migrateReset, true)
	cmd.add("db:migrate status", h.migrationStatus, false)
	cmd.add("db:reset", h.resetDB, true)

	return &CLI{cmd}
}

// Register adds a custom command to the router. Use this for anything
// project-specific: seeders, one-off scripts, data fixes, etc.
//
//	cli.Register("seed", func(ctx context.Context) {
//	    // your seed logic
//	}, false)
func (c *CLI) Register(name string, fn func(context.Context), destructive bool) {
	c.cmd.add(name, fn, destructive)
}

// Start dispatches on os.Args and runs the matched command.
// Call this at the end of your main() after all Register() calls.
func (c *CLI) Start(ctx context.Context) {
	c.cmd.run(ctx)
}
