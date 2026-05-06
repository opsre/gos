package usecase

import (
	"testing"

	domain "gos/internal/domain/release"
)

// TestShouldAutoApproveOnCreate 创建业务资源并返回处理结果。
func TestShouldAutoApproveOnCreate(t *testing.T) {
	t.Run("disabled approval does not auto approve", func(t *testing.T) {
		if shouldAutoApproveOnCreate(false, []string{"u-1"}, "u-1") {
			t.Fatalf("expected false when approval is disabled")
		}
	})

	t.Run("single self approver auto approves", func(t *testing.T) {
		if !shouldAutoApproveOnCreate(true, []string{"u-1"}, "u-1") {
			t.Fatalf("expected true for self approver")
		}
	})

	t.Run("all approvers are self auto approves", func(t *testing.T) {
		if !shouldAutoApproveOnCreate(true, []string{"u-1", "u-1"}, "u-1") {
			t.Fatalf("expected true when all approvers are self")
		}
	})

	t.Run("mixed approvers do not auto approve", func(t *testing.T) {
		if shouldAutoApproveOnCreate(true, []string{"u-1", "u-2"}, "u-1") {
			t.Fatalf("expected false when another approver exists")
		}
	})

	t.Run("empty creator does not auto approve", func(t *testing.T) {
		if shouldAutoApproveOnCreate(true, []string{"u-1"}, "") {
			t.Fatalf("expected false when creator is empty")
		}
	})
}

// TestResolveInitialReleaseOrderStatus 解析上下文数据，得到后续流程需要的结果。
func TestResolveInitialReleaseOrderStatus(t *testing.T) {
	template := domain.ReleaseTemplate{
		ApprovalEnabled:     true,
		ApprovalApproverIDs: []string{"u-1"},
	}

	if got := resolveInitialReleaseOrderStatus(template, "u-1"); got != domain.OrderStatusApproved {
		t.Fatalf("expected approved, got %s", got)
	}

	if got := resolveInitialReleaseOrderStatus(template, "u-2"); got != domain.OrderStatusPendingApproval {
		t.Fatalf("expected pending_approval, got %s", got)
	}

	if got := resolveInitialReleaseOrderStatus(domain.ReleaseTemplate{}, "u-1"); got != domain.OrderStatusPending {
		t.Fatalf("expected pending, got %s", got)
	}
}
