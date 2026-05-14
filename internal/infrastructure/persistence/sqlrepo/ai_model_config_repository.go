package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	aidomain "gos/internal/domain/ai"
)

type AIModelConfigRepository struct {
	db     *sql.DB
	driver string
}

func NewAIModelConfigRepository(db *sql.DB, driver string) *AIModelConfigRepository {
	return &AIModelConfigRepository{
		db:     db,
		driver: strings.ToLower(strings.TrimSpace(driver)),
	}
}

func (r *AIModelConfigRepository) InitSchema(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("ai model config repository db is nil")
	}
	switch r.driver {
	case "mysql":
		_, err := r.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS ai_model_config (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  name VARCHAR(120) NOT NULL,
  provider VARCHAR(64) NOT NULL,
  base_url VARCHAR(500) NOT NULL,
  model VARCHAR(160) NOT NULL,
  api_key_cipher TEXT NOT NULL,
  temperature DOUBLE NOT NULL DEFAULT 0.2,
  max_tokens INT NOT NULL DEFAULT 2048,
  timeout_sec INT NOT NULL DEFAULT 60,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  is_diagnosis_model TINYINT(1) NOT NULL DEFAULT 0,
  created_by VARCHAR(64) NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  KEY idx_ai_model_config_diagnosis (is_diagnosis_model, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`)
		return err
	default:
		_, err := r.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS ai_model_config (
  id TEXT NOT NULL PRIMARY KEY,
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  base_url TEXT NOT NULL,
  model TEXT NOT NULL,
  api_key_cipher TEXT NOT NULL,
  temperature REAL NOT NULL DEFAULT 0.2,
  max_tokens INTEGER NOT NULL DEFAULT 2048,
  timeout_sec INTEGER NOT NULL DEFAULT 60,
  enabled INTEGER NOT NULL DEFAULT 1,
  is_diagnosis_model INTEGER NOT NULL DEFAULT 0,
  created_by TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ai_model_config_diagnosis ON ai_model_config (is_diagnosis_model, enabled);
`)
		return err
	}
}

func (r *AIModelConfigRepository) CreateModelConfig(ctx context.Context, item aidomain.ModelConfig) error {
	const q = `
INSERT INTO ai_model_config (
	id, name, provider, base_url, model, api_key_cipher, temperature, max_tokens, timeout_sec,
	enabled, is_diagnosis_model, created_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err := r.db.ExecContext(
		ctx,
		q,
		item.ID,
		item.Name,
		string(item.Provider),
		item.BaseURL,
		item.Model,
		item.APIKeyCipher,
		item.Temperature,
		item.MaxTokens,
		item.TimeoutSec,
		boolToInt(item.Enabled),
		boolToInt(item.IsDiagnosisModel),
		item.CreatedBy,
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	return err
}

func (r *AIModelConfigRepository) ListModelConfigs(ctx context.Context) ([]aidomain.ModelConfig, error) {
	const q = `
SELECT id, name, provider, base_url, model, api_key_cipher, temperature, max_tokens, timeout_sec,
	enabled, is_diagnosis_model, created_by, created_at, updated_at
FROM ai_model_config
ORDER BY is_diagnosis_model DESC, updated_at DESC, created_at DESC;`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]aidomain.ModelConfig, 0)
	for rows.Next() {
		item, scanErr := scanAIModelConfig(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *AIModelConfigRepository) GetModelConfigByID(ctx context.Context, id string) (aidomain.ModelConfig, error) {
	const q = `
SELECT id, name, provider, base_url, model, api_key_cipher, temperature, max_tokens, timeout_sec,
	enabled, is_diagnosis_model, created_by, created_at, updated_at
FROM ai_model_config
WHERE id = ?;`
	item, err := scanAIModelConfig(r.db.QueryRowContext(ctx, q, strings.TrimSpace(id)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return aidomain.ModelConfig{}, aidomain.ErrModelConfigNotFound
		}
		return aidomain.ModelConfig{}, err
	}
	return item, nil
}

func (r *AIModelConfigRepository) UpdateModelConfig(ctx context.Context, id string, input aidomain.ModelConfigUpdateInput) (aidomain.ModelConfig, error) {
	const q = `
UPDATE ai_model_config
SET name = ?, provider = ?, base_url = ?, model = ?, api_key_cipher = ?, temperature = ?, max_tokens = ?,
	timeout_sec = ?, enabled = ?, updated_at = ?
WHERE id = ?;`
	result, err := r.db.ExecContext(
		ctx,
		q,
		input.Name,
		string(input.Provider),
		input.BaseURL,
		input.Model,
		input.APIKeyCipher,
		input.Temperature,
		input.MaxTokens,
		input.TimeoutSec,
		boolToInt(input.Enabled),
		input.UpdatedAt.UTC().UnixNano(),
		strings.TrimSpace(id),
	)
	if err != nil {
		return aidomain.ModelConfig{}, err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return aidomain.ModelConfig{}, aidomain.ErrModelConfigNotFound
	}
	return r.GetModelConfigByID(ctx, id)
}

func (r *AIModelConfigRepository) DeleteModelConfig(ctx context.Context, id string) error {
	item, err := r.GetModelConfigByID(ctx, id)
	if err != nil {
		return err
	}
	if item.IsDiagnosisModel {
		return aidomain.ErrDiagnosisModelInUse
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM ai_model_config WHERE id = ?;`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return aidomain.ErrModelConfigNotFound
	}
	return nil
}

func (r *AIModelConfigRepository) SetDiagnosisModel(ctx context.Context, id string, updatedAt time.Time) (aidomain.ModelConfig, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return aidomain.ModelConfig{}, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	trimmedID := strings.TrimSpace(id)
	if _, err := tx.ExecContext(ctx, `UPDATE ai_model_config SET is_diagnosis_model = 0, updated_at = ?;`, updatedAt.UTC().UnixNano()); err != nil {
		return aidomain.ModelConfig{}, err
	}
	result, err := tx.ExecContext(
		ctx,
		`UPDATE ai_model_config SET is_diagnosis_model = 1, updated_at = ? WHERE id = ?;`,
		updatedAt.UTC().UnixNano(),
		trimmedID,
	)
	if err != nil {
		return aidomain.ModelConfig{}, err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return aidomain.ModelConfig{}, aidomain.ErrModelConfigNotFound
	}
	if err := tx.Commit(); err != nil {
		return aidomain.ModelConfig{}, err
	}
	tx = nil
	return r.GetModelConfigByID(ctx, trimmedID)
}

func (r *AIModelConfigRepository) UnsetDiagnosisModel(ctx context.Context, id string, updatedAt time.Time) (aidomain.ModelConfig, error) {
	trimmedID := strings.TrimSpace(id)
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE ai_model_config SET is_diagnosis_model = 0, updated_at = ? WHERE id = ?;`,
		updatedAt.UTC().UnixNano(),
		trimmedID,
	)
	if err != nil {
		return aidomain.ModelConfig{}, err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return aidomain.ModelConfig{}, aidomain.ErrModelConfigNotFound
	}
	return r.GetModelConfigByID(ctx, trimmedID)
}

func (r *AIModelConfigRepository) GetDiagnosisModel(ctx context.Context) (aidomain.ModelConfig, error) {
	const q = `
SELECT id, name, provider, base_url, model, api_key_cipher, temperature, max_tokens, timeout_sec,
	enabled, is_diagnosis_model, created_by, created_at, updated_at
FROM ai_model_config
WHERE is_diagnosis_model = 1 AND enabled = 1
ORDER BY updated_at DESC
LIMIT 1;`
	item, err := scanAIModelConfig(r.db.QueryRowContext(ctx, q))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return aidomain.ModelConfig{}, aidomain.ErrDiagnosisModelNotConfigured
		}
		return aidomain.ModelConfig{}, err
	}
	return item, nil
}

type aiModelConfigScanner interface {
	Scan(dest ...any) error
}

func scanAIModelConfig(s aiModelConfigScanner) (aidomain.ModelConfig, error) {
	var (
		item                       aidomain.ModelConfig
		providerRaw                string
		enabledRaw                 int
		diagnosisRaw               int
		createdAtRaw, updatedAtRaw int64
	)
	if err := s.Scan(
		&item.ID,
		&item.Name,
		&providerRaw,
		&item.BaseURL,
		&item.Model,
		&item.APIKeyCipher,
		&item.Temperature,
		&item.MaxTokens,
		&item.TimeoutSec,
		&enabledRaw,
		&diagnosisRaw,
		&item.CreatedBy,
		&createdAtRaw,
		&updatedAtRaw,
	); err != nil {
		return aidomain.ModelConfig{}, err
	}
	item.Provider = aidomain.ModelProvider(providerRaw)
	item.Enabled = enabledRaw == 1
	item.IsDiagnosisModel = diagnosisRaw == 1
	item.CreatedAt = time.Unix(0, createdAtRaw).UTC()
	item.UpdatedAt = time.Unix(0, updatedAtRaw).UTC()
	return item, nil
}
