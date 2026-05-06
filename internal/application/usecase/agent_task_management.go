package usecase

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	agentdomain "gos/internal/domain/agent"
)

type AgentTaskManager struct {
	repo agentdomain.Repository
	now  func() time.Time
}

type AgentTaskOutput struct {
	ID              string                 `json:"id"`
	AgentID         string                 `json:"agent_id"`
	AgentCode       string                 `json:"agent_code"`
	TargetAgentIDs  []string               `json:"target_agent_ids"`
	SourceTaskID    string                 `json:"source_task_id"`
	DispatchBatchID string                 `json:"dispatch_batch_id"`
	Name            string                 `json:"name"`
	TaskMode        string                 `json:"task_mode"`
	TaskType        string                 `json:"task_type"`
	ShellType       string                 `json:"shell_type"`
	WorkDir         string                 `json:"work_dir"`
	ScriptID        string                 `json:"script_id"`
	ScriptName      string                 `json:"script_name"`
	ScriptPath      string                 `json:"script_path"`
	ScriptText      string                 `json:"script_text"`
	Variables       map[string]string      `json:"variables"`
	TimeoutSec      int                    `json:"timeout_sec"`
	Status          agentdomain.TaskStatus `json:"status"`
	ClaimedAt       *time.Time             `json:"claimed_at,omitempty"`
	StartedAt       *time.Time             `json:"started_at,omitempty"`
	FinishedAt      *time.Time             `json:"finished_at,omitempty"`
	ExitCode        int                    `json:"exit_code"`
	StdoutText      string                 `json:"stdout_text"`
	StderrText      string                 `json:"stderr_text"`
	FailureReason   string                 `json:"failure_reason"`
	RunCount        int                    `json:"run_count"`
	SuccessCount    int                    `json:"success_count"`
	FailureCount    int                    `json:"failure_count"`
	LastRunStatus   string                 `json:"last_run_status"`
	LastRunSummary  string                 `json:"last_run_summary"`
	CreatedBy       string                 `json:"created_by"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

type AgentTaskListOutput struct {
	Items []AgentTaskOutput
	Total int64
}

type CreateAgentTaskInput struct {
	AgentID        string
	TargetAgentIDs []string
	Name           string
	TaskMode       string
	TaskType       string
	ShellType      string
	WorkDir        string
	ScriptID       string
	ScriptPath     string
	ScriptText     string
	Variables      map[string]string
	TimeoutSec     int
	CreatedBy      string
}

type UpdateAgentTaskInput struct {
	AgentID        string
	TargetAgentIDs []string
	Name           string
	TaskMode       string
	WorkDir        string
	ScriptID       string
	Variables      map[string]string
	TimeoutSec     int
}

type StopAgentTaskInput struct {
	AgentID string
}

type ResumeAgentTaskInput struct {
	AgentID string
}

type ExecuteAgentTaskInput struct {
	AgentID string
}

type AgentTaskPollInput struct {
	AgentCode string
	Token     string
}

type FinishAgentTaskInput struct {
	AgentCode     string
	Token         string
	TaskID        string
	Status        agentdomain.TaskStatus
	ExitCode      int
	StdoutText    string
	StderrText    string
	FailureReason string
}

// NewAgentTaskManager 创建并返回对应组件实例。
func NewAgentTaskManager(repo agentdomain.Repository) *AgentTaskManager {
	return &AgentTaskManager{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

// Create 创建业务资源并返回处理结果。
func (uc *AgentTaskManager) Create(ctx context.Context, input CreateAgentTaskInput) (AgentTaskOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentTaskOutput{}, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	agentID := strings.TrimSpace(input.AgentID)
	var instance agentdomain.Instance
	var err error
	if agentID != "" {
		instance, err = uc.repo.GetInstanceByID(ctx, agentID)
		if err != nil {
			return AgentTaskOutput{}, err
		}
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return AgentTaskOutput{}, fmt.Errorf("%w: task name is required", ErrInvalidInput)
	}
	taskMode := agentdomain.TaskMode(firstNonEmptyAgentString(strings.TrimSpace(input.TaskMode), string(agentdomain.TaskModeTemporary)))
	if !taskMode.Valid() {
		return AgentTaskOutput{}, fmt.Errorf("%w: unsupported task_mode", ErrInvalidInput)
	}
	script, err := uc.resolveTaskScript(ctx, input.ScriptID, input.TaskType, input.ShellType, input.ScriptPath, input.ScriptText)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	timeoutSec := input.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	if timeoutSec > 3600 {
		timeoutSec = 3600
	}
	workDir := strings.TrimSpace(input.WorkDir)
	if workDir == "" && instance.ID != "" {
		workDir = instance.WorkDir
	}
	now := uc.now()
	initialStatus := agentdomain.TaskStatusPending
	targetAgentIDs := normalizeTaskTargetAgentIDs(input.TargetAgentIDs)
	if taskMode == agentdomain.TaskModeTemporary {
		initialStatus = agentdomain.TaskStatusDraft
		if len(targetAgentIDs) > 0 {
			instance = agentdomain.Instance{}
		}
	} else if instance.ID != "" {
		initialStatus, err = uc.resolveNewTaskStatus(ctx, instance.ID, "")
		if err != nil {
			return AgentTaskOutput{}, err
		}
		targetAgentIDs = nil
	}
	item := agentdomain.Task{
		ID:             generateID("agtask"),
		AgentID:        strings.TrimSpace(instance.ID),
		AgentCode:      strings.TrimSpace(instance.AgentCode),
		TargetAgentIDs: targetAgentIDs,
		Name:           name,
		TaskMode:       taskMode,
		TaskType:       script.TaskType,
		ShellType:      script.ShellType,
		WorkDir:        workDir,
		ScriptID:       script.ScriptID,
		ScriptName:     script.ScriptName,
		ScriptPath:     script.ScriptPath,
		ScriptText:     script.ScriptText,
		Variables:      normalizeTaskVariables(input.Variables),
		TimeoutSec:     timeoutSec,
		Status:         initialStatus,
		CreatedBy:      strings.TrimSpace(input.CreatedBy),
		CreatedAt:      now,
		UpdatedAt:      now,
		StdoutText:     "",
		StderrText:     "",
		FailureReason:  "",
	}
	created, err := uc.repo.CreateTask(ctx, item)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	return toAgentTaskOutput(created), nil
}

// List 查询并返回列表数据。
func (uc *AgentTaskManager) List(ctx context.Context, page, pageSize int) (AgentTaskListOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentTaskListOutput{}, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	items, total, err := uc.repo.ListTasks(ctx, agentdomain.TaskListFilter{
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return AgentTaskListOutput{}, err
	}
	items, err = syncManagedScriptSnapshotsForTasks(ctx, uc.repo, items, nil)
	if err != nil {
		return AgentTaskListOutput{}, err
	}
	outputs := make([]AgentTaskOutput, 0, len(items))
	for _, item := range items {
		outputs = append(outputs, toAgentTaskOutput(item))
	}
	return AgentTaskListOutput{Items: outputs, Total: total}, nil
}

// Update 更新业务资源并返回处理结果。
func (uc *AgentTaskManager) Update(ctx context.Context, taskID string, input UpdateAgentTaskInput) (AgentTaskOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentTaskOutput{}, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	current, err := uc.repo.GetTaskByID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return AgentTaskOutput{}, err
	}
	if strings.TrimSpace(input.AgentID) != "" && current.AgentID != strings.TrimSpace(input.AgentID) {
		return AgentTaskOutput{}, agentdomain.ErrTaskNotFound
	}
	if current.TaskMode != agentdomain.TaskModeResident {
		return AgentTaskOutput{}, fmt.Errorf("%w: only resident task can be edited", ErrInvalidInput)
	}
	if current.Status == agentdomain.TaskStatusRunning || current.Status == agentdomain.TaskStatusClaimed {
		return AgentTaskOutput{}, fmt.Errorf("%w: resident task is executing, please edit later", ErrInvalidInput)
	}
	script, err := uc.resolveTaskScript(ctx, input.ScriptID, current.TaskType, current.ShellType, current.ScriptPath, current.ScriptText)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = current.Name
	}
	workDir := strings.TrimSpace(input.WorkDir)
	if workDir == "" {
		workDir = current.WorkDir
	}
	timeoutSec := input.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = current.TimeoutSec
	}
	if timeoutSec > 3600 {
		timeoutSec = 3600
	}
	current.Name = name
	current.TaskMode = agentdomain.TaskModeResident
	current.WorkDir = workDir
	current.TaskType = script.TaskType
	current.ShellType = script.ShellType
	current.ScriptID = script.ScriptID
	current.ScriptName = script.ScriptName
	current.ScriptPath = script.ScriptPath
	current.ScriptText = script.ScriptText
	current.Variables = normalizeTaskVariables(input.Variables)
	current.TimeoutSec = timeoutSec
	current.UpdatedAt = uc.now()
	updated, err := uc.repo.UpdateTask(ctx, current)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	return toAgentTaskOutput(updated), nil
}

// ListByAgent 查询并返回列表数据。
func (uc *AgentTaskManager) ListByAgent(ctx context.Context, agentID string, page, pageSize int) (AgentTaskListOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentTaskListOutput{}, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	items, total, err := uc.repo.ListTasks(ctx, agentdomain.TaskListFilter{
		AgentID:  strings.TrimSpace(agentID),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return AgentTaskListOutput{}, err
	}
	items, err = syncManagedScriptSnapshotsForTasks(ctx, uc.repo, items, nil)
	if err != nil {
		return AgentTaskListOutput{}, err
	}
	outputs := make([]AgentTaskOutput, 0, len(items))
	for _, item := range items {
		outputs = append(outputs, toAgentTaskOutput(item))
	}
	return AgentTaskListOutput{Items: outputs, Total: total}, nil
}

// Get 查询并返回指定资源数据。
func (uc *AgentTaskManager) Get(ctx context.Context, taskID string) (AgentTaskOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentTaskOutput{}, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	item, err := uc.repo.GetTaskByID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return AgentTaskOutput{}, err
	}
	item, err = syncManagedScriptSnapshotForTask(ctx, uc.repo, item, nil)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	return toAgentTaskOutput(item), nil
}

// Stop 封装当前模块的业务处理逻辑。
func (uc *AgentTaskManager) Stop(ctx context.Context, taskID string, input StopAgentTaskInput) (AgentTaskOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentTaskOutput{}, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	current, err := uc.repo.GetTaskByID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return AgentTaskOutput{}, err
	}
	if strings.TrimSpace(input.AgentID) != "" && current.AgentID != strings.TrimSpace(input.AgentID) {
		return AgentTaskOutput{}, agentdomain.ErrTaskNotFound
	}
	if current.TaskMode != agentdomain.TaskModeResident {
		return AgentTaskOutput{}, fmt.Errorf("%w: only resident task can be stopped", ErrInvalidInput)
	}
	if current.Status == agentdomain.TaskStatusCancelled {
		return toAgentTaskOutput(current), nil
	}
	updated, err := uc.repo.CancelTask(ctx, current.ID, uc.now(), "已手动停止常驻任务")
	if err != nil {
		return AgentTaskOutput{}, err
	}
	return toAgentTaskOutput(updated), nil
}

// Resume 封装当前模块的业务处理逻辑。
func (uc *AgentTaskManager) Resume(ctx context.Context, taskID string, input ResumeAgentTaskInput) (AgentTaskOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentTaskOutput{}, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	current, err := uc.repo.GetTaskByID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return AgentTaskOutput{}, err
	}
	if strings.TrimSpace(input.AgentID) != "" && current.AgentID != strings.TrimSpace(input.AgentID) {
		return AgentTaskOutput{}, agentdomain.ErrTaskNotFound
	}
	if current.TaskMode != agentdomain.TaskModeResident {
		return AgentTaskOutput{}, fmt.Errorf("%w: only resident task can be resumed", ErrInvalidInput)
	}
	if current.Status != agentdomain.TaskStatusCancelled {
		return AgentTaskOutput{}, fmt.Errorf("%w: only stopped resident task can be resumed", ErrInvalidInput)
	}
	nextStatus, err := uc.resolveNewTaskStatus(ctx, current.AgentID, current.ID)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	summary := "已重新启用常驻任务"
	if nextStatus == agentdomain.TaskStatusQueued {
		summary = "已重新启用常驻任务，当前存在已分配任务，已进入排队"
	}
	updated, err := uc.repo.ResumeTask(ctx, current.ID, nextStatus, uc.now(), summary)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	return toAgentTaskOutput(updated), nil
}

// Execute 封装当前模块的业务处理逻辑。
func (uc *AgentTaskManager) Execute(ctx context.Context, taskID string, input ExecuteAgentTaskInput) (AgentTaskOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentTaskOutput{}, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	current, err := uc.repo.GetTaskByID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return AgentTaskOutput{}, err
	}
	current, err = syncManagedScriptSnapshotForTask(ctx, uc.repo, current, nil)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	if strings.TrimSpace(input.AgentID) != "" && current.AgentID != strings.TrimSpace(input.AgentID) {
		return AgentTaskOutput{}, agentdomain.ErrTaskNotFound
	}
	if current.TaskMode != agentdomain.TaskModeTemporary {
		return AgentTaskOutput{}, fmt.Errorf("%w: only temporary task can be executed manually", ErrInvalidInput)
	}
	if len(current.TargetAgentIDs) > 0 {
		updated, err := uc.executeBoundTemporaryTask(ctx, current)
		if err != nil {
			return AgentTaskOutput{}, err
		}
		return toAgentTaskOutput(updated), nil
	}
	if strings.TrimSpace(current.AgentID) == "" {
		return AgentTaskOutput{}, fmt.Errorf("%w: task is not assigned to any agent", ErrInvalidInput)
	}
	instance, err := uc.repo.GetInstanceByID(ctx, current.AgentID)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	if instance.Status != agentdomain.StatusActive {
		return AgentTaskOutput{}, fmt.Errorf("%w: target agent is not active", ErrInvalidInput)
	}
	nextStatus, err := uc.resolveNewTaskStatus(ctx, current.AgentID, current.ID)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	updated, err := uc.repo.ActivateTemporaryTask(ctx, current.ID, nextStatus, uc.now())
	if err != nil {
		if errors.Is(err, agentdomain.ErrTaskNotClaimable) {
			return AgentTaskOutput{}, fmt.Errorf("%w: task is already queued or executing", ErrInvalidInput)
		}
		return AgentTaskOutput{}, err
	}
	return toAgentTaskOutput(updated), nil
}

// Delete 删除业务资源并返回处理结果。
func (uc *AgentTaskManager) Delete(ctx context.Context, taskID string, input StopAgentTaskInput) error {
	if uc == nil || uc.repo == nil {
		return fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	current, err := uc.repo.GetTaskByID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.AgentID) != "" && current.AgentID != strings.TrimSpace(input.AgentID) {
		return agentdomain.ErrTaskNotFound
	}
	if current.TaskMode != agentdomain.TaskModeResident {
		return fmt.Errorf("%w: only resident task can be deleted", ErrInvalidInput)
	}
	if current.Status == agentdomain.TaskStatusRunning || current.Status == agentdomain.TaskStatusClaimed {
		return fmt.Errorf("%w: resident task is executing, please stop it first", ErrInvalidInput)
	}
	return uc.repo.DeleteTask(ctx, current.ID)
}

// UpdateTemporaryTask 更新业务资源并返回处理结果。
func (uc *AgentTaskManager) UpdateTemporaryTask(ctx context.Context, taskID string, input UpdateAgentTaskInput) (AgentTaskOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentTaskOutput{}, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	current, err := uc.repo.GetTaskByID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return AgentTaskOutput{}, err
	}
	// 只允许编辑手动创建的临时任务（source_task_id 为空）
	if current.TaskMode != agentdomain.TaskModeTemporary || current.SourceTaskID != "" {
		return AgentTaskOutput{}, fmt.Errorf("%w: only manual temporary task can be edited", ErrInvalidInput)
	}
	if current.Status == agentdomain.TaskStatusRunning || current.Status == agentdomain.TaskStatusClaimed {
		return AgentTaskOutput{}, fmt.Errorf("%w: temporary task is executing, please edit later", ErrInvalidInput)
	}
	script, err := uc.resolveTaskScript(ctx, input.ScriptID, current.TaskType, current.ShellType, current.ScriptPath, current.ScriptText)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = current.Name
	}
	workDir := strings.TrimSpace(input.WorkDir)
	if workDir == "" {
		workDir = current.WorkDir
	}
	timeoutSec := input.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = current.TimeoutSec
	}
	if timeoutSec > 3600 {
		timeoutSec = 3600
	}
	current.Name = name
	current.WorkDir = workDir
	current.TaskType = script.TaskType
	current.ShellType = script.ShellType
	current.ScriptID = script.ScriptID
	current.ScriptName = script.ScriptName
	current.ScriptPath = script.ScriptPath
	current.ScriptText = script.ScriptText
	current.Variables = normalizeTaskVariables(input.Variables)
	targetAgentIDs := normalizeTaskTargetAgentIDs(input.TargetAgentIDs)
	current.TargetAgentIDs = targetAgentIDs
	if len(targetAgentIDs) > 0 {
		current.AgentID = ""
		current.AgentCode = ""
	}
	current.TimeoutSec = timeoutSec
	current.UpdatedAt = uc.now()
	updated, err := uc.repo.UpdateTask(ctx, current)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	return toAgentTaskOutput(updated), nil
}

// DeleteTemporaryTask 删除业务资源并返回处理结果。
func (uc *AgentTaskManager) DeleteTemporaryTask(ctx context.Context, taskID string) error {
	if uc == nil || uc.repo == nil {
		return fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	current, err := uc.repo.GetTaskByID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return err
	}
	// 只允许删除手动创建的临时任务（source_task_id 为空）
	if current.TaskMode != agentdomain.TaskModeTemporary || current.SourceTaskID != "" {
		return fmt.Errorf("%w: only manual temporary task can be deleted", ErrInvalidInput)
	}
	if current.Status == agentdomain.TaskStatusRunning || current.Status == agentdomain.TaskStatusClaimed {
		return fmt.Errorf("%w: temporary task is executing, please stop it first", ErrInvalidInput)
	}
	return uc.repo.DeleteTask(ctx, current.ID)
}

// UpdateResidentTask 更新业务资源并返回处理结果。
func (uc *AgentTaskManager) UpdateResidentTask(ctx context.Context, taskID string, input UpdateAgentTaskInput) (AgentTaskOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentTaskOutput{}, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	current, err := uc.repo.GetTaskByID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return AgentTaskOutput{}, err
	}
	if current.TaskMode != agentdomain.TaskModeResident {
		return AgentTaskOutput{}, fmt.Errorf("%w: only resident task can be edited", ErrInvalidInput)
	}
	if current.Status == agentdomain.TaskStatusRunning || current.Status == agentdomain.TaskStatusClaimed {
		return AgentTaskOutput{}, fmt.Errorf("%w: resident task is executing, please edit later", ErrInvalidInput)
	}
	script, err := uc.resolveTaskScript(ctx, input.ScriptID, current.TaskType, current.ShellType, current.ScriptPath, current.ScriptText)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = current.Name
	}
	workDir := strings.TrimSpace(input.WorkDir)
	if workDir == "" {
		workDir = current.WorkDir
	}
	timeoutSec := input.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = current.TimeoutSec
	}
	if timeoutSec > 3600 {
		timeoutSec = 3600
	}
	current.Name = name
	current.WorkDir = workDir
	current.TaskType = script.TaskType
	current.ShellType = script.ShellType
	current.ScriptID = script.ScriptID
	current.ScriptName = script.ScriptName
	current.ScriptPath = script.ScriptPath
	current.ScriptText = script.ScriptText
	current.Variables = normalizeTaskVariables(input.Variables)
	current.TimeoutSec = timeoutSec
	current.UpdatedAt = uc.now()
	updated, err := uc.repo.UpdateTask(ctx, current)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	return toAgentTaskOutput(updated), nil
}

// DeleteResidentTask 删除业务资源并返回处理结果。
func (uc *AgentTaskManager) DeleteResidentTask(ctx context.Context, taskID string) error {
	if uc == nil || uc.repo == nil {
		return fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	current, err := uc.repo.GetTaskByID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return err
	}
	if current.TaskMode != agentdomain.TaskModeResident {
		return fmt.Errorf("%w: only resident task can be deleted", ErrInvalidInput)
	}
	if current.Status == agentdomain.TaskStatusRunning || current.Status == agentdomain.TaskStatusClaimed {
		return fmt.Errorf("%w: resident task is executing, please stop it first", ErrInvalidInput)
	}
	return uc.repo.DeleteTask(ctx, current.ID)
}

// Poll 封装当前模块的业务处理逻辑。
func (uc *AgentTaskManager) Poll(ctx context.Context, input AgentTaskPollInput) (*AgentTaskOutput, error) {
	if uc == nil || uc.repo == nil {
		return nil, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	instance, err := uc.authenticateAgent(ctx, input.AgentCode, input.Token)
	if err != nil {
		return nil, err
	}
	task, claimed, err := uc.repo.ClaimNextPendingTask(ctx, instance.ID, uc.now())
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, nil
	}
	task, err = syncManagedScriptSnapshotForTask(ctx, uc.repo, task, nil)
	if err != nil {
		return nil, err
	}
	output := toAgentTaskOutput(task)
	return &output, nil
}

// Start 封装当前模块的业务处理逻辑。
func (uc *AgentTaskManager) Start(ctx context.Context, agentCode, token, taskID string) (AgentTaskOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentTaskOutput{}, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	instance, err := uc.authenticateAgent(ctx, agentCode, token)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	task, err := uc.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	if task.AgentID != instance.ID {
		return AgentTaskOutput{}, agentdomain.ErrHeartbeatAuthRejected
	}
	updated, err := uc.repo.MarkTaskRunning(ctx, task.ID, uc.now())
	if err != nil {
		return AgentTaskOutput{}, err
	}
	if _, err = uc.repo.UpdateRuntimeTask(ctx, instance.ID, buildRunningRuntimeTaskPayload(updated)); err != nil {
		return AgentTaskOutput{}, err
	}
	if strings.TrimSpace(updated.SourceTaskID) != "" && strings.TrimSpace(updated.DispatchBatchID) != "" {
		if _, err = uc.syncSourceTemporaryTaskState(ctx, updated.SourceTaskID, updated.DispatchBatchID); err != nil {
			return AgentTaskOutput{}, err
		}
	}
	return toAgentTaskOutput(updated), nil
}

// Finish 封装当前模块的业务处理逻辑。
func (uc *AgentTaskManager) Finish(ctx context.Context, input FinishAgentTaskInput) (AgentTaskOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentTaskOutput{}, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	instance, err := uc.authenticateAgent(ctx, input.AgentCode, input.Token)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	task, err := uc.repo.GetTaskByID(ctx, strings.TrimSpace(input.TaskID))
	if err != nil {
		return AgentTaskOutput{}, err
	}
	if task.AgentID != instance.ID {
		return AgentTaskOutput{}, agentdomain.ErrHeartbeatAuthRejected
	}
	status := input.Status
	if status == "" {
		if input.ExitCode == 0 {
			status = agentdomain.TaskStatusSuccess
		} else {
			status = agentdomain.TaskStatusFailed
		}
	}
	if status != agentdomain.TaskStatusSuccess && status != agentdomain.TaskStatusFailed && status != agentdomain.TaskStatusCancelled {
		return AgentTaskOutput{}, fmt.Errorf("%w: invalid task finish status", ErrInvalidStatus)
	}
	updated, err := uc.repo.FinishTask(
		ctx,
		task.ID,
		status,
		input.ExitCode,
		trimAgentTaskOutput(input.StdoutText),
		trimAgentTaskOutput(input.StderrText),
		trimAgentTaskOutput(input.FailureReason),
		uc.now(),
	)
	if err != nil {
		return AgentTaskOutput{}, err
	}
	if _, err = uc.repo.UpdateRuntimeTask(ctx, instance.ID, buildFinishedRuntimeTaskPayload(updated)); err != nil {
		return AgentTaskOutput{}, err
	}
	if strings.TrimSpace(updated.SourceTaskID) != "" && strings.TrimSpace(updated.DispatchBatchID) != "" {
		if _, err = uc.syncSourceTemporaryTaskState(ctx, updated.SourceTaskID, updated.DispatchBatchID); err != nil {
			return AgentTaskOutput{}, err
		}
	}
	return toAgentTaskOutput(updated), nil
}

// executeBoundTemporaryTask 封装当前模块的业务处理逻辑。
func (uc *AgentTaskManager) executeBoundTemporaryTask(ctx context.Context, sourceTask agentdomain.Task) (agentdomain.Task, error) {
	if uc == nil || uc.repo == nil {
		return agentdomain.Task{}, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	activeChildren, _, err := uc.repo.ListTasks(ctx, agentdomain.TaskListFilter{
		SourceTaskID: strings.TrimSpace(sourceTask.ID),
		Statuses:     activeDispatchTaskStatuses,
		Page:         1,
		PageSize:     500,
	})
	if err != nil {
		return agentdomain.Task{}, err
	}
	if len(activeChildren) > 0 {
		return agentdomain.Task{}, fmt.Errorf("%w: task already has an active dispatch batch", ErrInvalidInput)
	}
	targets, err := resolveTaskDispatchTargets(ctx, uc.repo, sourceTask)
	if err != nil {
		return agentdomain.Task{}, err
	}
	batchID := generateID("agbatch")
	dispatched, err := dispatchTemporaryTaskBatch(
		ctx,
		uc.repo,
		sourceTask,
		targets,
		strings.TrimSpace(sourceTask.Name),
		sourceTask.Variables,
		firstNonEmptyAgentString(strings.TrimSpace(sourceTask.CreatedBy), "manual_execute"),
		batchID,
		uc.now,
	)
	if err != nil {
		return agentdomain.Task{}, err
	}
	nextStatus := aggregateTaskBatchStatus(dispatched)
	sourceTask.Status = nextStatus
	sourceTask.LastRunStatus = nextStatus
	sourceTask.LastRunSummary = buildTaskBatchSummary("已下发执行批次", dispatched)
	sourceTask.DispatchBatchID = batchID
	sourceTask.RunCount++
	sourceTask.UpdatedAt = uc.now()
	return uc.repo.UpdateTask(ctx, sourceTask)
}

// syncSourceTemporaryTaskState 同步外部或内部状态数据。
func (uc *AgentTaskManager) syncSourceTemporaryTaskState(ctx context.Context, sourceTaskID string, batchID string) (agentdomain.Task, error) {
	if uc == nil || uc.repo == nil {
		return agentdomain.Task{}, fmt.Errorf("%w: agent task manager is not configured", ErrInvalidInput)
	}
	sourceTaskID = strings.TrimSpace(sourceTaskID)
	batchID = strings.TrimSpace(batchID)
	if sourceTaskID == "" || batchID == "" {
		return agentdomain.Task{}, fmt.Errorf("%w: source task batch identity is required", ErrInvalidInput)
	}
	source, err := uc.repo.GetTaskByID(ctx, sourceTaskID)
	if err != nil {
		return agentdomain.Task{}, err
	}
	items, _, err := uc.repo.ListTasks(ctx, agentdomain.TaskListFilter{
		SourceTaskID:    sourceTaskID,
		DispatchBatchID: batchID,
		Page:            1,
		PageSize:        500,
	})
	if err != nil {
		return agentdomain.Task{}, err
	}
	if len(items) == 0 {
		return source, nil
	}
	nextStatus := aggregateTaskBatchStatus(items)
	previousBatchID := strings.TrimSpace(source.DispatchBatchID)
	source.Status = nextStatus
	source.LastRunStatus = nextStatus
	source.LastRunSummary = buildTaskBatchSummary("最近一轮执行状态", items)
	source.UpdatedAt = uc.now()
	isTerminal := nextStatus == agentdomain.TaskStatusSuccess || nextStatus == agentdomain.TaskStatusFailed || nextStatus == agentdomain.TaskStatusCancelled
	if isTerminal && previousBatchID == batchID {
		if nextStatus == agentdomain.TaskStatusSuccess {
			source.SuccessCount++
		} else {
			source.FailureCount++
		}
		source.DispatchBatchID = ""
	}
	return uc.repo.UpdateTask(ctx, source)
}

type resolvedTaskScript struct {
	TaskType   string
	ShellType  string
	ScriptID   string
	ScriptName string
	ScriptPath string
	ScriptText string
}

// resolveTaskScript 解析上下文数据，得到后续流程需要的结果。
func (uc *AgentTaskManager) resolveTaskScript(ctx context.Context, scriptID, taskType, shellType, scriptPath, scriptText string) (resolvedTaskScript, error) {
	scriptID = strings.TrimSpace(scriptID)
	if scriptID != "" {
		item, err := uc.repo.GetScriptByID(ctx, scriptID)
		if err != nil {
			return resolvedTaskScript{}, err
		}
		return resolvedTaskScript{
			TaskType:   strings.TrimSpace(item.TaskType),
			ShellType:  firstNonEmptyAgentString(strings.TrimSpace(item.ShellType), "sh"),
			ScriptID:   item.ID,
			ScriptName: strings.TrimSpace(item.Name),
			ScriptPath: strings.TrimSpace(item.ScriptPath),
			ScriptText: strings.TrimSpace(item.ScriptText),
		}, nil
	}
	taskType = firstNonEmptyAgentString(strings.TrimSpace(taskType), string(agentdomain.TaskTypeShellScript))
	if taskType != string(agentdomain.TaskTypeFileDistribution) {
		return resolvedTaskScript{}, fmt.Errorf("%w: please choose a managed script", ErrInvalidInput)
	}
	if !agentdomain.TaskType(taskType).Valid() {
		return resolvedTaskScript{}, fmt.Errorf("%w: unsupported task_type", ErrInvalidInput)
	}
	shellType = firstNonEmptyAgentString(strings.TrimSpace(shellType), "sh")
	if shellType != "sh" && shellType != "bash" {
		return resolvedTaskScript{}, fmt.Errorf("%w: unsupported shell_type", ErrInvalidInput)
	}
	scriptPath = strings.TrimSpace(scriptPath)
	scriptText = strings.TrimSpace(scriptText)
	switch taskType {
	case string(agentdomain.TaskTypeShellScript):
		if scriptText == "" {
			return resolvedTaskScript{}, fmt.Errorf("%w: script_text is required", ErrInvalidInput)
		}
	case string(agentdomain.TaskTypeScriptFile):
		if scriptPath == "" {
			return resolvedTaskScript{}, fmt.Errorf("%w: script_path is required", ErrInvalidInput)
		}
		if scriptText == "" {
			return resolvedTaskScript{}, fmt.Errorf("%w: uploaded script content is required", ErrInvalidInput)
		}
		if !isSupportedScriptFile(scriptPath) {
			return resolvedTaskScript{}, fmt.Errorf("%w: script file only supports .sh/.bash", ErrInvalidInput)
		}
	case string(agentdomain.TaskTypeFileDistribution):
		if scriptPath == "" {
			return resolvedTaskScript{}, fmt.Errorf("%w: file name is required", ErrInvalidInput)
		}
		if scriptText == "" {
			return resolvedTaskScript{}, fmt.Errorf("%w: uploaded file content is required", ErrInvalidInput)
		}
	}
	return resolvedTaskScript{
		TaskType:   taskType,
		ShellType:  shellType,
		ScriptPath: scriptPath,
		ScriptText: scriptText,
	}, nil
}

// authenticateAgent 封装当前模块的业务处理逻辑。
func (uc *AgentTaskManager) authenticateAgent(ctx context.Context, agentCode, token string) (agentdomain.Instance, error) {
	agentCode = strings.TrimSpace(agentCode)
	if agentCode == "" {
		return agentdomain.Instance{}, fmt.Errorf("%w: agent_code is required", ErrInvalidInput)
	}
	item, err := uc.repo.GetInstanceByCode(ctx, agentCode)
	if err != nil {
		return agentdomain.Instance{}, err
	}
	if strings.TrimSpace(token) == "" || subtleConstantTimeCompare(strings.TrimSpace(item.Token), strings.TrimSpace(token)) == false {
		return agentdomain.Instance{}, agentdomain.ErrHeartbeatAuthRejected
	}
	if item.Status != agentdomain.StatusActive {
		return agentdomain.Instance{}, agentdomain.ErrHeartbeatAuthRejected
	}
	return item, nil
}

// resolveNewTaskStatus 解析上下文数据，得到后续流程需要的结果。
func (uc *AgentTaskManager) resolveNewTaskStatus(ctx context.Context, agentID, excludeTaskID string) (agentdomain.TaskStatus, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return agentdomain.TaskStatusPending, nil
	}
	items, _, err := uc.repo.ListTasks(ctx, agentdomain.TaskListFilter{
		AgentID:  agentID,
		Statuses: []agentdomain.TaskStatus{agentdomain.TaskStatusPending, agentdomain.TaskStatusQueued, agentdomain.TaskStatusClaimed, agentdomain.TaskStatusRunning},
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

// toAgentTaskOutput 将领域对象转换为接口响应结构。
func toAgentTaskOutput(item agentdomain.Task) AgentTaskOutput {
	variables := make(map[string]string, len(item.Variables))
	targetAgentIDs := make([]string, 0, len(item.TargetAgentIDs))
	targetAgentIDs = append(targetAgentIDs, item.TargetAgentIDs...)
	keys := make([]string, 0, len(item.Variables))
	for key := range item.Variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		variables[key] = item.Variables[key]
	}
	return AgentTaskOutput{
		ID:              item.ID,
		AgentID:         item.AgentID,
		AgentCode:       item.AgentCode,
		TargetAgentIDs:  targetAgentIDs,
		SourceTaskID:    item.SourceTaskID,
		DispatchBatchID: item.DispatchBatchID,
		Name:            item.Name,
		TaskMode:        string(item.TaskMode),
		TaskType:        item.TaskType,
		ShellType:       item.ShellType,
		WorkDir:         item.WorkDir,
		ScriptID:        item.ScriptID,
		ScriptName:      item.ScriptName,
		ScriptPath:      item.ScriptPath,
		ScriptText:      item.ScriptText,
		Variables:       variables,
		TimeoutSec:      item.TimeoutSec,
		Status:          item.Status,
		ClaimedAt:       item.ClaimedAt,
		StartedAt:       item.StartedAt,
		FinishedAt:      item.FinishedAt,
		ExitCode:        item.ExitCode,
		StdoutText:      item.StdoutText,
		StderrText:      item.StderrText,
		FailureReason:   item.FailureReason,
		RunCount:        item.RunCount,
		SuccessCount:    item.SuccessCount,
		FailureCount:    item.FailureCount,
		LastRunStatus:   string(item.LastRunStatus),
		LastRunSummary:  item.LastRunSummary,
		CreatedBy:       item.CreatedBy,
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
	}
}

// normalizeTaskVariables 标准化输入值，保证后续逻辑使用统一格式。
func normalizeTaskVariables(items map[string]string) map[string]string {
	if len(items) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(items))
	for key, value := range items {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		result[k] = strings.TrimSpace(value)
	}
	return result
}

// trimAgentTaskOutput 封装当前模块的业务处理逻辑。
func trimAgentTaskOutput(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 65535 {
		return value
	}
	return value[:65535]
}

// isSupportedScriptFile 封装当前模块的业务处理逻辑。
func isSupportedScriptFile(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".sh", ".bash":
		return true
	default:
		return false
	}
}

// buildRunningRuntimeTaskPayload 组装业务执行所需的输入数据。
func buildRunningRuntimeTaskPayload(task agentdomain.Task) agentdomain.RuntimeTaskPayload {
	return agentdomain.RuntimeTaskPayload{
		CurrentTaskID:      strings.TrimSpace(task.ID),
		CurrentTaskName:    strings.TrimSpace(task.Name),
		CurrentTaskType:    strings.TrimSpace(task.TaskType),
		CurrentTaskStarted: task.StartedAt,
		LastTaskStatus:     agentdomain.LastTaskStatusRunning,
		LastTaskSummary:    "任务执行中",
		LastTaskFinishedAt: nil,
	}
}

// buildFinishedRuntimeTaskPayload 组装业务执行所需的输入数据。
func buildFinishedRuntimeTaskPayload(task agentdomain.Task) agentdomain.RuntimeTaskPayload {
	return agentdomain.RuntimeTaskPayload{
		CurrentTaskID:      "",
		CurrentTaskName:    "",
		CurrentTaskType:    "",
		CurrentTaskStarted: nil,
		LastTaskStatus:     runtimeLastTaskStatus(task.LastRunStatus),
		LastTaskSummary:    strings.TrimSpace(task.LastRunSummary),
		LastTaskFinishedAt: task.FinishedAt,
	}
}

// runtimeLastTaskStatus 封装当前模块的业务处理逻辑。
func runtimeLastTaskStatus(status agentdomain.TaskStatus) agentdomain.LastTaskStatus {
	switch status {
	case agentdomain.TaskStatusRunning:
		return agentdomain.LastTaskStatusRunning
	case agentdomain.TaskStatusSuccess:
		return agentdomain.LastTaskStatusSuccess
	case agentdomain.TaskStatusCancelled:
		return agentdomain.LastTaskStatusCancelled
	case agentdomain.TaskStatusFailed:
		return agentdomain.LastTaskStatusFailed
	default:
		return agentdomain.LastTaskStatusUnknown
	}
}
