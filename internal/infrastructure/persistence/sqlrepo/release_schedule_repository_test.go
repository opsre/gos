package sqlrepo

import (
	"context"
	"testing"
	"time"

	domain "gos/internal/domain/release"
)

func TestReleaseScheduleRepositoryCreatesAndFetchesActiveSchedule(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	order := newTestReleaseOrder("ro-schedule", "RO-SCHEDULE", "app-1", "prod", domain.OrderStatusApproved, now)
	order.TemplateID = "rt-1"
	order.TemplateName = "Template 1"
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}

	deployAt := now.Add(time.Hour)
	schedule := domain.ReleaseOrderSchedule{
		ID:                    "rosch-1",
		ScheduleNo:            "RS-20260502180000",
		ReleaseOrderID:        order.ID,
		ReleaseOrderNo:        order.OrderNo,
		ApplicationID:         order.ApplicationID,
		ApplicationName:       order.ApplicationName,
		EnvCode:               order.EnvCode,
		TemplateID:            order.TemplateID,
		TemplateName:          order.TemplateName,
		ScheduleMode:          domain.ScheduleModeDeploy,
		DeployScheduledAt:     &deployAt,
		CDConflictAt:          &deployAt,
		Timezone:              "Asia/Shanghai",
		Status:                domain.ScheduleStatusScheduled,
		ApprovalRequired:      true,
		ApprovalMode:          domain.TemplateApprovalModeAny,
		ApprovalApproverIDs:   []string{"approver-1"},
		ApprovalApproverNames: []string{"Approver 1"},
		ApprovedAt:            &now,
		ApprovedBy:            "Approver 1",
		Remark:                "scheduled prod deploy",
		CreatorUserID:         "creator-1",
		CreatorName:           "Creator 1",
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	if err := repo.CreateSchedule(ctx, schedule); err != nil {
		t.Fatalf("CreateSchedule failed: %v", err)
	}

	active, err := repo.GetActiveScheduleByOrderID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetActiveScheduleByOrderID failed: %v", err)
	}
	if active.ID != schedule.ID {
		t.Fatalf("active schedule id = %s, want %s", active.ID, schedule.ID)
	}
	if active.ScheduleMode != domain.ScheduleModeDeploy {
		t.Fatalf("active schedule mode = %s, want %s", active.ScheduleMode, domain.ScheduleModeDeploy)
	}
	if active.DeployScheduledAt == nil || !active.DeployScheduledAt.Equal(deployAt) {
		t.Fatalf("deploy_scheduled_at = %v, want %v", active.DeployScheduledAt, deployAt)
	}
	if len(active.ApprovalApproverIDs) != 1 || active.ApprovalApproverIDs[0] != "approver-1" {
		t.Fatalf("approver ids = %#v, want approver-1", active.ApprovalApproverIDs)
	}
}

func TestReleaseScheduleRepositoryFindsActiveCDConflict(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	cdAt := now.Add(2 * time.Hour)
	order := newTestReleaseOrder("ro-cd-conflict", "RO-CD-CONFLICT", "app-1", "prod", domain.OrderStatusApproved, now)
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}
	if err := repo.CreateSchedule(ctx, domain.ReleaseOrderSchedule{
		ID:                 "rosch-conflict",
		ScheduleNo:         "RS-CONFLICT",
		ReleaseOrderID:     order.ID,
		ReleaseOrderNo:     order.OrderNo,
		ApplicationID:      order.ApplicationID,
		ApplicationName:    order.ApplicationName,
		EnvCode:            order.EnvCode,
		ScheduleMode:       domain.ScheduleModeExecute,
		ExecuteScheduledAt: &cdAt,
		CDConflictAt:       &cdAt,
		Timezone:           "Asia/Shanghai",
		Status:             domain.ScheduleStatusPendingApproval,
		CreatorUserID:      "creator-1",
		CreatorName:        "Creator 1",
		CreatedAt:          now,
		UpdatedAt:          now,
	}); err != nil {
		t.Fatalf("CreateSchedule failed: %v", err)
	}

	conflict, err := repo.FindActiveScheduleCDConflict(ctx, "app-1", "prod", cdAt, "")
	if err != nil {
		t.Fatalf("FindActiveScheduleCDConflict failed: %v", err)
	}
	if conflict.ID != "rosch-conflict" {
		t.Fatalf("conflict id = %s, want rosch-conflict", conflict.ID)
	}

	_, err = repo.FindActiveScheduleCDConflict(ctx, "app-1", "prod", cdAt, "rosch-conflict")
	if err != domain.ErrScheduleNotFound {
		t.Fatalf("FindActiveScheduleCDConflict excluding self error = %v, want ErrScheduleNotFound", err)
	}
}
