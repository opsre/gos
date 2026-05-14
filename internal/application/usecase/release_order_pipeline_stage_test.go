package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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
