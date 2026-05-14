package httpapi

import (
	"bytes"
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

func TestReleaseOrderHandlerRecordArtifactMetadata(t *testing.T) {
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

	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	order := releaseOrderHandlerTestOrder("ro-http-artifact", "RO-HTTP-ARTIFACT", now)
	execution := domain.ReleaseOrderExecution{
		ID:             "exec-http-ci",
		ReleaseOrderID: order.ID,
		PipelineScope:  domain.PipelineScopeCI,
		BindingID:      "binding-ci",
		BindingName:    "CI",
		Provider:       "jenkins",
		PipelineID:     "pipeline-ci",
		Status:         domain.ExecutionStatusSuccess,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.Create(context.Background(), order, []domain.ReleaseOrderExecution{execution}, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}

	manager := usecase.NewReleaseOrderManager(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		setCurrentUser(c, userdomain.User{ID: "usr-1"})
		c.Next()
	})
	NewReleaseOrderHandler(manager, nil, releaseOrderHandlerAllowAllAuthorizer{}, nil).RegisterRoutes(router)

	body := []byte(`{
		"execution_id": "exec-http-ci",
		"pipeline_scope": "ci",
		"artifact_name": "gc-certificate.jar",
		"artifact_type": "jar",
		"artifact_version": "1042",
		"artifact_url": "https://gc-oa.oss-cn-shanghai.aliyuncs.com/tempUpdate/gc-certificate-1042.jar",
		"bucket": "gc-oa",
		"object_key": "tempUpdate/gc-certificate-1042.jar",
		"size_bytes": 171609374,
		"metadata": {"commit": "abcdef1"}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/release-orders/ro-http-artifact/artifact-metadata", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp ReleaseOrderArtifactMetadataDataResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.ReleaseOrderID != order.ID {
		t.Fatalf("release_order_id = %q, want %q", resp.Data.ReleaseOrderID, order.ID)
	}
	if resp.Data.ArtifactURL == "" {
		t.Fatalf("artifact_url is empty")
	}
	if resp.Data.AdditionalFields["commit"] != "abcdef1" {
		t.Fatalf("metadata.commit = %v", resp.Data.AdditionalFields["commit"])
	}
}

func TestReleaseOrderHandlerDeleteManualArtifactMetadata(t *testing.T) {
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

	now := time.Date(2026, 5, 13, 9, 0, 0, 0, time.UTC)
	order := releaseOrderHandlerTestOrder("ro-http-artifact-delete", "RO-HTTP-ARTIFACT-DELETE", now)
	if err := repo.Create(context.Background(), order, nil, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}
	artifact, err := repo.UpsertArtifactMetadata(context.Background(), domain.ReleaseOrderArtifactMetadata{
		ID:             "roart-http-delete-manual",
		ReleaseOrderID: order.ID,
		PipelineScope:  domain.PipelineScopeCI,
		ArtifactName:   "manual-package.zip",
		ArtifactURL:    "https://oss.example.com/manual-package.zip",
		MetadataJSON:   `{"source":"manual"}`,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("UpsertArtifactMetadata failed: %v", err)
	}

	manager := usecase.NewReleaseOrderManager(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		setCurrentUser(c, userdomain.User{ID: "usr-1"})
		c.Next()
	})
	NewReleaseOrderHandler(manager, nil, releaseOrderHandlerAllowAllAuthorizer{}, nil).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodDelete, "/release-orders/ro-http-artifact-delete/artifact-metadata/"+artifact.ID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	items, err := repo.ListArtifactMetadata(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("ListArtifactMetadata failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("artifact metadata count = %d, want 0", len(items))
	}
}

func TestReleaseOrderHandlerListArtifactMetadataSummaries(t *testing.T) {
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
	if err := sqlrepo.NewProjectRepository(db, "sqlite").InitSchema(context.Background()); err != nil {
		t.Fatalf("project InitSchema failed: %v", err)
	}
	if err := sqlrepo.NewApplicationRepository(db, "sqlite").InitSchema(context.Background()); err != nil {
		t.Fatalf("application InitSchema failed: %v", err)
	}
	if err := sqlrepo.NewArtifactRepositoryConfigRepository(db, "sqlite").InitSchema(context.Background()); err != nil {
		t.Fatalf("artifact repository InitSchema failed: %v", err)
	}

	now := time.Date(2026, 5, 12, 15, 11, 21, 0, time.UTC)
	order := releaseOrderHandlerTestOrder("ro-http-artifact-list", "RO-20260512070847-8C67ECB7", now)
	order.ReleaseName = "尚信前端测试发布"
	order.ApplicationID = "app-notary"
	order.ApplicationName = "尚信前端-测试"
	order.EnvCode = "dev"
	order.Status = domain.OrderStatusSuccess
	if err := repo.Create(context.Background(), order, nil, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}
	if _, err := repo.UpsertArtifactMetadata(context.Background(), domain.ReleaseOrderArtifactMetadata{
		ID:              "roart-http-artifact-list",
		ReleaseOrderID:  order.ID,
		PipelineScope:   domain.PipelineScopeCI,
		ArtifactName:    "notarybusiness-9.zip",
		ArtifactType:    "zip",
		ArtifactVersion: "9",
		ArtifactURL:     "https://oss.example.com/notarybusiness-9.zip",
		RepositoryName:  "生产 OSS",
		Bucket:          "gc-oa",
		ObjectKey:       "release/notarybusiness-9.zip",
		BuildNumber:     "9",
		MetadataJSON:    "{}",
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("UpsertArtifactMetadata failed: %v", err)
	}

	manager := usecase.NewReleaseOrderManager(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		setCurrentUser(c, userdomain.User{ID: "usr-1"})
		c.Next()
	})
	NewReleaseOrderHandler(manager, nil, releaseOrderHandlerAllowAllAuthorizer{}, nil).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/artifacts/release-order-metadata?application_id=app-notary&keyword=%E5%89%8D%E7%AB%AF%E6%B5%8B%E8%AF%95&page=1&page_size=20", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp ReleaseOrderArtifactMetadataSummaryListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Total != 1 || len(resp.Data) != 1 {
		t.Fatalf("total=%d len=%d, want 1", resp.Total, len(resp.Data))
	}
	if resp.Data[0].ReleaseDisplayName != "尚信前端测试发布 - RO-20260512070847-8C67ECB7" {
		t.Fatalf("release_display_name = %q", resp.Data[0].ReleaseDisplayName)
	}
	if resp.Data[0].ArtifactName != "notarybusiness-9.zip" {
		t.Fatalf("artifact_name = %q", resp.Data[0].ArtifactName)
	}
}

type releaseOrderHandlerAllowAllAuthorizer struct{}

func (releaseOrderHandlerAllowAllAuthorizer) HasPermission(context.Context, userdomain.User, string, string, string) (bool, error) {
	return true, nil
}

func (releaseOrderHandlerAllowAllAuthorizer) ListEffectivePermissions(context.Context, userdomain.User) ([]userdomain.UserPermission, error) {
	return nil, nil
}

func releaseOrderHandlerTestOrder(id, orderNo string, now time.Time) domain.ReleaseOrder {
	return domain.ReleaseOrder{
		ID:                  id,
		OrderNo:             orderNo,
		OperationType:       domain.OperationTypeDeploy,
		ApplicationID:       "app-1",
		ApplicationName:     "App 1",
		TemplateID:          "tpl-1",
		TemplateName:        "Template 1",
		BindingID:           "binding-ci",
		EnvCode:             "prod",
		TriggerType:         domain.TriggerTypeManual,
		Status:              domain.OrderStatusBuilding,
		ApprovalApproverIDs: []string{},
		CreatorUserID:       "usr-1",
		TriggeredBy:         "usr-1",
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}
