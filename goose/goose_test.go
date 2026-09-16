package goose

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/khalidhaykay/cmdforge"
	pressly "github.com/pressly/goose/v3"
)

var migrationFiles = fstest.MapFS{
	"00001_users.sql": {Data: []byte("-- +goose Up\nCREATE TABLE users (id int);\n-- +goose Down\nDROP TABLE users;\n")},
	"00002_posts.sql": {Data: []byte("-- +goose Up\nCREATE TABLE posts (id int);\n-- +goose Down\nDROP TABLE posts;\n")},
}

func newTestMigrator(t *testing.T) (*Migrator, *sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		mock.ExpectClose()
		if err := db.Close(); err != nil {
			t.Error(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	migrator, err := NewWithDB(db, migrationFiles)
	if err != nil {
		t.Fatal(err)
	}
	return migrator, db, mock
}

func TestNewErrors(t *testing.T) {
	if m, err := NewWithDB(nil, migrationFiles); err == nil || m != nil {
		t.Fatal("expected nil database error")
	}
	_, db, _ := newTestMigrator(t)
	for _, files := range []fs.FS{nil, fstest.MapFS{}, fstest.MapFS{
		"00001_a.sql": {Data: []byte("-- +goose Up\nSELECT 1;")},
		"00001_b.sql": {Data: []byte("-- +goose Up\nSELECT 2;")},
	}} {
		if m, err := NewWithDB(db, files); err == nil || m != nil {
			t.Fatal("expected migration discovery error")
		}
	}
	_, err := NewWithDB(db, os.DirFS(t.TempDir()+"/missing"))
	if !errors.Is(err, pressly.ErrNoMigrations) {
		t.Fatalf("expected Goose discovery error, got %v", err)
	}
}

// Run through the public CLI so registration, routing and confirmation are exercised.
func runCLI(t *testing.T, m *Migrator, input, response string) string {
	t.Helper()
	oldArgs, oldIn, oldOut := os.Args, os.Stdin, os.Stdout
	stdin, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	if _, err := stdin.WriteString(response); err != nil {
		t.Fatal(err)
	}
	if _, err := stdin.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	os.Args = append([]string{"app"}, strings.Fields(input)...)
	os.Stdin, os.Stdout = stdin, stdout
	defer func() { os.Args, os.Stdin, os.Stdout = oldArgs, oldIn, oldOut }()
	cli := cmdforge.New()
	cli.Use(m)
	cli.Start(context.Background())
	if _, err := stdout.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(stdout)
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
}

func TestRegistrationAndDestructiveCancellation(t *testing.T) {
	m, db, mock := newTestMigrator(t)
	want := "Available commands:\n- db:migrate down\n- db:migrate refresh\n- db:migrate reset\n- db:migrate status\n- db:migrate up\n"
	if got := runCLI(t, m, "list", ""); got != want {
		t.Fatalf("list = %q", got)
	}
	for _, name := range []string{"db:migrate down", "db:migrate reset", "db:migrate refresh"} {
		if got := runCLI(t, m, name, "no\n"); !strings.Contains(got, "Aborted.") {
			t.Fatalf("%s: %q", name, got)
		}
	}
	// No database access should occur during construction, listing or cancellation.
	mock.ExpectPing()
	if err := db.Ping(); err != nil {
		t.Fatalf("caller database unusable: %v", err)
	}
}

func expectTable(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
}

func expectVersions(mock sqlmock.Sqlmock, versions ...int) {
	rows := sqlmock.NewRows([]string{"version_id", "is_applied"})
	for _, version := range versions {
		rows.AddRow(version, true)
	}
	mock.ExpectQuery("SELECT version_id, is_applied").WillReturnRows(rows)
}

func expectMigration(mock sqlmock.Sqlmock, table string, version int, up bool) {
	mock.ExpectBegin()
	if up {
		mock.ExpectExec("CREATE TABLE " + table).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("INSERT INTO goose_db_version").WithArgs(int64(version), true).WillReturnResult(sqlmock.NewResult(0, 1))
	} else {
		mock.ExpectExec("DROP TABLE " + table).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM goose_db_version").WithArgs(int64(version)).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
}

func TestProviderCommands(t *testing.T) {
	for _, command := range []string{"db:migrate up", "db:migrate down", "db:migrate reset", "db:migrate refresh"} {
		t.Run(command, func(t *testing.T) {
			m, db, mock := newTestMigrator(t)
			expectTable(mock)
			if command == "db:migrate up" {
				expectVersions(mock, 0) // HasPending
				expectVersions(mock, 0) // Up
				expectMigration(mock, "users", 1, true)
				expectMigration(mock, "posts", 2, true)
				expectLatest(mock, 2)
			} else {
				expectVersions(mock, 2, 1, 0)
				expectMigration(mock, "posts", 2, false)
				if command != "db:migrate down" {
					expectMigration(mock, "users", 1, false)
					expectLatest(mock, 0)
				}
				if command == "db:migrate refresh" {
					expectVersions(mock, 0)
					expectVersions(mock, 0)
					expectMigration(mock, "users", 1, true)
					expectMigration(mock, "posts", 2, true)
					expectLatest(mock, 2)
				}
			}
			runCLI(t, m, command, "yes\n")
			mock.ExpectPing()
			if err := db.Ping(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	m, _, mock := newTestMigrator(t)
	expectTable(mock)
	mock.ExpectQuery("SELECT tstamp, is_applied").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"tstamp", "is_applied"}).AddRow(time.Now(), true))
	mock.ExpectQuery("SELECT tstamp, is_applied").WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"tstamp", "is_applied"}))
	if got := runCLI(t, m, "db:migrate status", ""); got != "applied\t00001_users.sql\npending\t00002_posts.sql\n" {
		t.Fatalf("status = %q", got)
	}
}

func TestResetStopsOnRollbackError(t *testing.T) {
	m, _, mock := newTestMigrator(t)
	expectTable(mock)
	expectVersions(mock, 2, 1, 0)
	failure := errors.New("rollback failed")
	mock.ExpectBegin()
	mock.ExpectExec("DROP TABLE posts").WillReturnError(failure)
	mock.ExpectRollback()
	if err := m.reset(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("reset = %v", err)
	}
	// Any attempt to reapply after rollback failure is an unexpected query.
}

func TestContextAndEmptyRollback(t *testing.T) {
	t.Run("context", func(t *testing.T) {
		m, _, _ := newTestMigrator(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		for _, run := range []func(context.Context) error{m.up, m.down, m.status, m.rollback, m.reset} {
			if err := run(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("expected cancellation, got %v", err)
			}
		}
	})
	t.Run("no previous version", func(t *testing.T) {
		m, _, mock := newTestMigrator(t)
		expectTable(mock)
		expectVersions(mock, 0)
		if err := m.down(context.Background()); !errors.Is(err, pressly.ErrNoNextVersion) {
			t.Fatalf("down = %v", err)
		}
	})
}

func expectLatest(mock sqlmock.Sqlmock, version int) {
	mock.ExpectQuery("SELECT max").WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(version))
}

func TestURLConstructor(t *testing.T) {
	m, err := New("postgres://localhost/example?sslmode=disable", migrationFiles)
	if err != nil {
		t.Fatal(err)
	}
	if m.provider != nil || m.openProvider == nil {
		t.Fatal("constructor retained an owned pool")
	}
	runCLI(t, m, "list", "")
	runCLI(t, m, "db:migrate refresh", "no\n")
	if _, err := New("postgres://localhost:invalid/example", migrationFiles); err == nil {
		t.Fatal("expected invalid URL error")
	}
	if _, err := New("postgres://localhost/example", fstest.MapFS{}); err == nil {
		t.Fatal("expected provider construction error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.up(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("up = %v", err)
	}
}

func TestOwnedLifecycle(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			var dbs []*sql.DB
			opens := 0
			failure := errors.New("query failed")
			openDB := func() (*sql.DB, error) {
				db, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				dbs = append(dbs, db)
				opens++
				if opens > 1 {
					if fail {
						mock.ExpectQuery("SELECT EXISTS").WillReturnError(failure)
					} else {
						expectTable(mock)
						expectVersions(mock, 2, 1, 0)
					}
				}
				mock.ExpectClose()
				t.Cleanup(func() {
					if err := mock.ExpectationsWereMet(); err != nil {
						t.Error(err)
					}
				})
				return db, nil
			}
			m, err := newOwned(openDB, migrationFiles)
			if err != nil {
				t.Fatal(err)
			}
			runCLI(t, m, "list", "")
			runCLI(t, m, "db:migrate reset", "no\n")
			if opens != 1 {
				t.Fatal("listing or cancellation opened a pool")
			}
			for i := 0; i < 2; i++ {
				err := m.up(context.Background())
				if fail && !errors.Is(err, failure) || !fail && err != nil {
					t.Fatalf("up = %v", err)
				}
			}
			if opens != 3 {
				t.Fatalf("opens = %d", opens)
			}
			for _, db := range dbs {
				if err := db.Ping(); err == nil || !strings.Contains(err.Error(), "closed") {
					t.Fatalf("pool not closed: %v", err)
				}
			}
		})
	}
}

func TestOwnedConstructionCleanup(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectClose()
	if _, err := newOwned(func() (*sql.DB, error) { return db, nil }, fstest.MapFS{}); !errors.Is(err, pressly.ErrNoMigrations) {
		t.Fatalf("New = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("open failed")
	if _, err := newOwned(func() (*sql.DB, error) { return nil, failure }, migrationFiles); !errors.Is(err, failure) {
		t.Fatalf("New = %v", err)
	}
}

func TestOwnedRefreshSharesPool(t *testing.T) {
	opens := 0
	m, err := newOwned(func() (*sql.DB, error) {
		db, mock, err := sqlmock.New()
		if err != nil {
			return nil, err
		}
		opens++
		if opens > 1 {
			expectTable(mock)
			expectVersions(mock, 2, 1, 0)
			expectMigration(mock, "posts", 2, false)
			expectMigration(mock, "users", 1, false)
			expectLatest(mock, 0)
			expectVersions(mock, 0)
			expectVersions(mock, 0)
			expectMigration(mock, "users", 1, true)
			expectMigration(mock, "posts", 2, true)
			expectLatest(mock, 2)
		}
		mock.ExpectClose()
		t.Cleanup(func() {
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Error(err)
			}
		})
		return db, nil
	}, migrationFiles)
	if err != nil {
		t.Fatal(err)
	}
	runCLI(t, m, "db:migrate refresh", "yes\n")
	if opens != 2 {
		t.Fatalf("refresh opened multiple pools: %d", opens)
	}
}

func TestOwnedCloseError(t *testing.T) {
	failure := errors.New("close failed")
	_, err := newOwned(func() (*sql.DB, error) {
		db, mock, err := sqlmock.New()
		if err != nil {
			return nil, err
		}
		mock.ExpectClose().WillReturnError(failure)
		t.Cleanup(func() {
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Error(err)
			}
		})
		return db, nil
	}, migrationFiles)
	if !errors.Is(err, failure) {
		t.Fatalf("New = %v", err)
	}
}
