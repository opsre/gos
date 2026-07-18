package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

const schemaMigrationLockName = "gos_deploy_platform_schema_migration"

type schemaMigration struct {
	Version     string
	Description string
	Up          func(context.Context) error
}

// runSchemaMigrations applies immutable, versioned upgrades that cannot be expressed by
// CREATE TABLE IF NOT EXISTS alone. A version is recorded only after its upgrade succeeds.
func runSchemaMigrations(
	ctx context.Context,
	db *sql.DB,
	dbDriver string,
	migrations ...schemaMigration,
) error {
	if len(migrations) == 0 {
		return nil
	}
	dbDriver = strings.ToLower(strings.TrimSpace(dbDriver))
	releaseLock, err := acquireSchemaMigrationLock(ctx, db, dbDriver)
	if err != nil {
		return err
	}
	defer releaseLock()

	if err := ensureSchemaMigrationTable(ctx, db, dbDriver); err != nil {
		return err
	}
	for _, migration := range migrations {
		migration.Version = strings.TrimSpace(migration.Version)
		migration.Description = strings.TrimSpace(migration.Description)
		if migration.Version == "" || migration.Up == nil {
			return errors.New("schema migration requires a version and upgrade function")
		}
		applied, err := schemaMigrationApplied(ctx, db, migration.Version)
		if err != nil {
			return fmt.Errorf("check schema migration %s: %w", migration.Version, err)
		}
		if applied {
			continue
		}
		log.Printf("applying database schema migration %s: %s", migration.Version, migration.Description)
		if err := migration.Up(ctx); err != nil {
			return fmt.Errorf("apply schema migration %s (%s): %w", migration.Version, migration.Description, err)
		}
		if _, err := db.ExecContext(
			ctx,
			`INSERT INTO gos_schema_migration (version, description, applied_at) VALUES (?, ?, ?);`,
			migration.Version,
			migration.Description,
			time.Now().UTC().UnixNano(),
		); err != nil {
			return fmt.Errorf("record schema migration %s: %w", migration.Version, err)
		}
		log.Printf("database schema migration %s applied", migration.Version)
	}
	return nil
}

func ensureSchemaMigrationTable(ctx context.Context, db *sql.DB, dbDriver string) error {
	var statement string
	switch dbDriver {
	case "mysql":
		statement = `CREATE TABLE IF NOT EXISTS gos_schema_migration (
	version VARCHAR(128) PRIMARY KEY,
	description VARCHAR(500) NOT NULL DEFAULT '',
	applied_at BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`
	case "sqlite":
		statement = `CREATE TABLE IF NOT EXISTS gos_schema_migration (
	version TEXT PRIMARY KEY,
	description TEXT NOT NULL DEFAULT '',
	applied_at INTEGER NOT NULL
);`
	default:
		return fmt.Errorf("unsupported db driver: %s", dbDriver)
	}
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("create schema migration table: %w", err)
	}
	return nil
}

func schemaMigrationApplied(ctx context.Context, db *sql.DB, version string) (bool, error) {
	var found string
	err := db.QueryRowContext(
		ctx,
		`SELECT version FROM gos_schema_migration WHERE version = ?;`,
		strings.TrimSpace(version),
	).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func acquireSchemaMigrationLock(ctx context.Context, db *sql.DB, dbDriver string) (func(), error) {
	if dbDriver != "mysql" {
		return func() {}, nil
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open schema migration lock connection: %w", err)
	}
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, 30);`, schemaMigrationLockName).Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("acquire schema migration lock: %w", err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		_ = conn.Close()
		return nil, errors.New("timed out waiting for schema migration lock")
	}
	return func() {
		var released sql.NullInt64
		_ = conn.QueryRowContext(context.Background(), `SELECT RELEASE_LOCK(?);`, schemaMigrationLockName).Scan(&released)
		_ = conn.Close()
	}, nil
}
