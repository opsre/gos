package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/gitcredential"
)

// GitCredentialRepository persists Git credentials. The secret column is
// encrypted with the platform key on the way in and decrypted on the way out,
// following the artifact repository configuration precedent.
type GitCredentialRepository struct {
	db       *sql.DB
	dbDriver string
}

// NewGitCredentialRepository 创建并返回对应组件实例。
func NewGitCredentialRepository(db *sql.DB, dbDriver string) *GitCredentialRepository {
	return &GitCredentialRepository{
		db:       db,
		dbDriver: strings.ToLower(strings.TrimSpace(dbDriver)),
	}
}

// InitSchema 初始化依赖资源并返回可能的错误。
func (r *GitCredentialRepository) InitSchema(ctx context.Context) error {
	if err := r.createTable(ctx); err != nil {
		return err
	}
	if r.dbDriver == "sqlite" {
		if _, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_git_credential_status_updated_at ON git_credential (status, updated_at);`); err != nil {
			return err
		}
	}
	// The table itself is already created above with CREATE TABLE IF NOT EXISTS.
	// The versioned migration is registered so the applied version is recorded in
	// gos_schema_migration, which keeps the history complete for later upgrades
	// that do need to alter this table.
	return runSchemaMigrations(ctx, r.db, r.dbDriver, schemaMigration{
		Version:     "20260918_01_git_credential",
		Description: "create git_credential table for git credential management",
		Up:          r.createTable,
	})
}

// createTable issues the dialect-specific DDL. It is safe to run on every boot
// and on a database where the table already exists.
func (r *GitCredentialRepository) createTable(ctx context.Context) error {
	var schema string
	switch r.dbDriver {
	case "mysql":
		schema = `
CREATE TABLE IF NOT EXISTS git_credential (
	id VARCHAR(64) PRIMARY KEY,
	name VARCHAR(100) NOT NULL,
	provider VARCHAR(50) NOT NULL,
	base_url VARCHAR(500) NOT NULL,
	username VARCHAR(255) NOT NULL DEFAULT '',
	secret_ciphertext TEXT NOT NULL,
	auth_type VARCHAR(50) NOT NULL,
	status VARCHAR(50) NOT NULL,
	remark VARCHAR(500) NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uq_git_credential_name (name),
	KEY idx_git_credential_status_updated_at (status, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`
	case "sqlite":
		schema = `
CREATE TABLE IF NOT EXISTS git_credential (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	provider TEXT NOT NULL,
	base_url TEXT NOT NULL,
	username TEXT NOT NULL DEFAULT '',
	secret_ciphertext TEXT NOT NULL,
	auth_type TEXT NOT NULL,
	status TEXT NOT NULL,
	remark TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
	_, err := r.db.ExecContext(ctx, schema)
	return err
}

// Create 创建业务资源并返回处理结果。
func (r *GitCredentialRepository) Create(ctx context.Context, item domain.Credential) error {
	encryptedSecret, err := encryptStoredSecret(strings.TrimSpace(item.Secret))
	if err != nil {
		return err
	}
	const q = `
INSERT INTO git_credential (
	id, name, provider, base_url, username, secret_ciphertext, auth_type, status, remark, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err = r.db.ExecContext(
		ctx,
		q,
		item.ID,
		item.Name,
		string(item.Provider),
		item.BaseURL,
		item.Username,
		encryptedSecret,
		string(item.AuthType),
		string(item.Status),
		item.Remark,
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isDuplicateKeyError(r.dbDriver, err) {
			return domain.ErrNameDuplicated
		}
		return err
	}
	return nil
}

// GetByID 查询并返回指定资源数据。
func (r *GitCredentialRepository) GetByID(ctx context.Context, id string) (domain.Credential, error) {
	const q = `
SELECT id, name, provider, base_url, username, secret_ciphertext, auth_type, status, remark, created_at, updated_at
FROM git_credential
WHERE id = ?;`
	item, err := scanGitCredential(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Credential{}, domain.ErrNotFound
		}
		return domain.Credential{}, err
	}
	return item, nil
}

// List 查询并返回列表数据。
func (r *GitCredentialRepository) List(ctx context.Context, filter domain.ListFilter) ([]domain.Credential, int64, error) {
	where, args := buildGitCredentialWhere(filter)
	countQ := `SELECT COUNT(1) FROM git_credential` + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
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
	offset := (page - 1) * pageSize
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, offset)
	q := `
SELECT id, name, provider, base_url, username, secret_ciphertext, auth_type, status, remark, created_at, updated_at
FROM git_credential` + where + `
ORDER BY updated_at DESC, created_at DESC
LIMIT ? OFFSET ?;`
	rows, err := r.db.QueryContext(ctx, q, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]domain.Credential, 0)
	for rows.Next() {
		item, scanErr := scanGitCredential(rows)
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

// Update 更新业务资源并返回处理结果。
func (r *GitCredentialRepository) Update(ctx context.Context, id string, input domain.UpdateInput, updatedAt time.Time) (domain.Credential, error) {
	encryptedSecret, err := encryptStoredSecret(strings.TrimSpace(input.Secret))
	if err != nil {
		return domain.Credential{}, err
	}
	const q = `
UPDATE git_credential
SET name = ?, provider = ?, base_url = ?, username = ?, secret_ciphertext = ?, auth_type = ?, status = ?, remark = ?, updated_at = ?
WHERE id = ?;`
	if _, err := r.db.ExecContext(
		ctx,
		q,
		input.Name,
		string(input.Provider),
		input.BaseURL,
		input.Username,
		encryptedSecret,
		string(input.AuthType),
		string(input.Status),
		input.Remark,
		updatedAt.UTC().UnixNano(),
		id,
	); err != nil {
		if isDuplicateKeyError(r.dbDriver, err) {
			return domain.Credential{}, domain.ErrNameDuplicated
		}
		return domain.Credential{}, err
	}
	// The read-back also reports a missing row: MySQL returns 0 affected rows
	// both for an unknown id and for an update that changed nothing, so the
	// existence check cannot be based on RowsAffected alone.
	return r.GetByID(ctx, id)
}

// Delete 删除业务资源并返回处理结果。
func (r *GitCredentialRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM git_credential WHERE id = ?;`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func buildGitCredentialWhere(filter domain.ListFilter) (string, []any) {
	conditions := make([]string, 0, 2)
	args := make([]any, 0, 5)
	if strings.TrimSpace(filter.Keyword) != "" {
		conditions = append(conditions, "(name LIKE ? OR base_url LIKE ? OR username LIKE ? OR remark LIKE ?)")
		keyword := "%" + strings.TrimSpace(filter.Keyword) + "%"
		args = append(args, keyword, keyword, keyword, keyword)
	}
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, string(filter.Status))
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

type gitCredentialScanner interface {
	Scan(dest ...any) error
}

func scanGitCredential(scanner gitCredentialScanner) (domain.Credential, error) {
	var (
		item            domain.Credential
		provider        string
		encryptedSecret string
		authType        string
		status          string
		createdAt       int64
		updatedAt       int64
	)
	if err := scanner.Scan(
		&item.ID,
		&item.Name,
		&provider,
		&item.BaseURL,
		&item.Username,
		&encryptedSecret,
		&authType,
		&status,
		&item.Remark,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.Credential{}, err
	}
	secret, err := decryptStoredSecret(encryptedSecret)
	if err != nil {
		return domain.Credential{}, err
	}
	item.Provider = domain.Provider(provider)
	item.Secret = secret
	item.AuthType = domain.AuthType(authType)
	item.Status = domain.Status(status)
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}
