package strategy

import "context"

type TemplateListFilter struct {
	StrategyEngine StrategyEngine
	StrategyType   StrategyType
	Status         TemplateStatus
	Name           string
	Page           int
	PageSize       int
}

type TemplateUpdateInput struct {
	Name           string
	StrategyEngine StrategyEngine
	StrategyType   StrategyType
	StrategyConfig string
	Description    string
	Status         TemplateStatus
}

type RuntimeBindingListFilter struct {
	ApplicationID string
	EnvCode       string
	Page          int
	PageSize      int
}

type RuntimeBindingUpdateInput struct {
	K8sClusterRefID string
	Namespace       string
	WorkloadName    string
}

type StrategyBindingListFilter struct {
	ApplicationID string
	EnvCode       string
	Page          int
	PageSize      int
}

type StrategyBindingUpdateInput struct {
	StrategyTemplateID string
	OverridesConfig    string
}

type Repository interface {
	InitSchema(ctx context.Context) error

	CreateTemplate(ctx context.Context, item ReleaseStrategyTemplate) error
	GetTemplateByID(ctx context.Context, id string) (ReleaseStrategyTemplate, error)
	ListTemplates(ctx context.Context, filter TemplateListFilter) ([]ReleaseStrategyTemplate, int64, error)
	UpdateTemplate(ctx context.Context, id string, input TemplateUpdateInput) (ReleaseStrategyTemplate, error)
	DeleteTemplate(ctx context.Context, id string) error

	CreateRuntimeBinding(ctx context.Context, item ApplicationEnvRuntimeBinding) error
	GetRuntimeBindingByID(ctx context.Context, id string) (ApplicationEnvRuntimeBinding, error)
	GetRuntimeBindingByAppEnv(ctx context.Context, applicationID, envCode string) (ApplicationEnvRuntimeBinding, error)
	ListRuntimeBindings(ctx context.Context, filter RuntimeBindingListFilter) ([]ApplicationEnvRuntimeBinding, int64, error)
	UpdateRuntimeBinding(ctx context.Context, id string, input RuntimeBindingUpdateInput) (ApplicationEnvRuntimeBinding, error)
	DeleteRuntimeBinding(ctx context.Context, id string) error

	CreateStrategyBinding(ctx context.Context, item ApplicationEnvStrategyBinding) error
	GetStrategyBindingByID(ctx context.Context, id string) (ApplicationEnvStrategyBinding, error)
	GetStrategyBindingByAppEnv(ctx context.Context, applicationID, envCode string) (ApplicationEnvStrategyBinding, error)
	ListStrategyBindings(ctx context.Context, filter StrategyBindingListFilter) ([]ApplicationEnvStrategyBinding, int64, error)
	UpdateStrategyBinding(ctx context.Context, id string, input StrategyBindingUpdateInput) (ApplicationEnvStrategyBinding, error)
	DeleteStrategyBinding(ctx context.Context, id string) error
}
