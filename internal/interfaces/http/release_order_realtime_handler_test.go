package httpapi

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	domain "gos/internal/domain/release"
	userdomain "gos/internal/domain/user"
	"gos/internal/infrastructure/persistence/sqlrepo"

	_ "modernc.org/sqlite"
)

type releaseOrderRealtimeSyncFake struct {
	calls      *atomic.Int32
	state      *atomic.Int32
	changeOnce *sync.Once
}

type dynamicRealtimePermissionAuthorizer struct {
	valueProgress atomic.Bool
}

func (a *dynamicRealtimePermissionAuthorizer) HasPermission(
	_ context.Context,
	_ userdomain.User,
	permissionCode string,
	_ string,
	_ string,
) (bool, error) {
	if permissionCode == "release.param_snapshot.view" {
		return a.valueProgress.Load(), nil
	}
	return true, nil
}

func (*dynamicRealtimePermissionAuthorizer) ListEffectivePermissions(context.Context, userdomain.User) ([]userdomain.UserPermission, error) {
	return nil, nil
}

func (f releaseOrderRealtimeSyncFake) SyncOrder(context.Context, string) error {
	f.calls.Add(1)
	if f.changeOnce != nil {
		f.changeOnce.Do(func() { f.state.Add(1) })
	} else {
		f.state.Add(1)
	}
	return nil
}

func TestReleaseOrderRealtimeCoordinatorSharesWatcherAndStopsAfterGrace(t *testing.T) {
	t.Parallel()

	var loads atomic.Int32
	var syncCalls atomic.Int32
	var state atomic.Int32
	coordinator := newReleaseOrderRealtimeCoordinator(
		func(context.Context, string) (ReleaseOrderRealtimeSnapshotPayload, error) {
			loads.Add(1)
			return ReleaseOrderRealtimeSnapshotPayload{
				Order:            ReleaseOrderResponse{ID: "ro-realtime", Status: string(rune('0' + state.Load()))},
				Executions:       []ReleaseOrderExecutionResponse{},
				Steps:            []ReleaseOrderStepResponse{},
				ValueProgress:    []ReleaseOrderValueProgressResponse{},
				ArtifactMetadata: []ReleaseOrderArtifactMetadataResponse{},
				ApprovalRecords:  []ReleaseOrderApprovalRecordResponse{},
			}, nil
		},
		releaseOrderRealtimeSyncFake{calls: &syncCalls, state: &state, changeOnce: &sync.Once{}},
	)
	coordinator.interval = 10 * time.Millisecond
	coordinator.idleGrace = 30 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	events1, unsubscribe1, err := coordinator.subscribe(ctx, "ro-realtime")
	if err != nil {
		t.Fatalf("first subscribe failed: %v", err)
	}
	events2, unsubscribe2, err := coordinator.subscribe(ctx, "ro-realtime")
	if err != nil {
		unsubscribe1()
		t.Fatalf("second subscribe failed: %v", err)
	}

	initial1 := receiveRealtimeSnapshot(t, events1)
	initial2 := receiveRealtimeSnapshot(t, events2)
	if initial1.Version == "" || initial1.Version != initial2.Version {
		t.Fatalf("initial versions = %q and %q", initial1.Version, initial2.Version)
	}
	coordinator.mu.Lock()
	watcherCount := len(coordinator.watchers)
	subscriberCount := len(coordinator.watchers["ro-realtime"].subscribers)
	coordinator.mu.Unlock()
	if watcherCount != 1 || subscriberCount != 2 {
		t.Fatalf("watchers=%d subscribers=%d, want 1 and 2", watcherCount, subscriberCount)
	}
	if loads.Load() != 1 {
		t.Fatalf("initial loads=%d, want 1 shared load", loads.Load())
	}

	updated1 := receiveRealtimeSnapshot(t, events1)
	updated2 := receiveRealtimeSnapshot(t, events2)
	if updated1.Version == initial1.Version || updated1.Version != updated2.Version {
		t.Fatalf("updated versions = %q and %q, initial=%q", updated1.Version, updated2.Version, initial1.Version)
	}
	if syncCalls.Load() == 0 {
		t.Fatal("fast synchronizer was not called")
	}

	unsubscribe1()
	coordinator.mu.Lock()
	subscriberCount = len(coordinator.watchers["ro-realtime"].subscribers)
	coordinator.mu.Unlock()
	if subscriberCount != 1 {
		t.Fatalf("subscribers after first unsubscribe=%d, want 1", subscriberCount)
	}
	unsubscribe2()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		coordinator.mu.Lock()
		watcherCount = len(coordinator.watchers)
		coordinator.mu.Unlock()
		if watcherCount == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("watcher did not stop after idle grace")
}

func TestReleaseOrderRealtimeVersionIgnoresGeneratedAt(t *testing.T) {
	t.Parallel()

	coordinator := newReleaseOrderRealtimeCoordinator(
		func(context.Context, string) (ReleaseOrderRealtimeSnapshotPayload, error) {
			return ReleaseOrderRealtimeSnapshotPayload{
				Order:            ReleaseOrderResponse{ID: "ro-stable", Status: "running"},
				Executions:       []ReleaseOrderExecutionResponse{},
				Steps:            []ReleaseOrderStepResponse{},
				ValueProgress:    []ReleaseOrderValueProgressResponse{},
				ArtifactMetadata: []ReleaseOrderArtifactMetadataResponse{},
				ApprovalRecords:  []ReleaseOrderApprovalRecordResponse{},
			}, nil
		},
		nil,
	)
	var tick atomic.Int64
	coordinator.now = func() time.Time {
		return time.Unix(tick.Add(1), 0).UTC()
	}

	first, err := coordinator.loadSnapshot(context.Background(), "ro-stable")
	if err != nil {
		t.Fatalf("first loadSnapshot failed: %v", err)
	}
	second, err := coordinator.loadSnapshot(context.Background(), "ro-stable")
	if err != nil {
		t.Fatalf("second loadSnapshot failed: %v", err)
	}
	if first.Version != second.Version {
		t.Fatalf("version changed without business change: %q != %q", first.Version, second.Version)
	}
	if first.GeneratedAt.Equal(second.GeneratedAt) {
		t.Fatal("generated_at did not change; test does not cover volatile metadata")
	}

	encoded, err := json.Marshal(ReleaseOrderRealtimeSnapshotData{
		Version:                             first.Version,
		GeneratedAt:                         first.GeneratedAt,
		ValueProgressVisible:                true,
		ReleaseOrderRealtimeSnapshotPayload: first.Payload,
	})
	if err != nil {
		t.Fatalf("marshal client snapshot failed: %v", err)
	}
	if len(encoded) == 0 {
		t.Fatal("client snapshot encoded empty")
	}
}

func TestReleaseOrderRealtimeVisibleVersionTracksPermissionRepresentation(t *testing.T) {
	t.Parallel()

	authorizer := &dynamicRealtimePermissionAuthorizer{}
	handler := &ReleaseOrderHandler{authz: authorizer}
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/release-orders/ro-visible/events", nil)
	setCurrentUser(ginContext, userdomain.User{ID: "usr-visible"})
	snapshot := releaseOrderRealtimeSnapshot{
		Version:     "internal-version-must-not-leak",
		GeneratedAt: time.Now().UTC(),
		Payload:     releaseOrderRealtimeTestPayload(),
	}

	redacted, err := handler.realtimeSnapshotForCurrentUser(ginContext, snapshot)
	if err != nil {
		t.Fatalf("redacted snapshot failed: %v", err)
	}
	if redacted.ValueProgressVisible || len(redacted.ValueProgress) != 0 {
		t.Fatalf("redacted snapshot leaked value progress: %#v", redacted)
	}
	if redacted.Version == snapshot.Version || len(redacted.Version) != 64 {
		t.Fatalf("redacted visible version=%q", redacted.Version)
	}

	authorizer.valueProgress.Store(true)
	visible, err := handler.realtimeSnapshotForCurrentUser(ginContext, snapshot)
	if err != nil {
		t.Fatalf("visible snapshot failed: %v", err)
	}
	if !visible.ValueProgressVisible || len(visible.ValueProgress) != 1 {
		t.Fatalf("visible snapshot missing value progress: %#v", visible)
	}
	if visible.Version == redacted.Version {
		t.Fatalf("permission change kept same visible version=%q", visible.Version)
	}

	authorizer.valueProgress.Store(false)
	redactedAgain, err := handler.realtimeSnapshotForCurrentUser(ginContext, snapshot)
	if err != nil {
		t.Fatalf("second redacted snapshot failed: %v", err)
	}
	if redactedAgain.Version != redacted.Version {
		t.Fatalf("same redacted representation versions differ: %q != %q", redactedAgain.Version, redacted.Version)
	}
}

func TestReleaseOrderRealtimeWatcherClosesAfterConsecutiveLoadFailures(t *testing.T) {
	t.Parallel()

	var loads atomic.Int32
	coordinator := newReleaseOrderRealtimeCoordinator(
		func(context.Context, string) (ReleaseOrderRealtimeSnapshotPayload, error) {
			if loads.Add(1) == 1 {
				return releaseOrderRealtimeTestPayload(), nil
			}
			return ReleaseOrderRealtimeSnapshotPayload{}, errors.New("temporary snapshot failure")
		},
		nil,
	)
	coordinator.interval = 5 * time.Millisecond
	coordinator.idleGrace = time.Second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	events, unsubscribe, err := coordinator.subscribe(ctx, "ro-load-failure")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer unsubscribe()
	_ = receiveRealtimeSnapshot(t, events)

	select {
	case _, open := <-events:
		if open {
			t.Fatal("received unexpected snapshot instead of closed channel")
		}
	case <-ctx.Done():
		t.Fatal("watcher did not close channel after consecutive failures")
	}
	if loads.Load() < releaseOrderRealtimeMaxLoadFailures+1 {
		t.Fatalf("loads=%d, want initial success plus %d failures", loads.Load(), releaseOrderRealtimeMaxLoadFailures)
	}
	coordinator.mu.Lock()
	watcherCount := len(coordinator.watchers)
	coordinator.mu.Unlock()
	if watcherCount != 0 {
		t.Fatalf("watchers=%d after failure shutdown, want 0", watcherCount)
	}
}

func TestReleaseOrderRealtimeSnapshotRefreshesWatcherCache(t *testing.T) {
	t.Parallel()

	var state atomic.Int32
	state.Store(1)
	coordinator := newReleaseOrderRealtimeCoordinator(
		func(context.Context, string) (ReleaseOrderRealtimeSnapshotPayload, error) {
			payload := releaseOrderRealtimeTestPayload()
			payload.Order.Status = "state-" + string(rune('0'+state.Load()))
			return payload, nil
		},
		nil,
	)
	coordinator.interval = time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	events1, unsubscribe1, err := coordinator.subscribe(ctx, "ro-cache-refresh")
	if err != nil {
		t.Fatalf("first subscribe failed: %v", err)
	}
	defer unsubscribe1()
	initial := receiveRealtimeSnapshot(t, events1)
	if initial.Payload.Order.Status != "state-1" {
		t.Fatalf("initial status=%q", initial.Payload.Order.Status)
	}

	state.Store(2)
	fresh, err := coordinator.snapshot(ctx, "ro-cache-refresh")
	if err != nil {
		t.Fatalf("fresh snapshot failed: %v", err)
	}
	if fresh.Payload.Order.Status != "state-2" {
		t.Fatalf("fresh status=%q, want state-2", fresh.Payload.Order.Status)
	}
	broadcast := receiveRealtimeSnapshot(t, events1)
	if broadcast.Version != fresh.Version || broadcast.Payload.Order.Status != "state-2" {
		t.Fatalf("broadcast=%#v, want fresh snapshot %#v", broadcast, fresh)
	}

	events2, unsubscribe2, err := coordinator.subscribe(ctx, "ro-cache-refresh")
	if err != nil {
		t.Fatalf("second subscribe failed: %v", err)
	}
	defer unsubscribe2()
	secondInitial := receiveRealtimeSnapshot(t, events2)
	if secondInitial.Version != fresh.Version || secondInitial.Payload.Order.Status != "state-2" {
		t.Fatalf("second initial=%#v, want fresh snapshot %#v", secondInitial, fresh)
	}
}

func TestReleaseOrderRealtimeUnchangedRefreshDoesNotBroadcast(t *testing.T) {
	t.Parallel()

	coordinator := newReleaseOrderRealtimeCoordinator(
		func(context.Context, string) (ReleaseOrderRealtimeSnapshotPayload, error) {
			return releaseOrderRealtimeTestPayload(), nil
		},
		nil,
	)
	coordinator.interval = time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	events, unsubscribe, err := coordinator.subscribe(ctx, "ro-unchanged")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer unsubscribe()
	initial := receiveRealtimeSnapshot(t, events)

	coordinator.mu.Lock()
	watcher := coordinator.watchers["ro-unchanged"]
	coordinator.mu.Unlock()
	refreshed, err := coordinator.refreshWatcher(ctx, watcher, false, false)
	if err != nil {
		t.Fatalf("refreshWatcher failed: %v", err)
	}
	if refreshed.Version != initial.Version {
		t.Fatalf("unchanged refresh version=%q, initial=%q", refreshed.Version, initial.Version)
	}
	select {
	case event := <-events:
		t.Fatalf("unchanged refresh unexpectedly broadcast %#v", event)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestReleaseOrderRealtimeHTTPFallbackSynchronizesBeforeLoad(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	var state atomic.Int32
	coordinator := newReleaseOrderRealtimeCoordinator(
		func(context.Context, string) (ReleaseOrderRealtimeSnapshotPayload, error) {
			payload := releaseOrderRealtimeTestPayload()
			payload.Order.Status = "state-" + string(rune('0'+state.Load()))
			return payload, nil
		},
		releaseOrderRealtimeSyncFake{calls: &calls, state: &state},
	)
	snapshot, err := coordinator.snapshot(context.Background(), "ro-http-fallback")
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if calls.Load() != 1 || snapshot.Payload.Order.Status != "state-1" {
		t.Fatalf("sync calls=%d status=%q, want 1 and state-1", calls.Load(), snapshot.Payload.Order.Status)
	}
}

func TestReleaseOrderRealtimeSnapshotReusesVisibilityAndRedactsValueProgress(t *testing.T) {
	t.Parallel()

	handler, router := newReleaseOrderRealtimeHTTPTestServer(t, userdomain.User{ID: "usr-1"}, realtimePermissionAuthorizer{})
	handler.realtime = newReleaseOrderRealtimeCoordinator(func(context.Context, string) (ReleaseOrderRealtimeSnapshotPayload, error) {
		return releaseOrderRealtimeTestPayload(), nil
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/release-orders/ro-realtime-http/realtime-snapshot", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"concurrent_batch_progress":null`) {
		t.Fatalf("non-concurrent progress is not encoded as JSON null: %s", rec.Body.String())
	}
	var response ReleaseOrderRealtimeSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if len(response.Data.Version) != 64 {
		t.Fatalf("version=%q, want sha256 hex", response.Data.Version)
	}
	if response.Data.ValueProgressVisible {
		t.Fatal("value_progress_visible=true, want false")
	}
	if len(response.Data.ValueProgress) != 0 {
		t.Fatalf("value_progress=%#v, want redacted empty list", response.Data.ValueProgress)
	}
	if response.Data.ConcurrentBatchProgress != nil {
		t.Fatalf("concurrent_batch_progress=%#v, want null for non-concurrent order", response.Data.ConcurrentBatchProgress)
	}

	_, deniedRouter := newReleaseOrderRealtimeHTTPTestServer(t, userdomain.User{ID: "usr-denied"}, realtimePermissionAuthorizer{})
	deniedReq := httptest.NewRequest(http.MethodGet, "/release-orders/ro-realtime-http/realtime-snapshot", nil)
	deniedRec := httptest.NewRecorder()
	deniedRouter.ServeHTTP(deniedRec, deniedReq)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s, want 403", deniedRec.Code, deniedRec.Body.String())
	}
}

func TestReleaseOrderRealtimeSSEStartsWithSnapshotEvent(t *testing.T) {
	t.Parallel()

	handler, router := newReleaseOrderRealtimeHTTPTestServer(t, userdomain.User{ID: "usr-1"}, realtimePermissionAuthorizer{allowValueProgress: true})
	handler.realtime = newReleaseOrderRealtimeCoordinator(func(context.Context, string) (ReleaseOrderRealtimeSnapshotPayload, error) {
		return releaseOrderRealtimeTestPayload(), nil
	}, nil)
	handler.realtime.interval = time.Hour
	handler.realtime.idleGrace = 10 * time.Millisecond

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/release-orders/ro-realtime-http/events", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext failed: %v", err)
	}
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", response.StatusCode)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("Content-Type=%q", contentType)
	}

	reader := bufio.NewReader(response.Body)
	lines := make([]string, 0, 3)
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatalf("read SSE frame failed: %v", readErr)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		lines = append(lines, line)
	}
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "id: ") || lines[1] != "event: snapshot" || !strings.HasPrefix(lines[2], "data: ") {
		t.Fatalf("unexpected SSE frame: %#v", lines)
	}
	if len(strings.TrimPrefix(lines[0], "id: ")) != 64 {
		t.Fatalf("SSE id=%q, want sha256 hex", lines[0])
	}
	var data ReleaseOrderRealtimeSnapshotData
	if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[2], "data: ")), &data); err != nil {
		t.Fatalf("decode SSE data failed: %v", err)
	}
	if data.Order.ID != "ro-realtime-http" || !data.ValueProgressVisible || len(data.ValueProgress) != 1 {
		t.Fatalf("unexpected SSE snapshot data: %#v", data)
	}
	cancel()
}

func TestReleaseOrderRealtimeHeartbeatPushesRedactedSnapshotAfterPermissionDowngrade(t *testing.T) {
	t.Parallel()

	authorizer := &dynamicRealtimePermissionAuthorizer{}
	authorizer.valueProgress.Store(true)
	handler, router := newReleaseOrderRealtimeHTTPTestServer(t, userdomain.User{ID: "usr-1"}, authorizer)
	handler.realtime = newReleaseOrderRealtimeCoordinator(func(context.Context, string) (ReleaseOrderRealtimeSnapshotPayload, error) {
		return releaseOrderRealtimeTestPayload(), nil
	}, nil)
	handler.realtime.interval = time.Hour
	handler.realtime.heartbeat = 10 * time.Millisecond
	handler.realtime.idleGrace = 10 * time.Millisecond

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/release-orders/ro-realtime-http/events", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext failed: %v", err)
	}
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	initialLines := readReleaseOrderRealtimeSSEFrame(t, reader)
	initialData := decodeReleaseOrderRealtimeSSESnapshot(t, initialLines)
	if !initialData.ValueProgressVisible || len(initialData.ValueProgress) != 1 {
		t.Fatalf("initial snapshot not visible: %#v", initialData)
	}

	authorizer.valueProgress.Store(false)
	var downgradedLines []string
	for range 5 {
		candidate := readReleaseOrderRealtimeSSEFrame(t, reader)
		if len(candidate) >= 2 && candidate[1] == "event: snapshot" {
			downgradedLines = candidate
			break
		}
	}
	if len(downgradedLines) == 0 {
		t.Fatal("permission downgrade did not produce a replacement snapshot")
	}
	downgradedData := decodeReleaseOrderRealtimeSSESnapshot(t, downgradedLines)
	if downgradedData.ValueProgressVisible || len(downgradedData.ValueProgress) != 0 {
		t.Fatalf("downgraded snapshot still contains sensitive progress: %#v", downgradedData)
	}
	if downgradedData.Version == initialData.Version {
		t.Fatalf("permission downgrade kept visible version=%q", downgradedData.Version)
	}
	cancel()
}

type realtimePermissionAuthorizer struct {
	allowValueProgress bool
}

func (a realtimePermissionAuthorizer) HasPermission(
	context.Context,
	userdomain.User,
	string,
	string,
	string,
) (bool, error) {
	return a.allowValueProgress, nil
}

func (realtimePermissionAuthorizer) ListEffectivePermissions(context.Context, userdomain.User) ([]userdomain.UserPermission, error) {
	return nil, nil
}

func newReleaseOrderRealtimeHTTPTestServer(
	t *testing.T,
	user userdomain.User,
	authorizer RequestAuthorizer,
) (*ReleaseOrderHandler, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "release-realtime.db"))
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
	db.SetMaxOpenConns(1)
	now := time.Now().UTC()
	order := releaseOrderHandlerTestOrder("ro-realtime-http", "RO-REALTIME-HTTP", now)
	order.TemplateID = ""
	order.TemplateName = ""
	order.Status = domain.OrderStatusDraft
	if err := repo.Create(context.Background(), order, nil, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	manager := usecase.NewReleaseOrderManager(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler := NewReleaseOrderHandler(manager, nil, authorizer, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		setCurrentUser(c, user)
		c.Next()
	})
	handler.RegisterRoutes(router)
	return handler, router
}

func releaseOrderRealtimeTestPayload() ReleaseOrderRealtimeSnapshotPayload {
	return ReleaseOrderRealtimeSnapshotPayload{
		Order: ReleaseOrderResponse{
			ID:                    "ro-realtime-http",
			ApplicationID:         "app-1",
			EnvCode:               "prod",
			CreatorUserID:         "usr-1",
			ApprovalApproverIDs:   []string{},
			ApprovalApproverNames: []string{},
			Status:                "running",
		},
		Executions: []ReleaseOrderExecutionResponse{},
		Steps:      []ReleaseOrderStepResponse{},
		ValueProgress: []ReleaseOrderValueProgressResponse{
			{ParamKey: "secret", Value: "sensitive-value", Status: "resolved"},
		},
		PipelineStageView: ReleaseOrderPipelineStageListResponse{Data: []ReleaseOrderPipelineStageResponse{}},
		ArtifactMetadata:  []ReleaseOrderArtifactMetadataResponse{},
		ApprovalRecords:   []ReleaseOrderApprovalRecordResponse{},
	}
}

func readReleaseOrderRealtimeSSEFrame(t *testing.T, reader *bufio.Reader) []string {
	t.Helper()
	lines := make([]string, 0, 3)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE frame failed: %v", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return lines
		}
		lines = append(lines, line)
	}
}

func decodeReleaseOrderRealtimeSSESnapshot(t *testing.T, lines []string) ReleaseOrderRealtimeSnapshotData {
	t.Helper()
	if len(lines) != 3 || !strings.HasPrefix(lines[2], "data: ") {
		t.Fatalf("unexpected snapshot frame: %#v", lines)
	}
	var data ReleaseOrderRealtimeSnapshotData
	if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[2], "data: ")), &data); err != nil {
		t.Fatalf("decode SSE snapshot failed: %v", err)
	}
	return data
}

func receiveRealtimeSnapshot(
	t *testing.T,
	events <-chan releaseOrderRealtimeSnapshot,
) releaseOrderRealtimeSnapshot {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for realtime snapshot")
		return releaseOrderRealtimeSnapshot{}
	}
}
