package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunSchemaMigrationsRecordsOnlySuccessfulUpgradeAndSkipsRerun(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	applied := 0
	migration := schemaMigration{
		Version:     "test_001",
		Description: "test successful migration",
		Up: func(context.Context) error {
			applied++
			_, execErr := db.ExecContext(ctx, `CREATE TABLE migrated_resource (id TEXT PRIMARY KEY);`)
			return execErr
		},
	}
	if err := runSchemaMigrations(ctx, db, "sqlite", migration); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}
	if err := runSchemaMigrations(ctx, db, "sqlite", migration); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}
	if applied != 1 {
		t.Fatalf("migration applied %d times, want 1", applied)
	}

	var records int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM gos_schema_migration WHERE version = 'test_001';`).Scan(&records); err != nil {
		t.Fatalf("query migration record failed: %v", err)
	}
	if records != 1 {
		t.Fatalf("migration record count = %d, want 1", records)
	}

	wantErr := errors.New("upgrade failed")
	err = runSchemaMigrations(ctx, db, "sqlite", schemaMigration{
		Version:     "test_002",
		Description: "test failed migration",
		Up: func(context.Context) error {
			return wantErr
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("failed migration error = %v, want %v", err, wantErr)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM gos_schema_migration WHERE version = 'test_002';`).Scan(&records); err != nil {
		t.Fatalf("query failed migration record failed: %v", err)
	}
	if records != 0 {
		t.Fatalf("failed migration record count = %d, want 0", records)
	}
}

func TestUserRepositoryInitSchemaUpgradesLegacyDatabase(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE sys_user_session (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		access_token TEXT NOT NULL UNIQUE,
		expired_at INTEGER NOT NULL,
		client_ip TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	);`); err != nil {
		t.Fatalf("create legacy session table failed: %v", err)
	}

	repo := NewUserRepository(db, "sqlite")
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("upgrade legacy user schema failed: %v", err)
	}
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("repeat user schema migration failed: %v", err)
	}

	columns, err := repo.sqliteTableColumns(ctx, "sys_user_session")
	if err != nil {
		t.Fatalf("read session columns failed: %v", err)
	}
	for _, column := range []string{"revoked_at", "revoked_reason"} {
		if _, ok := columns[column]; !ok {
			t.Fatalf("legacy session table missing migrated column %s", column)
		}
	}
	if !sqliteTableExists(t, db, "sys_user_manager") {
		t.Fatal("legacy database missing migrated sys_user_manager table")
	}
	assertSchemaMigrationCount(t, db, "20260717_01_user_manager", 1)
}

func TestReleaseRepositoryInitSchemaUpgradesRecordedLegacyApprovalFlow(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	legacyStatements := []string{
		`CREATE TABLE sys_user (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			phone TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			password_hash TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE release_order_approval_flow_instance (
			id TEXT PRIMARY KEY,
			release_order_id TEXT NOT NULL UNIQUE,
			flow_definition_id TEXT NOT NULL,
			flow_name TEXT NOT NULL,
			flow_snapshot_json TEXT NOT NULL,
			status TEXT NOT NULL,
			current_gate TEXT NOT NULL DEFAULT '',
			current_task_id TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE release_order_approval_flow_task (
			id TEXT PRIMARY KEY,
			instance_id TEXT NOT NULL,
			release_order_id TEXT NOT NULL,
			node_code TEXT NOT NULL,
			node_name TEXT NOT NULL,
			gate TEXT NOT NULL,
			approval_mode TEXT NOT NULL,
			approver_ids_json TEXT NOT NULL,
			approver_names_json TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE release_order_deploy_snapshot (
			id TEXT PRIMARY KEY,
			release_order_id TEXT NOT NULL UNIQUE,
			provider TEXT NOT NULL DEFAULT '',
			gitops_type TEXT NOT NULL DEFAULT '',
			argocd_instance_id TEXT NOT NULL DEFAULT '',
			gitops_instance_id TEXT NOT NULL DEFAULT '',
			argocd_app_name TEXT NOT NULL DEFAULT '',
			repo_url TEXT NOT NULL DEFAULT '',
			branch TEXT NOT NULL DEFAULT '',
			source_path TEXT NOT NULL DEFAULT '',
			env_code TEXT NOT NULL DEFAULT '',
			snapshot_payload_json TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);`,
	}
	for _, statement := range legacyStatements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("create legacy approval schema failed: %v", err)
		}
	}
	if err := ensureSchemaMigrationTable(ctx, db, "sqlite"); err != nil {
		t.Fatalf("create legacy migration table failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO gos_schema_migration (version, description, applied_at)
		VALUES ('20260717_02_release_approval_flow', 'early v1.3 approval flow schema', 1);`); err != nil {
		t.Fatalf("record legacy approval migration failed: %v", err)
	}

	repo := NewReleaseRepository(db, "sqlite")
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("upgrade legacy release schema failed: %v", err)
	}
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("repeat release schema migration failed: %v", err)
	}

	instanceColumns, err := repo.sqliteTableColumns(ctx, "release_order_approval_flow_instance")
	if err != nil {
		t.Fatalf("read approval instance columns failed: %v", err)
	}
	for _, column := range []string{"current_scope", "current_node_code"} {
		if _, ok := instanceColumns[column]; !ok {
			t.Fatalf("legacy approval instance missing migrated column %s", column)
		}
	}
	taskColumns, err := repo.sqliteTableColumns(ctx, "release_order_approval_flow_task")
	if err != nil {
		t.Fatalf("read approval task columns failed: %v", err)
	}
	for _, column := range []string{"node_type", "agent_task_id", "agent_task_name", "agent_batch_id", "message"} {
		if _, ok := taskColumns[column]; !ok {
			t.Fatalf("legacy approval task missing migrated column %s", column)
		}
	}
	for _, table := range []string{
		"release_approval_flow_definition",
		"release_order_approval_flow_task_record",
		"release_application_approval_flow_binding",
	} {
		if !sqliteTableExists(t, db, table) {
			t.Fatalf("legacy database missing migrated table %s", table)
		}
	}
	for _, values := range [][]any{
		{"snapshot-1", "release-1", "argocd-1", "app-1"},
		{"snapshot-2", "release-1", "argocd-2", "app-2"},
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO release_order_deploy_snapshot (
			id, release_order_id, argocd_instance_id, argocd_app_name, snapshot_payload_json, created_at
		) VALUES (?, ?, ?, ?, '{}', 1);`, values...); err != nil {
			t.Fatalf("insert multi-instance release snapshot failed: %v", err)
		}
	}
	assertSchemaMigrationCount(t, db, "20260717_02_release_approval_flow", 1)
	assertSchemaMigrationCount(t, db, "20260718_01_release_approval_flow_runtime_columns", 1)
}

func TestPlatformParamRepositoryInitSchemaUpgradesLegacyColumns(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE platform_param_dict (
		id TEXT PRIMARY KEY,
		param_key TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		description TEXT NOT NULL,
		param_type TEXT NOT NULL,
		required INTEGER NOT NULL,
		builtin INTEGER NOT NULL,
		status INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);`); err != nil {
		t.Fatalf("create legacy platform parameter table failed: %v", err)
	}

	repo := NewPlatformParamRepository(db, "sqlite")
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("upgrade legacy platform parameter schema failed: %v", err)
	}
	columns, err := repo.sqliteTableColumns(ctx, "platform_param_dict")
	if err != nil {
		t.Fatalf("read platform parameter columns failed: %v", err)
	}
	for _, column := range []string{"gitops_locator", "cd_self_fill"} {
		if _, ok := columns[column]; !ok {
			t.Fatalf("legacy platform parameter table missing migrated column %s", column)
		}
	}
	assertSchemaMigrationCount(t, db, "deploy_platform_v1_1_platform_param", 1)
}

func TestArgoCDRepositoryInitSchemaUpgradesLegacyEnvironmentUniqueIndex(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE argocd_env_binding (
		id TEXT PRIMARY KEY,
		env_code TEXT NOT NULL UNIQUE,
		argocd_instance_id TEXT NOT NULL,
		priority INTEGER NOT NULL DEFAULT 1,
		status TEXT NOT NULL DEFAULT 'active',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);`); err != nil {
		t.Fatalf("create legacy ArgoCD environment binding table failed: %v", err)
	}

	repo := NewArgoCDApplicationRepository(db, "sqlite")
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("upgrade legacy ArgoCD schema failed: %v", err)
	}
	for _, values := range [][]any{
		{"binding-1", "prod", "argocd-1", 1, "active", int64(1), int64(1)},
		{"binding-2", "prod", "argocd-2", 2, "active", int64(1), int64(1)},
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO argocd_env_binding (
			id, env_code, argocd_instance_id, priority, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?);`, values...); err != nil {
			t.Fatalf("insert multi-instance environment binding failed: %v", err)
		}
	}
	assertSchemaMigrationCount(t, db, "deploy_platform_v1_2_argocd_multi_instance", 1)
}

func sqliteTableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?;`, table).Scan(&count); err != nil {
		t.Fatalf("query table %s failed: %v", table, err)
	}
	return count == 1
}

func assertSchemaMigrationCount(t *testing.T, db *sql.DB, version string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM gos_schema_migration WHERE version = ?;`, version).Scan(&count); err != nil {
		t.Fatalf("query migration %s failed: %v", version, err)
	}
	if count != want {
		t.Fatalf("migration %s count = %d, want %d", version, count, want)
	}
}
