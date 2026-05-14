package sqlrepo

import (
	"context"
	"database/sql"
	"testing"
	"time"

	appdomain "gos/internal/domain/application"

	_ "modernc.org/sqlite"
)

func TestApplicationRepositoryPersistsArtifactBinding(t *testing.T) {
	repo := newTestApplicationRepository(t)
	ctx := context.Background()
	now := time.Unix(1_710_000_000, 0).UTC()

	app := appdomain.Application{
		ID:                   "app-pay",
		Name:                 "支付中心",
		Key:                  "pay-center",
		ProjectID:            "project-1",
		OwnerUserID:          "user-1",
		Owner:                "赵昊宇",
		Status:               appdomain.StatusActive,
		ArtifactType:         "jar",
		ArtifactRepositoryID: "repo-oss",
		ArtifactDirectory:    "release/pay-center",
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	app.SetLanguage("java")

	if err := repo.Create(ctx, app); err != nil {
		t.Fatalf("Create err = %v", err)
	}

	got, err := repo.GetByID(ctx, "app-pay")
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if got.ArtifactRepositoryID != "repo-oss" {
		t.Fatalf("ArtifactRepositoryID = %q", got.ArtifactRepositoryID)
	}
	if got.ArtifactDirectory != "release/pay-center" {
		t.Fatalf("ArtifactDirectory = %q", got.ArtifactDirectory)
	}

	updated, err := repo.Update(ctx, "app-pay", appdomain.UpdateInput{
		Name:                 "支付中心",
		Key:                  "pay-center",
		ProjectID:            "project-1",
		OwnerUserID:          "user-1",
		Owner:                "赵昊宇",
		Status:               appdomain.StatusActive,
		ArtifactType:         "jar",
		Language:             "java",
		ArtifactRepositoryID: "repo-prod",
		ArtifactDirectory:    "release/pay-center/prod",
	}, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Update err = %v", err)
	}
	if updated.ArtifactRepositoryID != "repo-prod" {
		t.Fatalf("updated ArtifactRepositoryID = %q", updated.ArtifactRepositoryID)
	}
	if updated.ArtifactDirectory != "release/pay-center/prod" {
		t.Fatalf("updated ArtifactDirectory = %q", updated.ArtifactDirectory)
	}
}

func newTestApplicationRepository(t *testing.T) *ApplicationRepository {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() {
		_ = db.Close()
	})

	repo := NewApplicationRepository(db, "sqlite")
	projects := NewProjectRepository(db, "sqlite")
	if err := projects.InitSchema(context.Background()); err != nil {
		t.Fatalf("Init project schema err = %v", err)
	}
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema err = %v", err)
	}
	return repo
}
