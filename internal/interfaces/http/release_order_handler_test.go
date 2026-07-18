package httpapi

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gos/internal/application/usecase"
	domain "gos/internal/domain/release"
)

func TestNormalizeReleaseOrderErrorMessageStripsInvalidInputPrefix(t *testing.T) {
	err := fmt.Errorf("%w: 重放单不支持再次重放，继续重发请从原始单发起", usecase.ErrInvalidInput)

	got := normalizeReleaseOrderErrorMessage(err)
	want := "重放单不支持再次重放，继续重发请从原始单发起"

	if got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestReleaseOrderApprovalFlowResponseIncludesFrozenGraphSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	instance := domain.ReleaseOrderApprovalFlowInstance{
		ID:               "flow-instance-1",
		ReleaseOrderID:   "order-1",
		FlowDefinitionID: "flow-1",
		FlowName:         "生产发布审批",
		Nodes: []domain.ApprovalFlowNode{{
			Code: "security-review", Name: "安全审批", Gate: domain.ApprovalFlowGateBeforeCD,
			NodeType: domain.ApprovalFlowNodeTypeApproval, ApprovalMode: domain.TemplateApprovalModeAll,
			ApproverIDs: []string{"user-1"}, ApproverNames: []string{"安全负责人"}, PositionX: 320, PositionY: 120,
		}},
		Links: []domain.ApprovalFlowLink{{
			FromCode: "start", ToCode: "security-review",
			ExecutionScopes: []string{string(domain.ApprovalFlowExecutionScopeFullRelease)}, Priority: 1,
		}},
		Status: domain.ApprovalFlowInstanceStatusPendingApproval, CreatedAt: now, UpdatedAt: now,
	}

	tasks := []domain.ReleaseOrderApprovalFlowTask{{
		ID: "approval-task-1", NodeCode: "security-review", NodeName: "安全审批",
		Records: []domain.ReleaseOrderApprovalFlowTaskRecord{{
			ID: "approval-record-1", TaskID: "approval-task-1", Action: domain.ReleaseOrderApprovalActionApprove,
			OperatorUserID: "user-1", OperatorName: "安全负责人", Comment: "审批备注", CreatedAt: now,
		}},
	}}
	response := toReleaseOrderApprovalFlowResponse(instance, tasks)
	if len(response.Nodes) != 1 || response.Nodes[0].Code != "security-review" || response.Nodes[0].ApproverNames[0] != "安全负责人" {
		t.Fatalf("approval flow response nodes = %#v", response.Nodes)
	}
	if len(response.Links) != 1 || response.Links[0].FromCode != "start" || response.Links[0].ToCode != "security-review" {
		t.Fatalf("approval flow response links = %#v", response.Links)
	}
	if len(response.Tasks) != 1 || len(response.Tasks[0].Records) != 1 || response.Tasks[0].Records[0].Comment != "审批备注" {
		t.Fatalf("approval flow response task records = %#v", response.Tasks)
	}
}

func TestReleaseOrderHTTPContractDoesNotExposeDeprecatedSonService(t *testing.T) {
	for _, file := range []string{
		"release_order_handler.go",
		"../../../docs/docs.go",
	} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("ReadFile %s failed: %v", file, err)
		}

		text := string(source)
		for _, forbidden := range []string{"SonService", "son_service", `json:"son_service"`} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s should not expose deprecated %q", file, forbidden)
			}
		}
	}
}
