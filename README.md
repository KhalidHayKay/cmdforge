# cmdforge

A small in-app CLI and command runner for Go.

Use your application's existing services to run maintenance tasks, seed data, manage users, drain queues, or expose other operational commands without building a separate CLI application.

**`Register` is the primary way to add commands. `Extension` and `Use` provide an optional way to group and compose related commands.**

> cmdforge owns commands and composition. Extensions own their integrations.

## Install

```sh
go get github.com/khalidhaykay/cmdforge
```

The core module requires **Go 1.22 or newer** and uses only the Go standard library.

Optional integrations are distributed independently and do not add their dependencies or Go version requirements to cmdforge core.

## Quick start

```go
package main

import (
    "context"
    "fmt"

    "github.com/khalidhaykay/cmdforge"
)

func main() {
    cli := cmdforge.New()

    cli.Register("hello", func(ctx context.Context) {
        fmt.Println("Hello from cmdforge")
    }, false)

    cli.Start(context.Background())
}
```

Run commands through your application:

```sh
go run . list
go run . hello
```

`New()` includes a built-in `list` command that prints the registered command names.

The basic cmdforge model is:

```text
New
 ↓
Register
 ↓
Start
```

## Registering commands

`Register` is the primary API for adding commands:

```go
cli := cmdforge.New()

cli.Register("cache:clear", func(ctx context.Context) {
    cache.Clear(ctx)
}, false)

cli.Register("user:create", func(ctx context.Context) {
    users.Create(ctx)
}, false)

cli.Start(ctx)
```

Handlers have the signature:

```go
func(context.Context)
```

and receive the context passed to `Start`.

Command names may contain spaces:

```go
cli.Register("queue drain", drainQueue, false)
```

cmdforge performs exact command matching. It does not provide flag or argument parsing.

### Destructive commands

The final argument to `Register` marks a command as destructive:

```go
cli.Register("cache:purge", purgeCache, true)
```

Before executing a destructive command, cmdforge asks the user to type `yes`. Any other response aborts execution.

### Duplicate registrations

Registering an existing command name replaces its current handler and destructive setting.

This allows deliberate overrides, but also means an extension can replace a previously registered command with the same name.

## Grouping commands with extensions

For larger applications, related commands can optionally be grouped into an `Extension`.

An extension is deliberately small:

```go
type Extension interface {
    Register(*CLI)
}
```

For example:

```go
type UserCommands struct {
    Create func(context.Context)
    Delete func(context.Context)
}

func (u UserCommands) Register(cli *cmdforge.CLI) {
    cli.Register("user:create", u.Create, false)
    cli.Register("user:delete", u.Delete, true)
}
```

Then compose it with the CLI:

```go
userCommands := UserCommands{
    Create: createUser,
    Delete: deleteUser,
}

cli := cmdforge.New()

cli.Register("cache:clear", clearCache, false)
cli.Use(userCommands)

cli.Start(ctx)
```

`Use` simply asks an extension to register its commands:

```go
func (c *CLI) Use(extension Extension) {
    extension.Register(c)
}
```

There are no lifecycle hooks, dependency resolution, or plugin machinery.

Extensions are optional. Calling `Register` directly is always valid.

Multiple command groups and integrations can be composed in the same application:

```go
cli.Use(userCommands)
cli.Use(queueCommands)
cli.Use(migrator)
```

## Goose extension

`github.com/khalidhaykay/cmdforge/goose` is an independent optional module that exposes PostgreSQL migration commands through cmdforge using Goose.

cmdforge core does not depend on Goose, pgx, or any migration engine.

Install the extension separately:

```sh
go get github.com/khalidhaykay/cmdforge/goose
```

The Goose extension currently requires **Go 1.26 or newer** and targets PostgreSQL.

### Basic usage

The common API accepts a PostgreSQL database URL and an `fs.FS` containing your migrations:

```go
migrator, err := goosecmd.New(databaseURL, migrations.FS)
if err != nil {
    return err
}

cli := cmdforge.New()
cli.Use(migrator)

cli.Start(ctx)
```

`New` manages the database resources required by migration commands internally. Applications do not need to import a SQL driver, create a separate `*sql.DB`, or close migration resources.

Your application's normal database setup remains independent. For example, the application can continue using `pgxpool` for runtime queries while the Goose extension manages its own migration connections.

### Using an existing `*sql.DB`

Applications that need explicit connection ownership can use `NewWithDB`:

```go
migrator, err := goosecmd.NewWithDB(db, migrations.FS)
if err != nil {
    return err
}
```

With `NewWithDB`, the caller owns `db` and remains responsible for closing it. The extension never closes a caller-provided database.

Both constructors use PostgreSQL and expose the same migration commands.

## SQL migrations

The Goose extension uses standard Goose SQL migration files:

```text
migrations/
├── 00001_create_users.sql
├── 00002_create_accounts.sql
└── 00003_create_transactions.sql
```

For example:

```sql
-- +goose Up

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE
);

-- +goose Down

DROP TABLE users;
```

Goose owns migration discovery, ordering, execution, transactions, and version tracking.

Its default migration version table is `goose_db_version`.

## Embedded migrations

Embedded migrations work particularly well with an in-app CLI because the migration files ship with the application binary.

A dedicated migrations package keeps the files independent of your CLI package:

```text
application/
├── cmd/
│   └── cli/
│       └── main.go
└── migrations/
    ├── embed.go
    ├── 00001_create_users.sql
    └── 00002_create_posts.sql
```

In `migrations/embed.go`:

```go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

Then consume that filesystem from your CLI:

```go
package main

import (
    "context"
    "log"
    "os"

    "application/migrations"

    "github.com/khalidhaykay/cmdforge"
    goosecmd "github.com/khalidhaykay/cmdforge/goose"
)

func main() {
    migrator, err := goosecmd.New(
        os.Getenv("DATABASE_URL"),
        migrations.FS,
    )
    if err != nil {
        log.Fatal(err)
    }

    cli := cmdforge.New()

    cli.Register("cache:clear", func(ctx context.Context) {
        log.Println("Clearing cache...")
    }, false)

    cli.Use(migrator)
    cli.Start(context.Background())
}
```

Replace `application/migrations` with your application's module path.

The extension accepts `fs.FS`, so migrations do not have to be embedded. You can also use `os.DirFS(...)`, `fs.Sub(...)`, or another `fs.FS` implementation.

Migration SQL files must be at the root of the filesystem supplied to the extension. Use `fs.Sub` when selecting a nested migration directory from an existing filesystem.

## Migration commands

The Goose extension registers its commands when composed with:

```go
cli.Use(migrator)
```

| Command              | Behavior                                          | Confirmation |
| -------------------- | ------------------------------------------------- | ------------ |
| `db:migrate up`      | Apply all pending migrations                      | No           |
| `db:migrate down`    | Roll back the most recently applied migration     | Yes          |
| `db:migrate status`  | Show migration status                             | No           |
| `db:migrate reset`   | Roll back all applied migrations                  | Yes          |
| `db:migrate refresh` | Roll back all applied migrations and reapply them | Yes          |

`db:migrate refresh` is a cmdforge convenience command that performs a full rollback followed by migration up. If rollback fails, migration up is not attempted.

Reset and refresh operate through your migration SQL. They do not drop the database or unrelated tables.

## Error and process behavior

Unknown commands and command execution failures terminate command execution with a non-zero status.

Because process termination does not execute deferred functions, cleanup registered with `defer` is not guaranteed to run after a fatal command failure.

Normal command completion returns control to the application.

## Standalone example

A complete runnable example is available in [`example/`](example/).

The example is a standalone Go module and demonstrates:

- core command registration
- the independent Goose extension
- PostgreSQL migrations
- a dedicated embedded-migrations package
- the database URL constructor

With a PostgreSQL database available:

```sh
cd example

export DATABASE_URL='postgres://postgres:postgres@localhost:5432/myapp?sslmode=disable'

go run . list
go run . db:migrate up
go run . db:migrate status
```

## Upgrading from v0.1.x

cmdforge v0.2 removes the custom PostgreSQL migration engine from core.

Applications that need migrations can opt into the independent Goose extension.

Replace:

```go
cli := cmdforge.New(db, migrations)
```

with:

```go
cli := cmdforge.New()
```

The core command APIs remain:

```go
cli.Register(...)
cli.Use(...)
cli.Start(...)
```

### Migrating to Goose

Programmatic cmdforge migrations such as:

```go
[]cmdforge.Migration{
    {
        Name: "...",
        Up:   "...",
        Down: "...",
    },
}
```

should be replaced with numbered Goose SQL migration files containing `-- +goose Up` and `-- +goose Down` sections.

Then construct and register the Goose extension:

```go
migrator, err := goosecmd.New(databaseURL, migrationFS)
if err != nil {
    return err
}

cli.Use(migrator)
```

`Migration`, `Schema`, and cmdforge's custom migration engine have been removed.

Your application can continue using `pgxpool` or another database abstraction independently for normal runtime access.

### Existing databases

The legacy cmdforge migration engine and Goose maintain separate migration histories.

The old `schema_migrations` history is **not automatically converted** into Goose's `goose_db_version` history.

Do not run a new set of initial Goose migrations against an existing production database as though the database were empty. Existing databases require an application-specific migration or baselining strategy that reconciles the existing schema with Goose's migration state.

Your existing schema and data do not need to be deleted to adopt the Goose extension.

cmdforge performs no automatic migration-history conversion.

## Development

The repository contains three Go modules:

```text
.
├── go.mod          # cmdforge core
├── goose/
│   └── go.mod      # Goose extension
└── example/
    └── go.mod      # standalone example
```

`go.work` connects them for local repository development.

Core requires Go 1.22 or newer. Working across the Goose extension and example requires Go 1.26 or newer.

Run checks for each module:

```sh
# Core
go test ./...
go vet ./...

# Goose extension
cd goose
go test ./...
go vet ./...

# Example
cd ../example
go test ./...
go vet ./...
go build ./...
```

The Goose tests use mocked SQL connections and do not require a running PostgreSQL server.

## License

cmdforge is licensed under the [MIT License](LICENSE).
