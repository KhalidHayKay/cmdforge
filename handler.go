package cmdforge

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

// handler holds the dependencies needed to run migration commands.
// Previously this was CLIHandler in the original project — renamed to handler
// since it's now an internal type; the caller never touches it directly.
type handler struct {
	db         *pgxpool.Pool
	migrations []Migration
}

func newHandler(db *pgxpool.Pool, migrations []Migration) *handler {
	return &handler{db, migrations}
}

// migrateUp creates the schema_migrations tracking table if it doesn't exist,
// then runs any migrations that haven't been applied yet, in order.
func (h *handler) migrateUp(ctx context.Context) {
	_, err := h.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id SERIAL PRIMARY KEY,
			name TEXT UNIQUE NOT NULL,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		log.Fatalf("Schema migration table creation failed: %s", err)
	}

	var isUpToDate bool = true

	for _, m := range h.migrations {
		var exists bool

		err := h.db.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name=$1)`,
			m.Name,
		).Scan(&exists)
		if err != nil {
			log.Fatal(err)
		}

		if exists {
			continue
		}

		_, err = h.db.Exec(ctx, m.Up)
		if err != nil {
			log.Fatalf("Migration failed: %s\n%v", m.Name, err)
		}

		_, err = h.db.Exec(ctx,
			`INSERT INTO schema_migrations (name) VALUES ($1)`,
			m.Name,
		)
		if err != nil {
			log.Fatal(err)
		}

		isUpToDate = false

		log.Printf("Applied migration: %s", m.Name)
	}

	if isUpToDate {
		log.Println("Migration up to date")
	}
}

// migrateDown rolls back the last 5 applied migrations (matching the LIMIT 5
// in getSchemas). Runs in reverse-applied order since getSchemas returns DESC.
func (h *handler) migrateDown(ctx context.Context) {
	schemas, err := h.getSchemas(ctx)
	if err != nil {
		log.Fatalf("Schema migrations fetch error: %s", err)
	}

	if len(schemas) < 1 {
		log.Println("No migrations to rollback.")
		return
	}

	// Build a lookup map so we can find the Down SQL for each applied migration.
	// This replaces the direct access to the package-level migrations var.
	migrationMap := make(map[string]Migration)
	for _, mig := range h.migrations {
		migrationMap[mig.Name] = mig
	}

	for _, s := range schemas {
		target, ok := migrationMap[s.Name]

		if !ok {
			log.Fatalf("Migration %s not found in code", s.Name)
		}

		_, err = h.db.Exec(ctx, target.Down)
		if err != nil {
			log.Fatalf("Rollback failed: %s\n%v", target.Name, err)
		}

		_, err = h.db.Exec(ctx,
			`DELETE FROM schema_migrations WHERE name=$1`,
			target.Name,
		)
		if err != nil {
			log.Fatal(err)
		}

		log.Printf("Rolled back migration: %s", target.Name)
	}
}

func (h *handler) migrationStatus(ctx context.Context) {
	schemas, err := h.getSchemas(ctx)
	if err != nil {
		log.Fatalf("Schema migrations fetch err: %s", err)
	}

	appliedMap := make(map[string]Schema)
	for _, s := range schemas {
		appliedMap[s.Name] = s
	}

	for _, m := range h.migrations {
		if _, exists := appliedMap[m.Name]; exists {
			log.Printf("✅ %s..........APPLIED", m.Name)
		} else {
			log.Printf("⏳ %s..........PENDING", m.Name)
		}
	}
}

// resetDB is a convenience command: rolls back then re-applies all migrations.
// Registered as a destructive command so the router prompts for confirmation.
func (h *handler) resetDB(ctx context.Context) {
	log.Println("Migrating down...")
	h.migrateDown(ctx)
	log.Println("Migrating up...")
	h.migrateUp(ctx)
}

// seedDB is intentionally left as a no-op placeholder. Seeding is too
// project-specific to ship in the package; callers should register their
// own "seed" command via CLI.Register instead.
func (h *handler) seedDB() {
	fmt.Println("No seed logic implemented. Register your own seed command via CLI.Register.")
}

// getSchemas fetches the last 5 applied migrations from schema_migrations.
// LIMIT 5 means migrateDown only rolls back the last 5 — intentional to
// avoid accidentally nuking everything. Adjust if you need full rollbacks.
func (h *handler) getSchemas(ctx context.Context) (schemas []Schema, err error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, name, applied_at
		FROM schema_migrations
		ORDER BY id DESC
		LIMIT 5
	`)
	if err != nil {
		return
	}

	defer rows.Close()

	for rows.Next() {
		var schema Schema
		if err = rows.Scan(
			&schema.Id,
			&schema.Name,
			&schema.AppliedAt,
		); err != nil {
			return
		}
		schemas = append(schemas, schema)
	}

	err = rows.Err()

	return
}
