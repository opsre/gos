package strategy

import (
	"strings"
	"time"
)

type StrategyEngine string

const (
	StrategyEngineK8sNative   StrategyEngine = "k8s_native"
	StrategyEngineArgoRollouts StrategyEngine = "argo_rollouts"
)

func (s StrategyEngine) Valid() bool {
	switch s {
	case StrategyEngineK8sNative, StrategyEngineArgoRollouts:
		return true
	default:
		return false
	}
}

type StrategyType string

const (
	StrategyTypeRollingUpdate StrategyType = "rolling_update"
	StrategyTypeCanary        StrategyType = "canary"
	StrategyTypeBlueGreen     StrategyType = "blue_green"
)

func (s StrategyType) Valid() bool {
	switch s {
	case StrategyTypeRollingUpdate, StrategyTypeCanary, StrategyTypeBlueGreen:
		return true
	default:
		return false
	}
}

type TemplateStatus string

const (
	TemplateStatusActive   TemplateStatus = "active"
	TemplateStatusInactive TemplateStatus = "inactive"
)

func (s TemplateStatus) Valid() bool {
	switch s {
	case TemplateStatusActive, TemplateStatusInactive:
		return true
	default:
		return false
	}
}

type ReleaseStrategyTemplate struct {
	ID             string
	Name           string
	StrategyEngine StrategyEngine
	StrategyType   StrategyType
	StrategyConfig string
	Description    string
	Status         TemplateStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (t ReleaseStrategyTemplate) Clean() ReleaseStrategyTemplate {
	t.Name = strings.TrimSpace(t.Name)
	t.StrategyConfig = strings.TrimSpace(t.StrategyConfig)
	t.Description = strings.TrimSpace(t.Description)
	return t
}

type ApplicationEnvRuntimeBinding struct {
	ID               string
	ApplicationID    string
	EnvCode          string
	K8sClusterRefID  string
	Namespace        string
	WorkloadName     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (b ApplicationEnvRuntimeBinding) Clean() ApplicationEnvRuntimeBinding {
	b.ApplicationID = strings.TrimSpace(b.ApplicationID)
	b.EnvCode = strings.TrimSpace(b.EnvCode)
	b.K8sClusterRefID = strings.TrimSpace(b.K8sClusterRefID)
	b.Namespace = strings.TrimSpace(b.Namespace)
	b.WorkloadName = strings.TrimSpace(b.WorkloadName)
	return b
}

type StrategyTemplateConfig struct {
	MaxSurge        string           `json:"max_surge,omitempty"`
	MaxUnavailable  string           `json:"max_unavailable,omitempty"`
	TimeoutSeconds  int              `json:"timeout_seconds,omitempty"`
	CanarySteps     []CanaryStep     `json:"canary_steps,omitempty"`
	BlueGreen       *BlueGreenConfig `json:"blue_green,omitempty"`
	AutoPromotion   bool             `json:"auto_promotion,omitempty"`
	AutoRollback    bool             `json:"auto_rollback,omitempty"`
	RollbackEnabled bool             `json:"rollback_enabled,omitempty"`
}

type ApplicationEnvStrategyBinding struct {
	ID                 string
	ApplicationID      string
	EnvCode            string
	StrategyTemplateID string
	OverridesConfig    string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (b ApplicationEnvStrategyBinding) Clean() ApplicationEnvStrategyBinding {
	b.ApplicationID = strings.TrimSpace(b.ApplicationID)
	b.EnvCode = strings.TrimSpace(b.EnvCode)
	b.StrategyTemplateID = strings.TrimSpace(b.StrategyTemplateID)
	b.OverridesConfig = strings.TrimSpace(b.OverridesConfig)
	return b
}
