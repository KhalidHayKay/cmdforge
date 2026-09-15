package main

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/khalidhaykay/cmdforge"
	goosecmd "github.com/khalidhaykay/cmdforge/goose"
)

//go:embed migrations/*.sql
var migrations embed.FS

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	log.Println(os.Getenv("DATABASE_URL"))
	// Configuration and connection ownership belong to this application.
	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	migrationFS, err := fs.Sub(migrations, "migrations")
	if err != nil {
		log.Fatal(err)
	}

	migrator, err := goosecmd.New(db, migrationFS)
	if err != nil {
		log.Fatal(err)
	}

	cli := cmdforge.New()

	cli.Register("cache:clear", func(ctx context.Context) {
		log.Println("Clearing the application cache...")
		// Call your application's cache service here.
	}, false)

	cli.Use(migrator)

	cli.Start(context.Background())
}
