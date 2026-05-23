package usecase

import (
	"context"
	"testing"
	"time"

	appdomain "gos/internal/domain/application"
	artifactrepodomain "gos/internal/domain/artifactrepo"
	executorparamdomain "gos/internal/domain/executorparam"
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

// TestMaterializeCreateTemplateParamsResolvesBuiltinReleaseName 发布名称作为内置字段时应能写入管线参数快照。
func TestMaterializeCreateTemplateParamsResolvesBuiltinReleaseName(t *testing.T) {
	t.Parallel()

	manager := &ReleaseOrderManager{}
	resolved, err := manager.materializeCreateTemplateParams(
		context.Background(),
		appdomain.Application{Key: "app-1"},
		[]domain.ReleaseTemplateParam{
			{
				PipelineScope:     domain.PipelineScopeCI,
				ParamKey:          "release_title",
				ParamName:         "发布标题",
				ExecutorParamName: "RELEASE_TITLE",
				ValueSource:       domain.TemplateParamValueSourceBuiltin,
				SourceParamKey:    "release_name",
				Required:          true,
			},
		},
		nil,
		"prod",
		"release/2026-05-09",
		"portal-service",
		"20260509.1",
		"门户正式发布",
	)
	if err != nil {
		t.Fatalf("materializeCreateTemplateParams failed: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("resolved params length = %d, want 1", len(resolved))
	}
	if got := resolved[0].ParamValue; got != "门户正式发布" {
		t.Fatalf("resolved release_name value = %q, want %q", got, "门户正式发布")
	}
	if got := resolved[0].ValueSource; got != domain.ValueSourceBuiltin {
		t.Fatalf("resolved value source = %q, want %q", got, domain.ValueSourceBuiltin)
	}
}

// TestMaterializeCreateTemplateParamsUsesExecutorDefaultForBuiltin 内置字段无平台摘要值时应使用执行器参数默认值。
func TestMaterializeCreateTemplateParamsUsesExecutorDefaultForBuiltin(t *testing.T) {
	t.Parallel()

	manager := &ReleaseOrderManager{
		paramRepo: &releasePrecheckParamRepoFake{
			defs: map[string]executorparamdomain.ExecutorParamDef{
				"ep-oss-endpoint": {
					ID:                "ep-oss-endpoint",
					ExecutorParamName: "OSS_ENDPOINT",
					ParamKey:          "oss_endpoint",
					DefaultValue:      "oss-cn-shanghai.aliyuncs.com",
					Status:            executorparamdomain.StatusActive,
				},
			},
		},
	}
	resolved, err := manager.materializeCreateTemplateParams(
		context.Background(),
		appdomain.Application{Key: "app-1"},
		[]domain.ReleaseTemplateParam{
			{
				ExecutorParamDefID: "ep-oss-endpoint",
				PipelineScope:      domain.PipelineScopeCI,
				ParamKey:           "oss_endpoint",
				ParamName:          "OSS Endpoint",
				ExecutorParamName:  "OSS_ENDPOINT",
				ValueSource:        domain.TemplateParamValueSourceBuiltin,
				SourceParamKey:     "oss_endpoint",
			},
		},
		nil,
		"dev",
		"release/v1",
		"portal-service",
		"20260512.1",
		"门户测试发布",
	)
	if err != nil {
		t.Fatalf("materializeCreateTemplateParams failed: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("resolved params length = %d, want 1", len(resolved))
	}
	if got := resolved[0].ParamValue; got != "oss-cn-shanghai.aliyuncs.com" {
		t.Fatalf("resolved builtin default value = %q, want %q", got, "oss-cn-shanghai.aliyuncs.com")
	}
	if got := resolved[0].ValueSource; got != domain.ValueSourceBuiltin {
		t.Fatalf("resolved value source = %q, want %q", got, domain.ValueSourceBuiltin)
	}
}

// TestMaterializeCreateTemplateParamsReadsOSSSecretsFromApplicationArtifactRepository OSS 密钥类内置字段应从应用绑定制品库取值。
func TestMaterializeCreateTemplateParamsReadsOSSSecretsFromApplicationArtifactRepository(t *testing.T) {
	t.Parallel()

	artifactRepo := newArtifactRepositoryFake()
	artifactRepo.items["repo-oss"] = artifactrepodomain.ArtifactRepository{
		ID:              "repo-oss",
		RepositoryType:  artifactrepodomain.RepositoryTypeOSS,
		Endpoint:        "oss-cn-shanghai.aliyuncs.com",
		Bucket:          "gc-oa",
		Directory:       "tempUpdate",
		AccessKeyID:     "ak-from-artifact-repo",
		AccessKeySecret: "secret-from-artifact-repo",
		ACL:             artifactrepodomain.ACLPrivate,
		Status:          artifactrepodomain.StatusEnabled,
	}
	manager := &ReleaseOrderManager{}
	manager.SetArtifactRepository(artifactRepo)

	resolved, err := manager.materializeCreateTemplateParams(
		context.Background(),
		appdomain.Application{
			Key:                  "app-1",
			ArtifactRepositoryID: "repo-oss",
		},
		[]domain.ReleaseTemplateParam{
			{
				PipelineScope:     domain.PipelineScopeCI,
				ParamKey:          "oss_access_key_id",
				ParamName:         "OSS AccessKey ID",
				ExecutorParamName: "OSS_ACCESS_KEY_ID",
				ValueSource:       domain.TemplateParamValueSourceBuiltin,
				SourceParamKey:    "oss_access_key_id",
			},
			{
				PipelineScope:     domain.PipelineScopeCI,
				ParamKey:          "oss_access_key_secret",
				ParamName:         "OSS AccessKey Secret",
				ExecutorParamName: "OSS_ACCESS_KEY_SECRET",
				ValueSource:       domain.TemplateParamValueSourceBuiltin,
				SourceParamKey:    "oss_access_key_secret",
			},
		},
		nil,
		"dev",
		"release/v1",
		"portal-service",
		"20260512.1",
		"门户测试发布",
	)
	if err != nil {
		t.Fatalf("materializeCreateTemplateParams failed: %v", err)
	}
	values := map[string]string{}
	for _, item := range resolved {
		values[item.ParamKey] = item.ParamValue
	}
	if got := values["oss_access_key_id"]; got != "ak-from-artifact-repo" {
		t.Fatalf("oss_access_key_id = %q, want artifact repository access key id", got)
	}
	if got := values["oss_access_key_secret"]; got != "secret-from-artifact-repo" {
		t.Fatalf("oss_access_key_secret = %q, want artifact repository access key secret", got)
	}
}

// TestMaterializeCreateTemplateParamsResolvesGOSArtifactPathFromApplication 应用制品路径内置字段应从 App 基础信息取值。
func TestMaterializeCreateTemplateParamsResolvesGOSArtifactPathFromApplication(t *testing.T) {
	t.Parallel()

	manager := &ReleaseOrderManager{}
	resolved, err := manager.materializeCreateTemplateParams(
		context.Background(),
		appdomain.Application{
			Key:               "app-1",
			ArtifactDirectory: "release/pay-center",
		},
		[]domain.ReleaseTemplateParam{
			{
				PipelineScope:     domain.PipelineScopeCI,
				ParamKey:          "gos_artifact_path",
				ParamName:         "GOS_ARTIFACT_PATH",
				ExecutorParamName: "GOS_ARTIFACT_PATH",
				ValueSource:       domain.TemplateParamValueSourceBuiltin,
				SourceParamKey:    "gos_artifact_path",
			},
		},
		nil,
		"prod",
		"release/v1",
		"project-from-release",
		"20260523.1",
		"发布单名称",
	)
	if err != nil {
		t.Fatalf("materializeCreateTemplateParams failed: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("resolved params length = %d, want 1", len(resolved))
	}
	if got := resolved[0].ParamValue; got != "release/pay-center" {
		t.Fatalf("gos_artifact_path = %q, want app artifact directory", got)
	}
	if got := resolved[0].ValueSource; got != domain.ValueSourceBuiltin {
		t.Fatalf("gos_artifact_path value source = %q, want %q", got, domain.ValueSourceBuiltin)
	}
}

// TestResolveStandardFieldValueUsesCIOnlyForGOSArtifactURL GOS 制品地址只能从 CI 单元取值。
func TestResolveStandardFieldValueUsesCIOnlyForGOSArtifactURL(t *testing.T) {
	t.Parallel()

	manager := &ReleaseOrderManager{}
	got := manager.resolveStandardFieldValue(domain.ReleaseOrder{}, []domain.ReleaseOrderParam{
		{
			PipelineScope: domain.PipelineScopeCD,
			ParamKey:      "gos_artifact_url",
			ParamValue:    "https://cd.example.com/should-not-use.jar",
		},
		{
			PipelineScope: domain.PipelineScopeCI,
			ParamKey:      "gos_artifact_url",
			ParamValue:    "https://ci.example.com/app.jar",
		},
	}, nil, "", nil, "gos_artifact_url")
	if got != "https://ci.example.com/app.jar" {
		t.Fatalf("gos_artifact_url = %q, want CI value", got)
	}
}

// TestResolveStandardFieldValueUsesApplicationForGOSArtifactPath GOS 制品路径取 App 基础信息，不取发布参数。
func TestResolveStandardFieldValueUsesApplicationForGOSArtifactPath(t *testing.T) {
	t.Parallel()

	manager := &ReleaseOrderManager{}
	got := manager.resolveStandardFieldValue(domain.ReleaseOrder{}, []domain.ReleaseOrderParam{
		{
			PipelineScope: domain.PipelineScopeCD,
			ParamKey:      "gos_artifact_path",
			ParamValue:    "release/from-cd-param",
		},
		{
			PipelineScope: domain.PipelineScopeCI,
			ParamKey:      "gos_artifact_path",
			ParamValue:    "release/from-ci-param",
		},
	}, nil, "", map[string]string{"gos_artifact_path": "release/pay-center"}, "gos_artifact_path")
	if got != "release/pay-center" {
		t.Fatalf("gos_artifact_path = %q, want app artifact directory", got)
	}
}

// TestResolveTemplateExecutionParamValueUsesCIOnlyForGOSArtifactURL CD 参数引用 GOS 制品地址时也必须从 CI 单元取值。
func TestResolveTemplateExecutionParamValueUsesCIOnlyForGOSArtifactURL(t *testing.T) {
	t.Parallel()

	manager := &ReleaseOrderManager{}
	got := manager.resolveTemplateExecutionParamValue(
		domain.ReleaseOrder{},
		domain.PipelineScopeCD,
		domain.ReleaseTemplateParam{
			PipelineScope:     domain.PipelineScopeCD,
			ParamKey:          "gos_artifact_url",
			ExecutorParamName: "GOS_ARTIFACT_URL",
			ValueSource:       domain.TemplateParamValueSourceBuiltin,
			SourceParamKey:    "gos_artifact_url",
		},
		[]domain.ReleaseOrderParam{
			{
				PipelineScope: domain.PipelineScopeCD,
				ParamKey:      "gos_artifact_url",
				ParamValue:    "https://cd.example.com/should-not-use.jar",
			},
			{
				PipelineScope: domain.PipelineScopeCI,
				ParamKey:      "gos_artifact_url",
				ParamValue:    "https://ci.example.com/app.jar",
			},
		},
		nil,
		"",
		nil,
	)
	if got != "https://ci.example.com/app.jar" {
		t.Fatalf("CD gos_artifact_url = %q, want CI value", got)
	}
}

// TestResolveTemplateExecutionParamValueUsesApplicationForGOSArtifactPath 执行参数引用制品路径时应从 App 基础信息取值。
func TestResolveTemplateExecutionParamValueUsesApplicationForGOSArtifactPath(t *testing.T) {
	t.Parallel()

	manager := &ReleaseOrderManager{}
	got := manager.resolveTemplateExecutionParamValue(
		domain.ReleaseOrder{},
		domain.PipelineScopeCD,
		domain.ReleaseTemplateParam{
			PipelineScope:     domain.PipelineScopeCD,
			ParamKey:          "gos_artifact_path",
			ExecutorParamName: "GOS_ARTIFACT_PATH",
			ValueSource:       domain.TemplateParamValueSourceBuiltin,
			SourceParamKey:    "gos_artifact_path",
		},
		[]domain.ReleaseOrderParam{
			{
				PipelineScope: domain.PipelineScopeCD,
				ParamKey:      "gos_artifact_path",
				ParamValue:    "release/from-cd-param",
			},
		},
		nil,
		"",
		map[string]string{"gos_artifact_path": "release/pay-center"},
	)
	if got != "release/pay-center" {
		t.Fatalf("CD gos_artifact_path = %q, want app artifact directory", got)
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
