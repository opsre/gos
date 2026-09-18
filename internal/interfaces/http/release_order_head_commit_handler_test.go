package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	domain "gos/internal/domain/release"
	userdomain "gos/internal/domain/user"
	"gos/internal/infrastructure/persistence/sqlrepo"

	_ "modernc.org/sqlite"
)

const (
	headCommitHTTPOrderID    = "ro-head-commit-http"
	headCommitHTTPLegacyID   = "ro-head-commit-http-legacy"
	headCommitHTTPChangeTime = "2026-09-18T08:30:00Z"
)

// headCommitHTTPReleaseOrderResponse 是响应里 HEAD 快照字段的断言用子集。
type headCommitHTTPReleaseOrderResponse struct {
	HeadCommitSHA    string `json:"head_commit_sha"`
	HeadCommitRef    string `json:"head_commit_ref"`
	HeadChangeSHA    string `json:"head_change_sha"`
	HeadChangeTitle  string `json:"head_change_title"`
	HeadChangeAuthor string `json:"head_change_author"`
	HeadChangeAt     string `json:"head_change_at"`
	HeadChangeURL    string `json:"head_change_url"`
}

func newHeadCommitHTTPTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
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

	now := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)
	order := releaseOrderHandlerTestOrder(headCommitHTTPOrderID, "RO-HEAD-COMMIT-HTTP", now)
	if err := repo.Create(context.Background(), order, nil, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}
	changeAt := time.Date(2026, 9, 18, 8, 30, 0, 0, time.UTC)
	if err := repo.UpdateHeadCommit(context.Background(), order.ID, domain.ReleaseOrderHeadCommit{
		CommitSHA:    "sha-head-http",
		CommitRef:    "release/2026-09-18",
		ChangeSHA:    "sha-change-http",
		ChangeTitle:  "feat: HTTP 响应带出 HEAD 快照",
		ChangeAuthor: "Alice",
		ChangeAt:     &changeAt,
		ChangeURL:    "http://git.cloud.local:9080/code/bigData/fusion-source-web/-/commit/sha-change-http",
	}); err != nil {
		t.Fatalf("UpdateHeadCommit failed: %v", err)
	}

	// 历史发布单：创建时没有解析过 HEAD，列保持空值。
	legacy := releaseOrderHandlerTestOrder(headCommitHTTPLegacyID, "RO-HEAD-COMMIT-HTTP-LEGACY", now.Add(time.Second))
	if err := repo.Create(context.Background(), legacy, nil, nil, nil); err != nil {
		t.Fatalf("Create legacy release order failed: %v", err)
	}

	manager := usecase.NewReleaseOrderManager(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		setCurrentUser(c, userdomain.User{ID: "usr-1"})
		c.Next()
	})
	NewReleaseOrderHandler(manager, nil, releaseOrderHandlerAllowAllAuthorizer{}, nil).RegisterRoutes(router)
	return router
}

// TestReleaseOrderHandlerDetailExposesHeadCommitFields 覆盖详情响应：7 个新字段都在，
// head_change_at 按 RFC3339 输出，历史发布单为 null 而不是零值时间。
func TestReleaseOrderHandlerDetailExposesHeadCommitFields(t *testing.T) {
	router := newHeadCommitHTTPTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/release-orders/"+headCommitHTTPOrderID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data headCommitHTTPReleaseOrderResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.HeadCommitSHA != "sha-head-http" || resp.Data.HeadCommitRef != "release/2026-09-18" {
		t.Fatalf("head_commit = %q / %q; body=%s", resp.Data.HeadCommitSHA, resp.Data.HeadCommitRef, rec.Body.String())
	}
	if resp.Data.HeadChangeSHA != "sha-change-http" ||
		resp.Data.HeadChangeTitle != "feat: HTTP 响应带出 HEAD 快照" ||
		resp.Data.HeadChangeAuthor != "Alice" {
		t.Fatalf("head_change = %#v; body=%s", resp.Data, rec.Body.String())
	}
	if resp.Data.HeadChangeAt != headCommitHTTPChangeTime {
		t.Fatalf("head_change_at = %q, want %q", resp.Data.HeadChangeAt, headCommitHTTPChangeTime)
	}
	if resp.Data.HeadChangeURL != "http://git.cloud.local:9080/code/bigData/fusion-source-web/-/commit/sha-change-http" {
		t.Fatalf("head_change_url = %q", resp.Data.HeadChangeURL)
	}

	legacyReq := httptest.NewRequest(http.MethodGet, "/release-orders/"+headCommitHTTPLegacyID, nil)
	legacyRec := httptest.NewRecorder()
	router.ServeHTTP(legacyRec, legacyReq)
	if legacyRec.Code != http.StatusOK {
		t.Fatalf("legacy status = %d, body = %s", legacyRec.Code, legacyRec.Body.String())
	}
	if !strings.Contains(legacyRec.Body.String(), `"head_change_at":null`) {
		t.Fatalf("legacy order must report a null head_change_at: %s", legacyRec.Body.String())
	}
	var legacyResp struct {
		Data headCommitHTTPReleaseOrderResponse `json:"data"`
	}
	if err := json.Unmarshal(legacyRec.Body.Bytes(), &legacyResp); err != nil {
		t.Fatalf("decode legacy response: %v", err)
	}
	if legacyResp.Data.HeadCommitSHA != "" || legacyResp.Data.HeadChangeSHA != "" || legacyResp.Data.HeadChangeURL != "" {
		t.Fatalf("legacy order head commit fields = %#v, want empty", legacyResp.Data)
	}
}

// TestReleaseOrderHandlerListExposesHeadCommitFields 覆盖列表响应：列表页同样只读库字段。
func TestReleaseOrderHandlerListExposesHeadCommitFields(t *testing.T) {
	router := newHeadCommitHTTPTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/release-orders?page=1&page_size=10", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []headCommitHTTPReleaseOrderResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("list length = %d, want 2; body=%s", len(resp.Data), rec.Body.String())
	}
	var found bool
	for _, item := range resp.Data {
		if item.HeadCommitSHA != "sha-head-http" {
			continue
		}
		found = true
		if item.HeadChangeSHA != "sha-change-http" || item.HeadChangeAt != headCommitHTTPChangeTime {
			t.Fatalf("list head commit fields = %#v", item)
		}
	}
	if !found {
		t.Fatalf("list response lost the head commit columns: %s", rec.Body.String())
	}
}

// TestReleaseOrderRealtimeSnapshotExposesHeadCommitFields 覆盖 realtime 快照：
// 它复用详情/列表的同一套 toReleaseOrderResponse 映射。
func TestReleaseOrderRealtimeSnapshotExposesHeadCommitFields(t *testing.T) {
	t.Parallel()

	handler, router := newReleaseOrderRealtimeHTTPTestServer(t, userdomain.User{ID: "usr-1"}, realtimePermissionAuthorizer{})
	handler.realtime = newReleaseOrderRealtimeCoordinator(handler.loadReleaseOrderRealtimeSnapshot, nil)

	req := httptest.NewRequest(http.MethodGet, "/release-orders/ro-realtime-http/realtime-snapshot", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Order headCommitHTTPReleaseOrderResponse `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Order.HeadCommitSHA != "sha-realtime-head" || resp.Data.Order.HeadChangeSHA != "sha-realtime-change" {
		t.Fatalf("realtime order head commit fields = %#v; body=%s", resp.Data.Order, rec.Body.String())
	}
	if resp.Data.Order.HeadChangeAt != headCommitHTTPChangeTime {
		t.Fatalf("realtime head_change_at = %q, want %q", resp.Data.Order.HeadChangeAt, headCommitHTTPChangeTime)
	}
}
