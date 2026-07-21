package sqlrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/pipelinescan"
)

type PipelineScanRepository struct {
	db       *sql.DB
	dbDriver string
}

func NewPipelineScanRepository(db *sql.DB, dbDriver string) *PipelineScanRepository {
	return &PipelineScanRepository{
		db:       db,
		dbDriver: strings.ToLower(strings.TrimSpace(dbDriver)),
	}
}

func (r *PipelineScanRepository) InitSchema(ctx context.Context) error {
	statements, err := pipelineScanSchemaStatements(r.dbDriver)
	if err != nil {
		return err
	}
	for _, stmt := range statements {
		if _, execErr := r.db.ExecContext(ctx, stmt); execErr != nil {
			return execErr
		}
	}
	if err := runSchemaMigrations(
		ctx,
		r.db,
		r.dbDriver,
		schemaMigration{
			Version:     "deploy_platform_v1_3_1_pipeline_scan_rules",
			Description: "upgrade pipeline scan rule metadata for builtin rule management",
			Up:          r.migratePipelineScanRuleSchema,
		},
	); err != nil {
		return err
	}
	return r.ensureBuiltinPipelineScanRules(ctx)
}

func pipelineScanSchemaStatements(dbDriver string) ([]string, error) {
	switch dbDriver {
	case "mysql":
		return []string{
			`CREATE TABLE IF NOT EXISTS pipeline_scan_rules (
	id VARCHAR(64) PRIMARY KEY,
	rule_code VARCHAR(128) NOT NULL,
	rule_name VARCHAR(128) NOT NULL,
	category VARCHAR(32) NOT NULL,
	severity VARCHAR(32) NOT NULL,
	enabled TINYINT(1) NOT NULL,
	builtin TINYINT(1) NOT NULL DEFAULT 0,
	template_validation_scopes_json TEXT NOT NULL,
	scope_json TEXT NOT NULL,
	rule_dsl_json TEXT NOT NULL,
	message VARCHAR(500) NOT NULL,
	suggestion VARCHAR(500) NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uq_pipeline_scan_rule_code (rule_code),
	KEY idx_pipeline_scan_rule_enabled_category (enabled, category),
	KEY idx_pipeline_scan_rule_updated_at (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS pipeline_scan_results (
	id VARCHAR(64) PRIMARY KEY,
	pipeline_id VARCHAR(64) NOT NULL,
	pipeline_name VARCHAR(255) NOT NULL,
	scan_status VARCHAR(32) NOT NULL,
	total_findings INT NOT NULL,
	error_count INT NOT NULL,
	warning_count INT NOT NULL,
	info_count INT NOT NULL,
	script_hash VARCHAR(128) NOT NULL,
	last_scanned_at BIGINT NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uq_pipeline_scan_result_pipeline (pipeline_id),
	KEY idx_pipeline_scan_result_status_updated_at (scan_status, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS pipeline_scan_findings (
	id VARCHAR(64) PRIMARY KEY,
	pipeline_id VARCHAR(64) NOT NULL,
	rule_id VARCHAR(64) NOT NULL,
	rule_code VARCHAR(128) NOT NULL,
	rule_name VARCHAR(128) NOT NULL,
	severity VARCHAR(32) NOT NULL,
	line_no INT NOT NULL,
	matched_text TEXT NOT NULL,
	message VARCHAR(500) NOT NULL,
	suggestion VARCHAR(500) NOT NULL,
	details_json TEXT NOT NULL,
	status VARCHAR(32) NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	KEY idx_pipeline_scan_finding_pipeline (pipeline_id, status, severity),
	KEY idx_pipeline_scan_finding_rule (rule_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		}, nil
	case "sqlite":
		return []string{
			`CREATE TABLE IF NOT EXISTS pipeline_scan_rules (
	id TEXT PRIMARY KEY,
	rule_code TEXT NOT NULL UNIQUE,
	rule_name TEXT NOT NULL,
	category TEXT NOT NULL,
	severity TEXT NOT NULL,
	enabled INTEGER NOT NULL,
	builtin INTEGER NOT NULL DEFAULT 0,
	template_validation_scopes_json TEXT NOT NULL,
	scope_json TEXT NOT NULL,
	rule_dsl_json TEXT NOT NULL,
	message TEXT NOT NULL,
	suggestion TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
			`CREATE INDEX IF NOT EXISTS idx_pipeline_scan_rule_enabled_category ON pipeline_scan_rules (enabled, category);`,
			`CREATE INDEX IF NOT EXISTS idx_pipeline_scan_rule_updated_at ON pipeline_scan_rules (updated_at);`,
			`CREATE TABLE IF NOT EXISTS pipeline_scan_results (
	id TEXT PRIMARY KEY,
	pipeline_id TEXT NOT NULL UNIQUE,
	pipeline_name TEXT NOT NULL,
	scan_status TEXT NOT NULL,
	total_findings INTEGER NOT NULL,
	error_count INTEGER NOT NULL,
	warning_count INTEGER NOT NULL,
	info_count INTEGER NOT NULL,
	script_hash TEXT NOT NULL,
	last_scanned_at INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
			`CREATE INDEX IF NOT EXISTS idx_pipeline_scan_result_status_updated_at ON pipeline_scan_results (scan_status, updated_at);`,
			`CREATE TABLE IF NOT EXISTS pipeline_scan_findings (
	id TEXT PRIMARY KEY,
	pipeline_id TEXT NOT NULL,
	rule_id TEXT NOT NULL,
	rule_code TEXT NOT NULL,
	rule_name TEXT NOT NULL,
	severity TEXT NOT NULL,
	line_no INTEGER NOT NULL,
	matched_text TEXT NOT NULL,
	message TEXT NOT NULL,
	suggestion TEXT NOT NULL,
	details_json TEXT NOT NULL,
	status TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
			`CREATE INDEX IF NOT EXISTS idx_pipeline_scan_finding_pipeline ON pipeline_scan_findings (pipeline_id, status, severity);`,
			`CREATE INDEX IF NOT EXISTS idx_pipeline_scan_finding_rule ON pipeline_scan_findings (rule_id);`,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported db driver: %s", dbDriver)
	}
}

func (r *PipelineScanRepository) migratePipelineScanRuleSchema(ctx context.Context) error {
	switch r.dbDriver {
	case "mysql":
		exists, err := r.mysqlColumnExists(ctx, "pipeline_scan_rules", "template_validation_scopes_json")
		if err != nil {
			return err
		}
		if !exists {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE pipeline_scan_rules ADD COLUMN template_validation_scopes_json TEXT NULL AFTER enabled;`); err != nil {
				return err
			}
		}
		exists, err = r.mysqlColumnExists(ctx, "pipeline_scan_rules", "builtin")
		if err != nil {
			return err
		}
		if !exists {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE pipeline_scan_rules ADD COLUMN builtin TINYINT(1) NOT NULL DEFAULT 0 AFTER enabled;`); err != nil {
				return err
			}
		}
	case "sqlite":
		columns, err := r.sqliteTableColumns(ctx, "pipeline_scan_rules")
		if err != nil {
			return err
		}
		if _, ok := columns["template_validation_scopes_json"]; !ok {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE pipeline_scan_rules ADD COLUMN template_validation_scopes_json TEXT NOT NULL DEFAULT '[]';`); err != nil {
				return err
			}
		}
		if _, ok := columns["builtin"]; !ok {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE pipeline_scan_rules ADD COLUMN builtin INTEGER NOT NULL DEFAULT 0;`); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE pipeline_scan_rules SET template_validation_scopes_json = '[]' WHERE template_validation_scopes_json IS NULL OR template_validation_scopes_json = '';`); err != nil {
		return err
	}
	return nil
}

func (r *PipelineScanRepository) ensureBuiltinPipelineScanRules(ctx context.Context) error {
	now := time.Now().UTC()
	items := builtinPipelineScanRules(now)
	for _, item := range items {
		if err := r.upsertBuiltinPipelineScanRule(ctx, item); err != nil {
			return err
		}
		if err := r.normalizeBuiltinPipelineScanRuleID(ctx, item); err != nil {
			return err
		}
	}
	return r.normalizePipelineScanBuiltinFlags(ctx, builtinPipelineScanRuleCodes(items), now)
}

func builtinPipelineScanRules(now time.Time) []domain.Rule {
	return []domain.Rule{
		{
			ID:       "psr-builtin-gos-artifact-url",
			RuleCode: "artifact.gos.artifact_url.standard",
			RuleName: "GOS 制品地址输出规范",
			Category: domain.CategoryArtifact,
			Severity: domain.SeverityWarning,
			// 新部署默认不强制现有 Jenkinsfile 输出 GOS_ARTIFACT_URL；
			// 管理员确认团队已经采用该约定后再手动启用。
			Enabled:                  false,
			Builtin:                  true,
			TemplateValidationScopes: []string{"ci"},
			ScopeJSON:                "{}",
			RuleDSL:                  `{"matcher":{"type":"regex","pattern":"(?m)\\bGOS_ARTIFACT_URL\\s*="}}`,
			Message:                  "缺少 GOS_ARTIFACT_URL 制品地址输出",
			Suggestion:               `OSS 上传成功后输出 echo "GOS_ARTIFACT_URL=..."`,
			CreatedAt:                now,
			UpdatedAt:                now,
		},
	}
}

func builtinPipelineScanRuleCodes(items []domain.Rule) []string {
	codes := make([]string, 0, len(items))
	for _, item := range items {
		codes = append(codes, item.RuleCode)
	}
	return codes
}

func (r *PipelineScanRepository) upsertBuiltinPipelineScanRule(ctx context.Context, item domain.Rule) error {
	scopesJSON, err := marshalPipelineScanStringList(item.TemplateValidationScopes)
	if err != nil {
		return err
	}
	args := []any{
		item.ID,
		item.RuleCode,
		item.RuleName,
		string(item.Category),
		string(item.Severity),
		boolToInt(item.Enabled),
		boolToInt(item.Builtin),
		scopesJSON,
		item.ScopeJSON,
		item.RuleDSL,
		item.Message,
		item.Suggestion,
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	}
	switch r.dbDriver {
	case "mysql":
		const q = `
INSERT INTO pipeline_scan_rules (
	id, rule_code, rule_name, category, severity, enabled, builtin, template_validation_scopes_json, scope_json, rule_dsl_json, message, suggestion, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	rule_name = VALUES(rule_name),
	category = VALUES(category),
	severity = VALUES(severity),
	builtin = VALUES(builtin),
	template_validation_scopes_json = VALUES(template_validation_scopes_json),
	scope_json = VALUES(scope_json),
	rule_dsl_json = VALUES(rule_dsl_json),
	message = VALUES(message),
	suggestion = VALUES(suggestion),
	updated_at = VALUES(updated_at);`
		_, err := r.db.ExecContext(ctx, q, args...)
		return err
	case "sqlite":
		const q = `
INSERT INTO pipeline_scan_rules (
	id, rule_code, rule_name, category, severity, enabled, builtin, template_validation_scopes_json, scope_json, rule_dsl_json, message, suggestion, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(rule_code) DO UPDATE SET
	rule_name = excluded.rule_name,
	category = excluded.category,
	severity = excluded.severity,
	builtin = excluded.builtin,
	template_validation_scopes_json = excluded.template_validation_scopes_json,
	scope_json = excluded.scope_json,
	rule_dsl_json = excluded.rule_dsl_json,
	message = excluded.message,
	suggestion = excluded.suggestion,
	updated_at = excluded.updated_at;`
		_, err := r.db.ExecContext(ctx, q, args...)
		return err
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
}

// normalizeBuiltinPipelineScanRuleID migrates rows created before a rule became
// platform-owned to the stable builtin ID. The rule code has always been unique,
// so an upsert by rule_code can otherwise retain the historical random ID and
// make builtin-ID endpoints return 404. Existing findings are migrated together
// so historical scan details keep pointing at the rule.
func (r *PipelineScanRepository) normalizeBuiltinPipelineScanRuleID(ctx context.Context, item domain.Rule) error {
	var existingID string
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id FROM pipeline_scan_rules WHERE rule_code = ?;`,
		item.RuleCode,
	).Scan(&existingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	existingID = strings.TrimSpace(existingID)
	if existingID == "" || existingID == item.ID {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE pipeline_scan_findings SET rule_id = ? WHERE rule_id = ?;`,
		item.ID,
		existingID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE pipeline_scan_rules SET id = ?, updated_at = ? WHERE id = ? AND rule_code = ?;`,
		item.ID,
		item.UpdatedAt.UTC().UnixNano(),
		existingID,
		item.RuleCode,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PipelineScanRepository) normalizePipelineScanBuiltinFlags(ctx context.Context, builtinCodes []string, now time.Time) error {
	if len(builtinCodes) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(builtinCodes))
	args := make([]any, 0, len(builtinCodes)+1)
	args = append(args, now.UTC().UnixNano())
	for _, code := range builtinCodes {
		placeholders = append(placeholders, "?")
		args = append(args, code)
	}
	query := fmt.Sprintf(
		`UPDATE pipeline_scan_rules SET builtin = 0, updated_at = ? WHERE builtin <> 0 AND rule_code NOT IN (%s);`,
		strings.Join(placeholders, ", "),
	)
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *PipelineScanRepository) CreateRule(ctx context.Context, item domain.Rule) error {
	const q = `
INSERT INTO pipeline_scan_rules (
	id, rule_code, rule_name, category, severity, enabled, builtin, template_validation_scopes_json, scope_json, rule_dsl_json, message, suggestion, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	scopesJSON, err := marshalPipelineScanStringList(item.TemplateValidationScopes)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(
		ctx,
		q,
		item.ID,
		item.RuleCode,
		item.RuleName,
		string(item.Category),
		string(item.Severity),
		boolToInt(item.Enabled),
		boolToInt(item.Builtin),
		scopesJSON,
		item.ScopeJSON,
		item.RuleDSL,
		item.Message,
		item.Suggestion,
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isDuplicateKeyError(r.dbDriver, err) {
			return domain.ErrRuleDuplicated
		}
		return err
	}
	return nil
}

func (r *PipelineScanRepository) ListRules(ctx context.Context, filter domain.RuleListFilter) ([]domain.Rule, int64, error) {
	where := make([]string, 0, 4)
	args := make([]any, 0, 4)

	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		where = append(where, "(rule_code LIKE ? OR rule_name LIKE ? OR category LIKE ? OR severity LIKE ? OR scope_json LIKE ? OR rule_dsl_json LIKE ? OR message LIKE ? OR suggestion LIKE ?)")
		args = append(args, like, like, like, like, like, like, like, like)
	}
	if filter.Category != "" {
		where = append(where, "category = ?")
		args = append(args, string(filter.Category))
	}
	if filter.Severity != "" {
		where = append(where, "severity = ?")
		args = append(args, string(filter.Severity))
	}
	if filter.Enabled != nil {
		where = append(where, "enabled = ?")
		args = append(args, boolToInt(*filter.Enabled))
	}

	countQuery := "SELECT COUNT(1) FROM pipeline_scan_rules"
	if len(where) > 0 {
		countQuery += " WHERE " + strings.Join(where, " AND ")
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page, pageSize := normalizePipelineScanPage(filter.Page, filter.PageSize)
	listQuery := `
SELECT id, rule_code, rule_name, category, severity, enabled, COALESCE(builtin, 0), COALESCE(template_validation_scopes_json, '[]'), scope_json, rule_dsl_json, message, suggestion, created_at, updated_at
FROM pipeline_scan_rules`
	if len(where) > 0 {
		listQuery += " WHERE " + strings.Join(where, " AND ")
	}
	listQuery += " ORDER BY updated_at DESC LIMIT ? OFFSET ?;"

	rows, err := r.db.QueryContext(ctx, listQuery, append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]domain.Rule, 0)
	for rows.Next() {
		item, scanErr := scanPipelineScanRule(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *PipelineScanRepository) ListEnabledRules(ctx context.Context) ([]domain.Rule, error) {
	const q = `
SELECT id, rule_code, rule_name, category, severity, enabled, COALESCE(builtin, 0), COALESCE(template_validation_scopes_json, '[]'), scope_json, rule_dsl_json, message, suggestion, created_at, updated_at
FROM pipeline_scan_rules
WHERE enabled = ?
ORDER BY category ASC, updated_at DESC;`
	rows, err := r.db.QueryContext(ctx, q, boolToInt(true))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.Rule, 0)
	for rows.Next() {
		item, scanErr := scanPipelineScanRule(rows)
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

func (r *PipelineScanRepository) GetRuleByID(ctx context.Context, id string) (domain.Rule, error) {
	const q = `
SELECT id, rule_code, rule_name, category, severity, enabled, COALESCE(builtin, 0), COALESCE(template_validation_scopes_json, '[]'), scope_json, rule_dsl_json, message, suggestion, created_at, updated_at
FROM pipeline_scan_rules
WHERE id = ?;`
	item, err := scanPipelineScanRule(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Rule{}, domain.ErrRuleNotFound
		}
		return domain.Rule{}, err
	}
	return item, nil
}

func (r *PipelineScanRepository) UpdateRule(ctx context.Context, id string, input domain.RuleUpdateInput) (domain.Rule, error) {
	const q = `
UPDATE pipeline_scan_rules
SET rule_code = ?, rule_name = ?, category = ?, severity = ?, enabled = ?, template_validation_scopes_json = ?, scope_json = ?, rule_dsl_json = ?, message = ?, suggestion = ?, updated_at = ?
WHERE id = ?;`
	now := time.Now().UTC()
	scopesJSON, err := marshalPipelineScanStringList(input.TemplateValidationScopes)
	if err != nil {
		return domain.Rule{}, err
	}
	res, err := r.db.ExecContext(
		ctx,
		q,
		input.RuleCode,
		input.RuleName,
		string(input.Category),
		string(input.Severity),
		boolToInt(input.Enabled),
		scopesJSON,
		input.ScopeJSON,
		input.RuleDSL,
		input.Message,
		input.Suggestion,
		now.UnixNano(),
		id,
	)
	if err != nil {
		if isDuplicateKeyError(r.dbDriver, err) {
			return domain.Rule{}, domain.ErrRuleDuplicated
		}
		return domain.Rule{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Rule{}, err
	}
	if affected == 0 {
		return domain.Rule{}, domain.ErrRuleNotFound
	}
	return r.GetRuleByID(ctx, id)
}

func (r *PipelineScanRepository) DeleteRule(ctx context.Context, id string) error {
	const q = `DELETE FROM pipeline_scan_rules WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrRuleNotFound
	}
	return nil
}

func (r *PipelineScanRepository) SaveScan(ctx context.Context, result domain.Result, findings []domain.Finding) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = r.upsertScanResult(ctx, tx, result); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM pipeline_scan_findings WHERE pipeline_id = ?;`, result.PipelineID); err != nil {
		return err
	}
	const findingInsert = `
INSERT INTO pipeline_scan_findings (
	id, pipeline_id, rule_id, rule_code, rule_name, severity, line_no, matched_text, message, suggestion, details_json, status, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	for _, item := range findings {
		if _, err = tx.ExecContext(
			ctx,
			findingInsert,
			item.ID,
			item.PipelineID,
			item.RuleID,
			item.RuleCode,
			item.RuleName,
			string(item.Severity),
			item.LineNo,
			item.MatchedText,
			item.Message,
			item.Suggestion,
			item.DetailsJSON,
			string(item.Status),
			item.CreatedAt.UTC().UnixNano(),
			item.UpdatedAt.UTC().UnixNano(),
		); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (r *PipelineScanRepository) upsertScanResult(ctx context.Context, tx *sql.Tx, item domain.Result) error {
	switch r.dbDriver {
	case "mysql":
		const q = `
INSERT INTO pipeline_scan_results (
	id, pipeline_id, pipeline_name, scan_status, total_findings, error_count, warning_count, info_count, script_hash, last_scanned_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	pipeline_name = VALUES(pipeline_name),
	scan_status = VALUES(scan_status),
	total_findings = VALUES(total_findings),
	error_count = VALUES(error_count),
	warning_count = VALUES(warning_count),
	info_count = VALUES(info_count),
	script_hash = VALUES(script_hash),
	last_scanned_at = VALUES(last_scanned_at),
	updated_at = VALUES(updated_at);`
		_, err := tx.ExecContext(ctx, q, scanResultArgs(item)...)
		return err
	case "sqlite":
		const q = `
INSERT INTO pipeline_scan_results (
	id, pipeline_id, pipeline_name, scan_status, total_findings, error_count, warning_count, info_count, script_hash, last_scanned_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(pipeline_id) DO UPDATE SET
	pipeline_name = excluded.pipeline_name,
	scan_status = excluded.scan_status,
	total_findings = excluded.total_findings,
	error_count = excluded.error_count,
	warning_count = excluded.warning_count,
	info_count = excluded.info_count,
	script_hash = excluded.script_hash,
	last_scanned_at = excluded.last_scanned_at,
	updated_at = excluded.updated_at;`
		_, err := tx.ExecContext(ctx, q, scanResultArgs(item)...)
		return err
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
}

func scanResultArgs(item domain.Result) []any {
	return []any{
		item.ID,
		item.PipelineID,
		item.PipelineName,
		string(item.ScanStatus),
		item.TotalFindings,
		item.ErrorCount,
		item.WarningCount,
		item.InfoCount,
		item.ScriptHash,
		item.LastScannedAt.UTC().UnixNano(),
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	}
}

func (r *PipelineScanRepository) GetResultByPipelineID(ctx context.Context, pipelineID string) (domain.Result, []domain.Finding, error) {
	const resultQuery = `
SELECT id, pipeline_id, pipeline_name, scan_status, total_findings, error_count, warning_count, info_count, script_hash, last_scanned_at, created_at, updated_at
FROM pipeline_scan_results
WHERE pipeline_id = ?;`
	result, err := scanPipelineScanResult(r.db.QueryRowContext(ctx, resultQuery, pipelineID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Result{}, nil, domain.ErrResultNotFound
		}
		return domain.Result{}, nil, err
	}

	const findingsQuery = `
SELECT id, pipeline_id, rule_id, rule_code, rule_name, severity, line_no, matched_text, message, suggestion, details_json, status, created_at, updated_at
FROM pipeline_scan_findings
WHERE pipeline_id = ?
ORDER BY line_no ASC, rule_code ASC;`
	rows, err := r.db.QueryContext(ctx, findingsQuery, pipelineID)
	if err != nil {
		return domain.Result{}, nil, err
	}
	defer rows.Close()

	findings := make([]domain.Finding, 0)
	for rows.Next() {
		item, scanErr := scanPipelineScanFinding(rows)
		if scanErr != nil {
			return domain.Result{}, nil, scanErr
		}
		findings = append(findings, item)
	}
	if err := rows.Err(); err != nil {
		return domain.Result{}, nil, err
	}
	return result, findings, nil
}

func (r *PipelineScanRepository) ListResults(ctx context.Context, filter domain.ResultListFilter) ([]domain.Result, int64, error) {
	where := make([]string, 0, 2)
	args := make([]any, 0, 2)

	if pipelineName := strings.TrimSpace(filter.PipelineName); pipelineName != "" {
		where = append(where, "pipeline_name LIKE ?")
		args = append(args, "%"+pipelineName+"%")
	}
	if filter.ScanStatus != "" {
		where = append(where, "scan_status = ?")
		args = append(args, string(filter.ScanStatus))
	}

	countQuery := "SELECT COUNT(1) FROM pipeline_scan_results"
	if len(where) > 0 {
		countQuery += " WHERE " + strings.Join(where, " AND ")
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page, pageSize := normalizePipelineScanPage(filter.Page, filter.PageSize)
	listQuery := `
SELECT id, pipeline_id, pipeline_name, scan_status, total_findings, error_count, warning_count, info_count, script_hash, last_scanned_at, created_at, updated_at
FROM pipeline_scan_results`
	if len(where) > 0 {
		listQuery += " WHERE " + strings.Join(where, " AND ")
	}
	listQuery += " ORDER BY updated_at DESC LIMIT ? OFFSET ?;"

	rows, err := r.db.QueryContext(ctx, listQuery, append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]domain.Result, 0)
	for rows.Next() {
		item, scanErr := scanPipelineScanResult(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func normalizePipelineScanPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

func scanPipelineScanRule(s scanner) (domain.Rule, error) {
	var (
		item       domain.Rule
		category   string
		severity   string
		enabled    int
		builtin    int
		scopesJSON string
		createdAt  int64
		updatedAt  int64
	)
	if err := s.Scan(
		&item.ID,
		&item.RuleCode,
		&item.RuleName,
		&category,
		&severity,
		&enabled,
		&builtin,
		&scopesJSON,
		&item.ScopeJSON,
		&item.RuleDSL,
		&item.Message,
		&item.Suggestion,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.Rule{}, err
	}
	item.Category = domain.Category(category)
	item.Severity = domain.Severity(severity)
	item.Enabled = enabled > 0
	item.Builtin = builtin > 0
	item.TemplateValidationScopes = unmarshalPipelineScanStringList(scopesJSON)
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

func marshalPipelineScanStringList(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	bytes, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func unmarshalPipelineScanStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func (r *PipelineScanRepository) mysqlColumnExists(ctx context.Context, table, column string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(1) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`,
		table,
		column,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PipelineScanRepository) sqliteTableColumns(ctx context.Context, table string) (map[string]struct{}, error) {
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s);", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]struct{})
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return nil, err
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func scanPipelineScanResult(s scanner) (domain.Result, error) {
	var (
		item          domain.Result
		status        string
		lastScannedAt int64
		createdAt     int64
		updatedAt     int64
	)
	if err := s.Scan(
		&item.ID,
		&item.PipelineID,
		&item.PipelineName,
		&status,
		&item.TotalFindings,
		&item.ErrorCount,
		&item.WarningCount,
		&item.InfoCount,
		&item.ScriptHash,
		&lastScannedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.Result{}, err
	}
	item.ScanStatus = domain.ScanStatus(status)
	item.LastScannedAt = time.Unix(0, lastScannedAt).UTC()
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

func scanPipelineScanFinding(s scanner) (domain.Finding, error) {
	var (
		item      domain.Finding
		severity  string
		status    string
		createdAt int64
		updatedAt int64
	)
	if err := s.Scan(
		&item.ID,
		&item.PipelineID,
		&item.RuleID,
		&item.RuleCode,
		&item.RuleName,
		&severity,
		&item.LineNo,
		&item.MatchedText,
		&item.Message,
		&item.Suggestion,
		&item.DetailsJSON,
		&status,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.Finding{}, err
	}
	item.Severity = domain.Severity(severity)
	item.Status = domain.FindingStatus(status)
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}
