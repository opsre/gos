package usecase

import (
	"context"
	"testing"
	"time"

	executorparamdomain "gos/internal/domain/executorparam"
	domain "gos/internal/domain/release"
)

// TestDeriveReleaseProgressValueSupportsReleaseName 发布名称内置字段应能从发布单摘要取值。
func TestDeriveReleaseProgressValueSupportsReleaseName(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	value, ok := deriveReleaseProgressValue(domain.ReleaseOrder{
		ReleaseName: "地图共享平台正式发布",
		UpdatedAt:   now,
	}, ReleaseOrderValueProgressItem{
		ParamKey: "release_name",
	}, domain.ReleaseOrderExecution{})
	if !ok {
		t.Fatal("deriveReleaseProgressValue(release_name) ok = false, want true")
	}
	if value.Value != "地图共享平台正式发布" {
		t.Fatalf("release_name progress value = %q, want %q", value.Value, "地图共享平台正式发布")
	}
	if value.Source != "release_order_summary" {
		t.Fatalf("release_name progress source = %q, want release_order_summary", value.Source)
	}
}

// TestListValueProgressUsesExecutorDefaultForBuiltin 已创建发布单缺少参数快照时，取值进度应能展示执行器默认值。
func TestListValueProgressUsesExecutorDefaultForBuiltin(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.paramRepo = &releasePrecheckParamRepoFake{
		defs: map[string]executorparamdomain.ExecutorParamDef{
			"ep-oss-endpoint": {
				ID:                "ep-oss-endpoint",
				ExecutorParamName: "OSS_ENDPOINT",
				ParamKey:          "oss_endpoint",
				DefaultValue:      "oss-cn-shanghai.aliyuncs.com",
				Status:            executorparamdomain.StatusActive,
			},
		},
	}

	template := domain.ReleaseTemplate{
		ID:              "rt-progress-default",
		Name:            "template-progress-default",
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
			ID:            "rtb-progress-default-ci",
			TemplateID:    template.ID,
			PipelineScope: domain.PipelineScopeCI,
			BindingID:     "binding-progress-default-ci",
			BindingName:   "CI",
			Provider:      "jenkins",
			PipelineID:    "pipeline-progress-default-ci",
			Enabled:       true,
			SortNo:        1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	params := []domain.ReleaseTemplateParam{
		{
			ID:                 "rtp-progress-default-oss-endpoint",
			TemplateID:         template.ID,
			TemplateBindingID:  bindings[0].ID,
			PipelineScope:      domain.PipelineScopeCI,
			BindingID:          bindings[0].BindingID,
			ExecutorParamDefID: "ep-oss-endpoint",
			ParamKey:           "oss_endpoint",
			ParamName:          "OSS Endpoint",
			ExecutorParamName:  "OSS_ENDPOINT",
			ValueSource:        domain.TemplateParamValueSourceBuiltin,
			SourceParamKey:     "oss_endpoint",
			SortNo:             1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	if err := repo.CreateTemplate(ctx, template, bindings, params, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	order := testReleaseOrder("ro-progress-default", "RO-PROGRESS-DEFAULT", domain.OrderStatusFailed, now)
	order.TemplateID = template.ID
	order.TemplateName = template.Name
	order.BindingID = bindings[0].BindingID
	order.PipelineID = bindings[0].PipelineID
	executions := []domain.ReleaseOrderExecution{
		{
			ID:             "exec-progress-default-ci",
			ReleaseOrderID: order.ID,
			PipelineScope:  domain.PipelineScopeCI,
			BindingID:      bindings[0].BindingID,
			BindingName:    bindings[0].BindingName,
			Provider:       bindings[0].Provider,
			PipelineID:     bindings[0].PipelineID,
			Status:         domain.ExecutionStatusFailed,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
	if err := repo.Create(ctx, order, executions, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}

	items, err := manager.ListValueProgress(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListValueProgress failed: %v", err)
	}
	item := findValueProgressItemForTest(items, domain.PipelineScopeCI, "oss_endpoint")
	if item == nil {
		t.Fatalf("oss_endpoint progress item not found in %#v", items)
	}
	if item.Status != ReleaseOrderValueProgressResolved {
		t.Fatalf("oss_endpoint progress status = %s, want %s", item.Status, ReleaseOrderValueProgressResolved)
	}
	if item.Value != "oss-cn-shanghai.aliyuncs.com" {
		t.Fatalf("oss_endpoint progress value = %q, want %q", item.Value, "oss-cn-shanghai.aliyuncs.com")
	}
	if item.ValueSource != "executor_param_default" {
		t.Fatalf("oss_endpoint progress source = %q, want executor_param_default", item.ValueSource)
	}
}

// TestResolveValueProgressUsesCIOnlyForGOSArtifactURL 取值进度中的 GOS 制品地址只能来自 CI 单元。
func TestResolveValueProgressUsesCIOnlyForGOSArtifactURL(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	progress := resolveReleaseOrderValueProgressItem(
		domain.ReleaseOrder{UpdatedAt: now},
		domain.ReleaseTemplateParam{
			PipelineScope: domain.PipelineScopeCD,
			ParamKey:      "gos_artifact_url",
			ParamName:     "GOS_ARTIFACT_URL",
			ValueSource:   domain.TemplateParamValueSourceBuiltin,
		},
		indexReleaseOrderParams([]domain.ReleaseOrderParam{
			{
				PipelineScope: domain.PipelineScopeCD,
				ParamKey:      "gos_artifact_url",
				ParamValue:    "https://cd.example.com/should-not-use.jar",
				CreatedAt:     now,
			},
			{
				PipelineScope: domain.PipelineScopeCI,
				ParamKey:      "gos_artifact_url",
				ParamValue:    "https://ci.example.com/app.jar",
				CreatedAt:     now,
			},
		}),
		nil,
		"https://default.example.com/should-not-use.jar",
	)
	if progress.Status != ReleaseOrderValueProgressResolved {
		t.Fatalf("gos_artifact_url progress status = %s, want %s", progress.Status, ReleaseOrderValueProgressResolved)
	}
	if progress.Value != "https://ci.example.com/app.jar" {
		t.Fatalf("gos_artifact_url progress value = %q, want CI value", progress.Value)
	}
	if progress.PipelineParam {
		t.Fatal("gos_artifact_url pipeline_param = true, want false")
	}
	if progress.ValueKind != ReleaseOrderValueKindExecutionOutput {
		t.Fatalf("gos_artifact_url value kind = %q, want %q", progress.ValueKind, ReleaseOrderValueKindExecutionOutput)
	}
}

func findValueProgressItemForTest(
	items []ReleaseOrderValueProgressItem,
	scope domain.PipelineScope,
	paramKey string,
) *ReleaseOrderValueProgressItem {
	for idx := range items {
		if items[idx].PipelineScope == scope && items[idx].ParamKey == paramKey {
			return &items[idx]
		}
	}
	return nil
}
