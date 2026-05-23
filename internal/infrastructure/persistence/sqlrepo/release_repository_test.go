package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	appdomain "gos/internal/domain/application"
	artifactdomain "gos/internal/domain/artifactrepo"
	projectdomain "gos/internal/domain/project"
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

func TestReleaseRepositoryInitSchemaCreatesStrategySnapshotColumns(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	columns, err := repo.sqliteTableColumns(context.Background(), "release_order")
	if err != nil {
		t.Fatalf("sqliteTableColumns failed: %v", err)
	}
	for _, column := range []string{"delivery_engine", "strategy_snapshot_json"} {
		if _, ok := columns[column]; !ok {
			t.Fatalf("release_order missing column %s", column)
		}
	}

	now := time.Now().UTC()
	order := newTestReleaseOrder("ro-strategy-default", "RO-STRATEGY-DEFAULT", "app-1", "prod", domain.OrderStatusPending, now)
	if err := repo.Create(context.Background(), order, nil, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	got, err := repo.GetByID(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.DeliveryEngine != defaultReleaseOrderDeliveryEngine {
		t.Fatalf("DeliveryEngine = %q, want %q", got.DeliveryEngine, defaultReleaseOrderDeliveryEngine)
	}
	if got.StrategySnapshotJSON != defaultReleaseOrderStrategySnapshotJSON {
		t.Fatalf("StrategySnapshotJSON = %q, want %q", got.StrategySnapshotJSON, defaultReleaseOrderStrategySnapshotJSON)
	}
}

func TestCreateDeploySnapshotAllowsMultipleInstancesForSameOrder(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()
	order := newTestReleaseOrder("ro-multi-snapshot", "RO-MULTI-SNAPSHOT", "app-1", "prod", domain.OrderStatusSuccess, now)
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
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
			Branch:           "demo-prod",
			SourcePath:       "apps/demo/helm",
			EnvCode:          "prod",
			SnapshotPayload:  `{"image_version":"101"}`,
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
			Branch:           "demo-prod",
			SourcePath:       "apps/demo/helm",
			EnvCode:          "prod",
			SnapshotPayload:  `{"image_version":"101"}`,
			CreatedAt:        now,
		},
	} {
		if err := repo.CreateDeploySnapshot(ctx, snapshot); err != nil {
			t.Fatalf("CreateDeploySnapshot(%s) failed: %v", snapshot.ArgoCDInstanceID, err)
		}
	}

	var count int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM release_order_deploy_snapshot WHERE release_order_id = ?`, order.ID).Scan(&count); err != nil {
		t.Fatalf("count snapshots failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("snapshot count = %d, want 2", count)
	}

	snapshots, err := repo.ListDeploySnapshotsByOrderID(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListDeploySnapshotsByOrderID failed: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("ListDeploySnapshotsByOrderID returned %d snapshots, want 2", len(snapshots))
	}
}

func TestReleaseRepositoryListArtifactMetadataSummariesFiltersByProjectApplicationAndOrder(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 12, 15, 11, 21, 0, time.UTC)

	projectRepo := NewProjectRepository(repo.db, "sqlite")
	if err := projectRepo.InitSchema(ctx); err != nil {
		t.Fatalf("project InitSchema failed: %v", err)
	}
	if err := projectRepo.Create(ctx, projectdomain.Project{
		ID:        "project-shangxin",
		Name:      "尚信",
		Key:       "shangxin",
		Status:    projectdomain.StatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create project failed: %v", err)
	}

	applicationRepo := NewApplicationRepository(repo.db, "sqlite")
	if err := applicationRepo.InitSchema(ctx); err != nil {
		t.Fatalf("application InitSchema failed: %v", err)
	}
	app := appdomain.Application{
		ID:           "app-notary",
		Name:         "尚信前端-测试",
		Key:          "sx-notary",
		ProjectID:    "project-shangxin",
		RepoURL:      "https://git.example.com/notary.git",
		Owner:        "tester",
		Status:       appdomain.StatusActive,
		ArtifactType: "zip",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	app.SetLanguage("nodejs")
	if err := applicationRepo.Create(ctx, app); err != nil {
		t.Fatalf("Create application failed: %v", err)
	}

	order := newTestReleaseOrder("ro-artifact-center", "RO-20260512070847-8C67ECB7", app.ID, "dev", domain.OrderStatusSuccess, now)
	order.ReleaseName = "尚信前端测试发布"
	order.ApplicationName = app.Name
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}
	if _, err := repo.UpsertArtifactMetadata(ctx, domain.ReleaseOrderArtifactMetadata{
		ID:              "roart-artifact-center",
		ReleaseOrderID:  order.ID,
		PipelineScope:   domain.PipelineScopeCI,
		ArtifactName:    "notarybusiness-9.zip",
		ArtifactType:    "zip",
		ArtifactVersion: "9",
		ArtifactURL:     "https://oss.example.com/notarybusiness-9.zip",
		RepositoryID:    "repo-oss",
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

	items, total, err := repo.ListArtifactMetadataSummaries(ctx, domain.ArtifactMetadataListFilter{
		ProjectID:      "project-shangxin",
		ApplicationID:  app.ID,
		ReleaseOrderID: order.ID,
		Keyword:        "前端测试",
		PipelineScope:  domain.PipelineScopeCI,
		Page:           1,
		PageSize:       20,
	})
	if err != nil {
		t.Fatalf("ListArtifactMetadataSummaries failed: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("total=%d len=%d, want 1", total, len(items))
	}
	got := items[0]
	if got.Artifact.ArtifactName != "notarybusiness-9.zip" {
		t.Fatalf("ArtifactName = %q", got.Artifact.ArtifactName)
	}
	if got.ReleaseName != "尚信前端测试发布" || got.ReleaseOrderNo != "RO-20260512070847-8C67ECB7" {
		t.Fatalf("release context = %q/%q", got.ReleaseName, got.ReleaseOrderNo)
	}
	if got.ApplicationName != app.Name || got.ProjectName != "尚信" {
		t.Fatalf("application/project context = %q/%q", got.ApplicationName, got.ProjectName)
	}
}

func TestReleaseRepositoryDeleteOrdersRemovesArtifactMetadata(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)

	order := newTestReleaseOrder("ro-delete-artifacts", "RO-DELETE-ARTIFACTS", "app-delete-artifacts", "dev", domain.OrderStatusSuccess, now)
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}
	if _, err := repo.UpsertArtifactMetadata(ctx, domain.ReleaseOrderArtifactMetadata{
		ID:             "roart-delete-with-order",
		ReleaseOrderID: order.ID,
		PipelineScope:  domain.PipelineScopeCI,
		ArtifactName:   "pipeline-package.zip",
		ArtifactURL:    "https://oss.example.com/pipeline-package.zip",
		ExecutionID:    "exec-ci-delete",
		MetadataJSON:   "{}",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertArtifactMetadata failed: %v", err)
	}

	if err := repo.DeleteOrders(ctx, []string{order.ID}); err != nil {
		t.Fatalf("DeleteOrders failed: %v", err)
	}
	items, err := repo.ListArtifactMetadata(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListArtifactMetadata failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("artifact metadata count = %d, want 0", len(items))
	}
}

func TestReleaseRepositoryListArtifactMetadataSummariesUsesApplicationRepositoryConfig(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 12, 16, 0, 0, 0, time.UTC)

	artifactRepo := NewArtifactRepositoryConfigRepository(repo.db, "sqlite")
	if err := artifactRepo.InitSchema(ctx); err != nil {
		t.Fatalf("artifact repository InitSchema failed: %v", err)
	}
	if err := NewProjectRepository(repo.db, "sqlite").InitSchema(ctx); err != nil {
		t.Fatalf("project InitSchema failed: %v", err)
	}
	if err := artifactRepo.Create(ctx, artifactdomain.ArtifactRepository{
		ID:              "repo-app-oss",
		Name:            "应用绑定 OSS",
		RepositoryType:  artifactdomain.RepositoryTypeOSS,
		Endpoint:        "https://oss-cn-shanghai.aliyuncs.com",
		Bucket:          "app-bucket",
		Directory:       "release",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
		ACL:             artifactdomain.ACLPrivate,
		Status:          artifactdomain.StatusEnabled,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("Create artifact repository failed: %v", err)
	}

	applicationRepo := NewApplicationRepository(repo.db, "sqlite")
	if err := applicationRepo.InitSchema(ctx); err != nil {
		t.Fatalf("application InitSchema failed: %v", err)
	}
	app := appdomain.Application{
		ID:                   "app-bound-repo",
		Name:                 "绑定制品库应用",
		Key:                  "bound-repo",
		RepoURL:              "https://git.example.com/bound.git",
		Owner:                "tester",
		Status:               appdomain.StatusActive,
		ArtifactType:         "zip",
		ArtifactRepositoryID: "repo-app-oss",
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	app.SetLanguage("nodejs")
	if err := applicationRepo.Create(ctx, app); err != nil {
		t.Fatalf("Create application failed: %v", err)
	}

	order := newTestReleaseOrder("ro-bound-repo", "RO-BOUND-REPO", app.ID, "dev", domain.OrderStatusSuccess, now)
	order.ReleaseName = "绑定制品库发布"
	order.ApplicationName = app.Name
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}
	if _, err := repo.UpsertArtifactMetadata(ctx, domain.ReleaseOrderArtifactMetadata{
		ID:              "roart-bound-repo",
		ReleaseOrderID:  order.ID,
		PipelineScope:   domain.PipelineScopeCI,
		ArtifactName:    "bound-repo.zip",
		ArtifactType:    "zip",
		ArtifactVersion: "1",
		ArtifactURL:     "https://oss.example.com/bound-repo.zip",
		MetadataJSON:    "{}",
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("UpsertArtifactMetadata failed: %v", err)
	}

	items, total, err := repo.ListArtifactMetadataSummaries(ctx, domain.ArtifactMetadataListFilter{
		RepositoryID: "repo-app-oss",
		Page:         1,
		PageSize:     20,
	})
	if err != nil {
		t.Fatalf("ListArtifactMetadataSummaries failed: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("total=%d len=%d, want 1", total, len(items))
	}
	if items[0].Artifact.RepositoryID != "repo-app-oss" {
		t.Fatalf("RepositoryID = %q", items[0].Artifact.RepositoryID)
	}
	if items[0].Artifact.RepositoryName != "应用绑定 OSS" {
		t.Fatalf("RepositoryName = %q", items[0].Artifact.RepositoryName)
	}
	if items[0].Artifact.Bucket != "app-bucket" {
		t.Fatalf("Bucket = %q", items[0].Artifact.Bucket)
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
	if err := NewProjectRepository(db, "sqlite").InitSchema(context.Background()); err != nil {
		t.Fatalf("project InitSchema failed: %v", err)
	}
	if err := NewApplicationRepository(db, "sqlite").InitSchema(context.Background()); err != nil {
		t.Fatalf("application InitSchema failed: %v", err)
	}
	if err := NewArtifactRepositoryConfigRepository(db, "sqlite").InitSchema(context.Background()); err != nil {
		t.Fatalf("artifact repository InitSchema failed: %v", err)
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
