package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	agentdomain "gos/internal/domain/agent"
	domain "gos/internal/domain/release"
	"gos/internal/support/logx"
)

const releaseOrderAgentTaskLogDisplayLimit = 12000

type releaseOrderStepAgentTaskCache struct {
	taskByID        map[string]agentdomain.Task
	tasksByBatchKey map[string][]agentdomain.Task
}

// enrichAgentTaskStepDetails 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) enrichAgentTaskStepDetails(ctx context.Context, steps []domain.ReleaseOrderStep) []domain.ReleaseOrderStep {
	if len(steps) == 0 || uc.agentRepo == nil {
		return steps
	}
	result := make([]domain.ReleaseOrderStep, len(steps))
	copy(result, steps)
	cache := releaseOrderStepAgentTaskCache{
		taskByID:        make(map[string]agentdomain.Task),
		tasksByBatchKey: make(map[string][]agentdomain.Task),
	}
	for idx := range result {
		enriched, err := uc.enrichSingleAgentTaskStep(ctx, result[idx], &cache)
		if err != nil {
			logx.Error("release_order", "step_agent_log_enrich_failed", err,
				logx.F("release_order_id", result[idx].ReleaseOrderID),
				logx.F("step_id", result[idx].ID),
				logx.F("step_code", result[idx].StepCode),
			)
			continue
		}
		result[idx] = enriched
	}
	return result
}

// enrichSingleAgentTaskStep 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) enrichSingleAgentTaskStep(
	ctx context.Context,
	step domain.ReleaseOrderStep,
	cache *releaseOrderStepAgentTaskCache,
) (domain.ReleaseOrderStep, error) {
	sourceTaskID, batchID := parseHookTaskBatchIdentity(step.Message)
	if sourceTaskID != "" && batchID != "" {
		tasks, err := uc.loadAgentTaskBatchForStep(ctx, sourceTaskID, batchID, cache)
		if err != nil {
			return step, err
		}
		if len(tasks) == 0 {
			return step, nil
		}
		step.RelatedTaskIDs = collectAgentTaskIDs(tasks)
		step.RelatedTaskCount = len(tasks)
		step.RelatedTaskSummary = buildTaskBatchSummary("", tasks)
		step.DetailLog = buildAgentTaskBatchDetailLog(tasks, sourceTaskID, batchID)
		return step, nil
	}

	taskID := parseHookTaskID(step.Message)
	if taskID == "" {
		return step, nil
	}
	task, err := uc.loadAgentTaskForStep(ctx, taskID, cache)
	if err != nil {
		if errors.Is(err, agentdomain.ErrTaskNotFound) {
			return step, nil
		}
		return step, err
	}
	step.RelatedTaskIDs = []string{task.ID}
	step.RelatedTaskCount = 1
	step.RelatedTaskSummary = buildSingleAgentTaskSummary(task)
	step.DetailLog = buildSingleAgentTaskDetailLog(task)
	return step, nil
}

// loadAgentTaskForStep 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) loadAgentTaskForStep(
	ctx context.Context,
	taskID string,
	cache *releaseOrderStepAgentTaskCache,
) (agentdomain.Task, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return agentdomain.Task{}, agentdomain.ErrTaskNotFound
	}
	if item, ok := cache.taskByID[taskID]; ok {
		return item, nil
	}
	item, err := uc.agentRepo.GetTaskByID(ctx, taskID)
	if err != nil {
		return agentdomain.Task{}, err
	}
	cache.taskByID[taskID] = item
	return item, nil
}

// loadAgentTaskBatchForStep 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) loadAgentTaskBatchForStep(
	ctx context.Context,
	sourceTaskID string,
	batchID string,
	cache *releaseOrderStepAgentTaskCache,
) ([]agentdomain.Task, error) {
	key := strings.TrimSpace(sourceTaskID) + "::" + strings.TrimSpace(batchID)
	if items, ok := cache.tasksByBatchKey[key]; ok {
		return items, nil
	}
	items, _, err := uc.agentRepo.ListTasks(ctx, agentdomain.TaskListFilter{
		SourceTaskID:    strings.TrimSpace(sourceTaskID),
		DispatchBatchID: strings.TrimSpace(batchID),
		Page:            1,
		PageSize:        500,
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		left := firstNonEmpty(strings.TrimSpace(items[i].AgentCode), strings.TrimSpace(items[i].Name), strings.TrimSpace(items[i].ID))
		right := firstNonEmpty(strings.TrimSpace(items[j].AgentCode), strings.TrimSpace(items[j].Name), strings.TrimSpace(items[j].ID))
		if left != right {
			return strings.Compare(left, right) < 0
		}
		return strings.Compare(strings.TrimSpace(items[i].ID), strings.TrimSpace(items[j].ID)) < 0
	})
	for _, item := range items {
		cache.taskByID[item.ID] = item
	}
	cache.tasksByBatchKey[key] = items
	return items, nil
}

// collectAgentTaskIDs 封装当前模块的业务处理逻辑。
func collectAgentTaskIDs(tasks []agentdomain.Task) []string {
	if len(tasks) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(tasks))
	for _, item := range tasks {
		if value := strings.TrimSpace(item.ID); value != "" {
			result = append(result, value)
		}
	}
	return result
}

// buildSingleAgentTaskSummary 组装业务执行所需的输入数据。
func buildSingleAgentTaskSummary(task agentdomain.Task) string {
	parts := []string{
		fmt.Sprintf("目标 Agent：%s", firstNonEmpty(strings.TrimSpace(task.AgentCode), "未分配")),
		fmt.Sprintf("状态：%s", taskBatchStatusText(task.Status)),
	}
	if summary := firstNonEmpty(strings.TrimSpace(task.LastRunSummary), strings.TrimSpace(task.FailureReason)); summary != "" {
		parts = append(parts, fmt.Sprintf("摘要：%s", summary))
	}
	return strings.Join(parts, "，")
}

// buildSingleAgentTaskDetailLog 组装业务执行所需的输入数据。
func buildSingleAgentTaskDetailLog(task agentdomain.Task) string {
	sections := []string{
		fmt.Sprintf("任务号：%s", strings.TrimSpace(task.ID)),
		fmt.Sprintf("目标 Agent：%s", firstNonEmpty(strings.TrimSpace(task.AgentCode), "未分配")),
		fmt.Sprintf("当前状态：%s", taskBatchStatusText(task.Status)),
	}
	if summary := firstNonEmpty(strings.TrimSpace(task.LastRunSummary), strings.TrimSpace(task.FailureReason)); summary != "" {
		sections = append(sections, fmt.Sprintf("执行摘要：%s", summary))
	}
	if started := formatAgentTaskDisplayTime(task.StartedAt); started != "" {
		sections = append(sections, fmt.Sprintf("开始时间：%s", started))
	}
	if finished := formatAgentTaskDisplayTime(task.FinishedAt); finished != "" {
		sections = append(sections, fmt.Sprintf("结束时间：%s", finished))
	}
	if stdout := formatAgentTaskStreamSection("标准输出", task.StdoutText); stdout != "" {
		sections = append(sections, stdout)
	}
	if stderr := formatAgentTaskStreamSection("标准错误", task.StderrText); stderr != "" {
		sections = append(sections, stderr)
	}
	return truncateAgentTaskDetailLog(strings.Join(sections, "\n\n"))
}

// buildAgentTaskBatchDetailLog 组装业务执行所需的输入数据。
func buildAgentTaskBatchDetailLog(tasks []agentdomain.Task, sourceTaskID string, batchID string) string {
	sections := []string{buildTaskBatchSummary("", tasks)}
	if strings.TrimSpace(sourceTaskID) != "" {
		sections = append(sections, fmt.Sprintf("来源任务：%s", strings.TrimSpace(sourceTaskID)))
	}
	if strings.TrimSpace(batchID) != "" {
		sections = append(sections, fmt.Sprintf("批次号：%s", strings.TrimSpace(batchID)))
	}
	for index, item := range tasks {
		entryParts := []string{
			fmt.Sprintf("[%d] Agent：%s", index+1, firstNonEmpty(strings.TrimSpace(item.AgentCode), "未分配")),
			fmt.Sprintf("任务号：%s", strings.TrimSpace(item.ID)),
			fmt.Sprintf("状态：%s", taskBatchStatusText(item.Status)),
		}
		if summary := firstNonEmpty(strings.TrimSpace(item.LastRunSummary), strings.TrimSpace(item.FailureReason)); summary != "" {
			entryParts = append(entryParts, fmt.Sprintf("摘要：%s", summary))
		}
		if started := formatAgentTaskDisplayTime(item.StartedAt); started != "" {
			entryParts = append(entryParts, fmt.Sprintf("开始：%s", started))
		}
		if finished := formatAgentTaskDisplayTime(item.FinishedAt); finished != "" {
			entryParts = append(entryParts, fmt.Sprintf("结束：%s", finished))
		}
		if stdout := formatAgentTaskStreamSection("标准输出", item.StdoutText); stdout != "" {
			entryParts = append(entryParts, stdout)
		}
		if stderr := formatAgentTaskStreamSection("标准错误", item.StderrText); stderr != "" {
			entryParts = append(entryParts, stderr)
		}
		sections = append(sections, strings.Join(entryParts, "\n"))
	}
	return truncateAgentTaskDetailLog(strings.Join(sections, "\n\n"))
}

// formatAgentTaskDisplayTime 封装当前模块的业务处理逻辑。
func formatAgentTaskDisplayTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

// formatAgentTaskStreamSection 封装当前模块的业务处理逻辑。
func formatAgentTaskStreamSection(title string, text string) string {
	content := strings.TrimSpace(text)
	if content == "" {
		return ""
	}
	return title + "\n" + tailTruncateText(content, 4000)
}

// truncateAgentTaskDetailLog 封装当前模块的业务处理逻辑。
func truncateAgentTaskDetailLog(text string) string {
	content := strings.TrimSpace(text)
	if content == "" {
		return ""
	}
	return tailTruncateText(content, releaseOrderAgentTaskLogDisplayLimit)
}

// tailTruncateText 封装当前模块的业务处理逻辑。
func tailTruncateText(text string, limit int) string {
	content := strings.TrimSpace(text)
	if limit <= 0 || len(content) <= limit {
		return content
	}
	start := len(content) - limit
	return fmt.Sprintf("[日志较长，已截断，仅展示最后 %d 字符]\n%s", limit, content[start:])
}
