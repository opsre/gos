package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	applicationdomain "gos/internal/domain/application"
	gitcredentialdomain "gos/internal/domain/gitcredential"
	releasedomain "gos/internal/domain/release"
	userdomain "gos/internal/domain/user"
)

type gitCredentialHTTPRepository struct {
	items map[string]gitcredentialdomain.Credential
}

func newGitCredentialHTTPRepository() *gitCredentialHTTPRepository {
	return &gitCredentialHTTPRepository{items: make(map[string]gitcredentialdomain.Credential)}
}

func (r *gitCredentialHTTPRepository) InitSchema(context.Context) error { return nil }

func (r *gitCredentialHTTPRepository) Create(_ context.Context, item gitcredentialdomain.Credential) error {
	for _, existing := range r.items {
		if existing.Name == item.Name {
			return gitcredentialdomain.ErrNameDuplicated
		}
	}
	r.items[item.ID] = item
	return nil
}

func (r *gitCredentialHTTPRepository) GetByID(_ context.Context, id string) (gitcredentialdomain.Credential, error) {
	item, ok := r.items[id]
	if !ok {
		return gitcredentialdomain.Credential{}, gitcredentialdomain.ErrNotFound
	}
	return item, nil
}

func (r *gitCredentialHTTPRepository) List(_ context.Context, filter gitcredentialdomain.ListFilter) ([]gitcredentialdomain.Credential, int64, error) {
	items := make([]gitcredentialdomain.Credential, 0, len(r.items))
	for _, item := range r.items {
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		items = append(items, item)
	}
	return items, int64(len(items)), nil
}

func (r *gitCredentialHTTPRepository) Update(
	_ context.Context,
	id string,
	input gitcredentialdomain.UpdateInput,
	updatedAt time.Time,
) (gitcredentialdomain.Credential, error) {
	item, ok := r.items[id]
	if !ok {
		return gitcredentialdomain.Credential{}, gitcredentialdomain.ErrNotFound
	}
	item.Name = input.Name
	item.Provider = input.Provider
	item.BaseURL = input.BaseURL
	item.Username = input.Username
	item.Secret = input.Secret
	item.AuthType = input.AuthType
	item.Status = input.Status
	item.Remark = input.Remark
	item.UpdatedAt = updatedAt
	r.items[id] = item
	return item, nil
}

func (r *gitCredentialHTTPRepository) Delete(_ context.Context, id string) error {
	if _, ok := r.items[id]; !ok {
		return gitcredentialdomain.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

type gitCredentialHTTPAuthorizer struct {
	allowed   map[string]bool
	effective []userdomain.UserPermission
}

func (a gitCredentialHTTPAuthorizer) HasPermission(_ context.Context, _ userdomain.User, permissionCode string, _ string, _ string) (bool, error) {
	return a.allowed[permissionCode], nil
}

func (a gitCredentialHTTPAuthorizer) ListEffectivePermissions(context.Context, userdomain.User) ([]userdomain.UserPermission, error) {
	return a.effective, nil
}

func gitCredentialHTTPAllowAllAuthorizer() gitCredentialHTTPAuthorizer {
	return gitCredentialHTTPAuthorizer{allowed: map[string]bool{
		"component.credential.view":   true,
		"component.credential.manage": true,
	}}
}

func newGitCredentialHTTPRouter(t *testing.T, user userdomain.User, authorizer RequestAuthorizer, handler *GitCredentialHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		setCurrentUser(c, user)
		c.Next()
	})
	handler.RegisterRoutes(router)
	return router
}

func TestGitCredentialHandlerListNeverReturnsTheSecret(t *testing.T) {
	repo := newGitCredentialHTTPRepository()
	manager := usecase.NewGitCredentialManager(repo)
	created, err := manager.Create(context.Background(), usecase.GitCredentialInput{
		Name:     "内网 GitLab",
		BaseURL:  "http://git.cloud.local:9080",
		Username: "release-bot",
		Secret:   "glpat-plain-secret",
	})
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	handler := NewGitCredentialHandler(manager, nil, gitCredentialHTTPAllowAllAuthorizer())
	router := newGitCredentialHTTPRouter(t, userdomain.User{ID: "usr-1"}, gitCredentialHTTPAllowAllAuthorizer(), handler)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/git-credentials", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "glpat-plain-secret") {
		t.Fatalf("list response leaked the secret: %s", rec.Body.String())
	}

	var resp struct {
		Data  []map[string]any `json:"data"`
		Page  int              `json:"page"`
		Size  int              `json:"page_size"`
		Total int64            `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Total != 1 || len(resp.Data) != 1 {
		t.Fatalf("total = %d, data len = %d", resp.Total, len(resp.Data))
	}
	if _, exists := resp.Data[0]["secret"]; exists {
		t.Fatalf("response item carries a secret field: %+v", resp.Data[0])
	}
	if resp.Data[0]["secret_configured"] != true {
		t.Fatalf("secret_configured = %v, want true", resp.Data[0]["secret_configured"])
	}
	if resp.Data[0]["base_url"] != "http://git.cloud.local:9080" {
		t.Fatalf("base_url = %v", resp.Data[0]["base_url"])
	}

	single := httptest.NewRecorder()
	router.ServeHTTP(single, httptest.NewRequest(http.MethodGet, "/git-credentials/"+created.ID, nil))
	if single.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", single.Code, single.Body.String())
	}
	if strings.Contains(single.Body.String(), "glpat-plain-secret") {
		t.Fatalf("detail response leaked the secret: %s", single.Body.String())
	}

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/git-credentials/gc-missing", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, want 404; body = %s", missing.Code, missing.Body.String())
	}
}

func TestGitCredentialHandlerCreateUpdateAndDelete(t *testing.T) {
	repo := newGitCredentialHTTPRepository()
	manager := usecase.NewGitCredentialManager(repo)
	handler := NewGitCredentialHandler(manager, nil, gitCredentialHTTPAllowAllAuthorizer())
	router := newGitCredentialHTTPRouter(t, userdomain.User{ID: "usr-1"}, gitCredentialHTTPAllowAllAuthorizer(), handler)

	createBody := []byte(`{"name":"内网 GitLab","base_url":"http://git.cloud.local:9080","username":"release-bot","secret":"glpat-1","auth_type":"token"}`)
	createRec := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/git-credentials", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	// A blank secret keeps the stored one, so the update must not wipe it.
	updateBody := []byte(`{"name":"内网 GitLab","base_url":"http://git.cloud.local:9080","username":"release-bot","secret":"","remark":"编辑后"}`)
	updateRec := httptest.NewRecorder()
	updateReq := httptest.NewRequest(http.MethodPut, "/git-credentials/"+created.Data.ID, bytes.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updateRec.Code, updateRec.Body.String())
	}
	item, err := repo.GetByID(context.Background(), created.Data.ID)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if item.Secret != "glpat-1" {
		t.Fatalf("stored secret = %q, want the original one", item.Secret)
	}
	if item.Remark != "编辑后" {
		t.Fatalf("stored remark = %q", item.Remark)
	}

	duplicateRec := httptest.NewRecorder()
	duplicateReq := httptest.NewRequest(http.MethodPost, "/git-credentials", bytes.NewReader(createBody))
	duplicateReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(duplicateRec, duplicateReq)
	if duplicateRec.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409; body = %s", duplicateRec.Code, duplicateRec.Body.String())
	}

	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, httptest.NewRequest(http.MethodDelete, "/git-credentials/"+created.Data.ID, nil))
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleteRec.Code, deleteRec.Body.String())
	}
	deleteAgain := httptest.NewRecorder()
	router.ServeHTTP(deleteAgain, httptest.NewRequest(http.MethodDelete, "/git-credentials/"+created.Data.ID, nil))
	if deleteAgain.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want 404", deleteAgain.Code)
	}
}

func TestGitCredentialHandlerRequiresManagePermission(t *testing.T) {
	repo := newGitCredentialHTTPRepository()
	manager := usecase.NewGitCredentialManager(repo)
	handler := NewGitCredentialHandler(manager, nil, gitCredentialHTTPAuthorizer{})

	router := newGitCredentialHTTPRouter(t, userdomain.User{ID: "usr-1"}, gitCredentialHTTPAuthorizer{}, handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/git-credentials", bytes.NewReader([]byte(`{"name":"n","base_url":"http://git.cloud.local:9080","secret":"s"}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}

type gitCommitHTTPReleaseRepo struct {
	releasedomain.Repository
	orders map[string]releasedomain.ReleaseOrder
}

func (r *gitCommitHTTPReleaseRepo) GetByID(_ context.Context, id string) (releasedomain.ReleaseOrder, error) {
	order, ok := r.orders[id]
	if !ok {
		return releasedomain.ReleaseOrder{}, releasedomain.ErrOrderNotFound
	}
	return order, nil
}

type gitCommitHTTPApplicationRepo struct {
	applicationdomain.Repository
	applications map[string]applicationdomain.Application
}

func (r *gitCommitHTTPApplicationRepo) GetByID(_ context.Context, id string) (applicationdomain.Application, error) {
	application, ok := r.applications[id]
	if !ok {
		return applicationdomain.Application{}, applicationdomain.ErrNotFound
	}
	return application, nil
}

// newGitCommitHTTPRouter wires the recent-commits route over fakes. No credential
// is configured on purpose: every visible order resolves to the readable
// "no matching credential" error, so the test asserts the visibility filter
// without needing a GitLab double.
func newGitCommitHTTPRouter(t *testing.T, user userdomain.User, authorizer RequestAuthorizer) *gin.Engine {
	t.Helper()
	orderRepo := &gitCommitHTTPReleaseRepo{orders: map[string]releasedomain.ReleaseOrder{
		"ro-1": {ID: "ro-1", ApplicationID: "app-1", GitRef: "main"},
		"ro-2": {ID: "ro-2", ApplicationID: "app-2", GitRef: "main"},
	}}
	applicationRepo := &gitCommitHTTPApplicationRepo{applications: map[string]applicationdomain.Application{
		"app-1": {ID: "app-1", Name: "fusion-source-web", RepoURL: "http://git.cloud.local:9080/code/bigData/fusion-source-web.git"},
		"app-2": {ID: "app-2", Name: "fusion-source-api", RepoURL: "http://git.cloud.local:9080/code/bigData/fusion-source-api.git"},
	}}

	handler := NewGitCredentialHandler(
		usecase.NewGitCredentialManager(newGitCredentialHTTPRepository()),
		usecase.NewGitCommitManager(orderRepo, applicationRepo, newGitCredentialHTTPRepository()),
		authorizer,
	)
	return newGitCredentialHTTPRouter(t, user, authorizer, handler)
}

func gitCredentialHTTPScopedPermission() userdomain.UserPermission {
	return userdomain.UserPermission{
		PermissionCode: "release.view",
		ScopeType:      "application",
		ScopeValue:     "app-1",
		Enabled:        true,
	}
}

func requestRecentCommits(t *testing.T, router *gin.Engine) map[string]struct {
	ApplicationID string `json:"application_id"`
	Error         string `json:"error"`
	Commits       []any  `json:"commits"`
} {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/release-orders/recent-commits?order_ids=ro-1,ro-2&limit=5", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data map[string]struct {
			ApplicationID string `json:"application_id"`
			Error         string `json:"error"`
			Commits       []any  `json:"commits"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp.Data
}

func TestGitCredentialHandlerRecentCommitsFiltersInvisibleOrders(t *testing.T) {
	authorizer := gitCredentialHTTPAuthorizer{
		allowed:   map[string]bool{},
		effective: []userdomain.UserPermission{gitCredentialHTTPScopedPermission()},
	}
	router := newGitCommitHTTPRouter(t, userdomain.User{ID: "usr-1", Role: userdomain.RoleNormal}, authorizer)

	data := requestRecentCommits(t, router)
	if len(data) != 1 {
		t.Fatalf("data = %+v, want only the visible order", data)
	}
	entry, ok := data["ro-1"]
	if !ok {
		t.Fatalf("data = %+v, want ro-1", data)
	}
	if entry.ApplicationID != "app-1" {
		t.Fatalf("application_id = %q", entry.ApplicationID)
	}
	if !strings.Contains(entry.Error, "未找到匹配的 Git 凭证") {
		t.Fatalf("error = %q, want the missing credential hint", entry.Error)
	}
	if entry.Commits == nil {
		t.Fatalf("commits is null, want an empty array")
	}
}

func TestGitCredentialHandlerRecentCommitsAllowsAdministrators(t *testing.T) {
	authorizer := gitCredentialHTTPAllowAllAuthorizer()
	router := newGitCommitHTTPRouter(t, userdomain.User{ID: "usr-admin", Role: userdomain.RoleAdmin}, authorizer)

	data := requestRecentCommits(t, router)
	if len(data) != 2 {
		t.Fatalf("data = %+v, want both orders for an administrator", data)
	}
	for _, id := range []string{"ro-1", "ro-2"} {
		entry, ok := data[id]
		if !ok {
			t.Fatalf("data = %+v, want %s", data, id)
		}
		if !strings.Contains(entry.Error, "未找到匹配的 Git 凭证") {
			t.Fatalf("%s error = %q", id, entry.Error)
		}
	}
}

// TestGitCredentialHandlerRoutesCoexistWithReleaseOrderRoutes guards the one
// route that lives outside the credential prefix: /release-orders/recent-commits
// has to register alongside the release order handler's /release-orders/:id
// routes without a router conflict.
func TestGitCredentialHandlerRoutesCoexistWithReleaseOrderRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewGitCredentialHandler(
		usecase.NewGitCredentialManager(newGitCredentialHTTPRepository()),
		usecase.NewGitCommitManager(
			&gitCommitHTTPReleaseRepo{},
			&gitCommitHTTPApplicationRepo{},
			newGitCredentialHTTPRepository(),
		),
		gitCredentialHTTPAllowAllAuthorizer(),
	).RegisterRoutes(router)
	NewReleaseOrderHandler(
		usecase.NewReleaseOrderManager(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil),
		nil,
		gitCredentialHTTPAllowAllAuthorizer(),
		nil,
	).RegisterRoutes(router)

	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, want := range []string{
		http.MethodGet + " /release-orders/recent-commits",
		http.MethodGet + " /release-orders/:id",
		http.MethodGet + " /git-credentials",
		http.MethodPost + " /git-credentials/:id/test",
	} {
		if !registered[want] {
			t.Fatalf("route %q is not registered; got %+v", want, router.Routes())
		}
	}
}
