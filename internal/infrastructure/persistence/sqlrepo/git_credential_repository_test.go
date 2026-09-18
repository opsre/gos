package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	domain "gos/internal/domain/gitcredential"
	"gos/internal/support/secure"

	_ "modernc.org/sqlite"
)

func newGitCredentialTestRepository(t *testing.T) (*GitCredentialRepository, *sql.DB) {
	t.Helper()
	secure.SetSecretKey("git-credential-test-key")
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewGitCredentialRepository(db, "sqlite")
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema err = %v", err)
	}
	return repo, db
}

func gitCredentialTestItem(id string, now time.Time) domain.Credential {
	return domain.Credential{
		ID:        id,
		Name:      "gitlab-cloud",
		Provider:  domain.ProviderGitLab,
		BaseURL:   "http://git.cloud.local:9080",
		Username:  "release-bot",
		Secret:    "glpat-plain-secret",
		AuthType:  domain.AuthTypeToken,
		Status:    domain.StatusActive,
		Remark:    "内网 GitLab",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestGitCredentialRepositoryCRUDEncryptsSecret(t *testing.T) {
	repo, db := newGitCredentialTestRepository(t)
	ctx := context.Background()
	now := time.Unix(1000, 0).UTC()

	item := gitCredentialTestItem("gc-1", now)
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("Create err = %v", err)
	}

	got, err := repo.GetByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if got.Secret != "glpat-plain-secret" {
		t.Fatalf("Secret = %q, want the decrypted secret", got.Secret)
	}
	if got.Provider != domain.ProviderGitLab || got.AuthType != domain.AuthTypeToken || got.Status != domain.StatusActive {
		t.Fatalf("GetByID returned %+v", got)
	}
	if !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Fatalf("timestamps = %v / %v, want %v", got.CreatedAt, got.UpdatedAt, now)
	}

	var stored string
	if err := db.QueryRowContext(ctx, `SELECT secret_ciphertext FROM git_credential WHERE id = ?`, item.ID).Scan(&stored); err != nil {
		t.Fatalf("query stored secret: %v", err)
	}
	if stored == "glpat-plain-secret" {
		t.Fatalf("secret is stored in clear text")
	}
	if !strings.HasPrefix(stored, "enc:v1:") {
		t.Fatalf("stored secret = %q, want the secure prefix", stored)
	}

	var migrationVersion string
	if err := db.QueryRowContext(
		ctx,
		`SELECT version FROM gos_schema_migration WHERE version = ?`,
		"20260918_01_git_credential",
	).Scan(&migrationVersion); err != nil {
		t.Fatalf("schema migration was not recorded: %v", err)
	}

	items, total, err := repo.List(ctx, domain.ListFilter{Keyword: "gitlab", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("List total=%d len=%d, want 1", total, len(items))
	}
	if _, disabledTotal, err := repo.List(ctx, domain.ListFilter{Status: domain.StatusDisabled, Page: 1, PageSize: 20}); err != nil {
		t.Fatalf("List disabled err = %v", err)
	} else if disabledTotal != 0 {
		t.Fatalf("disabled total = %d, want 0", disabledTotal)
	}

	updated, err := repo.Update(ctx, item.ID, domain.UpdateInput{
		Name:     "gitlab-cloud-2",
		Provider: domain.ProviderGitLab,
		BaseURL:  "http://192.168.2.34:9080",
		Username: "release-bot-2",
		Secret:   "glpat-rotated",
		AuthType: domain.AuthTypePassword,
		Status:   domain.StatusDisabled,
		Remark:   "备用地址",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Update err = %v", err)
	}
	if updated.Name != "gitlab-cloud-2" || updated.BaseURL != "http://192.168.2.34:9080" || updated.Secret != "glpat-rotated" {
		t.Fatalf("Update returned %+v", updated)
	}
	if updated.Status != domain.StatusDisabled || updated.AuthType != domain.AuthTypePassword {
		t.Fatalf("Update status/auth type = %v / %v", updated.Status, updated.AuthType)
	}
	var rotated string
	if err := db.QueryRowContext(ctx, `SELECT secret_ciphertext FROM git_credential WHERE id = ?`, item.ID).Scan(&rotated); err != nil {
		t.Fatalf("query rotated secret: %v", err)
	}
	if rotated == "glpat-rotated" || rotated == stored {
		t.Fatalf("rotated secret was not re-encrypted")
	}

	if err := repo.Delete(ctx, item.ID); err != nil {
		t.Fatalf("Delete err = %v", err)
	}
	if _, err := repo.GetByID(ctx, item.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID after delete err = %v, want ErrNotFound", err)
	}
	if err := repo.Delete(ctx, item.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second Delete err = %v, want ErrNotFound", err)
	}
	if _, err := repo.Update(ctx, "gc-missing", domain.UpdateInput{Name: "x", Provider: domain.ProviderGitLab, BaseURL: "http://git.cloud.local:9080", AuthType: domain.AuthTypeToken, Status: domain.StatusActive}, now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Update missing err = %v, want ErrNotFound", err)
	}
}

func TestGitCredentialRepositoryRejectsDuplicateName(t *testing.T) {
	repo, _ := newGitCredentialTestRepository(t)
	ctx := context.Background()
	now := time.Unix(2000, 0).UTC()

	first := gitCredentialTestItem("gc-1", now)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create err = %v", err)
	}

	second := gitCredentialTestItem("gc-2", now)
	if err := repo.Create(ctx, second); !errors.Is(err, domain.ErrNameDuplicated) {
		t.Fatalf("duplicate Create err = %v, want ErrNameDuplicated", err)
	}

	third := gitCredentialTestItem("gc-3", now)
	third.Name = "gitlab-ip"
	if err := repo.Create(ctx, third); err != nil {
		t.Fatalf("Create with a free name err = %v", err)
	}
	if _, err := repo.Update(ctx, third.ID, domain.UpdateInput{
		Name:     "gitlab-cloud",
		Provider: domain.ProviderGitLab,
		BaseURL:  third.BaseURL,
		AuthType: domain.AuthTypeToken,
		Status:   domain.StatusActive,
	}, now); !errors.Is(err, domain.ErrNameDuplicated) {
		t.Fatalf("duplicate Update err = %v, want ErrNameDuplicated", err)
	}
}
