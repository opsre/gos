package sqlrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/notification"
)

type NotificationRepository struct {
	db       *sql.DB
	dbDriver string
}

// NewNotificationRepository 创建并返回对应组件实例。
func NewNotificationRepository(db *sql.DB, dbDriver string) *NotificationRepository {
	return &NotificationRepository{db: db, dbDriver: strings.ToLower(strings.TrimSpace(dbDriver))}
}

// InitSchema 封装当前模块的业务处理逻辑。
func (r *NotificationRepository) InitSchema(ctx context.Context) error {
	if r == nil || r.db == nil {
		return nil
	}
	stmts := r.mysqlSchemaStatements()
	if r.dbDriver == "sqlite" {
		stmts = r.sqliteSchemaStatements()
	}
	for _, stmt := range stmts {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return r.migrateSchema(ctx)
}

// mysqlSchemaStatements 封装当前模块的业务处理逻辑。
func (r *NotificationRepository) mysqlSchemaStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS notification_source (
	id VARCHAR(64) PRIMARY KEY,
	name VARCHAR(200) NOT NULL,
	source_type VARCHAR(32) NOT NULL,
	webhook_url TEXT NOT NULL,
	verification_param TEXT NOT NULL,
	enabled TINYINT(1) NOT NULL DEFAULT 1,
	remark TEXT NOT NULL,
	created_by VARCHAR(128) NOT NULL,
	updated_by VARCHAR(128) NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uk_notification_source_name (name)
);`,
		`CREATE TABLE IF NOT EXISTS notification_markdown_template (
	id VARCHAR(64) PRIMARY KEY,
	name VARCHAR(200) NOT NULL,
	title_template TEXT NOT NULL,
	body_template TEXT NOT NULL,
	conditions_json LONGTEXT NOT NULL,
	enabled TINYINT(1) NOT NULL DEFAULT 1,
	remark TEXT NOT NULL,
	created_by VARCHAR(128) NOT NULL,
	updated_by VARCHAR(128) NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uk_notification_markdown_template_name (name)
);`,
		`CREATE TABLE IF NOT EXISTS notification_hook (
	id VARCHAR(64) PRIMARY KEY,
	name VARCHAR(200) NOT NULL,
	source_id VARCHAR(64) NOT NULL,
	markdown_template_id VARCHAR(64) NOT NULL,
	enabled TINYINT(1) NOT NULL DEFAULT 1,
	remark TEXT NOT NULL,
	created_by VARCHAR(128) NOT NULL,
	updated_by VARCHAR(128) NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uk_notification_hook_name (name),
	KEY idx_notification_hook_source (source_id),
	KEY idx_notification_hook_template (markdown_template_id)
);`,
	}
}

// sqliteSchemaStatements 封装当前模块的业务处理逻辑。
func (r *NotificationRepository) sqliteSchemaStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS notification_source (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	source_type TEXT NOT NULL,
	webhook_url TEXT NOT NULL,
	verification_param TEXT NOT NULL DEFAULT '',
	enabled INTEGER NOT NULL DEFAULT 1,
	remark TEXT NOT NULL DEFAULT '',
	created_by TEXT NOT NULL DEFAULT '',
	updated_by TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
		`CREATE TABLE IF NOT EXISTS notification_markdown_template (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	title_template TEXT NOT NULL DEFAULT '',
	body_template TEXT NOT NULL DEFAULT '',
	conditions_json TEXT NOT NULL DEFAULT '[]',
	enabled INTEGER NOT NULL DEFAULT 1,
	remark TEXT NOT NULL DEFAULT '',
	created_by TEXT NOT NULL DEFAULT '',
	updated_by TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
		`CREATE TABLE IF NOT EXISTS notification_hook (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	source_id TEXT NOT NULL,
	markdown_template_id TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 1,
	remark TEXT NOT NULL DEFAULT '',
	created_by TEXT NOT NULL DEFAULT '',
	updated_by TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
		`CREATE INDEX IF NOT EXISTS idx_notification_hook_source ON notification_hook (source_id);`,
		`CREATE INDEX IF NOT EXISTS idx_notification_hook_template ON notification_hook (markdown_template_id);`,
	}
}

// migrateSchema 封装当前模块的业务处理逻辑。
func (r *NotificationRepository) migrateSchema(ctx context.Context) error {
	switch r.dbDriver {
	case "mysql":
		exists, err := r.mysqlColumnExists(ctx, "notification_source", "verification_param")
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		_, err = r.db.ExecContext(
			ctx,
			`ALTER TABLE notification_source ADD COLUMN verification_param TEXT NOT NULL AFTER webhook_url;`,
		)
		return err
	case "sqlite":
		columns, err := r.sqliteTableColumns(ctx, "notification_source")
		if err != nil {
			return err
		}
		if _, ok := columns["verification_param"]; ok {
			return nil
		}
		_, err = r.db.ExecContext(
			ctx,
			`ALTER TABLE notification_source ADD COLUMN verification_param TEXT NOT NULL DEFAULT '';`,
		)
		return err
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
}

// CreateSource 创建业务资源并返回处理结果。
func (r *NotificationRepository) CreateSource(ctx context.Context, item domain.Source) (domain.Source, error) {
	encryptedVerificationParam, err := encryptStoredSecret(item.VerificationParam)
	if err != nil {
		return domain.Source{}, err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO notification_source (
	id, name, source_type, webhook_url, verification_param, enabled, remark, created_by, updated_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		item.ID, item.Name, string(item.SourceType), item.WebhookURL, encryptedVerificationParam, notificationBoolToInt(item.Enabled), item.Remark, item.CreatedBy, item.UpdatedBy, item.CreatedAt.UTC().UnixNano(), item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		return domain.Source{}, err
	}
	return r.GetSourceByID(ctx, item.ID)
}

// UpdateSource 更新业务资源并返回处理结果。
func (r *NotificationRepository) UpdateSource(ctx context.Context, item domain.Source) (domain.Source, error) {
	encryptedVerificationParam, err := encryptStoredSecret(item.VerificationParam)
	if err != nil {
		return domain.Source{}, err
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE notification_source
SET name = ?, source_type = ?, webhook_url = ?, verification_param = ?, enabled = ?, remark = ?, updated_by = ?, updated_at = ?
WHERE id = ?;`,
		item.Name, string(item.SourceType), item.WebhookURL, encryptedVerificationParam, notificationBoolToInt(item.Enabled), item.Remark, item.UpdatedBy, item.UpdatedAt.UTC().UnixNano(), item.ID,
	)
	if err != nil {
		return domain.Source{}, err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return domain.Source{}, domain.ErrSourceNotFound
	}
	return r.GetSourceByID(ctx, item.ID)
}

// GetSourceByID 查询并返回指定资源数据。
func (r *NotificationRepository) GetSourceByID(ctx context.Context, id string) (domain.Source, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, name, source_type, webhook_url, verification_param, enabled, remark, created_by, updated_by, created_at, updated_at
FROM notification_source WHERE id = ?;`, strings.TrimSpace(id))
	item, err := scanNotificationSource(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Source{}, domain.ErrSourceNotFound
		}
		return domain.Source{}, err
	}
	return item, nil
}

// ListSources 查询并返回列表数据。
func (r *NotificationRepository) ListSources(ctx context.Context, filter domain.SourceListFilter) ([]domain.Source, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	where := []string{"1=1"}
	args := make([]any, 0)
	keyword := strings.TrimSpace(filter.Keyword)
	if keyword != "" {
		where = append(where, `(name LIKE ? OR webhook_url LIKE ?)`)
		like := "%" + keyword + "%"
		args = append(args, like, like)
	}
	if filter.Type != "" {
		where = append(where, `source_type = ?`)
		args = append(args, string(filter.Type))
	}
	if filter.Enabled != nil {
		where = append(where, `enabled = ?`)
		args = append(args, notificationBoolToInt(*filter.Enabled))
	}
	baseWhere := strings.Join(where, " AND ")
	countQuery := `SELECT COUNT(1) FROM notification_source WHERE ` + baseWhere
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := `
SELECT id, name, source_type, webhook_url, verification_param, enabled, remark, created_by, updated_by, created_at, updated_at
FROM notification_source
WHERE ` + baseWhere + `
ORDER BY updated_at DESC, created_at DESC
LIMIT ? OFFSET ?;`
	queryArgs := append(append([]any{}, args...), filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]domain.Source, 0)
	for rows.Next() {
		item, scanErr := scanNotificationSource(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// DeleteSource 删除业务资源并返回处理结果。
func (r *NotificationRepository) DeleteSource(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM notification_source WHERE id = ?;`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return domain.ErrSourceNotFound
	}
	return nil
}

// CreateMarkdownTemplate 创建业务资源并返回处理结果。
func (r *NotificationRepository) CreateMarkdownTemplate(ctx context.Context, item domain.MarkdownTemplate) (domain.MarkdownTemplate, error) {
	conditionsJSON, err := marshalMarkdownConditions(item.Conditions)
	if err != nil {
		return domain.MarkdownTemplate{}, err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO notification_markdown_template (
	id, name, title_template, body_template, conditions_json, enabled, remark, created_by, updated_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		item.ID, item.Name, item.TitleTemplate, item.BodyTemplate, conditionsJSON, notificationBoolToInt(item.Enabled), item.Remark, item.CreatedBy, item.UpdatedBy, item.CreatedAt.UTC().UnixNano(), item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		return domain.MarkdownTemplate{}, err
	}
	return r.GetMarkdownTemplateByID(ctx, item.ID)
}

// UpdateMarkdownTemplate 更新业务资源并返回处理结果。
func (r *NotificationRepository) UpdateMarkdownTemplate(ctx context.Context, item domain.MarkdownTemplate) (domain.MarkdownTemplate, error) {
	conditionsJSON, err := marshalMarkdownConditions(item.Conditions)
	if err != nil {
		return domain.MarkdownTemplate{}, err
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE notification_markdown_template
SET name = ?, title_template = ?, body_template = ?, conditions_json = ?, enabled = ?, remark = ?, updated_by = ?, updated_at = ?
WHERE id = ?;`,
		item.Name, item.TitleTemplate, item.BodyTemplate, conditionsJSON, notificationBoolToInt(item.Enabled), item.Remark, item.UpdatedBy, item.UpdatedAt.UTC().UnixNano(), item.ID,
	)
	if err != nil {
		return domain.MarkdownTemplate{}, err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return domain.MarkdownTemplate{}, domain.ErrMarkdownTemplateNotFound
	}
	return r.GetMarkdownTemplateByID(ctx, item.ID)
}

// GetMarkdownTemplateByID 查询并返回指定资源数据。
func (r *NotificationRepository) GetMarkdownTemplateByID(ctx context.Context, id string) (domain.MarkdownTemplate, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, name, title_template, body_template, conditions_json, enabled, remark, created_by, updated_by, created_at, updated_at
FROM notification_markdown_template WHERE id = ?;`, strings.TrimSpace(id))
	item, err := scanNotificationMarkdownTemplate(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.MarkdownTemplate{}, domain.ErrMarkdownTemplateNotFound
		}
		return domain.MarkdownTemplate{}, err
	}
	return item, nil
}

// ListMarkdownTemplates 查询并返回列表数据。
func (r *NotificationRepository) ListMarkdownTemplates(ctx context.Context, filter domain.MarkdownTemplateListFilter) ([]domain.MarkdownTemplate, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	where := []string{"1=1"}
	args := make([]any, 0)
	keyword := strings.TrimSpace(filter.Keyword)
	if keyword != "" {
		where = append(where, `(name LIKE ? OR body_template LIKE ? OR title_template LIKE ?)`)
		like := "%" + keyword + "%"
		args = append(args, like, like, like)
	}
	if filter.Enabled != nil {
		where = append(where, `enabled = ?`)
		args = append(args, notificationBoolToInt(*filter.Enabled))
	}
	baseWhere := strings.Join(where, " AND ")
	countQuery := `SELECT COUNT(1) FROM notification_markdown_template WHERE ` + baseWhere
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := `
SELECT id, name, title_template, body_template, conditions_json, enabled, remark, created_by, updated_by, created_at, updated_at
FROM notification_markdown_template
WHERE ` + baseWhere + `
ORDER BY updated_at DESC, created_at DESC
LIMIT ? OFFSET ?;`
	queryArgs := append(append([]any{}, args...), filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]domain.MarkdownTemplate, 0)
	for rows.Next() {
		item, scanErr := scanNotificationMarkdownTemplate(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// DeleteMarkdownTemplate 删除业务资源并返回处理结果。
func (r *NotificationRepository) DeleteMarkdownTemplate(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM notification_markdown_template WHERE id = ?;`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return domain.ErrMarkdownTemplateNotFound
	}
	return nil
}

// CreateHook 创建业务资源并返回处理结果。
func (r *NotificationRepository) CreateHook(ctx context.Context, item domain.Hook) (domain.Hook, error) {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO notification_hook (
	id, name, source_id, markdown_template_id, enabled, remark, created_by, updated_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		item.ID, item.Name, item.SourceID, item.MarkdownTemplateID, notificationBoolToInt(item.Enabled), item.Remark, item.CreatedBy, item.UpdatedBy, item.CreatedAt.UTC().UnixNano(), item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		return domain.Hook{}, err
	}
	return r.GetHookByID(ctx, item.ID)
}

// UpdateHook 更新业务资源并返回处理结果。
func (r *NotificationRepository) UpdateHook(ctx context.Context, item domain.Hook) (domain.Hook, error) {
	result, err := r.db.ExecContext(ctx, `
UPDATE notification_hook
SET name = ?, source_id = ?, markdown_template_id = ?, enabled = ?, remark = ?, updated_by = ?, updated_at = ?
WHERE id = ?;`,
		item.Name, item.SourceID, item.MarkdownTemplateID, notificationBoolToInt(item.Enabled), item.Remark, item.UpdatedBy, item.UpdatedAt.UTC().UnixNano(), item.ID,
	)
	if err != nil {
		return domain.Hook{}, err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return domain.Hook{}, domain.ErrHookNotFound
	}
	return r.GetHookByID(ctx, item.ID)
}

// GetHookByID 查询并返回指定资源数据。
func (r *NotificationRepository) GetHookByID(ctx context.Context, id string) (domain.Hook, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT h.id, h.name, h.source_id, s.name, s.source_type, h.markdown_template_id, t.name, h.enabled, h.remark, h.created_by, h.updated_by, h.created_at, h.updated_at
FROM notification_hook h
JOIN notification_source s ON s.id = h.source_id
JOIN notification_markdown_template t ON t.id = h.markdown_template_id
WHERE h.id = ?;`, strings.TrimSpace(id))
	item, err := scanNotificationHook(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Hook{}, domain.ErrHookNotFound
		}
		return domain.Hook{}, err
	}
	return item, nil
}

// ListHooks 查询并返回列表数据。
func (r *NotificationRepository) ListHooks(ctx context.Context, filter domain.HookListFilter) ([]domain.Hook, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	where := []string{"1=1"}
	args := make([]any, 0)
	keyword := strings.TrimSpace(filter.Keyword)
	if keyword != "" {
		where = append(where, `(h.name LIKE ? OR s.name LIKE ? OR t.name LIKE ?)`)
		like := "%" + keyword + "%"
		args = append(args, like, like, like)
	}
	if filter.Enabled != nil {
		where = append(where, `h.enabled = ?`)
		args = append(args, notificationBoolToInt(*filter.Enabled))
	}
	baseWhere := strings.Join(where, " AND ")
	countQuery := `SELECT COUNT(1) FROM notification_hook h JOIN notification_source s ON s.id = h.source_id JOIN notification_markdown_template t ON t.id = h.markdown_template_id WHERE ` + baseWhere
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := `
SELECT h.id, h.name, h.source_id, s.name, s.source_type, h.markdown_template_id, t.name, h.enabled, h.remark, h.created_by, h.updated_by, h.created_at, h.updated_at
FROM notification_hook h
JOIN notification_source s ON s.id = h.source_id
JOIN notification_markdown_template t ON t.id = h.markdown_template_id
WHERE ` + baseWhere + `
ORDER BY h.updated_at DESC, h.created_at DESC
LIMIT ? OFFSET ?;`
	queryArgs := append(append([]any{}, args...), filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]domain.Hook, 0)
	for rows.Next() {
		item, scanErr := scanNotificationHook(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// DeleteHook 删除业务资源并返回处理结果。
func (r *NotificationRepository) DeleteHook(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM notification_hook WHERE id = ?;`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return domain.ErrHookNotFound
	}
	return nil
}

// scanNotificationSource 封装当前模块的业务处理逻辑。
func scanNotificationSource(scanner interface{ Scan(dest ...any) error }) (domain.Source, error) {
	var item domain.Source
	var sourceType string
	var encryptedVerificationParam string
	var enabled int
	var createdAt, updatedAt int64
	if err := scanner.Scan(&item.ID, &item.Name, &sourceType, &item.WebhookURL, &encryptedVerificationParam, &enabled, &item.Remark, &item.CreatedBy, &item.UpdatedBy, &createdAt, &updatedAt); err != nil {
		return domain.Source{}, err
	}
	verificationParam, err := decryptStoredSecret(encryptedVerificationParam)
	if err != nil {
		return domain.Source{}, err
	}
	item.SourceType = domain.SourceType(sourceType)
	item.VerificationParam = verificationParam
	item.Enabled = enabled > 0
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

// scanNotificationMarkdownTemplate 封装当前模块的业务处理逻辑。
func scanNotificationMarkdownTemplate(scanner interface{ Scan(dest ...any) error }) (domain.MarkdownTemplate, error) {
	var item domain.MarkdownTemplate
	var conditionsJSON string
	var enabled int
	var createdAt, updatedAt int64
	if err := scanner.Scan(&item.ID, &item.Name, &item.TitleTemplate, &item.BodyTemplate, &conditionsJSON, &enabled, &item.Remark, &item.CreatedBy, &item.UpdatedBy, &createdAt, &updatedAt); err != nil {
		return domain.MarkdownTemplate{}, err
	}
	conditions, err := unmarshalMarkdownConditions(conditionsJSON)
	if err != nil {
		return domain.MarkdownTemplate{}, err
	}
	item.Conditions = conditions
	item.Enabled = enabled > 0
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

// scanNotificationHook 封装当前模块的业务处理逻辑。
func scanNotificationHook(scanner interface{ Scan(dest ...any) error }) (domain.Hook, error) {
	var item domain.Hook
	var sourceType string
	var enabled int
	var createdAt, updatedAt int64
	if err := scanner.Scan(&item.ID, &item.Name, &item.SourceID, &item.SourceName, &sourceType, &item.MarkdownTemplateID, &item.MarkdownTemplateName, &enabled, &item.Remark, &item.CreatedBy, &item.UpdatedBy, &createdAt, &updatedAt); err != nil {
		return domain.Hook{}, err
	}
	item.SourceType = domain.SourceType(sourceType)
	item.Enabled = enabled > 0
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

// marshalMarkdownConditions 封装当前模块的业务处理逻辑。
func marshalMarkdownConditions(items []domain.MarkdownTemplateCondition) (string, error) {
	normalized := make([]domain.MarkdownTemplateCondition, 0, len(items))
	for idx, item := range items {
		copyItem := item
		if copyItem.SortNo <= 0 {
			copyItem.SortNo = idx + 1
		}
		normalized = append(normalized, copyItem)
	}
	bytes, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal notification markdown conditions: %w", err)
	}
	return string(bytes), nil
}

// unmarshalMarkdownConditions 封装当前模块的业务处理逻辑。
func unmarshalMarkdownConditions(raw string) ([]domain.MarkdownTemplateCondition, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, nil
	}
	items := make([]domain.MarkdownTemplateCondition, 0)
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		return nil, fmt.Errorf("unmarshal notification markdown conditions: %w", err)
	}
	return items, nil
}

// notificationBoolToInt 封装当前模块的业务处理逻辑。
func notificationBoolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// mysqlColumnExists 封装当前模块的业务处理逻辑。
func (r *NotificationRepository) mysqlColumnExists(ctx context.Context, table, column string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(1)
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?;`,
		table,
		column,
	).Scan(&count)
	return count > 0, err
}

// sqliteTableColumns 封装当前模块的业务处理逻辑。
func (r *NotificationRepository) sqliteTableColumns(ctx context.Context, table string) (map[string]struct{}, error) {
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q);", table))
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
		columns[strings.TrimSpace(strings.ToLower(name))] = struct{}{}
	}
	return columns, rows.Err()
}
