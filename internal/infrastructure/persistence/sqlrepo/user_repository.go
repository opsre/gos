package sqlrepo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"

	domain "gos/internal/domain/user"
)

type UserRepository struct {
	db       *sql.DB
	dbDriver string
}

// NewUserRepository 创建并返回对应组件实例。
func NewUserRepository(db *sql.DB, dbDriver string) *UserRepository {
	return &UserRepository{
		db:       db,
		dbDriver: strings.ToLower(strings.TrimSpace(dbDriver)),
	}
}

// InitSchema 封装当前模块的业务处理逻辑。
func (r *UserRepository) InitSchema(ctx context.Context) error {
	stmts, err := r.schemaStatements()
	if err != nil {
		return err
	}
	for _, stmt := range stmts {
		if _, execErr := r.db.ExecContext(ctx, stmt); execErr != nil {
			return execErr
		}
	}
	return runSchemaMigrations(
		ctx,
		r.db,
		r.dbDriver,
		schemaMigration{
			Version:     "deploy_platform_v1_1_user_session",
			Description: "add user session revocation fields",
			Up:          r.migrateUserSessionSchema,
		},
		schemaMigration{
			Version:     "20260717_01_user_manager",
			Description: "create direct user manager relationships",
			Up:          r.ensureUserManagerSchema,
		},
	)
}

// schemaStatements 封装当前模块的业务处理逻辑。
func (r *UserRepository) schemaStatements() ([]string, error) {
	switch r.dbDriver {
	case "mysql":
		return []string{
			`CREATE TABLE IF NOT EXISTS sys_user (
	id VARCHAR(64) PRIMARY KEY,
	username VARCHAR(100) NOT NULL,
	display_name VARCHAR(100) NOT NULL,
	email VARCHAR(200) NOT NULL DEFAULT '',
	phone VARCHAR(50) NOT NULL DEFAULT '',
	role VARCHAR(20) NOT NULL,
	status VARCHAR(20) NOT NULL DEFAULT 'active',
	password_hash VARCHAR(255) NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uk_sys_user_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS sys_permission (
	id VARCHAR(64) PRIMARY KEY,
	code VARCHAR(100) NOT NULL,
	name VARCHAR(100) NOT NULL,
	module VARCHAR(50) NOT NULL,
	action VARCHAR(50) NOT NULL,
	description VARCHAR(500) NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uk_sys_permission_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS sys_user_permission (
	id VARCHAR(64) PRIMARY KEY,
	user_id VARCHAR(64) NOT NULL,
	permission_code VARCHAR(100) NOT NULL,
	scope_type VARCHAR(30) NOT NULL DEFAULT 'global',
	scope_value VARCHAR(200) NOT NULL DEFAULT '',
	enabled TINYINT(1) NOT NULL DEFAULT 1,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uk_sup_unique (user_id, permission_code, scope_type, scope_value),
	KEY idx_sup_user (user_id),
	KEY idx_sup_code (permission_code),
	KEY idx_sup_scope (scope_type, scope_value)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS sys_user_param_permission (
	id VARCHAR(64) PRIMARY KEY,
	user_id VARCHAR(64) NOT NULL,
	param_key VARCHAR(100) NOT NULL,
	application_id VARCHAR(64) NOT NULL DEFAULT '',
	can_view TINYINT(1) NOT NULL DEFAULT 0,
	can_edit TINYINT(1) NOT NULL DEFAULT 0,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uk_supp_unique (user_id, param_key, application_id),
	KEY idx_supp_user (user_id),
	KEY idx_supp_param (param_key),
	KEY idx_supp_app (application_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS sys_user_session (
	id VARCHAR(64) PRIMARY KEY,
	user_id VARCHAR(64) NOT NULL,
	access_token VARCHAR(512) NOT NULL,
	expired_at BIGINT NOT NULL,
	client_ip VARCHAR(64) NOT NULL DEFAULT '',
	user_agent VARCHAR(300) NOT NULL DEFAULT '',
	revoked_at BIGINT NULL,
	revoked_reason VARCHAR(64) NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL,
	UNIQUE KEY uk_sus_token (access_token),
	KEY idx_sus_user (user_id),
	KEY idx_sus_expired (expired_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		}, nil
	case "sqlite":
		return []string{
			`CREATE TABLE IF NOT EXISTS sys_user (
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
			`CREATE TABLE IF NOT EXISTS sys_permission (
	id TEXT PRIMARY KEY,
	code TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	module TEXT NOT NULL,
	action TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
			`CREATE TABLE IF NOT EXISTS sys_user_permission (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	permission_code TEXT NOT NULL,
	scope_type TEXT NOT NULL DEFAULT 'global',
	scope_value TEXT NOT NULL DEFAULT '',
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	UNIQUE(user_id, permission_code, scope_type, scope_value)
);`,
			`CREATE INDEX IF NOT EXISTS idx_sup_user ON sys_user_permission (user_id);`,
			`CREATE INDEX IF NOT EXISTS idx_sup_code ON sys_user_permission (permission_code);`,
			`CREATE INDEX IF NOT EXISTS idx_sup_scope ON sys_user_permission (scope_type, scope_value);`,
			`CREATE TABLE IF NOT EXISTS sys_user_param_permission (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	param_key TEXT NOT NULL,
	application_id TEXT NOT NULL DEFAULT '',
	can_view INTEGER NOT NULL DEFAULT 0,
	can_edit INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	UNIQUE(user_id, param_key, application_id)
);`,
			`CREATE INDEX IF NOT EXISTS idx_supp_user ON sys_user_param_permission (user_id);`,
			`CREATE INDEX IF NOT EXISTS idx_supp_param ON sys_user_param_permission (param_key);`,
			`CREATE INDEX IF NOT EXISTS idx_supp_app ON sys_user_param_permission (application_id);`,
			`CREATE TABLE IF NOT EXISTS sys_user_session (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	access_token TEXT NOT NULL UNIQUE,
	expired_at INTEGER NOT NULL,
	client_ip TEXT NOT NULL DEFAULT '',
	user_agent TEXT NOT NULL DEFAULT '',
	revoked_at INTEGER NULL,
	revoked_reason TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);`,
			`CREATE INDEX IF NOT EXISTS idx_sus_user ON sys_user_session (user_id);`,
			`CREATE INDEX IF NOT EXISTS idx_sus_expired ON sys_user_session (expired_at);`,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
}

func (r *UserRepository) ensureUserManagerSchema(ctx context.Context) error {
	var statements []string
	switch r.dbDriver {
	case "mysql":
		statements = []string{`CREATE TABLE IF NOT EXISTS sys_user_manager (
	user_id VARCHAR(64) PRIMARY KEY,
	manager_user_id VARCHAR(64) NOT NULL,
	updated_at BIGINT NOT NULL,
	KEY idx_sum_manager (manager_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`}
	case "sqlite":
		statements = []string{
			`CREATE TABLE IF NOT EXISTS sys_user_manager (
	user_id TEXT PRIMARY KEY,
	manager_user_id TEXT NOT NULL,
	updated_at INTEGER NOT NULL
);`,
			`CREATE INDEX IF NOT EXISTS idx_sum_manager ON sys_user_manager (manager_user_id);`,
		}
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
	for _, statement := range statements {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// migrateUserSessionSchema 补齐历史 session 表字段。
func (r *UserRepository) migrateUserSessionSchema(ctx context.Context) error {
	switch r.dbDriver {
	case "mysql":
		exists, err := r.mysqlColumnExists(ctx, "sys_user_session", "revoked_at")
		if err != nil {
			return err
		}
		if !exists {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE sys_user_session ADD COLUMN revoked_at BIGINT NULL AFTER user_agent;`); err != nil {
				return err
			}
		}
		exists, err = r.mysqlColumnExists(ctx, "sys_user_session", "revoked_reason")
		if err != nil {
			return err
		}
		if !exists {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE sys_user_session ADD COLUMN revoked_reason VARCHAR(64) NOT NULL DEFAULT '' AFTER revoked_at;`); err != nil {
				return err
			}
		}
	case "sqlite":
		columns, err := r.sqliteTableColumns(ctx, "sys_user_session")
		if err != nil {
			return err
		}
		if _, ok := columns["revoked_at"]; !ok {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE sys_user_session ADD COLUMN revoked_at INTEGER NULL;`); err != nil {
				return err
			}
		}
		if _, ok := columns["revoked_reason"]; !ok {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE sys_user_session ADD COLUMN revoked_reason TEXT NOT NULL DEFAULT '';`); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
	return nil
}

// mysqlColumnExists 封装当前模块的业务处理逻辑。
func (r *UserRepository) mysqlColumnExists(ctx context.Context, table string, column string) (bool, error) {
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
func (r *UserRepository) sqliteTableColumns(ctx context.Context, table string) (map[string]struct{}, error) {
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s);`, table))
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
		columns[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

// EnsureSeedData 校验前置条件，不满足时写入对应错误响应。
func (r *UserRepository) EnsureSeedData(
	ctx context.Context,
	adminUsername string,
	adminDisplayName string,
	adminPasswordHash string,
	now time.Time,
) error {
	if strings.TrimSpace(adminUsername) == "" || strings.TrimSpace(adminPasswordHash) == "" {
		return nil
	}

	if err := r.ensureBuiltinPermissions(ctx, now); err != nil {
		return err
	}

	username := strings.TrimSpace(adminUsername)
	var exists int
	const q = `SELECT COUNT(1) FROM sys_user WHERE username = ?;`
	if err := r.db.QueryRowContext(ctx, q, username).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}

	if strings.TrimSpace(adminDisplayName) == "" {
		adminDisplayName = "Administrator"
	}

	item := domain.User{
		ID:           "usr-admin",
		Username:     username,
		DisplayName:  strings.TrimSpace(adminDisplayName),
		Role:         domain.RoleAdmin,
		Status:       domain.StatusActive,
		PasswordHash: strings.TrimSpace(adminPasswordHash),
		CreatedAt:    now.UTC(),
		UpdatedAt:    now.UTC(),
	}
	return r.CreateUser(ctx, item)
}

// ensureBuiltinPermissions 校验前置条件，不满足时写入对应错误响应。
func (r *UserRepository) ensureBuiltinPermissions(ctx context.Context, now time.Time) error {
	permissions := []domain.Permission{
		{ID: "perm-application-view", Code: "application.view", Name: "查看应用", Module: "application", Action: "view", Description: "查看应用与详情"},
		{ID: "perm-application-manage", Code: "application.manage", Name: "管理应用", Module: "application", Action: "manage", Description: "创建/编辑/删除应用"},
		{ID: "perm-pipeline-view", Code: "pipeline.view", Name: "查看管线", Module: "pipeline", Action: "view", Description: "查看管线和绑定"},
		{ID: "perm-pipeline-manage", Code: "pipeline.manage", Name: "管理管线", Module: "pipeline", Action: "manage", Description: "编辑管线绑定"},
		{ID: "perm-platform-param-manage", Code: "platform_param.manage", Name: "管理标准字库", Module: "platform_param", Action: "manage", Description: "标准字库增删改查"},
		{ID: "perm-pipeline-param-manage", Code: "pipeline_param.manage", Name: "管理管线参数", Module: "pipeline_param", Action: "manage", Description: "管线参数映射维护"},
		{ID: "perm-artifact-repo-manage", Code: "artifact_repo.manage", Name: "管理制品库", Module: "artifact_repo", Action: "manage", Description: "制品库配置增删改查"},
		{ID: "perm-component-view", Code: "component.view", Name: "查看组件管理", Module: "component", Action: "view", Description: "访问组件管理模块"},
		{ID: "perm-component-argocd-view", Code: "component.argocd.view", Name: "查看ArgoCD管理", Module: "component", Action: "argocd_view", Description: "查看 ArgoCD 应用列表与详情"},
		{ID: "perm-component-argocd-manage", Code: "component.argocd.manage", Name: "管理ArgoCD", Module: "component", Action: "argocd_manage", Description: "执行 ArgoCD 手动同步与连接检查"},
		{ID: "perm-component-argocd-instance-view", Code: "component.argocd.instance.view", Name: "查看ArgoCD实例", Module: "component", Action: "argocd_instance_view", Description: "查看 ArgoCD 实例与环境绑定"},
		{ID: "perm-component-argocd-instance-manage", Code: "component.argocd.instance.manage", Name: "管理ArgoCD实例", Module: "component", Action: "argocd_instance_manage", Description: "新增编辑 ArgoCD 实例与连接检查"},
		{ID: "perm-component-argocd-binding-view", Code: "component.argocd.binding.view", Name: "查看ArgoCD环境绑定", Module: "component", Action: "argocd_binding_view", Description: "查看环境与 ArgoCD 的绑定关系"},
		{ID: "perm-component-argocd-binding-manage", Code: "component.argocd.binding.manage", Name: "管理ArgoCD环境绑定", Module: "component", Action: "argocd_binding_manage", Description: "维护环境与 ArgoCD 的绑定关系"},
		{ID: "perm-component-gitops-view", Code: "component.gitops.view", Name: "查看GitOps管理", Module: "component", Action: "gitops_view", Description: "查看 GitOps 仓库工作区状态"},
		{ID: "perm-component-gitops-manage", Code: "component.gitops.manage", Name: "管理GitOps", Module: "component", Action: "gitops_manage", Description: "编辑 GitOps 提交信息模版"},
		{ID: "perm-component-agent-view", Code: "component.agent.view", Name: "查看Agent管理", Module: "component", Action: "agent_view", Description: "查看 Agent 列表、心跳和任务状态"},
		{ID: "perm-component-agent-manage", Code: "component.agent.manage", Name: "管理Agent", Module: "component", Action: "agent_manage", Description: "新增、编辑和维护 Agent 实例"},
		{ID: "perm-release-view", Code: "release.view", Name: "查看发布单", Module: "release", Action: "view", Description: "查看发布单列表/详情"},
		{ID: "perm-release-param-snapshot-view", Code: "release.param_snapshot.view", Name: "查看参数快照", Module: "release", Action: "param_snapshot_view", Description: "查看发布详情中的参数快照"},
		{ID: "perm-release-template-manage", Code: "release.template.manage", Name: "管理发布模板", Module: "release", Action: "template_manage", Description: "发布模板增删改查"},
		{ID: "perm-release-create", Code: "release.create", Name: "创建发布单", Module: "release", Action: "create", Description: "创建发布单"},
		{ID: "perm-release-approval-view", Code: "release.approval.view", Name: "查看审批流", Module: "release", Action: "approval_view", Description: "查看发布审批配置与审批记录"},
		{ID: "perm-release-approval-submit", Code: "release.approval.submit", Name: "提交审批", Module: "release", Action: "approval_submit", Description: "提交发布单进入审批流"},
		{ID: "perm-release-approval-approve", Code: "release.approval.approve", Name: "审批通过", Module: "release", Action: "approval_approve", Description: "对发布单执行审批通过"},
		{ID: "perm-release-approval-reject", Code: "release.approval.reject", Name: "审批拒绝", Module: "release", Action: "approval_reject", Description: "对发布单执行审批拒绝"},
		{ID: "perm-release-param-config-view", Code: "release.param_config.view", Name: "展示额外参数配置", Module: "release", Action: "param_config_view", Description: "控制新建发布单页面额外参数区域展示"},
		{ID: "perm-release-execute", Code: "release.execute", Name: "执行发布单", Module: "release", Action: "execute", Description: "执行发布操作"},
		{ID: "perm-release-cancel", Code: "release.cancel", Name: "取消发布单", Module: "release", Action: "cancel", Description: "取消发布操作"},
		{ID: "perm-system-user-manage", Code: "system.user.manage", Name: "管理用户", Module: "system", Action: "user_manage", Description: "用户管理"},
		{ID: "perm-system-permission-manage", Code: "system.permission.manage", Name: "管理权限", Module: "system", Action: "permission_manage", Description: "授权管理"},
		{ID: "perm-system-notification-manage", Code: "system.notification.manage", Name: "管理通知", Module: "system", Action: "notification_manage", Description: "通知源、Markdown 模板与通知 Hook 管理"},
	}

	const upsert = `
INSERT INTO sys_permission (id, code, name, module, action, description, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
name = VALUES(name), module = VALUES(module), action = VALUES(action), description = VALUES(description), updated_at = VALUES(updated_at);`
	const sqliteUpsert = `
INSERT INTO sys_permission (id, code, name, module, action, description, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(code) DO UPDATE SET
name = excluded.name, module = excluded.module, action = excluded.action, description = excluded.description, updated_at = excluded.updated_at;`

	for _, item := range permissions {
		stmt := upsert
		if r.dbDriver == "sqlite" {
			stmt = sqliteUpsert
		}
		if _, err := r.db.ExecContext(
			ctx,
			stmt,
			item.ID,
			item.Code,
			item.Name,
			item.Module,
			item.Action,
			item.Description,
			now.UTC().UnixNano(),
			now.UTC().UnixNano(),
		); err != nil {
			return err
		}
	}
	return nil
}

// CreateUser 创建业务资源并返回处理结果。
func (r *UserRepository) CreateUser(ctx context.Context, item domain.User) error {
	const q = `
INSERT INTO sys_user (
	id, username, display_name, email, phone, role, status, password_hash, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	_, err := r.db.ExecContext(
		ctx, q,
		item.ID,
		item.Username,
		item.DisplayName,
		item.Email,
		item.Phone,
		string(item.Role),
		string(item.Status),
		item.PasswordHash,
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isDuplicateUserError(r.dbDriver, err) {
			return domain.ErrUsernameDuplicated
		}
		return err
	}
	return nil
}

// GetUserByID 查询并返回指定资源数据。
func (r *UserRepository) GetUserByID(ctx context.Context, id string) (domain.User, error) {
	const q = `
SELECT id, username, display_name, email, phone, role, status, password_hash, created_at, updated_at
FROM sys_user
WHERE id = ?;`
	row := r.db.QueryRowContext(ctx, q, strings.TrimSpace(id))
	item, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, err
	}
	return item, nil
}

// GetUserByUsername 查询并返回指定资源数据。
func (r *UserRepository) GetUserByUsername(ctx context.Context, username string) (domain.User, error) {
	const q = `
SELECT id, username, display_name, email, phone, role, status, password_hash, created_at, updated_at
FROM sys_user
WHERE username = ?;`
	row := r.db.QueryRowContext(ctx, q, strings.TrimSpace(username))
	item, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, err
	}
	return item, nil
}

// ListUsers 查询并返回列表数据。
func (r *UserRepository) ListUsers(ctx context.Context, filter domain.UserListFilter) ([]domain.User, int64, error) {
	args := make([]any, 0, 4)
	where := make([]string, 0, 4)
	if username := strings.TrimSpace(filter.Username); username != "" {
		where = append(where, "username LIKE ?")
		args = append(args, "%"+username+"%")
	}
	if name := strings.TrimSpace(filter.Name); name != "" {
		where = append(where, "display_name LIKE ?")
		args = append(args, "%"+name+"%")
	}
	if filter.Role != "" {
		where = append(where, "role = ?")
		args = append(args, string(filter.Role))
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(filter.Status))
	}

	countBuilder := strings.Builder{}
	countBuilder.WriteString(`SELECT COUNT(1) FROM sys_user`)
	if len(where) > 0 {
		countBuilder.WriteString(" WHERE ")
		countBuilder.WriteString(strings.Join(where, " AND "))
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countBuilder.String(), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listBuilder := strings.Builder{}
	listBuilder.WriteString(`
SELECT id, username, display_name, email, phone, role, status, password_hash, created_at, updated_at
FROM sys_user`)
	if len(where) > 0 {
		listBuilder.WriteString(" WHERE ")
		listBuilder.WriteString(strings.Join(where, " AND "))
	}
	listBuilder.WriteString(" ORDER BY created_at DESC LIMIT ? OFFSET ?;")
	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := r.db.QueryContext(ctx, listBuilder.String(), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := make([]domain.User, 0)
	for rows.Next() {
		item, scanErr := scanUser(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

// UpdateUser 更新业务资源并返回处理结果。
func (r *UserRepository) UpdateUser(
	ctx context.Context,
	id string,
	input domain.UserUpdateInput,
	updatedAt time.Time,
) (domain.User, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.User{}, domain.ErrUserNotFound
	}

	const q = `
UPDATE sys_user
SET display_name = ?, email = ?, phone = ?, role = ?, status = ?, password_hash = ?, updated_at = ?
WHERE id = ?;`
	res, err := r.db.ExecContext(
		ctx, q,
		strings.TrimSpace(input.DisplayName),
		strings.TrimSpace(input.Email),
		strings.TrimSpace(input.Phone),
		string(input.Role),
		string(input.Status),
		strings.TrimSpace(input.PasswordHash),
		updatedAt.UTC().UnixNano(),
		id,
	)
	if err != nil {
		return domain.User{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.User{}, err
	}
	if affected == 0 {
		return domain.User{}, domain.ErrUserNotFound
	}
	return r.GetUserByID(ctx, id)
}

// DeleteUser 删除业务资源并返回处理结果。
func (r *UserRepository) DeleteUser(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sys_user_manager WHERE user_id = ? OR manager_user_id = ?;`, id, id); err != nil {
		return err
	}
	const q = `DELETE FROM sys_user WHERE id = ?;`
	res, err := tx.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrUserNotFound
	}
	return tx.Commit()
}

// ListUserOptions 查询并返回列表数据。
func (r *UserRepository) ListUserOptions(ctx context.Context) ([]domain.User, error) {
	const q = `
SELECT id, username, display_name, email, phone, role, status, password_hash, created_at, updated_at
FROM sys_user
WHERE status = ?
ORDER BY display_name ASC, username ASC;`
	rows, err := r.db.QueryContext(ctx, q, string(domain.StatusActive))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.User, 0)
	for rows.Next() {
		item, scanErr := scanUser(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetUserManagerID 返回用户的直属主管。
func (r *UserRepository) GetUserManagerID(ctx context.Context, userID string) (string, error) {
	var managerUserID string
	err := r.db.QueryRowContext(ctx, `SELECT manager_user_id FROM sys_user_manager WHERE user_id = ?;`, strings.TrimSpace(userID)).Scan(&managerUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrUserManagerNotFound
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(managerUserID), nil
}

// SetUserManagerID 设置用户直属主管；空主管表示解除关系。
func (r *UserRepository) SetUserManagerID(ctx context.Context, userID string, managerUserID string, updatedAt time.Time) error {
	userID, managerUserID = strings.TrimSpace(userID), strings.TrimSpace(managerUserID)
	if managerUserID == "" {
		_, err := r.db.ExecContext(ctx, `DELETE FROM sys_user_manager WHERE user_id = ?;`, userID)
		return err
	}
	if r.dbDriver == "mysql" {
		_, err := r.db.ExecContext(ctx, `INSERT INTO sys_user_manager (user_id, manager_user_id, updated_at) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE manager_user_id = VALUES(manager_user_id), updated_at = VALUES(updated_at);`, userID, managerUserID, updatedAt.UTC().UnixNano())
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO sys_user_manager (user_id, manager_user_id, updated_at) VALUES (?, ?, ?) ON CONFLICT(user_id) DO UPDATE SET manager_user_id = excluded.manager_user_id, updated_at = excluded.updated_at;`, userID, managerUserID, updatedAt.UTC().UnixNano())
	return err
}

// ResolveUserManager 沿直属主管链向上解析第 level 级主管。
func (r *UserRepository) ResolveUserManager(ctx context.Context, userID string, level int) (domain.User, error) {
	current := strings.TrimSpace(userID)
	if current == "" || level <= 0 {
		return domain.User{}, domain.ErrUserManagerNotFound
	}
	for index := 0; index < level; index++ {
		next, err := r.GetUserManagerID(ctx, current)
		if err != nil {
			return domain.User{}, err
		}
		current = next
	}
	manager, err := r.GetUserByID(ctx, current)
	if err != nil {
		return domain.User{}, err
	}
	if manager.Status != domain.StatusActive || manager.Role == domain.RoleAdmin {
		return domain.User{}, domain.ErrUserManagerNotFound
	}
	return manager, nil
}

// ListUserOrganization 一次查询返回全部用户及直属主管关系，供组织架构画布使用。
func (r *UserRepository) ListUserOrganization(ctx context.Context) ([]domain.OrganizationNode, error) {
	const q = `
SELECT u.id, u.username, u.display_name, u.email, u.phone, u.role, u.status,
       u.password_hash, u.created_at, u.updated_at, COALESCE(manager_user.id, '')
FROM sys_user u
LEFT JOIN sys_user_manager m ON m.user_id = u.id
LEFT JOIN sys_user manager_user ON manager_user.id = m.manager_user_id AND manager_user.role <> ?
WHERE u.role <> ?
ORDER BY u.display_name ASC, u.username ASC;`
	rows, err := r.db.QueryContext(ctx, q, string(domain.RoleAdmin), string(domain.RoleAdmin))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.OrganizationNode, 0)
	for rows.Next() {
		var (
			item              domain.User
			managerUserID     string
			roleRaw           string
			statusRaw         string
			createdAtUnixNano int64
			updatedAtUnixNano int64
		)
		if err := rows.Scan(
			&item.ID,
			&item.Username,
			&item.DisplayName,
			&item.Email,
			&item.Phone,
			&roleRaw,
			&statusRaw,
			&item.PasswordHash,
			&createdAtUnixNano,
			&updatedAtUnixNano,
			&managerUserID,
		); err != nil {
			return nil, err
		}
		item.Role = domain.Role(strings.TrimSpace(roleRaw))
		item.Status = domain.Status(strings.TrimSpace(statusRaw))
		item.CreatedAt = time.Unix(0, createdAtUnixNano).UTC()
		item.UpdatedAt = time.Unix(0, updatedAtUnixNano).UTC()
		result = append(result, domain.OrganizationNode{User: item, ManagerUserID: strings.TrimSpace(managerUserID)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ListPermissions 查询并返回列表数据。
func (r *UserRepository) ListPermissions(ctx context.Context, filter domain.PermissionFilter) ([]domain.Permission, error) {
	args := make([]any, 0, 2)
	where := make([]string, 0, 2)
	if module := strings.TrimSpace(filter.Module); module != "" {
		where = append(where, "module = ?")
		args = append(args, module)
	}
	if action := strings.TrimSpace(filter.Action); action != "" {
		where = append(where, "action = ?")
		args = append(args, action)
	}

	builder := strings.Builder{}
	builder.WriteString(`SELECT id, code, name, module, action, description, created_at, updated_at FROM sys_permission`)
	if len(where) > 0 {
		builder.WriteString(" WHERE ")
		builder.WriteString(strings.Join(where, " AND "))
	}
	builder.WriteString(" ORDER BY module ASC, code ASC;")

	rows, err := r.db.QueryContext(ctx, builder.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.Permission, 0)
	for rows.Next() {
		item, scanErr := scanPermission(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ListUserPermissions 查询并返回列表数据。
func (r *UserRepository) ListUserPermissions(ctx context.Context, userID string) ([]domain.UserPermission, error) {
	const q = `
SELECT id, user_id, permission_code, scope_type, scope_value, enabled, created_at, updated_at
FROM sys_user_permission
WHERE user_id = ?
ORDER BY permission_code ASC, scope_type ASC, scope_value ASC;`

	rows, err := r.db.QueryContext(ctx, q, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.UserPermission, 0)
	for rows.Next() {
		item, scanErr := scanUserPermission(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GrantUserPermissions 封装当前模块的业务处理逻辑。
func (r *UserRepository) GrantUserPermissions(
	ctx context.Context,
	userID string,
	items []domain.UserPermissionGrant,
	now time.Time,
) error {
	const mysqlUpsert = `
INSERT INTO sys_user_permission (
	id, user_id, permission_code, scope_type, scope_value, enabled, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
enabled = VALUES(enabled), updated_at = VALUES(updated_at);`
	const sqliteUpsert = `
INSERT INTO sys_user_permission (
	id, user_id, permission_code, scope_type, scope_value, enabled, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id, permission_code, scope_type, scope_value) DO UPDATE SET
enabled = excluded.enabled, updated_at = excluded.updated_at;`

	stmt := mysqlUpsert
	if r.dbDriver == "sqlite" {
		stmt = sqliteUpsert
	}

	for _, item := range items {
		scopeType := strings.TrimSpace(item.ScopeType)
		if scopeType == "" {
			scopeType = "global"
		}
		scopeValue := strings.TrimSpace(item.ScopeValue)
		if _, err := r.db.ExecContext(
			ctx,
			stmt,
			"upr-"+newSimpleID(),
			strings.TrimSpace(userID),
			strings.TrimSpace(item.PermissionCode),
			scopeType,
			scopeValue,
			1,
			now.UTC().UnixNano(),
			now.UTC().UnixNano(),
		); err != nil {
			return err
		}
	}
	return nil
}

// RevokeUserPermissions 封装当前模块的业务处理逻辑。
func (r *UserRepository) RevokeUserPermissions(
	ctx context.Context,
	userID string,
	items []domain.UserPermissionGrant,
) error {
	const q = `
DELETE FROM sys_user_permission
WHERE user_id = ? AND permission_code = ? AND scope_type = ? AND scope_value = ?;`

	for _, item := range items {
		scopeType := strings.TrimSpace(item.ScopeType)
		if scopeType == "" {
			scopeType = "global"
		}
		scopeValue := strings.TrimSpace(item.ScopeValue)
		if _, err := r.db.ExecContext(ctx, q, strings.TrimSpace(userID), strings.TrimSpace(item.PermissionCode), scopeType, scopeValue); err != nil {
			return err
		}
	}
	return nil
}

// ListUserParamPermissions 查询并返回列表数据。
func (r *UserRepository) ListUserParamPermissions(
	ctx context.Context,
	userID string,
	applicationID string,
) ([]domain.UserParamPermission, error) {
	args := []any{strings.TrimSpace(userID)}
	builder := strings.Builder{}
	builder.WriteString(`
SELECT id, user_id, param_key, application_id, can_view, can_edit, created_at, updated_at
FROM sys_user_param_permission
WHERE user_id = ?`)
	if appID := strings.TrimSpace(applicationID); appID != "" {
		builder.WriteString(" AND (application_id = '' OR application_id = ?)")
		args = append(args, appID)
	}
	builder.WriteString(" ORDER BY param_key ASC, application_id ASC;")

	rows, err := r.db.QueryContext(ctx, builder.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.UserParamPermission, 0)
	for rows.Next() {
		item, scanErr := scanUserParamPermission(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// UpsertUserParamPermission 封装当前模块的业务处理逻辑。
func (r *UserRepository) UpsertUserParamPermission(
	ctx context.Context,
	item domain.UserParamPermission,
) (domain.UserParamPermission, error) {
	const mysqlUpsert = `
INSERT INTO sys_user_param_permission (
	id, user_id, param_key, application_id, can_view, can_edit, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
can_view = VALUES(can_view), can_edit = VALUES(can_edit), updated_at = VALUES(updated_at);`
	const sqliteUpsert = `
INSERT INTO sys_user_param_permission (
	id, user_id, param_key, application_id, can_view, can_edit, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id, param_key, application_id) DO UPDATE SET
can_view = excluded.can_view, can_edit = excluded.can_edit, updated_at = excluded.updated_at;`

	stmt := mysqlUpsert
	if r.dbDriver == "sqlite" {
		stmt = sqliteUpsert
	}
	nowNano := item.UpdatedAt.UTC().UnixNano()
	if _, err := r.db.ExecContext(
		ctx,
		stmt,
		item.ID,
		item.UserID,
		strings.ToLower(strings.TrimSpace(item.ParamKey)),
		strings.TrimSpace(item.ApplicationID),
		boolToTinyInt(item.CanView),
		boolToTinyInt(item.CanEdit),
		item.CreatedAt.UTC().UnixNano(),
		nowNano,
	); err != nil {
		return domain.UserParamPermission{}, err
	}

	return r.getUserParamPermissionByUnique(ctx, item.UserID, item.ParamKey, item.ApplicationID)
}

// getUserParamPermissionByUnique 查询并返回指定资源数据。
func (r *UserRepository) getUserParamPermissionByUnique(
	ctx context.Context,
	userID string,
	paramKey string,
	applicationID string,
) (domain.UserParamPermission, error) {
	const q = `
SELECT id, user_id, param_key, application_id, can_view, can_edit, created_at, updated_at
FROM sys_user_param_permission
WHERE user_id = ? AND param_key = ? AND application_id = ?;`
	row := r.db.QueryRowContext(
		ctx,
		q,
		strings.TrimSpace(userID),
		strings.ToLower(strings.TrimSpace(paramKey)),
		strings.TrimSpace(applicationID),
	)
	item, err := scanUserParamPermission(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.UserParamPermission{}, domain.ErrParamPermissionNotFound
		}
		return domain.UserParamPermission{}, err
	}
	return item, nil
}

// DeleteUserParamPermission 删除业务资源并返回处理结果。
func (r *UserRepository) DeleteUserParamPermission(ctx context.Context, id string) error {
	const q = `DELETE FROM sys_user_param_permission WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrParamPermissionNotFound
	}
	return nil
}

// CreateSession 创建业务资源并返回处理结果。
func (r *UserRepository) CreateSession(ctx context.Context, item domain.UserSession) error {
	const q = `
INSERT INTO sys_user_session (
	id, user_id, access_token, expired_at, client_ip, user_agent, revoked_at, revoked_reason, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);`
	var revokedAt any
	if item.RevokedAt != nil {
		revokedAt = item.RevokedAt.UTC().UnixNano()
	}
	_, err := r.db.ExecContext(
		ctx, q,
		item.ID,
		item.UserID,
		item.AccessToken,
		item.ExpiredAt.UTC().UnixNano(),
		item.ClientIP,
		item.UserAgent,
		revokedAt,
		strings.TrimSpace(item.RevokedReason),
		item.CreatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isDuplicateUserError(r.dbDriver, err) {
			return fmt.Errorf("duplicated session token")
		}
		return err
	}
	return nil
}

// GetSessionByAccessToken 查询并返回指定资源数据。
func (r *UserRepository) GetSessionByAccessToken(ctx context.Context, token string) (domain.UserSession, error) {
	const q = `
SELECT id, user_id, access_token, expired_at, client_ip, user_agent, revoked_at, revoked_reason, created_at
FROM sys_user_session
WHERE access_token = ?;`
	row := r.db.QueryRowContext(ctx, q, strings.TrimSpace(token))
	item, err := scanUserSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.UserSession{}, domain.ErrSessionNotFound
		}
		return domain.UserSession{}, err
	}
	return item, nil
}

// DeleteSessionByAccessToken 删除业务资源并返回处理结果。
func (r *UserRepository) DeleteSessionByAccessToken(ctx context.Context, token string) error {
	const q = `DELETE FROM sys_user_session WHERE access_token = ?;`
	_, err := r.db.ExecContext(ctx, q, strings.TrimSpace(token))
	return err
}

// DeleteSessionsByUserID 删除指定用户的所有登录会话。
func (r *UserRepository) RevokeSessionsByUserID(ctx context.Context, userID string, reason string, revokedAt time.Time) (int64, error) {
	const q = `
UPDATE sys_user_session
SET revoked_at = ?, revoked_reason = ?
WHERE user_id = ? AND revoked_at IS NULL;`
	res, err := r.db.ExecContext(
		ctx,
		q,
		revokedAt.UTC().UnixNano(),
		strings.TrimSpace(reason),
		strings.TrimSpace(userID),
	)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

// DeleteExpiredSessions 删除业务资源并返回处理结果。
func (r *UserRepository) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	const q = `DELETE FROM sys_user_session WHERE expired_at <= ?;`
	res, err := r.db.ExecContext(ctx, q, now.UTC().UnixNano())
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

type userScanner interface {
	Scan(dest ...any) error
}

// scanUser 封装当前模块的业务处理逻辑。
func scanUser(s userScanner) (domain.User, error) {
	var (
		item              domain.User
		roleRaw           string
		statusRaw         string
		createdAtUnixNano int64
		updatedAtUnixNano int64
	)
	if err := s.Scan(
		&item.ID,
		&item.Username,
		&item.DisplayName,
		&item.Email,
		&item.Phone,
		&roleRaw,
		&statusRaw,
		&item.PasswordHash,
		&createdAtUnixNano,
		&updatedAtUnixNano,
	); err != nil {
		return domain.User{}, err
	}
	item.Role = domain.Role(strings.TrimSpace(roleRaw))
	item.Status = domain.Status(strings.TrimSpace(statusRaw))
	item.CreatedAt = time.Unix(0, createdAtUnixNano).UTC()
	item.UpdatedAt = time.Unix(0, updatedAtUnixNano).UTC()
	return item, nil
}

// scanPermission 封装当前模块的业务处理逻辑。
func scanPermission(s userScanner) (domain.Permission, error) {
	var (
		item              domain.Permission
		createdAtUnixNano int64
		updatedAtUnixNano int64
	)
	if err := s.Scan(
		&item.ID,
		&item.Code,
		&item.Name,
		&item.Module,
		&item.Action,
		&item.Description,
		&createdAtUnixNano,
		&updatedAtUnixNano,
	); err != nil {
		return domain.Permission{}, err
	}
	item.CreatedAt = time.Unix(0, createdAtUnixNano).UTC()
	item.UpdatedAt = time.Unix(0, updatedAtUnixNano).UTC()
	return item, nil
}

// scanUserPermission 封装当前模块的业务处理逻辑。
func scanUserPermission(s userScanner) (domain.UserPermission, error) {
	var (
		item              domain.UserPermission
		enabledInt        int
		createdAtUnixNano int64
		updatedAtUnixNano int64
	)
	if err := s.Scan(
		&item.ID,
		&item.UserID,
		&item.PermissionCode,
		&item.ScopeType,
		&item.ScopeValue,
		&enabledInt,
		&createdAtUnixNano,
		&updatedAtUnixNano,
	); err != nil {
		return domain.UserPermission{}, err
	}
	item.Enabled = enabledInt == 1
	item.CreatedAt = time.Unix(0, createdAtUnixNano).UTC()
	item.UpdatedAt = time.Unix(0, updatedAtUnixNano).UTC()
	return item, nil
}

// scanUserParamPermission 封装当前模块的业务处理逻辑。
func scanUserParamPermission(s userScanner) (domain.UserParamPermission, error) {
	var (
		item              domain.UserParamPermission
		canView           int
		canEdit           int
		createdAtUnixNano int64
		updatedAtUnixNano int64
	)
	if err := s.Scan(
		&item.ID,
		&item.UserID,
		&item.ParamKey,
		&item.ApplicationID,
		&canView,
		&canEdit,
		&createdAtUnixNano,
		&updatedAtUnixNano,
	); err != nil {
		return domain.UserParamPermission{}, err
	}
	item.ParamKey = strings.ToLower(strings.TrimSpace(item.ParamKey))
	item.CanView = canView == 1
	item.CanEdit = canEdit == 1
	item.CreatedAt = time.Unix(0, createdAtUnixNano).UTC()
	item.UpdatedAt = time.Unix(0, updatedAtUnixNano).UTC()
	return item, nil
}

// scanUserSession 封装当前模块的业务处理逻辑。
func scanUserSession(s userScanner) (domain.UserSession, error) {
	var (
		item            domain.UserSession
		expiredUnixNano int64
		revokedUnixNano sql.NullInt64
		createdUnixNano int64
	)
	if err := s.Scan(
		&item.ID,
		&item.UserID,
		&item.AccessToken,
		&expiredUnixNano,
		&item.ClientIP,
		&item.UserAgent,
		&revokedUnixNano,
		&item.RevokedReason,
		&createdUnixNano,
	); err != nil {
		return domain.UserSession{}, err
	}
	item.ExpiredAt = time.Unix(0, expiredUnixNano).UTC()
	if revokedUnixNano.Valid {
		revokedAt := time.Unix(0, revokedUnixNano.Int64).UTC()
		item.RevokedAt = &revokedAt
	}
	item.RevokedReason = strings.TrimSpace(item.RevokedReason)
	item.CreatedAt = time.Unix(0, createdUnixNano).UTC()
	return item, nil
}

// isDuplicateUserError 封装当前模块的业务处理逻辑。
func isDuplicateUserError(dbDriver string, err error) bool {
	switch dbDriver {
	case "mysql":
		var mysqlErr *mysqlDriver.MySQLError
		return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
	case "sqlite":
		return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
	default:
		return false
	}
}

// boolToTinyInt 封装当前模块的业务处理逻辑。
func boolToTinyInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// newSimpleID 封装当前模块的业务处理逻辑。
func newSimpleID() string {
	var buffer [8]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return strings.ToLower(hex.EncodeToString(buffer[:]))
}
