package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/artifactrepo"
)

type ArtifactRepositoryConfigRepository struct {
	db       *sql.DB
	dbDriver string
}

func NewArtifactRepositoryConfigRepository(db *sql.DB, dbDriver string) *ArtifactRepositoryConfigRepository {
	return &ArtifactRepositoryConfigRepository{
		db:       db,
		dbDriver: strings.ToLower(strings.TrimSpace(dbDriver)),
	}
}

func (r *ArtifactRepositoryConfigRepository) InitSchema(ctx context.Context) error {
	var schema string
	switch r.dbDriver {
	case "mysql":
		schema = `
CREATE TABLE IF NOT EXISTS artifact_repository_config (
	id VARCHAR(64) PRIMARY KEY,
	name VARCHAR(100) NOT NULL,
	repository_type VARCHAR(50) NOT NULL,
	endpoint VARCHAR(500) NOT NULL,
	bucket VARCHAR(200) NOT NULL,
	directory VARCHAR(500) NOT NULL,
	access_key_id VARCHAR(255) NOT NULL,
	access_key_secret_ciphertext TEXT NOT NULL,
	acl VARCHAR(50) NOT NULL,
	status VARCHAR(50) NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	UNIQUE KEY uq_artifact_repository_name (name),
	KEY idx_artifact_repository_type_status_updated_at (repository_type, status, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`
	case "sqlite":
		schema = `
CREATE TABLE IF NOT EXISTS artifact_repository_config (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	repository_type TEXT NOT NULL,
	endpoint TEXT NOT NULL,
	bucket TEXT NOT NULL,
	directory TEXT NOT NULL,
	access_key_id TEXT NOT NULL,
	access_key_secret_ciphertext TEXT NOT NULL,
	acl TEXT NOT NULL,
	status TEXT NOT NULL,
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
		_, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_artifact_repository_type_status_updated_at ON artifact_repository_config (repository_type, status, updated_at);`)
		return err
	}
	return nil
}

func (r *ArtifactRepositoryConfigRepository) Create(ctx context.Context, item domain.ArtifactRepository) error {
	encryptedSecret, err := encryptStoredSecret(strings.TrimSpace(item.AccessKeySecret))
	if err != nil {
		return err
	}
	const q = `
INSERT INTO artifact_repository_config (
	id, name, repository_type, endpoint, bucket, directory, access_key_id, access_key_secret_ciphertext,
	acl, status, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err = r.db.ExecContext(
		ctx,
		q,
		item.ID,
		item.Name,
		string(item.RepositoryType),
		item.Endpoint,
		item.Bucket,
		item.Directory,
		item.AccessKeyID,
		encryptedSecret,
		string(item.ACL),
		string(item.Status),
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

func (r *ArtifactRepositoryConfigRepository) GetByID(ctx context.Context, id string) (domain.ArtifactRepository, error) {
	const q = `
SELECT id, name, repository_type, endpoint, bucket, directory, access_key_id, access_key_secret_ciphertext,
	acl, status, created_at, updated_at
FROM artifact_repository_config
WHERE id = ?;`
	item, err := scanArtifactRepository(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ArtifactRepository{}, domain.ErrNotFound
		}
		return domain.ArtifactRepository{}, err
	}
	return item, nil
}

func (r *ArtifactRepositoryConfigRepository) List(ctx context.Context, filter domain.ListFilter) ([]domain.ArtifactRepository, int64, error) {
	where, args := buildArtifactRepositoryWhere(filter)
	countQ := `SELECT COUNT(1) FROM artifact_repository_config` + where
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
SELECT id, name, repository_type, endpoint, bucket, directory, access_key_id, access_key_secret_ciphertext,
	acl, status, created_at, updated_at
FROM artifact_repository_config` + where + `
ORDER BY updated_at DESC, created_at DESC
LIMIT ? OFFSET ?;`
	rows, err := r.db.QueryContext(ctx, q, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]domain.ArtifactRepository, 0)
	for rows.Next() {
		item, scanErr := scanArtifactRepository(rows)
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

func (r *ArtifactRepositoryConfigRepository) Update(ctx context.Context, id string, input domain.UpdateInput, updatedAt time.Time) (domain.ArtifactRepository, error) {
	encryptedSecret, err := encryptStoredSecret(strings.TrimSpace(input.AccessKeySecret))
	if err != nil {
		return domain.ArtifactRepository{}, err
	}
	const q = `
UPDATE artifact_repository_config
SET name = ?, repository_type = ?, endpoint = ?, bucket = ?, directory = ?, access_key_id = ?,
	access_key_secret_ciphertext = ?, acl = ?, status = ?, updated_at = ?
WHERE id = ?;`
	result, err := r.db.ExecContext(
		ctx,
		q,
		input.Name,
		string(input.RepositoryType),
		input.Endpoint,
		input.Bucket,
		input.Directory,
		input.AccessKeyID,
		encryptedSecret,
		string(input.ACL),
		string(input.Status),
		updatedAt.UTC().UnixNano(),
		id,
	)
	if err != nil {
		if isDuplicateKeyError(r.dbDriver, err) {
			return domain.ArtifactRepository{}, domain.ErrNameDuplicated
		}
		return domain.ArtifactRepository{}, err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return domain.ArtifactRepository{}, domain.ErrNotFound
	}
	return r.GetByID(ctx, id)
}

func (r *ArtifactRepositoryConfigRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM artifact_repository_config WHERE id = ?;`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func buildArtifactRepositoryWhere(filter domain.ListFilter) (string, []any) {
	conditions := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if strings.TrimSpace(filter.Keyword) != "" {
		conditions = append(conditions, "(name LIKE ? OR endpoint LIKE ? OR bucket LIKE ?)")
		keyword := "%" + strings.TrimSpace(filter.Keyword) + "%"
		args = append(args, keyword, keyword, keyword)
	}
	if filter.RepositoryType != "" {
		conditions = append(conditions, "repository_type = ?")
		args = append(args, string(filter.RepositoryType))
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

type artifactRepositoryScanner interface {
	Scan(dest ...any) error
}

func scanArtifactRepository(scanner artifactRepositoryScanner) (domain.ArtifactRepository, error) {
	var (
		item            domain.ArtifactRepository
		repositoryType  string
		encryptedSecret string
		acl             string
		status          string
		createdAt       int64
		updatedAt       int64
	)
	if err := scanner.Scan(
		&item.ID,
		&item.Name,
		&repositoryType,
		&item.Endpoint,
		&item.Bucket,
		&item.Directory,
		&item.AccessKeyID,
		&encryptedSecret,
		&acl,
		&status,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.ArtifactRepository{}, err
	}
	secret, err := decryptStoredSecret(encryptedSecret)
	if err != nil {
		return domain.ArtifactRepository{}, err
	}
	item.RepositoryType = domain.RepositoryType(repositoryType)
	item.AccessKeySecret = secret
	item.ACL = domain.ACL(acl)
	item.Status = domain.Status(status)
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}
