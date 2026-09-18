package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
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

func TestListTrackableOrdersScansReleaseOrders(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()
	order := newTestReleaseOrder("ro-trackable", "RO-TRACKABLE", "app-1", "prod", domain.OrderStatusRunning, now)
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	items, total, err := repo.ListTrackableOrders(ctx, 1, 20)
	if err != nil {
		t.Fatalf("ListTrackableOrders failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].ID != order.ID {
		t.Fatalf("items[0].ID = %q, want %q", items[0].ID, order.ID)
	}
}

func TestReleaseRepositorySchemaDoesNotCreateDeprecatedSonService(t *testing.T) {
	for _, file := range []string{
		"release_repository.go",
		"../../../../script_sql/deploy_platform.sql",
	} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("ReadFile(%s) failed: %v", file, err)
		}
		if strings.Contains(string(source), "son_service") || strings.Contains(string(source), "SonService") {
			t.Fatalf("%s should not create or reference deprecated son_service", file)
		}
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

func TestActiveOrderQueueUsesConcurrentBatchFIFO(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 14, 0, 0, 0, time.UTC)
	orders := []domain.ReleaseOrder{
		newTestReleaseOrder("ro-fifo-1", "RO-FIFO-1", "app-1", "prod", domain.OrderStatusQueued, now),
		newTestReleaseOrder("ro-fifo-2", "RO-FIFO-2", "app-1", "prod", domain.OrderStatusQueued, now.Add(time.Second)),
		newTestReleaseOrder("ro-fifo-3", "RO-FIFO-3", "app-1", "prod", domain.OrderStatusQueued, now.Add(2*time.Second)),
	}
	for index := range orders {
		orders[index].IsConcurrent = true
		orders[index].ConcurrentBatchNo = "CB-FIFO"
		orders[index].ConcurrentBatchName = "FIFO"
		orders[index].ConcurrentBatchSeq = index + 1
		if err := repo.Create(ctx, orders[index], nil, nil, nil); err != nil {
			t.Fatalf("Create(%s) failed: %v", orders[index].ID, err)
		}
	}

	if _, err := repo.FindActiveOrderByApplicationEnv(ctx, "app-1", "prod", orders[0].ID); !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("first queue item blocker error=%v, want ErrOrderNotFound", err)
	}
	blocker, err := repo.FindActiveOrderByApplicationEnv(ctx, "app-1", "prod", orders[1].ID)
	if err != nil {
		t.Fatalf("FindActiveOrderByApplicationEnv failed: %v", err)
	}
	if blocker.ID != orders[0].ID {
		t.Fatalf("second queue blocker=%s, want %s", blocker.ID, orders[0].ID)
	}
	count, err := repo.CountActiveOrdersByApplicationEnv(ctx, "app-1", "prod", orders[2].ID)
	if err != nil {
		t.Fatalf("CountActiveOrdersByApplicationEnv failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("third queue ahead count=%d, want 2", count)
	}
}

func TestQueuedFollowerDoesNotBlockExecutingConcurrentBatchOwner(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 15, 30, 0, 0, time.UTC)
	owner := newTestReleaseOrder("ro-executing-owner", "RO-EXECUTING-OWNER", "app-1", "prod", domain.OrderStatusDeploying, now)
	owner.IsConcurrent = true
	owner.ConcurrentBatchNo = "CB-EXECUTING-OWNER"
	owner.ConcurrentBatchName = "并发发布"
	owner.ConcurrentBatchSeq = 1
	follower := newTestReleaseOrder("ro-queued-follower", "RO-QUEUED-FOLLOWER", "app-1", "prod", domain.OrderStatusQueued, now.Add(time.Second))
	follower.IsConcurrent = true
	follower.ConcurrentBatchNo = owner.ConcurrentBatchNo
	follower.ConcurrentBatchName = owner.ConcurrentBatchName
	follower.ConcurrentBatchSeq = 2
	for _, order := range []domain.ReleaseOrder{owner, follower} {
		if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
			t.Fatalf("Create(%s) failed: %v", order.ID, err)
		}
	}

	if _, err := repo.FindActiveOrderByApplicationEnv(ctx, owner.ApplicationID, owner.EnvCode, owner.ID); !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("queued follower blocker error=%v, want ErrOrderNotFound", err)
	}
	count, err := repo.CountActiveOrdersByApplicationEnv(ctx, owner.ApplicationID, owner.EnvCode, owner.ID)
	if err != nil {
		t.Fatalf("CountActiveOrdersByApplicationEnv failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("queued follower ahead count=%d, want 0 for executing owner", count)
	}
}

func TestFailedOrderDoesNotBlockApplicationEnvironment(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 14, 30, 0, 0, time.UTC)
	failed := newTestReleaseOrder("ro-failed-before-next", "RO-FAILED-BEFORE-NEXT", "app-1", "prod", domain.OrderStatusFailed, now)
	current := newTestReleaseOrder("ro-after-failed", "RO-AFTER-FAILED", "app-1", "prod", domain.OrderStatusPending, now.Add(time.Second))
	for _, item := range []domain.ReleaseOrder{failed, current} {
		if err := repo.Create(ctx, item, nil, nil, nil); err != nil {
			t.Fatalf("Create(%s) failed: %v", item.ID, err)
		}
	}

	if _, err := repo.FindActiveOrderByApplicationEnv(ctx, "app-1", "prod", current.ID); !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("failed order blocker error=%v, want ErrOrderNotFound", err)
	}
	count, err := repo.CountActiveOrdersByApplicationEnv(ctx, "app-1", "prod", current.ID)
	if err != nil {
		t.Fatalf("CountActiveOrdersByApplicationEnv failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("ahead count=%d, want 0 after failed order", count)
	}
}

func TestListTrackableOrdersIncludesPrematureSuccessWithFailedHook(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 15, 0, 0, 0, time.UTC)
	order := newTestReleaseOrder("ro-success-failed-hook", "RO-SUCCESS-FAILED-HOOK", "app-1", "prod", domain.OrderStatusSuccess, now)
	hook := domain.ReleaseOrderStep{
		ID:             "step-failed-hook",
		ReleaseOrderID: order.ID,
		StepScope:      domain.StepScopeGlobal,
		StepCode:       "hook:post_release:agent_task:1",
		StepName:       "Agent Hook",
		Status:         domain.StepStatusFailed,
		SortNo:         1,
		CreatedAt:      now,
	}
	if err := repo.Create(ctx, order, nil, nil, []domain.ReleaseOrderStep{hook}); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	items, total, err := repo.ListTrackableOrders(ctx, 1, 20)
	if err != nil {
		t.Fatalf("ListTrackableOrders failed: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != order.ID {
		t.Fatalf("total=%d items=%#v, want premature success order", total, items)
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

// releaseOrderHeadCommitColumns 是发布单 HEAD 快照的全部列，迁移与读写测试共用。
var releaseOrderHeadCommitColumns = []string{
	"head_commit_sha",
	"head_commit_ref",
	"head_change_sha",
	"head_change_title",
	"head_change_author",
	"head_change_at",
	"head_change_url",
}

// TestReleaseRepositoryHeadCommitColumnsRoundTrip 覆盖 HEAD 快照列的写入与读取：
// 建单 INSERT、列表/详情 SELECT、以及创建成功后异步补写的 UpdateHeadCommit。
func TestReleaseRepositoryHeadCommitColumnsRoundTrip(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	changeAt := now.Add(-time.Hour)
	changeURL := "http://git.cloud.local:9080/code/bigData/fusion-source-web/-/commit/sha-change-1"

	order := newTestReleaseOrder("ro-head-commit", "RO-HEAD-COMMIT", "app-1", "prod", domain.OrderStatusPending, now)
	order.HeadCommitSHA = "sha-head-1"
	order.HeadCommitRef = "release/2026-09-18"
	order.HeadChangeSHA = "sha-change-1"
	order.HeadChangeTitle = "feat: 支持发布单 HEAD 落库"
	order.HeadChangeAuthor = "Alice"
	order.HeadChangeAt = &changeAt
	order.HeadChangeURL = changeURL
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	legacy := newTestReleaseOrder("ro-head-commit-legacy", "RO-HEAD-COMMIT-LEGACY", "app-1", "prod", domain.OrderStatusSuccess, now.Add(time.Second))
	if err := repo.Create(ctx, legacy, nil, nil, nil); err != nil {
		t.Fatalf("Create legacy failed: %v", err)
	}

	got, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	assertReleaseOrderHeadCommit(t, got, "sha-head-1", "release/2026-09-18", "sha-change-1", "feat: 支持发布单 HEAD 落库", "Alice", &changeAt, changeURL)

	// 列表页直接读这些列，不再实时拉 Git。
	items, _, err := repo.List(ctx, domain.ListFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("List returned %d orders, want 2", len(items))
	}
	var listed domain.ReleaseOrder
	for _, item := range items {
		if item.ID == order.ID {
			listed = item
		}
	}
	assertReleaseOrderHeadCommit(t, listed, "sha-head-1", "release/2026-09-18", "sha-change-1", "feat: 支持发布单 HEAD 落库", "Alice", &changeAt, changeURL)

	// 历史发布单不补数据：列保持空值，时间列为 NULL。
	legacyGot, err := repo.GetByID(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("GetByID legacy failed: %v", err)
	}
	assertReleaseOrderHeadCommit(t, legacyGot, "", "", "", "", "", nil, "")

	// 创建成功后的异步补写路径。
	asyncAt := changeAt.Add(time.Minute)
	asyncURL := "http://git.cloud.local:9080/code/bigData/fusion-source-web/-/commit/sha-change-2"
	if err := repo.UpdateHeadCommit(ctx, order.ID, domain.ReleaseOrderHeadCommit{
		CommitSHA:    "sha-head-2",
		CommitRef:    "main",
		ChangeSHA:    "sha-change-2",
		ChangeTitle:  "fix: 补写 HEAD 快照",
		ChangeAuthor: "Bob",
		ChangeAt:     &asyncAt,
		ChangeURL:    asyncURL,
	}); err != nil {
		t.Fatalf("UpdateHeadCommit failed: %v", err)
	}
	got, err = repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID after update failed: %v", err)
	}
	assertReleaseOrderHeadCommit(t, got, "sha-head-2", "main", "sha-change-2", "fix: 补写 HEAD 快照", "Bob", &asyncAt, asyncURL)
	if !got.UpdatedAt.Equal(now) {
		t.Fatalf("updated_at = %v, want %v: 补写 HEAD 快照不能改动 updated_at（会影响同环境排队顺序）", got.UpdatedAt, now)
	}

	// 编辑链路整行回写这些列，必须是调用方给出的值。
	got.Remark = "edited"
	if err := repo.UpdateEditable(ctx, got, nil, nil, nil); err != nil {
		t.Fatalf("UpdateEditable failed: %v", err)
	}
	edited, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID after edit failed: %v", err)
	}
	assertReleaseOrderHeadCommit(t, edited, "sha-head-2", "main", "sha-change-2", "fix: 补写 HEAD 快照", "Bob", &asyncAt, asyncURL)

	if err := repo.UpdateHeadCommit(ctx, "ro-missing", domain.ReleaseOrderHeadCommit{CommitSHA: "sha"}); !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("UpdateHeadCommit on a missing order = %v, want ErrOrderNotFound", err)
	}
}

// TestReleaseRepositoryInitSchemaAddsHeadCommitColumnsToLegacyOrderTable 覆盖已部署库的补列：
// 老的 release_order 表没有 HEAD 快照列，InitSchema 必须通过版本化迁移补上，且可重复执行。
func TestReleaseRepositoryInitSchemaAddsHeadCommitColumnsToLegacyOrderTable(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	// 变更前的 release_order 定义（没有 head_commit_* / head_change_*）。
	if _, err := db.ExecContext(ctx, `CREATE TABLE release_order (
	id TEXT PRIMARY KEY,
	order_no TEXT NOT NULL UNIQUE,
	release_name TEXT NOT NULL DEFAULT '',
	previous_order_no TEXT NOT NULL DEFAULT '',
	operation_type TEXT NOT NULL DEFAULT 'deploy',
	source_order_id TEXT NOT NULL DEFAULT '',
	source_order_no TEXT NOT NULL DEFAULT '',
	is_concurrent INTEGER NOT NULL DEFAULT 0,
	concurrent_batch_no TEXT NOT NULL DEFAULT '',
	concurrent_batch_name TEXT NOT NULL DEFAULT '',
	concurrent_batch_seq INTEGER NOT NULL DEFAULT 0,
	application_id TEXT NOT NULL,
	application_name TEXT NOT NULL DEFAULT '',
	template_id TEXT NOT NULL DEFAULT '',
	template_name TEXT NOT NULL DEFAULT '',
	delivery_engine TEXT NOT NULL DEFAULT 'k8s_native',
	strategy_snapshot_json TEXT NOT NULL DEFAULT '{}',
	binding_id TEXT NOT NULL,
	pipeline_id TEXT NOT NULL DEFAULT '',
	env_code TEXT NOT NULL,
	git_ref TEXT NOT NULL DEFAULT '',
	image_tag TEXT NOT NULL DEFAULT '',
	trigger_type TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	approval_required INTEGER NOT NULL DEFAULT 0,
	approval_mode TEXT NOT NULL DEFAULT '',
	approval_approver_ids_json TEXT NOT NULL DEFAULT '[]',
	approval_approver_names_json TEXT NOT NULL DEFAULT '[]',
	approved_at INTEGER NULL,
	approved_by TEXT NOT NULL DEFAULT '',
	rejected_at INTEGER NULL,
	rejected_by TEXT NOT NULL DEFAULT '',
	rejected_reason TEXT NOT NULL DEFAULT '',
	remark TEXT NOT NULL DEFAULT '',
	creator_user_id TEXT NOT NULL DEFAULT '',
	triggered_by TEXT NOT NULL DEFAULT '',
	executor_user_id TEXT NOT NULL DEFAULT '',
	executor_name TEXT NOT NULL DEFAULT '',
	started_at INTEGER NULL,
	finished_at INTEGER NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`); err != nil {
		t.Fatalf("create legacy release_order failed: %v", err)
	}
	// 已经部署过的库把 v1.1 迁移标记为已应用，CREATE TABLE IF NOT EXISTS 因此不会再补列，
	// 只有本次新增的版本化迁移能补上这些列。
	if err := ensureSchemaMigrationTable(ctx, db, "sqlite"); err != nil {
		t.Fatalf("create schema migration table failed: %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO gos_schema_migration (version, description, applied_at) VALUES (?, ?, 1);`,
		"deploy_platform_v1_1_release_schema",
		"legacy release schema",
	); err != nil {
		t.Fatalf("record legacy migration failed: %v", err)
	}

	repo := NewReleaseRepository(db, "sqlite")
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema on a legacy database failed: %v", err)
	}
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("repeated InitSchema failed: %v", err)
	}

	columns, err := repo.sqliteTableColumns(ctx, "release_order")
	if err != nil {
		t.Fatalf("sqliteTableColumns failed: %v", err)
	}
	for _, column := range releaseOrderHeadCommitColumns {
		if _, ok := columns[column]; !ok {
			t.Fatalf("legacy release_order missing migrated column %s", column)
		}
	}
	assertSchemaMigrationCount(t, db, releaseOrderHeadCommitMigrationVersion, 1)
}

// TestReleaseRepositoryInitSchemaRegistersHeadCommitColumnsOnFreshInstall 覆盖新库：
// CREATE TABLE 已经带了这些列，版本化迁移必须仍然登记且幂等。
func TestReleaseRepositoryInitSchemaRegistersHeadCommitColumnsOnFreshInstall(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	columns, err := repo.sqliteTableColumns(context.Background(), "release_order")
	if err != nil {
		t.Fatalf("sqliteTableColumns failed: %v", err)
	}
	for _, column := range releaseOrderHeadCommitColumns {
		if _, ok := columns[column]; !ok {
			t.Fatalf("release_order missing column %s", column)
		}
	}
	assertSchemaMigrationCount(t, repo.db, releaseOrderHeadCommitMigrationVersion, 1)
}

func assertReleaseOrderHeadCommit(
	t *testing.T,
	order domain.ReleaseOrder,
	commitSHA string,
	commitRef string,
	changeSHA string,
	changeTitle string,
	changeAuthor string,
	changeAt *time.Time,
	changeURL string,
) {
	t.Helper()

	if order.HeadCommitSHA != commitSHA || order.HeadCommitRef != commitRef {
		t.Fatalf("head_commit = %q / %q, want %q / %q", order.HeadCommitSHA, order.HeadCommitRef, commitSHA, commitRef)
	}
	if order.HeadChangeSHA != changeSHA || order.HeadChangeTitle != changeTitle || order.HeadChangeAuthor != changeAuthor {
		t.Fatalf("head_change = %q / %q / %q", order.HeadChangeSHA, order.HeadChangeTitle, order.HeadChangeAuthor)
	}
	switch {
	case changeAt == nil && order.HeadChangeAt != nil:
		t.Fatalf("head_change_at = %v, want nil", order.HeadChangeAt)
	case changeAt != nil && order.HeadChangeAt == nil:
		t.Fatalf("head_change_at = nil, want %v", changeAt)
	case changeAt != nil && !order.HeadChangeAt.Equal(changeAt.UTC()):
		t.Fatalf("head_change_at = %v, want %v", order.HeadChangeAt, changeAt)
	}
	if order.HeadChangeURL != changeURL {
		t.Fatalf("head_change_url = %q, want %q", order.HeadChangeURL, changeURL)
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
