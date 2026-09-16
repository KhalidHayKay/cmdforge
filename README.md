# cmdforge

A small in-app CLI and command runner for Go.

Use your application's existing services to run maintenance tasks, seed data, manage users, drain queues, or expose other operational commands without building a separate CLI application.

**`Register` is the primary way to add commands. `Extension` and `Use` provide an optional way to group and compose related commands.**

> cmdforge owns commands and composition. Extensions own their integrations. Applications own configuration and resources unless an extension explicitly offers a convenience API that owns a resource internally.

## Install

```sh
go get github.com/khalidhaykay/cmdforge
```

> **Go versions:** Core declares Go 1.22 (its existing standard-library code and tests need no newer features). The Goose extension and example require Go 1.26 because Goose v3.28.0 requires it.

The core module uses only the Go standard library. Installing core does not bring Goose, pgx, or any database dependencies into its module graph. cmdforge is a command runner, not a migration framework.

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

## Goose extension

The optional, independent `github.com/khalidhaykay/cmdforge/goose` module integrates [Goose](https://github.com/pressly/goose) through `Extension.Register(*CLI)`. Core depends on no migration engine. The extension currently targets PostgreSQL.

Once the module split is released, install the extension with:

```sh
go get github.com/khalidhaykay/cmdforge/goose
```

For this unreleased checkout, use the repository workspace; see the release-order limitation under Development.

The common API accepts an application-provided database URL and an `fs.FS`:

```go
migrator, err := goosecmd.New(databaseURL, migrations.FS)
if err != nil {
    return err
}
cli.Use(migrator)
```

The extension uses pgx's stdlib adapter internally. It validates the connection configuration and discovers migrations at construction without contacting PostgreSQL. It retains no open pool between commands. Each executed migration command creates a fresh pool and Goose Provider and closes the pool on success or failure before reporting fatal errors. Refresh uses one pool for both rollback and reapply. Listing commands or declining confirmation needs no database connection. Repeated command execution is supported; no `defer migrator.Close()` is necessary.

Database connectivity and SQL parsing errors are discovered during command execution. Migration files are rediscovered for each command, so an on-disk filesystem may reflect changes since construction. The extension does not read application configuration or manage application runtime database access.

For advanced caller-owned connections:

```go
migrator, err := goosecmd.NewWithDB(db, migrations.FS)
```

The caller creates and owns `db` and is responsible for closing it. `NewWithDB` never closes it, even when construction or execution fails. This API also uses PostgreSQL. Both constructors share the same Provider configuration and command behavior; Goose's global Go-migration registry is disabled.

Other migration engines can be independent extensions/modules alongside Goose, without changing core or existing Goose consumers. Engine-specific history and any engine switch or baselining remain application concerns.

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

Prefer a dedicated resource package because Go embed patterns are package-relative:

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

`migrations/embed.go`:

```go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

`cmd/cli/main.go` (replace `application` with your module path):

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
    migrator, err := goosecmd.New(os.Getenv("DATABASE_URL"), migrations.FS)
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

The API accepts `fs.FS`, including `embed.FS`, `os.DirFS(...)`, `fs.Sub(...)`, and custom implementations. SQL files must be at the supplied filesystem's root. Use `fs.Sub` when selecting a nested directory in an existing embedded filesystem.

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

The example uses a dedicated embedded-migrations package and the URL constructor; it needs no SQL or driver imports. It loads configuration from a `.env` file.

## Upgrading from the old v0.x API

Core no longer contains the legacy PostgreSQL migration engine. Applications can opt into the independent Goose extension.

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
migrator, err := goosecmd.New(databaseURL, migrationFS)
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

Existing databases require an application-managed migration/baseline strategy before adopting Goose. Preserve the existing schema and data: deleting the schema is not a required upgrade step. Reconcile the already-applied schema with Goose version state through a strategy reviewed for your application. This extension performs no automatic history conversion.

RC.1 callers of `goosecmd.New(db, migrationFS)` should use `NewWithDB(db, migrationFS)` to retain caller ownership, or switch to `New(databaseURL, migrationFS)` for internal resource management. The core `New`, `Register`, `Use`, and `Start` APIs are unchanged.

## Development

The repository contains three modules: core (`.`), the optional Goose extension (`goose/`), and the standalone example (`example/`). `go.work` selects all three for local development. Go 1.26 or newer is required to work across them. Core can be checked independently with `GOWORK=off` on its own minimum Go version.

Run formatting, tests, and vet separately for every module:

```sh
gofmt -w *.go
go test ./...
go vet ./...

(cd goose && gofmt -w *.go && go test ./... && go vet ./...)
(cd example && gofmt -w *.go migrations/*.go && go test ./... && go vet ./... && go build ./...)
```

Goose tests exercise the real Provider with mocked SQL connections and test URL configuration without an external PostgreSQL server. The example has local replacements for both repository modules so it can also be checked with `GOWORK=off`; these are example-development settings. The Goose module has no local replacement.

### Release-order limitation

The Goose module currently records the real core version `v0.2.0-rc.1`. That tag still contains the old `goose` package, so using it with the new Goose module outside the workspace produces an ambiguous import. **This checkout is not ready to publish unchanged.** First release the split core (whose module archive excludes the nested Goose module), then update `goose/go.mod` to require that real version and validate with `GOWORK=off`. Release the extension using a `goose/`-prefixed version tag, and update the example to the released versions. The example's provisional Goose version is resolved locally until that release exists. No release or tag is created by this refactor.
