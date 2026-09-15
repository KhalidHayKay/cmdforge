package main

import (
	"database/sql"
	"io/fs"
	"testing"

	goosecmd "github.com/khalidhaykay/cmdforge/goose"
)

func TestEmbeddedMigrations(t *testing.T) {
	migrationFS, err := fs.Sub(migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	files, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected two embedded migrations, got %v", files)
	}
	// sql.Open is lazy: constructing the extension needs no running server.
	db, err := sql.Open("pgx", "postgres://localhost/example?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := goosecmd.New(db, migrationFS); err != nil {
		t.Fatal(err)
	}
}
