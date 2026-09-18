package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	appdomain "gos/internal/domain/application"
	releasedomain "gos/internal/domain/release"
	automationdomain "gos/internal/domain/releaseautomation"
	userdomain "gos/internal/domain/user"
)

const (
	releaseAutomationHTTPAppID      = "app-automation-http"
	releaseAutomationHTTPRepoURL    = "http://git.cloud.local:9080/code/bigData/fusion-source-web.git"
	releaseAutomationHTTPTemplateID = "rt-automation-http"
)

type releaseAutomationHTTPRepo struct {
	items map[string]automationdomain.Automation
	order []string
}

func newReleaseAutomationHTTPRepo() *releaseAutomationHTTPRepo {
	return &releaseAutomationHTTPRepo{items: make(map[string]automationdomain.Automation)}
}

func (r *releaseAutomationHTTPRepo) InitSchema(context.Context) error { return nil }

func (r *releaseAutomationHTTPRepo) Create(_ context.Context, item automationdomain.Automation) error {
	for _, existing := range r.items {
		if existing.ApplicationID == item.ApplicationID &&
			existing.EnvCode == item.EnvCode &&
			existing.GitRef == item.GitRef {
			return automationdomain.ErrDuplicated
		}
	}
	r.items[item.ID] = item
	r.order = append(r.order, item.ID)
	return nil
}

func (r *releaseAutomationHTTPRepo) GetByID(_ context.Context, id string) (automationdomain.Automation, error) {
	item, ok := r.items[id]
	if !ok {
		return automationdomain.Automation{}, automationdomain.ErrNotFound
	}
	return item, nil
}

func (r *releaseAutomationHTTPRepo) List(_ context.Context, _ automationdomain.ListFilter) ([]automationdomain.Automation, int64, error) {
	items := make([]automationdomain.Automation, 0, len(r.order))
	for _, id := range r.order {
		items = append(items, r.items[id])
	}
	return items, int64(len(items)), nil
}

func (r *releaseAutomationHTTPRepo) Update(_ context.Context, item automationdomain.Automation) error {
	if _, ok := r.items[item.ID]; !ok {
		return automationdomain.ErrNotFound
	}
	r.items[item.ID] = item
	return nil
}

func (r *releaseAutomationHTTPRepo) Delete(_ context.Context, id string) error {
	if _, ok := r.items[id]; !ok {
		return automationdomain.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *releaseAutomationHTTPRepo) ListEnabled(context.Context, int) ([]automationdomain.Automation, error) {
	return nil, nil
}

func (r *releaseAutomationHTTPRepo) ResetBaseline(
	_ context.Context,
	id string,
	applicationID string,
	envCode string,
	gitRef string,
	expectedSeenSHA string,
	seenSHA string,
	updatedAt time.Time,
) (bool, error) {
	item, ok := r.items[id]
	if !ok {
		return false, nil
	}
	if item.ApplicationID != applicationID || item.EnvCode != envCode || item.GitRef != gitRef {
		return false, nil
	}
	if item.LastSeenSHA != expectedSeenSHA {
		return false, nil
	}
	item.LastSeenSHA = seenSHA
	item.LastTriggeredSHA = ""
	item.LastOrderID = ""
	item.UpdatedAt = updatedAt
	r.items[id] = item
	return true, nil
}

func (r *releaseAutomationHTTPRepo) UpdateCheckState(context.Context, string, time.Time, string) error {
	return nil
}

func (r *releaseAutomationHTTPRepo) CommitTrigger(
	context.Context, string, string, string, string, string, time.Time, string,
) (bool, error) {
	return true, nil
}

type releaseAutomationHTTPAppStub struct{}

func (releaseAutomationHTTPAppStub) GetByID(context.Context, string) (appdomain.Application, error) {
	return appdomain.Application{
		ID:      releaseAutomationHTTPAppID,
		Name:    "fusion-source-web",
		Status:  appdomain.StatusActive,
		RepoURL: releaseAutomationHTTPRepoURL,
	}, nil
}

type releaseAutomationHTTPTemplateStub struct{}

func (releaseAutomationHTTPTemplateStub) GetTemplateByID(
	context.Context,
	string,
) (releasedomain.ReleaseTemplate, []releasedomain.ReleaseTemplateBinding, []releasedomain.ReleaseTemplateParam, []releasedomain.ReleaseTemplateGitOpsRule, []releasedomain.ReleaseTemplateHook, error) {
	return releasedomain.ReleaseTemplate{
		ID:            releaseAutomationHTTPTemplateID,
		Name:          "生产发布模板",
		ApplicationID: releaseAutomationHTTPAppID,
		Status:        releasedomain.TemplateStatusActive,
	}, nil, nil, nil, nil, nil
}

type releaseAutomationHTTPOrderStub struct{}

func (releaseAutomationHTTPOrderStub) Create(_ context.Context, input usecase.CreateReleaseOrderInput) (releasedomain.ReleaseOrder, error) {
	return releasedomain.ReleaseOrder{ID: "ro-1", OrderNo: "RO-1", TriggerType: input.TriggerType}, nil
}

func (releaseAutomationHTTPOrderStub) Build(context.Context, string, string, string) (releasedomain.ReleaseOrder, error) {
	return releasedomain.ReleaseOrder{}, nil
}

func (releaseAutomationHTTPOrderStub) Deploy(context.Context, string, string, string) (releasedomain.ReleaseOrder, error) {
	return releasedomain.ReleaseOrder{}, nil
}

func (releaseAutomationHTTPOrderStub) Execute(context.Context, string, string, string) (releasedomain.ReleaseOrder, error) {
	return releasedomain.ReleaseOrder{}, nil
}

func (releaseAutomationHTTPOrderStub) AutoDeployBuiltOrder(context.Context, string, string, string) (bool, error) {
	return false, nil
}

func (releaseAutomationHTTPOrderStub) FindActiveOrderByApplicationEnv(context.Context, string, string) (releasedomain.ReleaseOrder, error) {
	return releasedomain.ReleaseOrder{}, releasedomain.ErrOrderNotFound
}

func (releaseAutomationHTTPOrderStub) FindOpenOrderByApplicationEnv(context.Context, string, string, string) (releasedomain.ReleaseOrder, error) {
	return releasedomain.ReleaseOrder{}, releasedomain.ErrOrderNotFound
}

type releaseAutomationHTTPGitStub struct {
	head usecase.HeadCommit
	err  error
}

func (s releaseAutomationHTTPGitStub) ResolveHeadCommit(context.Context, string, string) (usecase.HeadCommit, error) {
	if s.err != nil {
		return usecase.HeadCommit{}, s.err
	}
	return s.head, nil
}

type releaseAutomationHTTPAuthorizer struct {
	allowed map[string]bool
}

func (a releaseAutomationHTTPAuthorizer) HasPermission(_ context.Context, _ userdomain.User, permissionCode string, _ string, _ string) (bool, error) {
	return a.allowed[permissionCode], nil
}

func (a releaseAutomationHTTPAuthorizer) ListEffectivePermissions(context.Context, userdomain.User) ([]userdomain.UserPermission, error) {
	return nil, nil
}

func releaseAutomationHTTPAllowAllAuthorizer() releaseAutomationHTTPAuthorizer {
	return releaseAutomationHTTPAuthorizer{allowed: map[string]bool{
		permissionReleaseAutomationView:   true,
		permissionReleaseAutomationManage: true,
	}}
}

func newReleaseAutomationHTTPRouter(
	t *testing.T,
	repo *releaseAutomationHTTPRepo,
	git releaseAutomationHTTPGitStub,
	authorizer RequestAuthorizer,
) (*gin.Engine, *usecase.ReleaseAutomationManager) {
	t.Helper()

	manager := usecase.NewReleaseAutomationManager(
		repo,
		releaseAutomationHTTPAppStub{},
		releaseAutomationHTTPTemplateStub{},
		releaseAutomationHTTPOrderStub{},
		git,
	)
	handler := NewReleaseAutomationHandler(manager, authorizer)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		setCurrentUser(c, userdomain.User{ID: "usr-1", Username: "zhangsan", DisplayName: "张三"})
		c.Next()
	})
	handler.RegisterRoutes(router)
	return router, manager
}

func releaseAutomationHTTPCreateBody(checkGit bool) []byte {
	body, _ := json.Marshal(map[string]any{
		"name":           "前端生产自动发布",
		"application_id": releaseAutomationHTTPAppID,
		"template_id":    releaseAutomationHTTPTemplateID,
		"env_code":       "prod",
		"git_ref":        "release/2026-09-18",
		"dispatch_mode":  "build_deploy",
		"params": []map[string]string{{
			"pipeline_scope":      "ci",
			"param_key":           "git_ref",
			"executor_param_name": "GIT_REF",
			"param_value":         "release/2026-09-18",
			"value_source":        "release_input",
		}},
		"check_git": checkGit,
	})
	return body
}

func releaseAutomationHTTPPost(t *testing.T, router *gin.Engine, method string, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// 列表只下发契约字段：内部列（配置创建者）与仓库地址都不出接口。
func TestReleaseAutomationHandlerListKeepsTheFrozenShape(t *testing.T) {
	repo := newReleaseAutomationHTTPRepo()
	router, _ := newReleaseAutomationHTTPRouter(t, repo, releaseAutomationHTTPGitStub{head: usecase.HeadCommit{CommitSHA: "sha-1"}}, releaseAutomationHTTPAllowAllAuthorizer())

	createRec := releaseAutomationHTTPPost(t, router, http.MethodPost, "/release-automations", releaseAutomationHTTPCreateBody(true))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/release-automations?page=1&page_size=20", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data     []map[string]any `json:"data"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int64            `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if resp.Total != 1 || len(resp.Data) != 1 {
		t.Fatalf("total=%d len=%d, want 1", resp.Total, len(resp.Data))
	}
	item := resp.Data[0]
	allowed := []string{
		"id", "name", "application_id", "application_name", "template_id", "template_name",
		"env_code", "git_ref", "dispatch_mode", "enabled", "params", "last_seen_sha",
		"last_triggered_sha", "last_order_id", "last_checked_at", "last_error", "created_at", "updated_at",
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	extra := make([]string, 0)
	for key := range item {
		if _, ok := allowedSet[key]; !ok {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	if len(extra) != 0 {
		t.Fatalf("list item carries fields outside the frozen contract: %v", extra)
	}
	if _, exists := item["creator_user_id"]; exists {
		t.Fatalf("list item leaked the internal creator column: %+v", item)
	}
	if item["last_seen_sha"] != "sha-1" {
		t.Fatalf("last_seen_sha = %v, want the baseline sha-1", item["last_seen_sha"])
	}
	if item["enabled"] != true {
		t.Fatalf("enabled = %v, want true", item["enabled"])
	}
	params, ok := item["params"].([]any)
	if !ok || len(params) != 1 {
		t.Fatalf("params = %v", item["params"])
	}
	first, _ := params[0].(map[string]any)
	if first["executor_param_name"] != "GIT_REF" || first["value_source"] != "release_input" {
		t.Fatalf("param = %+v", first)
	}
}

// 保存前的 git 校验失败：400 + 可读原因，且配置不落库。
func TestReleaseAutomationHandlerCreateRejectsWhenGitCheckFails(t *testing.T) {
	repo := newReleaseAutomationHTTPRepo()
	authorizer := releaseAutomationHTTPAllowAllAuthorizer()
	router, manager := newReleaseAutomationHTTPRouter(t, repo, releaseAutomationHTTPGitStub{err: errors.New("Git 凭证被拒绝或已过期")}, authorizer)

	rec := releaseAutomationHTTPPost(t, router, http.MethodPost, "/release-automations", releaseAutomationHTTPCreateBody(true))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp.Error != "Git 凭证被拒绝或已过期" {
		t.Fatalf("error = %q, want the readable git reason", resp.Error)
	}
	items, total, err := manager.List(context.Background(), automationdomain.ListFilter{})
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("saved %d configs, want 0 after a failed git check", total)
	}
}

// check_git=false 时不读 git，配置直接用未验证的基线落库。
func TestReleaseAutomationHandlerCreateSkipsGitWhenCheckDisabled(t *testing.T) {
	repo := newReleaseAutomationHTTPRepo()
	router, _ := newReleaseAutomationHTTPRouter(t, repo, releaseAutomationHTTPGitStub{err: errors.New("Git 凭证被拒绝或已过期")}, releaseAutomationHTTPAllowAllAuthorizer())

	rec := releaseAutomationHTTPPost(t, router, http.MethodPost, "/release-automations", releaseAutomationHTTPCreateBody(false))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
}

func TestReleaseAutomationHandlerCheckReportsReachability(t *testing.T) {
	repo := newReleaseAutomationHTTPRepo()
	git := releaseAutomationHTTPGitStub{head: usecase.HeadCommit{CommitSHA: "sha-1"}}
	router, _ := newReleaseAutomationHTTPRouter(t, repo, git, releaseAutomationHTTPAllowAllAuthorizer())

	createRec := releaseAutomationHTTPPost(t, router, http.MethodPost, "/release-automations", releaseAutomationHTTPCreateBody(true))
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/release-automations/"+created.Data.ID+"/check", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("check status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Reachable bool   `json:"reachable"`
			HeadSHA   string `json:"head_sha"`
			Message   string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode check: %v", err)
	}
	if !resp.Data.Reachable || resp.Data.HeadSHA != "sha-1" {
		t.Fatalf("check data = %+v, want reachable sha-1", resp.Data)
	}

	// 未找到配置：404，而不是 500。
	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/release-automations/rauto-missing/check", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, want 404; body = %s", missing.Code, missing.Body.String())
	}
}

func TestReleaseAutomationHandlerPermissions(t *testing.T) {
	cases := []struct {
		name    string
		allowed map[string]bool
		method  string
		path    string
		body    []byte
		want    int
	}{
		{
			name:    "list requires view",
			allowed: map[string]bool{permissionReleaseAutomationManage: true},
			method:  http.MethodGet,
			path:    "/release-automations",
			want:    http.StatusForbidden,
		},
		{
			name:    "list allowed with view",
			allowed: map[string]bool{permissionReleaseAutomationView: true},
			method:  http.MethodGet,
			path:    "/release-automations",
			want:    http.StatusOK,
		},
		{
			name:    "create requires manage",
			allowed: map[string]bool{permissionReleaseAutomationView: true},
			method:  http.MethodPost,
			path:    "/release-automations",
			body:    releaseAutomationHTTPCreateBody(true),
			want:    http.StatusForbidden,
		},
		{
			name:    "create allowed with manage",
			allowed: map[string]bool{permissionReleaseAutomationManage: true},
			method:  http.MethodPost,
			path:    "/release-automations",
			body:    releaseAutomationHTTPCreateBody(true),
			want:    http.StatusCreated,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newReleaseAutomationHTTPRepo()
			router, _ := newReleaseAutomationHTTPRouter(
				t,
				repo,
				releaseAutomationHTTPGitStub{head: usecase.HeadCommit{CommitSHA: "sha-1"}},
				releaseAutomationHTTPAuthorizer{allowed: tc.allowed},
			)
			rec := releaseAutomationHTTPPost(t, router, tc.method, tc.path, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestReleaseAutomationHandlerUpdateResetBaselineAndDelete(t *testing.T) {
	repo := newReleaseAutomationHTTPRepo()
	git := releaseAutomationHTTPGitStub{head: usecase.HeadCommit{CommitSHA: "sha-1"}}
	router, _ := newReleaseAutomationHTTPRouter(t, repo, git, releaseAutomationHTTPAllowAllAuthorizer())

	createRec := releaseAutomationHTTPPost(t, router, http.MethodPost, "/release-automations", releaseAutomationHTTPCreateBody(true))
	var created struct {
		Data struct {
			ID          string `json:"id"`
			LastSeenSHA string `json:"last_seen_sha"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Data.LastSeenSHA != "sha-1" {
		t.Fatalf("last_seen_sha = %q, want sha-1", created.Data.LastSeenSHA)
	}

	// 换分支后基线必须重设为新分支的 HEAD，旧 sha 不能被当成新提交。
	updateBody, _ := json.Marshal(map[string]any{
		"name":           "前端生产自动发布",
		"application_id": releaseAutomationHTTPAppID,
		"template_id":    releaseAutomationHTTPTemplateID,
		"env_code":       "prod",
		"git_ref":        "release/2026-09-19",
		"dispatch_mode":  "build",
	})
	updateRec := releaseAutomationHTTPPost(t, router, http.MethodPut, "/release-automations/"+created.Data.ID, updateBody)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updateRec.Code, updateRec.Body.String())
	}
	var updated struct {
		Data struct {
			LastSeenSHA string `json:"last_seen_sha"`
			GitRef      string `json:"git_ref"`
		} `json:"data"`
	}
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if updated.Data.GitRef != "release/2026-09-19" || updated.Data.LastSeenSHA != "sha-1" {
		t.Fatalf("updated = %+v, want the new branch with a fresh baseline", updated.Data)
	}

	deleteRec := releaseAutomationHTTPPost(t, router, http.MethodDelete, "/release-automations/"+created.Data.ID, nil)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleteRec.Code, deleteRec.Body.String())
	}
	missing := releaseAutomationHTTPPost(t, router, http.MethodDelete, "/release-automations/"+created.Data.ID, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want 404", missing.Code)
	}
}
