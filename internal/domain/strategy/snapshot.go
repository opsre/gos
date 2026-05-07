package strategy

type StrategySnapshot struct {
	StrategyName   string                 `json:"strategy_name"`
	StrategyType   StrategyType           `json:"strategy_type"`
	StrategyEngine StrategyEngine         `json:"strategy_engine"`
	TemplateID     string                 `json:"template_id"`
	EnvCode        string                 `json:"env_code"`
	ApplicationID  string                 `json:"application_id"`
	Namespace      string                 `json:"namespace"`
	WorkloadName   string                 `json:"workload_name"`
	Config         ResolvedStrategyConfig `json:"config"`
}

type ResolvedStrategyConfig struct {
	MaxSurge       string           `json:"max_surge,omitempty"`
	MaxUnavailable string           `json:"max_unavailable,omitempty"`
	TimeoutSeconds int              `json:"timeout_seconds,omitempty"`
	CanarySteps    []CanaryStep     `json:"canary_steps,omitempty"`
	BlueGreen      *BlueGreenConfig `json:"blue_green,omitempty"`
	AutoPromotion  bool             `json:"auto_promotion,omitempty"`
	AutoRollback   bool             `json:"auto_rollback,omitempty"`
}

type CanaryStep struct {
	SetWeight     int    `json:"set_weight"`
	PauseSeconds  int    `json:"pause_seconds,omitempty"`
	PauseDuration string `json:"pause_duration,omitempty"`
}

type BlueGreenConfig struct {
	ActiveService        string `json:"active_service"`
	PreviewService       string `json:"preview_service"`
	AutoPromotionSeconds int    `json:"auto_promotion_seconds,omitempty"`
}
