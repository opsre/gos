package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	domain "gos/internal/domain/release"
	userdomain "gos/internal/domain/user"
	"gos/internal/infrastructure/persistence/sqlrepo"

	_ "modernc.org/sqlite"
)

func TestReleaseOrderHandlerGetByIDIncludesDeploySnapshots(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS sys_user (
	id TEXT PRIMARY KEY,
	username TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	email TEXT NOT NULL DEFAULT '',
	phone TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'active',
	password_hash TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`); err != nil {
		t.Fatalf("create sys_user failed: %v", err)
	}

	repo := sqlrepo.NewReleaseRepository(db, "sqlite")
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	now := time.Date(2026, 5, 15, 9, 30, 0, 0, time.UTC)
	order := releaseOrderHandlerTestOrder("ro-http-detail-snapshot", "RO-HTTP-DETAIL-SNAPSHOT", now)
	order.Status = domain.OrderStatusDeploySuccess
	if err := repo.Create(context.Background(), order, nil, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}
	for _, snapshot := range []domain.DeploySnapshot{
		{
			ID:               "snapshot-shanghai",
			ReleaseOrderID:   order.ID,
			Provider:         "argocd",
			GitOpsType:       domain.GitOpsTypeHelm,
			ArgoCDInstanceID: "argocd-shanghai",
			ArgoCDAppName:    "demo-prod-shanghai",
			RepoURL:          "https://example.com/repo.git",
			Branch:           "prod",
			SourcePath:       "apps/demo/helm",
			EnvCode:          "prod",
			SnapshotPayload:  `{"image_version":"175","rules":[{"file_path":"values.yaml","target_path":"image.tag","value":"175"}]}`,
			CreatedAt:        now,
		},
		{
			ID:               "snapshot-east",
			ReleaseOrderID:   order.ID,
			Provider:         "argocd",
			GitOpsType:       domain.GitOpsTypeHelm,
			ArgoCDInstanceID: "argocd-east",
			ArgoCDAppName:    "demo-prod-east",
			RepoURL:          "https://example.com/repo.git",
			Branch:           "prod",
			SourcePath:       "apps/demo/helm",
			EnvCode:          "prod",
			SnapshotPayload:  `{"image_version":"175","rules":[{"file_path":"values.yaml","target_path":"image.tag","value":"175"}]}`,
			CreatedAt:        now.Add(time.Second),
		},
	} {
		if err := repo.CreateDeploySnapshot(context.Background(), snapshot); err != nil {
			t.Fatalf("CreateDeploySnapshot(%s) failed: %v", snapshot.ArgoCDInstanceID, err)
		}
	}

	manager := usecase.NewReleaseOrderManager(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		setCurrentUser(c, userdomain.User{ID: "usr-1"})
		c.Next()
	})
	NewReleaseOrderHandler(manager, nil, releaseOrderHandlerAllowAllAuthorizer{}, nil).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/release-orders/"+order.ID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			DeploySnapshots []struct {
				ArgoCDInstanceID string `json:"argocd_instance_id"`
				ArgoCDAppName    string `json:"argocd_app_name"`
				ImageVersion     string `json:"image_version"`
			} `json:"deploy_snapshots"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data.DeploySnapshots) != 2 {
		t.Fatalf("deploy_snapshots len = %d, want 2; body=%s", len(resp.Data.DeploySnapshots), rec.Body.String())
	}
	if resp.Data.DeploySnapshots[0].ArgoCDInstanceID != "argocd-shanghai" ||
		resp.Data.DeploySnapshots[1].ArgoCDInstanceID != "argocd-east" {
		t.Fatalf("deploy_snapshots order = %#v", resp.Data.DeploySnapshots)
	}
	if resp.Data.DeploySnapshots[0].ImageVersion != "175" {
		t.Fatalf("image_version = %q, want 175", resp.Data.DeploySnapshots[0].ImageVersion)
	}
}
