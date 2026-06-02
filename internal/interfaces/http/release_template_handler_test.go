package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	executorparamdomain "gos/internal/domain/executorparam"
	pipelinedomain "gos/internal/domain/pipeline"
	releasedomain "gos/internal/domain/release"
	userdomain "gos/internal/domain/user"
	"gos/internal/infrastructure/persistence/sqlrepo"

	_ "modernc.org/sqlite"
)

func TestReleaseTemplateSyncExecutorParamDefsAllowsReleaseCreator(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, paramRepo, jenkins := newReleaseTemplateSyncHandlerTestEnv(t, map[string]bool{
		releaseTemplateSyncPermissionKey("release.create", "application", "app-1"): true,
	})

	req := httptest.NewRequest(http.MethodPost, "/release-templates/tpl-sync/sync-executor-param-defs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if jenkins.calls != 1 {
		t.Fatalf("jenkins calls = %d, want 1", jenkins.calls)
	}

	var resp struct {
		Data usecase.SyncExecutorParamDefsOutput `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Total != 1 || resp.Data.Created != 1 {
		t.Fatalf("sync result = %+v, want total=1 created=1", resp.Data)
	}

	items, total, err := paramRepo.ListByPipeline(context.Background(), executorparamdomain.ListFilter{
		PipelineID:   "pipeline-ci",
		ExecutorType: executorparamdomain.ExecutorTypeJenkins,
		Page:         1,
		PageSize:     10,
	})
	if err != nil {
		t.Fatalf("ListByPipeline failed: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("synced params total=%d len=%d, want 1", total, len(items))
	}
	if items[0].ExecutorParamName != "BRANCH" {
		t.Fatalf("executor param name = %q, want BRANCH", items[0].ExecutorParamName)
	}
}

func TestReleaseTemplateSyncExecutorParamDefsRejectsUserWithoutTemplateAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, jenkins := newReleaseTemplateSyncHandlerTestEnv(t, map[string]bool{})

	req := httptest.NewRequest(http.MethodPost, "/release-templates/tpl-sync/sync-executor-param-defs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if jenkins.calls != 0 {
		t.Fatalf("jenkins calls = %d, want 0", jenkins.calls)
	}
}

func newReleaseTemplateSyncHandlerTestEnv(
	t *testing.T,
	permissions map[string]bool,
) (*gin.Engine, *sqlrepo.ExecutorParamRepository, *releaseTemplateSyncJenkinsFake) {
	t.Helper()

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "gos-release-template-sync-test.db"))
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

	ctx := context.Background()
	applicationRepo := sqlrepo.NewApplicationRepository(db, "sqlite")
	releaseRepo := sqlrepo.NewReleaseRepository(db, "sqlite")
	pipelineRepo := sqlrepo.NewPipelineRepository(db, "sqlite")
	paramRepo := sqlrepo.NewExecutorParamRepository(db, "sqlite")

	if err := applicationRepo.InitSchema(ctx); err != nil {
		t.Fatalf("application InitSchema failed: %v", err)
	}
	if err := releaseRepo.InitSchema(ctx); err != nil {
		t.Fatalf("release InitSchema failed: %v", err)
	}
	if err := pipelineRepo.InitSchema(ctx); err != nil {
		t.Fatalf("pipeline InitSchema failed: %v", err)
	}
	if err := paramRepo.InitSchema(ctx); err != nil {
		t.Fatalf("executor param InitSchema failed: %v", err)
	}

	now := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	if _, _, err := pipelineRepo.UpsertPipelines(ctx, []pipelinedomain.Pipeline{
		{
			ID:           "pipeline-ci",
			Provider:     pipelinedomain.ProviderJenkins,
			JobFullName:  "folder/demo",
			JobName:      "demo",
			JobURL:       "https://jenkins.example.com/job/folder/job/demo/",
			Status:       pipelinedomain.StatusActive,
			LastSyncedAt: now,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}); err != nil {
		t.Fatalf("UpsertPipelines failed: %v", err)
	}

	if err := releaseRepo.CreateTemplate(
		ctx,
		releasedomain.ReleaseTemplate{
			ID:              "tpl-sync",
			Name:            "Template Sync",
			ApplicationID:   "app-1",
			ApplicationName: "App 1",
			BindingID:       "binding-ci",
			BindingName:     "CI",
			BindingType:     "ci",
			Status:          releasedomain.TemplateStatusActive,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		[]releasedomain.ReleaseTemplateBinding{
			{
				ID:            "rtb-ci",
				TemplateID:    "tpl-sync",
				PipelineScope: releasedomain.PipelineScopeCI,
				BindingID:     "binding-ci",
				BindingName:   "CI",
				Provider:      "jenkins",
				PipelineID:    "pipeline-ci",
				Enabled:       true,
				SortNo:        1,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
		},
		nil,
		nil,
		nil,
	); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	jenkins := &releaseTemplateSyncJenkinsFake{
		jobSets: map[string]executorparamdomain.JenkinsJobParamSet{
			"folder/demo": {
				JobName:     "demo",
				JobFullName: "folder/demo",
				Params: []executorparamdomain.JenkinsParamSnapshot{
					{
						Name:         "BRANCH",
						ParamType:    executorparamdomain.ParamTypeChoice,
						SingleSelect: true,
						Required:     true,
						DefaultValue: "origin/main",
						RawMeta:      `{"choices":["origin/main"]}`,
						SortNo:       1,
					},
				},
			},
		},
	}

	manager := usecase.NewReleaseTemplateManager(releaseRepo, nil, nil, nil, nil, nil, nil, nil, nil)
	handler := NewReleaseTemplateHandler(manager, releaseTemplateSyncAuthzFake{permissions: permissions})
	handler.SetSyncer(usecase.NewSyncTemplatePipelineParams(releaseRepo, pipelineRepo, paramRepo, jenkins))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		setCurrentUser(c, userdomain.User{ID: "usr-release", Role: userdomain.RoleNormal})
		c.Next()
	})
	handler.RegisterRoutes(router)

	return router, paramRepo, jenkins
}

type releaseTemplateSyncAuthzFake struct {
	permissions map[string]bool
}

func (a releaseTemplateSyncAuthzFake) HasPermission(
	_ context.Context,
	_ userdomain.User,
	permissionCode string,
	scopeType string,
	scopeValue string,
) (bool, error) {
	return a.permissions[releaseTemplateSyncPermissionKey(permissionCode, scopeType, scopeValue)], nil
}

func (a releaseTemplateSyncAuthzFake) ListEffectivePermissions(
	_ context.Context,
	_ userdomain.User,
) ([]userdomain.UserPermission, error) {
	return nil, nil
}

func releaseTemplateSyncPermissionKey(permissionCode string, scopeType string, scopeValue string) string {
	return permissionCode + "|" + scopeType + "|" + scopeValue
}

type releaseTemplateSyncJenkinsFake struct {
	jobSets map[string]executorparamdomain.JenkinsJobParamSet
	calls   int
}

func (j *releaseTemplateSyncJenkinsFake) GetJobParamSet(
	_ context.Context,
	fullName string,
) (executorparamdomain.JenkinsJobParamSet, error) {
	j.calls++
	if item, ok := j.jobSets[fullName]; ok {
		return item, nil
	}
	return executorparamdomain.JenkinsJobParamSet{}, pipelinedomain.ErrPipelineNotFound
}
