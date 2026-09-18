package sqlrepo

import (
	"context"
	"database/sql"
	"testing"
	"time"

	domain "gos/internal/domain/artifactrepo"
	"gos/internal/support/secure"

	_ "modernc.org/sqlite"
)

func TestArtifactRepositoryConfigRepositoryCRUD(t *testing.T) {
	secure.SetSecretKey("artifact-repository-test-key")
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	repo := NewArtifactRepositoryConfigRepository(db, "sqlite")
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema err = %v", err)
	}

	now := time.Unix(1000, 0).UTC()
	item := domain.ArtifactRepository{
		ID:              "arc-1",
		Name:            "oa",
		RepositoryType:  domain.RepositoryTypeOSS,
		Endpoint:        "https://oss.example.com",
		Bucket:          "oa",
		Directory:       "release/jar",
		AccessKeyID:     "ak",
		AccessKeySecret: "secret",
		ACL:             domain.ACLPrivate,
		Status:          domain.StatusEnabled,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("Create err = %v", err)
	}

	got, err := repo.GetByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if got.AccessKeySecret != "secret" {
		t.Fatalf("AccessKeySecret = %q, want decrypted secret", got.AccessKeySecret)
	}

	var storedSecret string
	if err := db.QueryRowContext(ctx, `SELECT access_key_secret_ciphertext FROM artifact_repository_config WHERE id = ?`, item.ID).Scan(&storedSecret); err != nil {
		t.Fatalf("query stored secret: %v", err)
	}
	if storedSecret == "secret" {
		t.Fatalf("stored secret should be encrypted")
	}

	items, total, err := repo.List(ctx, domain.ListFilter{Keyword: "oa", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("List total=%d len=%d, want 1", total, len(items))
	}

	updated, err := repo.Update(ctx, item.ID, domain.UpdateInput{
		Name:            "oa-prod",
		RepositoryType:  domain.RepositoryTypeOSS,
		Endpoint:        "https://oss-prod.example.com",
		Bucket:          "oa-prod",
		Directory:       "/",
		AccessKeyID:     "ak-prod",
		AccessKeySecret: "secret-2",
		ACL:             domain.ACLPublicRead,
		Status:          domain.StatusDisabled,
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Update err = %v", err)
	}
	if updated.Name != "oa-prod" || updated.AccessKeySecret != "secret-2" || updated.ACL != domain.ACLPublicRead {
		t.Fatalf("Update returned %+v", updated)
	}

	if err := repo.Delete(ctx, item.ID); err != nil {
		t.Fatalf("Delete err = %v", err)
	}
	if _, err := repo.GetByID(ctx, item.ID); err != domain.ErrNotFound {
		t.Fatalf("GetByID after delete err = %v, want ErrNotFound", err)
	}
}

func TestArtifactRepositoryConfigRepositorySFTPRoundTrip(t *testing.T) {
	secure.SetSecretKey("artifact-repository-test-key")
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	repo := NewArtifactRepositoryConfigRepository(db, "sqlite")
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema err = %v", err)
	}

	now := time.Unix(2000, 0).UTC()
	item := domain.ArtifactRepository{
		ID:                 "arc-sftp",
		Name:               "oa-sftp",
		RepositoryType:     domain.RepositoryTypeSFTP,
		Endpoint:           "sftp.example.com",
		Port:               2222,
		Directory:          "/data/release",
		Username:           "deploy",
		Password:           "sftp-password",
		PrivateKey:         "-----BEGIN OPENSSH PRIVATE KEY-----\nkeybody\n-----END OPENSSH PRIVATE KEY-----",
		HostKeyFingerprint: "SHA256:9f8a7b6c5d4e",
		ACL:                domain.ACLPrivate,
		Status:             domain.StatusEnabled,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("Create err = %v", err)
	}

	got, err := repo.GetByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if got.RepositoryType != domain.RepositoryTypeSFTP {
		t.Fatalf("RepositoryType = %q, want sftp", got.RepositoryType)
	}
	if got.Port != 2222 || got.Username != "deploy" || got.Directory != "/data/release" {
		t.Fatalf("connection fields not round-tripped: %+v", got)
	}
	if got.Password != "sftp-password" {
		t.Fatalf("Password = %q, want decrypted value", got.Password)
	}
	if got.PrivateKey != item.PrivateKey {
		t.Fatalf("PrivateKey = %q, want decrypted value", got.PrivateKey)
	}
	if got.HostKeyFingerprint != "SHA256:9f8a7b6c5d4e" {
		t.Fatalf("HostKeyFingerprint = %q, want round-tripped value", got.HostKeyFingerprint)
	}
	// FTP concerns do not apply to an SFTP row, so both stay at their zero values.
	if got.DisableEPSV {
		t.Fatalf("DisableEPSV = true, want false for an SFTP row")
	}

	// Both new secrets must be encrypted at rest, not merely stored verbatim.
	rows, err := db.QueryContext(ctx, `SELECT password_ciphertext, private_key_ciphertext FROM artifact_repository_config WHERE id = ?`, item.ID)
	if err != nil {
		t.Fatalf("query stored secrets: %v", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("stored row missing")
	}
	var storedPassword, storedPrivateKey string
	if err := rows.Scan(&storedPassword, &storedPrivateKey); err != nil {
		t.Fatalf("scan stored secrets: %v", err)
	}
	if storedPassword == "sftp-password" {
		t.Fatalf("password should be encrypted at rest")
	}
	if storedPrivateKey == item.PrivateKey {
		t.Fatalf("private key should be encrypted at rest")
	}
}

func TestArtifactRepositoryConfigRepositoryFTPRoundTrip(t *testing.T) {
	secure.SetSecretKey("artifact-repository-test-key")
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	repo := NewArtifactRepositoryConfigRepository(db, "sqlite")
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema err = %v", err)
	}

	now := time.Unix(3000, 0).UTC()
	item := domain.ArtifactRepository{
		ID:             "arc-ftp",
		Name:           "oa-ftp",
		RepositoryType: domain.RepositoryTypeFTP,
		Endpoint:       "10.8.0.14",
		Port:           21,
		Directory:      "releases",
		Username:       "ftpuser",
		Password:       "ftp-password",
		// disable_epsv is the non-default for FTP, so setting it here proves the
		// column round-trips rather than merely matching the zero value.
		DisableEPSV: true,
		// The repository layer stores fields verbatim; defaulting ACL and status
		// is the use case's job, so this test pins them explicitly.
		ACL:       domain.ACLPrivate,
		Status:    domain.StatusEnabled,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("Create err = %v", err)
	}

	got, err := repo.GetByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if got.RepositoryType != domain.RepositoryTypeFTP || got.Port != 21 || got.Username != "ftpuser" {
		t.Fatalf("connection fields not round-tripped: %+v", got)
	}
	if got.Password != "ftp-password" {
		t.Fatalf("Password = %q, want decrypted value", got.Password)
	}
	if !got.DisableEPSV {
		t.Fatalf("DisableEPSV = false, want true")
	}
	// An FTP row carries no host key fingerprint, so the column must stay empty.
	if got.HostKeyFingerprint != "" {
		t.Fatalf("HostKeyFingerprint = %q, want empty", got.HostKeyFingerprint)
	}
	// An FTP row carries no private key, so the column must stay empty rather
	// than hold a ciphertext of nothing.
	if got.PrivateKey != "" {
		t.Fatalf("PrivateKey = %q, want empty", got.PrivateKey)
	}
	if got.ACL != domain.ACLPrivate {
		t.Fatalf("ACL = %q, want private", got.ACL)
	}
}

// TestArtifactRepositoryConfigRepositoryMigratesLegacyTable guards the upgrade
// path: a deployment that already has the OSS-era table must gain the FTP/SFTP
// columns on boot without losing existing rows, and InitSchema must stay safe
// to run repeatedly.
func TestArtifactRepositoryConfigRepositoryMigratesLegacyTable(t *testing.T) {
	secure.SetSecretKey("artifact-repository-test-key")
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	const legacySchema = `
CREATE TABLE artifact_repository_config (
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
	if _, err := db.ExecContext(ctx, legacySchema); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO artifact_repository_config
			(id, name, repository_type, endpoint, bucket, directory, access_key_id, access_key_secret_ciphertext, acl, status, created_at, updated_at)
		 VALUES ('arc-legacy', 'legacy', 'oss', 'https://oss.example.com', 'legacy', '/', 'ak', '', 'private', 'enabled', 1, 1);`,
	); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	repo := NewArtifactRepositoryConfigRepository(db, "sqlite")
	// Running twice proves the guarded ALTERs are idempotent: the second pass
	// finds the columns already present and does nothing.
	for attempt := 1; attempt <= 2; attempt++ {
		if err := repo.InitSchema(ctx); err != nil {
			t.Fatalf("InitSchema attempt %d err = %v", attempt, err)
		}
	}

	got, err := repo.GetByID(ctx, "arc-legacy")
	if err != nil {
		t.Fatalf("GetByID legacy row err = %v", err)
	}
	if got.Name != "legacy" || got.Endpoint != "https://oss.example.com" || got.Bucket != "legacy" {
		t.Fatalf("legacy row was altered: %+v", got)
	}
	if got.Port != 0 || got.Username != "" || got.Password != "" || got.PrivateKey != "" {
		t.Fatalf("remote access fields should default to empty: %+v", got)
	}

	// New columns must also be usable immediately after the upgrade.
	now := time.Unix(4000, 0).UTC()
	if err := repo.Create(ctx, domain.ArtifactRepository{
		ID:             "arc-after-migration",
		Name:           "after-migration",
		RepositoryType: domain.RepositoryTypeSFTP,
		Endpoint:       "sftp.example.com",
		Port:           22,
		Username:       "deploy",
		Password:       "pw",
		Status:         domain.StatusEnabled,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("Create after migration err = %v", err)
	}
	if _, err := repo.GetByID(ctx, "arc-after-migration"); err != nil {
		t.Fatalf("GetByID after migration err = %v", err)
	}
}
