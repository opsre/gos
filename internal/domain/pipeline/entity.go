package pipeline

import "time"

type Provider string

const (
	ProviderJenkins Provider = "jenkins"
	ProviderArgoCD  Provider = "argocd"
)

// Valid 封装当前模块的业务处理逻辑。
func (p Provider) Valid() bool {
	switch p {
	case ProviderJenkins, ProviderArgoCD:
		return true
	default:
		return false
	}
}

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

type TriggerMode string

const (
	TriggerManual  TriggerMode = "manual"
	TriggerWebhook TriggerMode = "webhook"
)

// Valid 封装当前模块的业务处理逻辑。
func (m TriggerMode) Valid() bool {
	switch m {
	case TriggerManual, TriggerWebhook:
		return true
	default:
		return false
	}
}

type BindingType string

const (
	BindingTypeCI BindingType = "ci"
	BindingTypeCD BindingType = "cd"
)

// Valid 封装当前模块的业务处理逻辑。
func (t BindingType) Valid() bool {
	switch t {
	case BindingTypeCI, BindingTypeCD:
		return true
	default:
		return false
	}
}

type Pipeline struct {
	ID             string
	Provider       Provider
	JobFullName    string
	JobName        string
	JobURL         string
	Description    string
	CredentialRef  string
	DefaultBranch  string
	Status         Status
	LastVerifiedAt *time.Time
	LastSyncedAt   time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type PipelineBinding struct {
	ID              string
	Name            string
	ApplicationID   string
	ApplicationName string
	BindingType     BindingType
	Provider        Provider
	PipelineID      string
	ExternalRef     string
	TriggerMode     TriggerMode
	Status          Status
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type JenkinsJob struct {
	Name     string
	FullName string
	URL      string
}

type JenkinsPipelineScript struct {
	DefinitionClass string
	Description     string
	Script          string
	ScriptPath      string
	Sandbox         bool
	FromSCM         bool
}

type JenkinsRawPipelineConfig struct {
	Description string
	Script      string
	Sandbox     bool
}
