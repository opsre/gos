package usecase

import (
	"context"
	"testing"
	"time"

	appdomain "gos/internal/domain/application"
	domain "gos/internal/domain/release"
)

// TestUpdatePendingReleaseOrderRebuildsSnapshot 组装业务执行所需的输入数据。
func TestUpdatePendingReleaseOrderRebuildsSnapshot(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	manager.appRepo = releaseOrderUpdateApplicationRepoStub{
		app: appdomain.Application{
			ID:     "app-1",
			Name:   "App 1",
			Key:    "app-1",
			Status: appdomain.StatusActive,
		},
	}

	oldTemplate := domain.ReleaseTemplate{
		ID:              "rt-old",
		Name:            "old-template",
		ApplicationID:   "app-1",
		ApplicationName: "App 1",
		BindingID:       "app-1",
		BindingName:     "App 1",
		BindingType:     "application",
		Status:          domain.TemplateStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	oldBindings := []domain.ReleaseTemplateBinding{
		{
			ID:            "rtb-old-ci",
			TemplateID:    oldTemplate.ID,
			PipelineScope: domain.PipelineScopeCI,
			BindingID:     "binding-old-ci",
			BindingName:   "Old CI",
			Provider:      "jenkins",
			PipelineID:    "pipeline-old-ci",
			Enabled:       true,
			SortNo:        1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	oldParams := []domain.ReleaseTemplateParam{
		{
			ID:                 "rtp-old-branch",
			TemplateID:         oldTemplate.ID,
			TemplateBindingID:  oldBindings[0].ID,
			PipelineScope:      domain.PipelineScopeCI,
			BindingID:          oldBindings[0].BindingID,
			ExecutorParamDefID: "ep-old-branch",
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
	if err := repo.CreateTemplate(ctx, oldTemplate, oldBindings, oldParams, nil, nil); err != nil {
		t.Fatalf("CreateTemplate old failed: %v", err)
	}

	newTemplate := domain.ReleaseTemplate{
		ID:              "rt-new",
		Name:            "new-template",
		ApplicationID:   "app-1",
		ApplicationName: "App 1",
		BindingID:       "app-1",
		BindingName:     "App 1",
		BindingType:     "application",
		Status:          domain.TemplateStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	newBindings := []domain.ReleaseTemplateBinding{
		{
			ID:            "rtb-new-ci",
			TemplateID:    newTemplate.ID,
			PipelineScope: domain.PipelineScopeCI,
			BindingID:     "binding-new-ci",
			BindingName:   "New CI",
			Provider:      "jenkins",
			PipelineID:    "pipeline-new-ci",
			Enabled:       true,
			SortNo:        1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	newParams := []domain.ReleaseTemplateParam{
		{
			ID:                 "rtp-new-branch",
			TemplateID:         newTemplate.ID,
			TemplateBindingID:  newBindings[0].ID,
			PipelineScope:      domain.PipelineScopeCI,
			BindingID:          newBindings[0].BindingID,
			ExecutorParamDefID: "ep-new-branch",
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
	if err := repo.CreateTemplate(ctx, newTemplate, newBindings, newParams, nil, nil); err != nil {
		t.Fatalf("CreateTemplate new failed: %v", err)
	}

	order := testReleaseOrder("ro-update", "RO-UPDATE", domain.OrderStatusPending, now)
	order.TemplateID = oldTemplate.ID
	order.TemplateName = oldTemplate.Name
	order.BindingID = oldBindings[0].BindingID
	order.PipelineID = oldBindings[0].PipelineID
	order.ReleaseName = "before name"
	order.Remark = "before update"
	executions := []domain.ReleaseOrderExecution{
		{
			ID:             "exec-old-ci",
			ReleaseOrderID: order.ID,
			PipelineScope:  domain.PipelineScopeCI,
			BindingID:      oldBindings[0].BindingID,
			BindingName:    oldBindings[0].BindingName,
			Provider:       oldBindings[0].Provider,
			PipelineID:     oldBindings[0].PipelineID,
			Status:         domain.ExecutionStatusPending,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
	params := []domain.ReleaseOrderParam{
		{
			ID:                "rop-old-branch",
			ReleaseOrderID:    order.ID,
			PipelineScope:     domain.PipelineScopeCI,
			BindingID:         oldBindings[0].BindingID,
			ParamKey:          "branch",
			ExecutorParamName: "BRANCH",
			ParamValue:        "release/old",
			ValueSource:       domain.ValueSourceReleaseInput,
			CreatedAt:         now,
		},
	}
	steps := []domain.ReleaseOrderStep{
		testReleaseStep(order.ID, "step-old-start", domain.StepScopeCI, "ci:trigger_pipeline", domain.StepStatusPending, 1, now),
	}
	if err := repo.Create(ctx, order, executions, params, steps); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}

	updated, err := manager.Update(ctx, order.ID, UpdateReleaseOrderInput{
		ApplicationID: "app-1",
		TemplateID:    newTemplate.ID,
		ReleaseName:   "after name",
		EnvCode:       "prod",
		GitRef:        "release/new",
		Remark:        "after update",
		CreatorUserID: order.CreatorUserID,
		TriggeredBy:   order.TriggeredBy,
		Params: []CreateReleaseOrderParamInput{
			{
				PipelineScope:     domain.PipelineScopeCI,
				ParamKey:          "branch",
				ExecutorParamName: "BRANCH",
				ParamValue:        "release/new",
				ValueSource:       domain.ValueSourceReleaseInput,
			},
		},
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if updated.TemplateID != newTemplate.ID {
		t.Fatalf("updated template_id = %s, want %s", updated.TemplateID, newTemplate.ID)
	}
	if updated.BindingID != newBindings[0].BindingID {
		t.Fatalf("updated binding_id = %s, want %s", updated.BindingID, newBindings[0].BindingID)
	}
	if updated.GitRef != "release/new" {
		t.Fatalf("updated git_ref = %s, want %s", updated.GitRef, "release/new")
	}
	if updated.ReleaseName != "after name" {
		t.Fatalf("updated release_name = %s, want %s", updated.ReleaseName, "after name")
	}
	if updated.Remark != "after update" {
		t.Fatalf("updated remark = %s, want %s", updated.Remark, "after update")
	}
	stored, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.ReleaseName != "after name" {
		t.Fatalf("stored release_name = %s, want %s", stored.ReleaseName, "after name")
	}

	storedExecutions, err := repo.ListExecutions(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListExecutions failed: %v", err)
	}
	if len(storedExecutions) != 1 || storedExecutions[0].BindingID != newBindings[0].BindingID {
		t.Fatalf("stored executions = %#v, want new binding", storedExecutions)
	}

	storedParams, err := repo.ListParams(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListParams failed: %v", err)
	}
	if len(storedParams) != 1 || storedParams[0].ParamValue != "release/new" {
		t.Fatalf("stored params = %#v, want updated branch value", storedParams)
	}

	storedSteps, err := repo.ListSteps(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListSteps failed: %v", err)
	}
	if len(storedSteps) == 0 {
		t.Fatalf("stored steps should be rebuilt")
	}
}

// TestCreateReleaseOrderDoesNotPromoteCIBranchToGitRefWithoutBuiltinMapping 创建业务资源并返回处理结果。
func TestCreateReleaseOrderDoesNotPromoteCIBranchToGitRefWithoutBuiltinMapping(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	manager.appRepo = releaseOrderUpdateApplicationRepoStub{
		app: appdomain.Application{
			ID:     "app-1",
			Name:   "App 1",
			Key:    "app-1",
			Status: appdomain.StatusActive,
		},
	}

	template := domain.ReleaseTemplate{
		ID:              "rt-create-no-builtin-branch",
		Name:            "template-create-no-builtin-branch",
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
			ID:            "rtb-create-no-builtin-branch-ci",
			TemplateID:    template.ID,
			PipelineScope: domain.PipelineScopeCI,
			BindingID:     "binding-create-no-builtin-branch-ci",
			BindingName:   "CI",
			Provider:      "jenkins",
			PipelineID:    "pipeline-create-no-builtin-branch-ci",
			Enabled:       true,
			SortNo:        1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	params := []domain.ReleaseTemplateParam{
		{
			ID:                 "rtp-create-no-builtin-branch-ci-branch",
			TemplateID:         template.ID,
			TemplateBindingID:  bindings[0].ID,
			PipelineScope:      domain.PipelineScopeCI,
			BindingID:          bindings[0].BindingID,
			ExecutorParamDefID: "ep-create-no-builtin-branch-ci-branch",
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
	if err := repo.CreateTemplate(ctx, template, bindings, params, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	order, err := manager.Create(ctx, CreateReleaseOrderInput{
		ApplicationID: "app-1",
		TemplateID:    template.ID,
		ReleaseName:   "CI only release",
		EnvCode:       "dev",
		CreatorUserID: "user-1",
		TriggeredBy:   "user-1",
		Params: []CreateReleaseOrderParamInput{
			{
				PipelineScope:     domain.PipelineScopeCI,
				ParamKey:          "branch",
				ExecutorParamName: "BRANCH",
				ParamValue:        "release/ci-only",
				ValueSource:       domain.ValueSourceReleaseInput,
			},
		},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if order.GitRef != "" {
		t.Fatalf("created git_ref = %q, want empty when template has no builtin branch mapping", order.GitRef)
	}
	if order.ReleaseName != "CI only release" {
		t.Fatalf("created release_name = %q, want %q", order.ReleaseName, "CI only release")
	}
}

// TestUpdateReleaseOrderDoesNotPromoteCIBranchToGitRefWithoutBuiltinMapping 更新业务资源并返回处理结果。
func TestUpdateReleaseOrderDoesNotPromoteCIBranchToGitRefWithoutBuiltinMapping(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	manager.appRepo = releaseOrderUpdateApplicationRepoStub{
		app: appdomain.Application{
			ID:     "app-1",
			Name:   "App 1",
			Key:    "app-1",
			Status: appdomain.StatusActive,
		},
	}

	template := domain.ReleaseTemplate{
		ID:              "rt-update-no-builtin-branch",
		Name:            "template-update-no-builtin-branch",
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
			ID:            "rtb-update-no-builtin-branch-ci",
			TemplateID:    template.ID,
			PipelineScope: domain.PipelineScopeCI,
			BindingID:     "binding-update-no-builtin-branch-ci",
			BindingName:   "CI",
			Provider:      "jenkins",
			PipelineID:    "pipeline-update-no-builtin-branch-ci",
			Enabled:       true,
			SortNo:        1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	params := []domain.ReleaseTemplateParam{
		{
			ID:                 "rtp-update-no-builtin-branch-ci-branch",
			TemplateID:         template.ID,
			TemplateBindingID:  bindings[0].ID,
			PipelineScope:      domain.PipelineScopeCI,
			BindingID:          bindings[0].BindingID,
			ExecutorParamDefID: "ep-update-no-builtin-branch-ci-branch",
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
	if err := repo.CreateTemplate(ctx, template, bindings, params, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	order := testReleaseOrder("ro-update-no-builtin-branch", "RO-UPDATE-NO-BUILTIN-BRANCH", domain.OrderStatusPending, now)
	order.TemplateID = template.ID
	order.TemplateName = template.Name
	order.BindingID = bindings[0].BindingID
	order.PipelineID = bindings[0].PipelineID
	order.GitRef = ""
	executions := []domain.ReleaseOrderExecution{
		{
			ID:             "exec-update-no-builtin-branch-ci",
			ReleaseOrderID: order.ID,
			PipelineScope:  domain.PipelineScopeCI,
			BindingID:      bindings[0].BindingID,
			BindingName:    bindings[0].BindingName,
			Provider:       bindings[0].Provider,
			PipelineID:     bindings[0].PipelineID,
			Status:         domain.ExecutionStatusPending,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
	if err := repo.Create(ctx, order, executions, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}

	updated, err := manager.Update(ctx, order.ID, UpdateReleaseOrderInput{
		ApplicationID: "app-1",
		TemplateID:    template.ID,
		EnvCode:       "dev",
		CreatorUserID: order.CreatorUserID,
		TriggeredBy:   order.TriggeredBy,
		Params: []CreateReleaseOrderParamInput{
			{
				PipelineScope:     domain.PipelineScopeCI,
				ParamKey:          "branch",
				ExecutorParamName: "BRANCH",
				ParamValue:        "release/ci-only",
				ValueSource:       domain.ValueSourceReleaseInput,
			},
		},
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.GitRef != "" {
		t.Fatalf("updated git_ref = %q, want empty when template has no builtin branch mapping", updated.GitRef)
	}
}

// TestUpdateReleaseOrderRejectsNonPendingStatus 更新业务资源并返回处理结果。
func TestUpdateReleaseOrderRejectsNonPendingStatus(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	manager.appRepo = releaseOrderUpdateApplicationRepoStub{
		app: appdomain.Application{
			ID:     "app-1",
			Name:   "App 1",
			Key:    "app-1",
			Status: appdomain.StatusActive,
		},
	}

	order := testReleaseOrder("ro-running-update", "RO-RUNNING-UPDATE", domain.OrderStatusApproved, now)
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}

	if _, err := manager.Update(ctx, order.ID, UpdateReleaseOrderInput{
		ApplicationID: order.ApplicationID,
		TemplateID:    order.TemplateID,
		EnvCode:       order.EnvCode,
		CreatorUserID: order.CreatorUserID,
	}); err == nil {
		t.Fatal("Update error = nil, want invalid status")
	}
}

// TestUpdateReleaseOrderRejectsApplicationChange 更新业务资源并返回处理结果。
func TestUpdateReleaseOrderRejectsApplicationChange(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	manager.appRepo = releaseOrderUpdateApplicationRepoStub{
		app: appdomain.Application{
			ID:     "app-1",
			Name:   "App 1",
			Key:    "app-1",
			Status: appdomain.StatusActive,
		},
	}

	template := domain.ReleaseTemplate{
		ID:              "rt-app-lock",
		Name:            "template-app-lock",
		ApplicationID:   "app-1",
		ApplicationName: "App 1",
		BindingID:       "app-1",
		BindingName:     "App 1",
		BindingType:     "application",
		Status:          domain.TemplateStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := repo.CreateTemplate(ctx, template, nil, nil, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	order := testReleaseOrder("ro-app-lock", "RO-APP-LOCK", domain.OrderStatusPending, now)
	order.TemplateID = template.ID
	order.TemplateName = template.Name
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}

	if _, err := manager.Update(ctx, order.ID, UpdateReleaseOrderInput{
		ApplicationID: "app-2",
		TemplateID:    template.ID,
		EnvCode:       order.EnvCode,
		CreatorUserID: order.CreatorUserID,
	}); err == nil {
		t.Fatal("Update error = nil, want invalid input when application changes")
	}
}

type releaseOrderUpdateApplicationRepoStub struct {
	app appdomain.Application
}

// Create 创建业务资源并返回处理结果。
func (s releaseOrderUpdateApplicationRepoStub) Create(context.Context, appdomain.Application) error {
	panic("unexpected call")
}

// GetByID 查询并返回指定资源数据。
func (s releaseOrderUpdateApplicationRepoStub) GetByID(context.Context, string) (appdomain.Application, error) {
	return s.app, nil
}

// List 查询并返回列表数据。
func (s releaseOrderUpdateApplicationRepoStub) List(context.Context, appdomain.ListFilter) ([]appdomain.Application, int64, error) {
	panic("unexpected call")
}

// Update 更新业务资源并返回处理结果。
func (s releaseOrderUpdateApplicationRepoStub) Update(context.Context, string, appdomain.UpdateInput, time.Time) (appdomain.Application, error) {
	panic("unexpected call")
}

// Delete 删除业务资源并返回处理结果。
func (s releaseOrderUpdateApplicationRepoStub) Delete(context.Context, string) error {
	panic("unexpected call")
}

// InitSchema 封装当前模块的业务处理逻辑。
func (s releaseOrderUpdateApplicationRepoStub) InitSchema(context.Context) error {
	return nil
}
