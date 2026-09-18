package sqlrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/releaseautomation"
)

// releaseAutomationMigrationVersion 是本表的版本号：它排在
// 20260918_01_git_credential / 20260918_02_release_order_head_commit 之后。
const releaseAutomationMigrationVersion = "20260918_03_release_automation"

type ReleaseAutomationRepository struct {
	db       *sql.DB
	dbDriver string
}

func NewReleaseAutomationRepository(db *sql.DB, dbDriver string) *ReleaseAutomationRepository {
	return &ReleaseAutomationRepository{
		db:       db,
		dbDriver: strings.ToLower(strings.TrimSpace(dbDriver)),
	}
}

// InitSchema 建表并跑列迁移。建表语句本身是幂等的，列迁移交给 columnExists 守卫，
// 版本号只用于记录本表的迁移批次，因此每次启动都可以安全执行。
func (r *ReleaseAutomationRepository) InitSchema(ctx context.Context) error {
	return runSchemaMigrations(ctx, r.db, r.dbDriver, schemaMigration{
		Version:     releaseAutomationMigrationVersion,
		Description: "create release automation table and backfill runtime columns",
		Up:          r.createTables,
	})
}

func (r *ReleaseAutomationRepository) createTables(ctx context.Context) error {
	statement, err := releaseAutomationSchemaStatement(r.dbDriver)
	if err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, statement); err != nil {
		return err
	}
	if r.dbDriver == "sqlite" {
		if _, err := r.db.ExecContext(ctx,
			`CREATE INDEX IF NOT EXISTS idx_release_automation_enabled_checked ON release_automation (enabled, last_checked_at);`,
		); err != nil {
			return err
		}
	}
	return r.migrateSchema(ctx)
}

func releaseAutomationSchemaStatement(dbDriver string) (string, error) {
	switch dbDriver {
	case "mysql":
		return `
CREATE TABLE IF NOT EXISTS release_automation (
	id VARCHAR(64) PRIMARY KEY,
	name VARCHAR(100) NOT NULL,
	application_id VARCHAR(64) NOT NULL,
	application_name VARCHAR(128) NOT NULL DEFAULT '',
	template_id VARCHAR(64) NOT NULL,
	template_name VARCHAR(128) NOT NULL DEFAULT '',
	env_code VARCHAR(64) NOT NULL DEFAULT '',
	git_ref VARCHAR(255) NOT NULL DEFAULT '',
	dispatch_mode VARCHAR(32) NOT NULL DEFAULT 'build',
	enabled TINYINT(1) NOT NULL DEFAULT 1,
	params_json TEXT NOT NULL,
	remark VARCHAR(500) NOT NULL DEFAULT '',
	last_seen_sha VARCHAR(64) NOT NULL DEFAULT '',
	last_triggered_sha VARCHAR(64) NOT NULL DEFAULT '',
	last_order_id VARCHAR(64) NOT NULL DEFAULT '',
	last_checked_at BIGINT NULL,
	last_error VARCHAR(1000) NOT NULL DEFAULT '',
	creator_user_id VARCHAR(64) NOT NULL DEFAULT '',
	creator_name VARCHAR(128) NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uk_release_automation_app_env_ref (application_id, env_code, git_ref),
	KEY idx_release_automation_enabled_checked (enabled, last_checked_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`, nil
	case "sqlite":
		return `
CREATE TABLE IF NOT EXISTS release_automation (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	application_id TEXT NOT NULL,
	application_name TEXT NOT NULL DEFAULT '',
	template_id TEXT NOT NULL,
	template_name TEXT NOT NULL DEFAULT '',
	env_code TEXT NOT NULL DEFAULT '',
	git_ref TEXT NOT NULL DEFAULT '',
	dispatch_mode TEXT NOT NULL DEFAULT 'build',
	enabled INTEGER NOT NULL DEFAULT 1,
	params_json TEXT NOT NULL DEFAULT '[]',
	remark TEXT NOT NULL DEFAULT '',
	last_seen_sha TEXT NOT NULL DEFAULT '',
	last_triggered_sha TEXT NOT NULL DEFAULT '',
	last_order_id TEXT NOT NULL DEFAULT '',
	last_checked_at INTEGER NULL,
	last_error TEXT NOT NULL DEFAULT '',
	creator_user_id TEXT NOT NULL DEFAULT '',
	creator_name TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	UNIQUE (application_id, env_code, git_ref)
);`, nil
	default:
		return "", fmt.Errorf("unsupported db driver: %s", dbDriver)
	}
}

// migrateSchema 为已经部署过的表补列。新装环境上 CREATE 已经建好所有列，
// 这些语句会被 columnExists 跳过，因此不需要区分环境。
func (r *ReleaseAutomationRepository) migrateSchema(ctx context.Context) error {
	for _, migration := range releaseAutomationColumnMigrations(r.dbDriver) {
		exists, err := r.columnExists(ctx, migration.column)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := r.db.ExecContext(ctx, migration.ddl); err != nil {
			return err
		}
	}
	return nil
}

type releaseAutomationColumnMigration struct {
	column string
	ddl    string
}

// releaseAutomationColumnMigrations 覆盖「配置创建者」两列：它们让自动建单可以带上
// 发起人，是发布单可追溯性的前提。SQLite 不允许 ADD COLUMN 无默认值的 NOT NULL 列，
// 所以两种方言各自给出带默认值的语句。
func releaseAutomationColumnMigrations(dbDriver string) []releaseAutomationColumnMigration {
	switch dbDriver {
	case "mysql":
		return []releaseAutomationColumnMigration{
			{"creator_user_id", `ALTER TABLE release_automation ADD COLUMN creator_user_id VARCHAR(64) NOT NULL DEFAULT '' AFTER last_error;`},
			{"creator_name", `ALTER TABLE release_automation ADD COLUMN creator_name VARCHAR(128) NOT NULL DEFAULT '' AFTER creator_user_id;`},
		}
	case "sqlite":
		return []releaseAutomationColumnMigration{
			{"creator_user_id", `ALTER TABLE release_automation ADD COLUMN creator_user_id TEXT NOT NULL DEFAULT '';`},
			{"creator_name", `ALTER TABLE release_automation ADD COLUMN creator_name TEXT NOT NULL DEFAULT '';`},
		}
	default:
		return nil
	}
}

func (r *ReleaseAutomationRepository) Create(ctx context.Context, item domain.Automation) error {
	params, err := marshalReleaseAutomationParams(item.Params)
	if err != nil {
		return err
	}
	const q = `
INSERT INTO release_automation (
	id, name, application_id, application_name, template_id, template_name, env_code, git_ref,
	dispatch_mode, enabled, params_json, remark, last_seen_sha, last_triggered_sha, last_order_id,
	last_checked_at, last_error, creator_user_id, creator_name, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err = r.db.ExecContext(
		ctx,
		q,
		item.ID,
		item.Name,
		item.ApplicationID,
		item.ApplicationName,
		item.TemplateID,
		item.TemplateName,
		item.EnvCode,
		item.GitRef,
		string(item.DispatchMode),
		boolToDBValue(r.dbDriver, item.Enabled),
		params,
		item.Remark,
		item.LastSeenSHA,
		item.LastTriggeredSHA,
		item.LastOrderID,
		nullableUnixNano(item.LastCheckedAt),
		item.LastError,
		item.CreatorUserID,
		item.CreatorName,
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isDuplicateKeyError(r.dbDriver, err) {
			return domain.ErrDuplicated
		}
		return err
	}
	return nil
}

func (r *ReleaseAutomationRepository) GetByID(ctx context.Context, id string) (domain.Automation, error) {
	const q = `
SELECT id, name, application_id, application_name, template_id, template_name, env_code, git_ref,
	dispatch_mode, enabled, params_json, remark, last_seen_sha, last_triggered_sha, last_order_id,
	last_checked_at, last_error, creator_user_id, creator_name, created_at, updated_at
FROM release_automation
WHERE id = ?;`
	item, err := scanReleaseAutomation(r.db.QueryRowContext(ctx, q, strings.TrimSpace(id)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Automation{}, domain.ErrNotFound
		}
		return domain.Automation{}, err
	}
	return item, nil
}

func (r *ReleaseAutomationRepository) List(ctx context.Context, filter domain.ListFilter) ([]domain.Automation, int64, error) {
	where, args := buildReleaseAutomationWhere(r.dbDriver, filter)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM release_automation`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	q := `
SELECT id, name, application_id, application_name, template_id, template_name, env_code, git_ref,
	dispatch_mode, enabled, params_json, remark, last_seen_sha, last_triggered_sha, last_order_id,
	last_checked_at, last_error, creator_user_id, creator_name, created_at, updated_at
FROM release_automation` + where + `
ORDER BY updated_at DESC, created_at DESC
LIMIT ? OFFSET ?;`
	rows, err := r.db.QueryContext(ctx, q, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]domain.Automation, 0)
	for rows.Next() {
		item, scanErr := scanReleaseAutomation(rows)
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

// ListEnabled 按「最久没检查的优先」返回启用中的配置：limit 是硬上限，避免一次
// 轮询把大量配置的 git 读都拉起来。
func (r *ReleaseAutomationRepository) ListEnabled(ctx context.Context, limit int) ([]domain.Automation, error) {
	if limit <= 0 {
		return []domain.Automation{}, nil
	}
	q := `
SELECT id, name, application_id, application_name, template_id, template_name, env_code, git_ref,
	dispatch_mode, enabled, params_json, remark, last_seen_sha, last_triggered_sha, last_order_id,
	last_checked_at, last_error, creator_user_id, creator_name, created_at, updated_at
FROM release_automation
WHERE enabled = ?
ORDER BY last_checked_at IS NULL DESC, last_checked_at ASC, created_at ASC
LIMIT ?;`
	rows, err := r.db.QueryContext(ctx, q, boolToDBValue(r.dbDriver, true), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]domain.Automation, 0)
	for rows.Next() {
		item, scanErr := scanReleaseAutomation(rows)
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

// Update 写入配置本身。它有意不动 last_seen_sha / last_triggered_sha / last_order_id /
// last_checked_at：这些是轮询的运行态，只有 CommitTrigger 和 UpdateCheckState 能改，
// 否则一次编辑就会把「已处理到哪个提交」抹掉。
func (r *ReleaseAutomationRepository) Update(ctx context.Context, item domain.Automation) error {
	params, err := marshalReleaseAutomationParams(item.Params)
	if err != nil {
		return err
	}
	const q = `
UPDATE release_automation
SET name = ?, application_id = ?, application_name = ?, template_id = ?, template_name = ?,
	env_code = ?, git_ref = ?, dispatch_mode = ?, enabled = ?, params_json = ?, remark = ?,
	updated_at = ?
WHERE id = ?;`
	result, err := r.db.ExecContext(
		ctx,
		q,
		item.Name,
		item.ApplicationID,
		item.ApplicationName,
		item.TemplateID,
		item.TemplateName,
		item.EnvCode,
		item.GitRef,
		string(item.DispatchMode),
		boolToDBValue(r.dbDriver, item.Enabled),
		params,
		item.Remark,
		item.UpdatedAt.UTC().UnixNano(),
		item.ID,
	)
	if err != nil {
		if isDuplicateKeyError(r.dbDriver, err) {
			return domain.ErrDuplicated
		}
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		if _, getErr := r.GetByID(ctx, item.ID); errors.Is(getErr, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
	}
	return nil
}

func (r *ReleaseAutomationRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM release_automation WHERE id = ?;`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ResetBaseline 是「换了应用/环境/分支」时唯一允许改写基线的入口（CommitTrigger 之外）。
// 身份与前值双重校验：只要轮询已经推进过这一行，本次就不覆盖。
func (r *ReleaseAutomationRepository) ResetBaseline(
	ctx context.Context,
	id string,
	applicationID string,
	envCode string,
	gitRef string,
	expectedSeenSHA string,
	seenSHA string,
	updatedAt time.Time,
) (bool, error) {
	const q = `
UPDATE release_automation
SET last_seen_sha = ?, last_triggered_sha = '', last_order_id = '', updated_at = ?
WHERE id = ? AND application_id = ? AND env_code = ? AND git_ref = ? AND last_seen_sha = ?;`
	result, err := r.db.ExecContext(
		ctx,
		q,
		seenSHA,
		updatedAt.UTC().UnixNano(),
		strings.TrimSpace(id),
		applicationID,
		envCode,
		gitRef,
		expectedSeenSHA,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *ReleaseAutomationRepository) UpdateCheckState(ctx context.Context, id string, checkedAt time.Time, lastError string) error {
	const q = `
UPDATE release_automation
SET last_checked_at = ?, last_error = ?
WHERE id = ?;`
	result, err := r.db.ExecContext(ctx, q, checkedAt.UTC().UnixNano(), lastError, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		if _, getErr := r.GetByID(ctx, id); errors.Is(getErr, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
	}
	return nil
}

// CommitTrigger 是本模块唯一的基线推进入口，条件更新保证多副本下只有一个实例能把
// 同一轮 HEAD 认领成自己的发布单。
func (r *ReleaseAutomationRepository) CommitTrigger(
	ctx context.Context,
	id string,
	expectedSeenSHA string,
	seenSHA string,
	triggeredSHA string,
	orderID string,
	checkedAt time.Time,
	lastError string,
) (bool, error) {
	const q = `
UPDATE release_automation
SET last_seen_sha = ?, last_triggered_sha = ?, last_order_id = ?, last_checked_at = ?, last_error = ?
WHERE id = ? AND last_seen_sha = ?;`
	result, err := r.db.ExecContext(
		ctx,
		q,
		seenSHA,
		triggeredSHA,
		orderID,
		checkedAt.UTC().UnixNano(),
		lastError,
		strings.TrimSpace(id),
		expectedSeenSHA,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func buildReleaseAutomationWhere(dbDriver string, filter domain.ListFilter) (string, []any) {
	conditions := make([]string, 0, 3)
	args := make([]any, 0, 6)
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		conditions = append(conditions, "(name LIKE ? OR application_name LIKE ? OR template_name LIKE ? OR env_code LIKE ? OR git_ref LIKE ?)")
		pattern := "%" + keyword + "%"
		args = append(args, pattern, pattern, pattern, pattern, pattern)
	}
	if applicationID := strings.TrimSpace(filter.ApplicationID); applicationID != "" {
		conditions = append(conditions, "application_id = ?")
		args = append(args, applicationID)
	}
	if filter.Enabled != nil {
		conditions = append(conditions, "enabled = ?")
		args = append(args, boolToDBValue(dbDriver, *filter.Enabled))
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

type releaseAutomationScanner interface {
	Scan(dest ...any) error
}

func scanReleaseAutomation(scanner releaseAutomationScanner) (domain.Automation, error) {
	var (
		item          domain.Automation
		dispatchMode  string
		enabled       any
		paramsJSON    string
		lastCheckedAt sql.NullInt64
		createdAt     int64
		updatedAt     int64
	)
	if err := scanner.Scan(
		&item.ID,
		&item.Name,
		&item.ApplicationID,
		&item.ApplicationName,
		&item.TemplateID,
		&item.TemplateName,
		&item.EnvCode,
		&item.GitRef,
		&dispatchMode,
		&enabled,
		&paramsJSON,
		&item.Remark,
		&item.LastSeenSHA,
		&item.LastTriggeredSHA,
		&item.LastOrderID,
		&lastCheckedAt,
		&item.LastError,
		&item.CreatorUserID,
		&item.CreatorName,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.Automation{}, err
	}
	params, err := unmarshalReleaseAutomationParams(paramsJSON)
	if err != nil {
		return domain.Automation{}, err
	}
	item.DispatchMode = domain.DispatchMode(strings.TrimSpace(dispatchMode))
	item.Enabled = scanBoolValue(enabled)
	item.Params = params
	// last_checked_at 可空：从未检查过的配置必须保持 nil，不能落成 0 值时间。
	if lastCheckedAt.Valid {
		checkedAt := time.Unix(0, lastCheckedAt.Int64).UTC()
		item.LastCheckedAt = &checkedAt
	}
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

// marshalReleaseAutomationParams 把参数组合序列化成一段 JSON。空参数存 "[]"，
// 读取端不需要区分 NULL 与空数组。
func marshalReleaseAutomationParams(params []domain.Param) (string, error) {
	if len(params) == 0 {
		return "[]", nil
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func unmarshalReleaseAutomationParams(raw string) ([]domain.Param, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []domain.Param{}, nil
	}
	params := make([]domain.Param, 0)
	if err := json.Unmarshal([]byte(text), &params); err != nil {
		return nil, fmt.Errorf("decode release automation params: %w", err)
	}
	return params, nil
}

// columnExists 报告列是否已经存在。MySQL 与 SQLite 走不同 catalog，和
// ArtifactRepositoryConfigRepository 保持同一实现方式。
func (r *ReleaseAutomationRepository) columnExists(ctx context.Context, column string) (bool, error) {
	const table = "release_automation"
	if r.dbDriver == "sqlite" {
		columns, err := r.sqliteTableColumns(ctx, table)
		if err != nil {
			return false, err
		}
		_, ok := columns[column]
		return ok, nil
	}
	const q = `SELECT COUNT(1)
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?;`
	var count int
	if err := r.db.QueryRowContext(ctx, q, table, column).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *ReleaseAutomationRepository) sqliteTableColumns(ctx context.Context, table string) (map[string]struct{}, error) {
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q);", table))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	columns := make(map[string]struct{})
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			return nil, err
		}
		columns[strings.TrimSpace(strings.ToLower(name))] = struct{}{}
	}
	return columns, rows.Err()
}
