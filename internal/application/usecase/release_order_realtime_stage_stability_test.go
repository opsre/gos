package usecase

import (
	"context"
	"testing"
	"time"

	pipelinedomain "gos/internal/domain/pipeline"
	domain "gos/internal/domain/release"
)

type countingPipelineStageRepository struct {
	domain.Repository
	replaceCalls int
}

func (r *countingPipelineStageRepository) ReplacePipelineStages(
	ctx context.Context,
	orderID string,
	stages []domain.ReleaseOrderPipelineStage,
) error {
	r.replaceCalls++
	return r.Repository.ReplacePipelineStages(ctx, orderID, stages)
}

type stablePipelineStageJenkins struct {
	segmentedReleaseNoopJenkinsExecutor
	stages []domain.ReleaseOrderPipelineStage
}

func (j stablePipelineStageJenkins) GetBuildStages(context.Context, string) ([]domain.ReleaseOrderPipelineStage, error) {
	return append([]domain.ReleaseOrderPipelineStage(nil), j.stages...), nil
}

func TestRefreshPipelineStagesSkipsStableTerminalRewrite(t *testing.T) {
	t.Parallel()

	manager, repository := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	firstNow := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	finishedAt := firstNow.Add(-time.Minute)
	startedAt := finishedAt.Add(-2 * time.Minute)
	order := testReleaseOrder("ro-stage-stable", "RO-STAGE-STABLE", domain.OrderStatusSuccess, firstNow)
	execution := testReleaseExecution(order.ID, "exec-stage-stable", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, firstNow)
	execution.BuildURL = "https://jenkins.example/job/demo/1/"
	if err := repository.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	countingRepo := &countingPipelineStageRepository{Repository: repository}
	manager.repo = countingRepo
	manager.now = func() time.Time { return firstNow }
	manager.jenkins = stablePipelineStageJenkins{
		stages: []domain.ReleaseOrderPipelineStage{
			{
				StageKey:       "build",
				StageName:      "Build",
				Status:         domain.PipelineStageStatusSuccess,
				RawStatus:      "SUCCESS",
				SortNo:         1,
				DurationMillis: 120000,
				StartedAt:      &startedAt,
				FinishedAt:     &finishedAt,
			},
		},
	}
	binding := pipelinedomain.PipelineBinding{Provider: pipelinedomain.ProviderJenkins}
	if _, err := manager.refreshPipelineStages(ctx, order, execution, binding); err != nil {
		t.Fatalf("first refreshPipelineStages failed: %v", err)
	}
	if countingRepo.replaceCalls != 1 {
		t.Fatalf("first replace calls=%d, want 1", countingRepo.replaceCalls)
	}
	firstStages, err := repository.ListPipelineStages(ctx, order.ID)
	if err != nil || len(firstStages) != 1 {
		t.Fatalf("first ListPipelineStages=%#v err=%v", firstStages, err)
	}

	manager.now = func() time.Time { return firstNow.Add(2 * time.Second) }
	if _, err := manager.refreshPipelineStages(ctx, order, execution, binding); err != nil {
		t.Fatalf("second refreshPipelineStages failed: %v", err)
	}
	if countingRepo.replaceCalls != 1 {
		t.Fatalf("stable refresh replace calls=%d, want unchanged 1", countingRepo.replaceCalls)
	}
	secondStages, err := repository.ListPipelineStages(ctx, order.ID)
	if err != nil || len(secondStages) != 1 {
		t.Fatalf("second ListPipelineStages=%#v err=%v", secondStages, err)
	}
	if !secondStages[0].CreatedAt.Equal(firstStages[0].CreatedAt) || !secondStages[0].UpdatedAt.Equal(firstStages[0].UpdatedAt) {
		t.Fatalf("stable stage timestamps changed: first=%#v second=%#v", firstStages[0], secondStages[0])
	}
}

func TestSyncPipelineStageFromStepSkipsUnchangedRewrite(t *testing.T) {
	t.Parallel()

	manager, repository := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	startedAt := now.Add(-2 * time.Minute)
	finishedAt := now.Add(-time.Minute)
	order := testReleaseOrder("ro-stage-step-stable", "RO-STAGE-STEP-STABLE", domain.OrderStatusSuccess, now)
	if err := repository.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	stage := domain.ReleaseOrderPipelineStage{
		ID:             "stage-step-stable",
		ReleaseOrderID: order.ID,
		PipelineScope:  string(domain.PipelineScopeCD),
		ExecutorType:   string(pipelinedomain.ProviderArgoCD),
		StageKey:       "argocd_sync",
		StageName:      "CD trigger ArgoCD",
		Status:         domain.PipelineStageStatusSuccess,
		RawStatus:      "synced",
		SortNo:         1,
		DurationMillis: finishedAt.Sub(startedAt).Milliseconds(),
		StartedAt:      &startedAt,
		FinishedAt:     &finishedAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repository.ReplacePipelineStages(ctx, order.ID, []domain.ReleaseOrderPipelineStage{stage}); err != nil {
		t.Fatalf("seed ReplacePipelineStages failed: %v", err)
	}
	countingRepo := &countingPipelineStageRepository{Repository: repository}
	manager.repo = countingRepo
	manager.now = func() time.Time { return now.Add(2 * time.Second) }

	if err := manager.syncPipelineStageFromStep(
		ctx,
		order.ID,
		"cd:argocd_sync",
		domain.StepStatusSuccess,
		"synced",
		&startedAt,
		&finishedAt,
	); err != nil {
		t.Fatalf("syncPipelineStageFromStep failed: %v", err)
	}
	if countingRepo.replaceCalls != 0 {
		t.Fatalf("unchanged step sync replace calls=%d, want 0", countingRepo.replaceCalls)
	}
	stored, err := repository.ListPipelineStages(ctx, order.ID)
	if err != nil || len(stored) != 1 {
		t.Fatalf("ListPipelineStages=%#v err=%v", stored, err)
	}
	if !stored[0].UpdatedAt.Equal(now) {
		t.Fatalf("updated_at=%v, want preserved %v", stored[0].UpdatedAt, now)
	}
}
