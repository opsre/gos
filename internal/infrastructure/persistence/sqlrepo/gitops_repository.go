package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	domain "gos/internal/domain/gitops"
)

type GitOpsRepository struct {
	db       *sql.DB
	dbDriver string
}

// NewGitOpsRepository 创建并返回对应组件实例。
func NewGitOpsRepository(db *sql.DB, dbDriver string) *GitOpsRepository {
	return &GitOpsRepository{db: db, dbDriver: strings.ToLower(strings.TrimSpace(dbDriver))}
}

// InitSchema 封装当前模块的业务处理逻辑。
func (r *GitOpsRepository) InitSchema(ctx context.Context) error {
	var statements []string
	switch r.dbDriver {
	case "mysql":
		statements = []string{
			`CREATE TABLE IF NOT EXISTS gitops_instance (
	id VARCHAR(64) PRIMARY KEY,
	instance_code VARCHAR(100) NOT NULL,
	name VARCHAR(120) NOT NULL,
	local_root VARCHAR(500) NOT NULL,
	default_branch VARCHAR(120) NOT NULL DEFAULT 'master',
	username VARCHAR(120) NOT NULL DEFAULT '',
	password_ciphertext TEXT NOT NULL,
	token_ciphertext TEXT NOT NULL,
	author_name VARCHAR(120) NOT NULL DEFAULT '',
	author_email VARCHAR(200) NOT NULL DEFAULT '',
	commit_message_template TEXT NOT NULL,
	command_timeout_sec INT NOT NULL DEFAULT 30,
	status VARCHAR(20) NOT NULL DEFAULT 'active',
	remark VARCHAR(500) NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uk_gitops_instance_code (instance_code),
	UNIQUE KEY uk_gitops_instance_local_root (local_root),
	KEY idx_gitops_instance_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		}
	case "sqlite":
		statements = []string{
			`CREATE TABLE IF NOT EXISTS gitops_instance (
	id TEXT PRIMARY KEY,
	instance_code TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	local_root TEXT NOT NULL UNIQUE,
	default_branch TEXT NOT NULL DEFAULT 'master',
	username TEXT NOT NULL DEFAULT '',
	password_ciphertext TEXT NOT NULL DEFAULT '',
	token_ciphertext TEXT NOT NULL DEFAULT '',
	author_name TEXT NOT NULL DEFAULT '',
	author_email TEXT NOT NULL DEFAULT '',
	commit_message_template TEXT NOT NULL DEFAULT '',
	command_timeout_sec INTEGER NOT NULL DEFAULT 30,
	status TEXT NOT NULL DEFAULT 'active',
	remark TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`,
			`CREATE INDEX IF NOT EXISTS idx_gitops_instance_status ON gitops_instance (status);`,
		}
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}

	for _, stmt := range statements {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// UpsertInstance 封装当前模块的业务处理逻辑。
func (r *GitOpsRepository) UpsertInstance(ctx context.Context, item domain.Instance) (domain.Instance, error) {
	const mysqlQ = `
INSERT INTO gitops_instance (
	id, instance_code, name, local_root, default_branch, username, password_ciphertext, token_ciphertext,
	author_name, author_email, commit_message_template, command_timeout_sec, status, remark, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	name = VALUES(name),
	local_root = VALUES(local_root),
	default_branch = VALUES(default_branch),
	username = VALUES(username),
	password_ciphertext = VALUES(password_ciphertext),
	token_ciphertext = VALUES(token_ciphertext),
	author_name = VALUES(author_name),
	author_email = VALUES(author_email),
	commit_message_template = VALUES(commit_message_template),
	command_timeout_sec = VALUES(command_timeout_sec),
	status = VALUES(status),
	remark = VALUES(remark),
	updated_at = VALUES(updated_at);`
	const sqliteQ = `
INSERT INTO gitops_instance (
	id, instance_code, name, local_root, default_branch, username, password_ciphertext, token_ciphertext,
	author_name, author_email, commit_message_template, command_timeout_sec, status, remark, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(instance_code) DO UPDATE SET
	name = excluded.name,
	local_root = excluded.local_root,
	default_branch = excluded.default_branch,
	username = excluded.username,
	password_ciphertext = excluded.password_ciphertext,
	token_ciphertext = excluded.token_ciphertext,
	author_name = excluded.author_name,
	author_email = excluded.author_email,
	commit_message_template = excluded.commit_message_template,
	command_timeout_sec = excluded.command_timeout_sec,
	status = excluded.status,
	remark = excluded.remark,
	updated_at = excluded.updated_at;`
	q := mysqlQ
	if r.dbDriver == "sqlite" {
		q = sqliteQ
	}
	createdAt := item.CreatedAt.UTC().UnixNano()
	if createdAt == 0 {
		createdAt = item.UpdatedAt.UTC().UnixNano()
	}
	if _, err := r.db.ExecContext(ctx, q,
		item.ID,
		strings.TrimSpace(item.InstanceCode),
		strings.TrimSpace(item.Name),
		strings.TrimSpace(item.LocalRoot),
		strings.TrimSpace(item.DefaultBranch),
		strings.TrimSpace(item.Username),
		strings.TrimSpace(item.Password),
		strings.TrimSpace(item.Token),
		strings.TrimSpace(item.AuthorName),
		strings.TrimSpace(item.AuthorEmail),
		strings.TrimSpace(item.CommitMessageTemplate),
		item.CommandTimeoutSec,
		string(item.Status),
		strings.TrimSpace(item.Remark),
		createdAt,
		item.UpdatedAt.UTC().UnixNano(),
	); err != nil {
		return domain.Instance{}, err
	}
	return r.GetInstanceByCode(ctx, item.InstanceCode)
}

// CreateInstance 创建业务资源并返回处理结果。
func (r *GitOpsRepository) CreateInstance(ctx context.Context, item domain.Instance) (domain.Instance, error) {
	encryptedPassword, err := encryptStoredSecret(strings.TrimSpace(item.Password))
	if err != nil {
		return domain.Instance{}, err
	}
	encryptedToken, err := encryptStoredSecret(strings.TrimSpace(item.Token))
	if err != nil {
		return domain.Instance{}, err
	}
	const q = `
INSERT INTO gitops_instance (
	id, instance_code, name, local_root, default_branch, username, password_ciphertext, token_ciphertext,
	author_name, author_email, commit_message_template, command_timeout_sec, status, remark, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	if _, err := r.db.ExecContext(ctx, q,
		item.ID,
		strings.TrimSpace(item.InstanceCode),
		strings.TrimSpace(item.Name),
		strings.TrimSpace(item.LocalRoot),
		strings.TrimSpace(item.DefaultBranch),
		strings.TrimSpace(item.Username),
		encryptedPassword,
		encryptedToken,
		strings.TrimSpace(item.AuthorName),
		strings.TrimSpace(item.AuthorEmail),
		strings.TrimSpace(item.CommitMessageTemplate),
		item.CommandTimeoutSec,
		string(item.Status),
		strings.TrimSpace(item.Remark),
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	); err != nil {
		return domain.Instance{}, err
	}
	return r.GetInstanceByID(ctx, item.ID)
}

// UpdateInstance 更新业务资源并返回处理结果。
func (r *GitOpsRepository) UpdateInstance(ctx context.Context, item domain.Instance) (domain.Instance, error) {
	encryptedPassword, err := encryptStoredSecret(strings.TrimSpace(item.Password))
	if err != nil {
		return domain.Instance{}, err
	}
	encryptedToken, err := encryptStoredSecret(strings.TrimSpace(item.Token))
	if err != nil {
		return domain.Instance{}, err
	}
	const q = `
UPDATE gitops_instance
SET instance_code = ?, name = ?, local_root = ?, default_branch = ?, username = ?, password_ciphertext = ?, token_ciphertext = ?,
	author_name = ?, author_email = ?, commit_message_template = ?, command_timeout_sec = ?, status = ?, remark = ?, updated_at = ?
WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q,
		strings.TrimSpace(item.InstanceCode),
		strings.TrimSpace(item.Name),
		strings.TrimSpace(item.LocalRoot),
		strings.TrimSpace(item.DefaultBranch),
		strings.TrimSpace(item.Username),
		encryptedPassword,
		encryptedToken,
		strings.TrimSpace(item.AuthorName),
		strings.TrimSpace(item.AuthorEmail),
		strings.TrimSpace(item.CommitMessageTemplate),
		item.CommandTimeoutSec,
		string(item.Status),
		strings.TrimSpace(item.Remark),
		item.UpdatedAt.UTC().UnixNano(),
		strings.TrimSpace(item.ID),
	)
	if err != nil {
		return domain.Instance{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Instance{}, err
	}
	if affected == 0 {
		return domain.Instance{}, domain.ErrInstanceNotFound
	}
	return r.GetInstanceByID(ctx, item.ID)
}

// GetInstanceByID 查询并返回指定资源数据。
func (r *GitOpsRepository) GetInstanceByID(ctx context.Context, id string) (domain.Instance, error) {
	const q = `
SELECT id, instance_code, name, local_root, default_branch, username, password_ciphertext, token_ciphertext,
	author_name, author_email, commit_message_template, command_timeout_sec, status, remark, created_at, updated_at
FROM gitops_instance WHERE id = ?;`
	item, err := scanGitOpsInstance(r.db.QueryRowContext(ctx, q, strings.TrimSpace(id)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Instance{}, domain.ErrInstanceNotFound
		}
		return domain.Instance{}, err
	}
	return item, nil
}

// GetInstanceByCode 查询并返回指定资源数据。
func (r *GitOpsRepository) GetInstanceByCode(ctx context.Context, code string) (domain.Instance, error) {
	const q = `
SELECT id, instance_code, name, local_root, default_branch, username, password_ciphertext, token_ciphertext,
	author_name, author_email, commit_message_template, command_timeout_sec, status, remark, created_at, updated_at
FROM gitops_instance WHERE instance_code = ?;`
	item, err := scanGitOpsInstance(r.db.QueryRowContext(ctx, q, strings.TrimSpace(code)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Instance{}, domain.ErrInstanceNotFound
		}
		return domain.Instance{}, err
	}
	return item, nil
}

// ListInstances 查询并返回列表数据。
func (r *GitOpsRepository) ListInstances(ctx context.Context, filter domain.InstanceListFilter) ([]domain.Instance, int64, error) {
	args := make([]any, 0, 8)
	where := make([]string, 0, 2)
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		where = append(where, "(instance_code LIKE ? OR name LIKE ? OR local_root LIKE ?)")
		like := "%" + keyword + "%"
		args = append(args, like, like, like)
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(filter.Status))
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	countSQL := `SELECT COUNT(1) FROM gitops_instance`
	if len(where) > 0 {
		countSQL += " WHERE " + strings.Join(where, " AND ")
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	querySQL := `SELECT id, instance_code, name, local_root, default_branch, username, password_ciphertext, token_ciphertext,
	author_name, author_email, commit_message_template, command_timeout_sec, status, remark, created_at, updated_at
FROM gitops_instance`
	if len(where) > 0 {
		querySQL += " WHERE " + strings.Join(where, " AND ")
	}
	querySQL += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	rows, err := r.db.QueryContext(ctx, querySQL, append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]domain.Instance, 0)
	for rows.Next() {
		item, scanErr := scanGitOpsInstance(rows)
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

// ListActiveInstances 查询并返回列表数据。
func (r *GitOpsRepository) ListActiveInstances(ctx context.Context) ([]domain.Instance, error) {
	const q = `
SELECT id, instance_code, name, local_root, default_branch, username, password_ciphertext, token_ciphertext,
	author_name, author_email, commit_message_template, command_timeout_sec, status, remark, created_at, updated_at
FROM gitops_instance WHERE status = ? ORDER BY instance_code ASC;`
	rows, err := r.db.QueryContext(ctx, q, string(domain.StatusActive))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Instance, 0)
	for rows.Next() {
		item, scanErr := scanGitOpsInstance(rows)
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

type gitOpsInstanceScanner interface{ Scan(dest ...any) error }

// scanGitOpsInstance 封装当前模块的业务处理逻辑。
func scanGitOpsInstance(scanner gitOpsInstanceScanner) (domain.Instance, error) {
	var (
		item              domain.Instance
		encryptedPassword string
		encryptedToken    string
		status            string
		createdAt         int64
		updatedAt         int64
	)
	if err := scanner.Scan(
		&item.ID,
		&item.InstanceCode,
		&item.Name,
		&item.LocalRoot,
		&item.DefaultBranch,
		&item.Username,
		&encryptedPassword,
		&encryptedToken,
		&item.AuthorName,
		&item.AuthorEmail,
		&item.CommitMessageTemplate,
		&item.CommandTimeoutSec,
		&status,
		&item.Remark,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.Instance{}, err
	}
	password, err := decryptStoredSecret(encryptedPassword)
	if err != nil {
		return domain.Instance{}, err
	}
	token, err := decryptStoredSecret(encryptedToken)
	if err != nil {
		return domain.Instance{}, err
	}
	item.Password = password
	item.Token = token
	item.Status = domain.Status(status)
	item.CreatedAt = unixNanoToTime(createdAt)
	item.UpdatedAt = unixNanoToTime(updatedAt)
	return item, nil
}
