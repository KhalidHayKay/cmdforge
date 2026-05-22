# cmdforge

A lightweight CLI runner and PostgreSQL migration tool for Go projects. Ships with built-in `db:migrate` commands and lets you register any number of custom commands on top.

Built on top of [`pgx/v5`](https://github.com/jackc/pgx).

---

## Install

```bash
go get github.com/khalidhaykay/cmdforge
```

---

## Usage

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/yourusername/cmdforge"
)

var migrations = []cmdforge.Migration{
    {
        Name: "000001_create_users_table",
        Up:   `CREATE TABLE users (id BIGSERIAL PRIMARY KEY, email TEXT NOT NULL UNIQUE);`,
        Down: `DROP TABLE IF EXISTS users CASCADE;`,
    },
}

func main() {
    ctx := context.Background()

    db, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    cli := cmdforge.New(db, migrations)

    // Optional: register your own commands
    cli.Register("seed", func(ctx context.Context) {
        // seed logic
    }, false)

    cli.Start(ctx)
}
```

Build and run:

```bash
go build -o myapp ./cmd/cli
./myapp db:migrate up
```

---

## Built-in commands

These are registered automatically when you call `cmdforge.New()`.

| Command             | Description                                     | Destructive |
| ------------------- | ----------------------------------------------- | ----------- |
| `list`              | Print all available commands                    | No          |
| `db:migrate up`     | Apply all pending migrations                    | No          |
| `db:migrate down`   | Roll back the last 5 applied migrations         | **Yes**     |
| `db:migrate status` | Show applied / pending state for each migration | No          |
| `db:reset`          | Roll back then re-apply all migrations          | **Yes**     |

Destructive commands prompt the user to type `yes` before proceeding.

---

## Custom commands

Use `Register` to add any project-specific command:

```go
cli.Register("seed", seedHandler, false)
cli.Register("db:truncate", truncateHandler, true) // will prompt for confirmation
```

The handler signature is `func(ctx context.Context)`.

---

## Migrations

Define migrations as a `[]cmdforge.Migration` slice in your project. The package ships none — your schema is your business.

```go
cmdforge.Migration{
    Name: "000001_create_users_table", // must be unique; use a sortable prefix
    Up:   `CREATE TABLE ...`,
    Down: `DROP TABLE IF EXISTS ... CASCADE;`,
}
```

Migration state is tracked in a `schema_migrations` table that the package creates automatically on first `db:migrate up`.

**Name uniqueness matters** — the name is the key used to track whether a migration has been applied. Changing a name after it's been applied will cause the runner to treat it as a new, unapplied migration.

---

## Notes

- **PostgreSQL only (v1).** The migration handler is built on `*pgxpool.Pool` and uses Postgres-specific SQL (`$1` placeholders, `SERIAL`). Abstracting this away would be false generality since the migration SQL itself is Postgres-flavored anyway.
- **`db:migrate down` rolls back the last 5** applied migrations, not all of them. This is intentional to avoid accidental full rollbacks. Use `db:reset` if you want a full wipe and re-apply.
- **No file-based migrations.** Migrations live in Go code as strings. This keeps the tool dependency-free and makes migrations part of your binary.

---

## Project structure

```
cmdforge/
├── cli.go        # Public entry point: New(), Register(), Start()
├── cmd.go        # Command router and destructive-op confirmation
├── handler.go    # Built-in db migration command implementations
├── type.go       # Public types: Command, Migration, Schema
├── go.mod
└── example/
    └── main.go   # Example of a consuming project
```
