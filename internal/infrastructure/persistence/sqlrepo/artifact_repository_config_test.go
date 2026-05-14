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
