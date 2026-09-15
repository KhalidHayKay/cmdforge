package cmdforge

import "context"

// CLI registers and runs in-app commands. Create one with New.
type CLI struct {
	cmd *CMD
}

// New creates a CLI with the built-in list command and no external dependencies.
func New() *CLI {
	return &CLI{cmd: newCMD()}
}

// Register is the primary way to add commands. Use this for anything
// project-specific: seeders, one-off scripts, data fixes, etc.
// Registering an existing name replaces its handler and destructive flag.
//
//	cli.Register("seed", func(ctx context.Context) {
//	    // your seed logic
//	}, false)
func (c *CLI) Register(name string, fn func(context.Context), destructive bool) {
	c.cmd.add(name, fn, destructive)
}

// Use registers an optional group of commands through an extension.
func (c *CLI) Use(extension Extension) {
	extension.Register(c)
}

// Start dispatches on os.Args and runs the matched command.
// Call this after registering commands and extensions.
func (c *CLI) Start(ctx context.Context) {
	c.cmd.run(ctx)
}
