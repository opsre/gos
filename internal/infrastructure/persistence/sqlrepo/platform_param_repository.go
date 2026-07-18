package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/platformparam"
)

type PlatformParamRepository struct {
	db       *sql.DB
	dbDriver string
}

var protectedBuiltinPlatformParamKeys = []string{
	"gos_artifact_path",
}

// NewPlatformParamRepository 创建并返回对应组件实例。
func NewPlatformParamRepository(db *sql.DB, dbDriver string) *PlatformParamRepository {
	return &PlatformParamRepository{
		db:       db,
		dbDriver: strings.ToLower(strings.TrimSpace(dbDriver)),
	}
}

// InitSchema 封装当前模块的业务处理逻辑。
func (r *PlatformParamRepository) InitSchema(ctx context.Context) error {
	var schema string
	switch r.dbDriver {
	case "mysql":
		schema = `
CREATE TABLE IF NOT EXISTS platform_param_dict (
	id VARCHAR(64) PRIMARY KEY,
	param_key VARCHAR(100) NOT NULL,
	name VARCHAR(100) NOT NULL,
	description VARCHAR(500) NOT NULL,
	param_type VARCHAR(50) NOT NULL,
	required TINYINT(1) NOT NULL,
	gitops_locator TINYINT(1) NOT NULL DEFAULT 0,
	cd_self_fill TINYINT(1) NOT NULL DEFAULT 0,
	builtin TINYINT(1) NOT NULL,
	status TINYINT(1) NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uq_platform_param_key (param_key),
	KEY idx_platform_param_status_updated_at (status, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`
	case "sqlite":
		schema = `
CREATE TABLE IF NOT EXISTS platform_param_dict (
	id TEXT PRIMARY KEY,
	param_key TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	description TEXT NOT NULL,
	param_type TEXT NOT NULL,
	required INTEGER NOT NULL,
	gitops_locator INTEGER NOT NULL DEFAULT 0,
	cd_self_fill INTEGER NOT NULL DEFAULT 0,
	builtin INTEGER NOT NULL,
	status INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}

	if _, err := r.db.ExecContext(ctx, schema); err != nil {
		return err
	}
	if r.dbDriver == "sqlite" {
		_, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_platform_param_status_updated_at ON platform_param_dict (status, updated_at);`)
		if err != nil {
			return err
		}
	}
	if err := runSchemaMigrations(ctx, r.db, r.dbDriver, schemaMigration{
		Version:     "deploy_platform_v1_1_platform_param",
		Description: "add GitOps locator and CD self-fill platform parameter fields",
		Up:          r.migrateSchema,
	}); err != nil {
		return err
	}
	return r.ensureBuiltinParams(ctx)
}

// migrateSchema 封装当前模块的业务处理逻辑。
func (r *PlatformParamRepository) migrateSchema(ctx context.Context) error {
	switch r.dbDriver {
	case "mysql":
		exists, err := r.mysqlColumnExists(ctx, "platform_param_dict", "gitops_locator")
		if err != nil {
			return err
		}
		if !exists {
			if _, err = r.db.ExecContext(
				ctx,
				`ALTER TABLE platform_param_dict ADD COLUMN gitops_locator TINYINT(1) NOT NULL DEFAULT 0 AFTER required;`,
			); err != nil {
				return err
			}
		}
		exists, err = r.mysqlColumnExists(ctx, "platform_param_dict", "cd_self_fill")
		if err != nil {
			return err
		}
		if !exists {
			if _, err = r.db.ExecContext(
				ctx,
				`ALTER TABLE platform_param_dict ADD COLUMN cd_self_fill TINYINT(1) NOT NULL DEFAULT 0 AFTER gitops_locator;`,
			); err != nil {
				return err
			}
		}
	case "sqlite":
		columns, err := r.sqliteTableColumns(ctx, "platform_param_dict")
		if err != nil {
			return err
		}
		if _, ok := columns["gitops_locator"]; !ok {
			if _, err = r.db.ExecContext(
				ctx,
				`ALTER TABLE platform_param_dict ADD COLUMN gitops_locator INTEGER NOT NULL DEFAULT 0;`,
			); err != nil {
				return err
			}
		}
		if _, ok := columns["cd_self_fill"]; !ok {
			if _, err = r.db.ExecContext(
				ctx,
				`ALTER TABLE platform_param_dict ADD COLUMN cd_self_fill INTEGER NOT NULL DEFAULT 0;`,
			); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
	return nil
}

// ensureBuiltinParams 校验前置条件，不满足时写入对应错误响应。
func (r *PlatformParamRepository) ensureBuiltinParams(ctx context.Context) error {
	now := time.Now().UTC()
	items := []domain.PlatformParamDict{
		{
			ID:            "ppd-app-key",
			ParamKey:      "app_key",
			Name:          "应用标识",
			Description:   "",
			ParamType:     domain.ParamTypeString,
			Required:      false,
			GitOpsLocator: false,
			CDSelfFill:    false,
			Builtin:       true,
			Status:        domain.StatusEnabled,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "ppd-image-version",
			ParamKey:      "image_version",
			Name:          "镜像版本",
			Description:   "平台自动分配；Jenkins CI 默认取本次构建号 BUILD_NUMBER",
			ParamType:     domain.ParamTypeString,
			Required:      false,
			GitOpsLocator: false,
			CDSelfFill:    false,
			Builtin:       true,
			Status:        domain.StatusEnabled,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "ppd-release-name",
			ParamKey:      "release_name",
			Name:          "发布名称",
			Description:   "发布单内置名称，可用于模板参数、GitOps 替换规则和通知模板",
			ParamType:     domain.ParamTypeString,
			Required:      false,
			GitOpsLocator: false,
			CDSelfFill:    false,
			Builtin:       true,
			Status:        domain.StatusEnabled,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "ppd-oss-endpoint",
			ParamKey:      "oss_endpoint",
			Name:          "OSS Endpoint",
			Description:   "平台从 App 绑定制品库注入；Jenkins 参数名建议 OSS_ENDPOINT",
			ParamType:     domain.ParamTypeString,
			Required:      false,
			GitOpsLocator: false,
			CDSelfFill:    false,
			Builtin:       true,
			Status:        domain.StatusEnabled,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "ppd-oss-bucket",
			ParamKey:      "oss_bucket",
			Name:          "OSS Bucket",
			Description:   "平台从 App 绑定制品库注入；Jenkins 参数名建议 OSS_BUCKET",
			ParamType:     domain.ParamTypeString,
			Required:      false,
			GitOpsLocator: false,
			CDSelfFill:    false,
			Builtin:       true,
			Status:        domain.StatusEnabled,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "ppd-oss-dir",
			ParamKey:      "oss_dir",
			Name:          "OSS 目录",
			Description:   "平台从 App 绑定制品库注入；Jenkins 参数名建议 OSS_DIR",
			ParamType:     domain.ParamTypeString,
			Required:      false,
			GitOpsLocator: false,
			CDSelfFill:    false,
			Builtin:       true,
			Status:        domain.StatusEnabled,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "ppd-oss-acl",
			ParamKey:      "oss_acl",
			Name:          "OSS ACL",
			Description:   "平台从 App 绑定制品库注入；Jenkins 参数名建议 OSS_ACL",
			ParamType:     domain.ParamTypeString,
			Required:      false,
			GitOpsLocator: false,
			CDSelfFill:    false,
			Builtin:       true,
			Status:        domain.StatusEnabled,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "ppd-oss-access-key-id",
			ParamKey:      "oss_access_key_id",
			Name:          "OSS AccessKey ID",
			Description:   "平台从 App 绑定制品库注入；Jenkins 参数名建议 OSS_ACCESS_KEY_ID",
			ParamType:     domain.ParamTypeString,
			Required:      false,
			GitOpsLocator: false,
			CDSelfFill:    false,
			Builtin:       true,
			Status:        domain.StatusEnabled,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "ppd-oss-access-key-secret",
			ParamKey:      "oss_access_key_secret",
			Name:          "OSS AccessKey Secret",
			Description:   "平台从 App 绑定制品库注入；Jenkins 参数名建议 OSS_ACCESS_KEY_SECRET",
			ParamType:     domain.ParamTypeString,
			Required:      false,
			GitOpsLocator: false,
			CDSelfFill:    false,
			Builtin:       true,
			Status:        domain.StatusEnabled,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "ppd-gos-artifact-url",
			ParamKey:      "gos_artifact_url",
			Name:          "GOS_ARTIFACT_URL",
			Description:   "CI 制品地址标准输出字段；Jenkins 日志需输出 GOS_ARTIFACT_URL=制品下载地址。管线扫描可识别 CI/CD 脚本中的输出格式，但发布单取值、CD 传参、GitOps 和 Hook 变量只沿用 CI 单元产出的 gos_artifact_url，不从 CD 单元取值或覆盖。",
			ParamType:     domain.ParamTypeString,
			Required:      false,
			GitOpsLocator: false,
			CDSelfFill:    false,
			Builtin:       true,
			Status:        domain.StatusEnabled,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "ppd-gos-artifact-path",
			ParamKey:      "gos_artifact_path",
			Name:          "GOS_ARTIFACT_PATH",
			Description:   "应用基础信息中的制品路径；发布模板、GitOps 替换规则和 Hook 变量可直接引用，不从发布执行日志或 CI/CD 输出取值。",
			ParamType:     domain.ParamTypeString,
			Required:      false,
			GitOpsLocator: false,
			CDSelfFill:    false,
			Builtin:       true,
			Status:        domain.StatusEnabled,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}

	// Builtin keys are platform-owned metadata. We keep them in sync on startup so the
	// UI can rely on a stable, non-editable dictionary entry without manual intervention.
	for _, item := range items {
		if err := r.upsertBuiltinParam(ctx, item); err != nil {
			return err
		}
	}
	return r.normalizeBuiltinFlags(ctx, builtinPlatformParamKeys(items), now)
}

func builtinPlatformParamKeys(items []domain.PlatformParamDict) []string {
	keys := make([]string, 0, len(items))
	for _, item := range items {
		keys = append(keys, item.ParamKey)
	}
	return keys
}

// upsertBuiltinParam 封装当前模块的业务处理逻辑。
func (r *PlatformParamRepository) upsertBuiltinParam(ctx context.Context, item domain.PlatformParamDict) error {
	switch r.dbDriver {
	case "mysql":
		const q = `
INSERT INTO platform_param_dict (
	id, param_key, name, description, param_type, required, gitops_locator, cd_self_fill, builtin, status, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	name = VALUES(name),
	description = VALUES(description),
	param_type = VALUES(param_type),
	required = VALUES(required),
	gitops_locator = VALUES(gitops_locator),
	cd_self_fill = VALUES(cd_self_fill),
	builtin = VALUES(builtin),
	status = VALUES(status),
	updated_at = VALUES(updated_at);`
		_, err := r.db.ExecContext(
			ctx,
			q,
			item.ID,
			item.ParamKey,
			item.Name,
			item.Description,
			string(item.ParamType),
			boolToInt(item.Required),
			boolToInt(item.GitOpsLocator),
			boolToInt(item.CDSelfFill),
			boolToInt(item.Builtin),
			int(item.Status),
			item.CreatedAt.UTC().UnixNano(),
			item.UpdatedAt.UTC().UnixNano(),
		)
		return err
	case "sqlite":
		const q = `
INSERT INTO platform_param_dict (
	id, param_key, name, description, param_type, required, gitops_locator, cd_self_fill, builtin, status, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(param_key) DO UPDATE SET
	name = excluded.name,
	description = excluded.description,
	param_type = excluded.param_type,
	required = excluded.required,
	gitops_locator = excluded.gitops_locator,
	cd_self_fill = excluded.cd_self_fill,
	builtin = excluded.builtin,
	status = excluded.status,
	updated_at = excluded.updated_at;`
		_, err := r.db.ExecContext(
			ctx,
			q,
			item.ID,
			item.ParamKey,
			item.Name,
			item.Description,
			string(item.ParamType),
			boolToInt(item.Required),
			boolToInt(item.GitOpsLocator),
			boolToInt(item.CDSelfFill),
			boolToInt(item.Builtin),
			int(item.Status),
			item.CreatedAt.UTC().UnixNano(),
			item.UpdatedAt.UTC().UnixNano(),
		)
		return err
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
}

// normalizeBuiltinFlags 标准化输入值，保证后续逻辑使用统一格式。
func (r *PlatformParamRepository) normalizeBuiltinFlags(ctx context.Context, builtinKeys []string, now time.Time) error {
	if len(builtinKeys) == 0 {
		return nil
	}

	placeholders := make([]string, 0, len(builtinKeys))
	args := make([]any, 0, len(builtinKeys)+1)
	args = append(args, now.UTC().UnixNano())
	for _, key := range builtinKeys {
		placeholders = append(placeholders, "?")
		args = append(args, key)
	}

	// Builtin ownership is decided by the platform seed list, not by historical UI data.
	// This keeps old manually-created rows from staying "builtin" after we removed that input.
	query := fmt.Sprintf(
		`UPDATE platform_param_dict SET builtin = 0, updated_at = ? WHERE builtin <> 0 AND param_key NOT IN (%s);`,
		strings.Join(placeholders, ", "),
	)
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// Create 创建业务资源并返回处理结果。
func (r *PlatformParamRepository) Create(ctx context.Context, item domain.PlatformParamDict) error {
	const q = `
INSERT INTO platform_param_dict (
	id, param_key, name, description, param_type, required, gitops_locator, cd_self_fill, builtin, status, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	_, err := r.db.ExecContext(
		ctx,
		q,
		item.ID,
		item.ParamKey,
		item.Name,
		item.Description,
		string(item.ParamType),
		boolToInt(item.Required),
		boolToInt(item.GitOpsLocator),
		boolToInt(item.CDSelfFill),
		boolToInt(item.Builtin),
		int(item.Status),
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isDuplicateKeyError(r.dbDriver, err) {
			return domain.ErrParamKeyDuplicated
		}
		return err
	}
	return nil
}

// GetByID 查询并返回指定资源数据。
func (r *PlatformParamRepository) GetByID(ctx context.Context, id string) (domain.PlatformParamDict, error) {
	const q = `
SELECT id, param_key, name, description, param_type, required, gitops_locator, cd_self_fill, builtin, status, created_at, updated_at
FROM platform_param_dict
WHERE id = ?;`

	row := r.db.QueryRowContext(ctx, q, id)
	item, err := scanPlatformParam(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.PlatformParamDict{}, domain.ErrNotFound
		}
		return domain.PlatformParamDict{}, err
	}
	return protectBuiltinPlatformParam(item), nil
}

// GetByParamKey 查询并返回指定资源数据。
func (r *PlatformParamRepository) GetByParamKey(ctx context.Context, paramKey string) (domain.PlatformParamDict, error) {
	const q = `
SELECT id, param_key, name, description, param_type, required, gitops_locator, cd_self_fill, builtin, status, created_at, updated_at
FROM platform_param_dict
WHERE param_key = ?;`

	row := r.db.QueryRowContext(ctx, q, paramKey)
	item, err := scanPlatformParam(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.PlatformParamDict{}, domain.ErrNotFound
		}
		return domain.PlatformParamDict{}, err
	}
	return protectBuiltinPlatformParam(item), nil
}

// List 查询并返回列表数据。
func (r *PlatformParamRepository) List(ctx context.Context, filter domain.ListFilter) ([]domain.PlatformParamDict, int64, error) {
	where := make([]string, 0, 4)
	args := make([]any, 0, 4)

	if filter.ParamKey != "" {
		where = append(where, "param_key LIKE ?")
		args = append(args, "%"+filter.ParamKey+"%")
	}
	if filter.Name != "" {
		where = append(where, "name LIKE ?")
		args = append(args, "%"+filter.Name+"%")
	}
	if filter.Status != nil {
		where = append(where, "status = ?")
		args = append(args, int(*filter.Status))
	}
	if filter.Builtin != nil {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(protectedBuiltinPlatformParamKeys)), ",")
		if *filter.Builtin {
			where = append(where, fmt.Sprintf("(builtin = ? OR param_key IN (%s))", placeholders))
			args = append(args, boolToInt(true))
		} else {
			where = append(where, fmt.Sprintf("(builtin = ? AND param_key NOT IN (%s))", placeholders))
			args = append(args, boolToInt(false))
		}
		for _, key := range protectedBuiltinPlatformParamKeys {
			args = append(args, key)
		}
	}
	if filter.GitOpsLocator != nil {
		where = append(where, "gitops_locator = ?")
		args = append(args, boolToInt(*filter.GitOpsLocator))
	}
	if filter.CDSelfFill != nil {
		where = append(where, "cd_self_fill = ?")
		args = append(args, boolToInt(*filter.CDSelfFill))
	}

	countQuery := "SELECT COUNT(1) FROM platform_param_dict"
	if len(where) > 0 {
		countQuery += " WHERE " + strings.Join(where, " AND ")
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := `
SELECT id, param_key, name, description, param_type, required, gitops_locator, cd_self_fill, builtin, status, created_at, updated_at
FROM platform_param_dict`
	if len(where) > 0 {
		listQuery += " WHERE " + strings.Join(where, " AND ")
	}
	listQuery += " ORDER BY updated_at DESC LIMIT ? OFFSET ?;"

	offset := (filter.Page - 1) * filter.PageSize
	rows, err := r.db.QueryContext(ctx, listQuery, append(args, filter.PageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]domain.PlatformParamDict, 0)
	for rows.Next() {
		item, scanErr := scanPlatformParam(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, protectBuiltinPlatformParam(item))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Update 更新业务资源并返回处理结果。
func (r *PlatformParamRepository) Update(ctx context.Context, id string, input domain.UpdateInput, updatedAt time.Time) (domain.PlatformParamDict, error) {
	const q = `
UPDATE platform_param_dict
SET param_key = ?, name = ?, description = ?, param_type = ?, required = ?, gitops_locator = ?, cd_self_fill = ?, builtin = ?, status = ?, updated_at = ?
WHERE id = ?;`

	res, err := r.db.ExecContext(
		ctx,
		q,
		input.ParamKey,
		input.Name,
		input.Description,
		string(input.ParamType),
		boolToInt(input.Required),
		boolToInt(input.GitOpsLocator),
		boolToInt(input.CDSelfFill),
		boolToInt(input.Builtin),
		int(input.Status),
		updatedAt.UTC().UnixNano(),
		id,
	)
	if err != nil {
		if isDuplicateKeyError(r.dbDriver, err) {
			return domain.PlatformParamDict{}, domain.ErrParamKeyDuplicated
		}
		return domain.PlatformParamDict{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.PlatformParamDict{}, err
	}
	if affected == 0 {
		return domain.PlatformParamDict{}, domain.ErrNotFound
	}
	return r.GetByID(ctx, id)
}

// Delete 删除业务资源并返回处理结果。
func (r *PlatformParamRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM platform_param_dict WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// scanPlatformParam 封装当前模块的业务处理逻辑。
func scanPlatformParam(s scanner) (domain.PlatformParamDict, error) {
	var (
		item          domain.PlatformParamDict
		paramType     string
		required      int
		gitopsLocator int
		cdSelfFill    int
		builtin       int
		status        int
		createdAt     int64
		updatedAt     int64
	)

	if err := s.Scan(
		&item.ID,
		&item.ParamKey,
		&item.Name,
		&item.Description,
		&paramType,
		&required,
		&gitopsLocator,
		&cdSelfFill,
		&builtin,
		&status,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.PlatformParamDict{}, err
	}

	item.ParamType = domain.ParamType(paramType)
	item.Required = required > 0
	item.GitOpsLocator = gitopsLocator > 0
	item.CDSelfFill = cdSelfFill > 0
	item.Builtin = builtin > 0
	item.Status = domain.Status(status)
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

func protectBuiltinPlatformParam(item domain.PlatformParamDict) domain.PlatformParamDict {
	if isProtectedBuiltinPlatformParamKey(item.ParamKey) {
		item.Builtin = true
	}
	return item
}

func isProtectedBuiltinPlatformParamKey(paramKey string) bool {
	key := strings.ToLower(strings.TrimSpace(paramKey))
	for _, protectedKey := range protectedBuiltinPlatformParamKeys {
		if key == protectedKey {
			return true
		}
	}
	return false
}

// mysqlColumnExists 封装当前模块的业务处理逻辑。
func (r *PlatformParamRepository) mysqlColumnExists(ctx context.Context, table, column string) (bool, error) {
	const q = `
SELECT COUNT(1)
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = ?
  AND COLUMN_NAME = ?;`
	var count int
	if err := r.db.QueryRowContext(ctx, q, table, column).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// sqliteTableColumns 封装当前模块的业务处理逻辑。
func (r *PlatformParamRepository) sqliteTableColumns(ctx context.Context, table string) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s);`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]string)
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
		columns[strings.ToLower(strings.TrimSpace(name))] = columnType
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

// boolToInt 封装当前模块的业务处理逻辑。
func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
