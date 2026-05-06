package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	projectdomain "gos/internal/domain/project"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

type ProjectRepository struct {
	db       *sql.DB
	dbDriver string
}

// NewProjectRepository 创建并返回对应组件实例。
func NewProjectRepository(db *sql.DB, dbDriver string) *ProjectRepository {
	return &ProjectRepository{db: db, dbDriver: strings.ToLower(strings.TrimSpace(dbDriver))}
}

// InitSchema 封装当前模块的业务处理逻辑。
func (r *ProjectRepository) InitSchema(ctx context.Context) error {
	var schema string
	switch r.dbDriver {
	case "mysql":
		schema = `
CREATE TABLE IF NOT EXISTS projects (
	id VARCHAR(64) PRIMARY KEY,
	name VARCHAR(128) NOT NULL,
	project_key VARCHAR(128) NOT NULL,
	description TEXT NOT NULL,
	status VARCHAR(32) NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uq_project_key (project_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`
	case "sqlite":
		schema = `
CREATE TABLE IF NOT EXISTS projects (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	project_key TEXT NOT NULL UNIQUE,
	description TEXT NOT NULL,
	status TEXT NOT NULL,
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
func (r *ProjectRepository) Create(ctx context.Context, item projectdomain.Project) error {
	const q = `
INSERT INTO projects (id, name, project_key, description, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);`
	_, err := r.db.ExecContext(ctx, q,
		item.ID,
		item.Name,
		item.Key,
		item.Description,
		string(item.Status),
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isProjectDuplicateKeyError(r.dbDriver, err) {
			return projectdomain.ErrKeyDuplicated
		}
		return err
	}
	return nil
}

// GetByID 查询并返回指定资源数据。
func (r *ProjectRepository) GetByID(ctx context.Context, id string) (projectdomain.Project, error) {
	const q = `
SELECT id, name, project_key, description, status, created_at, updated_at
FROM projects
WHERE id = ?;`
	row := r.db.QueryRowContext(ctx, q, id)
	item, err := scanProject(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return projectdomain.Project{}, projectdomain.ErrNotFound
		}
		return projectdomain.Project{}, err
	}
	return item, nil
}

// List 查询并返回列表数据。
func (r *ProjectRepository) List(ctx context.Context, filter projectdomain.ListFilter) ([]projectdomain.Project, int64, error) {
	args := make([]any, 0, 4)
	where := make([]string, 0, 3)
	if filter.Key != "" {
		where = append(where, "project_key = ?")
		args = append(args, filter.Key)
	}
	if filter.Name != "" {
		where = append(where, "name = ?")
		args = append(args, filter.Name)
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(filter.Status))
	}

	countSQL := strings.Builder{}
	countSQL.WriteString("SELECT COUNT(1) FROM projects")
	if len(where) > 0 {
		countSQL.WriteString(" WHERE ")
		countSQL.WriteString(strings.Join(where, " AND "))
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countSQL.String(), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := strings.Builder{}
	query.WriteString(`
SELECT id, name, project_key, description, status, created_at, updated_at
FROM projects`)
	if len(where) > 0 {
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(where, " AND "))
	}
	query.WriteString(" ORDER BY created_at DESC LIMIT ? OFFSET ?;")
	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := r.db.QueryContext(ctx, query.String(), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]projectdomain.Project, 0)
	for rows.Next() {
		item, err := scanProject(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Update 更新业务资源并返回处理结果。
func (r *ProjectRepository) Update(ctx context.Context, id string, input projectdomain.UpdateInput, updatedAt time.Time) (projectdomain.Project, error) {
	const q = `
UPDATE projects
SET name = ?, project_key = ?, description = ?, status = ?, updated_at = ?
WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q,
		input.Name,
		input.Key,
		input.Description,
		string(input.Status),
		updatedAt.UTC().UnixNano(),
		id,
	)
	if err != nil {
		if isProjectDuplicateKeyError(r.dbDriver, err) {
			return projectdomain.Project{}, projectdomain.ErrKeyDuplicated
		}
		return projectdomain.Project{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return projectdomain.Project{}, err
	}
	if affected == 0 {
		return projectdomain.Project{}, projectdomain.ErrNotFound
	}
	return r.GetByID(ctx, id)
}

// Delete 删除业务资源并返回处理结果。
func (r *ProjectRepository) Delete(ctx context.Context, id string) error {
	const countQ = `SELECT COUNT(1) FROM applications WHERE project_id = ?;`
	var refs int64
	if err := r.db.QueryRowContext(ctx, countQ, id).Scan(&refs); err != nil {
		return err
	}
	if refs > 0 {
		return projectdomain.ErrInUse
	}

	const q = `DELETE FROM projects WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return projectdomain.ErrNotFound
	}
	return nil
}

type projectScanner interface{ Scan(dest ...any) error }

// scanProject 封装当前模块的业务处理逻辑。
func scanProject(s projectScanner) (projectdomain.Project, error) {
	var item projectdomain.Project
	var statusRaw string
	var createdAt int64
	var updatedAt int64
	if err := s.Scan(&item.ID, &item.Name, &item.Key, &item.Description, &statusRaw, &createdAt, &updatedAt); err != nil {
		return projectdomain.Project{}, err
	}
	item.Status = projectdomain.Status(statusRaw)
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

// isProjectDuplicateKeyError 封装当前模块的业务处理逻辑。
func isProjectDuplicateKeyError(dbDriver string, err error) bool {
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
