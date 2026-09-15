# cmdforge

A small in-app CLI and command runner for Go.

Use your application's existing services to run maintenance tasks, seed data, manage users, drain queues, or expose other operational commands without building a separate CLI application.

**`Register` is the primary way to add commands. `Extension` and `Use` provide an optional way to group and compose related commands.**

> cmdforge owns commands and composition. Integrations own their underlying behavior. The consuming application owns resources and configuration.

## Install

```sh
go get github.com/khalidhaykay/cmdforge
```

> **Go version:** This module currently requires Go 1.26 or newer because of the Goose version used by the optional migration integration.

The core `cmdforge` package itself uses only the Go standard library and has no database dependencies.

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

Run the command through your application:

```sh
go run . list
go run . hello
```

`New()` includes a built-in `list` command that prints the registered command names.

That's the basic cmdforge model:

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

Cmdforge performs exact command matching. It does not provide flag or argument parsing.

### Destructive commands

The final argument to `Register` marks a command as destructive:

```go
cli.Register("cache:purge", purgeCache, true)
```

Before executing a destructive command, cmdforge asks the user to type `yes`. Any other response aborts execution.

### Duplicate registrations

Registering an existing command name replaces its current handler and destructive setting.

This allows commands to be deliberately overridden, but also means extensions can replace previously registered commands with the same name. Be deliberate when composing command groups that may share command names.

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
appCommands := UserCommands{
    Create: createUser,
    Delete: deleteUser,
}

cli := cmdforge.New()

cli.Register("cache:clear", clearCache, false)
cli.Use(appCommands)

cli.Start(ctx)
```

`Use` is simply:

```go
func (c *CLI) Use(extension Extension) {
    extension.Register(c)
}
```

There are no lifecycle hooks, initialization phases, dependency resolution, or plugin machinery.

Extensions are optional. Calling `Register` directly is always valid.

Multiple related command groups can be composed in the same way:

```go
cli.Use(userCommands)
cli.Use(queueCommands)
cli.Use(migrator)
```

## Goose migrations

Cmdforge does not implement database migrations itself.

The optional `github.com/khalidhaykay/cmdforge/goose` package integrates [Goose](https://github.com/pressly/goose) with cmdforge, exposing Goose migrations through the same in-app CLI used for the rest of your application's commands.

```text
cmdforge
    │
    └── Use(migrator)
            │
            ▼
      cmdforge/goose
            │
            ▼
      Goose Provider
            │
            ▼
          *sql.DB
```

The contract is intentionally small:

> **You provide the database connection and migration filesystem; cmdforge wires them into Goose commands.**

The application creates and owns its `*sql.DB` and supplies an `fs.FS` containing normal Goose SQL migration files.

The extension does not:

- read your environment or configuration
- open a database connection
- create a connection pool
- close the supplied database
- manage your application's runtime database abstraction

The integration uses Goose's PostgreSQL dialect and accepts a caller-provided `*sql.DB`. The examples below use pgx as the `database/sql` driver.

### PostgreSQL setup

If your application uses pgx:

```sh
go get github.com/jackc/pgx/v5
```

Import its `database/sql` driver:

```go
import _ "github.com/jackc/pgx/v5/stdlib"
```

Then create the migration connection in your application:

```go
db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
if err != nil {
    return err
}
defer db.Close()
```

Your application's normal runtime database access is independent of this.

For example, an application may continue using `pgxpool.Pool` for repositories, queries, and transactions while providing a separate `*sql.DB` to the migration integration.

## SQL migrations

Instead of defining migrations in Go structures, use normal Goose SQL migration files:

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

Goose owns migration discovery, ordering, transactions, version tracking, and SQL execution.

Its default migration version table is `goose_db_version`.

## Embedded migrations

Embedded migrations work particularly well with an in-app CLI because the migration files can ship inside the application binary.

Given:

```text
myapp/
├── main.go
└── migrations/
    └── 00001_create_users.sql
```

embed the files:

```go
//go:embed migrations/*.sql
var migrations embed.FS
```

The Goose extension expects the migration files at the root of the supplied filesystem, so use `fs.Sub`:

```go
migrationFS, err := fs.Sub(migrations, "migrations")
if err != nil {
    return err
}
```

Then construct the migrator:

```go
migrator, err := goosecmd.New(db, migrationFS)
if err != nil {
    return err
}
```

and compose it with cmdforge:

```go
cli := cmdforge.New()

cli.Register("cache:clear", clearCache, false)
cli.Use(migrator)

cli.Start(ctx)
```

A complete setup looks like:

```go
package main

import (
    "context"
    "database/sql"
    "embed"
    "io/fs"
    "log"
    "os"

    _ "github.com/jackc/pgx/v5/stdlib"

    "github.com/khalidhaykay/cmdforge"
    goosecmd "github.com/khalidhaykay/cmdforge/goose"
)

//go:embed migrations/*.sql
var migrations embed.FS

func main() {
    if err := run(context.Background()); err != nil {
        log.Fatal(err)
    }
}

func run(ctx context.Context) error {
    db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
    if err != nil {
        return err
    }
    defer db.Close()

    migrationFS, err := fs.Sub(migrations, "migrations")
    if err != nil {
        return err
    }

    migrator, err := goosecmd.New(db, migrationFS)
    if err != nil {
        return err
    }

    cli := cmdforge.New()

    cli.Register("cache:clear", func(ctx context.Context) {
        log.Println("Clearing cache...")
    }, false)

    cli.Use(migrator)
    cli.Start(ctx)

    return nil
}
```

`goosecmd.New` returns the migrator and any initialization error discovered while preparing the Goose Provider.

The extension accepts `fs.FS`, not a hardcoded directory. This means migration storage is controlled by the application.

For example, you can use:

```go
fs.Sub(embeddedFiles, "migrations")
```

or:

```go
os.DirFS("sql/changes")
```

or any other suitable `fs.FS`.

The directory itself does not need to be named `migrations`.

## Migration commands

The Goose extension registers its commands only when it is explicitly composed with:

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

The integration delegates migration behavior to the Goose Provider rather than implementing its own migration engine.

`db:migrate refresh` is a cmdforge convenience operation that performs a full rollback followed by migration up. It stops if the rollback fails.

Reset and refresh operate only through migration SQL. They do not drop the database or unrelated tables.

## Error and process behavior

Unknown commands and command execution failures are reported as errors and terminate command execution with a non-zero status.

Because process termination does not run deferred functions, application cleanup registered with `defer` is not guaranteed to execute on fatal command failures.

Normal command completion returns control to the application.

## Standalone example

A complete runnable example is available in [`example/`](example/).

It is intentionally a standalone Go module so that the core cmdforge package remains independent of the example application's database choices.

With a PostgreSQL database available:

```sh
cd example

export DATABASE_URL='postgres://postgres:postgres@localhost:5432/myapp?sslmode=disable'

go run . list
go run . db:migrate up
go run . db:migrate status
```

The example uses embedded migrations and pgx's `database/sql` driver.

## Upgrading from the old v0.x API

This release replaces cmdforge's custom PostgreSQL migration engine with the optional Goose integration.

Replace:

```go
cli := cmdforge.New(db, migrations)
```

with:

```go
cli := cmdforge.New()
```

Replace programmatic migration definitions such as:

```go
[]cmdforge.Migration{
    {
        Name: "...",
        Up:   "...",
        Down: "...",
    },
}
```

with numbered Goose SQL migration files containing `-- +goose Up` and `-- +goose Down` sections.

Then explicitly construct and register the Goose integration:

```go
migrator, err := goosecmd.New(db, migrationFS)
if err != nil {
    return err
}

cli.Use(migrator)
```

`Migration`, `Schema`, and cmdforge's custom migration engine have been removed.

Your application may independently continue using `pgxpool` or another database abstraction for its normal runtime access.

### Important: existing migration history

The old `schema_migrations` history is **not automatically converted** into Goose's `goose_db_version` history.

Do not simply run the new Goose migrations against an existing production database that was migrated using the old cmdforge engine.

Existing databases require an application-managed migration/baseline strategy before adopting the new migration integration.

## Development

Run formatting and checks from the repository root:

```sh
gofmt -w *.go goose/*.go
go test ./...
go vet ./...
```

The Goose integration tests exercise the real Goose Provider against a mocked SQL connection and do not require a running PostgreSQL server.

The standalone example can be checked separately:

```sh
cd example

go test ./...
go vet ./...
```
