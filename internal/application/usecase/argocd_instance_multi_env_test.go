package usecase

import (
	"context"
	"database/sql"
	"testing"
	"time"

	argocddomain "gos/internal/domain/argocdapp"
	"gos/internal/infrastructure/persistence/sqlrepo"

	_ "modernc.org/sqlite"
)

func TestUpdateEnvBindingsAllowsMultipleInstancesForSameEnv(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlrepo.NewArgoCDApplicationRepository(db, "sqlite")
	ctx := context.Background()
	if err := sqlrepo.NewGitOpsRepository(db, "sqlite").InitSchema(ctx); err != nil {
		t.Fatalf("gitops InitSchema failed: %v", err)
	}
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	now := time.Now().UTC()
	for _, item := range []argocddomain.Instance{
		{
			ID:           "argocd-shanghai",
			InstanceCode: "argocd-shanghai",
			Name:         "ArgoCD Shanghai",
			BaseURL:      "https://argocd-shanghai.example.com",
			AuthMode:     "token",
			Status:       argocddomain.StatusActive,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "argocd-east",
			InstanceCode: "argocd-east",
			Name:         "ArgoCD East",
			BaseURL:      "https://argocd-east.example.com",
			AuthMode:     "token",
			Status:       argocddomain.StatusActive,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	} {
		if _, err := repo.UpsertInstance(ctx, item); err != nil {
			t.Fatalf("UpsertInstance(%s) failed: %v", item.ID, err)
		}
	}

	manager := NewArgoCDInstanceManager(repo, nil, nil)
	got, err := manager.UpdateEnvBindings(ctx, []UpdateArgoCDEnvBindingItem{
		{EnvCode: "prod", ArgoCDInstanceID: "argocd-shanghai", Status: argocddomain.StatusActive},
		{EnvCode: "prod", ArgoCDInstanceID: "argocd-east", Status: argocddomain.StatusActive},
	})
	if err != nil {
		t.Fatalf("UpdateEnvBindings failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("UpdateEnvBindings returned %d bindings, want 2", len(got))
	}
}
