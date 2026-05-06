package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	domain "gos/internal/domain/release"

	_ "modernc.org/sqlite"
)

// TestCountActiveOrdersByApplicationEnv_IncludesQueuedAndRunning 封装当前模块的业务处理逻辑。
func TestCountActiveOrdersByApplicationEnv_IncludesQueuedAndRunning(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()

	activeQueued := newTestReleaseOrder("ro-queued", "RO-QUEUED", "app-1", "prod", domain.OrderStatusQueued, now)
	activeDeploying := newTestReleaseOrder("ro-deploying", "RO-DEPLOYING", "app-1", "prod", domain.OrderStatusDeploying, now.Add(time.Second))
	inactiveSuccess := newTestReleaseOrder("ro-success", "RO-SUCCESS", "app-1", "prod", domain.OrderStatusSuccess, now.Add(2*time.Second))
	otherApp := newTestReleaseOrder("ro-other", "RO-OTHER", "app-2", "prod", domain.OrderStatusDeploying, now.Add(3*time.Second))

	for _, item := range []domain.ReleaseOrder{activeQueued, activeDeploying, inactiveSuccess, otherApp} {
		if err := repo.Create(ctx, item, nil, nil, nil); err != nil {
			t.Fatalf("Create(%s) failed: %v", item.OrderNo, err)
		}
	}

	count, err := repo.CountActiveOrdersByApplicationEnv(ctx, "app-1", "prod", "")
	if err != nil {
		t.Fatalf("CountActiveOrdersByApplicationEnv failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("CountActiveOrdersByApplicationEnv = %d, want 2", count)
	}
}

// TestFindActiveOrderByApplicationEnv_PrioritizesDeployingBeforeQueued 封装当前模块的业务处理逻辑。
func TestFindActiveOrderByApplicationEnv_PrioritizesDeployingBeforeQueued(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()

	queued := newTestReleaseOrder("ro-queued", "RO-QUEUED", "app-1", "prod", domain.OrderStatusQueued, now)
	deploying := newTestReleaseOrder("ro-deploying", "RO-DEPLOYING", "app-1", "prod", domain.OrderStatusDeploying, now.Add(time.Second))

	if err := repo.Create(ctx, queued, nil, nil, nil); err != nil {
		t.Fatalf("Create queued failed: %v", err)
	}
	if err := repo.Create(ctx, deploying, nil, nil, nil); err != nil {
		t.Fatalf("Create deploying failed: %v", err)
	}

	item, err := repo.FindActiveOrderByApplicationEnv(ctx, "app-1", "prod", "")
	if err != nil {
		t.Fatalf("FindActiveOrderByApplicationEnv failed: %v", err)
	}
	if item.ID != deploying.ID {
		t.Fatalf("FindActiveOrderByApplicationEnv returned %s, want %s", item.ID, deploying.ID)
	}
}

// TestList_StatusFilterSupportsLegacyAndBusinessAlias 查询并返回列表数据。
func TestList_StatusFilterSupportsLegacyAndBusinessAlias(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()

	successLegacy := newTestReleaseOrder("ro-success-legacy", "RO-SUCCESS-LEGACY", "app-1", "dev", domain.OrderStatusSuccess, now)
	successBusiness := newTestReleaseOrder("ro-success-biz", "RO-SUCCESS-BIZ", "app-1", "dev", domain.OrderStatusDeploySuccess, now.Add(time.Second))
	failedLegacy := newTestReleaseOrder("ro-failed-legacy", "RO-FAILED-LEGACY", "app-1", "dev", domain.OrderStatusFailed, now.Add(2*time.Second))
	failedBusiness := newTestReleaseOrder("ro-failed-biz", "RO-FAILED-BIZ", "app-1", "dev", domain.OrderStatusDeployFailed, now.Add(3*time.Second))
	runningLegacy := newTestReleaseOrder("ro-running-legacy", "RO-RUNNING-LEGACY", "app-1", "dev", domain.OrderStatusRunning, now.Add(4*time.Second))
	runningBusiness := newTestReleaseOrder("ro-running-biz", "RO-RUNNING-BIZ", "app-1", "dev", domain.OrderStatusDeploying, now.Add(5*time.Second))

	for _, item := range []domain.ReleaseOrder{
		successLegacy,
		successBusiness,
		failedLegacy,
		failedBusiness,
		runningLegacy,
		runningBusiness,
	} {
		if err := repo.Create(ctx, item, nil, nil, nil); err != nil {
			t.Fatalf("Create(%s) failed: %v", item.OrderNo, err)
		}
	}

	assertIDs := func(items []domain.ReleaseOrder, expected ...string) {
		got := make(map[string]struct{}, len(items))
		for _, item := range items {
			got[item.ID] = struct{}{}
		}
		if len(got) != len(expected) {
			t.Fatalf("got %d items, want %d", len(got), len(expected))
		}
		for _, id := range expected {
			if _, ok := got[id]; !ok {
				t.Fatalf("expected order %s to be returned", id)
			}
		}
	}

	successItems, _, err := repo.List(ctx, domain.ListFilter{
		Status:   domain.OrderStatusDeploySuccess,
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("List by deploy_success failed: %v", err)
	}
	assertIDs(successItems, successLegacy.ID, successBusiness.ID)

	failedItems, _, err := repo.List(ctx, domain.ListFilter{
		Status:   domain.OrderStatusDeployFailed,
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("List by deploy_failed failed: %v", err)
	}
	assertIDs(failedItems, failedLegacy.ID, failedBusiness.ID)

	runningItems, _, err := repo.List(ctx, domain.ListFilter{
		Status:   domain.OrderStatusDeploying,
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("List by deploying failed: %v", err)
	}
	assertIDs(runningItems, runningLegacy.ID, runningBusiness.ID)
}

// TestList_ConcurrentBatchFilters 查询并返回列表数据。
func TestList_ConcurrentBatchFilters(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()

	targetByNo := newTestReleaseOrder("ro-batch-no", "RO-BATCH-NO", "app-1", "dev", domain.OrderStatusApproved, now)
	targetByNo.ConcurrentBatchNo = "CB-20260504-ABC"
	targetByNo.ConcurrentBatchName = "灰度发布第一批"
	targetByName := newTestReleaseOrder("ro-batch-name", "RO-BATCH-NAME", "app-1", "dev", domain.OrderStatusApproved, now.Add(time.Second))
	targetByName.ConcurrentBatchNo = "CB-20260504-DEF"
	targetByName.ConcurrentBatchName = "核心服务并发发布"
	other := newTestReleaseOrder("ro-batch-other", "RO-BATCH-OTHER", "app-1", "dev", domain.OrderStatusApproved, now.Add(2*time.Second))
	other.ConcurrentBatchNo = "CB-20260504-GHI"
	other.ConcurrentBatchName = "普通批次"

	for _, item := range []domain.ReleaseOrder{targetByNo, targetByName, other} {
		if err := repo.Create(ctx, item, nil, nil, nil); err != nil {
			t.Fatalf("Create(%s) failed: %v", item.OrderNo, err)
		}
	}

	itemsByNo, totalByNo, err := repo.List(ctx, domain.ListFilter{
		ConcurrentBatchNo: "ABC",
		Page:              1,
		PageSize:          20,
	})
	if err != nil {
		t.Fatalf("List by concurrent batch no failed: %v", err)
	}
	if totalByNo != 1 || len(itemsByNo) != 1 || itemsByNo[0].ID != targetByNo.ID {
		t.Fatalf("List by no returned (%d, %#v), want %s only", totalByNo, itemsByNo, targetByNo.ID)
	}

	itemsByName, totalByName, err := repo.List(ctx, domain.ListFilter{
		ConcurrentBatchName: "核心服务",
		Page:                1,
		PageSize:            20,
	})
	if err != nil {
		t.Fatalf("List by concurrent batch name failed: %v", err)
	}
	if totalByName != 1 || len(itemsByName) != 1 || itemsByName[0].ID != targetByName.ID {
		t.Fatalf("List by name returned (%d, %#v), want %s only", totalByName, itemsByName, targetByName.ID)
	}
}

// TestList_VisibilityIncludesAppCreatorAndApprover 查询并返回列表数据。
func TestList_VisibilityIncludesAppCreatorAndApprover(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()

	appVisible := newTestReleaseOrder("ro-visible-app", "RO-VISIBLE-APP", "app-visible", "prod", domain.OrderStatusApproved, now)
	creatorVisible := newTestReleaseOrder("ro-visible-creator", "RO-VISIBLE-CREATOR", "app-hidden", "prod", domain.OrderStatusApproved, now.Add(time.Second))
	creatorVisible.CreatorUserID = "viewer"
	approverVisible := newTestReleaseOrder("ro-visible-approver", "RO-VISIBLE-APPROVER", "app-hidden", "prod", domain.OrderStatusApproved, now.Add(2*time.Second))
	approverVisible.ApprovalApproverIDs = []string{"viewer"}
	hidden := newTestReleaseOrder("ro-hidden", "RO-HIDDEN", "app-hidden", "prod", domain.OrderStatusApproved, now.Add(3*time.Second))

	for _, item := range []domain.ReleaseOrder{appVisible, creatorVisible, approverVisible, hidden} {
		if err := repo.Create(ctx, item, nil, nil, nil); err != nil {
			t.Fatalf("Create(%s) failed: %v", item.OrderNo, err)
		}
	}

	items, total, err := repo.List(ctx, domain.ListFilter{
		ApplicationIDs:  []string{"app-visible"},
		VisibleToUserID: "viewer",
		Page:            1,
		PageSize:        20,
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 3 {
		t.Fatalf("List total = %d, want 3", total)
	}
	got := make(map[string]struct{}, len(items))
	for _, item := range items {
		got[item.ID] = struct{}{}
	}
	for _, expected := range []string{appVisible.ID, creatorVisible.ID, approverVisible.ID} {
		if _, ok := got[expected]; !ok {
			t.Fatalf("expected visible order %s to be returned", expected)
		}
	}
	if _, ok := got[hidden.ID]; ok {
		t.Fatalf("did not expect hidden order %s to be returned", hidden.ID)
	}
}

// TestListApprovalRecordSummaries_VisibilityIncludesAppCreatorAndApprover 查询并返回列表数据。
func TestListApprovalRecordSummaries_VisibilityIncludesAppCreatorAndApprover(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()

	appVisible := newTestReleaseOrder("ro-summary-app", "RO-SUMMARY-APP", "app-visible", "prod", domain.OrderStatusApproving, now)
	creatorVisible := newTestReleaseOrder("ro-summary-creator", "RO-SUMMARY-CREATOR", "app-hidden", "prod", domain.OrderStatusApproving, now.Add(time.Second))
	creatorVisible.CreatorUserID = "viewer"
	approverVisible := newTestReleaseOrder("ro-summary-approver", "RO-SUMMARY-APPROVER", "app-hidden", "prod", domain.OrderStatusApproving, now.Add(2*time.Second))
	approverVisible.ApprovalApproverIDs = []string{"viewer"}
	hidden := newTestReleaseOrder("ro-summary-hidden", "RO-SUMMARY-HIDDEN", "app-hidden", "prod", domain.OrderStatusApproving, now.Add(3*time.Second))

	for _, item := range []domain.ReleaseOrder{appVisible, creatorVisible, approverVisible, hidden} {
		if err := repo.Create(ctx, item, nil, nil, nil); err != nil {
			t.Fatalf("Create(%s) failed: %v", item.OrderNo, err)
		}
		if err := repo.CreateApprovalRecord(ctx, domain.ReleaseOrderApprovalRecord{
			ID:             "rec-" + item.ID,
			ReleaseOrderID: item.ID,
			Action:         domain.ReleaseOrderApprovalActionSubmit,
			OperatorUserID: "operator",
			OperatorName:   "operator",
			Comment:        "submitted",
			CreatedAt:      item.CreatedAt,
		}); err != nil {
			t.Fatalf("CreateApprovalRecord(%s) failed: %v", item.OrderNo, err)
		}
	}

	items, total, err := repo.ListApprovalRecordSummaries(ctx, domain.ApprovalRecordListFilter{
		ApplicationIDs:  []string{"app-visible"},
		VisibleToUserID: "viewer",
		Page:            1,
		PageSize:        20,
	})
	if err != nil {
		t.Fatalf("ListApprovalRecordSummaries failed: %v", err)
	}
	if total != 3 {
		t.Fatalf("ListApprovalRecordSummaries total = %d, want 3", total)
	}
	got := make(map[string]struct{}, len(items))
	for _, item := range items {
		got[item.ReleaseOrderID] = struct{}{}
	}
	for _, expected := range []string{appVisible.ID, creatorVisible.ID, approverVisible.ID} {
		if _, ok := got[expected]; !ok {
			t.Fatalf("expected visible summary %s to be returned", expected)
		}
	}
	if _, ok := got[hidden.ID]; ok {
		t.Fatalf("did not expect hidden summary %s to be returned", hidden.ID)
	}
}

// TestCreateTemplate_PersistsHookEnvCodes 创建业务资源并返回处理结果。
func TestCreateTemplate_PersistsHookEnvCodes(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()

	template := domain.ReleaseTemplate{
		ID:              "rt-1",
		Name:            "template-1",
		ApplicationID:   "app-1",
		ApplicationName: "App 1",
		BindingID:       "app-1",
		BindingName:     "App 1",
		BindingType:     "application",
		Status:          domain.TemplateStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	hooks := []domain.ReleaseTemplateHook{
		{
			ID:               "hook-1",
			TemplateID:       template.ID,
			HookType:         domain.TemplateHookTypeWebhookNotification,
			Name:             "prod hook",
			ExecuteStage:     domain.TemplateHookExecuteStageBuildComplete,
			ExecuteStages:    []domain.TemplateHookExecuteStage{domain.TemplateHookExecuteStageBuildComplete, domain.TemplateHookExecuteStagePostRelease},
			TriggerCondition: domain.TemplateHookTriggerOnSuccess,
			FailurePolicy:    domain.TemplateHookFailurePolicyWarnOnly,
			EnvCodes:         []string{"prod", "pre"},
			WebhookMethod:    "POST",
			WebhookURL:       "https://example.com/hook",
			WebhookBody:      "{}",
			SortNo:           1,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}

	if err := repo.CreateTemplate(ctx, template, nil, nil, nil, hooks); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	_, _, _, _, storedHooks, err := repo.GetTemplateByID(ctx, template.ID)
	if err != nil {
		t.Fatalf("GetTemplateByID failed: %v", err)
	}
	if len(storedHooks) != 1 {
		t.Fatalf("stored hooks len = %d, want 1", len(storedHooks))
	}
	if got := storedHooks[0].EnvCodes; len(got) != 2 || got[0] != "prod" || got[1] != "pre" {
		t.Fatalf("stored hook env codes = %#v, want [prod pre]", got)
	}
	if storedHooks[0].ExecuteStage != domain.TemplateHookExecuteStageBuildComplete {
		t.Fatalf("stored hook execute stage = %s, want %s", storedHooks[0].ExecuteStage, domain.TemplateHookExecuteStageBuildComplete)
	}
	if got := storedHooks[0].ExecuteStages; len(got) != 2 || got[0] != domain.TemplateHookExecuteStageBuildComplete || got[1] != domain.TemplateHookExecuteStagePostRelease {
		t.Fatalf("stored hook execute stages = %#v, want [build_complete post_release]", got)
	}
}

// TestConfirmAppReleaseState_RejectsOutdatedOrder 封装当前模块的业务处理逻辑。
func TestConfirmAppReleaseState_RejectsOutdatedOrder(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()

	older := newTestReleaseOrder("ro-live-older", "RO-LIVE-OLDER", "app-1", "prod", domain.OrderStatusDeploySuccess, now)
	newer := newTestReleaseOrder("ro-live-newer", "RO-LIVE-NEWER", "app-1", "prod", domain.OrderStatusDeploySuccess, now.Add(time.Minute))

	for _, item := range []domain.ReleaseOrder{older, newer} {
		if err := repo.Create(ctx, item, nil, nil, nil); err != nil {
			t.Fatalf("Create(%s) failed: %v", item.OrderNo, err)
		}
		if err := repo.UpsertAppReleaseState(ctx, domain.AppReleaseState{
			ID:                    "state-" + item.ID,
			ReleaseOrderID:        item.ID,
			ReleaseOrderNo:        item.OrderNo,
			ApplicationID:         item.ApplicationID,
			ApplicationName:       item.ApplicationName,
			EnvCode:               item.EnvCode,
			OperationType:         item.OperationType,
			StateStatus:           domain.AppReleaseStateStatusPendingConfirm,
			ParamsSnapshotJSON:    "[]",
			ExecutionSnapshotJSON: "[]",
			DeploySnapshotJSON:    "",
			ResultSnapshotJSON:    "{}",
			CreatedAt:             item.CreatedAt,
			UpdatedAt:             item.UpdatedAt,
		}); err != nil {
			t.Fatalf("UpsertAppReleaseState(%s) failed: %v", item.OrderNo, err)
		}
	}

	ok, err := repo.IsLatestOrderByApplicationEnv(ctx, older.ApplicationID, older.EnvCode, older.ID)
	if err != nil {
		t.Fatalf("IsLatestOrderByApplicationEnv failed: %v", err)
	}
	if ok {
		t.Fatalf("expected older order to be non-latest")
	}

	_, err = repo.ConfirmAppReleaseState(ctx, older.ID, "tester", now.Add(2*time.Minute))
	if !errors.Is(err, domain.ErrAppReleaseStateNotConfirmable) {
		t.Fatalf("ConfirmAppReleaseState error = %v, want ErrAppReleaseStateNotConfirmable", err)
	}
}

// newTestReleaseRepository 封装当前模块的业务处理逻辑。
func newTestReleaseRepository(t *testing.T) *ReleaseRepository {
	t.Helper()

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

	repo := NewReleaseRepository(db, "sqlite")
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	return repo
}

// newTestReleaseOrder 封装当前模块的业务处理逻辑。
func newTestReleaseOrder(id, orderNo, applicationID, envCode string, status domain.OrderStatus, createdAt time.Time) domain.ReleaseOrder {
	return domain.ReleaseOrder{
		ID:                  id,
		OrderNo:             orderNo,
		OperationType:       domain.OperationTypeDeploy,
		ApplicationID:       applicationID,
		ApplicationName:     applicationID,
		BindingID:           "binding-1",
		EnvCode:             envCode,
		TriggerType:         domain.TriggerTypeManual,
		Status:              status,
		ApprovalApproverIDs: []string{},
		CreatorUserID:       "tester",
		TriggeredBy:         "tester",
		CreatedAt:           createdAt,
		UpdatedAt:           createdAt,
	}
}
