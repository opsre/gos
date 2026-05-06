package sqlrepo

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// TestPlatformParamRepositoryInitSchemaSyncsBuiltinAppKeyDescription 同步外部或内部状态数据。
func TestPlatformParamRepositoryInitSchemaSyncsBuiltinAppKeyDescription(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() {
		_ = db.Close()
	})

	repo := NewPlatformParamRepository(db, "sqlite")
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	item, err := repo.GetByParamKey(context.Background(), "app_key")
	if err != nil {
		t.Fatalf("GetByParamKey failed: %v", err)
	}

	const want = ""
	if item.Description != want {
		t.Fatalf("app_key description = %q, want %q", item.Description, want)
	}
}
