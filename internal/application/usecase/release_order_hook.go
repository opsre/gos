package usecase

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	agentdomain "gos/internal/domain/agent"
	notificationdomain "gos/internal/domain/notification"
	domain "gos/internal/domain/release"
)

var hookTaskIDPattern = regexp.MustCompile(`task_id=([A-Za-z0-9-]+)`)
var hookTaskLabelPattern = regexp.MustCompile(`任务号[:：]\s*([A-Za-z0-9-]+)`)
var hookSourceTaskIDPattern = regexp.MustCompile(`source_task_id=([A-Za-z0-9-]+)`)
var hookDispatchBatchIDPattern = regexp.MustCompile(`batch_id=([A-Za-z0-9-]+)`)
var unresolvedNotificationCorePlaceholderPattern = regexp.MustCompile(`(?i)\{(?:release_stage_rich|release_status_rich)\}`)
var templateWebhookHTTPTimeout = 10 * time.Second

// syncHooksAfterRelease 同步外部或内部状态数据。
func (uc *ReleaseOrderManager) syncHooksAfterRelease(
	ctx context.Context,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
) (bool, bool, domain.OrderStatus, string, error) {
	mainStatus, finalMessage, mainDone := evaluateMainReleaseStatus(executions)
	if !mainDone {
		return false, false, order.Status, "", nil
	}

	if err := uc.ensureMainReleaseFinishStep(ctx, order.ID, mainStatus); err != nil {
		return false, false, order.Status, "", err
	}

	if order.Status.IsTerminal() {
		var err error
		now := uc.now()
		order, err = uc.repo.UpdateStatus(ctx, order.ID, domain.OrderStatusRunning, order.StartedAt, nil, now)
		if err != nil {
			return false, false, order.Status, "", err
		}
	}

	updated, finished, blockingHookFailed, err := uc.syncHooksForStage(
		ctx,
		order,
		executions,
		domain.TemplateHookExecuteStagePostRelease,
		mainStatus,
	)
	if err != nil {
		return false, false, order.Status, "", err
	}
	if !finished {
		return updated, false, domain.OrderStatusRunning, "", nil
	}
	if blockingHookFailed {
		return updated, true, domain.OrderStatusFailed, "Hook 执行失败", nil
	}
	return updated, true, mainStatus, finalMessage, nil
}

// syncHooksAfterBuild 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) syncHooksAfterBuild(
	ctx context.Context,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
) (bool, bool, bool, error) {
	ciExecution := findExecutionByScope(executions, domain.PipelineScopeCI)
	if ciExecution == nil || ciExecution.Status != domain.ExecutionStatusSuccess {
		return false, true, false, nil
	}
	return uc.syncHooksForStage(ctx, order, executions, domain.TemplateHookExecuteStageBuildComplete, domain.OrderStatusSuccess)
}

// evaluateMainReleaseStatus 启动当前进程并完成依赖初始化。
func evaluateMainReleaseStatus(executions []domain.ReleaseOrderExecution) (domain.OrderStatus, string, bool) {
	if len(executions) == 0 {
		return domain.OrderStatusSuccess, "发布完成", true
	}

	orderStatus := domain.OrderStatusSuccess
	message := "发布完成"
	for _, item := range executions {
		switch item.Status {
		case domain.ExecutionStatusFailed:
			orderStatus = domain.OrderStatusFailed
			message = "存在失败执行单元"
		case domain.ExecutionStatusCancelled:
			if orderStatus != domain.OrderStatusFailed {
				orderStatus = domain.OrderStatusCancelled
				message = "存在已取消执行单元"
			}
		case domain.ExecutionStatusPending, domain.ExecutionStatusRunning:
			return domain.OrderStatusRunning, "", false
		}
	}
	return orderStatus, message, true
}

// findExecutionByScope 封装当前模块的业务处理逻辑。
func findExecutionByScope(items []domain.ReleaseOrderExecution, scope domain.PipelineScope) *domain.ReleaseOrderExecution {
	for idx := range items {
		if items[idx].PipelineScope == scope {
			return &items[idx]
		}
	}
	return nil
}

// ensureMainReleaseFinishStep 校验前置条件，不满足时写入对应错误响应。
func (uc *ReleaseOrderManager) ensureMainReleaseFinishStep(
	ctx context.Context,
	orderID string,
	mainStatus domain.OrderStatus,
) error {
	if mainStatus != domain.OrderStatusSuccess {
		return uc.markStepFinished(ctx, orderID, "global:release_finish", domain.StepStatusFailed, "主发布流程失败")
	}
	current, err := uc.repo.GetStepByCode(ctx, orderID, "global:release_finish")
	if err != nil {
		if errors.Is(err, domain.ErrStepNotFound) {
			return nil
		}
		return err
	}
	now := uc.now()
	startedAt := current.StartedAt
	if startedAt == nil {
		startedAt = &now
	}
	return uc.markStep(ctx, orderID, "global:release_finish", domain.StepStatusRunning, "主发布流程完成，等待发布后 Hook 执行", startedAt, nil)
}

// collectHookSteps 封装当前模块的业务处理逻辑。
func collectHookSteps(steps []domain.ReleaseOrderStep) []domain.ReleaseOrderStep {
	result := make([]domain.ReleaseOrderStep, 0)
	for _, item := range steps {
		code := strings.ToLower(strings.TrimSpace(item.StepCode))
		if strings.HasPrefix(code, "hook:") {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SortNo != result[j].SortNo {
			return result[i].SortNo < result[j].SortNo
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

// parseHookExecuteStage 解析输入内容并返回结构化结果。
func parseHookExecuteStage(stepCode string) domain.TemplateHookExecuteStage {
	parts := strings.Split(strings.TrimSpace(stepCode), ":")
	if len(parts) >= 4 {
		return domain.TemplateHookExecuteStage(strings.TrimSpace(parts[1]))
	}
	return domain.TemplateHookExecuteStagePostRelease
}

// parseHookSortNo 解析输入内容并返回结构化结果。
func parseHookSortNo(stepCode string) int {
	parts := strings.Split(strings.TrimSpace(stepCode), ":")
	if len(parts) < 3 {
		return 0
	}
	value, _ := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
	return value
}

// shouldTriggerTemplateHook 封装当前模块的业务处理逻辑。
func shouldTriggerTemplateHook(condition domain.TemplateHookTriggerCondition, mainStatus domain.OrderStatus) bool {
	switch condition {
	case domain.TemplateHookTriggerOnFailed:
		return mainStatus == domain.OrderStatusFailed || mainStatus == domain.OrderStatusCancelled
	case domain.TemplateHookTriggerAlways:
		return true
	default:
		return mainStatus == domain.OrderStatusSuccess
	}
}

// hookMatchesOrderEnv 封装当前模块的业务处理逻辑。
func hookMatchesOrderEnv(envCodes []string, orderEnvCode string) bool {
	if len(envCodes) == 0 {
		return true
	}
	normalizedOrderEnv := strings.TrimSpace(orderEnvCode)
	if normalizedOrderEnv == "" {
		return false
	}
	for _, item := range envCodes {
		if strings.EqualFold(strings.TrimSpace(item), normalizedOrderEnv) {
			return true
		}
	}
	return false
}

// buildTemplateHookEnvSkipMessage 组装业务执行所需的输入数据。
func buildTemplateHookEnvSkipMessage(envCodes []string, orderEnvCode string) string {
	normalizedOrderEnv := strings.TrimSpace(orderEnvCode)
	if normalizedOrderEnv == "" {
		return "已按环境条件跳过"
	}
	filtered := make([]string, 0, len(envCodes))
	for _, item := range envCodes {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		filtered = append(filtered, value)
	}
	if len(filtered) == 0 {
		return "已按环境条件跳过"
	}
	return fmt.Sprintf("当前环境 %s 未命中 Hook 执行环境（%s），已跳过", normalizedOrderEnv, strings.Join(filtered, " / "))
}

// syncHooksForStage 同步外部或内部状态数据。
func (uc *ReleaseOrderManager) syncHooksForStage(
	ctx context.Context,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	executeStage domain.TemplateHookExecuteStage,
	triggerStatus domain.OrderStatus,
) (bool, bool, bool, error) {
	steps, err := uc.repo.ListSteps(ctx, order.ID)
	if err != nil {
		return false, false, false, err
	}
	hookSteps := collectHookSteps(steps)
	if len(hookSteps) == 0 {
		return false, true, false, nil
	}

	templateHooks, err := uc.loadTemplateHooksForOrder(ctx, order)
	if err != nil {
		return false, false, false, err
	}
	hookBySort := make(map[int]domain.ReleaseTemplateHook, len(templateHooks))
	hasStageHook := false
	for _, item := range templateHooks {
		item.ExecuteStages = domain.NormalizeTemplateHookExecuteStages(item.ExecuteStages, item.ExecuteStage)
		item.ExecuteStage = domain.PrimaryTemplateHookExecuteStage(item.ExecuteStages, item.ExecuteStage)
		hookBySort[item.SortNo] = item
		if domain.TemplateHookHasExecuteStage(item.ExecuteStages, item.ExecuteStage, executeStage) {
			hasStageHook = true
		}
	}
	if !hasStageHook {
		return false, true, false, nil
	}

	updated := false
	blockingHookFailed := false
	for _, step := range hookSteps {
		if parseHookExecuteStage(step.StepCode) != executeStage {
			continue
		}
		hookCfg, ok := hookBySort[parseHookSortNo(step.StepCode)]
		if !ok {
			if step.Status != domain.StepStatusFailed && step.Status != domain.StepStatusSuccess {
				if err := uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusFailed, "未找到 Hook 模板快照，无法继续执行"); err != nil {
					return updated, false, blockingHookFailed, err
				}
				updated = true
			}
			blockingHookFailed = true
			continue
		}

		if !hookMatchesOrderEnv(hookCfg.EnvCodes, order.EnvCode) {
			if step.Status != domain.StepStatusSuccess {
				if err := uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusSuccess, buildTemplateHookEnvSkipMessage(hookCfg.EnvCodes, order.EnvCode)); err != nil {
					return updated, false, blockingHookFailed, err
				}
				updated = true
			}
			continue
		}

		if !shouldTriggerTemplateHook(hookCfg.TriggerCondition, triggerStatus) {
			if step.Status != domain.StepStatusSuccess {
				if err := uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusSuccess, "已按触发条件跳过"); err != nil {
					return updated, false, blockingHookFailed, err
				}
				updated = true
			}
			continue
		}

		switch step.Status {
		case domain.StepStatusPending:
			stepUpdated, dispatchErr := uc.dispatchTemplateHookStep(ctx, order, executions, hookCfg, step)
			if dispatchErr != nil {
				if hookCfg.FailurePolicy == domain.TemplateHookFailurePolicyWarnOnly {
					if err := uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusSuccess, "Hook 执行失败已忽略："+dispatchErr.Error()); err != nil {
						return updated, false, blockingHookFailed, err
					}
					updated = true
					return updated, false, blockingHookFailed, nil
				}
				if err := uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusFailed, dispatchErr.Error()); err != nil {
					return updated, false, blockingHookFailed, err
				}
				updated = true
				if hookFailureBlocksRelease(hookCfg) {
					blockingHookFailed = true
					return updated, false, blockingHookFailed, nil
				}
				continue
			}
			updated = updated || stepUpdated
			return updated, false, blockingHookFailed, nil
		case domain.StepStatusRunning:
			stepUpdated, finished, failed, syncErr := uc.syncRunningHookStep(ctx, order, hookCfg, step)
			if syncErr != nil {
				return updated, false, blockingHookFailed, syncErr
			}
			updated = updated || stepUpdated
			if !finished {
				return updated, false, blockingHookFailed, nil
			}
			if failed && hookFailureBlocksRelease(hookCfg) {
				blockingHookFailed = true
			}
		case domain.StepStatusFailed:
			if hookFailureBlocksRelease(hookCfg) {
				blockingHookFailed = true
			}
		}
	}

	return updated, true, blockingHookFailed, nil
}

func hookFailureBlocksRelease(hook domain.ReleaseTemplateHook) bool {
	if hook.FailurePolicy != domain.TemplateHookFailurePolicyBlockRelease {
		return false
	}
	switch hook.HookType {
	case domain.TemplateHookTypeNotificationHook, domain.TemplateHookTypeWebhookNotification:
		return false
	default:
		return true
	}
}

func (uc *ReleaseOrderManager) hasBlockingFailedPostReleaseHook(
	ctx context.Context,
	order domain.ReleaseOrder,
	steps []domain.ReleaseOrderStep,
) (bool, error) {
	templateHooks, err := uc.loadTemplateHooksForOrder(ctx, order)
	if err != nil {
		return false, err
	}
	hookBySort := make(map[int]domain.ReleaseTemplateHook, len(templateHooks))
	for _, item := range templateHooks {
		hookBySort[item.SortNo] = item
	}
	for _, step := range steps {
		if step.Status != domain.StepStatusFailed ||
			parseHookExecuteStage(step.StepCode) != domain.TemplateHookExecuteStagePostRelease {
			continue
		}
		hook, ok := hookBySort[parseHookSortNo(step.StepCode)]
		if !ok || hookFailureBlocksRelease(hook) {
			return true, nil
		}
	}
	return false, nil
}

// loadTemplateHooksForOrder 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) loadTemplateHooksForOrder(ctx context.Context, order domain.ReleaseOrder) ([]domain.ReleaseTemplateHook, error) {
	templateID := strings.TrimSpace(order.TemplateID)
	if templateID == "" {
		return nil, nil
	}
	_, _, _, _, hooks, err := uc.repo.GetTemplateByID(ctx, templateID)
	if err != nil {
		return nil, err
	}
	sort.Slice(hooks, func(i, j int) bool {
		if hooks[i].SortNo != hooks[j].SortNo {
			return hooks[i].SortNo < hooks[j].SortNo
		}
		return hooks[i].CreatedAt.Before(hooks[j].CreatedAt)
	})
	return hooks, nil
}

// dispatchTemplateHookStep 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) dispatchTemplateHookStep(
	ctx context.Context,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	hook domain.ReleaseTemplateHook,
	step domain.ReleaseOrderStep,
) (bool, error) {
	switch hook.HookType {
	case domain.TemplateHookTypeAgentTask:
		return uc.dispatchAgentTaskHookStep(ctx, order, executions, hook, step)
	case domain.TemplateHookTypeNotificationHook:
		return uc.dispatchNotificationHookStep(ctx, order, executions, hook, step)
	case domain.TemplateHookTypeWebhookNotification:
		return uc.dispatchWebhookHookStep(ctx, order, executions, hook, step)
	default:
		return false, fmt.Errorf("%w: unsupported hook type %s", ErrInvalidInput, hook.HookType)
	}
}

// dispatchAgentTaskHookStep 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) dispatchAgentTaskHookStep(
	ctx context.Context,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	hook domain.ReleaseTemplateHook,
	step domain.ReleaseOrderStep,
) (bool, error) {
	if uc.agentRepo == nil {
		return false, fmt.Errorf("%w: agent repository is not configured", ErrInvalidInput)
	}

	sourceTask, err := uc.agentRepo.GetTaskByID(ctx, strings.TrimSpace(hook.TargetID))
	if err != nil {
		return false, err
	}
	sourceTask, err = syncManagedScriptSnapshotForTask(ctx, uc.agentRepo, sourceTask, nil)
	if err != nil {
		return false, err
	}
	if !isReusableAgentTaskHookTarget(sourceTask) {
		return false, fmt.Errorf("%w: hook target task must be a manual temporary task", ErrInvalidInput)
	}
	variables, err := uc.buildHookTaskVariables(ctx, order, executions, hook, parseHookExecuteStage(step.StepCode))
	if err != nil {
		return false, err
	}
	mergeAgentTaskVariables(variables, sourceTask.Variables)
	targets, err := resolveTaskDispatchTargets(ctx, uc.agentRepo, sourceTask)
	if err != nil {
		return false, err
	}
	batchID := generateID("agbatch")
	dispatchedTasks, err := dispatchTemporaryTaskBatch(
		ctx,
		uc.agentRepo,
		sourceTask,
		targets,
		fmt.Sprintf("%s · %s", firstNonEmpty(strings.TrimSpace(hook.Name), "发布后 Hook"), strings.TrimSpace(order.OrderNo)),
		variables,
		"release_hook",
		batchID,
		uc.now,
	)
	if err != nil {
		return false, err
	}
	now := uc.now()
	message := buildHookTaskBatchProgressMessage(hook, sourceTask, dispatchedTasks, batchID)
	if step.Status == domain.StepStatusPending {
		return true, uc.markStep(ctx, order.ID, step.StepCode, domain.StepStatusRunning, message, &now, nil)
	}
	return true, uc.markStep(ctx, order.ID, step.StepCode, domain.StepStatusRunning, message, step.StartedAt, nil)
}

// mergeAgentTaskVariables 封装当前模块的业务处理逻辑。
func mergeAgentTaskVariables(target map[string]string, taskVariables map[string]string) {
	if len(taskVariables) == 0 {
		return
	}
	for key, value := range taskVariables {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			continue
		}
		target[normalizedKey] = strings.TrimSpace(value)
	}
}

// dispatchWebhookHookStep 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) dispatchWebhookHookStep(
	ctx context.Context,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	hook domain.ReleaseTemplateHook,
	step domain.ReleaseOrderStep,
) (bool, error) {
	webhookURL := strings.TrimSpace(hook.WebhookURL)
	if webhookURL == "" {
		return false, fmt.Errorf("%w: webhook url is required", ErrInvalidInput)
	}
	variables, err := uc.buildHookTaskVariables(ctx, order, executions, hook, parseHookExecuteStage(step.StepCode))
	if err != nil {
		return false, err
	}
	body := renderHookString(variables, strings.TrimSpace(hook.WebhookBody))
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(firstNonEmpty(strings.TrimSpace(hook.WebhookMethod), "POST")), webhookURL, bytes.NewBufferString(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := sendTemplateWebhook(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	message := fmt.Sprintf("Webhook：%s %s", req.Method, webhookURL)
	if len(bytes.TrimSpace(respBody)) > 0 {
		message += "，响应：" + strings.TrimSpace(string(respBody))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		summary := strings.TrimSpace(string(respBody))
		if summary == "" {
			return false, fmt.Errorf("%w: webhook returned %d", ErrInvalidInput, resp.StatusCode)
		}
		return false, fmt.Errorf("%w: webhook returned %d: %s", ErrInvalidInput, resp.StatusCode, summary)
	}
	if step.Status == domain.StepStatusPending {
		now := uc.now()
		if err := uc.markStep(ctx, order.ID, step.StepCode, domain.StepStatusRunning, message, &now, nil); err != nil {
			return false, err
		}
		return true, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusSuccess, message)
	}
	return true, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusSuccess, message)
}

// dispatchNotificationHookStep 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) dispatchNotificationHookStep(
	ctx context.Context,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	hook domain.ReleaseTemplateHook,
	step domain.ReleaseOrderStep,
) (bool, error) {
	if uc.notificationRepo == nil {
		return false, fmt.Errorf("%w: notification repository is not configured", ErrInvalidInput)
	}

	notificationHook, err := uc.notificationRepo.GetHookByID(ctx, strings.TrimSpace(hook.TargetID))
	if err != nil {
		return false, err
	}
	if !notificationHook.Enabled {
		return false, fmt.Errorf("%w: notification hook is disabled", ErrInvalidInput)
	}
	source, err := uc.notificationRepo.GetSourceByID(ctx, notificationHook.SourceID)
	if err != nil {
		return false, err
	}
	if !source.Enabled {
		return false, fmt.Errorf("%w: notification source is disabled", ErrInvalidInput)
	}
	template, err := uc.notificationRepo.GetMarkdownTemplateByID(ctx, notificationHook.MarkdownTemplateID)
	if err != nil {
		return false, err
	}
	if !template.Enabled {
		return false, fmt.Errorf("%w: notification markdown template is disabled", ErrInvalidInput)
	}

	executeStage := parseHookExecuteStage(step.StepCode)
	variables, err := uc.buildHookTaskVariables(ctx, order, executions, hook, executeStage)
	if err != nil {
		return false, err
	}
	enforceNotificationCoreVariables(order, executions, executeStage, variables)
	title, body := renderNotificationMarkdownTemplate(variables, template)
	if containsUnresolvedNotificationCorePlaceholder(title) || containsUnresolvedNotificationCorePlaceholder(body) {
		return false, fmt.Errorf("%w: notification markdown has unresolved core placeholders", ErrInvalidInput)
	}
	if strings.TrimSpace(title) == "" && strings.TrimSpace(body) == "" {
		return false, fmt.Errorf("%w: notification markdown content is empty", ErrInvalidInput)
	}
	req, err := buildNotificationHookRequest(ctx, source, title, body)
	if err != nil {
		return false, err
	}
	resp, err := sendTemplateWebhook(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	message := fmt.Sprintf("通知 Hook：%s · %s", firstNonEmpty(strings.TrimSpace(notificationHook.Name), strings.TrimSpace(hook.Name), strings.TrimSpace(notificationHook.ID)), source.Name)
	if len(bytes.TrimSpace(respBody)) > 0 {
		message += "，响应：" + strings.TrimSpace(string(respBody))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		summary := strings.TrimSpace(string(respBody))
		if summary == "" {
			return false, fmt.Errorf("%w: notification hook returned %d", ErrInvalidInput, resp.StatusCode)
		}
		return false, fmt.Errorf("%w: notification hook returned %d: %s", ErrInvalidInput, resp.StatusCode, summary)
	}
	if step.Status == domain.StepStatusPending {
		now := uc.now()
		if err := uc.markStep(ctx, order.ID, step.StepCode, domain.StepStatusRunning, message, &now, nil); err != nil {
			return false, err
		}
	}
	return true, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusSuccess, message)
}

// buildNotificationHookRequest 组装业务执行所需的输入数据。
func buildNotificationHookRequest(ctx context.Context, source notificationdomain.Source, title string, body string) (*http.Request, error) {
	webhookURL := strings.TrimSpace(source.WebhookURL)
	if webhookURL == "" {
		return nil, fmt.Errorf("%w: notification source webhook_url is required", ErrInvalidInput)
	}
	if source.SourceType == notificationdomain.SourceTypeDingTalk {
		signedURL, err := buildDingTalkWebhookURL(webhookURL, source.VerificationParam)
		if err != nil {
			return nil, err
		}
		webhookURL = signedURL
	}
	payload := make(map[string]any)
	switch source.SourceType {
	case notificationdomain.SourceTypeDingTalk:
		dingTitle := strings.TrimSpace(firstNonEmpty(title, "GOS Release Notification"))
		dingText := strings.TrimSpace(firstNonEmpty(body, title))
		if keywords := strings.TrimSpace(source.Keywords); keywords != "" && !strings.Contains(dingTitle, keywords) && !strings.Contains(dingText, keywords) {
			dingText = strings.TrimSpace(keywords + "\n\n" + dingText)
		}
		payload["msgtype"] = "markdown"
		payload["markdown"] = map[string]string{
			"title": dingTitle,
			"text":  dingText,
		}
	case notificationdomain.SourceTypeWeCom:
		content := strings.TrimSpace(body)
		if content == "" {
			content = strings.TrimSpace(title)
		} else if strings.TrimSpace(title) != "" {
			content = fmt.Sprintf("## %s\n\n%s", strings.TrimSpace(title), content)
		}
		payload["msgtype"] = "markdown"
		payload["markdown"] = map[string]string{
			"content": content,
		}
	case notificationdomain.SourceTypeFeishu:
		feishuTitle := buildFeishuNotificationTitle(title, source.VerificationParam)
		payload["msg_type"] = "interactive"
		payload["card"] = map[string]any{
			"config": map[string]bool{
				"wide_screen_mode": true,
			},
			"header": map[string]any{
				"title": map[string]string{
					"tag":     "plain_text",
					"content": feishuTitle,
				},
			},
			"elements": []map[string]string{
				{
					"tag":     "markdown",
					"content": strings.TrimSpace(firstNonEmpty(body, feishuTitle)),
				},
			},
		}
	default:
		return nil, fmt.Errorf("%w: unsupported notification source type %s", ErrInvalidInput, source.SourceType)
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// buildFeishuNotificationTitle 组装业务执行所需的输入数据。
func buildFeishuNotificationTitle(title string, keyword string) string {
	normalizedTitle := strings.TrimSpace(firstNonEmpty(title, "GOS Release Notification"))
	keyword = strings.TrimSpace(keyword)
	if keyword == "" || strings.Contains(normalizedTitle, keyword) {
		return normalizedTitle
	}
	return strings.TrimSpace(keyword + " " + normalizedTitle)
}

// buildDingTalkWebhookURL 组装业务执行所需的输入数据。
func buildDingTalkWebhookURL(webhookURL string, verificationParam string) (string, error) {
	secret := strings.TrimSpace(verificationParam)
	if secret == "" {
		return webhookURL, nil
	}
	parsedURL, err := url.Parse(webhookURL)
	if err != nil {
		return "", fmt.Errorf("%w: invalid dingtalk webhook_url", ErrInvalidInput)
	}
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	stringToSign := timestamp + "\n" + secret
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	query := parsedURL.Query()
	query.Set("timestamp", timestamp)
	query.Set("sign", signature)
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

// sendTemplateWebhook 封装当前模块的业务处理逻辑。
func sendTemplateWebhook(req *http.Request) (*http.Response, error) {
	webhookClient := &http.Client{Timeout: templateWebhookHTTPTimeout}
	return webhookClient.Do(req)
}

// syncRunningHookStep 同步外部或内部状态数据。
func (uc *ReleaseOrderManager) syncRunningHookStep(
	ctx context.Context,
	order domain.ReleaseOrder,
	hook domain.ReleaseTemplateHook,
	step domain.ReleaseOrderStep,
) (bool, bool, bool, error) {
	switch hook.HookType {
	case domain.TemplateHookTypeAgentTask:
		return uc.syncRunningAgentTaskHookStep(ctx, order, hook, step)
	case domain.TemplateHookTypeNotificationHook:
		return false, true, false, nil
	case domain.TemplateHookTypeWebhookNotification:
		return false, true, false, nil
	default:
		return false, true, true, fmt.Errorf("%w: unsupported hook type %s", ErrInvalidInput, hook.HookType)
	}
}

// syncRunningAgentTaskHookStep 同步外部或内部状态数据。
func (uc *ReleaseOrderManager) syncRunningAgentTaskHookStep(
	ctx context.Context,
	order domain.ReleaseOrder,
	hook domain.ReleaseTemplateHook,
	step domain.ReleaseOrderStep,
) (bool, bool, bool, error) {
	sourceTaskID, batchID := parseHookTaskBatchIdentity(step.Message)
	if sourceTaskID != "" && batchID != "" {
		tasks, _, err := uc.agentRepo.ListTasks(ctx, agentdomain.TaskListFilter{
			SourceTaskID:    sourceTaskID,
			DispatchBatchID: batchID,
			Page:            1,
			PageSize:        500,
		})
		if err != nil {
			return false, false, false, err
		}
		if len(tasks) == 0 {
			failMessage := "Hook 任务批次不存在，无法继续追踪"
			if hook.FailurePolicy == domain.TemplateHookFailurePolicyWarnOnly {
				return true, true, false, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusSuccess, "Hook 执行失败已忽略："+failMessage)
			}
			return true, true, true, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusFailed, failMessage)
		}
		sourceTask, err := uc.agentRepo.GetTaskByID(ctx, sourceTaskID)
		if err != nil {
			sourceTask = agentdomain.Task{
				ID:   sourceTaskID,
				Name: firstNonEmpty(strings.TrimSpace(hook.TargetName), sourceTaskID),
			}
		}
		progressMessage := buildHookTaskBatchProgressMessage(hook, sourceTask, tasks, batchID)
		switch aggregateTaskBatchStatus(tasks) {
		case agentdomain.TaskStatusPending, agentdomain.TaskStatusQueued, agentdomain.TaskStatusClaimed, agentdomain.TaskStatusRunning:
			if strings.TrimSpace(progressMessage) != strings.TrimSpace(step.Message) {
				return true, false, false, uc.markStep(ctx, order.ID, step.StepCode, domain.StepStatusRunning, progressMessage, step.StartedAt, nil)
			}
			return false, false, false, nil
		case agentdomain.TaskStatusSuccess:
			return true, true, false, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusSuccess, buildHookTaskBatchTerminalMessage(hook, sourceTask, tasks, batchID, "执行成功"))
		case agentdomain.TaskStatusCancelled:
			if hook.FailurePolicy == domain.TemplateHookFailurePolicyWarnOnly {
				return true, true, false, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusSuccess, "Hook 执行已取消并忽略："+buildHookTaskBatchTerminalMessage(hook, sourceTask, tasks, batchID, "已取消"))
			}
			return true, true, true, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusFailed, buildHookTaskBatchTerminalMessage(hook, sourceTask, tasks, batchID, "已取消"))
		case agentdomain.TaskStatusFailed:
			if hook.FailurePolicy == domain.TemplateHookFailurePolicyWarnOnly {
				return true, true, false, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusSuccess, "Hook 执行失败已忽略："+buildHookTaskBatchTerminalMessage(hook, sourceTask, tasks, batchID, "执行失败"))
			}
			return true, true, true, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusFailed, buildHookTaskBatchTerminalMessage(hook, sourceTask, tasks, batchID, "执行失败"))
		default:
			return false, false, false, nil
		}
	}

	taskID := parseHookTaskID(step.Message)
	if taskID == "" {
		failMessage := "Hook 任务缺少任务号，无法继续追踪"
		if hook.FailurePolicy == domain.TemplateHookFailurePolicyWarnOnly {
			return true, true, false, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusSuccess, "Hook 执行失败已忽略："+failMessage)
		}
		return true, true, true, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusFailed, failMessage)
	}
	task, err := uc.agentRepo.GetTaskByID(ctx, taskID)
	if err != nil {
		return false, false, false, err
	}

	progressMessage := buildHookTaskProgressMessage(hook, task, agentdomain.Instance{
		AgentCode: task.AgentCode,
		Name:      task.AgentCode,
	})
	switch task.Status {
	case agentdomain.TaskStatusPending, agentdomain.TaskStatusQueued, agentdomain.TaskStatusClaimed, agentdomain.TaskStatusRunning:
		if strings.TrimSpace(progressMessage) != strings.TrimSpace(step.Message) {
			return true, false, false, uc.markStep(ctx, order.ID, step.StepCode, domain.StepStatusRunning, progressMessage, step.StartedAt, nil)
		}
		return false, false, false, nil
	case agentdomain.TaskStatusSuccess:
		return true, true, false, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusSuccess, buildHookTaskTerminalMessage(hook, task, "执行成功"))
	case agentdomain.TaskStatusCancelled:
		if hook.FailurePolicy == domain.TemplateHookFailurePolicyWarnOnly {
			return true, true, false, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusSuccess, "Hook 执行已取消并忽略："+buildHookTaskTerminalMessage(hook, task, "已取消"))
		}
		return true, true, true, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusFailed, buildHookTaskTerminalMessage(hook, task, "已取消"))
	case agentdomain.TaskStatusFailed:
		if hook.FailurePolicy == domain.TemplateHookFailurePolicyWarnOnly {
			return true, true, false, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusSuccess, "Hook 执行失败已忽略："+buildHookTaskTerminalMessage(hook, task, "执行失败"))
		}
		return true, true, true, uc.markStepFinished(ctx, order.ID, step.StepCode, domain.StepStatusFailed, buildHookTaskTerminalMessage(hook, task, "执行失败"))
	default:
		return false, false, false, nil
	}
}

// resolveHookTargetAgent 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveHookTargetAgent(
	ctx context.Context,
	order domain.ReleaseOrder,
	sourceTask agentdomain.Task,
) (agentdomain.Instance, error) {
	if uc.agentRepo == nil {
		return agentdomain.Instance{}, fmt.Errorf("%w: agent repository is not configured", ErrInvalidInput)
	}
	if strings.TrimSpace(sourceTask.AgentID) != "" {
		instance, err := uc.agentRepo.GetInstanceByID(ctx, sourceTask.AgentID)
		if err == nil && instance.Status == agentdomain.StatusActive {
			return instance, nil
		}
	}

	items, _, err := uc.agentRepo.ListInstances(ctx, agentdomain.ListFilter{
		Status:   agentdomain.StatusActive,
		Page:     1,
		PageSize: 500,
	})
	if err != nil {
		return agentdomain.Instance{}, err
	}
	if len(items) == 0 {
		return agentdomain.Instance{}, fmt.Errorf("%w: no active agent instance", ErrInvalidInput)
	}

	targetEnv := strings.ToLower(strings.TrimSpace(order.EnvCode))
	sort.SliceStable(items, func(i, j int) bool {
		left := agentInstancePriority(items[i], targetEnv)
		right := agentInstancePriority(items[j], targetEnv)
		if left != right {
			return left < right
		}
		return strings.Compare(items[i].AgentCode, items[j].AgentCode) < 0
	})
	return items[0], nil
}

// agentInstancePriority 封装当前模块的业务处理逻辑。
func agentInstancePriority(item agentdomain.Instance, targetEnv string) int {
	priority := 30
	if strings.EqualFold(strings.TrimSpace(item.EnvironmentCode), targetEnv) {
		priority -= 20
	}
	switch deriveAgentRuntimeState(item) {
	case agentdomain.RuntimeStateOnline:
		priority -= 8
	case agentdomain.RuntimeStateBusy:
		priority -= 4
	}
	return priority
}

// buildHookTaskVariables 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildHookTaskVariables(
	ctx context.Context,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	hook domain.ReleaseTemplateHook,
	executeStage domain.TemplateHookExecuteStage,
) (map[string]string, error) {
	orderParams, err := uc.repo.ListParams(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	appKey := ""
	appRepoURL := ""
	artifactValues := map[string]string{}
	if uc.appRepo != nil && strings.TrimSpace(order.ApplicationID) != "" {
		if appItem, appErr := uc.appRepo.GetByID(ctx, order.ApplicationID); appErr == nil {
			appKey = strings.TrimSpace(appItem.Key)
			appRepoURL = strings.TrimSpace(appItem.RepoURL)
			artifactValues, _ = uc.resolveApplicationArtifactParamValues(ctx, appItem)
		}
	}

	keys := []string{
		"app_key",
		"app_name",
		"repo_url",
		"release_name",
		"project_name",
		"env",
		"env_code",
		"branch",
		"git_ref",
		"image_version",
		"image_tag",
		standardParamCIJob,
		standardParamCIBuild,
		standardParamGOSArtifactURL,
		standardParamGOSArtifactPath,
	}
	values := make(map[string]string, len(keys)+4)
	for _, key := range keys {
		if value := strings.TrimSpace(uc.resolveStandardFieldValue(order, orderParams, executions, appKey, appRepoURL, artifactValues, key)); value != "" {
			values[key] = value
		}
	}
	values["order_no"] = strings.TrimSpace(order.OrderNo)
	releaseDetailURL, err := uc.buildReleaseDetailURL(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	values["release_detail_url"] = releaseDetailURL
	values["operation_type"] = strings.TrimSpace(string(order.OperationType))
	values["source_order_no"] = strings.TrimSpace(order.SourceOrderNo)
	values["executor_user_id"] = strings.TrimSpace(firstNonEmpty(order.ExecutorUserID, order.CreatorUserID))
	values["executor_name"] = strings.TrimSpace(firstNonEmpty(order.ExecutorName, order.TriggeredBy))
	releaseStage := string(normalizeHookExecuteStage(executeStage))
	values["release_stage"] = releaseStage
	values["release_stage_rich"] = buildNotificationReleaseStageRichValue(releaseStage)
	if releaseStatus := deriveHookReleaseStatus(order, executions, executeStage); releaseStatus != "" {
		values["release_status"] = releaseStatus
		values["release_status_rich"] = buildNotificationReleaseStatusRichValue(releaseStatus)
	}
	for _, item := range orderParams {
		key := strings.ToLower(strings.TrimSpace(item.ParamKey))
		if key == "" {
			continue
		}
		if isCIOnlyStandardParamKey(key) && item.PipelineScope != domain.PipelineScopeCI {
			continue
		}
		if _, exists := values[key]; exists {
			continue
		}
		value := strings.TrimSpace(item.ParamValue)
		if value == "" {
			continue
		}
		values[key] = value
	}
	return values, nil
}

func (uc *ReleaseOrderManager) buildReleaseDetailURL(ctx context.Context, orderID string) (string, error) {
	if uc == nil || uc.systemManagementSettings == nil || strings.TrimSpace(orderID) == "" {
		return "", nil
	}
	currentSiteURL, err := uc.systemManagementSettings.LoadCurrentSiteURL(ctx)
	if err != nil {
		return "", err
	}
	currentSiteURL = strings.TrimSpace(currentSiteURL)
	if currentSiteURL == "" {
		return "", nil
	}
	parsed, err := url.Parse(currentSiteURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("%w: current site URL is invalid", ErrInvalidInput)
	}
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	detailURL, err := url.JoinPath(parsed.String(), "releases", strings.TrimSpace(orderID))
	if err != nil {
		return "", fmt.Errorf("%w: build release detail URL failed", ErrInvalidInput)
	}
	return detailURL, nil
}

// enforceNotificationCoreVariables 封装当前模块的业务处理逻辑。
func enforceNotificationCoreVariables(
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	executeStage domain.TemplateHookExecuteStage,
	values map[string]string,
) {
	if values == nil {
		return
	}
	releaseStage := strings.TrimSpace(string(normalizeHookExecuteStage(executeStage)))
	if releaseStage != "" {
		values["release_stage"] = releaseStage
	}
	if strings.TrimSpace(values["release_stage_rich"]) == "" && releaseStage != "" {
		values["release_stage_rich"] = buildNotificationReleaseStageRichValue(releaseStage)
	}
	releaseStatus := strings.TrimSpace(values["release_status"])
	if releaseStatus == "" {
		releaseStatus = deriveHookReleaseStatus(order, executions, executeStage)
		if strings.TrimSpace(releaseStatus) != "" {
			values["release_status"] = strings.TrimSpace(releaseStatus)
		}
	}
	if strings.TrimSpace(values["release_status_rich"]) == "" && strings.TrimSpace(releaseStatus) != "" {
		values["release_status_rich"] = buildNotificationReleaseStatusRichValue(releaseStatus)
	}
}

// containsUnresolvedNotificationCorePlaceholder 解析上下文数据，得到后续流程需要的结果。
func containsUnresolvedNotificationCorePlaceholder(text string) bool {
	return unresolvedNotificationCorePlaceholderPattern.MatchString(strings.TrimSpace(text))
}

// normalizeHookExecuteStage 标准化输入值，保证后续逻辑使用统一格式。
func normalizeHookExecuteStage(stage domain.TemplateHookExecuteStage) domain.TemplateHookExecuteStage {
	if stage == "" {
		return domain.TemplateHookExecuteStagePostRelease
	}
	return stage
}

// deriveHookReleaseStatus 封装当前模块的业务处理逻辑。
func deriveHookReleaseStatus(
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	executeStage domain.TemplateHookExecuteStage,
) string {
	mainStatus, _, mainDone := evaluateMainReleaseStatus(executions)
	if mainDone {
		return strings.TrimSpace(string(mainStatus))
	}

	if executeStage == domain.TemplateHookExecuteStageBuildComplete {
		ciExecution := findExecutionByScope(executions, domain.PipelineScopeCI)
		if ciExecution != nil {
			switch ciExecution.Status {
			case domain.ExecutionStatusSuccess:
				return strings.TrimSpace(string(domain.OrderStatusSuccess))
			case domain.ExecutionStatusFailed:
				return strings.TrimSpace(string(domain.OrderStatusFailed))
			case domain.ExecutionStatusCancelled:
				return strings.TrimSpace(string(domain.OrderStatusCancelled))
			case domain.ExecutionStatusRunning:
				return strings.TrimSpace(string(domain.OrderStatusBuilding))
			}
		}
	}

	status := strings.TrimSpace(string(order.Status))
	if status != "" {
		return status
	}
	return strings.TrimSpace(string(domain.OrderStatusRunning))
}

// buildNotificationReleaseStageRichValue 组装业务执行所需的输入数据。
func buildNotificationReleaseStageRichValue(stage string) string {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case string(domain.TemplateHookExecuteStageBuildComplete):
		return "🟠 构建完成"
	case string(domain.TemplateHookExecuteStagePostRelease):
		return "🔵 发布完成"
	default:
		stage = strings.TrimSpace(stage)
		if stage == "" {
			return ""
		}
		return "🟡 " + stage
	}
}

// buildNotificationReleaseStatusRichValue 组装业务执行所需的输入数据。
func buildNotificationReleaseStatusRichValue(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case string(domain.OrderStatusSuccess), string(domain.OrderStatusDeploySuccess):
		return "🟢 成功"
	case string(domain.OrderStatusFailed), string(domain.OrderStatusDeployFailed):
		return "🔴 失败"
	case string(domain.OrderStatusCancelled):
		return "⚪ 已取消"
	case string(domain.OrderStatusBuilding):
		return "🟠 构建中"
	case string(domain.OrderStatusBuiltWaitingDeploy):
		return "🟠 已构建待部署"
	case string(domain.OrderStatusDeploying):
		return "🔵 部署中"
	case string(domain.OrderStatusPending):
		return "🟡 待执行"
	case string(domain.OrderStatusRunning):
		return "🔵 执行中"
	case string(domain.OrderStatusPendingApproval):
		return "🟣 待审批"
	case string(domain.OrderStatusApproving):
		return "🟣 审批中"
	case string(domain.OrderStatusApproved):
		return "🟢 审批通过"
	case string(domain.OrderStatusRejected):
		return "🔴 审批拒绝"
	case string(domain.OrderStatusQueued):
		return "🟡 排队中"
	default:
		status = strings.TrimSpace(status)
		if status == "" {
			return ""
		}
		return "🟡 " + status
	}
}

// renderHookString 封装当前模块的业务处理逻辑。
func renderHookString(values map[string]string, template string) string {
	text := strings.TrimSpace(template)
	if text == "" {
		return ""
	}
	if len(values) == 0 {
		return text
	}
	replacerArgs := make([]string, 0, len(values)*2)
	for key, value := range values {
		replacerArgs = append(replacerArgs, "{"+key+"}", value)
	}
	return strings.NewReplacer(replacerArgs...).Replace(text)
}

// resolveHookTaskInitialStatus 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveHookTaskInitialStatus(ctx context.Context, agentID string) (agentdomain.TaskStatus, error) {
	if uc.agentRepo == nil || strings.TrimSpace(agentID) == "" {
		return agentdomain.TaskStatusPending, nil
	}
	items, _, err := uc.agentRepo.ListTasks(ctx, agentdomain.TaskListFilter{
		AgentID:  strings.TrimSpace(agentID),
		Statuses: []agentdomain.TaskStatus{agentdomain.TaskStatusPending, agentdomain.TaskStatusQueued, agentdomain.TaskStatusClaimed, agentdomain.TaskStatusRunning},
		Page:     1,
		PageSize: 500,
	})
	if err != nil {
		return agentdomain.TaskStatusPending, err
	}
	if len(items) > 0 {
		return agentdomain.TaskStatusQueued, nil
	}
	return agentdomain.TaskStatusPending, nil
}

// buildHookTaskProgressMessage 组装业务执行所需的输入数据。
func buildHookTaskProgressMessage(hook domain.ReleaseTemplateHook, task agentdomain.Task, agentInstance agentdomain.Instance) string {
	agentName := firstNonEmpty(strings.TrimSpace(agentInstance.Name), strings.TrimSpace(agentInstance.AgentCode), strings.TrimSpace(task.AgentCode), "未指定 Agent")
	taskName := firstNonEmpty(strings.TrimSpace(hook.TargetName), strings.TrimSpace(task.Name), strings.TrimSpace(task.ScriptName), strings.TrimSpace(task.ID))
	statusText := "待领取"
	switch task.Status {
	case agentdomain.TaskStatusQueued:
		statusText = "排队中"
	case agentdomain.TaskStatusClaimed:
		statusText = "已领取"
	case agentdomain.TaskStatusRunning:
		statusText = "执行中"
	}
	return fmt.Sprintf("任务：%s，目标 Agent：%s，当前状态：%s，task_id=%s", taskName, agentName, statusText, strings.TrimSpace(task.ID))
}

// buildHookTaskBatchProgressMessage 组装业务执行所需的输入数据。
func buildHookTaskBatchProgressMessage(hook domain.ReleaseTemplateHook, sourceTask agentdomain.Task, tasks []agentdomain.Task, batchID string) string {
	taskName := firstNonEmpty(strings.TrimSpace(hook.TargetName), strings.TrimSpace(sourceTask.Name), strings.TrimSpace(sourceTask.ScriptName), strings.TrimSpace(sourceTask.ID))
	summary := buildTaskBatchSummary(fmt.Sprintf("任务：%s", taskName), tasks)
	return fmt.Sprintf("%s，source_task_id=%s，batch_id=%s", summary, strings.TrimSpace(sourceTask.ID), strings.TrimSpace(batchID))
}

// buildHookTaskTerminalMessage 组装业务执行所需的输入数据。
func buildHookTaskTerminalMessage(hook domain.ReleaseTemplateHook, task agentdomain.Task, prefix string) string {
	taskName := firstNonEmpty(strings.TrimSpace(hook.TargetName), strings.TrimSpace(task.Name), strings.TrimSpace(task.ScriptName), strings.TrimSpace(task.ID))
	summary := firstNonEmpty(strings.TrimSpace(task.LastRunSummary), strings.TrimSpace(task.FailureReason))
	if summary == "" {
		return fmt.Sprintf("%s：%s，任务号：%s", prefix, taskName, strings.TrimSpace(task.ID))
	}
	return fmt.Sprintf("%s：%s，任务号：%s，摘要：%s", prefix, taskName, strings.TrimSpace(task.ID), summary)
}

// buildHookTaskBatchTerminalMessage 组装业务执行所需的输入数据。
func buildHookTaskBatchTerminalMessage(hook domain.ReleaseTemplateHook, sourceTask agentdomain.Task, tasks []agentdomain.Task, batchID string, prefix string) string {
	taskName := firstNonEmpty(strings.TrimSpace(hook.TargetName), strings.TrimSpace(sourceTask.Name), strings.TrimSpace(sourceTask.ScriptName), strings.TrimSpace(sourceTask.ID))
	summary := buildTaskBatchSummary("", tasks)
	if summary == "" {
		return fmt.Sprintf("%s：%s，source_task_id=%s，batch_id=%s", prefix, taskName, strings.TrimSpace(sourceTask.ID), strings.TrimSpace(batchID))
	}
	return fmt.Sprintf("%s：%s，%s，source_task_id=%s，batch_id=%s", prefix, taskName, summary, strings.TrimSpace(sourceTask.ID), strings.TrimSpace(batchID))
}

// parseHookTaskID 解析输入内容并返回结构化结果。
func parseHookTaskID(message string) string {
	trimmed := strings.TrimSpace(message)
	matches := hookTaskIDPattern.FindStringSubmatch(trimmed)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	matches = hookTaskLabelPattern.FindStringSubmatch(trimmed)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// parseHookTaskBatchIdentity 解析输入内容并返回结构化结果。
func parseHookTaskBatchIdentity(message string) (string, string) {
	sourceMatches := hookSourceTaskIDPattern.FindStringSubmatch(strings.TrimSpace(message))
	batchMatches := hookDispatchBatchIDPattern.FindStringSubmatch(strings.TrimSpace(message))
	if len(sourceMatches) < 2 || len(batchMatches) < 2 {
		return "", ""
	}
	return strings.TrimSpace(sourceMatches[1]), strings.TrimSpace(batchMatches[1])
}

// deriveAgentRuntimeState 封装当前模块的业务处理逻辑。
func deriveAgentRuntimeState(item agentdomain.Instance) agentdomain.RuntimeState {
	if item.Status == agentdomain.StatusDisabled {
		return agentdomain.RuntimeStateDisabled
	}
	if item.Status == agentdomain.StatusMaintenance {
		return agentdomain.RuntimeStateMaintenance
	}
	if item.CurrentTaskID != "" {
		return agentdomain.RuntimeStateBusy
	}
	if item.LastHeartbeatAt.IsZero() {
		return agentdomain.RuntimeStateOffline
	}
	if time.Since(item.LastHeartbeatAt.UTC()) > 2*time.Minute {
		return agentdomain.RuntimeStateOffline
	}
	return agentdomain.RuntimeStateOnline
}
