package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	agentdomain "gos/internal/domain/agent"
)

var activeDispatchTaskStatuses = []agentdomain.TaskStatus{
	agentdomain.TaskStatusPending,
	agentdomain.TaskStatusQueued,
	agentdomain.TaskStatusClaimed,
	agentdomain.TaskStatusRunning,
}

// normalizeTaskTargetAgentIDs 标准化输入值，保证后续逻辑使用统一格式。
func normalizeTaskTargetAgentIDs(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
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

// resolveTaskDispatchTargets 解析上下文数据，得到后续流程需要的结果。
func resolveTaskDispatchTargets(ctx context.Context, repo agentdomain.Repository, sourceTask agentdomain.Task) ([]agentdomain.Instance, error) {
	targetAgentIDs := normalizeTaskTargetAgentIDs(sourceTask.TargetAgentIDs)
	if len(targetAgentIDs) == 0 {
		agentID := strings.TrimSpace(sourceTask.AgentID)
		if agentID == "" {
			return nil, fmt.Errorf("%w: task has no bound target agents", ErrInvalidInput)
		}
		targetAgentIDs = []string{agentID}
	}

	targets := make([]agentdomain.Instance, 0, len(targetAgentIDs))
	for _, agentID := range targetAgentIDs {
		item, err := repo.GetInstanceByID(ctx, agentID)
		if err != nil {
			return nil, err
		}
		if item.Status != agentdomain.StatusActive {
			return nil, fmt.Errorf("%w: target agent %s is not active", ErrInvalidInput, firstNonEmptyAgentString(strings.TrimSpace(item.Name), strings.TrimSpace(item.AgentCode), agentID))
		}
		targets = append(targets, item)
	}
	sort.SliceStable(targets, func(i, j int) bool {
		left := firstNonEmptyAgentString(strings.TrimSpace(targets[i].Name), strings.TrimSpace(targets[i].AgentCode), strings.TrimSpace(targets[i].ID))
		right := firstNonEmptyAgentString(strings.TrimSpace(targets[j].Name), strings.TrimSpace(targets[j].AgentCode), strings.TrimSpace(targets[j].ID))
		return strings.Compare(left, right) < 0
	})
	return targets, nil
}

// resolveDispatchTaskInitialStatus 解析上下文数据，得到后续流程需要的结果。
func resolveDispatchTaskInitialStatus(ctx context.Context, repo agentdomain.Repository, agentID, excludeTaskID string) (agentdomain.TaskStatus, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return agentdomain.TaskStatusPending, nil
	}
	items, _, err := repo.ListTasks(ctx, agentdomain.TaskListFilter{
		AgentID:  agentID,
		Statuses: activeDispatchTaskStatuses,
		Page:     1,
		PageSize: 500,
	})
	if err != nil {
		return agentdomain.TaskStatusPending, err
	}
	excludeTaskID = strings.TrimSpace(excludeTaskID)
	for _, item := range items {
		if excludeTaskID != "" && item.ID == excludeTaskID {
			continue
		}
		return agentdomain.TaskStatusQueued, nil
	}
	return agentdomain.TaskStatusPending, nil
}

// dispatchTemporaryTaskBatch 封装当前模块的业务处理逻辑。
func dispatchTemporaryTaskBatch(
	ctx context.Context,
	repo agentdomain.Repository,
	sourceTask agentdomain.Task,
	targets []agentdomain.Instance,
	name string,
	variables map[string]string,
	createdBy string,
	batchID string,
	now func() time.Time,
) ([]agentdomain.Task, error) {
	createdAt := now()
	normalizedVariables := normalizeTaskVariables(variables)
	initialStatuses := make(map[string]agentdomain.TaskStatus, len(targets))
	for _, item := range targets {
		status, err := resolveDispatchTaskInitialStatus(ctx, repo, item.ID, "")
		if err != nil {
			return nil, err
		}
		initialStatuses[item.ID] = status
	}

	result := make([]agentdomain.Task, 0, len(targets))
	for _, item := range targets {
		workDir := firstNonEmptyAgentString(strings.TrimSpace(sourceTask.WorkDir), strings.TrimSpace(item.WorkDir))
		created, err := repo.CreateTask(ctx, agentdomain.Task{
			ID:              generateID("agtask"),
			AgentID:         strings.TrimSpace(item.ID),
			AgentCode:       strings.TrimSpace(item.AgentCode),
			TargetAgentIDs:  nil,
			SourceTaskID:    strings.TrimSpace(sourceTask.ID),
			DispatchBatchID: strings.TrimSpace(batchID),
			Name:            strings.TrimSpace(name),
			TaskMode:        agentdomain.TaskModeTemporary,
			TaskType:        sourceTask.TaskType,
			ShellType:       sourceTask.ShellType,
			WorkDir:         workDir,
			ScriptID:        sourceTask.ScriptID,
			ScriptName:      sourceTask.ScriptName,
			ScriptPath:      sourceTask.ScriptPath,
			ScriptText:      sourceTask.ScriptText,
			Variables:       normalizedVariables,
			TimeoutSec:      sourceTask.TimeoutSec,
			Status:          initialStatuses[item.ID],
			CreatedBy:       strings.TrimSpace(createdBy),
			CreatedAt:       createdAt,
			UpdatedAt:       createdAt,
			StdoutText:      "",
			StderrText:      "",
			FailureReason:   "",
		})
		if err != nil {
			return nil, err
		}
		result = append(result, created)
	}
	return result, nil
}

// aggregateTaskBatchStatus 封装当前模块的业务处理逻辑。
func aggregateTaskBatchStatus(tasks []agentdomain.Task) agentdomain.TaskStatus {
	hasPending := false
	hasQueued := false
	hasClaimed := false
	hasRunning := false
	hasFailed := false
	hasCancelled := false
	allSuccess := len(tasks) > 0
	for _, item := range tasks {
		switch item.Status {
		case agentdomain.TaskStatusPending:
			hasPending = true
			allSuccess = false
		case agentdomain.TaskStatusQueued:
			hasQueued = true
			allSuccess = false
		case agentdomain.TaskStatusClaimed:
			hasClaimed = true
			allSuccess = false
		case agentdomain.TaskStatusRunning:
			hasRunning = true
			allSuccess = false
		case agentdomain.TaskStatusFailed:
			hasFailed = true
			allSuccess = false
		case agentdomain.TaskStatusCancelled:
			hasCancelled = true
			allSuccess = false
		case agentdomain.TaskStatusSuccess:
		default:
			allSuccess = false
		}
	}
	switch {
	case hasRunning:
		return agentdomain.TaskStatusRunning
	case hasClaimed:
		return agentdomain.TaskStatusClaimed
	case hasQueued:
		return agentdomain.TaskStatusQueued
	case hasPending:
		return agentdomain.TaskStatusPending
	case hasFailed:
		return agentdomain.TaskStatusFailed
	case hasCancelled:
		return agentdomain.TaskStatusCancelled
	case allSuccess:
		return agentdomain.TaskStatusSuccess
	default:
		return agentdomain.TaskStatusDraft
	}
}

// taskBatchStatusText 封装当前模块的业务处理逻辑。
func taskBatchStatusText(status agentdomain.TaskStatus) string {
	switch status {
	case agentdomain.TaskStatusPending:
		return "待领取"
	case agentdomain.TaskStatusQueued:
		return "排队中"
	case agentdomain.TaskStatusClaimed:
		return "已领取"
	case agentdomain.TaskStatusRunning:
		return "执行中"
	case agentdomain.TaskStatusSuccess:
		return "执行成功"
	case agentdomain.TaskStatusFailed:
		return "执行失败"
	case agentdomain.TaskStatusCancelled:
		return "已取消"
	default:
		return "待执行"
	}
}

// buildTaskBatchSummary 组装业务执行所需的输入数据。
func buildTaskBatchSummary(prefix string, tasks []agentdomain.Task) string {
	if len(tasks) == 0 {
		return strings.TrimSpace(prefix)
	}
	counts := map[agentdomain.TaskStatus]int{}
	for _, item := range tasks {
		counts[item.Status]++
	}
	parts := make([]string, 0, 6)
	for _, status := range []agentdomain.TaskStatus{
		agentdomain.TaskStatusSuccess,
		agentdomain.TaskStatusRunning,
		agentdomain.TaskStatusClaimed,
		agentdomain.TaskStatusQueued,
		agentdomain.TaskStatusPending,
		agentdomain.TaskStatusFailed,
		agentdomain.TaskStatusCancelled,
	} {
		if counts[status] == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %d", taskBatchStatusText(status), counts[status]))
	}
	summary := fmt.Sprintf("共 %d 台 Agent", len(tasks))
	if len(parts) > 0 {
		summary += "（" + strings.Join(parts, " / ") + "）"
	}
	if strings.TrimSpace(prefix) == "" {
		return summary
	}
	return strings.TrimSpace(prefix) + "，" + summary
}
