package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	domain "gos/internal/domain/release"
)

type BatchExecuteReleaseOrdersInput struct {
	OrderIDs           []string
	BatchName          string
	StagedDispatchMode BatchExecuteStagedDispatchMode
}

type BatchExecuteReleaseOrdersOutput struct {
	BatchNo        string                `json:"batch_no"`
	BatchName      string                `json:"batch_name"`
	Orders         []domain.ReleaseOrder `json:"orders"`
	DispatchErrors []string              `json:"dispatch_errors"`
}

type BatchExecuteStagedDispatchMode string

const (
	BatchExecuteStagedDispatchModeExecute BatchExecuteStagedDispatchMode = "execute"
	BatchExecuteStagedDispatchModeBuild   BatchExecuteStagedDispatchMode = "build"
)

type ReleaseOrderConcurrentBatchQueueState string

const (
	ReleaseOrderConcurrentBatchQueueStatePending   ReleaseOrderConcurrentBatchQueueState = "pending"
	ReleaseOrderConcurrentBatchQueueStateQueued    ReleaseOrderConcurrentBatchQueueState = "queued"
	ReleaseOrderConcurrentBatchQueueStateExecuting ReleaseOrderConcurrentBatchQueueState = "executing"
	ReleaseOrderConcurrentBatchQueueStateSuccess   ReleaseOrderConcurrentBatchQueueState = "success"
	ReleaseOrderConcurrentBatchQueueStateFailed    ReleaseOrderConcurrentBatchQueueState = "failed"
	ReleaseOrderConcurrentBatchQueueStateCancelled ReleaseOrderConcurrentBatchQueueState = "cancelled"
)

type ReleaseOrderConcurrentBatchProgressItem struct {
	OrderID             string                                `json:"order_id"`
	OrderNo             string                                `json:"order_no"`
	ApplicationID       string                                `json:"application_id"`
	ApplicationName     string                                `json:"application_name"`
	EnvCode             string                                `json:"env_code"`
	Status              domain.OrderStatus                    `json:"status"`
	OperationType       domain.OperationType                  `json:"operation_type"`
	ConcurrentBatchSeq  int                                   `json:"concurrent_batch_seq"`
	QueueState          ReleaseOrderConcurrentBatchQueueState `json:"queue_state"`
	QueuePosition       int                                   `json:"queue_position"`
	HasRunningExecution bool                                  `json:"has_running_execution"`
	StartedAt           *time.Time                            `json:"started_at"`
	FinishedAt          *time.Time                            `json:"finished_at"`
}

type ReleaseOrderConcurrentBatchProgressOutput struct {
	OrderID      string                                    `json:"order_id"`
	OrderNo      string                                    `json:"order_no"`
	BatchNo      string                                    `json:"batch_no"`
	BatchName    string                                    `json:"batch_name"`
	IsConcurrent bool                                      `json:"is_concurrent"`
	Total        int                                       `json:"total"`
	Queued       int                                       `json:"queued"`
	Executing    int                                       `json:"executing"`
	Success      int                                       `json:"success"`
	Failed       int                                       `json:"failed"`
	Cancelled    int                                       `json:"cancelled"`
	Items        []ReleaseOrderConcurrentBatchProgressItem `json:"items"`
}

// BatchExecute 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) BatchExecute(ctx context.Context, input BatchExecuteReleaseOrdersInput) (BatchExecuteReleaseOrdersOutput, error) {
	orderIDs := normalizeBatchExecuteOrderIDs(input.OrderIDs)
	if len(orderIDs) < 2 {
		return BatchExecuteReleaseOrdersOutput{}, fmt.Errorf("%w: 至少选择两张待执行发布单", ErrInvalidInput)
	}
	batchName := strings.TrimSpace(input.BatchName)
	if batchName == "" {
		return BatchExecuteReleaseOrdersOutput{}, fmt.Errorf("%w: 并发名称不能为空", ErrInvalidInput)
	}
	mode := normalizeBatchExecuteStagedDispatchMode(input.StagedDispatchMode)

	orders := make([]domain.ReleaseOrder, 0, len(orderIDs))
	dispatchActions := make(map[string]ReleaseOrderDispatchAction, len(orderIDs))
	blockedMessages := make([]string, 0)
	for _, orderID := range orderIDs {
		item, err := uc.repo.GetByID(ctx, orderID)
		if err != nil {
			return BatchExecuteReleaseOrdersOutput{}, err
		}
		if !isExecutableOrderStatus(item.Status) {
			return BatchExecuteReleaseOrdersOutput{}, fmt.Errorf("%w: 发布单 %s 不处于待执行或已批准状态", ErrInvalidInput, item.OrderNo)
		}
		executions, err := uc.repo.ListExecutions(ctx, item.ID)
		if err != nil {
			return BatchExecuteReleaseOrdersOutput{}, err
		}
		params, err := uc.repo.ListParams(ctx, item.ID)
		if err != nil {
			return BatchExecuteReleaseOrdersOutput{}, err
		}
		dispatchAction := resolveBatchExecuteDispatchAction(item, executions, mode)
		dispatchActions[item.ID] = dispatchAction
		precheck, err := uc.buildOrderPrecheck(ctx, item, executions, params, dispatchAction)
		if err != nil {
			return BatchExecuteReleaseOrdersOutput{}, err
		}
		if !precheck.Executable {
			reason := strings.TrimSpace(precheck.ConflictMessage)
			if reason == "" {
				for _, precheckItem := range precheck.Items {
					if precheckItem.Status == ReleaseOrderPrecheckItemStatusBlocked {
						reason = strings.TrimSpace(precheckItem.Message)
						break
					}
				}
			}
			if reason == "" {
				reason = "当前发布单未通过执行前预检"
			}
			blockedMessages = append(blockedMessages, fmt.Sprintf("%s：%s", item.OrderNo, reason))
		}
		orders = append(orders, item)
	}
	if len(blockedMessages) > 0 {
		return BatchExecuteReleaseOrdersOutput{}, fmt.Errorf("%w: %s", ErrConcurrentReleaseBlocked, strings.Join(blockedMessages, "；"))
	}

	batchNo := generateConcurrentBatchNo(uc.now())
	if err := uc.repo.UpdateConcurrentBatch(ctx, orderIDs, batchNo, batchName, true); err != nil {
		return BatchExecuteReleaseOrdersOutput{}, err
	}

	output := BatchExecuteReleaseOrdersOutput{
		BatchNo:        batchNo,
		BatchName:      batchName,
		Orders:         make([]domain.ReleaseOrder, 0, len(orderIDs)),
		DispatchErrors: make([]string, 0),
	}
	for _, item := range orders {
		action := dispatchActions[item.ID]
		if action == "" {
			action = ReleaseOrderDispatchActionExecute
		}
		var (
			updated domain.ReleaseOrder
			err     error
		)
		if action == ReleaseOrderDispatchActionBuild {
			updated, err = uc.Build(ctx, item.ID, "", "")
		} else {
			updated, err = uc.Execute(ctx, item.ID, "", "")
		}
		if err != nil {
			output.DispatchErrors = append(output.DispatchErrors, fmt.Sprintf("%s：%s", item.OrderNo, normalizeBatchDispatchErrorMessage(err)))
			current, currentErr := uc.repo.GetByID(ctx, item.ID)
			if currentErr == nil {
				output.Orders = append(output.Orders, current)
				continue
			}
			output.Orders = append(output.Orders, item)
			continue
		}
		output.Orders = append(output.Orders, updated)
	}
	return output, nil
}

// GetConcurrentBatchProgress 查询并返回指定资源数据。
func (uc *ReleaseOrderManager) GetConcurrentBatchProgress(ctx context.Context, id string) (ReleaseOrderConcurrentBatchProgressOutput, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ReleaseOrderConcurrentBatchProgressOutput{}, ErrInvalidID
	}
	order, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return ReleaseOrderConcurrentBatchProgressOutput{}, err
	}
	return uc.getStoredConcurrentBatchProgress(ctx, order)
}

func (uc *ReleaseOrderManager) getStoredConcurrentBatchProgress(
	ctx context.Context,
	order domain.ReleaseOrder,
) (ReleaseOrderConcurrentBatchProgressOutput, error) {
	output := ReleaseOrderConcurrentBatchProgressOutput{
		OrderID:      order.ID,
		OrderNo:      order.OrderNo,
		BatchNo:      strings.TrimSpace(order.ConcurrentBatchNo),
		BatchName:    strings.TrimSpace(order.ConcurrentBatchName),
		IsConcurrent: order.IsConcurrent,
		Items:        make([]ReleaseOrderConcurrentBatchProgressItem, 0),
	}
	if !order.IsConcurrent || strings.TrimSpace(order.ConcurrentBatchNo) == "" {
		output.Items = append(output.Items, ReleaseOrderConcurrentBatchProgressItem{
			OrderID:            order.ID,
			OrderNo:            order.OrderNo,
			ApplicationID:      order.ApplicationID,
			ApplicationName:    order.ApplicationName,
			EnvCode:            order.EnvCode,
			Status:             order.Status,
			OperationType:      order.OperationType,
			ConcurrentBatchSeq: order.ConcurrentBatchSeq,
			QueueState:         resolveConcurrentBatchQueueState(order.Status, false),
			StartedAt:          order.StartedAt,
			FinishedAt:         order.FinishedAt,
		})
		output.Total = 1
		return output, nil
	}

	orders, err := uc.repo.ListByConcurrentBatchNo(ctx, order.ConcurrentBatchNo)
	if err != nil {
		return ReleaseOrderConcurrentBatchProgressOutput{}, err
	}
	sort.SliceStable(orders, func(i, j int) bool {
		if orders[i].ConcurrentBatchSeq != orders[j].ConcurrentBatchSeq {
			return orders[i].ConcurrentBatchSeq < orders[j].ConcurrentBatchSeq
		}
		return orders[i].CreatedAt.Before(orders[j].CreatedAt)
	})

	type itemWithGroup struct {
		item     ReleaseOrderConcurrentBatchProgressItem
		groupKey string
	}
	items := make([]itemWithGroup, 0, len(orders))
	grouped := make(map[string][]int)
	for _, current := range orders {
		executions, execErr := uc.repo.ListExecutions(ctx, current.ID)
		if execErr != nil {
			return ReleaseOrderConcurrentBatchProgressOutput{}, execErr
		}
		hasRunning := hasRunningExecution(executions)
		item := ReleaseOrderConcurrentBatchProgressItem{
			OrderID:             current.ID,
			OrderNo:             current.OrderNo,
			ApplicationID:       current.ApplicationID,
			ApplicationName:     current.ApplicationName,
			EnvCode:             current.EnvCode,
			Status:              current.Status,
			OperationType:       current.OperationType,
			ConcurrentBatchSeq:  current.ConcurrentBatchSeq,
			HasRunningExecution: hasRunning,
			StartedAt:           current.StartedAt,
			FinishedAt:          current.FinishedAt,
		}
		groupKey := strings.TrimSpace(current.ApplicationID) + "::" + strings.TrimSpace(current.EnvCode)
		grouped[groupKey] = append(grouped[groupKey], len(items))
		items = append(items, itemWithGroup{item: item, groupKey: groupKey})
	}

	for _, indexes := range grouped {
		for _, idx := range indexes {
			current := &items[idx].item
			if current.Status.IsTerminal() {
				current.QueueState = resolveConcurrentBatchQueueState(current.Status, current.HasRunningExecution)
				continue
			}
			current.QueueState = resolveConcurrentBatchQueueState(current.Status, current.HasRunningExecution)
		}
		queuePosition := 0
		for _, idx := range indexes {
			current := &items[idx].item
			if current.QueueState != ReleaseOrderConcurrentBatchQueueStateQueued {
				continue
			}
			queuePosition++
			current.QueuePosition = queuePosition
		}
	}

	output.Total = len(items)
	for _, wrapped := range items {
		output.Items = append(output.Items, wrapped.item)
		switch wrapped.item.QueueState {
		case ReleaseOrderConcurrentBatchQueueStateQueued:
			output.Queued++
		case ReleaseOrderConcurrentBatchQueueStateExecuting:
			output.Executing++
		case ReleaseOrderConcurrentBatchQueueStateSuccess:
			output.Success++
		case ReleaseOrderConcurrentBatchQueueStateFailed:
			output.Failed++
		case ReleaseOrderConcurrentBatchQueueStateCancelled:
			output.Cancelled++
		}
	}
	return output, nil
}

// normalizeBatchExecuteOrderIDs 标准化输入值，保证后续逻辑使用统一格式。
func normalizeBatchExecuteOrderIDs(values []string) []string {
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

// generateConcurrentBatchNo 封装当前模块的业务处理逻辑。
func generateConcurrentBatchNo(now time.Time) string {
	entropy := make([]byte, 3)
	if _, err := rand.Read(entropy); err != nil {
		return "CB-" + now.UTC().Format("20060102150405")
	}
	return "CB-" + now.UTC().Format("20060102150405") + "-" + strings.ToUpper(hex.EncodeToString(entropy))
}

// normalizeBatchDispatchErrorMessage 标准化输入值，保证后续逻辑使用统一格式。
func normalizeBatchDispatchErrorMessage(err error) string {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "执行失败"
	}
	return message
}

// normalizeBatchExecuteStagedDispatchMode 标准化输入值，保证后续逻辑使用统一格式。
func normalizeBatchExecuteStagedDispatchMode(mode BatchExecuteStagedDispatchMode) BatchExecuteStagedDispatchMode {
	switch strings.ToLower(strings.TrimSpace(string(mode))) {
	case string(BatchExecuteStagedDispatchModeBuild):
		return BatchExecuteStagedDispatchModeBuild
	default:
		return BatchExecuteStagedDispatchModeExecute
	}
}

// resolveBatchExecuteDispatchAction 解析上下文数据，得到后续流程需要的结果。
func resolveBatchExecuteDispatchAction(
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	mode BatchExecuteStagedDispatchMode,
) ReleaseOrderDispatchAction {
	if mode != BatchExecuteStagedDispatchModeBuild {
		return ReleaseOrderDispatchActionExecute
	}
	if !supportsBatchStagedDispatch(order, executions) {
		return ReleaseOrderDispatchActionExecute
	}
	return ReleaseOrderDispatchActionBuild
}

// supportsBatchStagedDispatch 封装当前模块的业务处理逻辑。
func supportsBatchStagedDispatch(order domain.ReleaseOrder, executions []domain.ReleaseOrderExecution) bool {
	if order.OperationType != domain.OperationTypeDeploy {
		return false
	}
	hasCI := false
	hasCD := false
	for _, item := range executions {
		switch item.PipelineScope {
		case domain.PipelineScopeCI:
			hasCI = true
		case domain.PipelineScopeCD:
			hasCD = true
		}
	}
	return hasCI && hasCD
}

// resolveConcurrentBatchQueueState 解析上下文数据，得到后续流程需要的结果。
func resolveConcurrentBatchQueueState(
	status domain.OrderStatus,
	hasRunningExecution bool,
) ReleaseOrderConcurrentBatchQueueState {
	switch status {
	case domain.OrderStatusSuccess, domain.OrderStatusDeploySuccess:
		return ReleaseOrderConcurrentBatchQueueStateSuccess
	case domain.OrderStatusFailed, domain.OrderStatusDeployFailed:
		return ReleaseOrderConcurrentBatchQueueStateFailed
	case domain.OrderStatusCancelled:
		return ReleaseOrderConcurrentBatchQueueStateCancelled
	case domain.OrderStatusQueued:
		return ReleaseOrderConcurrentBatchQueueStateQueued
	case domain.OrderStatusBuilding:
		if hasRunningExecution {
			return ReleaseOrderConcurrentBatchQueueStateExecuting
		}
		return ReleaseOrderConcurrentBatchQueueStateQueued
	case domain.OrderStatusDeploying:
		return ReleaseOrderConcurrentBatchQueueStateExecuting
	case domain.OrderStatusBuiltWaitingDeploy:
		return ReleaseOrderConcurrentBatchQueueStatePending
	case domain.OrderStatusPending, domain.OrderStatusApproved:
		return ReleaseOrderConcurrentBatchQueueStatePending
	}
	if hasRunningExecution {
		return ReleaseOrderConcurrentBatchQueueStateExecuting
	}
	if status == domain.OrderStatusRunning {
		return ReleaseOrderConcurrentBatchQueueStateQueued
	}
	return ReleaseOrderConcurrentBatchQueueStatePending
}

// hasRunningExecution 封装当前模块的业务处理逻辑。
func hasRunningExecution(executions []domain.ReleaseOrderExecution) bool {
	for _, item := range executions {
		if item.Status == domain.ExecutionStatusRunning {
			return true
		}
	}
	return false
}

// shouldQueueInConcurrentBatch 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) shouldQueueInConcurrentBatch(
	ctx context.Context,
	order domain.ReleaseOrder,
	lock domain.ReleaseExecutionLock,
) bool {
	if uc == nil || uc.repo == nil || !order.IsConcurrent {
		return false
	}
	batchNo := strings.TrimSpace(order.ConcurrentBatchNo)
	if batchNo == "" || strings.TrimSpace(lock.ReleaseOrderID) == "" {
		return false
	}
	conflictOrder, err := uc.repo.GetByID(ctx, lock.ReleaseOrderID)
	if err != nil {
		return false
	}
	return conflictOrder.IsConcurrent &&
		strings.TrimSpace(conflictOrder.ConcurrentBatchNo) == batchNo &&
		strings.TrimSpace(conflictOrder.ApplicationID) == strings.TrimSpace(order.ApplicationID) &&
		strings.TrimSpace(conflictOrder.EnvCode) == strings.TrimSpace(order.EnvCode)
}

// shouldQueueBehindConcurrentOrder 判断两个发布单是否属于同一并发批次的同应用同环境队列。
func shouldQueueBehindConcurrentOrder(order domain.ReleaseOrder, conflict domain.ReleaseOrder) bool {
	batchNo := strings.TrimSpace(order.ConcurrentBatchNo)
	return order.IsConcurrent &&
		conflict.IsConcurrent &&
		batchNo != "" &&
		strings.TrimSpace(conflict.ConcurrentBatchNo) == batchNo &&
		strings.TrimSpace(conflict.ApplicationID) == strings.TrimSpace(order.ApplicationID) &&
		strings.TrimSpace(conflict.EnvCode) == strings.TrimSpace(order.EnvCode)
}
