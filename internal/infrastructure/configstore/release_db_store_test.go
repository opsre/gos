package configstore

import (
	"context"
	"database/sql"
	"testing"

	"gos/internal/application/usecase"

	_ "modernc.org/sqlite"
)

// TestDatabaseReleaseStoreFallbackAndPersistence 封装当前模块的业务处理逻辑。
func TestDatabaseReleaseStoreFallbackAndPersistence(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	store := NewDatabaseReleaseStore(db, "sqlite", releaseDBStoreStub{
		envOptions: []string{"dev", "test", "prod"},
		concurrency: usecase.ReleaseConcurrencySettingsOutput{
			Enabled:          true,
			LockScope:        usecase.ReleaseConcurrencyLockScopeApplicationEnv,
			ConflictStrategy: usecase.ReleaseConcurrencyConflictStrategyReject,
			LockTimeoutSec:   1800,
		},
	})
	ctx := context.Background()
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	options, err := store.LoadEnvOptions(ctx)
	if err != nil {
		t.Fatalf("LoadEnvOptions fallback failed: %v", err)
	}
	if len(options) != 3 || options[1] != "test" {
		t.Fatalf("fallback env options = %#v, want [dev test prod]", options)
	}

	if err := store.SaveEnvOptions(ctx, []string{"dev", "prod", "prod"}); err != nil {
		t.Fatalf("SaveEnvOptions failed: %v", err)
	}
	if err := store.SaveConcurrencySettings(ctx, usecase.ReleaseConcurrencySettingsInput{
		Enabled:          true,
		LockScope:        usecase.ReleaseConcurrencyLockScopeGitOpsRepoBranch,
		ConflictStrategy: usecase.ReleaseConcurrencyConflictStrategyQueue,
		LockTimeoutSec:   600,
	}); err != nil {
		t.Fatalf("SaveConcurrencySettings failed: %v", err)
	}

	reloadedOptions, err := store.LoadEnvOptions(ctx)
	if err != nil {
		t.Fatalf("LoadEnvOptions persisted failed: %v", err)
	}
	if len(reloadedOptions) != 2 || reloadedOptions[0] != "dev" || reloadedOptions[1] != "prod" {
		t.Fatalf("persisted env options = %#v, want [dev prod]", reloadedOptions)
	}
	if err := store.SaveEnvOptions(ctx, nil); err != nil {
		t.Fatalf("SaveEnvOptions empty failed: %v", err)
	}
	emptyOptions, err := store.LoadEnvOptions(ctx)
	if err != nil {
		t.Fatalf("LoadEnvOptions empty failed: %v", err)
	}
	if len(emptyOptions) != 0 {
		t.Fatalf("empty env options = %#v, want empty", emptyOptions)
	}

	reloadedConcurrency, err := store.LoadConcurrencySettings(ctx)
	if err != nil {
		t.Fatalf("LoadConcurrencySettings persisted failed: %v", err)
	}
	if !reloadedConcurrency.Enabled || reloadedConcurrency.LockScope != usecase.ReleaseConcurrencyLockScopeGitOpsRepoBranch || reloadedConcurrency.ConflictStrategy != usecase.ReleaseConcurrencyConflictStrategyQueue || reloadedConcurrency.LockTimeoutSec != 600 {
		t.Fatalf("persisted concurrency = %#v, want updated values", reloadedConcurrency)
	}
}

type releaseDBStoreStub struct {
	envOptions  []string
	concurrency usecase.ReleaseConcurrencySettingsOutput
}

// LoadEnvOptions 封装当前模块的业务处理逻辑。
func (s releaseDBStoreStub) LoadEnvOptions(context.Context) ([]string, error) {
	return append([]string(nil), s.envOptions...), nil
}

// SaveEnvOptions 封装当前模块的业务处理逻辑。
func (s releaseDBStoreStub) SaveEnvOptions(context.Context, []string) error {
	return nil
}

// LoadConcurrencySettings 封装当前模块的业务处理逻辑。
func (s releaseDBStoreStub) LoadConcurrencySettings(context.Context) (usecase.ReleaseConcurrencySettingsOutput, error) {
	return s.concurrency, nil
}

// SaveConcurrencySettings 封装当前模块的业务处理逻辑。
func (s releaseDBStoreStub) SaveConcurrencySettings(context.Context, usecase.ReleaseConcurrencySettingsInput) error {
	return nil
}

// LoadGitOpsConfig 封装当前模块的业务处理逻辑。
func (s releaseDBStoreStub) LoadGitOpsConfig(context.Context) (usecase.ReleaseGitOpsConfigOutput, error) {
	return usecase.ReleaseGitOpsConfigOutput{
		HelmScanPath:      "apps/helm",
		KustomizeScanPath: "apps/{app_key}/overlays/{env}",
	}, nil
}

// SaveGitOpsConfig 封装当前模块的业务处理逻辑。
func (s releaseDBStoreStub) SaveGitOpsConfig(context.Context, usecase.ReleaseGitOpsConfigInput) error {
	return nil
}
