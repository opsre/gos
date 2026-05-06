package sqlrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/application"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

type ApplicationRepository struct {
	db       *sql.DB
	dbDriver string
}

// NewApplicationRepository 创建并返回对应组件实例。
func NewApplicationRepository(db *sql.DB, dbDriver string) *ApplicationRepository {
	return &ApplicationRepository{db: db, dbDriver: strings.ToLower(strings.TrimSpace(dbDriver))}
}

// InitSchema 封装当前模块的业务处理逻辑。
func (r *ApplicationRepository) InitSchema(ctx context.Context) error {
	var schema string
	switch r.dbDriver {
	case "mysql":
		schema = `
CREATE TABLE IF NOT EXISTS applications (
	id VARCHAR(64) PRIMARY KEY,
	name VARCHAR(128) NOT NULL,
	app_key VARCHAR(128) NOT NULL,
	project_id VARCHAR(64) NOT NULL DEFAULT '',
	repo_url TEXT NOT NULL,
	description TEXT NOT NULL,
	owner_user_id VARCHAR(64) NOT NULL DEFAULT '',
	owner VARCHAR(128) NOT NULL,
	status VARCHAR(32) NOT NULL,
	artifact_type VARCHAR(64) NOT NULL,
	language VARCHAR(64) NOT NULL,
	gitops_branch_mappings JSON NULL,
	release_branches JSON NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uq_application_key (app_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`
	case "sqlite":
		schema = `
CREATE TABLE IF NOT EXISTS applications (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	app_key TEXT NOT NULL UNIQUE,
	project_id TEXT NOT NULL DEFAULT '',
	repo_url TEXT NOT NULL,
	description TEXT NOT NULL,
	owner_user_id TEXT NOT NULL DEFAULT '',
	owner TEXT NOT NULL,
	status TEXT NOT NULL,
	artifact_type TEXT NOT NULL,
	language TEXT NOT NULL,
	gitops_branch_mappings TEXT NOT NULL DEFAULT '[]',
	release_branches TEXT NOT NULL DEFAULT '[]',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}

	if _, err := r.db.ExecContext(ctx, schema); err != nil {
		return err
	}
	return r.migrateSchema(ctx)
}

// migrateSchema 封装当前模块的业务处理逻辑。
func (r *ApplicationRepository) migrateSchema(ctx context.Context) error {
	switch r.dbDriver {
	case "mysql":
		exists, err := r.mysqlColumnExists(ctx, "applications", "owner_user_id")
		if err != nil {
			return err
		}
		if !exists {
			if _, err = r.db.ExecContext(
				ctx,
				`ALTER TABLE applications ADD COLUMN owner_user_id VARCHAR(64) NOT NULL DEFAULT '' AFTER description;`,
			); err != nil {
				return err
			}
		}
		exists, err = r.mysqlColumnExists(ctx, "applications", "project_id")
		if err != nil {
			return err
		}
		if !exists {
			if _, err = r.db.ExecContext(
				ctx,
				`ALTER TABLE applications ADD COLUMN project_id VARCHAR(64) NOT NULL DEFAULT '' AFTER app_key;`,
			); err != nil {
				return err
			}
		}
		exists, err = r.mysqlColumnExists(ctx, "applications", "gitops_branch_mappings")
		if err != nil {
			return err
		}
		if !exists {
			if _, err = r.db.ExecContext(
				ctx,
				`ALTER TABLE applications ADD COLUMN gitops_branch_mappings JSON NULL AFTER language;`,
			); err != nil {
				return err
			}
		}
		exists, err = r.mysqlColumnExists(ctx, "applications", "release_branches")
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		_, err = r.db.ExecContext(
			ctx,
			`ALTER TABLE applications ADD COLUMN release_branches JSON NULL AFTER gitops_branch_mappings;`,
		)
		return err
	case "sqlite":
		columns, err := r.sqliteTableColumns(ctx, "applications")
		if err != nil {
			return err
		}
		if _, ok := columns["owner_user_id"]; !ok {
			if _, err = r.db.ExecContext(
				ctx,
				`ALTER TABLE applications ADD COLUMN owner_user_id TEXT NOT NULL DEFAULT '';`,
			); err != nil {
				return err
			}
		}
		if _, ok := columns["project_id"]; !ok {
			if _, err = r.db.ExecContext(
				ctx,
				`ALTER TABLE applications ADD COLUMN project_id TEXT NOT NULL DEFAULT '';`,
			); err != nil {
				return err
			}
		}
		if _, ok := columns["gitops_branch_mappings"]; !ok {
			if _, err = r.db.ExecContext(
				ctx,
				`ALTER TABLE applications ADD COLUMN gitops_branch_mappings TEXT NOT NULL DEFAULT '[]';`,
			); err != nil {
				return err
			}
		}
		if _, ok := columns["release_branches"]; ok {
			return nil
		}
		_, err = r.db.ExecContext(
			ctx,
			`ALTER TABLE applications ADD COLUMN release_branches TEXT NOT NULL DEFAULT '[]';`,
		)
		return err
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
}

// Create 创建业务资源并返回处理结果。
func (r *ApplicationRepository) Create(ctx context.Context, app domain.Application) error {
	const q = `
INSERT INTO applications (
	id, name, app_key, project_id, repo_url, description, owner_user_id, owner, status, artifact_type, language, gitops_branch_mappings, release_branches, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	mappingsJSON, err := marshalGitOpsBranchMappings(app.GitOpsBranchMappings)
	if err != nil {
		return err
	}
	releaseBranchesJSON, err := marshalReleaseBranchOptions(app.ReleaseBranches)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(
		ctx,
		q,
		app.ID,
		app.Name,
		app.Key,
		app.ProjectID,
		app.RepoURL,
		app.Description,
		app.OwnerUserID,
		app.Owner,
		string(app.Status),
		app.ArtifactType,
		app.Language(),
		mappingsJSON,
		releaseBranchesJSON,
		app.CreatedAt.UTC().UnixNano(),
		app.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isDuplicateKeyError(r.dbDriver, err) {
			return domain.ErrKeyDuplicated
		}
		return err
	}
	return nil
}

// GetByID 查询并返回指定资源数据。
func (r *ApplicationRepository) GetByID(ctx context.Context, id string) (domain.Application, error) {
	const q = `
SELECT a.id, a.name, a.app_key, a.project_id, COALESCE(p.name, ''), COALESCE(p.project_key, ''), a.repo_url, a.description, a.owner_user_id, a.owner, a.status, a.artifact_type, a.language, a.gitops_branch_mappings, a.release_branches, a.created_at, a.updated_at
FROM applications a
LEFT JOIN projects p ON p.id = a.project_id
WHERE a.id = ?;`

	row := r.db.QueryRowContext(ctx, q, id)
	app, err := scanApplication(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Application{}, domain.ErrNotFound
		}
		return domain.Application{}, err
	}
	return app, nil
}

// List 查询并返回列表数据。
func (r *ApplicationRepository) List(ctx context.Context, filter domain.ListFilter) ([]domain.Application, int64, error) {
	args := make([]any, 0, 4)
	builder := strings.Builder{}
	countBuilder := strings.Builder{}
	countBuilder.WriteString(`SELECT COUNT(1) FROM applications a`)

	where := make([]string, 0, 4)
	if len(filter.ApplicationIDs) > 0 {
		placeholders := make([]string, 0, len(filter.ApplicationIDs))
		for _, item := range filter.ApplicationIDs {
			placeholders = append(placeholders, "?")
			args = append(args, item)
		}
		where = append(where, "a.id IN ("+strings.Join(placeholders, ", ")+")")
	}
	if filter.Keyword != "" {
		where = append(where, "(a.app_key LIKE ? OR a.name LIKE ?)")
		like := "%" + filter.Keyword + "%"
		args = append(args, like, like)
	}
	if filter.Key != "" {
		where = append(where, "a.app_key = ?")
		args = append(args, filter.Key)
	}
	if filter.Name != "" {
		where = append(where, "a.name = ?")
		args = append(args, filter.Name)
	}
	if filter.ProjectID != "" {
		where = append(where, "a.project_id = ?")
		args = append(args, filter.ProjectID)
	}
	if filter.Status != "" {
		where = append(where, "a.status = ?")
		args = append(args, string(filter.Status))
	}
	if len(where) > 0 {
		countBuilder.WriteString(" WHERE ")
		countBuilder.WriteString(strings.Join(where, " AND "))
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, countBuilder.String(), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	builder.WriteString(`
SELECT a.id, a.name, a.app_key, a.project_id, COALESCE(p.name, ''), COALESCE(p.project_key, ''), a.repo_url, a.description, a.owner_user_id, a.owner, a.status, a.artifact_type, a.language, a.gitops_branch_mappings, a.release_branches, a.created_at, a.updated_at
FROM applications a
LEFT JOIN projects p ON p.id = a.project_id`)
	if len(where) > 0 {
		builder.WriteString(" WHERE ")
		builder.WriteString(strings.Join(where, " AND "))
	}
	builder.WriteString(" ORDER BY a.created_at DESC LIMIT ? OFFSET ?;")

	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, builder.String(), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	apps := make([]domain.Application, 0)
	for rows.Next() {
		app, err := scanApplication(rows)
		if err != nil {
			return nil, 0, err
		}
		apps = append(apps, app)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return apps, total, nil
}

// Update 更新业务资源并返回处理结果。
func (r *ApplicationRepository) Update(ctx context.Context, id string, input domain.UpdateInput, updatedAt time.Time) (domain.Application, error) {
	const q = `
UPDATE applications
SET name = ?, app_key = ?, project_id = ?, repo_url = ?, description = ?, owner_user_id = ?, owner = ?, status = ?, artifact_type = ?, language = ?, gitops_branch_mappings = ?, release_branches = ?, updated_at = ?
WHERE id = ?;`

	mappingsJSON, err := marshalGitOpsBranchMappings(input.GitOpsBranchMappings)
	if err != nil {
		return domain.Application{}, err
	}
	releaseBranchesJSON, err := marshalReleaseBranchOptions(input.ReleaseBranches)
	if err != nil {
		return domain.Application{}, err
	}

	res, err := r.db.ExecContext(
		ctx,
		q,
		input.Name,
		input.Key,
		input.ProjectID,
		input.RepoURL,
		input.Description,
		input.OwnerUserID,
		input.Owner,
		string(input.Status),
		input.ArtifactType,
		input.Language,
		mappingsJSON,
		releaseBranchesJSON,
		updatedAt.UTC().UnixNano(),
		id,
	)
	if err != nil {
		if isDuplicateKeyError(r.dbDriver, err) {
			return domain.Application{}, domain.ErrKeyDuplicated
		}
		return domain.Application{}, err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Application{}, err
	}
	if affected == 0 {
		return domain.Application{}, domain.ErrNotFound
	}
	return r.GetByID(ctx, id)
}

// Delete 删除业务资源并返回处理结果。
func (r *ApplicationRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM applications WHERE id = ?;`

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

type scanner interface {
	Scan(dest ...any) error
}

// scanApplication 封装当前模块的业务处理逻辑。
func scanApplication(s scanner) (domain.Application, error) {
	var (
		app                domain.Application
		statusRaw          string
		langRaw            string
		mappingsRaw        sql.NullString
		releaseBranchesRaw sql.NullString
		createdAt          int64
		updatedAt          int64
	)

	if err := s.Scan(
		&app.ID,
		&app.Name,
		&app.Key,
		&app.ProjectID,
		&app.ProjectName,
		&app.ProjectKey,
		&app.RepoURL,
		&app.Description,
		&app.OwnerUserID,
		&app.Owner,
		&statusRaw,
		&app.ArtifactType,
		&langRaw,
		&mappingsRaw,
		&releaseBranchesRaw,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.Application{}, err
	}

	app.Status = domain.Status(statusRaw)
	app.SetLanguage(langRaw)
	app.GitOpsBranchMappings = unmarshalGitOpsBranchMappings(mappingsRaw.String)
	app.ReleaseBranches = unmarshalReleaseBranchOptions(releaseBranchesRaw.String)
	app.CreatedAt = time.Unix(0, createdAt).UTC()
	app.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return app, nil
}

// marshalReleaseBranchOptions 封装当前模块的业务处理逻辑。
func marshalReleaseBranchOptions(values []domain.ReleaseBranchOption) (string, error) {
	if len(values) == 0 {
		return "[]", nil
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// unmarshalReleaseBranchOptions 封装当前模块的业务处理逻辑。
func unmarshalReleaseBranchOptions(raw string) []domain.ReleaseBranchOption {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	items := make([]domain.ReleaseBranchOption, 0)
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	result := make([]domain.ReleaseBranchOption, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		branch := strings.TrimSpace(item.Branch)
		if branch == "" {
			continue
		}
		key := strings.ToLower(branch)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if name == "" {
			name = branch
		}
		result = append(result, domain.ReleaseBranchOption{
			Name:   name,
			Branch: branch,
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// marshalGitOpsBranchMappings 封装当前模块的业务处理逻辑。
func marshalGitOpsBranchMappings(values []domain.GitOpsBranchMapping) (string, error) {
	if len(values) == 0 {
		return "[]", nil
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// unmarshalGitOpsBranchMappings 封装当前模块的业务处理逻辑。
func unmarshalGitOpsBranchMappings(raw string) []domain.GitOpsBranchMapping {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	items := make([]domain.GitOpsBranchMapping, 0)
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	result := make([]domain.GitOpsBranchMapping, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		envCode := strings.TrimSpace(item.EnvCode)
		branch := strings.TrimSpace(item.Branch)
		if envCode == "" || branch == "" {
			continue
		}
		key := strings.ToLower(envCode)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, domain.GitOpsBranchMapping{
			EnvCode: envCode,
			Branch:  branch,
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// mysqlColumnExists 封装当前模块的业务处理逻辑。
func (r *ApplicationRepository) mysqlColumnExists(ctx context.Context, table, column string) (bool, error) {
	const q = `
SELECT COUNT(1)
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?;`

	var count int
	if err := r.db.QueryRowContext(ctx, q, table, column).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// sqliteTableColumns 封装当前模块的业务处理逻辑。
func (r *ApplicationRepository) sqliteTableColumns(ctx context.Context, table string) (map[string]struct{}, error) {
	q := fmt.Sprintf("PRAGMA table_info(%q);", table)
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]struct{})
	for rows.Next() {
		var (
			cid       int
			name      string
			typ       string
			notNull   int
			defaultV  sql.NullString
			primaryID int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultV, &primaryID); err != nil {
			return nil, err
		}
		columns[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

// isDuplicateKeyError 封装当前模块的业务处理逻辑。
func isDuplicateKeyError(dbDriver string, err error) bool {
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
