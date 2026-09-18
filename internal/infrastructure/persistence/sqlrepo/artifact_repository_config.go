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
	port INT NOT NULL DEFAULT 0,
	bucket VARCHAR(200) NOT NULL,
	directory VARCHAR(500) NOT NULL,
	access_key_id VARCHAR(255) NOT NULL,
	access_key_secret_ciphertext TEXT NOT NULL,
	username VARCHAR(255) NOT NULL DEFAULT '',
	password_ciphertext TEXT NOT NULL,
	private_key_ciphertext TEXT NOT NULL,
	disable_epsv TINYINT(1) NOT NULL DEFAULT 0,
	host_key_fingerprint VARCHAR(255) NOT NULL DEFAULT '',
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
	port INTEGER NOT NULL DEFAULT 0,
	bucket TEXT NOT NULL,
	directory TEXT NOT NULL,
	access_key_id TEXT NOT NULL,
	access_key_secret_ciphertext TEXT NOT NULL,
	username TEXT NOT NULL DEFAULT '',
	password_ciphertext TEXT NOT NULL DEFAULT '',
	private_key_ciphertext TEXT NOT NULL DEFAULT '',
	disable_epsv INTEGER NOT NULL DEFAULT 0,
	host_key_fingerprint TEXT NOT NULL DEFAULT '',
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
		if _, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_artifact_repository_type_status_updated_at ON artifact_repository_config (repository_type, status, updated_at);`); err != nil {
			return err
		}
	}
	return r.migrateSchema(ctx)
}

// migrateSchema backfills columns that were added after the table had already
// been deployed. Each statement is guarded by a column existence check, so it
// is safe to run on every boot and on a fresh install where the CREATE above
// has already produced the columns.
func (r *ArtifactRepositoryConfigRepository) migrateSchema(ctx context.Context) error {
	for _, migration := range artifactRepositoryColumnMigrations(r.dbDriver) {
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

type artifactRepositoryColumnMigration struct {
	column string
	ddl    string
}

// artifactRepositoryColumnMigrations returns the dialect-specific DDL for the
// FTP/SFTP columns. SQLite refuses to add a NOT NULL column without a default,
// while MySQL accepts one and backfills the implicit default, which is why the
// two lists cannot be shared.
func artifactRepositoryColumnMigrations(dbDriver string) []artifactRepositoryColumnMigration {
	switch dbDriver {
	case "mysql":
		return []artifactRepositoryColumnMigration{
			{"port", `ALTER TABLE artifact_repository_config ADD COLUMN port INT NOT NULL DEFAULT 0 AFTER endpoint;`},
			{"username", `ALTER TABLE artifact_repository_config ADD COLUMN username VARCHAR(255) NOT NULL DEFAULT '' AFTER access_key_secret_ciphertext;`},
			{"password_ciphertext", `ALTER TABLE artifact_repository_config ADD COLUMN password_ciphertext TEXT NOT NULL AFTER username;`},
			{"private_key_ciphertext", `ALTER TABLE artifact_repository_config ADD COLUMN private_key_ciphertext TEXT NOT NULL AFTER password_ciphertext;`},
			{"disable_epsv", `ALTER TABLE artifact_repository_config ADD COLUMN disable_epsv TINYINT(1) NOT NULL DEFAULT 0 AFTER private_key_ciphertext;`},
			{"host_key_fingerprint", `ALTER TABLE artifact_repository_config ADD COLUMN host_key_fingerprint VARCHAR(255) NOT NULL DEFAULT '' AFTER disable_epsv;`},
		}
	case "sqlite":
		return []artifactRepositoryColumnMigration{
			{"port", `ALTER TABLE artifact_repository_config ADD COLUMN port INTEGER NOT NULL DEFAULT 0;`},
			{"username", `ALTER TABLE artifact_repository_config ADD COLUMN username TEXT NOT NULL DEFAULT '';`},
			{"password_ciphertext", `ALTER TABLE artifact_repository_config ADD COLUMN password_ciphertext TEXT NOT NULL DEFAULT '';`},
			{"private_key_ciphertext", `ALTER TABLE artifact_repository_config ADD COLUMN private_key_ciphertext TEXT NOT NULL DEFAULT '';`},
			{"disable_epsv", `ALTER TABLE artifact_repository_config ADD COLUMN disable_epsv INTEGER NOT NULL DEFAULT 0;`},
			{"host_key_fingerprint", `ALTER TABLE artifact_repository_config ADD COLUMN host_key_fingerprint TEXT NOT NULL DEFAULT '';`},
		}
	default:
		return nil
	}
}

func (r *ArtifactRepositoryConfigRepository) Create(ctx context.Context, item domain.ArtifactRepository) error {
	secrets, err := encryptArtifactRepositorySecrets(item.AccessKeySecret, item.Password, item.PrivateKey)
	if err != nil {
		return err
	}
	const q = `
INSERT INTO artifact_repository_config (
	id, name, repository_type, endpoint, port, bucket, directory, access_key_id, access_key_secret_ciphertext,
	username, password_ciphertext, private_key_ciphertext, disable_epsv, host_key_fingerprint,
	acl, status, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err = r.db.ExecContext(
		ctx,
		q,
		item.ID,
		item.Name,
		string(item.RepositoryType),
		item.Endpoint,
		item.Port,
		item.Bucket,
		item.Directory,
		item.AccessKeyID,
		secrets.accessKeySecret,
		item.Username,
		secrets.password,
		secrets.privateKey,
		boolToDBValue(r.dbDriver, item.DisableEPSV),
		item.HostKeyFingerprint,
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

type encryptedArtifactRepositorySecrets struct {
	accessKeySecret string
	password        string
	privateKey      string
}

// encryptArtifactRepositorySecrets encrypts every credential column with the
// platform key. secure.EncryptString maps an empty input to an empty string, so
// a repository type that does not use a given credential stores nothing rather
// than a ciphertext of nothing.
func encryptArtifactRepositorySecrets(accessKeySecret, password, privateKey string) (encryptedArtifactRepositorySecrets, error) {
	encryptedAccessKeySecret, err := encryptStoredSecret(strings.TrimSpace(accessKeySecret))
	if err != nil {
		return encryptedArtifactRepositorySecrets{}, err
	}
	encryptedPassword, err := encryptStoredSecret(strings.TrimSpace(password))
	if err != nil {
		return encryptedArtifactRepositorySecrets{}, err
	}
	encryptedPrivateKey, err := encryptStoredSecret(strings.TrimSpace(privateKey))
	if err != nil {
		return encryptedArtifactRepositorySecrets{}, err
	}
	return encryptedArtifactRepositorySecrets{
		accessKeySecret: encryptedAccessKeySecret,
		password:        encryptedPassword,
		privateKey:      encryptedPrivateKey,
	}, nil
}

func (r *ArtifactRepositoryConfigRepository) GetByID(ctx context.Context, id string) (domain.ArtifactRepository, error) {
	const q = `
SELECT id, name, repository_type, endpoint, port, bucket, directory, access_key_id, access_key_secret_ciphertext,
	username, password_ciphertext, private_key_ciphertext, disable_epsv, host_key_fingerprint,
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
SELECT id, name, repository_type, endpoint, port, bucket, directory, access_key_id, access_key_secret_ciphertext,
	username, password_ciphertext, private_key_ciphertext, disable_epsv, host_key_fingerprint,
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
	secrets, err := encryptArtifactRepositorySecrets(input.AccessKeySecret, input.Password, input.PrivateKey)
	if err != nil {
		return domain.ArtifactRepository{}, err
	}
	const q = `
UPDATE artifact_repository_config
SET name = ?, repository_type = ?, endpoint = ?, port = ?, bucket = ?, directory = ?, access_key_id = ?,
	access_key_secret_ciphertext = ?, username = ?, password_ciphertext = ?, private_key_ciphertext = ?,
	disable_epsv = ?, host_key_fingerprint = ?, acl = ?, status = ?, updated_at = ?
WHERE id = ?;`
	result, err := r.db.ExecContext(
		ctx,
		q,
		input.Name,
		string(input.RepositoryType),
		input.Endpoint,
		input.Port,
		input.Bucket,
		input.Directory,
		input.AccessKeyID,
		secrets.accessKeySecret,
		input.Username,
		secrets.password,
		secrets.privateKey,
		boolToDBValue(r.dbDriver, input.DisableEPSV),
		input.HostKeyFingerprint,
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
		item                domain.ArtifactRepository
		repositoryType      string
		encryptedSecret     string
		encryptedPassword   string
		encryptedPrivateKey string
		disableEPSV         any
		acl                 string
		status              string
		createdAt           int64
		updatedAt           int64
	)
	if err := scanner.Scan(
		&item.ID,
		&item.Name,
		&repositoryType,
		&item.Endpoint,
		&item.Port,
		&item.Bucket,
		&item.Directory,
		&item.AccessKeyID,
		&encryptedSecret,
		&item.Username,
		&encryptedPassword,
		&encryptedPrivateKey,
		&disableEPSV,
		&item.HostKeyFingerprint,
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
	password, err := decryptStoredSecret(encryptedPassword)
	if err != nil {
		return domain.ArtifactRepository{}, err
	}
	privateKey, err := decryptStoredSecret(encryptedPrivateKey)
	if err != nil {
		return domain.ArtifactRepository{}, err
	}
	item.RepositoryType = domain.RepositoryType(repositoryType)
	item.AccessKeySecret = secret
	item.Password = password
	item.PrivateKey = privateKey
	// disable_epsv is TINYINT(1) on MySQL and INTEGER on SQLite, which the
	// drivers surface as different Go types, so it is scanned through any and
	// normalised rather than bound straight to *bool.
	item.DisableEPSV = scanBoolValue(disableEPSV)
	item.ACL = domain.ACL(acl)
	item.Status = domain.Status(status)
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

// columnExists reports whether the named column is already present on
// artifact_repository_config. MySQL and SQLite expose that through different
// catalogs, so the lookup branches on the driver.
func (r *ArtifactRepositoryConfigRepository) columnExists(ctx context.Context, column string) (bool, error) {
	const table = "artifact_repository_config"
	if r.dbDriver == "sqlite" {
		columns, err := r.sqliteTableColumns(ctx, table)
		if err != nil {
			return false, err
		}
		_, ok := columns[column]
		return ok, nil
	}
	return r.mysqlColumnExists(ctx, table, column)
}

func (r *ArtifactRepositoryConfigRepository) mysqlColumnExists(ctx context.Context, table, column string) (bool, error) {
	const q = `SELECT COUNT(1)
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?;`
	var count int
	if err := r.db.QueryRowContext(ctx, q, table, column).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *ArtifactRepositoryConfigRepository) sqliteTableColumns(ctx context.Context, table string) (map[string]struct{}, error) {
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
