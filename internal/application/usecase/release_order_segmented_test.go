package usecase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pipelinedomain "gos/internal/domain/pipeline"
	domain "gos/internal/domain/release"
)

// TestBuildDispatchMarksOrderBuilding 组装业务执行所需的输入数据。
func TestBuildDispatchMarksOrderBuilding(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	manager.jenkins = segmentedReleaseNoopJenkinsExecutor{}

	order := testReleaseOrder("ro-build", "RO-BUILD", domain.OrderStatusPending, now)
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}
	if err := repo.Create(ctx, order, executions, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	updated, err := manager.Build(ctx, order.ID, "", "")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if updated.Status != domain.OrderStatusBuilding {
		t.Fatalf("updated status = %s, want %s", updated.Status, domain.OrderStatusBuilding)
	}
	if updated.StartedAt == nil {
		t.Fatal("updated started_at = nil, want non-nil")
	}
}

// TestBuildRequiresCDExecution 组装业务执行所需的输入数据。
func TestBuildRequiresCDExecution(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	manager.jenkins = segmentedReleaseNoopJenkinsExecutor{}

	order := testReleaseOrder("ro-build-ci-only", "RO-BUILD-CI-ONLY", domain.OrderStatusPending, now)
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
	}
	if err := repo.Create(ctx, order, executions, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if _, err := manager.Build(ctx, order.ID, "", ""); err == nil {
		t.Fatal("Build error = nil, want error")
	}
}

// TestBuildSuccessTransitionsToBuiltWaitingDeploy 组装业务执行所需的输入数据。
func TestBuildSuccessTransitionsToBuiltWaitingDeploy(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	startedAt := now.Add(-2 * time.Minute)
	manager.now = func() time.Time { return now }
	manager.jenkins = segmentedReleaseNoopJenkinsExecutor{}

	order := testReleaseOrder("ro-built", "RO-BUILT", domain.OrderStatusBuilding, now)
	order.StartedAt = &startedAt
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}
	steps := []domain.ReleaseOrderStep{
		testReleaseStep(order.ID, "step-finish", domain.StepScopeGlobal, "global:release_finish", domain.StepStatusPending, 99, now),
	}
	if err := repo.Create(ctx, order, executions, nil, steps); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	tracker := NewTrackReleaseExecution(manager, nil)
	tracker.now = func() time.Time { return now }
	updated, err := tracker.syncNextStepAfterExecution(ctx, order)
	if err != nil {
		t.Fatalf("syncNextStepAfterExecution failed: %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	stored, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.Status != domain.OrderStatusBuiltWaitingDeploy {
		t.Fatalf("stored status = %s, want %s", stored.Status, domain.OrderStatusBuiltWaitingDeploy)
	}
	if stored.StartedAt == nil || !stored.StartedAt.Equal(startedAt) {
		t.Fatalf("stored started_at = %#v, want %v", stored.StartedAt, startedAt)
	}
}

// TestTrackerDoesNotAutoDeployAfterBuildSuccess 组装业务执行所需的输入数据。
func TestTrackerDoesNotAutoDeployAfterBuildSuccess(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	startedAt := now.Add(-2 * time.Minute)
	manager.now = func() time.Time { return now }
	manager.jenkins = segmentedReleaseNoopJenkinsExecutor{}

	order := testReleaseOrder("ro-build-no-autodeploy", "RO-BUILD-NO-AUTODEPLOY", domain.OrderStatusBuilding, now)
	order.StartedAt = &startedAt
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}
	steps := []domain.ReleaseOrderStep{
		testReleaseStep(order.ID, "step-finish", domain.StepScopeGlobal, "global:release_finish", domain.StepStatusPending, 99, now),
	}
	if err := repo.Create(ctx, order, executions, nil, steps); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	tracker := NewTrackReleaseExecution(manager, nil)
	tracker.now = func() time.Time { return now }
	updated, skipped, err := tracker.syncOrder(ctx, order)
	if err != nil {
		t.Fatalf("syncOrder failed: %v", err)
	}
	if skipped {
		t.Fatal("skipped = true, want false")
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	stored, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.Status != domain.OrderStatusBuiltWaitingDeploy {
		t.Fatalf("stored status = %s, want %s", stored.Status, domain.OrderStatusBuiltWaitingDeploy)
	}

	storedExecutions, err := repo.ListExecutions(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListExecutions failed: %v", err)
	}
	cdExecution := findExecutionByScopeAndStatus(storedExecutions, domain.PipelineScopeCD, domain.ExecutionStatusPending)
	if cdExecution == nil {
		t.Fatalf("cd execution status changed unexpectedly: %#v", storedExecutions)
	}
	if findExecutionByScopeAndStatus(storedExecutions, domain.PipelineScopeCD, domain.ExecutionStatusRunning) != nil {
		t.Fatalf("cd execution started unexpectedly: %#v", storedExecutions)
	}
}

// TestSyncFailedOrderClosesRunningCDAndKeepsHooksPending 同步外部或内部状态数据。
func TestSyncFailedOrderClosesRunningCDAndKeepsHooksPending(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	startedAt := now.Add(-2 * time.Minute)
	manager.now = func() time.Time { return now }

	order := testReleaseOrder("ro-failed-dirty", "RO-FAILED-DIRTY", domain.OrderStatusFailed, now)
	order.TemplateID = ""
	order.TemplateName = ""
	order.StartedAt = &startedAt
	order.FinishedAt = &now
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now),
		{
			ID:             "exec-cd",
			ReleaseOrderID: order.ID,
			PipelineScope:  domain.PipelineScopeCD,
			BindingID:      "binding-cd",
			BindingName:    "Binding cd",
			Provider:       "argocd",
			PipelineID:     "pipeline-cd",
			Status:         domain.ExecutionStatusRunning,
			ExternalRunID:  "commit-1",
			StartedAt:      &startedAt,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
	steps := []domain.ReleaseOrderStep{
		testReleaseStep(order.ID, "step-git-push", domain.StepScopeCD, "cd:git_push", domain.StepStatusFailed, 10, now),
		{
			ID:             "step-health",
			ReleaseOrderID: order.ID,
			StepScope:      domain.StepScopeCD,
			StepCode:       "cd:health_check",
			StepName:       "cd:health_check",
			Status:         domain.StepStatusRunning,
			SortNo:         11,
			CreatedAt:      now,
			StartedAt:      &startedAt,
		},
		{
			ID:             "step-hook",
			ReleaseOrderID: order.ID,
			StepScope:      domain.StepScopeGlobal,
			StepCode:       "hook:post_release:notification_hook:1",
			StepName:       "hook:post_release:notification_hook:1",
			Status:         domain.StepStatusPending,
			SortNo:         12,
			CreatedAt:      now,
		},
		testReleaseStep(order.ID, "step-finish", domain.StepScopeGlobal, "global:release_finish", domain.StepStatusFailed, 99, now),
	}
	if err := repo.Create(ctx, order, executions, nil, steps); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	tracker := NewTrackReleaseExecution(manager, nil)
	tracker.now = func() time.Time { return now }
	updated, skipped, err := tracker.syncOrder(ctx, order)
	if err != nil {
		t.Fatalf("syncOrder failed: %v", err)
	}
	if skipped {
		t.Fatal("skipped = true, want false")
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	storedExecutions, err := repo.ListExecutions(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListExecutions failed: %v", err)
	}
	cdExecution := findExecutionByScopeAndStatus(storedExecutions, domain.PipelineScopeCD, domain.ExecutionStatusFailed)
	if cdExecution == nil {
		t.Fatalf("cd execution was not closed as failed: %#v", storedExecutions)
	}

	storedSteps, err := repo.ListSteps(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListSteps failed: %v", err)
	}
	healthStep := findStepByCode(storedSteps, "cd:health_check")
	if healthStep == nil || healthStep.Status != domain.StepStatusFailed {
		t.Fatalf("health step = %#v, want failed", healthStep)
	}
	hookStep := findStepByCode(storedSteps, "hook:post_release:notification_hook:1")
	if hookStep == nil || hookStep.Status != domain.StepStatusPending {
		t.Fatalf("hook step = %#v, want pending", hookStep)
	}
}

// TestSyncFailedOrderExecutesPostReleaseFailureHooks 同步外部或内部状态数据。
func TestSyncFailedOrderExecutesPostReleaseFailureHooks(t *testing.T) {
	t.Parallel()

	webhookCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCalls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	startedAt := now.Add(-2 * time.Minute)
	manager.now = func() time.Time { return now }

	template := domain.ReleaseTemplate{
		ID:              "rt-failed-hook",
		Name:            "Failed Hook Template",
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
			ID:               "hook-post-release-failed",
			TemplateID:       template.ID,
			HookType:         domain.TemplateHookTypeWebhookNotification,
			Name:             "失败后通知",
			ExecuteStage:     domain.TemplateHookExecuteStagePostRelease,
			TriggerCondition: domain.TemplateHookTriggerOnFailed,
			FailurePolicy:    domain.TemplateHookFailurePolicyBlockRelease,
			WebhookMethod:    http.MethodPost,
			WebhookURL:       server.URL,
			WebhookBody:      `{"order_no":"{order_no}","status":"{release_status}"}`,
			SortNo:           1,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}
	if err := repo.CreateTemplate(ctx, template, nil, nil, nil, hooks); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	order := testReleaseOrder("ro-failed-hook", "RO-FAILED-HOOK", domain.OrderStatusFailed, now)
	order.TemplateID = template.ID
	order.TemplateName = template.Name
	order.StartedAt = &startedAt
	order.FinishedAt = &now
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now),
		{
			ID:             "exec-cd",
			ReleaseOrderID: order.ID,
			PipelineScope:  domain.PipelineScopeCD,
			BindingID:      "binding-cd",
			BindingName:    "Binding cd",
			Provider:       "argocd",
			PipelineID:     "pipeline-cd",
			Status:         domain.ExecutionStatusRunning,
			ExternalRunID:  "commit-1",
			StartedAt:      &startedAt,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
	steps := []domain.ReleaseOrderStep{
		testReleaseStep(order.ID, "step-git-push", domain.StepScopeCD, "cd:git_push", domain.StepStatusFailed, 10, now),
		{
			ID:             "step-health",
			ReleaseOrderID: order.ID,
			StepScope:      domain.StepScopeCD,
			StepCode:       "cd:health_check",
			StepName:       "cd:health_check",
			Status:         domain.StepStatusRunning,
			SortNo:         11,
			CreatedAt:      now,
			StartedAt:      &startedAt,
		},
		{
			ID:             "step-hook",
			ReleaseOrderID: order.ID,
			StepScope:      domain.StepScopeGlobal,
			StepCode:       "hook:post_release:webhook_notification:1",
			StepName:       "失败后通知",
			Status:         domain.StepStatusPending,
			SortNo:         12,
			CreatedAt:      now,
		},
		testReleaseStep(order.ID, "step-finish", domain.StepScopeGlobal, "global:release_finish", domain.StepStatusFailed, 99, now),
	}
	if err := repo.Create(ctx, order, executions, nil, steps); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	tracker := NewTrackReleaseExecution(manager, nil)
	tracker.now = func() time.Time { return now }

	updated, skipped, err := tracker.syncOrder(ctx, order)
	if err != nil {
		t.Fatalf("first syncOrder failed: %v", err)
	}
	if skipped {
		t.Fatal("first skipped = true, want false")
	}
	if !updated {
		t.Fatal("first updated = false, want true")
	}

	stored, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.Status != domain.OrderStatusRunning {
		t.Fatalf("stored status after first sync = %s, want %s", stored.Status, domain.OrderStatusRunning)
	}
	storedSteps, err := repo.ListSteps(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListSteps failed: %v", err)
	}
	hookStep := findStepByCode(storedSteps, "hook:post_release:webhook_notification:1")
	if hookStep == nil || hookStep.Status != domain.StepStatusSuccess {
		t.Fatalf("hook step after first sync = %#v, want success", hookStep)
	}
	if webhookCalls != 1 {
		t.Fatalf("webhookCalls after first sync = %d, want 1", webhookCalls)
	}

	updated, skipped, err = tracker.syncOrder(ctx, stored)
	if err != nil {
		t.Fatalf("second syncOrder failed: %v", err)
	}
	if skipped {
		t.Fatal("second skipped = true, want false")
	}

	stored, err = repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.Status != domain.OrderStatusFailed {
		t.Fatalf("stored status after second sync = %s, want %s", stored.Status, domain.OrderStatusFailed)
	}
}

// TestDeployDispatchFromBuiltWaitingDeploy 封装当前模块的业务处理逻辑。
func TestDeployDispatchFromBuiltWaitingDeploy(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	startedAt := now.Add(-3 * time.Minute)
	manager.now = func() time.Time { return now }
	manager.jenkins = segmentedReleaseNoopJenkinsExecutor{}

	order := testReleaseOrder("ro-deploy", "RO-DEPLOY", domain.OrderStatusBuiltWaitingDeploy, now)
	order.StartedAt = &startedAt
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}
	if err := repo.Create(ctx, order, executions, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	updated, err := manager.Deploy(ctx, order.ID, "", "")
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	if updated.Status != domain.OrderStatusDeploying {
		t.Fatalf("updated status = %s, want %s", updated.Status, domain.OrderStatusDeploying)
	}
	if updated.StartedAt == nil || !updated.StartedAt.Equal(startedAt) {
		t.Fatalf("updated started_at = %#v, want %v", updated.StartedAt, startedAt)
	}
}

// TestDeployReturnsSuccessWhenReloadFailsAfterDispatch 封装当前模块的业务处理逻辑。
func TestDeployReturnsSuccessWhenReloadFailsAfterDispatch(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	startedAt := now.Add(-3 * time.Minute)
	manager.now = func() time.Time { return now }
	manager.jenkins = segmentedReleaseNoopJenkinsExecutor{}
	manager.repo = &segmentedReleaseFlakyGetByIDRepo{
		Repository: repo,
		failOnCall: 2,
	}

	order := testReleaseOrder("ro-deploy-reload-fail", "RO-DEPLOY-RELOAD-FAIL", domain.OrderStatusBuiltWaitingDeploy, now)
	order.StartedAt = &startedAt
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}
	if err := repo.Create(ctx, order, executions, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	updated, err := manager.Deploy(ctx, order.ID, "", "")
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	if updated.Status != domain.OrderStatusDeploying {
		t.Fatalf("updated status = %s, want %s", updated.Status, domain.OrderStatusDeploying)
	}

	stored, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.Status != domain.OrderStatusDeploying {
		t.Fatalf("stored status = %s, want %s", stored.Status, domain.OrderStatusDeploying)
	}
}

// TestStartNextPendingExecutionClaimsPendingExecutionOnce 封装当前模块的业务处理逻辑。
func TestStartNextPendingExecutionClaimsPendingExecutionOnce(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	jenkins := &segmentedReleaseCountingJenkinsExecutor{}
	manager.jenkins = jenkins
	manager.pipelineRepo = segmentedReleasePipelineRepo{}

	order := testReleaseOrder("ro-claim-once", "RO-CLAIM-ONCE", domain.OrderStatusPending, now)
	order.TemplateID = ""
	order.TemplateName = ""
	executions := []domain.ReleaseOrderExecution{
		{
			ID:             "exec-ci",
			ReleaseOrderID: order.ID,
			PipelineScope:  domain.PipelineScopeCI,
			Provider:       "jenkins",
			PipelineID:     "pipeline-ci",
			Status:         domain.ExecutionStatusPending,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
	if err := repo.Create(ctx, order, executions, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := manager.startNextPendingExecution(ctx, order, executions, nil); err != nil {
		t.Fatalf("first startNextPendingExecution failed: %v", err)
	}
	if err := manager.startNextPendingExecution(ctx, order, executions, nil); err != nil {
		t.Fatalf("second startNextPendingExecution failed: %v", err)
	}
	if jenkins.triggerCount != 1 {
		t.Fatalf("triggerCount = %d, want 1", jenkins.triggerCount)
	}

	storedExecutions, err := repo.ListExecutions(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListExecutions failed: %v", err)
	}
	running := findExecutionByScopeAndStatus(storedExecutions, domain.PipelineScopeCI, domain.ExecutionStatusRunning)
	if running == nil {
		t.Fatalf("running execution not found: %#v", storedExecutions)
	}
	if running.QueueURL != "queue-1" {
		t.Fatalf("running queue_url = %q, want %q", running.QueueURL, "queue-1")
	}
}

// TestDeployReturnsSuccessWhenUpdateStatusReloadFails 更新业务资源并返回处理结果。
func TestDeployReturnsSuccessWhenUpdateStatusReloadFails(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	startedAt := now.Add(-3 * time.Minute)
	manager.now = func() time.Time { return now }
	manager.jenkins = segmentedReleaseNoopJenkinsExecutor{}
	manager.repo = &segmentedReleaseDispatchFallbackRepo{
		Repository:          repo,
		failUpdateStatusHit: true,
	}

	order := testReleaseOrder("ro-deploy-status-reload-fail", "RO-DEPLOY-STATUS-RELOAD-FAIL", domain.OrderStatusBuiltWaitingDeploy, now)
	order.StartedAt = &startedAt
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}
	if err := repo.Create(ctx, order, executions, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	updated, err := manager.Deploy(ctx, order.ID, "u-1", "tester")
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	if updated.Status != domain.OrderStatusDeploying {
		t.Fatalf("updated status = %s, want %s", updated.Status, domain.OrderStatusDeploying)
	}
	if updated.ExecutorUserID != "u-1" {
		t.Fatalf("updated executor_user_id = %q, want %q", updated.ExecutorUserID, "u-1")
	}
}

// TestDeployReturnsSuccessWhenUpdateExecutorReloadFails 更新业务资源并返回处理结果。
func TestDeployReturnsSuccessWhenUpdateExecutorReloadFails(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	startedAt := now.Add(-3 * time.Minute)
	manager.now = func() time.Time { return now }
	manager.jenkins = segmentedReleaseNoopJenkinsExecutor{}
	manager.repo = &segmentedReleaseDispatchFallbackRepo{
		Repository:            repo,
		failUpdateExecutorHit: true,
	}

	order := testReleaseOrder("ro-deploy-executor-reload-fail", "RO-DEPLOY-EXECUTOR-RELOAD-FAIL", domain.OrderStatusBuiltWaitingDeploy, now)
	order.StartedAt = &startedAt
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}
	if err := repo.Create(ctx, order, executions, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	updated, err := manager.Deploy(ctx, order.ID, "u-2", "tester-2")
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	if updated.Status != domain.OrderStatusDeploying {
		t.Fatalf("updated status = %s, want %s", updated.Status, domain.OrderStatusDeploying)
	}
	if updated.ExecutorUserID != "u-2" {
		t.Fatalf("updated executor_user_id = %q, want %q", updated.ExecutorUserID, "u-2")
	}
	if updated.ExecutorName != "tester-2" {
		t.Fatalf("updated executor_name = %q, want %q", updated.ExecutorName, "tester-2")
	}
}

// TestBuildSuccessWaitsForBuildCompleteHook 组装业务执行所需的输入数据。
func TestBuildSuccessWaitsForBuildCompleteHook(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	startedAt := now.Add(-2 * time.Minute)
	manager.now = func() time.Time { return now }
	manager.jenkins = segmentedReleaseNoopJenkinsExecutor{}

	template := domain.ReleaseTemplate{
		ID:              "rt-1",
		Name:            "Template 1",
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
			ID:               "hook-build-complete",
			TemplateID:       template.ID,
			HookType:         domain.TemplateHookTypeWebhookNotification,
			Name:             "构建完成通知",
			ExecuteStage:     domain.TemplateHookExecuteStageBuildComplete,
			TriggerCondition: domain.TemplateHookTriggerOnSuccess,
			FailurePolicy:    domain.TemplateHookFailurePolicyBlockRelease,
			WebhookMethod:    http.MethodPost,
			WebhookURL:       server.URL,
			WebhookBody:      `{"order_no":"{order_no}"}`,
			SortNo:           1,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}
	if err := repo.CreateTemplate(ctx, template, nil, nil, nil, hooks); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	order := testReleaseOrder("ro-build-hook", "RO-BUILD-HOOK", domain.OrderStatusBuilding, now)
	order.StartedAt = &startedAt
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}
	steps := defaultReleaseOrderSteps(order.ID, executions, now, "", hooks, order.EnvCode)
	if err := repo.Create(ctx, order, executions, nil, steps); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	tracker := NewTrackReleaseExecution(manager, nil)
	tracker.now = func() time.Time { return now }
	updated, err := tracker.syncNextStepAfterExecution(ctx, order)
	if err != nil {
		t.Fatalf("first syncNextStepAfterExecution failed: %v", err)
	}
	if !updated {
		t.Fatal("first updated = false, want true")
	}

	stored, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.Status != domain.OrderStatusBuilding {
		t.Fatalf("stored status after first sync = %s, want %s", stored.Status, domain.OrderStatusBuilding)
	}

	updated, err = tracker.syncNextStepAfterExecution(ctx, stored)
	if err != nil {
		t.Fatalf("second syncNextStepAfterExecution failed: %v", err)
	}
	if !updated {
		t.Fatal("second updated = false, want true")
	}

	stored, err = repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.Status != domain.OrderStatusBuiltWaitingDeploy {
		t.Fatalf("stored status = %s, want %s", stored.Status, domain.OrderStatusBuiltWaitingDeploy)
	}
}

// TestDefaultReleaseOrderStepsCreatesHookStepForEachSelectedStage 创建业务资源并返回处理结果。
func TestDefaultReleaseOrderStepsCreatesHookStepForEachSelectedStage(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	orderID := "ro-hook-multi-stage"
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(orderID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
		testReleaseExecution(orderID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}
	hooks := []domain.ReleaseTemplateHook{
		{
			ID:               "hook-1",
			TemplateID:       "rt-1",
			HookType:         domain.TemplateHookTypeNotificationHook,
			Name:             "多阶段通知",
			ExecuteStage:     domain.TemplateHookExecuteStagePostRelease,
			ExecuteStages:    []domain.TemplateHookExecuteStage{domain.TemplateHookExecuteStageBuildComplete, domain.TemplateHookExecuteStagePostRelease},
			TriggerCondition: domain.TemplateHookTriggerOnSuccess,
			FailurePolicy:    domain.TemplateHookFailurePolicyWarnOnly,
			SortNo:           1,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}

	steps := defaultReleaseOrderSteps(orderID, executions, now, "", hooks, "prod")
	buildHookCount := 0
	postReleaseHookCount := 0
	for _, item := range steps {
		switch item.StepCode {
		case "hook:build_complete:notification_hook:1":
			buildHookCount++
		case "hook:post_release:notification_hook:1":
			postReleaseHookCount++
		}
	}
	if buildHookCount != 1 {
		t.Fatalf("build_complete hook step count = %d, want 1", buildHookCount)
	}
	if postReleaseHookCount != 1 {
		t.Fatalf("post_release hook step count = %d, want 1", postReleaseHookCount)
	}
}

type segmentedReleaseNoopJenkinsExecutor struct{}

type segmentedReleaseCountingJenkinsExecutor struct {
	triggerCount int
}

type segmentedReleasePipelineRepo struct{}

// InitSchema 封装当前模块的业务处理逻辑。
func (segmentedReleasePipelineRepo) InitSchema(context.Context) error { return nil }

// UpsertPipelines 封装当前模块的业务处理逻辑。
func (segmentedReleasePipelineRepo) UpsertPipelines(context.Context, []pipelinedomain.Pipeline) (int, int, error) {
	return 0, 0, nil
}

// MarkMissingPipelinesInactive 封装当前模块的业务处理逻辑。
func (segmentedReleasePipelineRepo) MarkMissingPipelinesInactive(context.Context, pipelinedomain.Provider, []string, time.Time) (int, error) {
	return 0, nil
}

// ListPipelines 查询并返回列表数据。
func (segmentedReleasePipelineRepo) ListPipelines(context.Context, pipelinedomain.PipelineListFilter) ([]pipelinedomain.Pipeline, int64, error) {
	return nil, 0, nil
}

// GetPipelineByID 查询并返回指定资源数据。
func (segmentedReleasePipelineRepo) GetPipelineByID(context.Context, string) (pipelinedomain.Pipeline, error) {
	return pipelinedomain.Pipeline{
		ID:          "pipeline-ci",
		Provider:    pipelinedomain.ProviderJenkins,
		JobFullName: "demo/job",
		Status:      pipelinedomain.StatusActive,
	}, nil
}

// MarkPipelineVerified 封装当前模块的业务处理逻辑。
func (segmentedReleasePipelineRepo) MarkPipelineVerified(context.Context, string, time.Time, time.Time) (pipelinedomain.Pipeline, error) {
	return pipelinedomain.Pipeline{}, nil
}

// CreateBinding 创建业务资源并返回处理结果。
func (segmentedReleasePipelineRepo) CreateBinding(context.Context, pipelinedomain.PipelineBinding) error {
	return nil
}

// ListBindingsByApplication 查询并返回列表数据。
func (segmentedReleasePipelineRepo) ListBindingsByApplication(context.Context, pipelinedomain.BindingListFilter) ([]pipelinedomain.PipelineBinding, int64, error) {
	return nil, 0, nil
}

// GetBindingByID 查询并返回指定资源数据。
func (segmentedReleasePipelineRepo) GetBindingByID(context.Context, string) (pipelinedomain.PipelineBinding, error) {
	return pipelinedomain.PipelineBinding{}, pipelinedomain.ErrBindingNotFound
}

// UpdateBinding 更新业务资源并返回处理结果。
func (segmentedReleasePipelineRepo) UpdateBinding(context.Context, string, pipelinedomain.BindingUpdateInput, time.Time) (pipelinedomain.PipelineBinding, error) {
	return pipelinedomain.PipelineBinding{}, nil
}

// DeleteBinding 删除业务资源并返回处理结果。
func (segmentedReleasePipelineRepo) DeleteBinding(context.Context, string) error { return nil }

type segmentedReleaseFlakyGetByIDRepo struct {
	domain.Repository
	getByIDCalls int
	failOnCall   int
}

// GetByID 查询并返回指定资源数据。
func (r *segmentedReleaseFlakyGetByIDRepo) GetByID(ctx context.Context, id string) (domain.ReleaseOrder, error) {
	r.getByIDCalls++
	if r.failOnCall > 0 && r.getByIDCalls == r.failOnCall {
		return domain.ReleaseOrder{}, domain.ErrOrderNotFound
	}
	return r.Repository.GetByID(ctx, id)
}

type segmentedReleaseDispatchFallbackRepo struct {
	domain.Repository
	failUpdateStatusHit   bool
	failUpdateExecutorHit bool
}

// UpdateStatus 更新业务资源并返回处理结果。
func (r *segmentedReleaseDispatchFallbackRepo) UpdateStatus(
	ctx context.Context,
	id string,
	status domain.OrderStatus,
	startedAt *time.Time,
	finishedAt *time.Time,
	updatedAt time.Time,
) (domain.ReleaseOrder, error) {
	item, err := r.Repository.UpdateStatus(ctx, id, status, startedAt, finishedAt, updatedAt)
	if err != nil {
		return item, err
	}
	if r.failUpdateStatusHit {
		r.failUpdateStatusHit = false
		return domain.ReleaseOrder{}, domain.ErrOrderNotFound
	}
	return item, nil
}

// UpdateExecutor 更新业务资源并返回处理结果。
func (r *segmentedReleaseDispatchFallbackRepo) UpdateExecutor(
	ctx context.Context,
	id string,
	executorUserID string,
	executorName string,
	updatedAt time.Time,
) (domain.ReleaseOrder, error) {
	item, err := r.Repository.UpdateExecutor(ctx, id, executorUserID, executorName, updatedAt)
	if err != nil {
		return item, err
	}
	if r.failUpdateExecutorHit {
		r.failUpdateExecutorHit = false
		return domain.ReleaseOrder{}, domain.ErrOrderNotFound
	}
	return item, nil
}

// TriggerBuild 组装业务执行所需的输入数据。
func (segmentedReleaseNoopJenkinsExecutor) TriggerBuild(context.Context, string, map[string]string) (string, error) {
	return "", nil
}

// TriggerBuild 组装业务执行所需的输入数据。
func (e *segmentedReleaseCountingJenkinsExecutor) TriggerBuild(context.Context, string, map[string]string) (string, error) {
	e.triggerCount++
	return "queue-1", nil
}

// GetQueueItem 查询并返回指定资源数据。
func (segmentedReleaseNoopJenkinsExecutor) GetQueueItem(context.Context, string) (string, bool, string, error) {
	return "", false, "", nil
}

// GetQueueItem 查询并返回指定资源数据。
func (*segmentedReleaseCountingJenkinsExecutor) GetQueueItem(context.Context, string) (string, bool, string, error) {
	return "", false, "", nil
}

// AbortQueueItem 封装当前模块的业务处理逻辑。
func (segmentedReleaseNoopJenkinsExecutor) AbortQueueItem(context.Context, string) error {
	return nil
}

// AbortQueueItem 封装当前模块的业务处理逻辑。
func (*segmentedReleaseCountingJenkinsExecutor) AbortQueueItem(context.Context, string) error {
	return nil
}

// AbortBuild 组装业务执行所需的输入数据。
func (segmentedReleaseNoopJenkinsExecutor) AbortBuild(context.Context, string) error {
	return nil
}

// AbortBuild 组装业务执行所需的输入数据。
func (*segmentedReleaseCountingJenkinsExecutor) AbortBuild(context.Context, string) error {
	return nil
}

// GetBuildStages 组装业务执行所需的输入数据。
func (segmentedReleaseNoopJenkinsExecutor) GetBuildStages(context.Context, string) ([]domain.ReleaseOrderPipelineStage, error) {
	return nil, nil
}

// GetBuildStages 组装业务执行所需的输入数据。
func (*segmentedReleaseCountingJenkinsExecutor) GetBuildStages(context.Context, string) ([]domain.ReleaseOrderPipelineStage, error) {
	return nil, nil
}

// GetBuildStageLog 组装业务执行所需的输入数据。
func (segmentedReleaseNoopJenkinsExecutor) GetBuildStageLog(context.Context, string, string) (domain.ReleaseOrderPipelineStageLog, error) {
	return domain.ReleaseOrderPipelineStageLog{}, nil
}

// GetBuildStageLog 组装业务执行所需的输入数据。
func (*segmentedReleaseCountingJenkinsExecutor) GetBuildStageLog(context.Context, string, string) (domain.ReleaseOrderPipelineStageLog, error) {
	return domain.ReleaseOrderPipelineStageLog{}, nil
}
