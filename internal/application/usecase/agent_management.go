package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	agentdomain "gos/internal/domain/agent"
)

const defaultAgentOfflineAfter = 45 * time.Second

type AgentManager struct {
	repo         agentdomain.Repository
	now          func() time.Time
	offlineAfter time.Duration
}

type CreateAgentInput struct {
	AgentCode       string
	Name            string
	EnvironmentCode string
	WorkDir         string
	Tags            []string
	Status          agentdomain.Status
	Remark          string
}

type UpdateAgentInput = CreateAgentInput

type AgentListOutput struct {
	Items []AgentOutput
	Total int64
}

type AgentOutput struct {
	ID                        string                     `json:"id"`
	AgentCode                 string                     `json:"agent_code"`
	Name                      string                     `json:"name"`
	EnvironmentCode           string                     `json:"environment_code"`
	WorkDir                   string                     `json:"work_dir"`
	Token                     string                     `json:"token,omitempty"`
	Tags                      []string                   `json:"tags"`
	Hostname                  string                     `json:"hostname"`
	HostIP                    string                     `json:"host_ip"`
	AgentVersion              string                     `json:"agent_version"`
	OS                        string                     `json:"os"`
	Arch                      string                     `json:"arch"`
	Status                    agentdomain.Status         `json:"status"`
	RuntimeState              agentdomain.RuntimeState   `json:"runtime_state"`
	LastHeartbeatAt           *time.Time                 `json:"last_heartbeat_at,omitempty"`
	HeartbeatAgeSec           int64                      `json:"heartbeat_age_sec"`
	CurrentTaskID             string                     `json:"current_task_id"`
	CurrentTaskName           string                     `json:"current_task_name"`
	CurrentTaskType           string                     `json:"current_task_type"`
	CurrentTaskStartedAt      *time.Time                 `json:"current_task_started_at,omitempty"`
	CurrentResidentTaskID     string                     `json:"current_resident_task_id"`
	CurrentResidentTaskName   string                     `json:"current_resident_task_name"`
	CurrentResidentTaskStatus agentdomain.TaskStatus     `json:"current_resident_task_status"`
	LastTaskStatus            agentdomain.LastTaskStatus `json:"last_task_status"`
	LastTaskSummary           string                     `json:"last_task_summary"`
	LastTaskFinishedAt        *time.Time                 `json:"last_task_finished_at,omitempty"`
	Remark                    string                     `json:"remark"`
	CreatedAt                 time.Time                  `json:"created_at"`
	UpdatedAt                 time.Time                  `json:"updated_at"`
}

type AgentHeartbeatInput struct {
	AgentCode          string
	Token              string
	Hostname           string
	HostIP             string
	AgentVersion       string
	OS                 string
	Arch               string
	WorkDir            string
	Tags               []string
	CurrentTaskID      string
	CurrentTaskName    string
	CurrentTaskType    string
	CurrentTaskStarted *time.Time
	LastTaskStatus     agentdomain.LastTaskStatus
	LastTaskSummary    string
	LastTaskFinishedAt *time.Time
}

type AgentRegisterInput struct {
	RegistrationToken string
	MachineID         string
	Name              string
	EnvironmentCode   string
	Hostname          string
	HostIP            string
	AgentVersion      string
	OS                string
	Arch              string
	WorkDir           string
	Tags              []string
}

type AgentRegisterOutput struct {
	AgentID    string `json:"agent_id"`
	AgentCode  string `json:"agent_code"`
	Token      string `json:"token"`
	Name       string `json:"name"`
	WorkDir    string `json:"work_dir"`
	Registered bool   `json:"registered"`
}

type AgentInstallConfigOutput struct {
	AgentID           string `json:"agent_id"`
	AgentCode         string `json:"agent_code"`
	RegistrationToken string `json:"registration_token,omitempty"`
	SuggestedPath     string `json:"suggested_path"`
	LaunchCommand     string `json:"launch_command"`
	ConfigYAML        string `json:"config_yaml"`
	ResolvedServerURL string `json:"resolved_server_url"`
	HeartbeatInterval string `json:"heartbeat_interval"`
	PollInterval      string `json:"poll_interval"`
}

// NewAgentManager 创建并返回对应组件实例。
func NewAgentManager(repo agentdomain.Repository) *AgentManager {
	return &AgentManager{repo: repo, now: func() time.Time { return time.Now().UTC() }, offlineAfter: defaultAgentOfflineAfter}
}

// List 查询并返回列表数据。
func (uc *AgentManager) List(ctx context.Context, filter agentdomain.ListFilter) (AgentListOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentListOutput{}, fmt.Errorf("%w: agent manager is not configured", ErrInvalidInput)
	}
	if filter.Status != "" && !filter.Status.Valid() {
		return AgentListOutput{}, fmt.Errorf("%w: invalid agent status", ErrInvalidStatus)
	}
	if filter.RuntimeState != "" && !filter.RuntimeState.Valid() {
		return AgentListOutput{}, fmt.Errorf("%w: invalid runtime state", ErrInvalidStatus)
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	queryFilter := filter
	queryFilter.Page = 1
	queryFilter.PageSize = 500
	items, _, err := uc.repo.ListInstances(ctx, queryFilter)
	if err != nil {
		return AgentListOutput{}, err
	}
	outputs := make([]AgentOutput, 0, len(items))
	for _, item := range items {
		output := uc.toOutput(item, false)
		uc.enrichCurrentTask(ctx, &output)
		uc.enrichCurrentResidentTask(ctx, &output)
		if filter.RuntimeState != "" && output.RuntimeState != filter.RuntimeState {
			continue
		}
		outputs = append(outputs, output)
	}
	total := int64(len(outputs))
	start := (page - 1) * pageSize
	if start >= len(outputs) {
		return AgentListOutput{Items: []AgentOutput{}, Total: total}, nil
	}
	end := start + pageSize
	if end > len(outputs) {
		end = len(outputs)
	}
	return AgentListOutput{Items: outputs[start:end], Total: total}, nil
}

// Get 查询并返回指定资源数据。
func (uc *AgentManager) Get(ctx context.Context, id string) (AgentOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentOutput{}, fmt.Errorf("%w: agent manager is not configured", ErrInvalidInput)
	}
	item, err := uc.repo.GetInstanceByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return AgentOutput{}, err
	}
	output := uc.toOutput(item, false)
	uc.enrichCurrentTask(ctx, &output)
	uc.enrichCurrentResidentTask(ctx, &output)
	return output, nil
}

// Create 创建业务资源并返回处理结果。
func (uc *AgentManager) Create(ctx context.Context, input CreateAgentInput) (AgentOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentOutput{}, fmt.Errorf("%w: agent manager is not configured", ErrInvalidInput)
	}
	now := uc.now()
	item, err := uc.normalizeInput("", input, now)
	if err != nil {
		return AgentOutput{}, err
	}
	item.ID = generateID("agt")
	item.CreatedAt = now
	item.UpdatedAt = now
	created, err := uc.repo.CreateInstance(ctx, item)
	if err != nil {
		return AgentOutput{}, err
	}
	return uc.toOutput(created, true), nil
}

// Update 更新业务资源并返回处理结果。
func (uc *AgentManager) Update(ctx context.Context, id string, input UpdateAgentInput) (AgentOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentOutput{}, fmt.Errorf("%w: agent manager is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return AgentOutput{}, ErrInvalidID
	}
	current, err := uc.repo.GetInstanceByID(ctx, id)
	if err != nil {
		return AgentOutput{}, err
	}
	item, err := uc.normalizeInput(id, input, uc.now())
	if err != nil {
		return AgentOutput{}, err
	}
	item.ID = current.ID
	item.MachineID = current.MachineID
	item.CreatedAt = current.CreatedAt
	item.UpdatedAt = uc.now()
	item.Token = current.Token
	item.Hostname = current.Hostname
	item.HostIP = current.HostIP
	item.AgentVersion = current.AgentVersion
	item.OS = current.OS
	item.Arch = current.Arch
	item.LastHeartbeatAt = current.LastHeartbeatAt
	item.CurrentTaskID = current.CurrentTaskID
	item.CurrentTaskName = current.CurrentTaskName
	item.CurrentTaskType = current.CurrentTaskType
	item.CurrentTaskStarted = current.CurrentTaskStarted
	item.LastTaskStatus = current.LastTaskStatus
	item.LastTaskSummary = current.LastTaskSummary
	item.LastTaskFinishedAt = current.LastTaskFinishedAt
	item.Token = current.Token
	updated, err := uc.repo.UpdateInstance(ctx, item)
	if err != nil {
		return AgentOutput{}, err
	}
	return uc.toOutput(updated, true), nil
}

// ResetToken 封装当前模块的业务处理逻辑。
func (uc *AgentManager) ResetToken(ctx context.Context, id string) (AgentOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentOutput{}, fmt.Errorf("%w: agent manager is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return AgentOutput{}, ErrInvalidID
	}
	current, err := uc.repo.GetInstanceByID(ctx, id)
	if err != nil {
		return AgentOutput{}, err
	}
	current.Token = generateAgentToken()
	current.UpdatedAt = uc.now()
	updated, err := uc.repo.UpdateInstance(ctx, current)
	if err != nil {
		return AgentOutput{}, err
	}
	return uc.toOutput(updated, true), nil
}

// Delete 删除业务资源并返回处理结果。
func (uc *AgentManager) Delete(ctx context.Context, id string) error {
	if uc == nil || uc.repo == nil {
		return fmt.Errorf("%w: agent manager is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidID
	}
	current, err := uc.repo.GetInstanceByID(ctx, id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(current.CurrentTaskID) != "" {
		return fmt.Errorf("%w: Agent 当前仍有执行中的任务，请先停止或等待任务结束", agentdomain.ErrInstanceDeleteBlocked)
	}

	relatedTasks, err := uc.listAllTasksForAgentDelete(ctx)
	if err != nil {
		return err
	}
	now := uc.now()
	for _, task := range relatedTasks {
		if task.AgentID != current.ID {
			continue
		}
		if task.Status == agentdomain.TaskStatusPending || task.Status == agentdomain.TaskStatusQueued || task.Status == agentdomain.TaskStatusClaimed || task.Status == agentdomain.TaskStatusRunning {
			return fmt.Errorf("%w: Agent 仍存在未结束任务，请先处理后再删除", agentdomain.ErrInstanceDeleteBlocked)
		}
	}

	for _, task := range relatedTasks {
		updatedTask := task
		needsUpdate := false

		if updatedTask.AgentID == current.ID {
			if updatedTask.TaskMode == agentdomain.TaskModeResident {
				if err := uc.repo.DeleteTask(ctx, updatedTask.ID); err != nil {
					return err
				}
				continue
			}
			updatedTask.AgentID = ""
			if updatedTask.Status == agentdomain.TaskStatusDraft {
				finishedAt := now
				updatedTask.Status = agentdomain.TaskStatusCancelled
				updatedTask.LastRunStatus = agentdomain.TaskStatusCancelled
				updatedTask.LastRunSummary = "原绑定 Agent 已删除，任务已取消"
				updatedTask.FailureReason = "原绑定 Agent 已删除，任务已取消"
				updatedTask.FinishedAt = &finishedAt
			}
			needsUpdate = true
		}

		if len(updatedTask.TargetAgentIDs) > 0 {
			filteredTargetIDs := removeAgentIDFromTaskTargets(updatedTask.TargetAgentIDs, current.ID)
			if len(filteredTargetIDs) != len(updatedTask.TargetAgentIDs) {
				updatedTask.TargetAgentIDs = filteredTargetIDs
				needsUpdate = true
			}
		}

		if !needsUpdate {
			continue
		}
		updatedTask.UpdatedAt = now
		if _, err := uc.repo.UpdateTask(ctx, updatedTask); err != nil {
			return err
		}
	}

	return uc.repo.DeleteInstance(ctx, current.ID)
}

// BuildInstallConfig 组装业务执行所需的输入数据。
func (uc *AgentManager) BuildInstallConfig(ctx context.Context, id string, baseURL string) (AgentInstallConfigOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentInstallConfigOutput{}, fmt.Errorf("%w: agent manager is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return AgentInstallConfigOutput{}, ErrInvalidID
	}
	item, err := uc.repo.GetInstanceByID(ctx, id)
	if err != nil {
		return AgentInstallConfigOutput{}, err
	}
	return uc.buildInstallConfigOutput(ctx, baseURL, item)
}

// listAllTasksForAgentDelete 查询并返回列表数据。
func (uc *AgentManager) listAllTasksForAgentDelete(ctx context.Context) ([]agentdomain.Task, error) {
	page := 1
	pageSize := 500
	result := make([]agentdomain.Task, 0)
	for {
		items, total, err := uc.repo.ListTasks(ctx, agentdomain.TaskListFilter{
			Page:     page,
			PageSize: pageSize,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		if int64(len(result)) >= total || len(items) == 0 {
			break
		}
		page++
	}
	return result, nil
}

// removeAgentIDFromTaskTargets 查询并返回指定资源数据。
func removeAgentIDFromTaskTargets(items []string, agentID string) []string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || len(items) == 0 {
		return append([]string(nil), items...)
	}
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" || value == agentID {
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

// BuildBootstrapConfig 组装业务执行所需的输入数据。
func (uc *AgentManager) BuildBootstrapConfig(ctx context.Context, baseURL string) (AgentInstallConfigOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentInstallConfigOutput{}, fmt.Errorf("%w: agent manager is not configured", ErrInvalidInput)
	}
	return uc.buildInstallConfigOutput(ctx, baseURL, agentdomain.Instance{
		Name:         "Auto Registered Agent",
		WorkDir:      "/tmp/gos-agent",
		Tags:         []string{"linux"},
		AgentVersion: "v1.0.0",
	})
}

// ResetBootstrapToken 封装当前模块的业务处理逻辑。
func (uc *AgentManager) ResetBootstrapToken(ctx context.Context, baseURL string) (AgentInstallConfigOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentInstallConfigOutput{}, fmt.Errorf("%w: agent manager is not configured", ErrInvalidInput)
	}
	if _, err := uc.repo.ResetBootstrapToken(ctx); err != nil {
		return AgentInstallConfigOutput{}, err
	}
	return uc.BuildBootstrapConfig(ctx, baseURL)
}

// Register 封装当前模块的业务处理逻辑。
func (uc *AgentManager) Register(ctx context.Context, input AgentRegisterInput) (AgentRegisterOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentRegisterOutput{}, fmt.Errorf("%w: agent manager is not configured", ErrInvalidInput)
	}
	bootstrapToken, err := uc.repo.GetBootstrapToken(ctx)
	if err != nil {
		return AgentRegisterOutput{}, err
	}
	if strings.TrimSpace(input.RegistrationToken) == "" || !subtleConstantTimeCompare(strings.TrimSpace(bootstrapToken), strings.TrimSpace(input.RegistrationToken)) {
		return AgentRegisterOutput{}, agentdomain.ErrBootstrapTokenInvalid
	}
	machineID := strings.TrimSpace(input.MachineID)
	if machineID == "" {
		return AgentRegisterOutput{}, fmt.Errorf("%w: machine_id is required", ErrInvalidInput)
	}
	workDir := strings.TrimSpace(input.WorkDir)
	if workDir == "" {
		return AgentRegisterOutput{}, fmt.Errorf("%w: work_dir is required", ErrInvalidInput)
	}

	now := uc.now()
	instance, err := uc.repo.GetInstanceByMachineID(ctx, machineID)
	if err != nil && !errors.Is(err, agentdomain.ErrInstanceNotFound) {
		return AgentRegisterOutput{}, err
	}
	registered := false
	if errors.Is(err, agentdomain.ErrInstanceNotFound) {
		instance = agentdomain.Instance{
			ID:              generateID("agt"),
			MachineID:       machineID,
			AgentCode:       uc.generateAutoAgentCode(input),
			Name:            firstNonEmptyAgentString(strings.TrimSpace(input.Name), strings.TrimSpace(input.Hostname), uc.generateAutoAgentCode(input)),
			EnvironmentCode: strings.TrimSpace(input.EnvironmentCode),
			WorkDir:         workDir,
			Token:           generateAgentToken(),
			Tags:            normalizeAgentTags(input.Tags),
			Hostname:        strings.TrimSpace(input.Hostname),
			HostIP:          strings.TrimSpace(input.HostIP),
			AgentVersion:    strings.TrimSpace(input.AgentVersion),
			OS:              strings.TrimSpace(input.OS),
			Arch:            strings.TrimSpace(input.Arch),
			Status:          agentdomain.StatusActive,
			LastTaskStatus:  agentdomain.LastTaskStatusUnknown,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		created, createErr := uc.repo.CreateInstance(ctx, instance)
		if createErr != nil {
			return AgentRegisterOutput{}, createErr
		}
		instance = created
		registered = true
	} else {
		instance.MachineID = machineID
		instance.WorkDir = workDir
		instance.Tags = normalizeAgentTags(input.Tags)
		instance.Hostname = firstNonEmptyAgentString(strings.TrimSpace(input.Hostname), instance.Hostname)
		instance.HostIP = firstNonEmptyAgentString(strings.TrimSpace(input.HostIP), instance.HostIP)
		instance.AgentVersion = firstNonEmptyAgentString(strings.TrimSpace(input.AgentVersion), instance.AgentVersion)
		instance.OS = firstNonEmptyAgentString(strings.TrimSpace(input.OS), instance.OS)
		instance.Arch = firstNonEmptyAgentString(strings.TrimSpace(input.Arch), instance.Arch)
		if strings.TrimSpace(instance.EnvironmentCode) == "" {
			instance.EnvironmentCode = strings.TrimSpace(input.EnvironmentCode)
		}
		if strings.TrimSpace(instance.Name) == "" {
			instance.Name = firstNonEmptyAgentString(strings.TrimSpace(input.Name), strings.TrimSpace(input.Hostname), instance.AgentCode)
		}
		if strings.TrimSpace(instance.Token) == "" {
			instance.Token = generateAgentToken()
		}
		instance.UpdatedAt = now
		updated, updateErr := uc.repo.UpdateInstance(ctx, instance)
		if updateErr != nil {
			return AgentRegisterOutput{}, updateErr
		}
		instance = updated
	}
	return AgentRegisterOutput{
		AgentID:    instance.ID,
		AgentCode:  instance.AgentCode,
		Token:      instance.Token,
		Name:       instance.Name,
		WorkDir:    instance.WorkDir,
		Registered: registered,
	}, nil
}

// buildInstallConfigOutput 组装业务执行所需的输入数据。
func (uc *AgentManager) buildInstallConfigOutput(ctx context.Context, baseURL string, item agentdomain.Instance) (AgentInstallConfigOutput, error) {
	bootstrapToken, err := uc.repo.GetBootstrapToken(ctx)
	if err != nil {
		return AgentInstallConfigOutput{}, err
	}
	resolvedBaseURL := normalizeAgentBaseURL(baseURL)
	configDoc := struct {
		Server struct {
			BaseURL string `yaml:"base_url"`
		} `yaml:"server"`
		Agent struct {
			RegistrationToken string   `yaml:"registration_token"`
			Name              string   `yaml:"name,omitempty"`
			EnvironmentCode   string   `yaml:"environment_code,omitempty"`
			WorkDir           string   `yaml:"work_dir"`
			HeartbeatInterval string   `yaml:"heartbeat_interval"`
			PollInterval      string   `yaml:"poll_interval"`
			Version           string   `yaml:"version"`
			Tags              []string `yaml:"tags,omitempty"`
		} `yaml:"agent"`
	}{}
	configDoc.Server.BaseURL = resolvedBaseURL
	configDoc.Agent.RegistrationToken = bootstrapToken
	configDoc.Agent.Name = strings.TrimSpace(item.Name)
	configDoc.Agent.EnvironmentCode = strings.TrimSpace(item.EnvironmentCode)
	configDoc.Agent.WorkDir = firstNonEmptyAgentString(strings.TrimSpace(item.WorkDir), "/tmp/gos-agent")
	configDoc.Agent.HeartbeatInterval = "15s"
	configDoc.Agent.PollInterval = "5s"
	configDoc.Agent.Version = firstNonEmptyAgentString(strings.TrimSpace(item.AgentVersion), "v1.0.0")
	configDoc.Agent.Tags = append([]string(nil), item.Tags...)

	content, err := yaml.Marshal(configDoc)
	if err != nil {
		return AgentInstallConfigOutput{}, err
	}
	suggestedPath := "/etc/gos-agent/config.yaml"
	return AgentInstallConfigOutput{
		AgentID:           item.ID,
		AgentCode:         item.AgentCode,
		RegistrationToken: bootstrapToken,
		SuggestedPath:     suggestedPath,
		LaunchCommand:     fmt.Sprintf("./gos-agent --config %s", suggestedPath),
		ConfigYAML:        string(content),
		ResolvedServerURL: resolvedBaseURL,
		HeartbeatInterval: configDoc.Agent.HeartbeatInterval,
		PollInterval:      configDoc.Agent.PollInterval,
	}, nil
}

// generateAutoAgentCode 封装当前模块的业务处理逻辑。
func (uc *AgentManager) generateAutoAgentCode(input AgentRegisterInput) string {
	base := firstNonEmptyAgentString(strings.TrimSpace(input.Name), strings.TrimSpace(input.Hostname), "agent")
	base = normalizeAgentCodeSegment(base)
	if base == "" {
		base = "agent"
	}
	suffixSource := strings.ToLower(strings.TrimSpace(input.MachineID))
	if len(suffixSource) > 6 {
		suffixSource = suffixSource[len(suffixSource)-6:]
	}
	suffixSource = normalizeAgentCodeSegment(suffixSource)
	if suffixSource == "" {
		suffixSource = generateID("agt")[:6]
	}
	code := strings.Trim(base+"-"+suffixSource, "-")
	if len(code) > 100 {
		code = code[:100]
	}
	return code
}

// UpdateStatus 更新业务资源并返回处理结果。
func (uc *AgentManager) UpdateStatus(ctx context.Context, id string, status agentdomain.Status) (AgentOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentOutput{}, fmt.Errorf("%w: agent manager is not configured", ErrInvalidInput)
	}
	if !status.Valid() {
		return AgentOutput{}, fmt.Errorf("%w: invalid agent status", ErrInvalidStatus)
	}
	current, err := uc.repo.GetInstanceByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return AgentOutput{}, err
	}
	current.Status = status
	current.UpdatedAt = uc.now()
	updated, err := uc.repo.UpdateInstance(ctx, current)
	if err != nil {
		return AgentOutput{}, err
	}
	return uc.toOutput(updated, false), nil
}

// Heartbeat 封装当前模块的业务处理逻辑。
func (uc *AgentManager) Heartbeat(ctx context.Context, input AgentHeartbeatInput) (AgentOutput, error) {
	if uc == nil || uc.repo == nil {
		return AgentOutput{}, fmt.Errorf("%w: agent manager is not configured", ErrInvalidInput)
	}
	agentCode := strings.TrimSpace(input.AgentCode)
	if agentCode == "" {
		return AgentOutput{}, fmt.Errorf("%w: agent_code is required", ErrInvalidInput)
	}
	item, err := uc.repo.GetInstanceByCode(ctx, agentCode)
	if err != nil {
		return AgentOutput{}, err
	}
	if strings.TrimSpace(input.Token) == "" || subtleConstantTimeCompare(strings.TrimSpace(item.Token), strings.TrimSpace(input.Token)) == false {
		return AgentOutput{}, agentdomain.ErrHeartbeatAuthRejected
	}
	if item.Status == agentdomain.StatusDisabled {
		return AgentOutput{}, agentdomain.ErrHeartbeatAuthRejected
	}
	payload := agentdomain.HeartbeatPayload{
		Hostname:           firstNonEmptyAgentString(strings.TrimSpace(input.Hostname), strings.TrimSpace(item.Hostname)),
		HostIP:             strings.TrimSpace(input.HostIP),
		AgentVersion:       strings.TrimSpace(input.AgentVersion),
		OS:                 strings.TrimSpace(input.OS),
		Arch:               strings.TrimSpace(input.Arch),
		WorkDir:            firstNonEmptyAgentString(strings.TrimSpace(input.WorkDir), strings.TrimSpace(item.WorkDir)),
		Tags:               normalizeAgentTags(input.Tags),
		CurrentTaskID:      strings.TrimSpace(input.CurrentTaskID),
		CurrentTaskName:    strings.TrimSpace(input.CurrentTaskName),
		CurrentTaskType:    strings.TrimSpace(input.CurrentTaskType),
		CurrentTaskStarted: input.CurrentTaskStarted,
		LastTaskStatus:     normalizeLastTaskStatus(input.LastTaskStatus, item.LastTaskStatus),
		LastTaskSummary:    strings.TrimSpace(input.LastTaskSummary),
		LastTaskFinishedAt: input.LastTaskFinishedAt,
	}
	updated, err := uc.repo.UpdateHeartbeat(ctx, item.ID, payload)
	if err != nil {
		return AgentOutput{}, err
	}
	return uc.toOutput(updated, false), nil
}

// normalizeInput 标准化输入值，保证后续逻辑使用统一格式。
func (uc *AgentManager) normalizeInput(id string, input CreateAgentInput, now time.Time) (agentdomain.Instance, error) {
	agentCode := strings.ToLower(strings.TrimSpace(input.AgentCode))
	if agentCode == "" {
		return agentdomain.Instance{}, fmt.Errorf("%w: agent_code is required", ErrInvalidInput)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return agentdomain.Instance{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	workDir := strings.TrimSpace(input.WorkDir)
	if workDir == "" {
		return agentdomain.Instance{}, fmt.Errorf("%w: work_dir is required", ErrInvalidInput)
	}
	status := input.Status
	if status == "" {
		status = agentdomain.StatusActive
	}
	if !status.Valid() {
		return agentdomain.Instance{}, fmt.Errorf("%w: invalid agent status", ErrInvalidStatus)
	}
	token := generateAgentToken()
	return agentdomain.Instance{
		ID:              id,
		AgentCode:       agentCode,
		Name:            name,
		EnvironmentCode: strings.TrimSpace(input.EnvironmentCode),
		WorkDir:         workDir,
		Token:           token,
		Tags:            normalizeAgentTags(input.Tags),
		Status:          status,
		LastTaskStatus:  agentdomain.LastTaskStatusUnknown,
		Remark:          strings.TrimSpace(input.Remark),
		UpdatedAt:       now,
	}, nil
}

// toOutput 将领域对象转换为接口响应结构。
func (uc *AgentManager) toOutput(item agentdomain.Instance, includeToken bool) AgentOutput {
	now := uc.now()
	runtimeState := resolveAgentRuntimeState(item, now, uc.offlineAfter)
	heartbeatAge := int64(0)
	if !item.LastHeartbeatAt.IsZero() {
		heartbeatAge = int64(now.Sub(item.LastHeartbeatAt).Seconds())
		if heartbeatAge < 0 {
			heartbeatAge = 0
		}
	}
	output := AgentOutput{
		ID:              item.ID,
		AgentCode:       item.AgentCode,
		Name:            item.Name,
		EnvironmentCode: item.EnvironmentCode,
		WorkDir:         item.WorkDir,
		Tags:            append([]string(nil), item.Tags...),
		Hostname:        item.Hostname,
		HostIP:          item.HostIP,
		AgentVersion:    item.AgentVersion,
		OS:              item.OS,
		Arch:            item.Arch,
		Status:          item.Status,
		RuntimeState:    runtimeState,
		HeartbeatAgeSec: heartbeatAge,
		CurrentTaskID:   item.CurrentTaskID,
		CurrentTaskName: item.CurrentTaskName,
		CurrentTaskType: item.CurrentTaskType,
		LastTaskStatus:  item.LastTaskStatus,
		LastTaskSummary: item.LastTaskSummary,
		Remark:          item.Remark,
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
	}
	if includeToken {
		output.Token = item.Token
	}
	if !item.LastHeartbeatAt.IsZero() {
		value := item.LastHeartbeatAt
		output.LastHeartbeatAt = &value
	}
	if item.CurrentTaskStarted != nil && !item.CurrentTaskStarted.IsZero() {
		value := item.CurrentTaskStarted.UTC()
		output.CurrentTaskStartedAt = &value
	}
	if item.LastTaskFinishedAt != nil && !item.LastTaskFinishedAt.IsZero() {
		value := item.LastTaskFinishedAt.UTC()
		output.LastTaskFinishedAt = &value
	}
	return output
}

// enrichCurrentTask 封装当前模块的业务处理逻辑。
func (uc *AgentManager) enrichCurrentTask(ctx context.Context, output *AgentOutput) {
	if uc == nil || uc.repo == nil || output == nil {
		return
	}
	if strings.TrimSpace(output.CurrentTaskID) == "" {
		return
	}
	if strings.TrimSpace(output.CurrentTaskName) != "" && strings.TrimSpace(output.CurrentTaskType) != "" {
		return
	}
	task, err := uc.repo.GetTaskByID(ctx, strings.TrimSpace(output.CurrentTaskID))
	if err != nil {
		return
	}
	if strings.TrimSpace(output.CurrentTaskName) == "" {
		output.CurrentTaskName = strings.TrimSpace(task.Name)
	}
	if strings.TrimSpace(output.CurrentTaskType) == "" {
		output.CurrentTaskType = strings.TrimSpace(task.TaskType)
	}
}

// enrichCurrentResidentTask 封装当前模块的业务处理逻辑。
func (uc *AgentManager) enrichCurrentResidentTask(ctx context.Context, output *AgentOutput) {
	if uc == nil || uc.repo == nil || output == nil || strings.TrimSpace(output.ID) == "" {
		return
	}
	list, _, err := uc.repo.ListTasks(ctx, agentdomain.TaskListFilter{
		AgentID:  strings.TrimSpace(output.ID),
		Page:     1,
		PageSize: 100,
	})
	if err != nil {
		return
	}
	current, ok := selectCurrentResidentTask(list)
	if !ok {
		return
	}
	output.CurrentResidentTaskID = strings.TrimSpace(current.ID)
	output.CurrentResidentTaskName = strings.TrimSpace(current.Name)
	output.CurrentResidentTaskStatus = current.Status
}

// selectCurrentResidentTask 封装当前模块的业务处理逻辑。
func selectCurrentResidentTask(items []agentdomain.Task) (agentdomain.Task, bool) {
	bestIndex := -1
	bestScore := -1
	for idx, item := range items {
		if item.TaskMode != agentdomain.TaskModeResident {
			continue
		}
		score := residentTaskPriority(item.Status)
		if score > bestScore {
			bestScore = score
			bestIndex = idx
			continue
		}
		if score == bestScore && bestIndex >= 0 && item.UpdatedAt.After(items[bestIndex].UpdatedAt) {
			bestIndex = idx
		}
	}
	if bestIndex < 0 {
		return agentdomain.Task{}, false
	}
	return items[bestIndex], true
}

// residentTaskPriority 封装当前模块的业务处理逻辑。
func residentTaskPriority(status agentdomain.TaskStatus) int {
	switch status {
	case agentdomain.TaskStatusRunning:
		return 4
	case agentdomain.TaskStatusClaimed:
		return 3
	case agentdomain.TaskStatusPending:
		return 2
	case agentdomain.TaskStatusCancelled:
		return 1
	default:
		return 0
	}
}

// resolveAgentRuntimeState 解析上下文数据，得到后续流程需要的结果。
func resolveAgentRuntimeState(item agentdomain.Instance, now time.Time, offlineAfter time.Duration) agentdomain.RuntimeState {
	switch item.Status {
	case agentdomain.StatusDisabled:
		return agentdomain.RuntimeStateDisabled
	case agentdomain.StatusMaintenance:
		return agentdomain.RuntimeStateMaintenance
	}
	if item.LastHeartbeatAt.IsZero() || now.Sub(item.LastHeartbeatAt) > offlineAfter {
		return agentdomain.RuntimeStateOffline
	}
	if strings.TrimSpace(item.CurrentTaskID) != "" {
		return agentdomain.RuntimeStateBusy
	}
	return agentdomain.RuntimeStateOnline
}

// normalizeAgentTags 标准化输入值，保证后续逻辑使用统一格式。
func normalizeAgentTags(items []string) []string {
	set := make(map[string]struct{})
	result := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, exists := set[value]; exists {
			continue
		}
		set[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// normalizeAgentCodeSegment 标准化输入值，保证后续逻辑使用统一格式。
func normalizeAgentCodeSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	builder := strings.Builder{}
	lastDash := false
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteRune('-')
			lastDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	return result
}

// normalizeLastTaskStatus 标准化输入值，保证后续逻辑使用统一格式。
func normalizeLastTaskStatus(value agentdomain.LastTaskStatus, fallback agentdomain.LastTaskStatus) agentdomain.LastTaskStatus {
	if value.Valid() {
		if value == "" {
			if fallback.Valid() && fallback != "" {
				return fallback
			}
			return agentdomain.LastTaskStatusUnknown
		}
		return value
	}
	if fallback.Valid() && fallback != "" {
		return fallback
	}
	return agentdomain.LastTaskStatusUnknown
}

// firstNonEmptyAgentString 封装当前模块的业务处理逻辑。
func firstNonEmptyAgentString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// generateAgentToken 封装当前模块的业务处理逻辑。
func generateAgentToken() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return generateID("agtok")
	}
	return hex.EncodeToString(buf)
}

// subtleConstantTimeCompare 封装当前模块的业务处理逻辑。
func subtleConstantTimeCompare(left string, right string) bool {
	if len(left) != len(right) {
		return false
	}
	var diff byte
	for i := 0; i < len(left); i++ {
		diff |= left[i] ^ right[i]
	}
	return diff == 0
}

// normalizeAgentBaseURL 标准化输入值，保证后续逻辑使用统一格式。
func normalizeAgentBaseURL(baseURL string) string {
	value := strings.TrimSpace(baseURL)
	if value == "" {
		return "http://127.0.0.1:8081"
	}
	return strings.TrimRight(value, "/")
}
