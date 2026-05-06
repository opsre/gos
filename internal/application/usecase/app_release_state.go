package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	pipelinedomain "gos/internal/domain/pipeline"
	domain "gos/internal/domain/release"
)

type RollbackSupportedAction string

const (
	RollbackSupportedActionRollback    RollbackSupportedAction = "rollback"
	RollbackSupportedActionReplay      RollbackSupportedAction = "replay"
	RollbackSupportedActionUnsupported RollbackSupportedAction = "unsupported"
)

type ApplicationRollbackStateView struct {
	StateID        string     `json:"state_id"`
	ReleaseOrderID string     `json:"release_order_id"`
	ReleaseOrderNo string     `json:"release_order_no"`
	TemplateID     string     `json:"template_id"`
	TemplateName   string     `json:"template_name"`
	CDProvider     string     `json:"cd_provider"`
	GitRef         string     `json:"git_ref"`
	HasCIExecution bool       `json:"has_ci_execution"`
	HasCDExecution bool       `json:"has_cd_execution"`
	ImageTag       string     `json:"image_tag"`
	ConfirmedAt    *time.Time `json:"confirmed_at"`
	ConfirmedBy    string     `json:"confirmed_by"`
}

type ApplicationRollbackCapabilityOutput struct {
	ApplicationID   string                       `json:"application_id"`
	ApplicationName string                       `json:"application_name"`
	EnvCode         string                       `json:"env_code"`
	SupportedAction RollbackSupportedAction      `json:"supported_action"`
	Reason          string                       `json:"reason"`
	CurrentState    ApplicationRollbackStateView `json:"current_state"`
	TargetState     ApplicationRollbackStateView `json:"target_state"`
}

type ApplicationRollbackPrecheckParam struct {
	PipelineScope     string `json:"pipeline_scope"`
	ParamKey          string `json:"param_key"`
	ExecutorParamName string `json:"executor_param_name"`
	ParamValue        string `json:"param_value"`
	ValueSource       string `json:"value_source"`
}

type ApplicationRollbackPrecheckOutput struct {
	ApplicationID    string                             `json:"application_id"`
	ApplicationName  string                             `json:"application_name"`
	EnvCode          string                             `json:"env_code"`
	Action           RollbackSupportedAction            `json:"action"`
	SupportedAction  RollbackSupportedAction            `json:"supported_action"`
	Reason           string                             `json:"reason"`
	Executable       bool                               `json:"executable"`
	WaitingForLock   bool                               `json:"waiting_for_lock"`
	AheadCount       int                                `json:"ahead_count"`
	LockEnabled      bool                               `json:"lock_enabled"`
	LockScope        string                             `json:"lock_scope"`
	ConflictStrategy string                             `json:"conflict_strategy"`
	LockKey          string                             `json:"lock_key"`
	ConflictOrderNo  string                             `json:"conflict_order_no"`
	ConflictMessage  string                             `json:"conflict_message"`
	PreviewScope     string                             `json:"preview_scope"`
	TemplateID       string                             `json:"template_id"`
	TemplateName     string                             `json:"template_name"`
	CurrentState     ApplicationRollbackStateView       `json:"current_state"`
	TargetState      ApplicationRollbackStateView       `json:"target_state"`
	Items            []ReleaseOrderPrecheckItem         `json:"items"`
	Params           []ApplicationRollbackPrecheckParam `json:"params"`
}

// RecordAppReleaseState 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) RecordAppReleaseState(ctx context.Context, releaseOrderID string) error {
	if uc == nil || uc.repo == nil {
		return fmt.Errorf("%w: release order manager is not configured", ErrInvalidInput)
	}
	order, err := uc.repo.GetByID(ctx, strings.TrimSpace(releaseOrderID))
	if err != nil {
		return err
	}
	switch order.Status {
	case domain.OrderStatusSuccess, domain.OrderStatusDeploySuccess:
	default:
		return nil
	}

	executions, err := uc.repo.ListExecutions(ctx, order.ID)
	if err != nil {
		return err
	}
	params, err := uc.repo.ListParams(ctx, order.ID)
	if err != nil {
		return err
	}

	paramsPayload, err := json.Marshal(params)
	if err != nil {
		return err
	}
	executionsPayload, err := json.Marshal(executions)
	if err != nil {
		return err
	}

	hasCIExecution := false
	hasCDExecution := false
	cdProvider := ""
	for _, item := range executions {
		switch item.PipelineScope {
		case domain.PipelineScopeCI:
			hasCIExecution = true
		case domain.PipelineScopeCD:
			hasCDExecution = true
			if cdProvider == "" {
				cdProvider = strings.TrimSpace(item.Provider)
			}
		}
	}

	deploySnapshotJSON := ""
	gitopsType := domain.GitOpsType("")
	if snapshot, snapshotErr := uc.repo.GetDeploySnapshotByOrderID(ctx, order.ID); snapshotErr == nil {
		if payload, marshalErr := json.Marshal(snapshot); marshalErr == nil {
			deploySnapshotJSON = string(payload)
			gitopsType = snapshot.GitOpsType
		}
	} else if !errors.Is(snapshotErr, domain.ErrDeploySnapshotNotFound) {
		return snapshotErr
	}

	resultPayload, err := json.Marshal(map[string]any{
		"status":          order.Status,
		"business_status": order.BusinessStatus,
		"source_order_id": order.SourceOrderID,
		"source_order_no": order.SourceOrderNo,
		"started_at":      order.StartedAt,
		"finished_at":     order.FinishedAt,
		"triggered_by":    order.TriggeredBy,
		"executor_name":   order.ExecutorName,
	})
	if err != nil {
		return err
	}

	state := domain.AppReleaseState{
		ID:                    generateID("arst"),
		ReleaseOrderID:        order.ID,
		ReleaseOrderNo:        order.OrderNo,
		ApplicationID:         order.ApplicationID,
		ApplicationName:       order.ApplicationName,
		EnvCode:               strings.TrimSpace(order.EnvCode),
		OperationType:         order.OperationType,
		TemplateID:            order.TemplateID,
		TemplateName:          order.TemplateName,
		CDProvider:            cdProvider,
		GitOpsType:            gitopsType,
		HasCIExecution:        hasCIExecution,
		HasCDExecution:        hasCDExecution,
		GitRef:                strings.TrimSpace(order.GitRef),
		ImageTag:              strings.TrimSpace(order.ImageTag),
		StateStatus:           domain.AppReleaseStateStatusPendingConfirm,
		IsCurrentLive:         false,
		PreviousStateID:       "",
		ConfirmedAt:           nil,
		ConfirmedBy:           "",
		ParamsSnapshotJSON:    string(paramsPayload),
		ExecutionSnapshotJSON: string(executionsPayload),
		DeploySnapshotJSON:    deploySnapshotJSON,
		ResultSnapshotJSON:    string(resultPayload),
		CreatedAt:             releaseOrderFinishedAtOrCreatedAt(order),
		UpdatedAt:             uc.now(),
	}
	return uc.repo.UpsertAppReleaseState(ctx, state)
}

// ConfirmAppReleaseState 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) ConfirmAppReleaseState(
	ctx context.Context,
	releaseOrderID string,
	confirmedBy string,
) (domain.AppReleaseState, error) {
	if uc == nil || uc.repo == nil {
		return domain.AppReleaseState{}, fmt.Errorf("%w: release order manager is not configured", ErrInvalidInput)
	}
	return uc.repo.ConfirmAppReleaseState(ctx, strings.TrimSpace(releaseOrderID), strings.TrimSpace(confirmedBy), uc.now())
}

// CanConfirmAppReleaseState 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) CanConfirmAppReleaseState(
	ctx context.Context,
	releaseOrderID string,
) (bool, error) {
	if uc == nil || uc.repo == nil {
		return false, fmt.Errorf("%w: release order manager is not configured", ErrInvalidInput)
	}
	state, err := uc.repo.GetAppReleaseStateByOrderID(ctx, strings.TrimSpace(releaseOrderID))
	if err != nil {
		if errors.Is(err, domain.ErrAppReleaseStateNotFound) {
			return false, nil
		}
		return false, err
	}
	if state.StateStatus != domain.AppReleaseStateStatusPendingConfirm {
		return false, nil
	}
	return uc.repo.IsLatestOrderByApplicationEnv(ctx, state.ApplicationID, state.EnvCode, state.ReleaseOrderID)
}

// GetAppReleaseStateByOrderID 查询并返回指定资源数据。
func (uc *ReleaseOrderManager) GetAppReleaseStateByOrderID(
	ctx context.Context,
	releaseOrderID string,
) (domain.AppReleaseState, error) {
	if uc == nil || uc.repo == nil {
		return domain.AppReleaseState{}, fmt.Errorf("%w: release order manager is not configured", ErrInvalidInput)
	}
	return uc.repo.GetAppReleaseStateByOrderID(ctx, strings.TrimSpace(releaseOrderID))
}

// ListCurrentAppReleaseStateSummaries 查询并返回列表数据。
func (uc *ReleaseOrderManager) ListCurrentAppReleaseStateSummaries(
	ctx context.Context,
	applicationIDs []string,
) ([]domain.AppReleaseStateSummary, error) {
	if uc == nil || uc.repo == nil {
		return nil, fmt.Errorf("%w: release order manager is not configured", ErrInvalidInput)
	}
	return uc.repo.ListCurrentAppReleaseStateSummaries(ctx, applicationIDs)
}

// GetApplicationRollbackCapability 查询并返回指定资源数据。
func (uc *ReleaseOrderManager) GetApplicationRollbackCapability(
	ctx context.Context,
	applicationID string,
	envCode string,
) (ApplicationRollbackCapabilityOutput, error) {
	if uc == nil || uc.repo == nil {
		return ApplicationRollbackCapabilityOutput{}, fmt.Errorf("%w: release order manager is not configured", ErrInvalidInput)
	}
	appID := strings.TrimSpace(applicationID)
	env := strings.TrimSpace(envCode)
	if appID == "" || env == "" {
		return ApplicationRollbackCapabilityOutput{}, fmt.Errorf("%w: application_id and env_code are required", ErrInvalidInput)
	}

	current, err := uc.repo.GetCurrentAppReleaseState(ctx, appID, env)
	if err != nil {
		if errors.Is(err, domain.ErrAppReleaseStateNotFound) {
			return ApplicationRollbackCapabilityOutput{
				ApplicationID:   appID,
				EnvCode:         env,
				SupportedAction: RollbackSupportedActionUnsupported,
				Reason:          "当前环境还没有已确认生效的版本",
			}, nil
		}
		return ApplicationRollbackCapabilityOutput{}, err
	}

	result := ApplicationRollbackCapabilityOutput{
		ApplicationID:   appID,
		ApplicationName: current.ApplicationName,
		EnvCode:         env,
		CurrentState:    toRollbackStateView(current),
		SupportedAction: RollbackSupportedActionUnsupported,
	}

	if strings.TrimSpace(current.PreviousStateID) == "" {
		result.Reason = "当前环境没有可回退的上一个生效版本"
		return result, nil
	}
	target, err := uc.repo.GetAppReleaseStateByID(ctx, current.PreviousStateID)
	if err != nil {
		if errors.Is(err, domain.ErrAppReleaseStateNotFound) {
			result.Reason = "上一个生效版本记录不存在或已失效"
			return result, nil
		}
		return ApplicationRollbackCapabilityOutput{}, err
	}
	result.TargetState = toRollbackStateView(target)

	switch {
	case target.HasCDExecution &&
		strings.EqualFold(strings.TrimSpace(target.CDProvider), string(pipelinedomain.ProviderArgoCD)) &&
		target.GitOpsType == domain.GitOpsTypeHelm &&
		strings.TrimSpace(target.DeploySnapshotJSON) != "":
		result.SupportedAction = RollbackSupportedActionRollback
		result.Reason = "支持标准回滚"
		return result, nil
	case target.HasCDExecution && !strings.EqualFold(strings.TrimSpace(target.CDProvider), string(pipelinedomain.ProviderArgoCD)):
		result.SupportedAction = RollbackSupportedActionReplay
		result.Reason = "当前目标版本仅支持标准重放"
		return result, nil
	case target.HasCIExecution && !target.HasCDExecution:
		result.SupportedAction = RollbackSupportedActionReplay
		result.Reason = "当前目标版本仅存在 CI 快照，仅支持标准重放"
		return result, nil
	case target.HasCDExecution && strings.EqualFold(strings.TrimSpace(target.CDProvider), string(pipelinedomain.ProviderArgoCD)):
		result.Reason = "当前目标版本缺少可用的 ArgoCD 回滚快照，暂不支持标准回滚"
		return result, nil
	default:
		result.Reason = "当前目标版本缺少可用执行快照，无法回滚或重放"
		return result, nil
	}
}

// CreateApplicationRollbackOrder 创建业务资源并返回处理结果。
func (uc *ReleaseOrderManager) CreateApplicationRollbackOrder(
	ctx context.Context,
	applicationID string,
	envCode string,
	action RollbackSupportedAction,
	creatorUserID string,
	triggeredBy string,
) (domain.ReleaseOrder, error) {
	precheck, err := uc.GetApplicationRollbackPrecheck(ctx, applicationID, envCode, action)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	if precheck.SupportedAction == RollbackSupportedActionUnsupported {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: %s", ErrInvalidInput, precheck.Reason)
	}
	if !precheck.Executable {
		reason := firstNonEmpty(strings.TrimSpace(precheck.ConflictMessage), strings.TrimSpace(precheck.Reason), "当前环境暂不支持创建恢复单")
		if strings.TrimSpace(precheck.ConflictMessage) != "" {
			return domain.ReleaseOrder{}, fmt.Errorf("%w: %s", ErrConcurrentReleaseBlocked, reason)
		}
		return domain.ReleaseOrder{}, fmt.Errorf("%w: %s", ErrInvalidInput, reason)
	}
	sourceOrderID := strings.TrimSpace(precheck.TargetState.ReleaseOrderID)
	if sourceOrderID == "" {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: 未找到可用的来源发布单", ErrInvalidInput)
	}
	switch precheck.Action {
	case RollbackSupportedActionRollback:
		return uc.CreateStandardRollbackByOrder(ctx, sourceOrderID, creatorUserID, triggeredBy)
	case RollbackSupportedActionReplay:
		return uc.CreatePipelineReplayByOrder(ctx, sourceOrderID, creatorUserID, triggeredBy)
	default:
		return domain.ReleaseOrder{}, fmt.Errorf("%w: unsupported rollback action", ErrInvalidInput)
	}
}

// toRollbackStateView 将领域对象转换为接口响应结构。
func toRollbackStateView(state domain.AppReleaseState) ApplicationRollbackStateView {
	return ApplicationRollbackStateView{
		StateID:        state.ID,
		ReleaseOrderID: state.ReleaseOrderID,
		ReleaseOrderNo: state.ReleaseOrderNo,
		TemplateID:     state.TemplateID,
		TemplateName:   state.TemplateName,
		CDProvider:     state.CDProvider,
		GitRef:         state.GitRef,
		HasCIExecution: state.HasCIExecution,
		HasCDExecution: state.HasCDExecution,
		ImageTag:       state.ImageTag,
		ConfirmedAt:    state.ConfirmedAt,
		ConfirmedBy:    state.ConfirmedBy,
	}
}

// GetApplicationRollbackPrecheck 查询并返回指定资源数据。
func (uc *ReleaseOrderManager) GetApplicationRollbackPrecheck(
	ctx context.Context,
	applicationID string,
	envCode string,
	action RollbackSupportedAction,
) (ApplicationRollbackPrecheckOutput, error) {
	capability, err := uc.GetApplicationRollbackCapability(ctx, applicationID, envCode)
	if err != nil {
		return ApplicationRollbackPrecheckOutput{}, err
	}
	output := ApplicationRollbackPrecheckOutput{
		ApplicationID:   capability.ApplicationID,
		ApplicationName: capability.ApplicationName,
		EnvCode:         capability.EnvCode,
		SupportedAction: capability.SupportedAction,
		Reason:          capability.Reason,
		Executable:      true,
		CurrentState:    capability.CurrentState,
		TargetState:     capability.TargetState,
		Items:           make([]ReleaseOrderPrecheckItem, 0, 6),
		Params:          make([]ApplicationRollbackPrecheckParam, 0),
	}

	requestedAction := action
	if requestedAction == "" {
		requestedAction = capability.SupportedAction
	}
	output.Action = requestedAction

	actionItem := ReleaseOrderPrecheckItem{
		Key:     "recovery_action",
		Name:    "恢复方式",
		Status:  ReleaseOrderPrecheckItemStatusPass,
		Message: capability.Reason,
	}
	if strings.TrimSpace(actionItem.Message) == "" {
		actionItem.Message = "恢复方式验证通过"
	}

	switch {
	case capability.SupportedAction == RollbackSupportedActionUnsupported:
		output.Executable = false
		actionItem.Status = ReleaseOrderPrecheckItemStatusBlocked
		if strings.TrimSpace(actionItem.Message) == "" {
			actionItem.Message = "当前环境暂不支持标准回滚或标准重放"
		}
		output.Items = append(output.Items, actionItem)
		return output, nil
	case requestedAction != capability.SupportedAction:
		output.Executable = false
		actionItem.Status = ReleaseOrderPrecheckItemStatusBlocked
		actionItem.Message = fmt.Sprintf("当前环境仅支持 %s，请按支持动作继续", rollbackSupportedActionLabel(capability.SupportedAction))
		output.Reason = actionItem.Message
		output.Items = append(output.Items, actionItem)
		return output, nil
	default:
		output.Items = append(output.Items, actionItem)
	}

	sourceOrderID := strings.TrimSpace(capability.TargetState.ReleaseOrderID)
	if sourceOrderID == "" {
		output.Executable = false
		output.Reason = "未找到可用的来源发布单"
		output.Items = append(output.Items, ReleaseOrderPrecheckItem{
			Key:     "source_order",
			Name:    "来源版本",
			Status:  ReleaseOrderPrecheckItemStatusBlocked,
			Message: output.Reason,
		})
		return output, nil
	}
	sourceOrder, sourceExecutions, err := uc.loadRecoverySourceOrder(ctx, sourceOrderID)
	if err != nil {
		return ApplicationRollbackPrecheckOutput{}, err
	}
	sourceParams, err := uc.repo.ListParams(ctx, sourceOrder.ID)
	if err != nil {
		return ApplicationRollbackPrecheckOutput{}, err
	}
	template, templateBindings, templateParams, _, _, err := uc.repo.GetTemplateByID(ctx, strings.TrimSpace(sourceOrder.TemplateID))
	if err != nil {
		return ApplicationRollbackPrecheckOutput{}, err
	}
	output.TemplateID = strings.TrimSpace(template.ID)
	output.TemplateName = strings.TrimSpace(template.Name)
	output.Items = append(output.Items, ReleaseOrderPrecheckItem{
		Key:     "source_version",
		Name:    "目标版本",
		Status:  ReleaseOrderPrecheckItemStatusPass,
		Message: fmt.Sprintf("将从 %s 恢复，模板 %s", firstNonEmpty(strings.TrimSpace(sourceOrder.OrderNo), "-"), firstNonEmpty(strings.TrimSpace(template.Name), "-")),
	})

	switch requestedAction {
	case RollbackSupportedActionRollback:
		targetState, stateErr := uc.repo.GetAppReleaseStateByID(ctx, capability.TargetState.StateID)
		if stateErr != nil {
			return ApplicationRollbackPrecheckOutput{}, stateErr
		}
		sourceCDExecution, resolveErr := resolveCDExecution(sourceExecutions)
		if resolveErr != nil {
			return ApplicationRollbackPrecheckOutput{}, resolveErr
		}
		if !isArgoCDExecution(sourceCDExecution) {
			output.Executable = false
			output.Reason = "目标版本不是 ArgoCD CD 单元，无法执行标准回滚"
			output.Items = append(output.Items, ReleaseOrderPrecheckItem{
				Key:     "execution_units",
				Name:    "执行单元",
				Status:  ReleaseOrderPrecheckItemStatusBlocked,
				Message: output.Reason,
			})
			return output, nil
		}
		if !canCreateArgoReplayFromStatus(sourceOrder.Status) {
			output.Executable = false
			output.Reason = "目标版本当前状态不支持创建标准回滚单"
			output.Items = append(output.Items, ReleaseOrderPrecheckItem{
				Key:     "source_status",
				Name:    "来源发布单状态",
				Status:  ReleaseOrderPrecheckItemStatusBlocked,
				Message: output.Reason,
			})
			return output, nil
		}
		cdBinding, ok := selectRecoveryTemplateBinding(templateBindings, domain.PipelineScopeCD)
		if !ok {
			output.Executable = false
			output.Reason = "目标版本模板未配置可用的 CD 执行器"
			output.Items = append(output.Items, ReleaseOrderPrecheckItem{
				Key:     "template_binding",
				Name:    "模板执行器",
				Status:  ReleaseOrderPrecheckItemStatusBlocked,
				Message: output.Reason,
			})
			return output, nil
		}
		if !strings.EqualFold(strings.TrimSpace(cdBinding.Provider), string(pipelinedomain.ProviderArgoCD)) {
			output.Executable = false
			output.Reason = "目标版本模板的 CD 执行器不是 ArgoCD，无法执行标准回滚"
			output.Items = append(output.Items, ReleaseOrderPrecheckItem{
				Key:     "template_binding",
				Name:    "模板执行器",
				Status:  ReleaseOrderPrecheckItemStatusBlocked,
				Message: output.Reason,
			})
			return output, nil
		}
		if strings.TrimSpace(targetState.DeploySnapshotJSON) == "" {
			output.Executable = false
			output.Reason = "目标版本缺少可用的 Helm/ArgoCD 部署快照"
			output.Items = append(output.Items, ReleaseOrderPrecheckItem{
				Key:     "deploy_snapshot",
				Name:    "部署快照",
				Status:  ReleaseOrderPrecheckItemStatusBlocked,
				Message: output.Reason,
			})
			return output, nil
		}
		output.PreviewScope = string(domain.PipelineScopeCD)
		output.Items = append(output.Items,
			ReleaseOrderPrecheckItem{
				Key:     "template_binding",
				Name:    "模板执行器",
				Status:  ReleaseOrderPrecheckItemStatusPass,
				Message: "已命中 ArgoCD CD 执行器，可执行标准回滚",
			},
			ReleaseOrderPrecheckItem{
				Key:     "deploy_snapshot",
				Name:    "部署快照",
				Status:  ReleaseOrderPrecheckItemStatusPass,
				Message: "已找到可用的 Helm/ArgoCD 部署快照",
			},
			ReleaseOrderPrecheckItem{
				Key:     "param_snapshot",
				Name:    "参数快照",
				Status:  ReleaseOrderPrecheckItemStatusPass,
				Message: fmt.Sprintf("共加载 %d 个来源参数，用于回滚前审阅", len(sourceParams)),
			},
		)
		output.Params = toApplicationRollbackPrecheckParams(sourceParams)
	case RollbackSupportedActionReplay:
		sourceReplayExecution, resolveErr := resolveReplayExecution(sourceExecutions)
		if resolveErr != nil {
			return ApplicationRollbackPrecheckOutput{}, resolveErr
		}
		replayScope := sourceReplayExecution.PipelineScope
		replayParamsFromSource := filterReleaseOrderParamsByScope(sourceParams, replayScope)
		if replayScope == domain.PipelineScopeCD && len(replayParamsFromSource) == 0 {
			ciParams := filterReleaseOrderParamsByScope(sourceParams, domain.PipelineScopeCI)
			if len(ciParams) > 0 {
				replayScope = domain.PipelineScopeCI
				replayParamsFromSource = ciParams
			}
		}
		if len(replayParamsFromSource) == 0 {
			output.Executable = false
			output.Reason = "来源发布单缺少可重放参数快照，无法执行标准重放"
			output.Items = append(output.Items, ReleaseOrderPrecheckItem{
				Key:     "param_snapshot",
				Name:    "参数快照",
				Status:  ReleaseOrderPrecheckItemStatusBlocked,
				Message: output.Reason,
			})
			return output, nil
		}
		replayBinding, ok := selectRecoveryTemplateBinding(templateBindings, replayScope)
		if !ok {
			output.Executable = false
			output.Reason = fmt.Sprintf("目标版本模板未配置可用的 %s 执行器", strings.ToUpper(string(replayScope)))
			output.Items = append(output.Items, ReleaseOrderPrecheckItem{
				Key:     "template_binding",
				Name:    "模板执行器",
				Status:  ReleaseOrderPrecheckItemStatusBlocked,
				Message: output.Reason,
			})
			return output, nil
		}
		if strings.EqualFold(strings.TrimSpace(replayBinding.Provider), string(pipelinedomain.ProviderArgoCD)) {
			output.Executable = false
			output.Reason = fmt.Sprintf("目标版本模板的 %s 执行器不是管线，无法执行标准重放", strings.ToUpper(string(replayScope)))
			output.Items = append(output.Items, ReleaseOrderPrecheckItem{
				Key:     "template_binding",
				Name:    "模板执行器",
				Status:  ReleaseOrderPrecheckItemStatusBlocked,
				Message: output.Reason,
			})
			return output, nil
		}
		if matchErr := ensureReplayParamsMatchTemplate(templateParams, replayParamsFromSource, replayScope); matchErr != nil {
			output.Executable = false
			output.Reason = matchErr.Error()
			output.Items = append(output.Items, ReleaseOrderPrecheckItem{
				Key:     "param_snapshot",
				Name:    "参数快照",
				Status:  ReleaseOrderPrecheckItemStatusBlocked,
				Message: output.Reason,
			})
			return output, nil
		}
		output.PreviewScope = string(replayScope)
		output.Items = append(output.Items,
			ReleaseOrderPrecheckItem{
				Key:     "template_binding",
				Name:    "模板执行器",
				Status:  ReleaseOrderPrecheckItemStatusPass,
				Message: fmt.Sprintf("已命中 %s 执行器，可执行标准重放", strings.ToUpper(string(replayScope))),
			},
			ReleaseOrderPrecheckItem{
				Key:     "param_snapshot",
				Name:    "参数快照",
				Status:  ReleaseOrderPrecheckItemStatusPass,
				Message: fmt.Sprintf("共加载 %d 个 %s 参数，将按历史值重放", len(replayParamsFromSource), strings.ToUpper(string(replayScope))),
			},
		)
		output.Params = toApplicationRollbackPrecheckParams(replayParamsFromSource)
	default:
		return ApplicationRollbackPrecheckOutput{}, fmt.Errorf("%w: unsupported rollback action", ErrInvalidInput)
	}

	guard, err := uc.evaluateApplicationRollbackGuard(
		ctx,
		sourceOrder.ApplicationID,
		firstNonEmpty(strings.TrimSpace(sourceOrder.EnvCode), strings.TrimSpace(envCode)),
	)
	if err != nil {
		return ApplicationRollbackPrecheckOutput{}, err
	}
	output.LockEnabled = guard.Settings.Enabled
	output.LockScope = string(guard.Settings.LockScope)
	output.ConflictStrategy = string(guard.Settings.ConflictStrategy)
	output.LockKey = guard.LockKey
	output.ConflictOrderNo = strings.TrimSpace(guard.ConflictOrderNo)
	output.ConflictMessage = strings.TrimSpace(guard.Message)
	output.AheadCount = guard.AheadCount
	output.WaitingForLock = guard.WaitingForLock
	if guard.Item.Key != "" {
		if guard.Item.Status == ReleaseOrderPrecheckItemStatusBlocked {
			output.Executable = false
		}
		output.Items = append(output.Items, guard.Item)
	}

	return output, nil
}

type applicationRollbackGuard struct {
	Settings        ReleaseConcurrencySettingsOutput
	LockKey         string
	ConflictOrderNo string
	WaitingForLock  bool
	AheadCount      int
	Message         string
	Item            ReleaseOrderPrecheckItem
}

// evaluateApplicationRollbackGuard 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) evaluateApplicationRollbackGuard(
	ctx context.Context,
	applicationID string,
	envCode string,
) (applicationRollbackGuard, error) {
	settings, err := uc.loadReleaseConcurrencySettings(ctx)
	if err != nil {
		return applicationRollbackGuard{}, err
	}
	guard := applicationRollbackGuard{Settings: settings}
	item := ReleaseOrderPrecheckItem{
		Key:     "concurrency_lock",
		Name:    "并发发布",
		Status:  ReleaseOrderPrecheckItemStatusPass,
		Message: "未检测到执行互斥冲突",
	}
	if settings.Enabled {
		item.Message = fmt.Sprintf("并发控制已启用，当前按 %s 加锁", settings.LockScope)
	}

	conflictOrder, err := uc.repo.FindActiveOrderByApplicationEnv(ctx, applicationID, envCode, "")
	if err != nil && !errors.Is(err, domain.ErrOrderNotFound) {
		return applicationRollbackGuard{}, err
	}
	if err == nil {
		aheadCount, countErr := uc.repo.CountActiveOrdersByApplicationEnv(ctx, applicationID, envCode, "")
		if countErr != nil {
			return applicationRollbackGuard{}, countErr
		}
		if aheadCount <= 0 {
			aheadCount = 1
		}
		guard.AheadCount = aheadCount
		guard.ConflictOrderNo = strings.TrimSpace(conflictOrder.OrderNo)
		guard.Message = fmt.Sprintf("当前应用在环境 %s 前面还有 %d 单，请等待先前执行单结束后再发起恢复", firstNonEmpty(strings.TrimSpace(envCode), "-"), aheadCount)
		item.Status = ReleaseOrderPrecheckItemStatusBlocked
		item.Message = guard.Message
		guard.Item = item
		return guard, nil
	}

	guard.Item = item
	return guard, nil
}

// toApplicationRollbackPrecheckParams 将领域对象转换为接口响应结构。
func toApplicationRollbackPrecheckParams(items []domain.ReleaseOrderParam) []ApplicationRollbackPrecheckParam {
	result := make([]ApplicationRollbackPrecheckParam, 0, len(items))
	sort.SliceStable(items, func(i, j int) bool {
		leftScope := strings.TrimSpace(string(items[i].PipelineScope))
		rightScope := strings.TrimSpace(string(items[j].PipelineScope))
		if leftScope != rightScope {
			if leftScope == string(domain.PipelineScopeCI) {
				return true
			}
			if rightScope == string(domain.PipelineScopeCI) {
				return false
			}
			return leftScope < rightScope
		}
		leftKey := strings.ToLower(strings.TrimSpace(items[i].ParamKey))
		rightKey := strings.ToLower(strings.TrimSpace(items[j].ParamKey))
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		return strings.TrimSpace(items[i].ExecutorParamName) < strings.TrimSpace(items[j].ExecutorParamName)
	})
	for _, item := range items {
		result = append(result, ApplicationRollbackPrecheckParam{
			PipelineScope:     string(item.PipelineScope),
			ParamKey:          strings.TrimSpace(item.ParamKey),
			ExecutorParamName: strings.TrimSpace(item.ExecutorParamName),
			ParamValue:        strings.TrimSpace(item.ParamValue),
			ValueSource:       string(item.ValueSource),
		})
	}
	return result
}

// rollbackSupportedActionLabel 封装当前模块的业务处理逻辑。
func rollbackSupportedActionLabel(action RollbackSupportedAction) string {
	switch action {
	case RollbackSupportedActionRollback:
		return "标准回滚"
	case RollbackSupportedActionReplay:
		return "标准重放"
	default:
		return "不支持"
	}
}

// releaseOrderFinishedAtOrCreatedAt 创建业务资源并返回处理结果。
func releaseOrderFinishedAtOrCreatedAt(order domain.ReleaseOrder) time.Time {
	if order.FinishedAt != nil {
		return order.FinishedAt.UTC()
	}
	return order.CreatedAt.UTC()
}
