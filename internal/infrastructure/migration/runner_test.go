package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestDefaultMigrationsApplyToEmptyDatabase(t *testing.T) {
	db := openTestDatabase(t)
	runner := NewRunner(DefaultMigrations())

	if err := runner.Apply(context.Background(), db); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	var version int
	var name, appliedChecksum, appliedAt string
	err := db.QueryRow(`
SELECT version, name, checksum, applied_at
FROM t_schema_migrations`).Scan(&version, &name, &appliedChecksum, &appliedAt)
	if err != nil {
		t.Fatalf("read migration record: %v", err)
	}
	defaultMigration := DefaultMigrations()[0]
	if version != defaultMigration.Version || name != defaultMigration.Name {
		t.Fatalf("migration record = (%d, %q), want (%d, %q)", version, name, defaultMigration.Version, defaultMigration.Name)
	}
	if appliedChecksum != checksum(defaultMigration.SQL) {
		t.Fatalf("checksum = %q, want %q", appliedChecksum, checksum(defaultMigration.SQL))
	}
	if parsed, err := time.Parse(time.RFC3339Nano, appliedAt); err != nil || parsed.Location() != time.UTC {
		t.Fatalf("applied_at = %q, want UTC RFC3339Nano", appliedAt)
	}

	tables := tableNames(t, db)
	if got, want := fmt.Sprint(tables), "[t_schema_migrations]"; got != want {
		t.Fatalf("tables = %s, want %s", got, want)
	}
}

func TestApplySkipsAlreadyAppliedMigrations(t *testing.T) {
	db := openTestDatabase(t)
	runner := NewRunner(DefaultMigrations())

	if err := runner.Apply(context.Background(), db); err != nil {
		t.Fatalf("first Apply() error = %v", err)
	}
	if err := runner.Apply(context.Background(), db); err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM t_schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migration record count = %d, want 1", count)
	}
}

func TestApplyAddsSecondMigrationInItsOwnVersion(t *testing.T) {
	db := openTestDatabase(t)
	migrations := append(DefaultMigrations(), Migration{
		Version: 2,
		Name:    "create_second_table",
		SQL:     "CREATE TABLE t_second (id INTEGER NOT NULL)",
	})

	if err := NewRunner(migrations).Apply(context.Background(), db); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM t_schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("migration record count = %d, want 2", count)
	}
	if !hasTable(t, db, "t_second") {
		t.Fatal("second migration table was not created")
	}
}

func TestApplyRollsBackFailedMigration(t *testing.T) {
	db := openTestDatabase(t)
	migrations := append(DefaultMigrations(), Migration{
		Version: 2,
		Name:    "failed_second_migration",
		SQL:     "CREATE TABLE t_rollback (id INTEGER); INSERT INTO missing_table VALUES (1)",
	})

	err := NewRunner(migrations).Apply(context.Background(), db)
	if err == nil {
		t.Fatal("Apply() error = nil, want failed migration")
	}
	if hasTable(t, db, "t_rollback") {
		t.Fatal("failed migration left its table behind")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM t_schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migration record count = %d, want 1", count)
	}
}

func TestApplyRejectsMigrationDrift(t *testing.T) {
	tests := []struct {
		name  string
		query string
		value string
	}{
		{name: "name", query: "UPDATE t_schema_migrations SET name = ? WHERE version = 1", value: "renamed"},
		{name: "checksum", query: "UPDATE t_schema_migrations SET checksum = ? WHERE version = 1", value: "changed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := openTestDatabase(t)
			runner := NewRunner(DefaultMigrations())
			if err := runner.Apply(context.Background(), db); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(test.query, test.value); err != nil {
				t.Fatal(err)
			}

			err := runner.Apply(context.Background(), db)
			if !errors.Is(err, ErrMigrationDrift) {
				t.Fatalf("Apply() error = %v, want ErrMigrationDrift", err)
			}
		})
	}
}

func TestApplyRejectsUnknownAppliedMigration(t *testing.T) {
	db := openTestDatabase(t)
	runner := NewRunner(DefaultMigrations())
	if err := runner.Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO t_schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
		99,
		"future_migration",
		"checksum",
		"2026-09-04T00:00:00Z",
	); err != nil {
		t.Fatal(err)
	}

	err := runner.Apply(context.Background(), db)
	if !errors.Is(err, ErrUnknownAppliedMigration) {
		t.Fatalf("Apply() error = %v, want ErrUnknownAppliedMigration", err)
	}
}

func TestNewRunnerRejectsInvalidMigrationVersions(t *testing.T) {
	tests := []struct {
		name       string
		migrations []Migration
	}{
		{name: "zero", migrations: []Migration{{Version: 0, Name: "zero", SQL: "SELECT 1"}}},
		{name: "negative", migrations: []Migration{{Version: -1, Name: "negative", SQL: "SELECT 1"}}},
		{
			name:       "duplicate",
			migrations: []Migration{{Version: 1, Name: "one", SQL: "SELECT 1"}, {Version: 1, Name: "duplicate", SQL: "SELECT 1"}},
		},
		{
			name:       "not increasing",
			migrations: []Migration{{Version: 2, Name: "two", SQL: "SELECT 1"}, {Version: 1, Name: "one", SQL: "SELECT 1"}},
		},
		{name: "empty name", migrations: []Migration{{Version: 1, Name: " ", SQL: "SELECT 1"}}},
		{name: "empty SQL", migrations: []Migration{{Version: 1, Name: "one", SQL: "\n\t"}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := openTestDatabase(t)
			err := NewRunner(test.migrations).Apply(context.Background(), db)
			if !errors.Is(err, ErrInvalidMigration) {
				t.Fatalf("Apply() error = %v, want ErrInvalidMigration", err)
			}
		})
	}
}

func TestDefaultMigrationsReturnsCopy(t *testing.T) {
	first := DefaultMigrations()
	first[0].Name = "changed"
	second := DefaultMigrations()
	if second[0].Name == "changed" {
		t.Fatal("DefaultMigrations() returned shared storage")
	}
}

func openTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	return db
}

func tableNames(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`
SELECT name
FROM sqlite_master
WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

func hasTable(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		name,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count > 0
}
