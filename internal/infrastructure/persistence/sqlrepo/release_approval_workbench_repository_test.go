package sqlrepo

import (
	"context"
	"testing"
	"time"

	domain "gos/internal/domain/release"
)

func TestApprovalWorkbenchAggregatesFlowAndLegacyTasks(t *testing.T) {
	t.Parallel()

	repo := newTestReleaseRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()

	flowOrder := newTestReleaseOrder("ro-flow-workbench", "RO-FLOW-WORKBENCH", "app-flow", "prod", domain.OrderStatusPending, now)
	legacyOrder := newTestReleaseOrder("ro-legacy-workbench", "RO-LEGACY-WORKBENCH", "app-legacy", "test", domain.OrderStatusApproving, now.Add(time.Minute))
	legacyOrder.ApprovalRequired = true
	legacyOrder.ApprovalMode = domain.TemplateApprovalModeAny
	legacyOrder.ApprovalApproverIDs = []string{"user-approver"}
	legacyOrder.ApprovalApproverNames = []string{"Approver"}

	for _, order := range []domain.ReleaseOrder{flowOrder, legacyOrder} {
		if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
			t.Fatalf("Create(%s) failed: %v", order.ID, err)
		}
	}

	instance := domain.ReleaseOrderApprovalFlowInstance{
		ID:               "flow-instance-workbench",
		ReleaseOrderID:   flowOrder.ID,
		FlowDefinitionID: "flow-definition-workbench",
		FlowName:         "生产发布审批",
		Status:           domain.ApprovalFlowInstanceStatusPendingApproval,
		CurrentGate:      domain.ApprovalFlowGateBeforeCI,
		CurrentScope:     domain.ApprovalFlowExecutionScopeBuildOnly,
		CurrentNodeCode:  "ci-approval",
		CurrentTaskID:    "flow-task-workbench",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := repo.CreateApprovalFlowInstance(ctx, instance); err != nil {
		t.Fatalf("CreateApprovalFlowInstance failed: %v", err)
	}
	task := domain.ReleaseOrderApprovalFlowTask{
		ID:             instance.CurrentTaskID,
		InstanceID:     instance.ID,
		ReleaseOrderID: flowOrder.ID,
		NodeCode:       instance.CurrentNodeCode,
		NodeName:       "CI 审批",
		Gate:           domain.ApprovalFlowGateBeforeCI,
		ApprovalMode:   domain.TemplateApprovalModeAll,
		ApproverIDs:    []string{"user-approver", "user-owner"},
		ApproverNames:  []string{"Approver", "Owner"},
		Status:         domain.ApprovalFlowTaskStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.CreateApprovalFlowTask(ctx, task); err != nil {
		t.Fatalf("CreateApprovalFlowTask failed: %v", err)
	}

	items, total, err := repo.ListApprovalWorkbenchTasks(ctx, domain.ApprovalWorkbenchListFilter{UserID: "user-approver", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListApprovalWorkbenchTasks failed: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("pending tasks total=%d len=%d, want 2", total, len(items))
	}
	sources := map[domain.ApprovalWorkbenchSource]bool{}
	for _, item := range items {
		sources[item.Source] = true
	}
	if !sources[domain.ApprovalWorkbenchSourceFlow] || !sources[domain.ApprovalWorkbenchSourceLegacy] {
		t.Fatalf("pending task sources = %#v, want flow and legacy", sources)
	}

	flowRecord := domain.ReleaseOrderApprovalFlowTaskRecord{
		ID:             "flow-task-record-workbench",
		TaskID:         task.ID,
		Action:         domain.ReleaseOrderApprovalActionApprove,
		OperatorUserID: "user-approver",
		OperatorName:   "Approver",
		Comment:        "同意构建",
		CreatedAt:      now.Add(2 * time.Minute),
	}
	if err := repo.CreateApprovalFlowTaskRecord(ctx, flowRecord); err != nil {
		t.Fatalf("CreateApprovalFlowTaskRecord failed: %v", err)
	}
	if err := repo.CreateApprovalRecord(ctx, domain.ReleaseOrderApprovalRecord{
		ID:             "legacy-record-workbench",
		ReleaseOrderID: legacyOrder.ID,
		Action:         domain.ReleaseOrderApprovalActionApprove,
		OperatorUserID: "user-approver",
		OperatorName:   "Approver",
		Comment:        "同意发布",
		CreatedAt:      now.Add(3 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateApprovalRecord failed: %v", err)
	}

	items, total, err = repo.ListApprovalWorkbenchTasks(ctx, domain.ApprovalWorkbenchListFilter{UserID: "user-approver", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListApprovalWorkbenchTasks after action failed: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("acted tasks total=%d len=%d, want 0", total, len(items))
	}

	records, recordTotal, err := repo.ListApprovalWorkbenchRecords(ctx, domain.ApprovalWorkbenchListFilter{UserID: "user-approver", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListApprovalWorkbenchRecords failed: %v", err)
	}
	if recordTotal != 2 || len(records) != 2 {
		t.Fatalf("handled records total=%d len=%d, want 2", recordTotal, len(records))
	}
	recordSources := map[domain.ApprovalWorkbenchSource]bool{}
	for _, record := range records {
		recordSources[record.Source] = true
	}
	if !recordSources[domain.ApprovalWorkbenchSourceFlow] || !recordSources[domain.ApprovalWorkbenchSourceLegacy] {
		t.Fatalf("handled record sources = %#v, want flow and legacy", recordSources)
	}
}
