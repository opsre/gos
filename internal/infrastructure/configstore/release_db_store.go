package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gos/internal/application/usecase"
)

const (
	releaseSettingsKeyEnvOptions   = "release_env_options"
	releaseSettingsKeyConcurrency  = "release_concurrency"
	releaseSettingsKeyGitOpsConfig = "release_gitops_config"
	settingsUpdatedAtSQLiteLayout  = "2006-01-02 15:04:05"
)

type DatabaseReleaseStore struct {
	db       *sql.DB
	driver   string
	fallback usecase.ReleaseSettingsStore
}

// NewDatabaseReleaseStore 创建并返回对应组件实例。
func NewDatabaseReleaseStore(db *sql.DB, driver string, fallback usecase.ReleaseSettingsStore) *DatabaseReleaseStore {
	return &DatabaseReleaseStore{
		db:       db,
		driver:   strings.TrimSpace(driver),
		fallback: fallback,
	}
}

// InitSchema 封装当前模块的业务处理逻辑。
func (s *DatabaseReleaseStore) InitSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("release settings store db is nil")
	}
	switch strings.ToLower(strings.TrimSpace(s.driver)) {
	case "mysql":
		_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS system_settings (
  setting_key VARCHAR(120) NOT NULL PRIMARY KEY,
  setting_value JSON NOT NULL,
  updated_at DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`)
		return err
	default:
		_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS system_settings (
  setting_key TEXT NOT NULL PRIMARY KEY,
  setting_value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
`)
		return err
	}
}

// LoadEnvOptions 封装当前模块的业务处理逻辑。
func (s *DatabaseReleaseStore) LoadEnvOptions(ctx context.Context) ([]string, error) {
	var stored []string
	ok, err := s.loadJSONSetting(ctx, releaseSettingsKeyEnvOptions, &stored)
	if err != nil {
		return nil, err
	}
	if ok {
		return normalizeStringList(stored), nil
	}
	if s.fallback == nil {
		return nil, nil
	}
	return s.fallback.LoadEnvOptions(ctx)
}

// SaveEnvOptions 封装当前模块的业务处理逻辑。
func (s *DatabaseReleaseStore) SaveEnvOptions(ctx context.Context, values []string) error {
	values = normalizeStringList(values)
	if len(values) == 0 {
		return fmt.Errorf("release env options are required")
	}
	if err := s.saveJSONSetting(ctx, releaseSettingsKeyEnvOptions, values); err != nil {
		return err
	}
	if s.fallback != nil {
		_ = s.fallback.SaveEnvOptions(ctx, values)
	}
	return nil
}

// LoadConcurrencySettings 封装当前模块的业务处理逻辑。
func (s *DatabaseReleaseStore) LoadConcurrencySettings(ctx context.Context) (usecase.ReleaseConcurrencySettingsOutput, error) {
	var stored usecase.ReleaseConcurrencySettingsOutput
	ok, err := s.loadJSONSetting(ctx, releaseSettingsKeyConcurrency, &stored)
	if err != nil {
		return usecase.ReleaseConcurrencySettingsOutput{}, err
	}
	if ok {
		return normalizeDBConcurrencySettings(stored), nil
	}
	if s.fallback == nil {
		return usecase.ReleaseConcurrencySettingsOutput{}, nil
	}
	return s.fallback.LoadConcurrencySettings(ctx)
}

// SaveConcurrencySettings 封装当前模块的业务处理逻辑。
func (s *DatabaseReleaseStore) SaveConcurrencySettings(ctx context.Context, input usecase.ReleaseConcurrencySettingsInput) error {
	normalized := normalizeDBConcurrencySettings(usecase.ReleaseConcurrencySettingsOutput(input))
	if err := s.saveJSONSetting(ctx, releaseSettingsKeyConcurrency, normalized); err != nil {
		return err
	}
	if s.fallback != nil {
		_ = s.fallback.SaveConcurrencySettings(ctx, normalized)
	}
	return nil
}

// loadJSONSetting 封装当前模块的业务处理逻辑。
func (s *DatabaseReleaseStore) loadJSONSetting(ctx context.Context, key string, target interface{}) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("release settings store db is nil")
	}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT setting_value FROM system_settings WHERE setting_key = ?`, strings.TrimSpace(key)).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	if strings.TrimSpace(raw) == "" {
		return false, nil
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return false, fmt.Errorf("decode system setting %s failed: %w", key, err)
	}
	return true, nil
}

// saveJSONSetting 封装当前模块的业务处理逻辑。
func (s *DatabaseReleaseStore) saveJSONSetting(ctx context.Context, key string, value interface{}) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("release settings store db is nil")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode system setting %s failed: %w", key, err)
	}
	now := time.Now().UTC()
	switch strings.ToLower(strings.TrimSpace(s.driver)) {
	case "mysql":
		_, err = s.db.ExecContext(ctx, `
INSERT INTO system_settings (setting_key, setting_value, updated_at)
VALUES (?, CAST(? AS JSON), ?)
ON DUPLICATE KEY UPDATE setting_value = VALUES(setting_value), updated_at = VALUES(updated_at)
`, strings.TrimSpace(key), string(payload), now)
	default:
		_, err = s.db.ExecContext(ctx, `
INSERT INTO system_settings (setting_key, setting_value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(setting_key) DO UPDATE SET setting_value = excluded.setting_value, updated_at = excluded.updated_at
`, strings.TrimSpace(key), string(payload), now.Format(settingsUpdatedAtSQLiteLayout))
	}
	if err != nil {
		return fmt.Errorf("save system setting %s failed: %w", key, err)
	}
	return nil
}

// LoadGitOpsConfig 封装当前模块的业务处理逻辑。
func (s *DatabaseReleaseStore) LoadGitOpsConfig(ctx context.Context) (usecase.ReleaseGitOpsConfigOutput, error) {
	var stored usecase.ReleaseGitOpsConfigOutput
	ok, err := s.loadJSONSetting(ctx, releaseSettingsKeyGitOpsConfig, &stored)
	if err != nil {
		return usecase.ReleaseGitOpsConfigOutput{}, err
	}
	if ok {
		return normalizeDBGitOpsConfig(stored), nil
	}
	if s.fallback == nil {
		return usecase.ReleaseGitOpsConfigOutput{}, nil
	}
	return s.fallback.LoadGitOpsConfig(ctx)
}

// SaveGitOpsConfig 封装当前模块的业务处理逻辑。
func (s *DatabaseReleaseStore) SaveGitOpsConfig(ctx context.Context, input usecase.ReleaseGitOpsConfigInput) error {
	normalized := normalizeDBGitOpsConfig(usecase.ReleaseGitOpsConfigOutput(input))
	if err := s.saveJSONSetting(ctx, releaseSettingsKeyGitOpsConfig, normalized); err != nil {
		return err
	}
	if s.fallback != nil {
		_ = s.fallback.SaveGitOpsConfig(ctx, normalized)
	}
	return nil
}

// normalizeDBGitOpsConfig 标准化输入值，保证后续逻辑使用统一格式。
func normalizeDBGitOpsConfig(input usecase.ReleaseGitOpsConfigOutput) usecase.ReleaseGitOpsConfigOutput {
	helmPath := strings.TrimSpace(input.HelmScanPath)
	if helmPath == "" {
		helmPath = "apps/helm"
	}
	kustomizePath := strings.TrimSpace(input.KustomizeScanPath)
	if kustomizePath == "" {
		kustomizePath = "apps/{app_key}/overlays/{env}"
	}
	return usecase.ReleaseGitOpsConfigOutput{
		HelmScanPath:      strings.TrimRight(helmPath, "/"),
		KustomizeScanPath: strings.TrimRight(kustomizePath, "/"),
	}
}

// normalizeDBConcurrencySettings 标准化输入值，保证后续逻辑使用统一格式。
func normalizeDBConcurrencySettings(input usecase.ReleaseConcurrencySettingsOutput) usecase.ReleaseConcurrencySettingsOutput {
	scope := input.LockScope
	if !scope.Valid() {
		scope = usecase.ReleaseConcurrencyLockScopeApplicationEnv
	}
	strategy := input.ConflictStrategy
	if !strategy.Valid() {
		strategy = usecase.ReleaseConcurrencyConflictStrategyReject
	}
	timeout := input.LockTimeoutSec
	if timeout <= 0 {
		timeout = 1800
	}
	if timeout < 30 {
		timeout = 30
	}
	if timeout > 86400 {
		timeout = 86400
	}
	return usecase.ReleaseConcurrencySettingsOutput{
		Enabled:          input.Enabled,
		LockScope:        scope,
		ConflictStrategy: strategy,
		LockTimeoutSec:   timeout,
	}
}
