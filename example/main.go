package main

import (
	"context"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/khalidhaykay/cmdforge"
	"github.com/khalidhaykay/cmdforge/example/migrations"
	goosecmd "github.com/khalidhaykay/cmdforge/goose"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	migrator, err := goosecmd.New(os.Getenv("DATABASE_URL"), migrations.FS)
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
