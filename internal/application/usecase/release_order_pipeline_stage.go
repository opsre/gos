package usecase

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	pipelinedomain "gos/internal/domain/pipeline"
	domain "gos/internal/domain/release"
)

type ReleaseOrderPipelineStageView struct {
	ShowModule   bool
	ExecutorType string
	Message      string
	Stages       []domain.ReleaseOrderPipelineStage
}

// ListPipelineStagesView 查询并返回列表数据。
func (uc *ReleaseOrderManager) ListPipelineStagesView(
	ctx context.Context,
	orderID string,
) (ReleaseOrderPipelineStageView, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return ReleaseOrderPipelineStageView{}, ErrInvalidID
	}

	order, err := uc.repo.GetByID(ctx, orderID)
	if err != nil {
		return ReleaseOrderPipelineStageView{}, err
	}

	executions, err := uc.ListExecutions(ctx, order.ID)
	if err != nil {
		return ReleaseOrderPipelineStageView{}, err
	}

	view := ReleaseOrderPipelineStageView{}
	syncMessages := make([]string, 0)
	for _, execution := range executions {
		provider := strings.ToLower(strings.TrimSpace(execution.Provider))
		if provider != string(pipelinedomain.ProviderJenkins) && provider != string(pipelinedomain.ProviderArgoCD) {
			continue
		}
		view.ShowModule = true
		if view.ExecutorType == "" {
			view.ExecutorType = strings.TrimSpace(execution.Provider)
		} else if !strings.EqualFold(view.ExecutorType, execution.Provider) {
			view.ExecutorType = "mixed"
		}
		binding, bindingErr := uc.resolveExecutionBinding(ctx, order, execution)
		if bindingErr != nil {
			continue
		}
		syncMessage, _ := uc.refreshPipelineStages(ctx, order, execution, binding)
		if strings.TrimSpace(syncMessage) != "" {
			syncMessages = append(syncMessages, syncMessage)
		}
	}
	if !view.ShowModule {
		return view, nil
	}

	stages, err := uc.repo.ListPipelineStages(ctx, order.ID)
	if err != nil {
		return ReleaseOrderPipelineStageView{}, err
	}
	view.Stages = stages

	if len(syncMessages) > 0 {
		view.Message = strings.Join(syncMessages, "；")
		return view, nil
	}

	if len(stages) == 0 {
		view.Message = defaultPipelineStageMessage(order.Status)
	}
	return view, nil
}

// GetPipelineStageLog 查询并返回指定资源数据。
func (uc *ReleaseOrderManager) GetPipelineStageLog(
	ctx context.Context,
	orderID string,
	stageID string,
) (domain.ReleaseOrderPipelineStage, domain.ReleaseOrderPipelineStageLog, error) {
	orderID = strings.TrimSpace(orderID)
	stageID = strings.TrimSpace(stageID)
	if orderID == "" || stageID == "" {
		return domain.ReleaseOrderPipelineStage{}, domain.ReleaseOrderPipelineStageLog{}, ErrInvalidID
	}
	if uc.jenkins == nil {
		return domain.ReleaseOrderPipelineStage{}, domain.ReleaseOrderPipelineStageLog{}, fmt.Errorf("%w: jenkins executor is not configured", ErrInvalidInput)
	}

	stage, err := uc.repo.GetPipelineStageByID(ctx, orderID, stageID)
	if err != nil {
		return domain.ReleaseOrderPipelineStage{}, domain.ReleaseOrderPipelineStageLog{}, err
	}
	order, err := uc.repo.GetByID(ctx, orderID)
	if err != nil {
		return domain.ReleaseOrderPipelineStage{}, domain.ReleaseOrderPipelineStageLog{}, err
	}
	execution, err := uc.repo.GetExecutionByScope(ctx, order.ID, domain.PipelineScope(strings.ToLower(strings.TrimSpace(stage.PipelineScope))))
	if err != nil {
		return domain.ReleaseOrderPipelineStage{}, domain.ReleaseOrderPipelineStageLog{}, err
	}
	if strings.ToLower(strings.TrimSpace(execution.Provider)) != string(pipelinedomain.ProviderJenkins) {
		return domain.ReleaseOrderPipelineStage{}, domain.ReleaseOrderPipelineStageLog{}, fmt.Errorf("%w: only jenkins binding supports pipeline stages", ErrInvalidInput)
	}
	buildURL, message, err := uc.resolveBuildURLForPipelineStages(ctx, order, execution)
	if err != nil {
		return domain.ReleaseOrderPipelineStage{}, domain.ReleaseOrderPipelineStageLog{}, err
	}
	if buildURL == "" {
		if strings.TrimSpace(message) == "" {
			message = defaultPipelineStageMessage(order.Status)
		}
		return domain.ReleaseOrderPipelineStage{}, domain.ReleaseOrderPipelineStageLog{}, fmt.Errorf("%w: %s", ErrInvalidInput, message)
	}

	var logResult domain.ReleaseOrderPipelineStageLog
	if reader, ok := uc.jenkins.(JenkinsReleaseStageLogNamedReader); ok {
		logResult, err = reader.GetBuildStageLogWithName(ctx, buildURL, stage.StageKey, stage.StageName)
	} else {
		logResult, err = uc.jenkins.GetBuildStageLog(ctx, buildURL, stage.StageKey)
	}
	if err != nil {
		if isResourceNotFoundError(err) {
			return domain.ReleaseOrderPipelineStage{}, domain.ReleaseOrderPipelineStageLog{}, fmt.Errorf("%w: Jenkins 阶段日志暂不可用，请稍后刷新", ErrInvalidInput)
		}
		return domain.ReleaseOrderPipelineStage{}, domain.ReleaseOrderPipelineStageLog{}, fmt.Errorf("%w: Jenkins 阶段日志加载失败：%s", ErrInvalidInput, trimPipelineStageError(err))
	}
	logResult.ReleaseOrderID = orderID
	logResult.StageID = stage.ID
	logResult.PipelineScope = stage.PipelineScope
	logResult.ExecutorType = stage.ExecutorType
	if strings.TrimSpace(logResult.StageName) == "" {
		logResult.StageName = stage.StageName
	}
	if logResult.FetchedAt.IsZero() {
		logResult.FetchedAt = uc.now()
	}

	return stage, logResult, nil
}

// refreshPipelineStages 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) refreshPipelineStages(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	binding pipelinedomain.PipelineBinding,
) (string, error) {
	switch binding.Provider {
	case pipelinedomain.ProviderJenkins:
		if uc.jenkins == nil {
			return "Jenkins 阶段同步未配置", fmt.Errorf("jenkins executor is not configured")
		}
	case pipelinedomain.ProviderArgoCD:
		return uc.refreshArgoCDStages(ctx, order, execution, binding)
	default:
		return "", nil
	}

	buildURL, message, err := uc.resolveBuildURLForPipelineStages(ctx, order, execution)
	if err != nil {
		return "Jenkins 阶段同步失败：" + trimPipelineStageError(err), err
	}
	if buildURL == "" {
		return message, nil
	}

	items, err := uc.jenkins.GetBuildStages(ctx, buildURL)
	if err != nil {
		if isResourceNotFoundError(err) {
			return "Jenkins 阶段数据暂不可用，稍后自动重试", nil
		}
		return "Jenkins 阶段同步失败：" + trimPipelineStageError(err), err
	}

	now := uc.now()
	persisted := make([]domain.ReleaseOrderPipelineStage, 0, len(items))
	for _, item := range items {
		stageKey := strings.TrimSpace(item.StageKey)
		if stageKey == "" {
			stageKey = strings.TrimSpace(item.StageName)
		}
		if stageKey == "" {
			continue
		}
		status := item.Status
		if !status.Valid() {
			status = domain.PipelineStageStatusPending
		}
		persisted = append(persisted, domain.ReleaseOrderPipelineStage{
			ID:             stablePipelineStageID(order.ID, string(binding.Provider), string(execution.PipelineScope), stageKey),
			ReleaseOrderID: order.ID,
			ExecutionID:    execution.ID,
			PipelineScope:  strings.TrimSpace(string(execution.PipelineScope)),
			ExecutorType:   strings.TrimSpace(execution.Provider),
			StageKey:       stageKey,
			StageName:      firstNonEmpty(strings.TrimSpace(item.StageName), stageKey),
			Status:         status,
			RawStatus:      strings.TrimSpace(item.RawStatus),
			SortNo:         item.SortNo,
			DurationMillis: item.DurationMillis,
			StartedAt:      item.StartedAt,
			FinishedAt:     item.FinishedAt,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
	}
	existing, err := uc.repo.ListPipelineStages(ctx, order.ID)
	if err != nil {
		return "", err
	}
	merged := make([]domain.ReleaseOrderPipelineStage, 0, len(existing)+len(persisted))
	for _, item := range existing {
		if strings.EqualFold(strings.TrimSpace(item.PipelineScope), string(execution.PipelineScope)) {
			continue
		}
		merged = append(merged, item)
	}
	merged = append(merged, persisted...)

	if err := uc.repo.ReplacePipelineStages(ctx, order.ID, merged); err != nil {
		return "", err
	}
	if len(persisted) == 0 {
		return defaultPipelineStageMessage(order.Status), nil
	}
	return "", nil
}

// resolveBuildURLForPipelineStages 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveBuildURLForPipelineStages(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
) (buildURL string, message string, err error) {
	buildURL = strings.TrimSpace(execution.BuildURL)
	if buildURL != "" {
		return buildURL, "", nil
	}

	queueURL := strings.TrimSpace(execution.QueueURL)
	if queueURL == "" {
		return "", defaultPipelineStageMessage(order.Status), nil
	}
	if uc.jenkins == nil {
		return "", "Jenkins 阶段同步未配置", fmt.Errorf("jenkins executor is not configured")
	}

	resolvedBuildURL, cancelled, why, queueErr := uc.jenkins.GetQueueItem(ctx, queueURL)
	if queueErr != nil {
		if isResourceNotFoundError(queueErr) {
			return "", "Jenkins 队列信息暂不可用，稍后自动重试", nil
		}
		return "", "", queueErr
	}
	if cancelled {
		reason := strings.TrimSpace(why)
		if reason == "" {
			reason = "Jenkins 队列任务已取消"
		}
		return "", reason, nil
	}

	buildURL = strings.TrimSpace(resolvedBuildURL)
	if buildURL == "" {
		return "", "Jenkins 排队中，等待分配构建任务", nil
	}

	_, _ = uc.repo.UpdateExecutionByScope(ctx, order.ID, execution.PipelineScope, domain.ExecutionUpdateInput{
		Status:    execution.Status,
		QueueURL:  queueURL,
		BuildURL:  buildURL,
		StartedAt: execution.StartedAt,
		UpdatedAt: uc.now(),
	})
	return buildURL, "", nil
}

// stablePipelineStageID 封装当前模块的业务处理逻辑。
func stablePipelineStageID(orderID string, executorType string, pipelineScope string, stageKey string) string {
	sum := sha1.Sum([]byte(
		strings.TrimSpace(orderID) + ":" +
			strings.TrimSpace(executorType) + ":" +
			strings.TrimSpace(pipelineScope) + ":" +
			strings.TrimSpace(stageKey),
	))
	return "rps-" + hex.EncodeToString(sum[:12])
}

// defaultPipelineStageMessage 封装当前模块的业务处理逻辑。
func defaultPipelineStageMessage(status domain.OrderStatus) string {
	switch status {
	case domain.OrderStatusPending:
		return "当前发布单尚未执行，执行后将展示 Jenkins 阶段进度。"
	case domain.OrderStatusBuiltWaitingDeploy:
		return "构建已完成，等待部署触发后展示后续阶段进度。"
	case domain.OrderStatusBuilding, domain.OrderStatusRunning:
		return "Jenkins 已触发，等待阶段数据同步。"
	case domain.OrderStatusSuccess, domain.OrderStatusFailed, domain.OrderStatusCancelled:
		return "当前构建未返回阶段数据。"
	default:
		return "等待 Jenkins 阶段数据同步。"
	}
}

// trimPipelineStageError 封装当前模块的业务处理逻辑。
func trimPipelineStageError(err error) string {
	if err == nil {
		return "未知错误"
	}
	message := strings.Join(strings.Fields(strings.TrimSpace(err.Error())), " ")
	if message == "" {
		return "未知错误"
	}
	if len(message) > 180 {
		return message[:180] + "..."
	}
	return message
}

// refreshArgoCDStages 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) refreshArgoCDStages(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	binding pipelinedomain.PipelineBinding,
) (string, error) {
	if uc.argocdRepo == nil || uc.argocdFactory == nil {
		return "ArgoCD 阶段同步未配置", fmt.Errorf("argocd executor is not configured")
	}
	gitopsType, err := uc.resolveOrderGitOpsType(ctx, order)
	if err != nil {
		return "ArgoCD 阶段同步失败：" + trimPipelineStageError(err), err
	}
	steps, err := uc.ListSteps(ctx, order.ID)
	if err != nil {
		return "", err
	}

	var app ArgoCDApplicationSnapshot
	syncMessage := ""
	if syncStep := findStepByCode(steps, scopeStepCode(execution.PipelineScope, "argocd_sync")); syncStep != nil &&
		(syncStep.Status == domain.StepStatusRunning || syncStep.Status == domain.StepStatusSuccess) {
		_, _, client, resolveErr := uc.resolveArgoCDExecutionContext(ctx, order, execution, nil)
		if resolveErr != nil {
			return "ArgoCD 阶段同步失败：" + trimPipelineStageError(resolveErr), resolveErr
		}
		_, loadedApp, appErr := resolveArgoCDApplicationByRef(ctx, client, binding.ExternalRef, strings.TrimSpace(order.EnvCode), gitopsType)
		if appErr != nil {
			if isResourceNotFoundError(appErr) {
				syncMessage = "ArgoCD Application 暂不可用，稍后自动重试"
			} else if strings.TrimSpace(binding.ExternalRef) == "" {
				syncMessage = "ArgoCD 绑定未配置 GitOps 子目录"
			} else {
				return "ArgoCD 阶段同步失败：" + trimPipelineStageError(appErr), appErr
			}
		} else {
			app = loadedApp
		}
	}

	now := uc.now()
	stageDefs := []struct {
		key         string
		defaultName string
	}{
		{key: "gitops_update", defaultName: argoCDUpdateStageName(gitopsType)},
		{key: "git_commit", defaultName: "CD Git 提交"},
		{key: "git_push", defaultName: "CD Git 推送"},
		{key: "argocd_sync", defaultName: "CD 触发 ArgoCD"},
		{key: "health_check", defaultName: "CD 健康检查"},
	}

	persisted := make([]domain.ReleaseOrderPipelineStage, 0, len(stageDefs))
	for idx, def := range stageDefs {
		step := findStepByCode(steps, scopeStepCode(execution.PipelineScope, def.key))
		stageStatus := pipelineStageStatusFromStep(step)
		rawStatus := pipelineStageRawStatusFromStep(step)
		if app != nil {
			switch def.key {
			case "argocd_sync":
				if stageStatus == domain.PipelineStageStatusPending || stageStatus == domain.PipelineStageStatusRunning {
					stageStatus = mapArgoCDSyncStageStatus(app.GetSyncStatus(), app.GetOperationPhase())
				}
				rawStatus = firstNonEmpty(strings.TrimSpace(app.GetOperationPhase()), strings.TrimSpace(app.GetSyncStatus()), rawStatus)
			case "health_check":
				if stageStatus == domain.PipelineStageStatusPending || stageStatus == domain.PipelineStageStatusRunning {
					stageStatus = mapArgoCDHealthStageStatus(app.GetHealthStatus(), app.GetOperationPhase())
				}
				rawStatus = firstNonEmpty(strings.TrimSpace(app.GetHealthStatus()), rawStatus)
			}
		}
		persisted = append(persisted, domain.ReleaseOrderPipelineStage{
			ID:             stablePipelineStageID(order.ID, string(binding.Provider), string(execution.PipelineScope), def.key),
			ReleaseOrderID: order.ID,
			ExecutionID:    execution.ID,
			PipelineScope:  strings.TrimSpace(string(execution.PipelineScope)),
			ExecutorType:   strings.TrimSpace(execution.Provider),
			StageKey:       def.key,
			StageName:      firstNonEmpty(stageNameFromStep(step), def.defaultName),
			Status:         stageStatus,
			RawStatus:      rawStatus,
			SortNo:         idx + 1,
			DurationMillis: computePipelineStageDuration(step, now),
			StartedAt:      stageStartedAt(step),
			FinishedAt:     stageFinishedAt(step),
			CreatedAt:      now,
			UpdatedAt:      now,
		})
	}

	existing, err := uc.repo.ListPipelineStages(ctx, order.ID)
	if err != nil {
		return "", err
	}
	merged := make([]domain.ReleaseOrderPipelineStage, 0, len(existing)+len(persisted))
	for _, item := range existing {
		if strings.EqualFold(strings.TrimSpace(item.PipelineScope), string(execution.PipelineScope)) {
			continue
		}
		merged = append(merged, item)
	}
	merged = append(merged, persisted...)

	if err := uc.repo.ReplacePipelineStages(ctx, order.ID, merged); err != nil {
		return "", err
	}
	return syncMessage, nil
}

// argoCDUpdateStageName 更新业务资源并返回处理结果。
func argoCDUpdateStageName(gitopsType domain.GitOpsType) string {
	if normalizeTemplateGitOpsType(gitopsType, true) == domain.GitOpsTypeHelm {
		return "CD 更新 Helm Values"
	}
	return "CD 更新 GitOps 配置"
}

// pipelineStageStatusFromStep 封装当前模块的业务处理逻辑。
func pipelineStageStatusFromStep(step *domain.ReleaseOrderStep) domain.PipelineStageStatus {
	if step == nil {
		return domain.PipelineStageStatusPending
	}
	switch step.Status {
	case domain.StepStatusSuccess:
		return domain.PipelineStageStatusSuccess
	case domain.StepStatusFailed:
		return domain.PipelineStageStatusFailed
	case domain.StepStatusRunning:
		return domain.PipelineStageStatusRunning
	default:
		return domain.PipelineStageStatusPending
	}
}

// pipelineStageRawStatusFromStep 封装当前模块的业务处理逻辑。
func pipelineStageRawStatusFromStep(step *domain.ReleaseOrderStep) string {
	if step == nil {
		return ""
	}
	return strings.TrimSpace(step.Message)
}

// stageNameFromStep 封装当前模块的业务处理逻辑。
func stageNameFromStep(step *domain.ReleaseOrderStep) string {
	if step == nil {
		return ""
	}
	return strings.TrimSpace(step.StepName)
}

// stageStartedAt 封装当前模块的业务处理逻辑。
func stageStartedAt(step *domain.ReleaseOrderStep) *time.Time {
	if step == nil {
		return nil
	}
	return step.StartedAt
}

// stageFinishedAt 封装当前模块的业务处理逻辑。
func stageFinishedAt(step *domain.ReleaseOrderStep) *time.Time {
	if step == nil {
		return nil
	}
	return step.FinishedAt
}

// computePipelineStageDuration 封装当前模块的业务处理逻辑。
func computePipelineStageDuration(step *domain.ReleaseOrderStep, now time.Time) int64 {
	if step == nil || step.StartedAt == nil {
		return 0
	}
	end := step.FinishedAt
	if end == nil && step.Status == domain.StepStatusRunning {
		end = &now
	}
	if end == nil || end.Before(*step.StartedAt) {
		return 0
	}
	return end.Sub(*step.StartedAt).Milliseconds()
}

// mapArgoCDSyncStageStatus 同步外部或内部状态数据。
func mapArgoCDSyncStageStatus(syncStatus string, operationPhase string) domain.PipelineStageStatus {
	switch strings.ToLower(strings.TrimSpace(operationPhase)) {
	case "failed", "error":
		return domain.PipelineStageStatusFailed
	case "succeeded":
		if strings.EqualFold(strings.TrimSpace(syncStatus), "synced") {
			return domain.PipelineStageStatusSuccess
		}
	}
	switch strings.ToLower(strings.TrimSpace(syncStatus)) {
	case "synced":
		return domain.PipelineStageStatusSuccess
	case "outofsync":
		return domain.PipelineStageStatusRunning
	default:
		if strings.EqualFold(strings.TrimSpace(operationPhase), "running") {
			return domain.PipelineStageStatusRunning
		}
		return domain.PipelineStageStatusPending
	}
}

// mapArgoCDHealthStageStatus 封装当前模块的业务处理逻辑。
func mapArgoCDHealthStageStatus(healthStatus string, operationPhase string) domain.PipelineStageStatus {
	switch strings.ToLower(strings.TrimSpace(healthStatus)) {
	case "healthy":
		return domain.PipelineStageStatusSuccess
	case "degraded", "missing":
		return domain.PipelineStageStatusFailed
	case "progressing":
		return domain.PipelineStageStatusRunning
	default:
		if strings.EqualFold(strings.TrimSpace(operationPhase), "running") {
			return domain.PipelineStageStatusRunning
		}
		return domain.PipelineStageStatusPending
	}
}
