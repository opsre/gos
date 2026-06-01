package usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	executorparamdomain "gos/internal/domain/executorparam"
	pipelinedomain "gos/internal/domain/pipeline"
	scandomain "gos/internal/domain/pipelinescan"
	domain "gos/internal/domain/release"
)

func TestPrecheckExecuteBlocksViolatedTemplate(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }

	template := domain.ReleaseTemplate{
		ID:              "rt-violated",
		Name:            "违规模板",
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
			ID:            "rtb-violated-ci",
			TemplateID:    template.ID,
			PipelineScope: domain.PipelineScopeCI,
			BindingID:     "binding-ci",
			BindingName:   "CI",
			Provider:      "jenkins",
			PipelineID:    "pipeline-ci",
			Enabled:       true,
			SortNo:        1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	if err := repo.CreateTemplate(ctx, template, bindings, nil, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	manager.SetPipelineScanRepository(&pipelineScanRepoFake{
		rules: []scandomain.Rule{
			{
				ID:                       "rule-oss",
				RuleCode:                 "artifact.oss.pipeline_params.standard",
				RuleName:                 "OSS 参数规范",
				Category:                 scandomain.CategoryArtifact,
				Severity:                 scandomain.SeverityError,
				Enabled:                  true,
				TemplateValidationScopes: []string{"ci"},
				Message:                  "OSS 参数缺失",
			},
		},
		result: scandomain.Result{
			ID:           "scan-ci",
			PipelineID:   "pipeline-ci",
			PipelineName: "CI",
			ScanStatus:   scandomain.ScanStatusFailed,
		},
		findings: []scandomain.Finding{
			{
				ID:         "finding-ci",
				PipelineID: "pipeline-ci",
				RuleID:     "rule-oss",
				RuleCode:   "artifact.oss.pipeline_params.standard",
				RuleName:   "OSS 参数规范",
				Severity:   scandomain.SeverityError,
				Message:    "缺少 OSS 参数",
				Status:     scandomain.FindingStatusOpen,
			},
		},
	})

	order := testReleaseOrder("ro-violated", "RO-VIOLATED", domain.OrderStatusPending, now)
	order.TemplateID = template.ID
	order.TemplateName = template.Name
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
	}
	if err := repo.Create(ctx, order, executions, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}

	precheck, err := manager.PrecheckExecute(ctx, order.ID)
	if err != nil {
		t.Fatalf("PrecheckExecute failed: %v", err)
	}
	if precheck.Executable {
		t.Fatal("precheck executable = true, want blocked for violated template")
	}
	item := findPrecheckItem(precheck.Items, "template_compliance")
	if item == nil {
		t.Fatal("template_compliance item missing")
	}
	if item.Status != ReleaseOrderPrecheckItemStatusBlocked {
		t.Fatalf("template_compliance status = %s, want blocked", item.Status)
	}
	if !strings.Contains(item.Message, "CI 1项违规") {
		t.Fatalf("template_compliance message = %q, want CI violation summary", item.Message)
	}
}

func TestPrecheckExecuteBlocksIncompleteCDParamMapping(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }

	template := domain.ReleaseTemplate{
		ID:              "rt-incomplete-param",
		Name:            "参数映射不完整模板",
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
			ID:            "rtb-param-ci",
			TemplateID:    template.ID,
			PipelineScope: domain.PipelineScopeCI,
			BindingID:     "binding-ci",
			BindingName:   "CI",
			Provider:      "jenkins",
			PipelineID:    "pipeline-ci",
			Enabled:       true,
			SortNo:        1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "rtb-param-cd",
			TemplateID:    template.ID,
			PipelineScope: domain.PipelineScopeCD,
			BindingID:     "binding-cd",
			BindingName:   "CD",
			Provider:      "jenkins",
			PipelineID:    "pipeline-cd",
			Enabled:       true,
			SortNo:        2,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	params := []domain.ReleaseTemplateParam{
		{
			ID:                 "rtp-ci-branch",
			TemplateID:         template.ID,
			TemplateBindingID:  bindings[0].ID,
			PipelineScope:      domain.PipelineScopeCI,
			BindingID:          bindings[0].BindingID,
			ExecutorParamDefID: "ep-ci-branch",
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
			ID:                 "rtp-cd-oss-bucket",
			TemplateID:         template.ID,
			TemplateBindingID:  bindings[1].ID,
			PipelineScope:      domain.PipelineScopeCD,
			BindingID:          bindings[1].BindingID,
			ExecutorParamDefID: "ep-cd-oss-bucket",
			ParamName:          "OSS Bucket",
			ExecutorParamName:  "OSS_BUCKET",
			ValueSource:        domain.TemplateParamValueSourceBuiltin,
			SourceParamKey:     "oss_bucket",
			Required:           true,
			SortNo:             1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	if err := repo.CreateTemplate(ctx, template, bindings, params, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	order := testReleaseOrder("ro-incomplete-param", "RO-INCOMPLETE-PARAM", domain.OrderStatusPending, now)
	order.TemplateID = template.ID
	order.TemplateName = template.Name
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}
	orderParams := []domain.ReleaseOrderParam{
		{
			ID:                "rop-ci-branch",
			ReleaseOrderID:    order.ID,
			PipelineScope:     domain.PipelineScopeCI,
			BindingID:         bindings[0].BindingID,
			ParamKey:          "branch",
			ExecutorParamName: "BRANCH",
			ParamValue:        "release/test",
			ValueSource:       domain.ValueSourceReleaseInput,
			CreatedAt:         now,
		},
	}
	if err := repo.Create(ctx, order, executions, orderParams, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}

	precheck, err := manager.PrecheckExecute(ctx, order.ID)
	if err != nil {
		t.Fatalf("PrecheckExecute failed: %v", err)
	}
	if precheck.Executable {
		t.Fatal("precheck executable = true, want blocked for incomplete CD param mapping")
	}
	item := findPrecheckItem(precheck.Items, "template_param_mapping")
	if item == nil {
		t.Fatal("template_param_mapping item missing")
	}
	if item.Status != ReleaseOrderPrecheckItemStatusBlocked {
		t.Fatalf("template_param_mapping status = %s, want blocked", item.Status)
	}
	if !strings.Contains(item.Message, "CD") || !strings.Contains(item.Message, "OSS_BUCKET") {
		t.Fatalf("template_param_mapping message = %q, want mention CD OSS_BUCKET", item.Message)
	}
}

func TestPrecheckExecuteBlocksJenkinsChoiceCandidateMismatch(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	manager.pipelineRepo = &pipelineScanPipelineRepoFake{
		pipelines: map[string]pipelinedomain.Pipeline{
			"pipeline-ci": {
				ID:          "pipeline-ci",
				Provider:    pipelinedomain.ProviderJenkins,
				JobFullName: "folder/choice-job",
				JobName:     "choice-job",
				Status:      pipelinedomain.StatusActive,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
	}
	manager.paramRepo = &releasePrecheckParamRepoFake{
		defs: map[string]executorparamdomain.ExecutorParamDef{
			"ep-ci-deploy-to": {
				ID:                "ep-ci-deploy-to",
				PipelineID:        "pipeline-ci",
				ExecutorType:      executorparamdomain.ExecutorTypeJenkins,
				ExecutorParamName: "DEPLOY_TO",
				ParamKey:          "env",
				ParamType:         executorparamdomain.ParamTypeChoice,
				Status:            executorparamdomain.StatusActive,
				RawMeta:           `{"choices":["dev","prod"]}`,
			},
		},
	}
	manager.jenkins = &releasePrecheckJenkinsFake{
		jobSets: map[string]executorparamdomain.JenkinsJobParamSet{
			"folder/choice-job": {
				JobFullName: "folder/choice-job",
				Params: []executorparamdomain.JenkinsParamSnapshot{
					{
						Name:      "DEPLOY_TO",
						ParamType: executorparamdomain.ParamTypeChoice,
						RawMeta:   `{"choices":["dev","prod","trial"]}`,
					},
				},
			},
		},
	}

	template := domain.ReleaseTemplate{
		ID:              "rt-choice",
		Name:            "候选值模板",
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
			ID:            "rtb-choice-ci",
			TemplateID:    template.ID,
			PipelineScope: domain.PipelineScopeCI,
			BindingID:     "binding-ci",
			BindingName:   "CI",
			Provider:      "jenkins",
			PipelineID:    "pipeline-ci",
			Enabled:       true,
			SortNo:        1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	params := []domain.ReleaseTemplateParam{
		{
			ID:                 "rtp-ci-deploy-to",
			TemplateID:         template.ID,
			TemplateBindingID:  bindings[0].ID,
			PipelineScope:      domain.PipelineScopeCI,
			BindingID:          bindings[0].BindingID,
			ExecutorParamDefID: "ep-ci-deploy-to",
			ParamKey:           "env",
			ParamName:          "环境",
			ExecutorParamName:  "DEPLOY_TO",
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

	order := testReleaseOrder("ro-choice", "RO-CHOICE", domain.OrderStatusPending, now)
	order.TemplateID = template.ID
	order.TemplateName = template.Name
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
	}
	orderParams := []domain.ReleaseOrderParam{
		{
			ID:                "rop-ci-deploy-to",
			ReleaseOrderID:    order.ID,
			PipelineScope:     domain.PipelineScopeCI,
			BindingID:         bindings[0].BindingID,
			ParamKey:          "env",
			ExecutorParamName: "DEPLOY_TO",
			ParamValue:        "dev",
			ValueSource:       domain.ValueSourceReleaseInput,
			CreatedAt:         now,
		},
	}
	if err := repo.Create(ctx, order, executions, orderParams, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}

	precheck, err := manager.PrecheckExecute(ctx, order.ID)
	if err != nil {
		t.Fatalf("PrecheckExecute failed: %v", err)
	}
	if precheck.Executable {
		t.Fatal("precheck executable = true, want blocked for Jenkins choice candidate mismatch")
	}
	item := findPrecheckItem(precheck.Items, "template_param_mapping")
	if item == nil {
		t.Fatal("template_param_mapping item missing")
	}
	if item.Status != ReleaseOrderPrecheckItemStatusBlocked {
		t.Fatalf("template_param_mapping status = %s, want blocked", item.Status)
	}
	if !strings.Contains(item.Message, "DEPLOY_TO") || !strings.Contains(item.Message, "候选值") {
		t.Fatalf("template_param_mapping message = %q, want mention DEPLOY_TO candidate mismatch", item.Message)
	}
}

func TestPrecheckExecuteAllowsStaleGitParameterCandidatesWhenSelectedValueStillExists(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	manager.pipelineRepo = &pipelineScanPipelineRepoFake{
		pipelines: map[string]pipelinedomain.Pipeline{
			"pipeline-ci": {
				ID:          "pipeline-ci",
				Provider:    pipelinedomain.ProviderJenkins,
				JobFullName: "folder/git-param-job",
				JobName:     "git-param-job",
				Status:      pipelinedomain.StatusActive,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
	}
	manager.paramRepo = &releasePrecheckParamRepoFake{
		defs: map[string]executorparamdomain.ExecutorParamDef{
			"ep-ci-branch": {
				ID:                "ep-ci-branch",
				PipelineID:        "pipeline-ci",
				ExecutorType:      executorparamdomain.ExecutorTypeJenkins,
				ExecutorParamName: "BRANCH",
				ParamKey:          "branch",
				ParamType:         executorparamdomain.ParamTypeChoice,
				Status:            executorparamdomain.StatusActive,
				RawMeta:           `{"_class":"net.uaznia.lukanus.hudson.plugins.gitparameter.GitParameterDefinition","type":"GitParameterDefinition","choices":["origin/release/v2.24.1","origin/master","origin/develop"]}`,
			},
		},
	}
	manager.jenkins = &releasePrecheckJenkinsFake{
		jobSets: map[string]executorparamdomain.JenkinsJobParamSet{
			"folder/git-param-job": {
				JobFullName: "folder/git-param-job",
				Params: []executorparamdomain.JenkinsParamSnapshot{
					{
						Name:      "BRANCH",
						ParamType: executorparamdomain.ParamTypeChoice,
						RawMeta:   `{"_class":"net.uaznia.lukanus.hudson.plugins.gitparameter.GitParameterDefinition","type":"GitParameterDefinition","choices":["origin/master","origin/develop"]}`,
					},
				},
			},
		},
	}

	template := domain.ReleaseTemplate{
		ID:              "rt-git-param",
		Name:            "动态分支模板",
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
			ID:            "rtb-git-param-ci",
			TemplateID:    template.ID,
			PipelineScope: domain.PipelineScopeCI,
			BindingID:     "binding-ci",
			BindingName:   "CI",
			Provider:      "jenkins",
			PipelineID:    "pipeline-ci",
			Enabled:       true,
			SortNo:        1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	templateParams := []domain.ReleaseTemplateParam{
		{
			ID:                 "rtp-ci-branch",
			TemplateID:         template.ID,
			TemplateBindingID:  bindings[0].ID,
			PipelineScope:      domain.PipelineScopeCI,
			BindingID:          bindings[0].BindingID,
			ExecutorParamDefID: "ep-ci-branch",
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
	if err := repo.CreateTemplate(ctx, template, bindings, templateParams, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	order := testReleaseOrder("ro-git-param", "RO-GIT-PARAM", domain.OrderStatusPending, now)
	order.TemplateID = template.ID
	order.TemplateName = template.Name
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
	}
	orderParams := []domain.ReleaseOrderParam{
		{
			ID:                "rop-ci-branch",
			ReleaseOrderID:    order.ID,
			PipelineScope:     domain.PipelineScopeCI,
			BindingID:         bindings[0].BindingID,
			ParamKey:          "branch",
			ExecutorParamName: "BRANCH",
			ParamValue:        "origin/master",
			ValueSource:       domain.ValueSourceReleaseInput,
			CreatedAt:         now,
		},
	}
	if err := repo.Create(ctx, order, executions, orderParams, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}

	precheck, err := manager.PrecheckExecute(ctx, order.ID)
	if err != nil {
		t.Fatalf("PrecheckExecute failed: %v", err)
	}
	if !precheck.Executable {
		t.Fatalf("precheck executable = false, items = %#v", precheck.Items)
	}
	item := findPrecheckItem(precheck.Items, "template_param_mapping")
	if item == nil {
		t.Fatal("template_param_mapping item missing")
	}
	if item.Status != ReleaseOrderPrecheckItemStatusPass {
		t.Fatalf("template_param_mapping status = %s, want pass, message=%q", item.Status, item.Message)
	}
}

func findPrecheckItem(items []ReleaseOrderPrecheckItem, key string) *ReleaseOrderPrecheckItem {
	for idx := range items {
		if items[idx].Key == key {
			return &items[idx]
		}
	}
	return nil
}

type releasePrecheckParamRepoFake struct {
	defs map[string]executorparamdomain.ExecutorParamDef
}

func (r *releasePrecheckParamRepoFake) InitSchema(context.Context) error { return nil }

func (r *releasePrecheckParamRepoFake) Upsert(context.Context, []executorparamdomain.ExecutorParamDef) (int, int, error) {
	return 0, 0, nil
}

func (r *releasePrecheckParamRepoFake) MarkMissingInactive(context.Context, executorparamdomain.ExecutorType, []string, time.Time) (int, error) {
	return 0, nil
}

func (r *releasePrecheckParamRepoFake) ListByPipeline(_ context.Context, filter executorparamdomain.ListFilter) ([]executorparamdomain.ExecutorParamDef, int64, error) {
	result := make([]executorparamdomain.ExecutorParamDef, 0, len(r.defs))
	for _, item := range r.defs {
		if strings.TrimSpace(filter.PipelineID) != "" && strings.TrimSpace(item.PipelineID) != strings.TrimSpace(filter.PipelineID) {
			continue
		}
		if filter.ExecutorType != "" && item.ExecutorType != filter.ExecutorType {
			continue
		}
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		if strings.TrimSpace(filter.ParamKey) != "" && strings.TrimSpace(item.ParamKey) != strings.TrimSpace(filter.ParamKey) {
			continue
		}
		result = append(result, item)
	}
	return result, int64(len(result)), nil
}

func (r *releasePrecheckParamRepoFake) ListByApplications(context.Context, executorparamdomain.ApplicationListFilter) ([]executorparamdomain.ExecutorParamDef, int64, error) {
	return nil, 0, nil
}

func (r *releasePrecheckParamRepoFake) GetByID(_ context.Context, id string) (executorparamdomain.ExecutorParamDef, error) {
	item, ok := r.defs[id]
	if !ok {
		return executorparamdomain.ExecutorParamDef{}, executorparamdomain.ErrNotFound
	}
	return item, nil
}

func (r *releasePrecheckParamRepoFake) UpdateParamKey(context.Context, string, string, time.Time) (executorparamdomain.ExecutorParamDef, error) {
	return executorparamdomain.ExecutorParamDef{}, nil
}

func (r *releasePrecheckParamRepoFake) CountByParamKey(context.Context, string) (int64, error) {
	return 0, nil
}

type releasePrecheckJenkinsFake struct {
	jobSets map[string]executorparamdomain.JenkinsJobParamSet
}

func (j *releasePrecheckJenkinsFake) TriggerBuild(context.Context, string, map[string]string) (string, error) {
	return "", nil
}

func (j *releasePrecheckJenkinsFake) GetQueueItem(context.Context, string) (string, bool, string, error) {
	return "", false, "", nil
}

func (j *releasePrecheckJenkinsFake) AbortQueueItem(context.Context, string) error { return nil }
func (j *releasePrecheckJenkinsFake) AbortBuild(context.Context, string) error     { return nil }

func (j *releasePrecheckJenkinsFake) GetBuildStages(context.Context, string) ([]domain.ReleaseOrderPipelineStage, error) {
	return nil, nil
}

func (j *releasePrecheckJenkinsFake) GetBuildStageLog(context.Context, string, string) (domain.ReleaseOrderPipelineStageLog, error) {
	return domain.ReleaseOrderPipelineStageLog{}, nil
}

func (j *releasePrecheckJenkinsFake) GetJobParamSet(_ context.Context, fullName string) (executorparamdomain.JenkinsJobParamSet, error) {
	item, ok := j.jobSets[fullName]
	if !ok {
		return executorparamdomain.JenkinsJobParamSet{}, pipelinedomain.ErrPipelineNotFound
	}
	return item, nil
}
