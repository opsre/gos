package usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	domain "gos/internal/domain/release"
)

// TestCreateStandardRollbackByOrderPreservesTemplateHooks 创建业务资源并返回处理结果。
func TestCreateStandardRollbackByOrderPreservesTemplateHooks(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }

	template := domain.ReleaseTemplate{
		ID:              "rt-rollback",
		Name:            "rollback-template",
		ApplicationID:   "app-1",
		ApplicationName: "App 1",
		BindingID:       "app-1",
		BindingName:     "App 1",
		BindingType:     "application",
		GitOpsType:      domain.GitOpsTypeHelm,
		Status:          domain.TemplateStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	bindings := []domain.ReleaseTemplateBinding{
		{
			ID:            "rtb-cd",
			TemplateID:    template.ID,
			PipelineScope: domain.PipelineScopeCD,
			BindingID:     "binding-cd",
			BindingName:   "ArgoCD",
			Provider:      "argocd",
			PipelineID:    "argocd-app",
			Enabled:       true,
			SortNo:        1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	hooks := []domain.ReleaseTemplateHook{
		{
			ID:               "hook-1",
			TemplateID:       template.ID,
			HookType:         domain.TemplateHookTypeWebhookNotification,
			Name:             "rollback notify",
			TriggerCondition: domain.TemplateHookTriggerAlways,
			FailurePolicy:    domain.TemplateHookFailurePolicyWarnOnly,
			WebhookMethod:    "POST",
			WebhookURL:       "https://example.com/hook",
			WebhookBody:      "{}",
			SortNo:           1,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}
	if err := repo.CreateTemplate(ctx, template, bindings, nil, nil, hooks); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	sourceOrder := testReleaseOrder("ro-source", "RO-SOURCE", domain.OrderStatusSuccess, now)
	sourceOrder.TemplateID = template.ID
	sourceOrder.TemplateName = template.Name
	sourceOrder.BindingID = "binding-cd"
	sourceExecution := domain.ReleaseOrderExecution{
		ID:             "exec-source-cd",
		ReleaseOrderID: sourceOrder.ID,
		PipelineScope:  domain.PipelineScopeCD,
		BindingID:      "binding-cd",
		BindingName:    "ArgoCD",
		Provider:       "argocd",
		PipelineID:     "argocd-app",
		Status:         domain.ExecutionStatusSuccess,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.Create(ctx, sourceOrder, []domain.ReleaseOrderExecution{sourceExecution}, nil, nil); err != nil {
		t.Fatalf("Create source order failed: %v", err)
	}
	if err := repo.CreateDeploySnapshot(ctx, domain.DeploySnapshot{
		ID:              "snapshot-1",
		ReleaseOrderID:  sourceOrder.ID,
		Provider:        "argocd",
		GitOpsType:      domain.GitOpsTypeHelm,
		RepoURL:         "https://example.com/repo.git",
		Branch:          "demo-prod",
		SourcePath:      "apps/demo/helm",
		EnvCode:         "prod",
		SnapshotPayload: `{"image_version":"175"}`,
		CreatedAt:       now,
	}); err != nil {
		t.Fatalf("CreateDeploySnapshot failed: %v", err)
	}

	rollbackOrder, err := manager.CreateStandardRollbackByOrder(ctx, sourceOrder.ID, "tester", "tester")
	if err != nil {
		t.Fatalf("CreateStandardRollbackByOrder failed: %v", err)
	}

	steps, err := repo.ListSteps(ctx, rollbackOrder.ID)
	if err != nil {
		t.Fatalf("ListSteps failed: %v", err)
	}

	foundHook := false
	for _, step := range steps {
		if step.StepCode == "hook:post_release:webhook_notification:1" || step.StepCode == "hook:webhook_notification:1" {
			foundHook = true
			if step.StepName != "rollback notify" {
				t.Fatalf("hook step name = %q, want %q", step.StepName, "rollback notify")
			}
		}
	}
	if !foundHook {
		t.Fatalf("expected rollback order %s to preserve hook step, got steps: %#v", rollbackOrder.ID, steps)
	}
}

// TestCreateStandardRollbackByOrderAllowsDeployFailedSource 创建业务资源并返回处理结果。
func TestCreateStandardRollbackByOrderAllowsDeployFailedSource(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }

	template := domain.ReleaseTemplate{
		ID:              "rt-rollback-allow-failed",
		Name:            "rollback-template",
		ApplicationID:   "app-1",
		ApplicationName: "App 1",
		BindingID:       "app-1",
		BindingName:     "App 1",
		BindingType:     "application",
		GitOpsType:      domain.GitOpsTypeHelm,
		Status:          domain.TemplateStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	bindings := []domain.ReleaseTemplateBinding{
		{
			ID:            "rtb-cd-failed",
			TemplateID:    template.ID,
			PipelineScope: domain.PipelineScopeCD,
			BindingID:     "binding-cd",
			BindingName:   "ArgoCD",
			Provider:      "argocd",
			PipelineID:    "argocd-app",
			Enabled:       true,
			SortNo:        1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	if err := repo.CreateTemplate(ctx, template, bindings, nil, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	sourceOrder := testReleaseOrder("ro-source-failed", "RO-SOURCE-FAILED", domain.OrderStatusDeployFailed, now)
	sourceOrder.TemplateID = template.ID
	sourceOrder.TemplateName = template.Name
	sourceOrder.BindingID = "binding-cd"
	sourceExecution := domain.ReleaseOrderExecution{
		ID:             "exec-source-cd-failed",
		ReleaseOrderID: sourceOrder.ID,
		PipelineScope:  domain.PipelineScopeCD,
		BindingID:      "binding-cd",
		BindingName:    "ArgoCD",
		Provider:       "argocd",
		PipelineID:     "argocd-app",
		Status:         domain.ExecutionStatusFailed,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.Create(ctx, sourceOrder, []domain.ReleaseOrderExecution{sourceExecution}, nil, nil); err != nil {
		t.Fatalf("Create source order failed: %v", err)
	}

	created, err := manager.CreateStandardRollbackByOrder(ctx, sourceOrder.ID, "tester", "tester")
	if err != nil {
		t.Fatalf("CreateStandardRollbackByOrder failed: %v", err)
	}
	if created.SourceOrderID != sourceOrder.ID {
		t.Fatalf("SourceOrderID = %s, want %s", created.SourceOrderID, sourceOrder.ID)
	}
	if created.OperationType != domain.OperationTypeRollback {
		t.Fatalf("OperationType = %s, want %s", created.OperationType, domain.OperationTypeRollback)
	}
}

// TestCreatePipelineReplayByOrderAllowsDeployFailedSource 创建业务资源并返回处理结果。
func TestCreatePipelineReplayByOrderAllowsDeployFailedSource(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }

	template := domain.ReleaseTemplate{
		ID:              "rt-replay-allow-failed",
		Name:            "replay-template",
		ApplicationID:   "app-1",
		ApplicationName: "App 1",
		BindingID:       "app-1",
		BindingName:     "App 1",
		BindingType:     "application",
		GitOpsType:      "",
		Status:          domain.TemplateStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	bindings := []domain.ReleaseTemplateBinding{
		{
			ID:            "rtb-ci-replay-failed",
			TemplateID:    template.ID,
			PipelineScope: domain.PipelineScopeCI,
			BindingID:     "binding-ci",
			BindingName:   "Jenkins CI",
			Provider:      "jenkins",
			PipelineID:    "pipeline-ci",
			Enabled:       true,
			SortNo:        1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "rtb-cd-replay-failed",
			TemplateID:    template.ID,
			PipelineScope: domain.PipelineScopeCD,
			BindingID:     "binding-cd",
			BindingName:   "Jenkins CD",
			Provider:      "jenkins",
			PipelineID:    "pipeline-cd",
			Enabled:       true,
			SortNo:        2,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	paramsRule := []domain.ReleaseTemplateParam{
		{
			ID:                 "rtp-ci-branch-replay-failed",
			TemplateID:         template.ID,
			TemplateBindingID:  bindings[0].ID,
			PipelineScope:      domain.PipelineScopeCI,
			BindingID:          bindings[0].BindingID,
			ExecutorParamDefID: "ep-branch-replay-failed",
			ParamKey:           "branch",
			ParamName:          "分支",
			ExecutorParamName:  "BRANCH",
			ValueSource:        domain.TemplateParamValueSourceReleaseInput,
			Required:           true,
			SortNo:             1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 "rtp-cd-env-replay-failed",
			TemplateID:         template.ID,
			TemplateBindingID:  bindings[1].ID,
			PipelineScope:      domain.PipelineScopeCD,
			BindingID:          bindings[1].BindingID,
			ExecutorParamDefID: "ep-env-replay-failed",
			ParamKey:           "deploy_env",
			ParamName:          "部署环境",
			ExecutorParamName:  "DEPLOY_ENV",
			ValueSource:        domain.TemplateParamValueSourceReleaseInput,
			Required:           true,
			SortNo:             1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	if err := repo.CreateTemplate(ctx, template, bindings, paramsRule, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	sourceOrder := testReleaseOrder("ro-replay-source-failed", "RO-REPLAY-SOURCE-FAILED", domain.OrderStatusDeployFailed, now)
	sourceOrder.TemplateID = template.ID
	sourceOrder.TemplateName = template.Name
	sourceOrder.BindingID = bindings[0].BindingID
	sourceExecution := domain.ReleaseOrderExecution{
		ID:             "exec-replay-source-failed-ci",
		ReleaseOrderID: sourceOrder.ID,
		PipelineScope:  domain.PipelineScopeCI,
		BindingID:      bindings[0].BindingID,
		BindingName:    bindings[0].BindingName,
		Provider:       bindings[0].Provider,
		PipelineID:     bindings[0].PipelineID,
		Status:         domain.ExecutionStatusFailed,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	skippedCDExecution := domain.ReleaseOrderExecution{
		ID:             "exec-replay-source-failed-cd-skipped",
		ReleaseOrderID: sourceOrder.ID,
		PipelineScope:  domain.PipelineScopeCD,
		BindingID:      bindings[1].BindingID,
		BindingName:    bindings[1].BindingName,
		Provider:       bindings[1].Provider,
		PipelineID:     bindings[1].PipelineID,
		Status:         domain.ExecutionStatusSkipped,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	sourceParams := []domain.ReleaseOrderParam{
		{
			ID:                "rop-replay-source-failed-branch",
			ReleaseOrderID:    sourceOrder.ID,
			PipelineScope:     domain.PipelineScopeCI,
			BindingID:         bindings[0].BindingID,
			ParamKey:          "branch",
			ExecutorParamName: "BRANCH",
			ParamValue:        "release/failed",
			ValueSource:       domain.ValueSourceReleaseInput,
			CreatedAt:         now,
		},
		{
			ID:                "rop-replay-source-failed-deploy-env",
			ReleaseOrderID:    sourceOrder.ID,
			PipelineScope:     domain.PipelineScopeCD,
			BindingID:         bindings[1].BindingID,
			ParamKey:          "deploy_env",
			ExecutorParamName: "DEPLOY_ENV",
			ParamValue:        "prod",
			ValueSource:       domain.ValueSourceReleaseInput,
			CreatedAt:         now,
		},
	}
	if err := repo.Create(ctx, sourceOrder, []domain.ReleaseOrderExecution{sourceExecution, skippedCDExecution}, sourceParams, nil); err != nil {
		t.Fatalf("Create source order failed: %v", err)
	}

	created, err := manager.CreatePipelineReplayByOrder(ctx, sourceOrder.ID, "tester", "tester")
	if err != nil {
		t.Fatalf("CreatePipelineReplayByOrder failed: %v", err)
	}
	if created.SourceOrderID != sourceOrder.ID {
		t.Fatalf("SourceOrderID = %s, want %s", created.SourceOrderID, sourceOrder.ID)
	}
	if created.OperationType != domain.OperationTypeReplay {
		t.Fatalf("OperationType = %s, want %s", created.OperationType, domain.OperationTypeReplay)
	}
	createdExecutions, err := repo.ListExecutions(ctx, created.ID)
	if err != nil {
		t.Fatalf("ListExecutions failed: %v", err)
	}
	if len(createdExecutions) != 2 ||
		findExecutionByScopeAndStatus(createdExecutions, domain.PipelineScopeCI, domain.ExecutionStatusPending) == nil ||
		findExecutionByScopeAndStatus(createdExecutions, domain.PipelineScopeCD, domain.ExecutionStatusPending) == nil {
		t.Fatalf("created executions=%#v, want pending CI and downstream CD replay", createdExecutions)
	}
	createdParams, err := repo.ListParams(ctx, created.ID)
	if err != nil {
		t.Fatalf("ListParams failed: %v", err)
	}
	if findReleaseParamValue(createdParams, domain.PipelineScopeCI, "branch") != "release/failed" ||
		findReleaseParamValue(createdParams, domain.PipelineScopeCD, "deploy_env") != "prod" {
		t.Fatalf("created params=%#v, want preserved CI and CD snapshots", createdParams)
	}
	createdSteps, err := repo.ListSteps(ctx, created.ID)
	if err != nil {
		t.Fatalf("ListSteps failed: %v", err)
	}
	if findStepByCode(createdSteps, "cd:trigger_pipeline") == nil {
		t.Fatalf("created steps=%#v, want downstream CD steps", createdSteps)
	}
}

func TestCreatePipelineReplayByOrderRetriesFailedCDWithoutRebuildingCI(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }

	template := domain.ReleaseTemplate{
		ID:              "rt-replay-cd-failed",
		Name:            "replay-cd-template",
		ApplicationID:   "app-1",
		ApplicationName: "App 1",
		BindingID:       "app-1",
		BindingName:     "App 1",
		BindingType:     "application",
		Status:          domain.TemplateStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	bindings := []domain.ReleaseTemplateBinding{
		{
			ID: "rtb-ci-replay-cd-failed", TemplateID: template.ID,
			PipelineScope: domain.PipelineScopeCI, BindingID: "binding-ci", BindingName: "Jenkins CI",
			Provider: "jenkins", PipelineID: "pipeline-ci", Enabled: true, SortNo: 1, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "rtb-cd-replay-cd-failed", TemplateID: template.ID,
			PipelineScope: domain.PipelineScopeCD, BindingID: "binding-cd", BindingName: "Jenkins CD",
			Provider: "jenkins", PipelineID: "pipeline-cd", Enabled: true, SortNo: 2, CreatedAt: now, UpdatedAt: now,
		},
	}
	templateParams := []domain.ReleaseTemplateParam{
		{
			ID: "rtp-cd-env-replay-cd-failed", TemplateID: template.ID, TemplateBindingID: bindings[1].ID,
			PipelineScope: domain.PipelineScopeCD, BindingID: bindings[1].BindingID,
			ParamKey: "deploy_env", ParamName: "部署环境", ExecutorParamName: "DEPLOY_ENV",
			ValueSource: domain.TemplateParamValueSourceReleaseInput, Required: true, SortNo: 1,
			CreatedAt: now, UpdatedAt: now,
		},
	}
	if err := repo.CreateTemplate(ctx, template, bindings, templateParams, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	sourceOrder := testReleaseOrder("ro-replay-source-cd-failed", "RO-REPLAY-SOURCE-CD-FAILED", domain.OrderStatusDeployFailed, now)
	sourceOrder.TemplateID = template.ID
	sourceOrder.TemplateName = template.Name
	sourceOrder.BindingID = bindings[0].BindingID
	ciExecution := domain.ReleaseOrderExecution{
		ID: "exec-replay-source-ci-success", ReleaseOrderID: sourceOrder.ID,
		PipelineScope: domain.PipelineScopeCI, BindingID: bindings[0].BindingID, BindingName: bindings[0].BindingName,
		Provider: bindings[0].Provider, PipelineID: bindings[0].PipelineID, Status: domain.ExecutionStatusSuccess,
		BuildURL: "https://jenkins.example/job/CI_FRONT/8/", CreatedAt: now, UpdatedAt: now,
	}
	cdExecution := domain.ReleaseOrderExecution{
		ID: "exec-replay-source-cd-failed", ReleaseOrderID: sourceOrder.ID,
		PipelineScope: domain.PipelineScopeCD, BindingID: bindings[1].BindingID, BindingName: bindings[1].BindingName,
		Provider: bindings[1].Provider, PipelineID: bindings[1].PipelineID, Status: domain.ExecutionStatusFailed,
		CreatedAt: now, UpdatedAt: now,
	}
	sourceParams := []domain.ReleaseOrderParam{
		{
			ID: "rop-replay-source-cd-env", ReleaseOrderID: sourceOrder.ID,
			PipelineScope: domain.PipelineScopeCD, BindingID: bindings[1].BindingID,
			ParamKey: "deploy_env", ExecutorParamName: "DEPLOY_ENV", ParamValue: "prod",
			ValueSource: domain.ValueSourceReleaseInput, CreatedAt: now,
		},
	}
	if err := repo.Create(ctx, sourceOrder, []domain.ReleaseOrderExecution{ciExecution, cdExecution}, sourceParams, nil); err != nil {
		t.Fatalf("Create source order failed: %v", err)
	}

	created, err := manager.CreatePipelineReplayByOrder(ctx, sourceOrder.ID, "tester", "tester")
	if err != nil {
		t.Fatalf("CreatePipelineReplayByOrder failed: %v", err)
	}
	createdExecutions, err := repo.ListExecutions(ctx, created.ID)
	if err != nil {
		t.Fatalf("ListExecutions failed: %v", err)
	}
	if findExecutionByScopeAndStatus(createdExecutions, domain.PipelineScopeCD, domain.ExecutionStatusPending) == nil {
		t.Fatalf("created executions=%#v, want pending CD replay", createdExecutions)
	}
	if hasExecutionForScope(createdExecutions, domain.PipelineScopeCI) {
		t.Fatalf("created executions=%#v, failed CD replay must not rebuild CI", createdExecutions)
	}
}

// TestCreatePipelineReplayByOrderRejectsReplaySourceBeforeParamValidation 创建业务资源并返回处理结果。
func TestCreatePipelineReplayByOrderRejectsReplaySourceBeforeParamValidation(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }

	template := domain.ReleaseTemplate{
		ID:              "rt-replay-source-replay",
		Name:            "replay-template",
		ApplicationID:   "app-1",
		ApplicationName: "App 1",
		BindingID:       "app-1",
		BindingName:     "App 1",
		BindingType:     "application",
		Status:          domain.TemplateStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	bindings := []domain.ReleaseTemplateBinding{
		{
			ID:            "rtb-ci-replay-source-replay",
			TemplateID:    template.ID,
			PipelineScope: domain.PipelineScopeCI,
			BindingID:     "binding-ci",
			BindingName:   "Jenkins CI",
			Provider:      "jenkins",
			PipelineID:    "pipeline-ci",
			Enabled:       true,
			SortNo:        1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	paramsRule := []domain.ReleaseTemplateParam{
		{
			ID:                 "rtp-ci-branch-replay-source-replay",
			TemplateID:         template.ID,
			TemplateBindingID:  bindings[0].ID,
			PipelineScope:      domain.PipelineScopeCI,
			BindingID:          bindings[0].BindingID,
			ExecutorParamDefID: "ep-branch-replay-source-replay",
			ParamKey:           "branch",
			ParamName:          "分支",
			ExecutorParamName:  "BRANCH",
			ValueSource:        domain.TemplateParamValueSourceReleaseInput,
			Required:           true,
			SortNo:             1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	if err := repo.CreateTemplate(ctx, template, bindings, paramsRule, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	sourceOrder := testReleaseOrder("ro-replay-source-replay", "RO-REPLAY-SOURCE-REPLAY", domain.OrderStatusSuccess, now)
	sourceOrder.OperationType = domain.OperationTypeReplay
	sourceOrder.TemplateID = template.ID
	sourceOrder.TemplateName = template.Name
	sourceOrder.BindingID = bindings[0].BindingID
	sourceExecution := domain.ReleaseOrderExecution{
		ID:             "exec-replay-source-replay-ci",
		ReleaseOrderID: sourceOrder.ID,
		PipelineScope:  domain.PipelineScopeCI,
		BindingID:      bindings[0].BindingID,
		BindingName:    bindings[0].BindingName,
		Provider:       bindings[0].Provider,
		PipelineID:     bindings[0].PipelineID,
		Status:         domain.ExecutionStatusSuccess,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	sourceParams := []domain.ReleaseOrderParam{
		{
			ID:                "rop-replay-source-replay-branch",
			ReleaseOrderID:    sourceOrder.ID,
			PipelineScope:     domain.PipelineScopeCI,
			BindingID:         bindings[0].BindingID,
			ParamKey:          "branch",
			ExecutorParamName: "BRANCH",
			ParamValue:        "release/source-replay",
			ValueSource:       domain.ValueSourceReleaseInput,
			CreatedAt:         now,
		},
		{
			ID:                "rop-replay-source-replay-app-key",
			ReleaseOrderID:    sourceOrder.ID,
			PipelineScope:     domain.PipelineScopeCI,
			BindingID:         bindings[0].BindingID,
			ParamKey:          "app_key",
			ExecutorParamName: "app_key",
			ParamValue:        "demo-app",
			ValueSource:       domain.ValueSourceApplication,
			CreatedAt:         now,
		},
	}
	if err := repo.Create(ctx, sourceOrder, []domain.ReleaseOrderExecution{sourceExecution}, sourceParams, nil); err != nil {
		t.Fatalf("Create source order failed: %v", err)
	}

	_, err := manager.CreatePipelineReplayByOrder(ctx, sourceOrder.ID, "tester", "tester")
	if err == nil {
		t.Fatalf("CreatePipelineReplayByOrder should fail")
	}
	if !strings.Contains(err.Error(), "重放单不支持再次重放，继续重发请从原始单发起") {
		t.Fatalf("error = %q, want replay-source guard", err.Error())
	}
	if strings.Contains(err.Error(), "当前模板已不再包含") {
		t.Fatalf("error = %q, should not expose template param validation", err.Error())
	}
}
