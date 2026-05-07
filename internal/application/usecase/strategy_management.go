package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gos/internal/domain/strategy"
)

type StrategyTemplateManager struct {
	repo strategy.Repository
	now  func() time.Time
}

func NewStrategyTemplateManager(repo strategy.Repository) *StrategyTemplateManager {
	return &StrategyTemplateManager{repo: repo, now: time.Now}
}

type CreateTemplateInput struct {
	Name           string `json:"name"`
	StrategyEngine string `json:"strategy_engine"`
	StrategyType   string `json:"strategy_type"`
	StrategyConfig string `json:"strategy_config"`
	Description    string `json:"description"`
}

func (m *StrategyTemplateManager) CreateTemplate(ctx context.Context, input CreateTemplateInput) (strategy.ReleaseStrategyTemplate, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return strategy.ReleaseStrategyTemplate{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}

	engine := strategy.StrategyEngine(strings.TrimSpace(input.StrategyEngine))
	if engine == "" {
		engine = strategy.StrategyEngineK8sNative
	}
	if !engine.Valid() {
		return strategy.ReleaseStrategyTemplate{}, fmt.Errorf("%w: invalid strategy_engine %q", ErrInvalidInput, engine)
	}

	stype := strategy.StrategyType(strings.TrimSpace(input.StrategyType))
	if stype == "" {
		stype = strategy.StrategyTypeRollingUpdate
	}
	if !stype.Valid() {
		return strategy.ReleaseStrategyTemplate{}, fmt.Errorf("%w: invalid strategy_type %q", ErrInvalidInput, stype)
	}

	now := m.now().UTC()
	item := strategy.ReleaseStrategyTemplate{
		ID:             generateID("rst"),
		Name:           input.Name,
		StrategyEngine: engine,
		StrategyType:   stype,
		StrategyConfig: strings.TrimSpace(input.StrategyConfig),
		Description:    strings.TrimSpace(input.Description),
		Status:         strategy.TemplateStatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	item = item.Clean()

	if err := m.repo.CreateTemplate(ctx, item); err != nil {
		return strategy.ReleaseStrategyTemplate{}, err
	}
	return item, nil
}

func (m *StrategyTemplateManager) GetTemplate(ctx context.Context, id string) (strategy.ReleaseStrategyTemplate, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return strategy.ReleaseStrategyTemplate{}, fmt.Errorf("%w: id is required", ErrInvalidInput)
	}
	return m.repo.GetTemplateByID(ctx, id)
}

func (m *StrategyTemplateManager) ListTemplates(ctx context.Context, filter strategy.TemplateListFilter) ([]strategy.ReleaseStrategyTemplate, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	return m.repo.ListTemplates(ctx, filter)
}

type UpdateTemplateInput struct {
	Name           *string `json:"name"`
	StrategyEngine *string `json:"strategy_engine"`
	StrategyType   *string `json:"strategy_type"`
	StrategyConfig *string `json:"strategy_config"`
	Description    *string `json:"description"`
	Status         *string `json:"status"`
}

func (m *StrategyTemplateManager) UpdateTemplate(ctx context.Context, id string, input UpdateTemplateInput) (strategy.ReleaseStrategyTemplate, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return strategy.ReleaseStrategyTemplate{}, fmt.Errorf("%w: id is required", ErrInvalidInput)
	}

	existing, err := m.repo.GetTemplateByID(ctx, id)
	if err != nil {
		return strategy.ReleaseStrategyTemplate{}, err
	}

	updateInput := strategy.TemplateUpdateInput{
		Name:           existing.Name,
		StrategyEngine: existing.StrategyEngine,
		StrategyType:   existing.StrategyType,
		StrategyConfig: existing.StrategyConfig,
		Description:    existing.Description,
		Status:         existing.Status,
	}

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return strategy.ReleaseStrategyTemplate{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
		}
		updateInput.Name = name
	}
	if input.StrategyEngine != nil {
		engine := strategy.StrategyEngine(strings.TrimSpace(*input.StrategyEngine))
		if !engine.Valid() {
			return strategy.ReleaseStrategyTemplate{}, fmt.Errorf("%w: invalid strategy_engine %q", ErrInvalidInput, engine)
		}
		updateInput.StrategyEngine = engine
	}
	if input.StrategyType != nil {
		stype := strategy.StrategyType(strings.TrimSpace(*input.StrategyType))
		if !stype.Valid() {
			return strategy.ReleaseStrategyTemplate{}, fmt.Errorf("%w: invalid strategy_type %q", ErrInvalidInput, stype)
		}
		updateInput.StrategyType = stype
	}
	if input.StrategyConfig != nil {
		updateInput.StrategyConfig = strings.TrimSpace(*input.StrategyConfig)
	}
	if input.Description != nil {
		updateInput.Description = strings.TrimSpace(*input.Description)
	}
	if input.Status != nil {
		status := strategy.TemplateStatus(strings.TrimSpace(*input.Status))
		if !status.Valid() {
			return strategy.ReleaseStrategyTemplate{}, fmt.Errorf("%w: invalid status %q", ErrInvalidInput, status)
		}
		updateInput.Status = status
	}

	return m.repo.UpdateTemplate(ctx, id, updateInput)
}

func (m *StrategyTemplateManager) DeleteTemplate(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidInput)
	}
	return m.repo.DeleteTemplate(ctx, id)
}

type CreateRuntimeBindingInput struct {
	ApplicationID   string `json:"application_id"`
	EnvCode         string `json:"env_code"`
	K8sClusterRefID string `json:"k8s_cluster_ref_id"`
	Namespace       string `json:"namespace"`
	WorkloadName    string `json:"workload_name"`
}

func (m *StrategyTemplateManager) CreateRuntimeBinding(ctx context.Context, input CreateRuntimeBindingInput) (strategy.ApplicationEnvRuntimeBinding, error) {
	input.ApplicationID = strings.TrimSpace(input.ApplicationID)
	if input.ApplicationID == "" {
		return strategy.ApplicationEnvRuntimeBinding{}, fmt.Errorf("%w: application_id is required", ErrInvalidInput)
	}
	input.EnvCode = strings.TrimSpace(input.EnvCode)
	if input.EnvCode == "" {
		return strategy.ApplicationEnvRuntimeBinding{}, fmt.Errorf("%w: env_code is required", ErrInvalidInput)
	}
	input.K8sClusterRefID = strings.TrimSpace(input.K8sClusterRefID)
	if input.K8sClusterRefID == "" {
		return strategy.ApplicationEnvRuntimeBinding{}, fmt.Errorf("%w: k8s_cluster_ref_id is required", ErrInvalidInput)
	}

	ns := strings.TrimSpace(input.Namespace)
	if ns == "" {
		ns = "default"
	}

	now := m.now().UTC()
	item := strategy.ApplicationEnvRuntimeBinding{
		ID:              generateID("aer"),
		ApplicationID:   input.ApplicationID,
		EnvCode:         input.EnvCode,
		K8sClusterRefID: input.K8sClusterRefID,
		Namespace:       ns,
		WorkloadName:    strings.TrimSpace(input.WorkloadName),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	item = item.Clean()

	if err := m.repo.CreateRuntimeBinding(ctx, item); err != nil {
		return strategy.ApplicationEnvRuntimeBinding{}, err
	}
	return item, nil
}

func (m *StrategyTemplateManager) GetRuntimeBinding(ctx context.Context, id string) (strategy.ApplicationEnvRuntimeBinding, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return strategy.ApplicationEnvRuntimeBinding{}, fmt.Errorf("%w: id is required", ErrInvalidInput)
	}
	return m.repo.GetRuntimeBindingByID(ctx, id)
}

func (m *StrategyTemplateManager) GetRuntimeBindingByAppEnv(ctx context.Context, applicationID, envCode string) (strategy.ApplicationEnvRuntimeBinding, error) {
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return strategy.ApplicationEnvRuntimeBinding{}, fmt.Errorf("%w: application_id is required", ErrInvalidInput)
	}
	envCode = strings.TrimSpace(envCode)
	if envCode == "" {
		return strategy.ApplicationEnvRuntimeBinding{}, fmt.Errorf("%w: env_code is required", ErrInvalidInput)
	}
	return m.repo.GetRuntimeBindingByAppEnv(ctx, applicationID, envCode)
}

func (m *StrategyTemplateManager) ListRuntimeBindings(ctx context.Context, filter strategy.RuntimeBindingListFilter) ([]strategy.ApplicationEnvRuntimeBinding, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	return m.repo.ListRuntimeBindings(ctx, filter)
}

type UpdateRuntimeBindingInput struct {
	K8sClusterRefID *string `json:"k8s_cluster_ref_id"`
	Namespace       *string `json:"namespace"`
	WorkloadName    *string `json:"workload_name"`
}

func (m *StrategyTemplateManager) UpdateRuntimeBinding(ctx context.Context, id string, input UpdateRuntimeBindingInput) (strategy.ApplicationEnvRuntimeBinding, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return strategy.ApplicationEnvRuntimeBinding{}, fmt.Errorf("%w: id is required", ErrInvalidInput)
	}

	existing, err := m.repo.GetRuntimeBindingByID(ctx, id)
	if err != nil {
		return strategy.ApplicationEnvRuntimeBinding{}, err
	}

	updateInput := strategy.RuntimeBindingUpdateInput{
		K8sClusterRefID: existing.K8sClusterRefID,
		Namespace:       existing.Namespace,
		WorkloadName:    existing.WorkloadName,
	}

	if input.K8sClusterRefID != nil {
		updateInput.K8sClusterRefID = strings.TrimSpace(*input.K8sClusterRefID)
	}
	if input.Namespace != nil {
		updateInput.Namespace = strings.TrimSpace(*input.Namespace)
	}
	if input.WorkloadName != nil {
		updateInput.WorkloadName = strings.TrimSpace(*input.WorkloadName)
	}

	return m.repo.UpdateRuntimeBinding(ctx, id, updateInput)
}

func (m *StrategyTemplateManager) DeleteRuntimeBinding(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidInput)
	}
	return m.repo.DeleteRuntimeBinding(ctx, id)
}

type CreateStrategyBindingInput struct {
	ApplicationID     string `json:"application_id"`
	EnvCode           string `json:"env_code"`
	StrategyTemplateID string `json:"strategy_template_id"`
	OverridesConfig   string `json:"overrides_config"`
}

func (m *StrategyTemplateManager) CreateStrategyBinding(ctx context.Context, input CreateStrategyBindingInput) (strategy.ApplicationEnvStrategyBinding, error) {
	input.ApplicationID = strings.TrimSpace(input.ApplicationID)
	if input.ApplicationID == "" {
		return strategy.ApplicationEnvStrategyBinding{}, fmt.Errorf("%w: application_id is required", ErrInvalidInput)
	}
	input.EnvCode = strings.TrimSpace(input.EnvCode)
	if input.EnvCode == "" {
		return strategy.ApplicationEnvStrategyBinding{}, fmt.Errorf("%w: env_code is required", ErrInvalidInput)
	}
	input.StrategyTemplateID = strings.TrimSpace(input.StrategyTemplateID)
	if input.StrategyTemplateID == "" {
		return strategy.ApplicationEnvStrategyBinding{}, fmt.Errorf("%w: strategy_template_id is required", ErrInvalidInput)
	}

	now := m.now().UTC()
	item := strategy.ApplicationEnvStrategyBinding{
		ID:                 generateID("aes"),
		ApplicationID:      input.ApplicationID,
		EnvCode:            input.EnvCode,
		StrategyTemplateID: input.StrategyTemplateID,
		OverridesConfig:    strings.TrimSpace(input.OverridesConfig),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	item = item.Clean()

	if err := m.repo.CreateStrategyBinding(ctx, item); err != nil {
		return strategy.ApplicationEnvStrategyBinding{}, err
	}
	return item, nil
}

func (m *StrategyTemplateManager) GetStrategyBinding(ctx context.Context, id string) (strategy.ApplicationEnvStrategyBinding, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return strategy.ApplicationEnvStrategyBinding{}, fmt.Errorf("%w: id is required", ErrInvalidInput)
	}
	return m.repo.GetStrategyBindingByID(ctx, id)
}

func (m *StrategyTemplateManager) GetStrategyBindingByAppEnv(ctx context.Context, applicationID, envCode string) (strategy.ApplicationEnvStrategyBinding, error) {
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return strategy.ApplicationEnvStrategyBinding{}, fmt.Errorf("%w: application_id is required", ErrInvalidInput)
	}
	envCode = strings.TrimSpace(envCode)
	if envCode == "" {
		return strategy.ApplicationEnvStrategyBinding{}, fmt.Errorf("%w: env_code is required", ErrInvalidInput)
	}
	return m.repo.GetStrategyBindingByAppEnv(ctx, applicationID, envCode)
}

func (m *StrategyTemplateManager) ListStrategyBindings(ctx context.Context, filter strategy.StrategyBindingListFilter) ([]strategy.ApplicationEnvStrategyBinding, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	return m.repo.ListStrategyBindings(ctx, filter)
}

type UpdateStrategyBindingInput struct {
	StrategyTemplateID *string `json:"strategy_template_id"`
	OverridesConfig   *string `json:"overrides_config"`
}

func (m *StrategyTemplateManager) UpdateStrategyBinding(ctx context.Context, id string, input UpdateStrategyBindingInput) (strategy.ApplicationEnvStrategyBinding, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return strategy.ApplicationEnvStrategyBinding{}, fmt.Errorf("%w: id is required", ErrInvalidInput)
	}

	existing, err := m.repo.GetStrategyBindingByID(ctx, id)
	if err != nil {
		return strategy.ApplicationEnvStrategyBinding{}, err
	}

	updateInput := strategy.StrategyBindingUpdateInput{
		StrategyTemplateID: existing.StrategyTemplateID,
		OverridesConfig:    existing.OverridesConfig,
	}

	if input.StrategyTemplateID != nil {
		updateInput.StrategyTemplateID = strings.TrimSpace(*input.StrategyTemplateID)
	}
	if input.OverridesConfig != nil {
		updateInput.OverridesConfig = strings.TrimSpace(*input.OverridesConfig)
	}

	return m.repo.UpdateStrategyBinding(ctx, id, updateInput)
}

func (m *StrategyTemplateManager) DeleteStrategyBinding(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidInput)
	}
	return m.repo.DeleteStrategyBinding(ctx, id)
}
