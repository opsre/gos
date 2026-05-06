package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "gos/internal/domain/release"
)

func TestReleaseOrderScheduleCreateSnapshotsTemplateApproval(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	createScheduleTemplate(t, repo, ctx, now, true, []string{"approver-1", "approver-2"})
	order := testReleaseOrder("ro-scheduled", "RO-SCHEDULED", domain.OrderStatusPending, now)
	order.TemplateID = "rt-schedule"
	order.TemplateName = "Schedule Template"
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}

	executeAt := now.Add(time.Hour)
	schedule, err := manager.CreateSchedule(ctx, order.ID, CreateReleaseOrderScheduleInput{
		ScheduleMode:       domain.ScheduleModeExecute,
		ExecuteScheduledAt: &executeAt,
		Timezone:           "Asia/Shanghai",
		Remark:             "prod window",
		CreatorUserID:      "creator-1",
		CreatorName:        "Creator 1",
	})
	if err != nil {
		t.Fatalf("CreateSchedule failed: %v", err)
	}

	if schedule.Status != domain.ScheduleStatusApproving {
		t.Fatalf("schedule status = %s, want %s", schedule.Status, domain.ScheduleStatusApproving)
	}
	if !schedule.ApprovalRequired {
		t.Fatal("schedule approval_required = false, want true")
	}
	if schedule.ApprovalMode != domain.TemplateApprovalModeAny {
		t.Fatalf("approval mode = %s, want %s", schedule.ApprovalMode, domain.TemplateApprovalModeAny)
	}
	if len(schedule.ApprovalApproverIDs) != 2 || schedule.ApprovalApproverIDs[0] != "approver-1" || schedule.ApprovalApproverIDs[1] != "approver-2" {
		t.Fatalf("approval approvers = %#v, want template approvers", schedule.ApprovalApproverIDs)
	}
	if schedule.CDConflictAt == nil || !schedule.CDConflictAt.Equal(executeAt) {
		t.Fatalf("cd_conflict_at = %v, want execute time %v", schedule.CDConflictAt, executeAt)
	}
	records, err := manager.ListScheduleApprovalRecords(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("ListScheduleApprovalRecords failed: %v", err)
	}
	if len(records) != 1 || records[0].Action != domain.ReleaseOrderApprovalActionSubmit || records[0].OperatorUserID != "creator-1" {
		t.Fatalf("approval records = %#v, want one auto submit record by creator", records)
	}
}

func TestReleaseOrderScheduleRejectsInvalidBuildDeployOrder(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	createScheduleTemplate(t, repo, ctx, now, false, nil)
	order := testReleaseOrder("ro-invalid-order", "RO-INVALID-ORDER", domain.OrderStatusPending, now)
	order.TemplateID = "rt-schedule"
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}

	buildAt := now.Add(time.Hour)
	deployAt := buildAt.Add(-time.Minute)
	_, err := manager.CreateSchedule(ctx, order.ID, CreateReleaseOrderScheduleInput{
		ScheduleMode:      domain.ScheduleModeBuildDeploy,
		BuildScheduledAt:  &buildAt,
		DeployScheduledAt: &deployAt,
		Timezone:          "Asia/Shanghai",
		CreatorUserID:     "creator-1",
		CreatorName:       "Creator 1",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("CreateSchedule error = %v, want ErrInvalidInput", err)
	}
}

func TestReleaseOrderScheduleRejectsActiveOrderScheduleAndCDConflict(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	createScheduleTemplate(t, repo, ctx, now, false, nil)
	executeAt := now.Add(time.Hour)

	orderA := testReleaseOrder("ro-conflict-a", "RO-CONFLICT-A", domain.OrderStatusPending, now)
	orderA.TemplateID = "rt-schedule"
	orderB := testReleaseOrder("ro-conflict-b", "RO-CONFLICT-B", domain.OrderStatusPending, now.Add(time.Second))
	orderB.TemplateID = "rt-schedule"
	for _, order := range []domain.ReleaseOrder{orderA, orderB} {
		if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{
			testReleaseExecution(order.ID, "exec-"+order.ID, domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
		}, nil, nil); err != nil {
			t.Fatalf("Create order %s failed: %v", order.ID, err)
		}
	}

	if _, err := manager.CreateSchedule(ctx, orderA.ID, CreateReleaseOrderScheduleInput{
		ScheduleMode:       domain.ScheduleModeExecute,
		ExecuteScheduledAt: &executeAt,
		Timezone:           "Asia/Shanghai",
		CreatorUserID:      "creator-1",
		CreatorName:        "Creator 1",
	}); err != nil {
		t.Fatalf("CreateSchedule orderA failed: %v", err)
	}

	_, err := manager.CreateSchedule(ctx, orderA.ID, CreateReleaseOrderScheduleInput{
		ScheduleMode:       domain.ScheduleModeExecute,
		ExecuteScheduledAt: ptrTime(executeAt.Add(time.Hour)),
		Timezone:           "Asia/Shanghai",
		CreatorUserID:      "creator-1",
		CreatorName:        "Creator 1",
	})
	if !errors.Is(err, ErrReferencedConflict) {
		t.Fatalf("second schedule for same order error = %v, want ErrReferencedConflict", err)
	}

	_, err = manager.CreateSchedule(ctx, orderB.ID, CreateReleaseOrderScheduleInput{
		ScheduleMode:       domain.ScheduleModeExecute,
		ExecuteScheduledAt: &executeAt,
		Timezone:           "Asia/Shanghai",
		CreatorUserID:      "creator-1",
		CreatorName:        "Creator 1",
	})
	if !errors.Is(err, ErrConcurrentReleaseBlocked) {
		t.Fatalf("same app/env/cd time error = %v, want ErrConcurrentReleaseBlocked", err)
	}
}

func TestReleaseOrderScheduleApproveLocksScheduleUpdate(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	createScheduleTemplate(t, repo, ctx, now, true, []string{"approver-1"})
	order := testReleaseOrder("ro-approve-lock", "RO-APPROVE-LOCK", domain.OrderStatusPending, now)
	order.TemplateID = "rt-schedule"
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}

	deployAt := now.Add(time.Hour)
	schedule, err := manager.CreateSchedule(ctx, order.ID, CreateReleaseOrderScheduleInput{
		ScheduleMode:      domain.ScheduleModeDeploy,
		DeployScheduledAt: &deployAt,
		Timezone:          "Asia/Shanghai",
		CreatorUserID:     "creator-1",
		CreatorName:       "Creator 1",
	})
	if err != nil {
		t.Fatalf("CreateSchedule failed: %v", err)
	}
	if schedule.Status != domain.ScheduleStatusApproving {
		t.Fatalf("schedule status = %s, want approving", schedule.Status)
	}

	approved, err := manager.ApproveSchedule(ctx, schedule.ID, "approver-1", "Approver 1", "approved")
	if err != nil {
		t.Fatalf("ApproveSchedule failed: %v", err)
	}
	if approved.Status != domain.ScheduleStatusScheduled {
		t.Fatalf("approved status = %s, want scheduled", approved.Status)
	}
	if approved.ApprovedAt == nil || approved.ApprovedBy != "Approver 1" {
		t.Fatalf("approved metadata = (%v, %q), want timestamp and Approver 1", approved.ApprovedAt, approved.ApprovedBy)
	}

	laterDeployAt := deployAt.Add(time.Hour)
	_, err = manager.UpdateSchedule(ctx, schedule.ID, UpdateReleaseOrderScheduleInput{
		ScheduleMode:      domain.ScheduleModeDeploy,
		DeployScheduledAt: &laterDeployAt,
		Timezone:          "Asia/Shanghai",
		CreatorUserID:     "creator-1",
		CreatorName:       "Creator 1",
	})
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("UpdateSchedule after approval error = %v, want ErrInvalidStatus", err)
	}
}

func TestReleaseOrderScheduleDispatchExpiresUnapprovedDueSchedule(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	dueAt := now.Add(-time.Minute)
	if err := repo.CreateSchedule(ctx, domain.ReleaseOrderSchedule{
		ID:                 "rosch-expire",
		ScheduleNo:         "RS-EXPIRE",
		ReleaseOrderID:     "ro-expire",
		ReleaseOrderNo:     "RO-EXPIRE",
		ApplicationID:      "app-1",
		ApplicationName:    "App 1",
		EnvCode:            "prod",
		TemplateID:         "rt-1",
		TemplateName:       "Template 1",
		ScheduleMode:       domain.ScheduleModeExecute,
		ExecuteScheduledAt: &dueAt,
		CDConflictAt:       &dueAt,
		Timezone:           "Asia/Shanghai",
		Status:             domain.ScheduleStatusPendingApproval,
		CreatorUserID:      "creator-1",
		CreatorName:        "Creator 1",
		CreatedAt:          now.Add(-time.Hour),
		UpdatedAt:          now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("CreateSchedule failed: %v", err)
	}

	result, err := manager.RunDueSchedules(ctx, 10)
	if err != nil {
		t.Fatalf("RunDueSchedules failed: %v", err)
	}
	if result.Expired != 1 {
		t.Fatalf("expired count = %d, want 1", result.Expired)
	}
	stored, err := repo.GetScheduleByID(ctx, "rosch-expire")
	if err != nil {
		t.Fatalf("GetScheduleByID failed: %v", err)
	}
	if stored.Status != domain.ScheduleStatusExpired {
		t.Fatalf("stored status = %s, want expired", stored.Status)
	}
	if stored.ExpiredAt == nil || stored.LastError == "" {
		t.Fatalf("expired metadata missing: expired_at=%v last_error=%q", stored.ExpiredAt, stored.LastError)
	}
}

func TestReleaseOrderScheduleDispatchBuildDueSchedule(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	manager.jenkins = segmentedReleaseNoopJenkinsExecutor{}
	ctx := context.Background()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	order := testReleaseOrder("ro-build-due", "RO-BUILD-DUE", domain.OrderStatusPending, now.Add(-time.Hour))
	order.TemplateID = "rt-1"
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}
	dueAt := now.Add(-time.Minute)
	if err := repo.CreateSchedule(ctx, domain.ReleaseOrderSchedule{
		ID:               "rosch-build-due",
		ScheduleNo:       "RS-BUILD-DUE",
		ReleaseOrderID:   order.ID,
		ReleaseOrderNo:   order.OrderNo,
		ApplicationID:    order.ApplicationID,
		ApplicationName:  order.ApplicationName,
		EnvCode:          order.EnvCode,
		TemplateID:       order.TemplateID,
		TemplateName:     order.TemplateName,
		ScheduleMode:     domain.ScheduleModeBuild,
		BuildScheduledAt: &dueAt,
		Timezone:         "Asia/Shanghai",
		Status:           domain.ScheduleStatusScheduled,
		CreatorUserID:    "creator-1",
		CreatorName:      "Creator 1",
		CreatedAt:        now.Add(-time.Hour),
		UpdatedAt:        now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("CreateSchedule failed: %v", err)
	}

	result, err := manager.RunDueSchedules(ctx, 10)
	if err != nil {
		t.Fatalf("RunDueSchedules failed: %v", err)
	}
	if result.Dispatched != 1 {
		t.Fatalf("dispatched count = %d, want 1", result.Dispatched)
	}
	stored, err := repo.GetScheduleByID(ctx, "rosch-build-due")
	if err != nil {
		t.Fatalf("GetScheduleByID failed: %v", err)
	}
	if stored.Status != domain.ScheduleStatusDispatched || stored.BuildDispatchedAt == nil {
		t.Fatalf("stored schedule = status %s build_dispatched_at %v, want dispatched with timestamp", stored.Status, stored.BuildDispatchedAt)
	}
	storedOrder, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if storedOrder.Status != domain.OrderStatusBuilding {
		t.Fatalf("order status = %s, want building", storedOrder.Status)
	}
}

func TestReleaseOrderScheduleListSchedulableOrdersFiltersActiveTerminalAndMode(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	createScheduleTemplate(t, repo, ctx, now, false, nil)

	activeOrder := testReleaseOrder("ro-schedulable-active", "RO-SCHEDULABLE-ACTIVE", domain.OrderStatusPending, now)
	activeOrder.TemplateID = "rt-schedule"
	noCIOrder := testReleaseOrder("ro-schedulable-no-ci", "RO-SCHEDULABLE-NO-CI", domain.OrderStatusPending, now.Add(time.Second))
	noCIOrder.TemplateID = "rt-schedule"
	okOrder := testReleaseOrder("ro-schedulable-ok", "RO-SCHEDULABLE-OK", domain.OrderStatusPending, now.Add(2*time.Second))
	okOrder.TemplateID = "rt-schedule"
	terminalOrder := testReleaseOrder("ro-schedulable-terminal", "RO-SCHEDULABLE-TERMINAL", domain.OrderStatusDeploySuccess, now.Add(3*time.Second))
	terminalOrder.TemplateID = "rt-schedule"

	if err := repo.Create(ctx, activeOrder, []domain.ReleaseOrderExecution{
		testReleaseExecution(activeOrder.ID, "exec-active-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
	}, nil, nil); err != nil {
		t.Fatalf("Create activeOrder failed: %v", err)
	}
	if err := repo.Create(ctx, noCIOrder, []domain.ReleaseOrderExecution{
		testReleaseExecution(noCIOrder.ID, "exec-no-ci-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now),
	}, nil, nil); err != nil {
		t.Fatalf("Create noCIOrder failed: %v", err)
	}
	if err := repo.Create(ctx, okOrder, []domain.ReleaseOrderExecution{
		testReleaseExecution(okOrder.ID, "exec-ok-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
	}, nil, nil); err != nil {
		t.Fatalf("Create okOrder failed: %v", err)
	}
	if err := repo.Create(ctx, terminalOrder, []domain.ReleaseOrderExecution{
		testReleaseExecution(terminalOrder.ID, "exec-terminal-ci", domain.PipelineScopeCI, domain.ExecutionStatusPending, now),
	}, nil, nil); err != nil {
		t.Fatalf("Create terminalOrder failed: %v", err)
	}
	buildAt := now.Add(time.Hour)
	if err := repo.CreateSchedule(ctx, domain.ReleaseOrderSchedule{
		ID:               "rosch-schedulable-active",
		ScheduleNo:       "RS-SCHEDULABLE-ACTIVE",
		ReleaseOrderID:   activeOrder.ID,
		ReleaseOrderNo:   activeOrder.OrderNo,
		ApplicationID:    activeOrder.ApplicationID,
		ApplicationName:  activeOrder.ApplicationName,
		EnvCode:          activeOrder.EnvCode,
		ScheduleMode:     domain.ScheduleModeBuild,
		BuildScheduledAt: &buildAt,
		Timezone:         "Asia/Shanghai",
		Status:           domain.ScheduleStatusScheduled,
		CreatorUserID:    "creator-1",
		CreatorName:      "Creator 1",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("CreateSchedule failed: %v", err)
	}

	items, total, err := manager.ListSchedulableOrders(ctx, ListSchedulableReleaseOrderInput{
		ListReleaseOrderInput: ListReleaseOrderInput{Page: 1, PageSize: 20},
		ScheduleMode:          domain.ScheduleModeBuild,
	})
	if err != nil {
		t.Fatalf("ListSchedulableOrders failed: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != okOrder.ID {
		t.Fatalf("schedulable items total=%d items=%#v, want only %s", total, items, okOrder.ID)
	}

	items, total, err = manager.ListSchedulableOrders(ctx, ListSchedulableReleaseOrderInput{
		ListReleaseOrderInput: ListReleaseOrderInput{Page: 1, PageSize: 20},
	})
	if err != nil {
		t.Fatalf("ListSchedulableOrders without mode failed: %v", err)
	}
	if total != 2 || len(items) != 2 || items[0].ID != noCIOrder.ID || items[1].ID != okOrder.ID {
		t.Fatalf("schedulable any-mode total=%d items=%#v, want %s then %s", total, items, noCIOrder.ID, okOrder.ID)
	}
}

func createScheduleTemplate(
	t *testing.T,
	repo domain.Repository,
	ctx context.Context,
	now time.Time,
	approvalEnabled bool,
	approverIDs []string,
) {
	t.Helper()
	approverNames := make([]string, 0, len(approverIDs))
	for _, approverID := range approverIDs {
		approverNames = append(approverNames, "Name "+approverID)
	}
	if err := repo.CreateTemplate(ctx, domain.ReleaseTemplate{
		ID:                    "rt-schedule",
		Name:                  "Schedule Template",
		ApplicationID:         "app-1",
		ApplicationName:       "App 1",
		BindingID:             "binding-1",
		BindingName:           "Binding 1",
		Status:                domain.TemplateStatusActive,
		ApprovalEnabled:       approvalEnabled,
		ApprovalMode:          domain.TemplateApprovalModeAny,
		ApprovalApproverIDs:   approverIDs,
		ApprovalApproverNames: approverNames,
		CreatedAt:             now,
		UpdatedAt:             now,
	}, nil, nil, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}
}
