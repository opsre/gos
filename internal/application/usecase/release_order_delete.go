package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domain "gos/internal/domain/release"
	"gos/internal/support/logx"
)

type BatchDeleteReleaseOrdersInput struct {
	OrderIDs []string
}

type BatchDeleteReleaseOrdersFailure struct {
	OrderID string `json:"order_id"`
	OrderNo string `json:"order_no"`
	Reason  string `json:"reason"`
}

type BatchDeleteReleaseOrdersOutput struct {
	DeletedOrderIDs []string                          `json:"deleted_order_ids"`
	Failed          []BatchDeleteReleaseOrdersFailure `json:"failed"`
}

type releaseOrderDeleteRepository interface {
	DeleteOrders(ctx context.Context, orderIDs []string) error
}

// Delete 删除业务资源并返回处理结果。
func (uc *ReleaseOrderManager) Delete(ctx context.Context, id string) error {
	orderID := strings.TrimSpace(id)
	if orderID == "" {
		return ErrInvalidID
	}
	order, err := uc.repo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	if err := uc.ensureReleaseOrderDeletable(ctx, order); err != nil {
		return err
	}
	return uc.deleteReleaseOrders(ctx, []string{order.ID})
}

// BatchDelete 删除业务资源并返回处理结果。
func (uc *ReleaseOrderManager) BatchDelete(
	ctx context.Context,
	input BatchDeleteReleaseOrdersInput,
) (BatchDeleteReleaseOrdersOutput, error) {
	orderIDs := normalizeBatchDeleteOrderIDs(input.OrderIDs)
	if len(orderIDs) == 0 {
		return BatchDeleteReleaseOrdersOutput{}, fmt.Errorf("%w: order_ids is required", ErrInvalidInput)
	}

	output := BatchDeleteReleaseOrdersOutput{
		DeletedOrderIDs: make([]string, 0, len(orderIDs)),
		Failed:          make([]BatchDeleteReleaseOrdersFailure, 0),
	}

	deletableIDs := make([]string, 0, len(orderIDs))
	for _, orderID := range orderIDs {
		order, err := uc.repo.GetByID(ctx, orderID)
		if err != nil {
			output.Failed = append(output.Failed, BatchDeleteReleaseOrdersFailure{
				OrderID: orderID,
				Reason:  normalizeDeleteReleaseOrderError(err),
			})
			continue
		}
		if err := uc.ensureReleaseOrderDeletable(ctx, order); err != nil {
			output.Failed = append(output.Failed, BatchDeleteReleaseOrdersFailure{
				OrderID: order.ID,
				OrderNo: order.OrderNo,
				Reason:  normalizeDeleteReleaseOrderError(err),
			})
			continue
		}
		deletableIDs = append(deletableIDs, order.ID)
	}

	if len(deletableIDs) == 0 {
		return output, nil
	}

	if err := uc.deleteReleaseOrders(ctx, deletableIDs); err != nil {
		return BatchDeleteReleaseOrdersOutput{}, err
	}
	output.DeletedOrderIDs = append(output.DeletedOrderIDs, deletableIDs...)
	return output, nil
}

// deleteReleaseOrders 删除业务资源并返回处理结果。
func (uc *ReleaseOrderManager) deleteReleaseOrders(ctx context.Context, orderIDs []string) error {
	repo, ok := uc.repo.(releaseOrderDeleteRepository)
	if !ok {
		return fmt.Errorf("%w: release repository does not support deleting orders", ErrInvalidInput)
	}
	if err := repo.DeleteOrders(ctx, orderIDs); err != nil {
		logx.Error("release_order", "delete_failed", err, logx.F("order_ids", strings.Join(orderIDs, ",")))
		return err
	}
	logx.Info("release_order", "delete_success", logx.F("order_ids", strings.Join(orderIDs, ",")))
	return nil
}

// ensureReleaseOrderDeletable 校验前置条件，不满足时写入对应错误响应。
func (uc *ReleaseOrderManager) ensureReleaseOrderDeletable(ctx context.Context, order domain.ReleaseOrder) error {
	if isReleaseOrderDeleteBlockedStatus(order.Status) {
		return fmt.Errorf("%w: 发布单当前状态不可删除", ErrInvalidStatus)
	}
	executions, err := uc.repo.ListExecutions(ctx, order.ID)
	if err != nil {
		return err
	}
	for _, item := range executions {
		if item.Status == domain.ExecutionStatusRunning {
			return fmt.Errorf("%w: 发布单存在执行中的任务，暂不可删除", ErrInvalidStatus)
		}
	}
	return nil
}

// isReleaseOrderDeleteBlockedStatus 删除业务资源并返回处理结果。
func isReleaseOrderDeleteBlockedStatus(status domain.OrderStatus) bool {
	switch status {
	case domain.OrderStatusRunning,
		domain.OrderStatusApproving,
		domain.OrderStatusBuilding,
		domain.OrderStatusQueued,
		domain.OrderStatusDeploying:
		return true
	default:
		return false
	}
}

// normalizeBatchDeleteOrderIDs 标准化输入值，保证后续逻辑使用统一格式。
func normalizeBatchDeleteOrderIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// normalizeDeleteReleaseOrderError 标准化输入值，保证后续逻辑使用统一格式。
func normalizeDeleteReleaseOrderError(err error) string {
	if err == nil {
		return "删除失败"
	}
	switch {
	case errors.Is(err, domain.ErrOrderNotFound):
		return "发布单不存在或已被删除"
	case errors.Is(err, ErrInvalidStatus):
		return "当前状态不可删除"
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "删除失败"
	}
	return message
}
