package cmdforge

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// handler holds the dependencies needed to run migration commands.
type handler struct {
	db         *pgxpool.Pool
	migrations []Migration
}

func newHandler(pgsql *pgxpool.Pool, migrations []Migration) *handler {
	return &handler{pgsql, migrations}
}

// migrateUp creates the schema_migrations tracking table if it doesn't exist,
// then runs any migrations that haven't been applied yet, in order.
func (h *handler) migrateUp(ctx context.Context) {
	h.createSchema(ctx)

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

// migrateDown rolls back the most recently applied migrations.
func (h *handler) migrateDown(ctx context.Context) {
	limit := 1

	schemas := h.getSchemas(ctx, &limit)

	if len(schemas) == 0 {
		log.Println("No migrations to rollback.")
		return
	}

	s := schemas[0]

	// Lookup map to find the Down SQL for each applied migration.
	migrationMap := make(map[string]Migration)
	for _, mig := range h.migrations {
		migrationMap[mig.Name] = mig
	}

	target, ok := migrationMap[s.Name]
	if !ok {
		log.Fatalf("Migration %s not found in code", s.Name)
	}

	_, err := h.db.Exec(ctx, target.Down)
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

// migrateDown rolls back all applied migrations.
func (h *handler) migrateReset(ctx context.Context) {
	schemas := h.getSchemas(ctx, nil)

	if len(schemas) < 1 {
		log.Println("No migrations to rollback.")
		return
	}

	// Lookup map to find the Down SQL for each applied migration.
	migrationMap := make(map[string]Migration)
	for _, mig := range h.migrations {
		migrationMap[mig.Name] = mig
	}

	for _, s := range schemas {
		target, ok := migrationMap[s.Name]

		if !ok {
			log.Fatalf("Migration %s not found in code", s.Name)
		}

		_, err := h.db.Exec(ctx, target.Down)
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
	schemas := h.getSchemas(ctx, nil)

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
	h.migrateReset(ctx)
	log.Println("Migrating up...")
	h.migrateUp(ctx)
}

// getSchemas fetches the lastest applied migrations from schema_migrations depending of the limit passed.
// if linit is set to nil, the all applied migration (in order of applied_at) are returned
func (h *handler) getSchemas(ctx context.Context, limit *int) []Schema {
	h.createSchema(ctx)

	baseQuery := `
		SELECT id, name, applied_at
		FROM schema_migrations
		ORDER BY id DESC
	`

	var rows pgx.Rows
	var err error

	if limit != nil {
		rows, err = h.db.Query(ctx, baseQuery+" LIMIT $1", *limit)
	} else {
		rows, err = h.db.Query(ctx, baseQuery)
	}

	if err != nil {
		log.Fatalf("Query build error while creating schame migration table: %s", err)
	}
	defer rows.Close()

	var schemas []Schema
	for rows.Next() {
		var schema Schema
		if err = rows.Scan(&schema.Id, &schema.Name, &schema.AppliedAt); err != nil {
			log.Fatalf("Schema migrations fetch err: %s", err)

		}
		schemas = append(schemas, schema)
	}

	if rows.Err() != nil {
		log.Fatalf("Schema migrations fetch err: %s", rows.Err())
	}

	return schemas
}

func (h *handler) createSchema(ctx context.Context) {
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
}
