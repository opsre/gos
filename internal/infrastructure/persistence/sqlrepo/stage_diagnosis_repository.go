package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	aidomain "gos/internal/domain/ai"
)

type StageDiagnosisRepository struct {
	db     *sql.DB
	driver string
}

func NewStageDiagnosisRepository(db *sql.DB, driver string) *StageDiagnosisRepository {
	return &StageDiagnosisRepository{
		db:     db,
		driver: strings.ToLower(strings.TrimSpace(driver)),
	}
}

func (r *StageDiagnosisRepository) InitSchema(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("stage diagnosis repository db is nil")
	}
	switch r.driver {
	case "mysql":
		_, err := r.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS release_order_pipeline_stage_diagnosis (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  release_order_id VARCHAR(64) NOT NULL,
  stage_id VARCHAR(128) NOT NULL,
  execution_id VARCHAR(64) NOT NULL DEFAULT '',
  pipeline_scope VARCHAR(16) NOT NULL,
  executor_type VARCHAR(32) NOT NULL,
  stage_name VARCHAR(255) NOT NULL,
  stage_status VARCHAR(32) NOT NULL,
  ai_model_config_id VARCHAR(64) NOT NULL,
  ai_model_name VARCHAR(120) NOT NULL,
  ai_model VARCHAR(160) NOT NULL,
  prompt_version VARCHAR(64) NOT NULL,
  log_hash VARCHAR(64) NOT NULL,
  log_excerpt MEDIUMTEXT,
  status VARCHAR(32) NOT NULL,
  result_json JSON,
  error_message TEXT,
  created_by VARCHAR(64) NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL,
  finished_at BIGINT NULL,
  KEY idx_stage_diagnosis_stage (release_order_id, stage_id, created_at),
  KEY idx_stage_diagnosis_cache (stage_id, log_hash, ai_model_config_id, prompt_version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`)
		return err
	default:
		_, err := r.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS release_order_pipeline_stage_diagnosis (
  id TEXT NOT NULL PRIMARY KEY,
  release_order_id TEXT NOT NULL,
  stage_id TEXT NOT NULL,
  execution_id TEXT NOT NULL DEFAULT '',
  pipeline_scope TEXT NOT NULL,
  executor_type TEXT NOT NULL,
  stage_name TEXT NOT NULL,
  stage_status TEXT NOT NULL,
  ai_model_config_id TEXT NOT NULL,
  ai_model_name TEXT NOT NULL,
  ai_model TEXT NOT NULL,
  prompt_version TEXT NOT NULL,
  log_hash TEXT NOT NULL,
  log_excerpt TEXT,
  status TEXT NOT NULL,
  result_json TEXT,
  error_message TEXT,
  created_by TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  finished_at INTEGER NULL
);
CREATE INDEX IF NOT EXISTS idx_stage_diagnosis_stage ON release_order_pipeline_stage_diagnosis (release_order_id, stage_id, created_at);
CREATE INDEX IF NOT EXISTS idx_stage_diagnosis_cache ON release_order_pipeline_stage_diagnosis (stage_id, log_hash, ai_model_config_id, prompt_version);
`)
		return err
	}
}

func (r *StageDiagnosisRepository) CreateStageDiagnosis(ctx context.Context, item aidomain.StageDiagnosis) error {
	const q = `
INSERT INTO release_order_pipeline_stage_diagnosis (
	id, release_order_id, stage_id, execution_id, pipeline_scope, executor_type, stage_name, stage_status,
	ai_model_config_id, ai_model_name, ai_model, prompt_version, log_hash, log_excerpt, status, result_json,
	error_message, created_by, created_at, finished_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err := r.db.ExecContext(
		ctx,
		q,
		item.ID,
		item.ReleaseOrderID,
		item.StageID,
		item.ExecutionID,
		item.PipelineScope,
		item.ExecutorType,
		item.StageName,
		item.StageStatus,
		item.AIModelConfigID,
		item.AIModelName,
		item.AIModel,
		item.PromptVersion,
		item.LogHash,
		item.LogExcerpt,
		string(item.Status),
		nullableJSONString(item.ResultJSON),
		item.ErrorMessage,
		item.CreatedBy,
		item.CreatedAt.UTC().UnixNano(),
		nullableTimeUnixNano(item.FinishedAt),
	)
	return err
}

func (r *StageDiagnosisRepository) FindSuccessfulStageDiagnosisByCacheKey(
	ctx context.Context,
	cache aidomain.StageDiagnosisCacheKey,
) (aidomain.StageDiagnosis, error) {
	const q = `
SELECT id, release_order_id, stage_id, execution_id, pipeline_scope, executor_type, stage_name, stage_status,
	ai_model_config_id, ai_model_name, ai_model, prompt_version, log_hash, log_excerpt, status, result_json,
	error_message, created_by, created_at, finished_at
FROM release_order_pipeline_stage_diagnosis
WHERE stage_id = ? AND log_hash = ? AND ai_model_config_id = ? AND prompt_version = ? AND status = ?
ORDER BY created_at DESC
LIMIT 1;`
	item, err := scanStageDiagnosis(r.db.QueryRowContext(
		ctx,
		q,
		strings.TrimSpace(cache.StageID),
		strings.TrimSpace(cache.LogHash),
		strings.TrimSpace(cache.AIModelConfigID),
		strings.TrimSpace(cache.PromptVersion),
		string(aidomain.StageDiagnosisStatusSuccess),
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return aidomain.StageDiagnosis{}, aidomain.ErrStageDiagnosisNotFound
		}
		return aidomain.StageDiagnosis{}, err
	}
	return item, nil
}

func (r *StageDiagnosisRepository) GetStageDiagnosisByID(ctx context.Context, id string) (aidomain.StageDiagnosis, error) {
	const q = `
SELECT id, release_order_id, stage_id, execution_id, pipeline_scope, executor_type, stage_name, stage_status,
	ai_model_config_id, ai_model_name, ai_model, prompt_version, log_hash, log_excerpt, status, result_json,
	error_message, created_by, created_at, finished_at
FROM release_order_pipeline_stage_diagnosis
WHERE id = ?;`
	item, err := scanStageDiagnosis(r.db.QueryRowContext(ctx, q, strings.TrimSpace(id)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return aidomain.StageDiagnosis{}, aidomain.ErrStageDiagnosisNotFound
		}
		return aidomain.StageDiagnosis{}, err
	}
	return item, nil
}

func (r *StageDiagnosisRepository) FindLatestStageDiagnosis(
	ctx context.Context,
	releaseOrderID string,
	stageID string,
) (aidomain.StageDiagnosis, error) {
	const q = `
SELECT id, release_order_id, stage_id, execution_id, pipeline_scope, executor_type, stage_name, stage_status,
	ai_model_config_id, ai_model_name, ai_model, prompt_version, log_hash, log_excerpt, status, result_json,
	error_message, created_by, created_at, finished_at
FROM release_order_pipeline_stage_diagnosis
WHERE release_order_id = ? AND stage_id = ?
ORDER BY created_at DESC
LIMIT 1;`
	item, err := scanStageDiagnosis(r.db.QueryRowContext(ctx, q, strings.TrimSpace(releaseOrderID), strings.TrimSpace(stageID)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return aidomain.StageDiagnosis{}, aidomain.ErrStageDiagnosisNotFound
		}
		return aidomain.StageDiagnosis{}, err
	}
	return item, nil
}

func scanStageDiagnosis(s aiModelConfigScanner) (aidomain.StageDiagnosis, error) {
	var (
		item          aidomain.StageDiagnosis
		statusRaw     string
		resultJSON    sql.NullString
		logExcerpt    sql.NullString
		errorMessage  sql.NullString
		finishedAtRaw sql.NullInt64
		createdAtRaw  int64
	)
	if err := s.Scan(
		&item.ID,
		&item.ReleaseOrderID,
		&item.StageID,
		&item.ExecutionID,
		&item.PipelineScope,
		&item.ExecutorType,
		&item.StageName,
		&item.StageStatus,
		&item.AIModelConfigID,
		&item.AIModelName,
		&item.AIModel,
		&item.PromptVersion,
		&item.LogHash,
		&logExcerpt,
		&statusRaw,
		&resultJSON,
		&errorMessage,
		&item.CreatedBy,
		&createdAtRaw,
		&finishedAtRaw,
	); err != nil {
		return aidomain.StageDiagnosis{}, err
	}
	item.Status = aidomain.StageDiagnosisStatus(statusRaw)
	if resultJSON.Valid {
		item.ResultJSON = resultJSON.String
	}
	if logExcerpt.Valid {
		item.LogExcerpt = logExcerpt.String
	}
	if errorMessage.Valid {
		item.ErrorMessage = errorMessage.String
	}
	item.CreatedAt = time.Unix(0, createdAtRaw).UTC()
	if finishedAtRaw.Valid {
		t := time.Unix(0, finishedAtRaw.Int64).UTC()
		item.FinishedAt = &t
	}
	return item, nil
}

func nullableJSONString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableTimeUnixNano(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().UnixNano()
}
