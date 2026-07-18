package usecase

import (
	"context"
	"database/sql"
	"testing"
	"time"

	domain "gos/internal/domain/release"
	"gos/internal/infrastructure/persistence/sqlrepo"

	_ "modernc.org/sqlite"
)

// TestCancelPendingOrderMarksExecutionsCancelled 封装当前模块的业务处理逻辑。
func TestCancelPendingOrderMarksExecutionsCancelled(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }

	order := testReleaseOrder("ro-pending", "RO-PENDING", domain.OrderStatusPending, now)
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
	}
	steps := []domain.ReleaseOrderStep{
		testReleaseStep(order.ID, "step-ci", domain.StepScopeCI, "ci:pipeline_running", domain.StepStatusPending, 10, now),
		testReleaseStep(order.ID, "step-finish", domain.StepScopeGlobal, "global:release_finish", domain.StepStatusPending, 99, now),
	}
	if err := repo.Create(ctx, order, executions, nil, steps); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	cancelled, err := manager.Cancel(ctx, order.ID)
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
	if cancelled.Status != domain.OrderStatusCancelled {
		t.Fatalf("cancelled order status = %s, want %s", cancelled.Status, domain.OrderStatusCancelled)
	}

	storedExecutions, err := repo.ListExecutions(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListExecutions failed: %v", err)
	}
	if len(storedExecutions) != 1 || storedExecutions[0].Status != domain.ExecutionStatusCancelled {
		t.Fatalf("stored executions = %#v, want cancelled", storedExecutions)
	}
}

// TestCancelRunningOrderMarksAllNonTerminalExecutionsCancelled 封装当前模块的业务处理逻辑。
func TestCancelRunningOrderMarksAllNonTerminalExecutionsCancelled(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	startedAt := now.Add(-2 * time.Minute)
	manager.now = func() time.Time { return now }

	order := testReleaseOrder("ro-running", "RO-RUNNING", domain.OrderStatusDeploying, now)
	order.StartedAt = &startedAt
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusRunning, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}
	if err := repo.Create(ctx, order, executions, nil, nil); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	cancelled, err := manager.Cancel(ctx, order.ID)
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
	if cancelled.Status != domain.OrderStatusCancelled {
		t.Fatalf("cancelled order status = %s, want %s", cancelled.Status, domain.OrderStatusCancelled)
	}

	storedExecutions, err := repo.ListExecutions(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListExecutions failed: %v", err)
	}
	for _, item := range storedExecutions {
		if item.Status != domain.ExecutionStatusCancelled {
			t.Fatalf("execution %s status = %s, want %s", item.ID, item.Status, domain.ExecutionStatusCancelled)
		}
	}
}

// TestSyncCancelledOrderDoesNotRevivePendingExecution 同步外部或内部状态数据。
func TestSyncCancelledOrderDoesNotRevivePendingExecution(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }

	order := testReleaseOrder("ro-cancelled", "RO-CANCELLED", domain.OrderStatusCancelled, now)
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
	}
	steps := []domain.ReleaseOrderStep{
		testReleaseStep(order.ID, "step-ci", domain.StepScopeCI, "ci:pipeline_running", domain.StepStatusPending, 10, now),
	}
	if err := repo.Create(ctx, order, executions, nil, steps); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	tracker := NewTrackReleaseExecution(manager, nil)
	tracker.now = func() time.Time { return now }
	updated, skipped, err := tracker.syncOrder(ctx, order)
	if err != nil {
		t.Fatalf("syncOrder failed: %v", err)
	}
	if skipped {
		t.Fatal("syncOrder skipped = true, want false")
	}
	if !updated {
		t.Fatal("syncOrder updated = false, want true")
	}

	storedOrder, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if storedOrder.Status != domain.OrderStatusCancelled {
		t.Fatalf("stored order status = %s, want %s", storedOrder.Status, domain.OrderStatusCancelled)
	}
	storedExecutions, err := repo.ListExecutions(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListExecutions failed: %v", err)
	}
	if len(storedExecutions) != 1 || storedExecutions[0].Status != domain.ExecutionStatusCancelled {
		t.Fatalf("stored executions = %#v, want cancelled", storedExecutions)
	}
}

// newReleaseOrderManagerForCancelTest 封装当前模块的业务处理逻辑。
func newReleaseOrderManagerForCancelTest(t *testing.T) (*ReleaseOrderManager, *sqlrepo.ReleaseRepository) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS sys_user (
	id TEXT PRIMARY KEY,
	username TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	email TEXT NOT NULL DEFAULT '',
	phone TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'active',
	password_hash TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`); err != nil {
		t.Fatalf("create sys_user failed: %v", err)
	}

	repo := sqlrepo.NewReleaseRepository(db, "sqlite")
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	if err := sqlrepo.NewProjectRepository(db, "sqlite").InitSchema(context.Background()); err != nil {
		t.Fatalf("project InitSchema failed: %v", err)
	}
	if err := sqlrepo.NewApplicationRepository(db, "sqlite").InitSchema(context.Background()); err != nil {
		t.Fatalf("application InitSchema failed: %v", err)
	}
	if err := sqlrepo.NewArtifactRepositoryConfigRepository(db, "sqlite").InitSchema(context.Background()); err != nil {
		t.Fatalf("artifact repository InitSchema failed: %v", err)
	}

	manager := NewReleaseOrderManager(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	manager.runAsync = func(task func()) { task() }
	return manager, repo
}

// testReleaseOrder 封装当前模块的业务处理逻辑。
func testReleaseOrder(id, orderNo string, status domain.OrderStatus, now time.Time) domain.ReleaseOrder {
	return domain.ReleaseOrder{
		ID:                  id,
		OrderNo:             orderNo,
		OperationType:       domain.OperationTypeDeploy,
		ApplicationID:       "app-1",
		ApplicationName:     "App 1",
		TemplateID:          "rt-1",
		TemplateName:        "Template 1",
		BindingID:           "binding-1",
		EnvCode:             "prod",
		TriggerType:         domain.TriggerTypeManual,
		Status:              status,
		ApprovalApproverIDs: []string{},
		CreatorUserID:       "tester",
		TriggeredBy:         "tester",
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

// testReleaseExecution 封装当前模块的业务处理逻辑。
func testReleaseExecution(orderID, executionID string, scope domain.PipelineScope, status domain.ExecutionStatus, now time.Time) domain.ReleaseOrderExecution {
	return domain.ReleaseOrderExecution{
		ID:             executionID,
		ReleaseOrderID: orderID,
		PipelineScope:  scope,
		BindingID:      "binding-" + string(scope),
		BindingName:    "Binding " + string(scope),
		Provider:       "jenkins",
		PipelineID:     "pipeline-" + string(scope),
		Status:         status,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// testReleaseStep 封装当前模块的业务处理逻辑。
func testReleaseStep(orderID, stepID string, scope domain.StepScope, code string, status domain.StepStatus, sortNo int, now time.Time) domain.ReleaseOrderStep {
	return domain.ReleaseOrderStep{
		ID:             stepID,
		ReleaseOrderID: orderID,
		StepScope:      scope,
		StepCode:       code,
		StepName:       code,
		Status:         status,
		SortNo:         sortNo,
		CreatedAt:      now,
	}
}
