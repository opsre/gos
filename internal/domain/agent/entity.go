package agent

import "time"

type Status string

const (
	StatusActive      Status = "active"
	StatusDisabled    Status = "disabled"
	StatusMaintenance Status = "maintenance"
)

// Valid 封装当前模块的业务处理逻辑。
func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusDisabled, StatusMaintenance:
		return true
	default:
		return false
	}
}

type RuntimeState string

const (
	RuntimeStateOnline      RuntimeState = "online"
	RuntimeStateOffline     RuntimeState = "offline"
	RuntimeStateBusy        RuntimeState = "busy"
	RuntimeStateDisabled    RuntimeState = "disabled"
	RuntimeStateMaintenance RuntimeState = "maintenance"
)

// Valid 封装当前模块的业务处理逻辑。
func (s RuntimeState) Valid() bool {
	switch s {
	case RuntimeStateOnline, RuntimeStateOffline, RuntimeStateBusy, RuntimeStateDisabled, RuntimeStateMaintenance:
		return true
	default:
		return false
	}
}

type LastTaskStatus string

const (
	LastTaskStatusUnknown   LastTaskStatus = "unknown"
	LastTaskStatusRunning   LastTaskStatus = "running"
	LastTaskStatusSuccess   LastTaskStatus = "success"
	LastTaskStatusFailed    LastTaskStatus = "failed"
	LastTaskStatusCancelled LastTaskStatus = "cancelled"
)

// Valid 封装当前模块的业务处理逻辑。
func (s LastTaskStatus) Valid() bool {
	switch s {
	case "", LastTaskStatusUnknown, LastTaskStatusRunning, LastTaskStatusSuccess, LastTaskStatusFailed, LastTaskStatusCancelled:
		return true
	default:
		return false
	}
}

type TaskStatus string

const (
	TaskStatusDraft     TaskStatus = "draft"
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusClaimed   TaskStatus = "claimed"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusSuccess   TaskStatus = "success"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// Valid 封装当前模块的业务处理逻辑。
func (s TaskStatus) Valid() bool {
	switch s {
	case TaskStatusDraft, TaskStatusPending, TaskStatusQueued, TaskStatusClaimed, TaskStatusRunning, TaskStatusSuccess, TaskStatusFailed, TaskStatusCancelled:
		return true
	default:
		return false
	}
}

type TaskMode string

const (
	TaskModeTemporary TaskMode = "temporary"
	TaskModeResident  TaskMode = "resident"
)

// Valid 封装当前模块的业务处理逻辑。
func (m TaskMode) Valid() bool {
	switch m {
	case TaskModeTemporary, TaskModeResident:
		return true
	default:
		return false
	}
}

type TaskType string

const (
	TaskTypeShellScript      TaskType = "shell_task"
	TaskTypeScriptFile       TaskType = "script_file_task"
	TaskTypeFileDistribution TaskType = "file_distribution_task"
)

// Valid 封装当前模块的业务处理逻辑。
func (t TaskType) Valid() bool {
	switch t {
	case TaskTypeShellScript, TaskTypeScriptFile, TaskTypeFileDistribution:
		return true
	default:
		return false
	}
}

// ScriptLibrarySupported 封装当前模块的业务处理逻辑。
func (t TaskType) ScriptLibrarySupported() bool {
	switch t {
	case TaskTypeShellScript, TaskTypeScriptFile:
		return true
	default:
		return false
	}
}

type Instance struct {
	ID                 string
	MachineID          string
	AgentCode          string
	Name               string
	EnvironmentCode    string
	WorkDir            string
	Token              string
	Tags               []string
	Hostname           string
	HostIP             string
	AgentVersion       string
	OS                 string
	Arch               string
	Status             Status
	LastHeartbeatAt    time.Time
	CurrentTaskID      string
	CurrentTaskName    string
	CurrentTaskType    string
	CurrentTaskStarted *time.Time
	LastTaskStatus     LastTaskStatus
	LastTaskSummary    string
	LastTaskFinishedAt *time.Time
	Remark             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type ListFilter struct {
	Keyword      string
	Status       Status
	RuntimeState RuntimeState
	Page         int
	PageSize     int
}

type HeartbeatPayload struct {
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
	LastTaskStatus     LastTaskStatus
	LastTaskSummary    string
	LastTaskFinishedAt *time.Time
}

type RuntimeTaskPayload struct {
	CurrentTaskID      string
	CurrentTaskName    string
	CurrentTaskType    string
	CurrentTaskStarted *time.Time
	LastTaskStatus     LastTaskStatus
	LastTaskSummary    string
	LastTaskFinishedAt *time.Time
}

type Task struct {
	ID              string
	AgentID         string
	AgentCode       string
	TargetAgentIDs  []string
	SourceTaskID    string
	DispatchBatchID string
	Name            string
	TaskMode        TaskMode
	TaskType        string
	ShellType       string
	WorkDir         string
	ScriptID        string
	ScriptName      string
	ScriptPath      string
	ScriptText      string
	Variables       map[string]string
	TimeoutSec      int
	Status          TaskStatus
	ClaimedAt       *time.Time
	StartedAt       *time.Time
	FinishedAt      *time.Time
	ExitCode        int
	StdoutText      string
	StderrText      string
	FailureReason   string
	RunCount        int
	SuccessCount    int
	FailureCount    int
	LastRunStatus   TaskStatus
	LastRunSummary  string
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type TaskListFilter struct {
	AgentID         string
	ScriptID        string
	SourceTaskID    string
	DispatchBatchID string
	Statuses        []TaskStatus
	Page            int
	PageSize        int
}

type Script struct {
	ID          string
	Name        string
	Description string
	TaskType    string
	ShellType   string
	ScriptPath  string
	ScriptText  string
	CreatedBy   string
	UpdatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ScriptListFilter struct {
	Keyword  string
	TaskType TaskType
	Page     int
	PageSize int
}
