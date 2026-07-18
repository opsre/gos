package usecase

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domain "gos/internal/domain/release"
)

type realtimeTrackJenkinsFake struct {
	calls atomic.Int32
}

type blockingRealtimeTrackJenkins struct {
	segmentedReleaseNoopJenkinsExecutor
	statusStarted chan struct{}
	releaseStatus chan struct{}
	abortCalls    atomic.Int32
}

func (f *blockingRealtimeTrackJenkins) GetBuildStatus(context.Context, string) (bool, string, error) {
	close(f.statusStarted)
	<-f.releaseStatus
	return true, "", nil
}

func (f *blockingRealtimeTrackJenkins) AbortBuild(context.Context, string) error {
	f.abortCalls.Add(1)
	return nil
}

func (f *realtimeTrackJenkinsFake) GetQueueItem(context.Context, string) (string, bool, string, error) {
	f.calls.Add(1)
	return "", false, "", nil
}

func (f *realtimeTrackJenkinsFake) GetBuildStatus(context.Context, string) (bool, string, error) {
	f.calls.Add(1)
	return true, "", nil
}

func TestRealtimeSyncOrderDoesNotDispatchManualOrApprovalStates(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	jenkins := &realtimeTrackJenkinsFake{}
	tracker := NewTrackReleaseExecution(manager, jenkins)

	statuses := []domain.OrderStatus{
		domain.OrderStatusDraft,
		domain.OrderStatusPendingApproval,
		domain.OrderStatusApproving,
		domain.OrderStatusApproved,
		domain.OrderStatusBuiltWaitingDeploy,
	}
	for index, status := range statuses {
		order := testReleaseOrder(
			"ro-realtime-readonly-"+string(rune('a'+index)),
			"RO-REALTIME-READONLY-"+string(rune('A'+index)),
			status,
			now,
		)
		executions := []domain.ReleaseOrderExecution{
			testReleaseExecution(order.ID, "exec-"+order.ID, domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
		}
		if err := repo.Create(ctx, order, executions, nil, nil); err != nil {
			t.Fatalf("Create(%s) failed: %v", status, err)
		}
		if err := tracker.SyncOrder(ctx, order.ID); err != nil {
			t.Fatalf("SyncOrder(%s) failed: %v", status, err)
		}

		stored, err := repo.GetByID(ctx, order.ID)
		if err != nil {
			t.Fatalf("GetByID(%s) failed: %v", status, err)
		}
		if stored.Status != status {
			t.Fatalf("status after realtime sync=%s, want %s", stored.Status, status)
		}
		storedExecutions, err := repo.ListExecutions(ctx, order.ID)
		if err != nil {
			t.Fatalf("ListExecutions(%s) failed: %v", status, err)
		}
		if len(storedExecutions) != 1 || storedExecutions[0].Status != domain.ExecutionStatusPending {
			t.Fatalf("execution after realtime sync(%s)=%#v, want pending", status, storedExecutions)
		}
	}
	if jenkins.calls.Load() != 0 {
		t.Fatalf("external Jenkins calls=%d, want 0", jenkins.calls.Load())
	}
}

func TestReleaseTrackerSerializesSameOrderSync(t *testing.T) {
	t.Parallel()

	manager := &ReleaseOrderManager{}
	var active atomic.Int32
	var maxActive atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := manager.lockOrderOperation("ro-serialized")
			current := active.Add(1)
			for {
				previous := maxActive.Load()
				if current <= previous || maxActive.CompareAndSwap(previous, current) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			active.Add(-1)
			unlock()
		}()
	}
	wg.Wait()
	if maxActive.Load() != 1 {
		t.Fatalf("max concurrent same-order sync=%d, want 1", maxActive.Load())
	}
}

func TestCancelWaitsForTrackerExternalReadAndRemainsCancelled(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx, cancelContext := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelContext()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	jenkins := &blockingRealtimeTrackJenkins{
		statusStarted: make(chan struct{}),
		releaseStatus: make(chan struct{}),
	}
	manager.jenkins = jenkins
	tracker := NewTrackReleaseExecution(manager, jenkins)
	tracker.now = func() time.Time { return now }

	order := testReleaseOrder("ro-cancel-track-race", "RO-CANCEL-TRACK-RACE", domain.OrderStatusRunning, now)
	execution := testReleaseExecution(order.ID, "exec-cancel-track-race", domain.PipelineScopeCI, domain.ExecutionStatusRunning, now)
	execution.BuildURL = "https://jenkins.example/job/demo/9/"
	step := testReleaseStep(order.ID, "step-cancel-track-race", domain.StepScopeCI, "ci:pipeline_running", domain.StepStatusRunning, 1, now)
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, []domain.ReleaseOrderStep{step}); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	trackDone := make(chan error, 1)
	go func() {
		trackDone <- tracker.SyncOrder(ctx, order.ID)
	}()
	select {
	case <-jenkins.statusStarted:
	case <-ctx.Done():
		t.Fatal("tracker did not reach external status read")
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
		t.Fatalf("Cancel returned before tracker released order lock: order=%#v err=%v", result.order, result.err)
	case <-time.After(30 * time.Millisecond):
	}

	close(jenkins.releaseStatus)
	if err := <-trackDone; err != nil {
		t.Fatalf("SyncOrder failed: %v", err)
	}
	result := <-cancelDone
	if result.err != nil {
		t.Fatalf("Cancel failed: %v", result.err)
	}
	if result.order.Status != domain.OrderStatusCancelled {
		t.Fatalf("Cancel status=%s, want cancelled", result.order.Status)
	}
	if jenkins.abortCalls.Load() != 1 {
		t.Fatalf("AbortBuild calls=%d, want 1", jenkins.abortCalls.Load())
	}

	stored, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.Status != domain.OrderStatusCancelled {
		t.Fatalf("stored status=%s, want cancelled", stored.Status)
	}
	storedExecutions, err := repo.ListExecutions(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListExecutions failed: %v", err)
	}
	if len(storedExecutions) != 1 || storedExecutions[0].Status != domain.ExecutionStatusCancelled {
		t.Fatalf("stored executions=%#v, want cancelled", storedExecutions)
	}
}
