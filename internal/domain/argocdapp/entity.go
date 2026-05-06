package argocdapp

import "time"

type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

// Valid 封装当前模块的业务处理逻辑。
func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusInactive:
		return true
	default:
		return false
	}
}

type Instance struct {
	ID                 string
	InstanceCode       string
	Name               string
	BaseURL            string
	InsecureSkipVerify bool
	AuthMode           string
	Token              string
	Username           string
	Password           string
	GitOpsInstanceID   string
	GitOpsInstanceCode string
	GitOpsInstanceName string
	ClusterName        string
	DefaultNamespace   string
	Status             Status
	HealthStatus       string
	LastCheckAt        time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Remark             string
}

type InstanceListFilter struct {
	Keyword  string
	Status   Status
	Page     int
	PageSize int
}

type EnvBinding struct {
	ID                 string
	EnvCode            string
	ArgoCDInstanceID   string
	ArgoCDInstanceCode string
	ArgoCDInstanceName string
	ClusterName        string
	Priority           int
	Status             Status
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Application 是平台侧同步保存的 ArgoCD Application 快照。
//
// 这里有意将模型命名为 Application，而不是 Pipeline：
// 1. ArgoCD 的核心对象是 Application，而不是 Jenkins 风格的 Job/Pipeline；
// 2. 平台当前阶段只需要管理和观察 ArgoCD Application 元数据；
// 3. 后续若将 CD 真正接入 ArgoCD，可在此基础上继续扩展 sync 状态、事件流和 GitOps 提交信息。
type Application struct {
	ID               string
	ArgoCDInstanceID string
	InstanceCode     string
	InstanceName     string
	ClusterName      string
	InstanceBaseURL  string
	AppName          string
	Project          string
	RepoURL          string
	SourcePath       string
	TargetRevision   string
	DestServer       string
	DestNamespace    string
	SyncStatus       string
	HealthStatus     string
	OperationPhase   string
	ArgoCDURL        string
	Status           Status
	RawMeta          string
	LastSyncedAt     time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ListFilter struct {
	ArgoCDInstanceID string
	AppName          string
	Project          string
	SyncStatus       string
	HealthStatus     string
	Status           Status
	Page             int
	PageSize         int
}
