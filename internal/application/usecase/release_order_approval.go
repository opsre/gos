package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/release"
	"gos/internal/support/logx"
)

type ListApprovalRecordSummaryInput struct {
	ApplicationID               string
	ApplicationIDs              []string
	VisibleApplicationEnvScopes []domain.ApplicationEnvScope
	VisibleToUserID             string
	OperatorUserID              string
	Page                        int
	PageSize                    int
}

// ListApprovalRecordSummaries 查询并返回列表数据。
func (uc *ReleaseOrderManager) ListApprovalRecordSummaries(
	ctx context.Context,
	input ListApprovalRecordSummaryInput,
) ([]domain.ReleaseOrderApprovalRecordSummary, int64, error) {
	if input.Page <= 0 {
		input.Page = 1
	}
	if input.PageSize <= 0 {
		input.PageSize = 20
	}
	if input.PageSize > 100 {
		input.PageSize = 100
	}
	return uc.repo.ListApprovalRecordSummaries(ctx, domain.ApprovalRecordListFilter{
		ApplicationID:               strings.TrimSpace(input.ApplicationID),
		ApplicationIDs:              normalizeReleaseApplicationIDs(input.ApplicationIDs),
		VisibleApplicationEnvScopes: normalizeReleaseApplicationEnvScopes(input.VisibleApplicationEnvScopes),
		VisibleToUserID:             strings.TrimSpace(input.VisibleToUserID),
		OperatorUserID:              strings.TrimSpace(input.OperatorUserID),
		Page:                        input.Page,
		PageSize:                    input.PageSize,
	})
}

// ListApprovalRecords 查询并返回列表数据。
func (uc *ReleaseOrderManager) ListApprovalRecords(ctx context.Context, orderID string) ([]domain.ReleaseOrderApprovalRecord, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, ErrInvalidID
	}
	if _, err := uc.repo.GetByID(ctx, orderID); err != nil {
		return nil, err
	}
	return uc.repo.ListApprovalRecords(ctx, orderID)
}

// SubmitApproval 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) SubmitApproval(
	ctx context.Context,
	orderID string,
	operatorUserID string,
	operatorName string,
	comment string,
) (domain.ReleaseOrder, error) {
	orderID = strings.TrimSpace(orderID)
	operatorUserID = strings.TrimSpace(operatorUserID)
	operatorName = strings.TrimSpace(operatorName)
	comment = strings.TrimSpace(comment)
	if orderID == "" {
		return domain.ReleaseOrder{}, ErrInvalidID
	}
	if operatorUserID == "" {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: operator_user_id is required", ErrInvalidInput)
	}

	order, err := uc.repo.GetByID(ctx, orderID)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	if !order.ApprovalRequired {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: current release order does not require approval", ErrInvalidInput)
	}
	if order.Status != domain.OrderStatusPendingApproval {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: release order cannot submit approval in current status", ErrInvalidInput)
	}

	now := uc.now()
	if _, err = uc.repo.UpdateApprovalStatus(
		ctx,
		order.ID,
		domain.OrderStatusApproving,
		nil,
		"",
		nil,
		"",
		"",
		now,
	); err != nil {
		return domain.ReleaseOrder{}, err
	}
	if err = uc.repo.CreateApprovalRecord(ctx, domain.ReleaseOrderApprovalRecord{
		ID:             generateID("roar"),
		ReleaseOrderID: order.ID,
		Action:         domain.ReleaseOrderApprovalActionSubmit,
		OperatorUserID: operatorUserID,
		OperatorName:   firstNonEmpty(operatorName, operatorUserID),
		Comment:        comment,
		CreatedAt:      now,
	}); err != nil {
		return domain.ReleaseOrder{}, err
	}
	logx.Info("release_order", "approval_submitted",
		logx.F("order_id", order.ID),
		logx.F("order_no", order.OrderNo),
		logx.F("operator_user_id", operatorUserID),
	)
	return uc.repo.GetByID(ctx, order.ID)
}

// Approve 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) Approve(
	ctx context.Context,
	orderID string,
	operatorUserID string,
	operatorName string,
	comment string,
) (domain.ReleaseOrder, error) {
	orderID = strings.TrimSpace(orderID)
	operatorUserID = strings.TrimSpace(operatorUserID)
	operatorName = strings.TrimSpace(operatorName)
	comment = strings.TrimSpace(comment)
	if orderID == "" {
		return domain.ReleaseOrder{}, ErrInvalidID
	}
	if operatorUserID == "" {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: operator_user_id is required", ErrInvalidInput)
	}

	order, err := uc.repo.GetByID(ctx, orderID)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	if !order.ApprovalRequired {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: current release order does not require approval", ErrInvalidInput)
	}
	if order.Status != domain.OrderStatusPendingApproval && order.Status != domain.OrderStatusApproving {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: release order cannot be approved in current status", ErrInvalidInput)
	}
	if !approvalIncludesUser(order.ApprovalApproverIDs, operatorUserID) {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: current user is not in approval approver list", ErrInvalidInput)
	}

	records, err := uc.repo.ListApprovalRecords(ctx, order.ID)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	if approvalAlreadyActed(records, operatorUserID, domain.ReleaseOrderApprovalActionApprove) {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: current approver has already approved", ErrInvalidInput)
	}
	if approvalAlreadyActed(records, operatorUserID, domain.ReleaseOrderApprovalActionReject) {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: current approver has already rejected", ErrInvalidInput)
	}

	now := uc.now()
	if err = uc.repo.CreateApprovalRecord(ctx, domain.ReleaseOrderApprovalRecord{
		ID:             generateID("roar"),
		ReleaseOrderID: order.ID,
		Action:         domain.ReleaseOrderApprovalActionApprove,
		OperatorUserID: operatorUserID,
		OperatorName:   firstNonEmpty(operatorName, operatorUserID),
		Comment:        comment,
		CreatedAt:      now,
	}); err != nil {
		return domain.ReleaseOrder{}, err
	}

	nextStatus := domain.OrderStatusApproving
	var approvedAt *time.Time
	approvedBy := ""
	if order.ApprovalMode != domain.TemplateApprovalModeAll || approvalAllApproversApproved(records, order.ApprovalApproverIDs, operatorUserID) {
		nextStatus = domain.OrderStatusApproved
		approvedAt = &now
		approvedBy = firstNonEmpty(operatorName, operatorUserID)
	}
	if _, err = uc.repo.UpdateApprovalStatus(
		ctx,
		order.ID,
		nextStatus,
		approvedAt,
		approvedBy,
		nil,
		"",
		"",
		now,
	); err != nil {
		return domain.ReleaseOrder{}, err
	}
	logx.Info("release_order", "approval_approved",
		logx.F("order_id", order.ID),
		logx.F("order_no", order.OrderNo),
		logx.F("operator_user_id", operatorUserID),
		logx.F("next_status", nextStatus),
	)
	return uc.repo.GetByID(ctx, order.ID)
}

// Reject 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) Reject(
	ctx context.Context,
	orderID string,
	operatorUserID string,
	operatorName string,
	comment string,
) (domain.ReleaseOrder, error) {
	orderID = strings.TrimSpace(orderID)
	operatorUserID = strings.TrimSpace(operatorUserID)
	operatorName = strings.TrimSpace(operatorName)
	comment = strings.TrimSpace(comment)
	if orderID == "" {
		return domain.ReleaseOrder{}, ErrInvalidID
	}
	if operatorUserID == "" {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: operator_user_id is required", ErrInvalidInput)
	}
	if comment == "" {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: reject reason is required", ErrInvalidInput)
	}

	order, err := uc.repo.GetByID(ctx, orderID)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	if !order.ApprovalRequired {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: current release order does not require approval", ErrInvalidInput)
	}
	if order.Status != domain.OrderStatusPendingApproval && order.Status != domain.OrderStatusApproving {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: release order cannot be rejected in current status", ErrInvalidInput)
	}
	if !approvalIncludesUser(order.ApprovalApproverIDs, operatorUserID) {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: current user is not in approval approver list", ErrInvalidInput)
	}

	records, err := uc.repo.ListApprovalRecords(ctx, order.ID)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	if approvalAlreadyActed(records, operatorUserID, domain.ReleaseOrderApprovalActionApprove) {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: current approver has already approved", ErrInvalidInput)
	}
	if approvalAlreadyActed(records, operatorUserID, domain.ReleaseOrderApprovalActionReject) {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: current approver has already rejected", ErrInvalidInput)
	}

	now := uc.now()
	if err = uc.repo.CreateApprovalRecord(ctx, domain.ReleaseOrderApprovalRecord{
		ID:             generateID("roar"),
		ReleaseOrderID: order.ID,
		Action:         domain.ReleaseOrderApprovalActionReject,
		OperatorUserID: operatorUserID,
		OperatorName:   firstNonEmpty(operatorName, operatorUserID),
		Comment:        comment,
		CreatedAt:      now,
	}); err != nil {
		return domain.ReleaseOrder{}, err
	}
	if _, err = uc.repo.UpdateApprovalStatus(
		ctx,
		order.ID,
		domain.OrderStatusRejected,
		nil,
		"",
		&now,
		firstNonEmpty(operatorName, operatorUserID),
		comment,
		now,
	); err != nil {
		return domain.ReleaseOrder{}, err
	}
	logx.Info("release_order", "approval_rejected",
		logx.F("order_id", order.ID),
		logx.F("order_no", order.OrderNo),
		logx.F("operator_user_id", operatorUserID),
	)
	return uc.repo.GetByID(ctx, order.ID)
}

// approvalAlreadyActed 封装当前模块的业务处理逻辑。
func approvalAlreadyActed(records []domain.ReleaseOrderApprovalRecord, operatorUserID string, action domain.ReleaseOrderApprovalAction) bool {
	operatorUserID = strings.TrimSpace(operatorUserID)
	for _, item := range records {
		if strings.TrimSpace(item.OperatorUserID) != operatorUserID {
			continue
		}
		if item.Action == action {
			return true
		}
	}
	return false
}

// approvalIncludesUser 封装当前模块的业务处理逻辑。
func approvalIncludesUser(approverIDs []string, operatorUserID string) bool {
	operatorUserID = strings.TrimSpace(operatorUserID)
	if operatorUserID == "" {
		return false
	}
	for _, item := range approverIDs {
		if strings.TrimSpace(item) == operatorUserID {
			return true
		}
	}
	return false
}

// approvalAllApproversApproved 封装当前模块的业务处理逻辑。
func approvalAllApproversApproved(
	records []domain.ReleaseOrderApprovalRecord,
	approverIDs []string,
	currentOperatorUserID string,
) bool {
	approved := make(map[string]struct{}, len(approverIDs)+1)
	for _, item := range records {
		if item.Action != domain.ReleaseOrderApprovalActionApprove {
			continue
		}
		userID := strings.TrimSpace(item.OperatorUserID)
		if userID == "" {
			continue
		}
		approved[userID] = struct{}{}
	}
	if userID := strings.TrimSpace(currentOperatorUserID); userID != "" {
		approved[userID] = struct{}{}
	}
	for _, item := range approverIDs {
		userID := strings.TrimSpace(item)
		if userID == "" {
			continue
		}
		if _, ok := approved[userID]; !ok {
			return false
		}
	}
	return true
}
