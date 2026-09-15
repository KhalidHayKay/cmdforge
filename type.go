package cmdforge

import "context"

// Command represents a CLI command with an optional destructive flag.
// Destructive commands require the user to confirm before execution.
type Command struct {
	Run         func(ctx context.Context)
	Destructive bool
}

// Extension optionally groups related commands by registering them on a CLI.
type Extension interface {
	Register(*CLI)
}
