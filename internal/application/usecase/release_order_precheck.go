package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	argocddomain "gos/internal/domain/argocdapp"
	pipelinedomain "gos/internal/domain/pipeline"
	domain "gos/internal/domain/release"
)

type ReleaseOrderPrecheckItemStatus string

const (
	ReleaseOrderPrecheckItemStatusPass    ReleaseOrderPrecheckItemStatus = "pass"
	ReleaseOrderPrecheckItemStatusWarn    ReleaseOrderPrecheckItemStatus = "warn"
	ReleaseOrderPrecheckItemStatusBlocked ReleaseOrderPrecheckItemStatus = "blocked"
)

type ReleaseOrderPrecheckItem struct {
	Key     string                         `json:"key"`
	Name    string                         `json:"name"`
	Status  ReleaseOrderPrecheckItemStatus `json:"status"`
	Message string                         `json:"message"`
}

type ReleaseOrderPrecheckOutput struct {
	OrderID          string                     `json:"order_id"`
	OrderNo          string                     `json:"order_no"`
	Executable       bool                       `json:"executable"`
	WaitingForLock   bool                       `json:"waiting_for_lock"`
	AheadCount       int                        `json:"ahead_count"`
	LockEnabled      bool                       `json:"lock_enabled"`
	LockScope        string                     `json:"lock_scope"`
	ConflictStrategy string                     `json:"conflict_strategy"`
	LockKey          string                     `json:"lock_key"`
	ConflictOrderNo  string                     `json:"conflict_order_no"`
	ConflictMessage  string                     `json:"conflict_message"`
	Items            []ReleaseOrderPrecheckItem `json:"items"`
}

type releaseDispatchGuard struct {
	Settings       ReleaseConcurrencySettingsOutput
	LockScope      domain.ExecutionLockScope
	LockKey        string
	ConflictLock   *domain.ReleaseExecutionLock
	ConflictOrder  *domain.ReleaseOrder
	WaitingForLock bool
	AheadCount     int
	Message        string
}

type ReleaseOrderDispatchAction string

const (
	ReleaseOrderDispatchActionExecute ReleaseOrderDispatchAction = "execute"
	ReleaseOrderDispatchActionBuild   ReleaseOrderDispatchAction = "build"
	ReleaseOrderDispatchActionDeploy  ReleaseOrderDispatchAction = "deploy"
)

// PrecheckExecute 检查业务状态并返回校验结果。
func (uc *ReleaseOrderManager) PrecheckExecute(ctx context.Context, id string) (ReleaseOrderPrecheckOutput, error) {
	return uc.precheckOrderDispatch(ctx, id, ReleaseOrderDispatchActionExecute)
}

// PrecheckBuild 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) PrecheckBuild(ctx context.Context, id string) (ReleaseOrderPrecheckOutput, error) {
	return uc.precheckOrderDispatch(ctx, id, ReleaseOrderDispatchActionBuild)
}

// PrecheckDeploy 检查业务状态并返回校验结果。
func (uc *ReleaseOrderManager) PrecheckDeploy(ctx context.Context, id string) (ReleaseOrderPrecheckOutput, error) {
	return uc.precheckOrderDispatch(ctx, id, ReleaseOrderDispatchActionDeploy)
}

// precheckOrderDispatch 检查业务状态并返回校验结果。
func (uc *ReleaseOrderManager) precheckOrderDispatch(
	ctx context.Context,
	id string,
	action ReleaseOrderDispatchAction,
) (ReleaseOrderPrecheckOutput, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ReleaseOrderPrecheckOutput{}, ErrInvalidID
	}
	order, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return ReleaseOrderPrecheckOutput{}, err
	}
	executions, err := uc.repo.ListExecutions(ctx, order.ID)
	if err != nil {
		return ReleaseOrderPrecheckOutput{}, err
	}
	params, err := uc.repo.ListParams(ctx, order.ID)
	if err != nil {
		return ReleaseOrderPrecheckOutput{}, err
	}
	return uc.buildOrderPrecheck(ctx, order, executions, params, action)
}

// buildOrderPrecheck 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildOrderPrecheck(
	ctx context.Context,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	params []domain.ReleaseOrderParam,
	action ReleaseOrderDispatchAction,
) (ReleaseOrderPrecheckOutput, error) {
	output := ReleaseOrderPrecheckOutput{
		OrderID:    order.ID,
		OrderNo:    order.OrderNo,
		Executable: true,
		Items:      make([]ReleaseOrderPrecheckItem, 0, 4),
	}

	statusItem, executionItem, pendingExecution := uc.resolveDispatchPrecheckItems(order, executions, action)
	if statusItem.Status == ReleaseOrderPrecheckItemStatusBlocked {
		output.Executable = false
	}
	output.Items = append(output.Items, statusItem)

	if executionItem.Status == ReleaseOrderPrecheckItemStatusBlocked {
		output.Executable = false
	}
	output.Items = append(output.Items, executionItem)

	if pendingExecution != nil {
		if referenceItem, ok, err := uc.buildExecutionReferencePrecheckItem(ctx, *pendingExecution); err != nil {
			return ReleaseOrderPrecheckOutput{}, err
		} else if ok {
			if referenceItem.Status == ReleaseOrderPrecheckItemStatusBlocked {
				output.Executable = false
			}
			output.Items = append(output.Items, referenceItem)
		}
		guard, err := uc.evaluateDispatchGuard(ctx, order, *pendingExecution, params)
		if err != nil {
			return ReleaseOrderPrecheckOutput{}, err
		}
		output.LockEnabled = guard.Settings.Enabled
		output.LockScope = string(guard.Settings.LockScope)
		output.ConflictStrategy = string(guard.Settings.ConflictStrategy)
		output.LockKey = guard.LockKey
		switch {
		case guard.ConflictLock != nil:
			output.ConflictOrderNo = strings.TrimSpace(guard.ConflictLock.ReleaseOrderNo)
			output.ConflictMessage = strings.TrimSpace(guard.Message)
		case guard.ConflictOrder != nil:
			output.ConflictOrderNo = strings.TrimSpace(guard.ConflictOrder.OrderNo)
			output.ConflictMessage = strings.TrimSpace(guard.Message)
		}
		output.AheadCount = guard.AheadCount
		if guard.Settings.Enabled || guard.ConflictLock != nil || guard.ConflictOrder != nil {
			itemName := "并发发布"
			if !guard.Settings.Enabled && guard.ConflictOrder != nil {
				itemName = "执行顺序"
			}
			item := ReleaseOrderPrecheckItem{
				Key:     "concurrency_lock",
				Name:    itemName,
				Status:  ReleaseOrderPrecheckItemStatusPass,
				Message: "未检测到执行互斥冲突",
			}
			if guard.Settings.Enabled {
				item.Message = fmt.Sprintf("并发控制已启用，当前按 %s 加锁", guard.Settings.LockScope)
			}
			switch {
			case (guard.ConflictLock != nil || guard.ConflictOrder != nil) && guard.WaitingForLock:
				item.Status = ReleaseOrderPrecheckItemStatusWarn
				item.Message = guard.Message
				output.WaitingForLock = true
			case guard.ConflictOrder != nil:
				item.Status = ReleaseOrderPrecheckItemStatusBlocked
				item.Message = guard.Message
				output.Executable = false
			case guard.ConflictLock != nil && guard.Settings.ConflictStrategy == ReleaseConcurrencyConflictStrategyReject:
				item.Status = ReleaseOrderPrecheckItemStatusBlocked
				item.Message = guard.Message
				output.Executable = false
			case guard.ConflictLock != nil && guard.Settings.ConflictStrategy == ReleaseConcurrencyConflictStrategyQueue:
				item.Status = ReleaseOrderPrecheckItemStatusWarn
				item.Message = guard.Message
				output.WaitingForLock = true
			}
			output.Items = append(output.Items, item)
		}
	}

	return output, nil
}

// resolveDispatchPrecheckItems 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveDispatchPrecheckItems(
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	action ReleaseOrderDispatchAction,
) (ReleaseOrderPrecheckItem, ReleaseOrderPrecheckItem, *domain.ReleaseOrderExecution) {
	statusItem := ReleaseOrderPrecheckItem{
		Key:     "order_status",
		Name:    "发布单状态",
		Status:  ReleaseOrderPrecheckItemStatusPass,
		Message: "发布单处于可执行状态",
	}
	executionItem := ReleaseOrderPrecheckItem{
		Key:     "execution_units",
		Name:    "执行单元",
		Status:  ReleaseOrderPrecheckItemStatusPass,
		Message: fmt.Sprintf("已配置 %d 个执行单元", len(executions)),
	}

	switch action {
	case ReleaseOrderDispatchActionBuild:
		hasCI := hasExecutionForScope(executions, domain.PipelineScopeCI)
		hasCD := hasExecutionForScope(executions, domain.PipelineScopeCD)
		switch {
		case order.OperationType != domain.OperationTypeDeploy:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "仅普通发布单支持先构建后部署"
		case !hasCI || !hasCD:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "当前发布单未同时配置 CI / CD 执行单元，无法分段构建"
		default:
			switch order.Status {
			case domain.OrderStatusPending:
				statusItem.Message = "发布单处于待执行状态，可进入构建阶段"
			case domain.OrderStatusApproved:
				statusItem.Message = "发布单已审批通过，可进入构建阶段"
			case domain.OrderStatusBuilding:
				statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
				statusItem.Message = "构建已发起，正在等待构建结果"
			case domain.OrderStatusBuiltWaitingDeploy:
				statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
				statusItem.Message = "构建已完成，请使用部署操作继续执行"
			case domain.OrderStatusPendingApproval:
				statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
				statusItem.Message = "发布单待审批，审批通过后才允许构建"
			case domain.OrderStatusApproving:
				statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
				statusItem.Message = "发布单审批中，审批完成后才允许构建"
			case domain.OrderStatusRejected:
				statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
				statusItem.Message = "发布单审批已拒绝，无法继续触发构建"
			case domain.OrderStatusQueued, domain.OrderStatusRunning, domain.OrderStatusDeploying:
				statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
				statusItem.Message = "发布单已进入执行中，无法重复触发构建"
			default:
				statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
				statusItem.Message = "当前发布单不是可构建状态，无法再次触发构建"
			}
		}
		target := findExecutionByScopeAndStatus(executions, domain.PipelineScopeCI, domain.ExecutionStatusPending)
		if target == nil {
			executionItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			executionItem.Message = "未找到可执行的 CI 构建单元"
		} else {
			executionItem.Message = "已选定 CI 构建单元，将只触发构建阶段"
		}
		return statusItem, executionItem, target
	case ReleaseOrderDispatchActionDeploy:
		switch order.Status {
		case domain.OrderStatusBuiltWaitingDeploy:
			statusItem.Message = "构建已完成，可进入部署阶段"
		case domain.OrderStatusBuilding:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "当前仍在构建中，构建完成后才允许部署"
		case domain.OrderStatusPending, domain.OrderStatusApproved:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "请先完成构建，再执行部署"
		case domain.OrderStatusPendingApproval:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "发布单待审批，审批通过并完成构建后才允许部署"
		case domain.OrderStatusApproving:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "发布单审批中，审批完成并构建成功后才允许部署"
		case domain.OrderStatusRejected:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "发布单审批已拒绝，无法继续部署"
		case domain.OrderStatusQueued, domain.OrderStatusRunning, domain.OrderStatusDeploying:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "部署已发起，无法重复触发"
		default:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "当前发布单不是可部署状态，无法再次触发部署"
		}
		target := findExecutionByScopeAndStatus(executions, domain.PipelineScopeCD, domain.ExecutionStatusPending)
		if target == nil {
			executionItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			executionItem.Message = "未找到可执行的 CD 部署单元"
		} else {
			executionItem.Message = "已选定 CD 部署单元，将只触发部署阶段"
		}
		return statusItem, executionItem, target
	default:
		switch order.Status {
		case domain.OrderStatusPending:
			statusItem.Message = "发布单处于待执行状态"
		case domain.OrderStatusApproved:
			statusItem.Message = "发布单已审批通过，可进入执行阶段"
		case domain.OrderStatusQueued:
			statusItem.Status = ReleaseOrderPrecheckItemStatusWarn
			statusItem.Message = "发布单已进入等待队列"
		case domain.OrderStatusRunning:
			statusItem.Message = "发布单已进入调度中"
		case domain.OrderStatusPendingApproval:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "发布单待审批，审批通过后才允许触发"
		case domain.OrderStatusApproving:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "发布单审批中，审批完成后才允许触发"
		case domain.OrderStatusRejected:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "发布单审批已拒绝，无法继续触发"
		case domain.OrderStatusBuilding:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "当前发布单正在构建中，无法再次触发完整发布"
		case domain.OrderStatusBuiltWaitingDeploy:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "当前发布单已完成构建，请改用部署操作继续执行"
		case domain.OrderStatusDeploying:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "发布单已进入发布中，无法再次触发"
		default:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "当前发布单不是可执行状态，无法再次触发"
		}
		target := findExecutionByStatus(executions, domain.ExecutionStatusPending)
		if len(executions) == 0 || target == nil {
			executionItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			executionItem.Message = "未找到可执行的待执行单元"
		}
		return statusItem, executionItem, target
	}
}

// buildExecutionReferencePrecheckItem 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildExecutionReferencePrecheckItem(
	ctx context.Context,
	execution domain.ReleaseOrderExecution,
) (ReleaseOrderPrecheckItem, bool, error) {
	if uc == nil || uc.pipelineRepo == nil || strings.TrimSpace(execution.BindingID) == "" {
		return ReleaseOrderPrecheckItem{}, false, nil
	}
	item := ReleaseOrderPrecheckItem{
		Key:     "execution_reference",
		Name:    "模板绑定",
		Status:  ReleaseOrderPrecheckItemStatusPass,
		Message: "模板绑定引用正常",
	}

	binding, err := uc.pipelineRepo.GetBindingByID(ctx, execution.BindingID)
	if err == nil {
		if binding.Status == pipelinedomain.StatusInactive {
			if strings.TrimSpace(execution.PipelineID) != "" {
				item.Status = ReleaseOrderPrecheckItemStatusWarn
				item.Message = fmt.Sprintf("模板引用的绑定 %s 已失效，将回退到快照管线 %s 继续执行，建议尽快更新模板绑定", firstNonEmpty(binding.Name, execution.BindingName, execution.BindingID), execution.PipelineID)
			} else {
				item.Status = ReleaseOrderPrecheckItemStatusBlocked
				item.Message = fmt.Sprintf("模板引用的绑定 %s 已失效，且未保存可回退的管线 ID，请先更新模板绑定", firstNonEmpty(binding.Name, execution.BindingName, execution.BindingID))
			}
		}
		return item, true, nil
	}
	if !errors.Is(err, pipelinedomain.ErrBindingNotFound) {
		return ReleaseOrderPrecheckItem{}, false, err
	}
	if strings.TrimSpace(execution.PipelineID) != "" {
		pipeline, pipelineErr := uc.pipelineRepo.GetPipelineByID(ctx, execution.PipelineID)
		if pipelineErr == nil {
			if activeErr := ensureActivePipelineRecord(pipeline, "快照管线"); activeErr == nil {
				item.Status = ReleaseOrderPrecheckItemStatusWarn
				item.Message = fmt.Sprintf("模板引用的绑定 %s 已失效，将回退到快照管线 %s 继续执行，建议尽快更新模板绑定", firstNonEmpty(execution.BindingName, execution.BindingID), execution.PipelineID)
				return item, true, nil
			}
			item.Status = ReleaseOrderPrecheckItemStatusBlocked
			item.Message = fmt.Sprintf("模板引用的绑定 %s 已失效，且快照管线 %s 不可用，请先更新模板绑定", firstNonEmpty(execution.BindingName, execution.BindingID), execution.PipelineID)
			return item, true, nil
		}
		if !errors.Is(pipelineErr, pipelinedomain.ErrPipelineNotFound) {
			return ReleaseOrderPrecheckItem{}, false, pipelineErr
		}
	}
	item.Status = ReleaseOrderPrecheckItemStatusBlocked
	item.Message = fmt.Sprintf("模板引用的绑定 %s 已失效，且未找到可回退的快照管线，请先更新模板绑定", firstNonEmpty(execution.BindingName, execution.BindingID))
	return item, true, nil
}

// evaluateDispatchGuard 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) evaluateDispatchGuard(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	params []domain.ReleaseOrderParam,
) (releaseDispatchGuard, error) {
	settings, err := uc.loadReleaseConcurrencySettings(ctx)
	if err != nil {
		return releaseDispatchGuard{}, err
	}
	guard := releaseDispatchGuard{Settings: settings}

	conflictOrder, err := uc.repo.FindActiveOrderByApplicationEnv(ctx, order.ApplicationID, order.EnvCode, order.ID)
	if err != nil && !errors.Is(err, domain.ErrOrderNotFound) {
		return releaseDispatchGuard{}, err
	}
	if err == nil {
		guard.ConflictOrder = &conflictOrder
		aheadCount, countErr := uc.repo.CountActiveOrdersByApplicationEnv(ctx, order.ApplicationID, order.EnvCode, order.ID)
		if countErr != nil {
			return releaseDispatchGuard{}, countErr
		}
		if aheadCount <= 0 {
			aheadCount = 1
		}
		guard.AheadCount = aheadCount
		guard.Message = fmt.Sprintf("当前应用在环境 %s 前面还有 %d 单，请等待先前执行单结束后再点击发布", firstNonEmpty(strings.TrimSpace(order.EnvCode), "-"), aheadCount)
		return guard, nil
	}

	if !settings.Enabled {
		return guard, nil
	}

	lockScope, lockKey, err := uc.buildExecutionLockIdentity(ctx, order, execution, params, settings)
	if err != nil {
		return releaseDispatchGuard{}, err
	}
	guard.LockScope = lockScope
	guard.LockKey = lockKey

	lock, err := uc.repo.FindActiveExecutionLock(ctx, lockKey, order.ID, uc.now())
	if err != nil && !errors.Is(err, domain.ErrExecutionLockNotFound) {
		return releaseDispatchGuard{}, err
	}
	if err == nil {
		guard.ConflictLock = &lock
		if uc.shouldQueueInConcurrentBatch(ctx, order, lock) {
			guard.WaitingForLock = true
			guard.Message = fmt.Sprintf("当前批次的同应用同环境发布单 %s 正在执行，已进入顺序等待队列", firstNonEmpty(lock.ReleaseOrderNo, lock.ReleaseOrderID))
			return guard, nil
		}
		if settings.ConflictStrategy == ReleaseConcurrencyConflictStrategyQueue {
			guard.WaitingForLock = true
			guard.Message = fmt.Sprintf("当前目标已被发布单 %s 占用，已进入排队等待", firstNonEmpty(lock.ReleaseOrderNo, lock.ReleaseOrderID))
			return guard, nil
		}
		guard.Message = fmt.Sprintf("当前目标已被发布单 %s 占用，请稍后再试", firstNonEmpty(lock.ReleaseOrderNo, lock.ReleaseOrderID))
	}
	return guard, nil
}

// ensureExecutionLock 校验前置条件，不满足时写入对应错误响应。
func (uc *ReleaseOrderManager) ensureExecutionLock(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	params []domain.ReleaseOrderParam,
) (releaseDispatchGuard, bool, error) {
	guard, err := uc.evaluateDispatchGuard(ctx, order, execution, params)
	if err != nil {
		return releaseDispatchGuard{}, false, err
	}
	if guard.ConflictOrder != nil {
		if guard.WaitingForLock {
			return guard, false, nil
		}
		return guard, false, fmt.Errorf("%w: %s", ErrConcurrentReleaseBlocked, guard.Message)
	}
	if !guard.Settings.Enabled {
		return guard, true, nil
	}
	if guard.ConflictLock != nil {
		if guard.WaitingForLock {
			return guard, false, nil
		}
		if guard.Settings.ConflictStrategy == ReleaseConcurrencyConflictStrategyQueue {
			return guard, false, nil
		}
		return guard, false, fmt.Errorf("%w: %s", ErrConcurrentReleaseBlocked, guard.Message)
	}
	lock := domain.ReleaseExecutionLock{
		ID:             generateID("rlk"),
		LockScope:      guard.LockScope,
		LockKey:        guard.LockKey,
		ApplicationID:  order.ApplicationID,
		EnvCode:        order.EnvCode,
		ReleaseOrderID: order.ID,
		ReleaseOrderNo: order.OrderNo,
		Status:         domain.ExecutionLockStatusActive,
		OwnerType:      "release_order",
		CreatedAt:      uc.now(),
	}
	expiredAt := uc.now().Add(time.Duration(guard.Settings.LockTimeoutSec) * time.Second)
	lock.ExpiredAt = &expiredAt
	_, acquired, err := uc.repo.AcquireExecutionLock(ctx, lock, uc.now())
	if err != nil {
		return releaseDispatchGuard{}, false, err
	}
	if !acquired {
		if guard.WaitingForLock || guard.Settings.ConflictStrategy == ReleaseConcurrencyConflictStrategyQueue {
			return guard, false, nil
		}
		if guard.Settings.ConflictStrategy == ReleaseConcurrencyConflictStrategyQueue {
			guard.WaitingForLock = true
			return guard, false, nil
		}
		return releaseDispatchGuard{}, false, fmt.Errorf("%w: %s", ErrConcurrentReleaseBlocked, guard.Message)
	}
	return guard, true, nil
}

// touchExecutionLocks 将领域对象转换为接口响应结构。
func (uc *ReleaseOrderManager) touchExecutionLocks(ctx context.Context, order domain.ReleaseOrder) error {
	settings, err := uc.loadReleaseConcurrencySettings(ctx)
	if err != nil || !settings.Enabled {
		return err
	}
	expires := uc.now().Add(time.Duration(settings.LockTimeoutSec) * time.Second)
	return uc.repo.TouchExecutionLocksByOrderID(ctx, order.ID, expires)
}

// releaseExecutionLocks 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) releaseExecutionLocks(ctx context.Context, orderID string, status domain.ExecutionLockStatus) error {
	if uc == nil || uc.repo == nil || strings.TrimSpace(orderID) == "" {
		return nil
	}
	return uc.repo.ReleaseExecutionLocksByOrderID(ctx, orderID, status, uc.now())
}

// loadReleaseConcurrencySettings 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) loadReleaseConcurrencySettings(ctx context.Context) (ReleaseConcurrencySettingsOutput, error) {
	if uc == nil || uc.releaseSettings == nil {
		return normalizeConcurrencySettings(ReleaseConcurrencySettingsOutput{}), nil
	}
	settings, err := uc.releaseSettings.LoadConcurrencySettings(ctx)
	if err != nil {
		return ReleaseConcurrencySettingsOutput{}, err
	}
	return normalizeConcurrencySettings(settings), nil
}

// buildExecutionLockIdentity 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildExecutionLockIdentity(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	params []domain.ReleaseOrderParam,
	settings ReleaseConcurrencySettingsOutput,
) (domain.ExecutionLockScope, string, error) {
	scope := domain.ExecutionLockScope(settings.LockScope)
	switch settings.LockScope {
	case ReleaseConcurrencyLockScopeApplication:
		return scope, fmt.Sprintf("app:%s", strings.TrimSpace(order.ApplicationID)), nil
	case ReleaseConcurrencyLockScopeGitOpsRepoBranch:
		if isArgoCDExecution(execution) {
			if key, err := uc.buildGitOpsRepoBranchLockKey(ctx, order, execution, params); err == nil && strings.TrimSpace(key) != "" {
				return scope, key, nil
			}
		}
		fallthrough
	case ReleaseConcurrencyLockScopeApplicationEnv:
		return domain.ExecutionLockScopeApplicationEnv, fmt.Sprintf("app:%s:env:%s", strings.TrimSpace(order.ApplicationID), strings.TrimSpace(order.EnvCode)), nil
	default:
		return domain.ExecutionLockScopeApplicationEnv, fmt.Sprintf("app:%s:env:%s", strings.TrimSpace(order.ApplicationID), strings.TrimSpace(order.EnvCode)), nil
	}
}

// buildGitOpsRepoBranchLockKey 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildGitOpsRepoBranchLockKey(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	params []domain.ReleaseOrderParam,
) (string, error) {
	snapshot, err := uc.repo.GetDeploySnapshotByOrderID(ctx, order.ID)
	if err == nil && strings.TrimSpace(snapshot.RepoURL) != "" {
		branch := uc.resolveGitOpsBranchByApplication(
			ctx,
			order.ApplicationID,
			firstNonEmpty(strings.TrimSpace(snapshot.EnvCode), strings.TrimSpace(order.EnvCode)),
			argocddomain.Instance{},
			strings.TrimSpace(snapshot.Branch),
		)
		return fmt.Sprintf("repo:%s:branch:%s", strings.TrimSpace(snapshot.RepoURL), branch), nil
	}
	if err != nil && !errors.Is(err, domain.ErrDeploySnapshotNotFound) {
		return "", err
	}
	template, _, _, _, _, err := uc.repo.GetTemplateByID(ctx, strings.TrimSpace(order.TemplateID))
	if err != nil {
		return "", err
	}
	binding, argocdInstance, client, err := uc.resolveArgoCDExecutionContext(ctx, order, execution, params)
	if err != nil {
		return "", err
	}
	environment := uc.resolveArgoCDEnvironment(order, params)
	appName, app, err := resolveArgoCDApplicationByRef(ctx, client, binding.ExternalRef, environment, normalizeTemplateGitOpsType(template.GitOpsType, true))
	_ = appName
	if err != nil {
		return "", err
	}
	repoURL := strings.TrimSpace(app.GetRepoURL())
	branch := uc.resolveGitOpsTargetBranch(ctx, order, params, argocdInstance, app)
	if repoURL == "" {
		return "", fmt.Errorf("argocd application repo url is empty")
	}
	return fmt.Sprintf("repo:%s:branch:%s", repoURL, branch), nil
}
