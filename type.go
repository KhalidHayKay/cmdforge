package cmdforge

import (
	"context"
	"time"
)

// Command represents a CLI command with an optional destructive flag.
// Destructive commands require the user to confirm before execution.
type Command struct {
	Run         func(ctx context.Context)
	Destructive bool
}

// Migration represents a single database migration with an Up and Down SQL statement.
// Name should be unique and ideally sortable (e.g. "000001_create_users").
type Migration struct {
	Name string
	Up   string
	Down string
}

// Schema mirrors a row in the schema_migrations table.
// Used internally to track which migrations have been applied.
type Schema struct {
	Id        int64
	Name      string
	AppliedAt time.Time
}
