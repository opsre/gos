package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domain "gos/internal/domain/release"
)

type concurrentTestReleaseSettingsStore struct {
	ReleaseSettingsStore
	concurrency ReleaseConcurrencySettingsOutput
}

func (s *concurrentTestReleaseSettingsStore) LoadConcurrencySettings(context.Context) (ReleaseConcurrencySettingsOutput, error) {
	return s.concurrency, nil
}

type concurrentQueueOwnerRepository struct {
	domain.Repository
	lock     domain.ReleaseExecutionLock
	conflict domain.ReleaseOrder
}

func (r *concurrentQueueOwnerRepository) FindActiveExecutionLock(context.Context, string, string, time.Time) (domain.ReleaseExecutionLock, error) {
	return r.lock, nil
}

func (r *concurrentQueueOwnerRepository) FindActiveOrderByApplicationEnv(context.Context, string, string, string) (domain.ReleaseOrder, error) {
	return r.conflict, nil
}

func (r *concurrentQueueOwnerRepository) CountActiveOrdersByApplicationEnv(context.Context, string, string, string) (int, error) {
	return 1, nil
}

func (r *concurrentQueueOwnerRepository) AcquireExecutionLock(context.Context, domain.ReleaseExecutionLock, time.Time) (domain.ReleaseExecutionLock, bool, error) {
	return r.lock, true, nil
}

func TestBatchExecuteRequiresBatchName(t *testing.T) {
	t.Parallel()

	manager := &ReleaseOrderManager{}
	_, err := manager.BatchExecute(context.Background(), BatchExecuteReleaseOrdersInput{
		OrderIDs: []string{"order-a", "order-b"},
	})
	if err == nil {
		t.Fatal("BatchExecute returned nil error, want invalid input")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("BatchExecute error = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "并发名称") {
		t.Fatalf("BatchExecute error = %q, want mention 并发名称", err.Error())
	}
}

func TestConcurrentQueueOwnerResumesWhenItAlreadyOwnsExecutionLock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 7, 18, 15, 30, 0, 0, time.UTC)
	earlier := testReleaseOrder("ro-concurrent-earlier", "RO-CONCURRENT-EARLIER", domain.OrderStatusQueued, now.Add(-time.Second))
	earlier.IsConcurrent = true
	earlier.ConcurrentBatchNo = "CB-OWNER-RECOVERY"
	earlier.ConcurrentBatchName = "并发发布"
	earlier.ConcurrentBatchSeq = 1
	owner := testReleaseOrder("ro-concurrent-lock-owner", "RO-CONCURRENT-LOCK-OWNER", domain.OrderStatusQueued, now)
	owner.IsConcurrent = true
	owner.ConcurrentBatchNo = earlier.ConcurrentBatchNo
	owner.ConcurrentBatchName = earlier.ConcurrentBatchName
	owner.ConcurrentBatchSeq = 2
	expiredAt := now.Add(30 * time.Minute)
	lockKey := "app:app-1:env:prod"
	repo := &concurrentQueueOwnerRepository{
		conflict: earlier,
		lock: domain.ReleaseExecutionLock{
			ID:             "lock-owner-recovery",
			LockScope:      domain.ExecutionLockScopeApplicationEnv,
			LockKey:        lockKey,
			ApplicationID:  owner.ApplicationID,
			EnvCode:        owner.EnvCode,
			ReleaseOrderID: owner.ID,
			ReleaseOrderNo: owner.OrderNo,
			Status:         domain.ExecutionLockStatusActive,
			OwnerType:      "release_order",
			CreatedAt:      now,
			ExpiredAt:      &expiredAt,
		},
	}
	manager := NewReleaseOrderManager(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	manager.releaseSettings = &concurrentTestReleaseSettingsStore{concurrency: ReleaseConcurrencySettingsOutput{
		Enabled:          true,
		LockScope:        ReleaseConcurrencyLockScopeApplicationEnv,
		ConflictStrategy: ReleaseConcurrencyConflictStrategyReject,
		LockTimeoutSec:   1800,
	}}
	manager.now = func() time.Time { return now }

	guard, acquired, err := manager.ensureExecutionLock(ctx, owner, domain.ReleaseOrderExecution{}, nil)
	if err != nil {
		t.Fatalf("ensureExecutionLock failed: %v", err)
	}
	if !acquired {
		t.Fatalf("queue owner should resume with its existing lock: guard=%#v", guard)
	}
	if guard.LockKey != lockKey || guard.ConflictOrder != nil || guard.ConflictLock != nil {
		t.Fatalf("queue owner guard=%#v, want owned lock without conflict", guard)
	}
}

func TestConcurrentBatchConflictIsAcceptedIntoQueueEvenWithRejectStrategy(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 13, 0, 0, 0, time.UTC)
	owner := testReleaseOrder("ro-concurrent-owner", "RO-CONCURRENT-OWNER", domain.OrderStatusDeploying, now)
	owner.IsConcurrent = true
	owner.ConcurrentBatchNo = "CB-FIFO"
	owner.ConcurrentBatchName = "并发发布"
	owner.ConcurrentBatchSeq = 1
	waiting := testReleaseOrder("ro-concurrent-waiting", "RO-CONCURRENT-WAITING", domain.OrderStatusPending, now.Add(time.Second))
	waiting.IsConcurrent = true
	waiting.ConcurrentBatchNo = owner.ConcurrentBatchNo
	waiting.ConcurrentBatchName = owner.ConcurrentBatchName
	waiting.ConcurrentBatchSeq = 2
	if err := repo.Create(ctx, owner, nil, nil, nil); err != nil {
		t.Fatalf("Create owner failed: %v", err)
	}
	if err := repo.Create(ctx, waiting, nil, nil, nil); err != nil {
		t.Fatalf("Create waiting failed: %v", err)
	}

	guard, acquired, err := manager.ensureExecutionLock(ctx, waiting, domain.ReleaseOrderExecution{}, nil)
	if err != nil {
		t.Fatalf("ensureExecutionLock failed: %v", err)
	}
	if acquired {
		t.Fatal("acquired=true, want queued behind current batch owner")
	}
	if !guard.WaitingForLock || guard.ConflictOrder == nil || guard.ConflictOrder.ID != owner.ID {
		t.Fatalf("guard=%#v, want waiting conflict with owner", guard)
	}
	if !strings.Contains(guard.Message, "顺序等待队列") {
		t.Fatalf("message=%q, want queue confirmation", guard.Message)
	}
}

func TestQueuedFullReleaseRetainsContinuousCIDispatchMode(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 20, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	manager.pipelineRepo = segmentedReleasePipelineRepo{}
	jenkins := &segmentedReleaseCapturingJenkinsExecutor{}
	manager.jenkins = jenkins

	owner := testReleaseOrder("ro-full-release-owner", "RO-FULL-RELEASE-OWNER", domain.OrderStatusDeploying, now)
	owner.IsConcurrent = true
	owner.ConcurrentBatchNo = "CB-FULL-RELEASE"
	owner.ConcurrentBatchName = "Full release batch"
	owner.ConcurrentBatchSeq = 1
	waiting := testReleaseOrder("ro-full-release-waiting", "RO-FULL-RELEASE-WAITING", domain.OrderStatusPending, now.Add(time.Second))
	waiting.TemplateID = ""
	waiting.IsConcurrent = true
	waiting.ConcurrentBatchNo = owner.ConcurrentBatchNo
	waiting.ConcurrentBatchName = owner.ConcurrentBatchName
	waiting.ConcurrentBatchSeq = 2
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(waiting.ID, "exec-waiting-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
		testReleaseExecution(waiting.ID, "exec-waiting-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}
	steps := defaultReleaseOrderSteps(waiting.ID, executions, now, "", nil, waiting.EnvCode)
	if err := repo.Create(ctx, owner, nil, nil, nil); err != nil {
		t.Fatalf("Create owner failed: %v", err)
	}
	if err := repo.Create(ctx, waiting, executions, nil, steps); err != nil {
		t.Fatalf("Create waiting order failed: %v", err)
	}

	dispatched, err := manager.Execute(ctx, waiting.ID, "tester", "Tester")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if dispatched.Status != domain.OrderStatusDeploying {
		t.Fatalf("queued full release status=%s, want %s to retain continuous mode", dispatched.Status, domain.OrderStatusDeploying)
	}
	if jenkins.triggerCount != 0 {
		t.Fatalf("jenkins trigger count=%d, want 0 while the order is queued", jenkins.triggerCount)
	}

	if _, err := repo.UpdateStatus(ctx, owner.ID, domain.OrderStatusSuccess, nil, &now, now); err != nil {
		t.Fatalf("finish owner failed: %v", err)
	}
	storedExecutions, err := repo.ListExecutions(ctx, waiting.ID)
	if err != nil {
		t.Fatalf("ListExecutions failed: %v", err)
	}
	if err := manager.startNextPendingExecution(ctx, dispatched, storedExecutions, nil); err != nil {
		t.Fatalf("startNextPendingExecution failed: %v", err)
	}
	if jenkins.triggerCount != 1 {
		t.Fatalf("jenkins trigger count=%d, want 1 CI trigger after queue clears", jenkins.triggerCount)
	}
	storedOrder, err := repo.GetByID(ctx, waiting.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if storedOrder.Status != domain.OrderStatusDeploying {
		t.Fatalf("running full release status=%s, want %s", storedOrder.Status, domain.OrderStatusDeploying)
	}
	storedExecutions, err = repo.ListExecutions(ctx, waiting.ID)
	if err != nil {
		t.Fatalf("ListExecutions after start failed: %v", err)
	}
	if runningCI := findExecutionByScopeAndStatus(storedExecutions, domain.PipelineScopeCI, domain.ExecutionStatusRunning); runningCI == nil {
		t.Fatalf("stored executions=%#v, want running CI", storedExecutions)
	}
}

func TestConcurrentBatchQueueStateTreatsUnstartedFullReleaseAsQueued(t *testing.T) {
	t.Parallel()

	if got := resolveConcurrentBatchQueueState(domain.OrderStatusDeploying, false); got != ReleaseOrderConcurrentBatchQueueStateQueued {
		t.Fatalf("queue state=%s, want %s", got, ReleaseOrderConcurrentBatchQueueStateQueued)
	}
	if got := resolveConcurrentBatchQueueState(domain.OrderStatusDeploying, true); got != ReleaseOrderConcurrentBatchQueueStateExecuting {
		t.Fatalf("running state=%s, want %s", got, ReleaseOrderConcurrentBatchQueueStateExecuting)
	}
}
