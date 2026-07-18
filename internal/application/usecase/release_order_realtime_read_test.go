package usecase

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	domain "gos/internal/domain/release"
)

type realtimeReadOnlySpyRepository struct {
	domain.Repository
	updateStatusCalls    atomic.Int32
	updateExecutionCalls atomic.Int32
	updateStepCalls      atomic.Int32
	replaceStageCalls    atomic.Int32
}

type blockingRealtimeReadRepository struct {
	domain.Repository
	listExecutionCalls atomic.Int32
	readStarted        chan struct{}
	releaseRead        chan struct{}
}

func (r *blockingRealtimeReadRepository) ListExecutions(
	ctx context.Context,
	orderID string,
) ([]domain.ReleaseOrderExecution, error) {
	if r.listExecutionCalls.Add(1) == 1 {
		close(r.readStarted)
		<-r.releaseRead
	}
	return r.Repository.ListExecutions(ctx, orderID)
}

func (r *realtimeReadOnlySpyRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status domain.OrderStatus,
	startedAt *time.Time,
	finishedAt *time.Time,
	updatedAt time.Time,
) (domain.ReleaseOrder, error) {
	r.updateStatusCalls.Add(1)
	return r.Repository.UpdateStatus(ctx, id, status, startedAt, finishedAt, updatedAt)
}

func (r *realtimeReadOnlySpyRepository) UpdateExecutionByScope(
	ctx context.Context,
	orderID string,
	scope domain.PipelineScope,
	input domain.ExecutionUpdateInput,
) (domain.ReleaseOrderExecution, error) {
	r.updateExecutionCalls.Add(1)
	return r.Repository.UpdateExecutionByScope(ctx, orderID, scope, input)
}

func (r *realtimeReadOnlySpyRepository) UpdateStep(
	ctx context.Context,
	orderID string,
	stepCode string,
	input domain.StepUpdateInput,
) (domain.ReleaseOrderStep, error) {
	r.updateStepCalls.Add(1)
	return r.Repository.UpdateStep(ctx, orderID, stepCode, input)
}

func (r *realtimeReadOnlySpyRepository) ReplacePipelineStages(
	ctx context.Context,
	orderID string,
	stages []domain.ReleaseOrderPipelineStage,
) error {
	r.replaceStageCalls.Add(1)
	return r.Repository.ReplacePipelineStages(ctx, orderID, stages)
}

func TestLoadStoredReleaseOrderRealtimeAggregatePerformsNoReconcileWrites(t *testing.T) {
	t.Parallel()

	manager, repository := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 11, 0, 0, 0, time.UTC)
	finishedAt := now.Add(-time.Minute)
	order := testReleaseOrder("ro-realtime-read-only", "RO-REALTIME-READ-ONLY", domain.OrderStatusRunning, now)
	order.TemplateID = ""
	order.TemplateName = ""
	execution := testReleaseExecution(order.ID, "exec-realtime-read-only", domain.PipelineScopeCD, domain.ExecutionStatusPending, now)
	execution.Provider = "argocd"
	step := testReleaseStep(
		order.ID,
		"step-realtime-health",
		domain.StepScopeCD,
		"cd:health_check",
		domain.StepStatusPending,
		1,
		now,
	)
	if err := repository.Create(
		ctx,
		order,
		[]domain.ReleaseOrderExecution{execution},
		nil,
		[]domain.ReleaseOrderStep{step},
	); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	stage := domain.ReleaseOrderPipelineStage{
		ID:             "stage-realtime-health",
		ReleaseOrderID: order.ID,
		ExecutionID:    execution.ID,
		PipelineScope:  string(domain.PipelineScopeCD),
		ExecutorType:   "argocd",
		StageKey:       "health_check",
		StageName:      "CD health check",
		Status:         domain.PipelineStageStatusSuccess,
		RawStatus:      "Healthy",
		SortNo:         1,
		StartedAt:      execution.StartedAt,
		FinishedAt:     &finishedAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repository.ReplacePipelineStages(ctx, order.ID, []domain.ReleaseOrderPipelineStage{stage}); err != nil {
		t.Fatalf("seed ReplacePipelineStages failed: %v", err)
	}

	spy := &realtimeReadOnlySpyRepository{Repository: repository}
	manager.repo = spy
	aggregate, err := manager.LoadStoredReleaseOrderRealtimeAggregate(ctx, order.ID)
	if err != nil {
		t.Fatalf("LoadStoredReleaseOrderRealtimeAggregate failed: %v", err)
	}
	if aggregate.Order.Status != domain.OrderStatusRunning {
		t.Fatalf("aggregate order status=%s, want raw running", aggregate.Order.Status)
	}
	if len(aggregate.Executions) != 1 || aggregate.Executions[0].Status != domain.ExecutionStatusPending {
		t.Fatalf("aggregate executions=%#v, want raw pending", aggregate.Executions)
	}
	if len(aggregate.Steps) != 1 || aggregate.Steps[0].Status != domain.StepStatusPending {
		t.Fatalf("aggregate steps=%#v, want raw pending", aggregate.Steps)
	}
	if _, err := manager.GetStoredReleaseOrderByID(ctx, order.ID); err != nil {
		t.Fatalf("GetStoredReleaseOrderByID failed: %v", err)
	}
	if spy.updateStatusCalls.Load() != 0 ||
		spy.updateExecutionCalls.Load() != 0 ||
		spy.updateStepCalls.Load() != 0 ||
		spy.replaceStageCalls.Load() != 0 {
		t.Fatalf(
			"realtime read wrote repository: status=%d execution=%d step=%d stages=%d",
			spy.updateStatusCalls.Load(),
			spy.updateExecutionCalls.Load(),
			spy.updateStepCalls.Load(),
			spy.replaceStageCalls.Load(),
		)
	}

	storedOrder, err := repository.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("stored GetByID failed: %v", err)
	}
	storedExecutions, err := repository.ListExecutions(ctx, order.ID)
	if err != nil {
		t.Fatalf("stored ListExecutions failed: %v", err)
	}
	storedSteps, err := repository.ListSteps(ctx, order.ID)
	if err != nil {
		t.Fatalf("stored ListSteps failed: %v", err)
	}
	if storedOrder.Status != domain.OrderStatusRunning ||
		len(storedExecutions) != 1 || storedExecutions[0].Status != domain.ExecutionStatusPending ||
		len(storedSteps) != 1 || storedSteps[0].Status != domain.StepStatusPending {
		t.Fatalf("stored state was reconciled by read: order=%#v executions=%#v steps=%#v", storedOrder, storedExecutions, storedSteps)
	}
}

func TestPublicReleaseOrderReadsDoNotReconcileOrFinalize(t *testing.T) {
	t.Parallel()

	manager, repository := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	startedAt := now.Add(-time.Minute)
	order := testReleaseOrder("ro-public-read-only", "RO-PUBLIC-READ-ONLY", domain.OrderStatusRunning, now)
	order.StartedAt = &startedAt
	execution := testReleaseExecution(order.ID, "exec-public-read-only", domain.PipelineScopeCD, domain.ExecutionStatusPending, now)
	steps := []domain.ReleaseOrderStep{
		testReleaseStep(order.ID, "step-public-health", domain.StepScopeCD, "cd:health_check", domain.StepStatusSuccess, 1, now),
		testReleaseStep(order.ID, "step-public-hook", domain.StepScopeGlobal, "hook:post_release:agent_task:1", domain.StepStatusFailed, 2, now),
	}
	if err := repository.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, steps); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	spy := &realtimeReadOnlySpyRepository{Repository: repository}
	manager.repo = spy
	got, err := manager.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != domain.OrderStatusRunning {
		t.Fatalf("GetByID status=%s, want raw running", got.Status)
	}
	items, _, err := manager.List(ctx, ListReleaseOrderInput{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 1 || items[0].Status != domain.OrderStatusRunning {
		t.Fatalf("List items=%#v, want raw running order", items)
	}
	executions, err := manager.ListExecutions(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListExecutions failed: %v", err)
	}
	if len(executions) != 1 || executions[0].Status != domain.ExecutionStatusPending {
		t.Fatalf("ListExecutions=%#v, want raw pending", executions)
	}
	if _, err := manager.ListSteps(ctx, order.ID); err != nil {
		t.Fatalf("ListSteps failed: %v", err)
	}
	if spy.updateStatusCalls.Load() != 0 ||
		spy.updateExecutionCalls.Load() != 0 ||
		spy.updateStepCalls.Load() != 0 ||
		spy.replaceStageCalls.Load() != 0 {
		t.Fatalf(
			"public read wrote repository: status=%d execution=%d step=%d stages=%d",
			spy.updateStatusCalls.Load(),
			spy.updateExecutionCalls.Load(),
			spy.updateStepCalls.Load(),
			spy.replaceStageCalls.Load(),
		)
	}
}

func TestStoredRealtimeAggregateSerializesWithCancel(t *testing.T) {
	t.Parallel()

	manager, repository := newReleaseOrderManagerForCancelTest(t)
	ctx, cancelContext := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelContext()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	order := testReleaseOrder("ro-realtime-read-cancel", "RO-REALTIME-READ-CANCEL", domain.OrderStatusRunning, now)
	order.TemplateID = ""
	order.TemplateName = ""
	execution := testReleaseExecution(order.ID, "exec-realtime-read-cancel", domain.PipelineScopeCI, domain.ExecutionStatusPending, now)
	if err := repository.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	blockingRepo := &blockingRealtimeReadRepository{
		Repository:  repository,
		readStarted: make(chan struct{}),
		releaseRead: make(chan struct{}),
	}
	manager.repo = blockingRepo
	aggregateDone := make(chan error, 1)
	go func() {
		_, err := manager.LoadStoredReleaseOrderRealtimeAggregate(ctx, order.ID)
		aggregateDone <- err
	}()
	select {
	case <-blockingRepo.readStarted:
	case <-ctx.Done():
		t.Fatal("aggregate did not reach blocked storage read")
	}

	type cancelResult struct {
		order domain.ReleaseOrder
		err   error
	}
	cancelDone := make(chan cancelResult, 1)
	go func() {
		item, err := manager.Cancel(ctx, order.ID)
		cancelDone <- cancelResult{order: item, err: err}
	}()
	select {
	case result := <-cancelDone:
		t.Fatalf("Cancel returned during locked aggregate read: order=%#v err=%v", result.order, result.err)
	case <-time.After(30 * time.Millisecond):
	}

	close(blockingRepo.releaseRead)
	if err := <-aggregateDone; err != nil {
		t.Fatalf("LoadStoredReleaseOrderRealtimeAggregate failed: %v", err)
	}
	result := <-cancelDone
	if result.err != nil {
		t.Fatalf("Cancel failed: %v", result.err)
	}
	if result.order.Status != domain.OrderStatusCancelled {
		t.Fatalf("cancel result status=%s, want cancelled", result.order.Status)
	}
	stored, err := repository.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.Status != domain.OrderStatusCancelled {
		t.Fatalf("stored status=%s, want cancelled", stored.Status)
	}
}
