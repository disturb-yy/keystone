// Package migration 提供 Keystone 本机 SQLite 的增量 Migration runner。
package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	schemaMigrationsTable     = "t_schema_migrations"
	createSchemaMigrationsSQL = `
CREATE TABLE t_schema_migrations (
	version INTEGER PRIMARY KEY NOT NULL,
	name TEXT NOT NULL,
	checksum TEXT NOT NULL,
	applied_at TEXT NOT NULL
)`
)

var (
	// ErrMigrationDrift 表示已应用 Migration 的名称或 SQL 校验和发生变化。
	ErrMigrationDrift = errors.New("migration drift detected")
	// ErrUnknownAppliedMigration 表示数据库记录了当前 runner 不认识的版本。
	ErrUnknownAppliedMigration = errors.New("unknown applied migration")
	// ErrInvalidMigration 表示 Migration 版本不是正整数递增序列。
	ErrInvalidMigration = errors.New("invalid migration")
)

// Migration 描述一个只向前应用的数据库变更。
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// Runner 按版本顺序将 Migration 应用到调用方提供的数据库连接。
type Runner struct {
	migrations []Migration
}

type appliedMigration struct {
	Version  int
	Name     string
	Checksum string
}

var defaultMigrations = []Migration{
	{
		Version: 1,
		Name:    "create_schema_migrations",
		SQL:     createSchemaMigrationsSQL,
	},
}

// NewRunner 创建一个持有 Migration 副本的 runner。
func NewRunner(migrations []Migration) Runner {
	return Runner{migrations: cloneMigrations(migrations)}
}

// DefaultMigrations 返回 Keystone 当前内置 Migration 的副本。
func DefaultMigrations() []Migration {
	return cloneMigrations(defaultMigrations)
}

// Apply 校验并按顺序应用尚未记录的 Migration。
//
// 每个 Migration 都在独立事务中执行；调用方负责在外部持有本机状态锁。
func (r Runner) Apply(ctx context.Context, db *sql.DB) error {
	if ctx == nil {
		return errors.New("apply migrations: nil context")
	}
	if db == nil {
		return errors.New("apply migrations: nil database")
	}
	if err := validateMigrations(r.migrations); err != nil {
		return fmt.Errorf("validate migrations: %w", err)
	}

	hasTable, err := migrationTableExists(ctx, db)
	if err != nil {
		return fmt.Errorf("inspect migration table: %w", err)
	}

	var applied []appliedMigration
	if hasTable {
		applied, err = readAppliedMigrations(ctx, db)
		if err != nil {
			return fmt.Errorf("read applied migrations: %w", err)
		}
	}
	if err := validateAppliedMigrations(applied, r.migrations); err != nil {
		return err
	}

	for _, migration := range r.migrations {
		if isApplied(applied, migration.Version) {
			continue
		}
		if err := applyMigration(ctx, db, migration); err != nil {
			return err
		}
	}
	return nil
}

func cloneMigrations(migrations []Migration) []Migration {
	return append([]Migration(nil), migrations...)
}

func validateMigrations(migrations []Migration) error {
	previousVersion := 0
	for index, migration := range migrations {
		if migration.Version <= 0 {
			return fmt.Errorf("%w: version %d must be positive", ErrInvalidMigration, migration.Version)
		}
		if strings.TrimSpace(migration.Name) == "" {
			return fmt.Errorf("%w: version %d name must not be empty", ErrInvalidMigration, migration.Version)
		}
		if strings.TrimSpace(migration.SQL) == "" {
			return fmt.Errorf("%w: version %d SQL must not be empty", ErrInvalidMigration, migration.Version)
		}
		if index > 0 && migration.Version <= previousVersion {
			return fmt.Errorf(
				"%w: version %d must be greater than %d",
				ErrInvalidMigration,
				migration.Version,
				previousVersion,
			)
		}
		previousVersion = migration.Version
	}
	return nil
}

func migrationTableExists(ctx context.Context, db *sql.DB) (bool, error) {
	const query = `
SELECT COUNT(*)
FROM sqlite_master
WHERE type = 'table' AND name = ?`

	var count int
	if err := db.QueryRowContext(ctx, query, schemaMigrationsTable).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func readAppliedMigrations(ctx context.Context, db *sql.DB) ([]appliedMigration, error) {
	const query = `
SELECT version, name, checksum
FROM t_schema_migrations
ORDER BY version`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var migrations []appliedMigration
	for rows.Next() {
		var migration appliedMigration
		if err := rows.Scan(&migration.Version, &migration.Name, &migration.Checksum); err != nil {
			return nil, err
		}
		migrations = append(migrations, migration)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return migrations, nil
}

func validateAppliedMigrations(applied []appliedMigration, configured []Migration) error {
	configuredByVersion := make(map[int]Migration, len(configured))
	for _, migration := range configured {
		configuredByVersion[migration.Version] = migration
	}
	for _, appliedMigration := range applied {
		migration, ok := configuredByVersion[appliedMigration.Version]
		if !ok {
			return fmt.Errorf("%w: version %d", ErrUnknownAppliedMigration, appliedMigration.Version)
		}
		if appliedMigration.Name != migration.Name || appliedMigration.Checksum != checksum(migration.SQL) {
			return fmt.Errorf("%w: version %d", ErrMigrationDrift, appliedMigration.Version)
		}
	}
	return nil
}

func isApplied(applied []appliedMigration, version int) bool {
	for _, migration := range applied {
		if migration.Version == version {
			return true
		}
	}
	return false
}

func applyMigration(ctx context.Context, db *sql.DB, migration Migration) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d (%s): %w", migration.Version, migration.Name, err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback migration %d (%s): %w", migration.Version, migration.Name, rollbackErr))
		}
	}()

	if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
		return fmt.Errorf("execute migration %d (%s): %w", migration.Version, migration.Name, err)
	}
	const insertQuery = `
INSERT INTO t_schema_migrations (version, name, checksum, applied_at)
VALUES (?, ?, ?, ?)`
	if _, err := tx.ExecContext(
		ctx,
		insertQuery,
		migration.Version,
		migration.Name,
		checksum(migration.SQL),
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record migration %d (%s): %w", migration.Version, migration.Name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d (%s): %w", migration.Version, migration.Name, err)
	}
	committed = true
	return nil
}

func checksum(sqlText string) string {
	digest := sha256.Sum256([]byte(sqlText))
	return hex.EncodeToString(digest[:])
}
