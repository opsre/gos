package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	pipelinedomain "gos/internal/domain/pipeline"
	domain "gos/internal/domain/release"
)

func TestGetPipelineStageLogMapsJenkinsStageLogError(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	manager.jenkins = stageLogFailingJenkinsExecutor{
		err: fmt.Errorf("jenkins request failed: status=404"),
	}

	order := testReleaseOrder("ro-stage-log", "RO-STAGE-LOG", domain.OrderStatusBuilding, now)
	execution := testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusRunning, now)
	execution.BuildURL = "http://jenkins.example/job/demo/1/"
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	stage := domain.ReleaseOrderPipelineStage{
		ID:             "rps-stage",
		ReleaseOrderID: order.ID,
		ExecutionID:    execution.ID,
		PipelineScope:  string(domain.PipelineScopeCI),
		ExecutorType:   "jenkins",
		StageKey:       "7",
		StageName:      "构建",
		Status:         domain.PipelineStageStatusRunning,
		SortNo:         1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.ReplacePipelineStages(ctx, order.ID, []domain.ReleaseOrderPipelineStage{stage}); err != nil {
		t.Fatalf("ReplacePipelineStages failed: %v", err)
	}

	_, _, err := manager.GetPipelineStageLog(ctx, order.ID, stage.ID)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("GetPipelineStageLog error = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "Jenkins 阶段日志暂不可用") {
		t.Fatalf("GetPipelineStageLog error = %q, want stage-log unavailable message", err.Error())
	}
}

func TestGetPipelineStageLogPassesStageNameToNamedJenkinsExecutor(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	jenkins := &stageLogNamedJenkinsExecutor{}
	manager.jenkins = jenkins

	order := testReleaseOrder("ro-stage-log-name", "RO-STAGE-LOG-NAME", domain.OrderStatusBuilding, now)
	execution := testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusRunning, now)
	execution.BuildURL = "http://jenkins.example/job/demo/1/"
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	stage := domain.ReleaseOrderPipelineStage{
		ID:             "rps-stage-name",
		ReleaseOrderID: order.ID,
		ExecutionID:    execution.ID,
		PipelineScope:  string(domain.PipelineScopeCI),
		ExecutorType:   "jenkins",
		StageKey:       "7",
		StageName:      "Upload OSS",
		Status:         domain.PipelineStageStatusFailed,
		SortNo:         1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.ReplacePipelineStages(ctx, order.ID, []domain.ReleaseOrderPipelineStage{stage}); err != nil {
		t.Fatalf("ReplacePipelineStages failed: %v", err)
	}

	_, stageLog, err := manager.GetPipelineStageLog(ctx, order.ID, stage.ID)
	if err != nil {
		t.Fatalf("GetPipelineStageLog failed: %v", err)
	}
	if jenkins.stageName != "Upload OSS" {
		t.Fatalf("stage name passed to Jenkins = %q, want Upload OSS", jenkins.stageName)
	}
	if stageLog.Content != "scoped stage log" {
		t.Fatalf("stage log content = %q, want scoped stage log", stageLog.Content)
	}
}

func TestGetPipelineStageLogReturnsArgoCDStepLog(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }

	order := testReleaseOrder("ro-argocd-stage-log", "RO-ARGOCD-STAGE-LOG", domain.OrderStatusFailed, now)
	execution := testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusFailed, now)
	execution.Provider = string(pipelinedomain.ProviderArgoCD)
	execution.BindingName = "ArgoCD"
	execution.PipelineID = ""

	startedAt := now.Add(-time.Minute)
	finishedAt := now
	step := testReleaseStep(
		order.ID,
		"step-gitops-update",
		domain.StepScopeCD,
		scopeStepCode(domain.PipelineScopeCD, "gitops_update"),
		domain.StepStatusFailed,
		1,
		now,
	)
	step.ExecutionID = execution.ID
	step.StepName = "CD 更新 Helm Values"
	step.Message = "Helm values 写回失败: mkdir /gitops: read-only file system"
	step.StartedAt = &startedAt
	step.FinishedAt = &finishedAt

	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, []domain.ReleaseOrderStep{step}); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	stage := domain.ReleaseOrderPipelineStage{
		ID:             "rps-argocd-gitops-update",
		ReleaseOrderID: order.ID,
		ExecutionID:    execution.ID,
		PipelineScope:  string(domain.PipelineScopeCD),
		ExecutorType:   string(pipelinedomain.ProviderArgoCD),
		StageKey:       "gitops_update",
		StageName:      "CD 更新 Helm Values",
		Status:         domain.PipelineStageStatusFailed,
		RawStatus:      step.Message,
		SortNo:         1,
		StartedAt:      &startedAt,
		FinishedAt:     &finishedAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.ReplacePipelineStages(ctx, order.ID, []domain.ReleaseOrderPipelineStage{stage}); err != nil {
		t.Fatalf("ReplacePipelineStages failed: %v", err)
	}

	_, stageLog, err := manager.GetPipelineStageLog(ctx, order.ID, stage.ID)
	if err != nil {
		t.Fatalf("GetPipelineStageLog failed: %v", err)
	}
	if !strings.Contains(stageLog.Content, "Helm values 写回失败: mkdir /gitops: read-only file system") {
		t.Fatalf("stage log content = %q, want ArgoCD step failure message", stageLog.Content)
	}
	if stageLog.ExecutorType != string(pipelinedomain.ProviderArgoCD) {
		t.Fatalf("stage log executor = %q, want argocd", stageLog.ExecutorType)
	}
	if stageLog.HasMore {
		t.Fatalf("stage log HasMore = true, want false for ArgoCD step snapshot log")
	}
}

type stageLogFailingJenkinsExecutor struct {
	segmentedReleaseNoopJenkinsExecutor
	err error
}

func (e stageLogFailingJenkinsExecutor) GetBuildStageLog(context.Context, string, string) (domain.ReleaseOrderPipelineStageLog, error) {
	return domain.ReleaseOrderPipelineStageLog{}, e.err
}

type stageLogNamedJenkinsExecutor struct {
	segmentedReleaseNoopJenkinsExecutor
	stageName string
}

func (e *stageLogNamedJenkinsExecutor) GetBuildStageLog(context.Context, string, string) (domain.ReleaseOrderPipelineStageLog, error) {
	return domain.ReleaseOrderPipelineStageLog{}, fmt.Errorf("legacy stage log method should not be used")
}

func (e *stageLogNamedJenkinsExecutor) GetBuildStageLogWithName(
	_ context.Context,
	_ string,
	_ string,
	stageName string,
) (domain.ReleaseOrderPipelineStageLog, error) {
	e.stageName = stageName
	return domain.ReleaseOrderPipelineStageLog{Content: "scoped stage log"}, nil
}
