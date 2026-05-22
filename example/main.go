package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/khalidhaykay/cmdforge"
)

// migrations is your project's own migration list.
// The package ships none — this is intentional; every project defines its own schema.
var migrations = []cmdforge.Migration{
	{
		Name: "000001_create_users_table",
		Up: `CREATE TABLE users (
			id BIGSERIAL PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		Down: `DROP TABLE IF EXISTS users CASCADE;`,
	},
	{
		Name: "000002_create_posts_table",
		Up: `CREATE TABLE posts (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			body TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		Down: `DROP TABLE IF EXISTS posts CASCADE;`,
	},
}

func main() {
	ctx := context.Background()

	godotenv.Load()

	// You own the db connection. The package doesn't care how you build it —
	// env var, config file, secrets manager, whatever your project uses.
	db, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	cli := cmdforge.New(db, migrations)

	// Register any project-specific commands on top of the built-in ones.
	cli.Register("seed", func(ctx context.Context) {
		log.Println("Seeding database...")
		// your seed logic here
	}, false)

	cli.Register("db:truncate", func(ctx context.Context) {
		log.Println("Truncating all tables...")
		// your truncate logic here
	}, true) // destructive = true means it will prompt for confirmation

	cli.Start(ctx)
}
