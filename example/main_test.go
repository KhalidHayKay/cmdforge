package main

import (
	"io/fs"
	"testing"

	"github.com/khalidhaykay/cmdforge/example/migrations"
	goosecmd "github.com/khalidhaykay/cmdforge/goose"
)

func TestEmbeddedMigrations(t *testing.T) {
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected two embedded migrations, got %v", files)
	}
	if _, err := goosecmd.New("postgres://localhost/example?sslmode=disable", migrations.FS); err != nil {
		t.Fatal(err)
	}
}
