package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	appdomain "gos/internal/domain/application"
	domain "gos/internal/domain/pipeline"
)

type PipelineBindingManager struct {
	repo    domain.Repository
	appRepo appdomain.Repository
	now     func() time.Time
}

type CreatePipelineBindingInput struct {
	BindingType domain.BindingType
	Provider    domain.Provider
	PipelineID  string
	ExternalRef string
	TriggerMode domain.TriggerMode
	Status      domain.Status
}

// NewPipelineBindingManager 创建并返回对应组件实例。
func NewPipelineBindingManager(repo domain.Repository, appRepo appdomain.Repository) *PipelineBindingManager {
	return &PipelineBindingManager{
		repo:    repo,
		appRepo: appRepo,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// Create 创建业务资源并返回处理结果。
func (uc *PipelineBindingManager) Create(ctx context.Context, applicationID string, input CreatePipelineBindingInput) (domain.PipelineBinding, error) {
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return domain.PipelineBinding{}, ErrInvalidID
	}

	app, err := uc.appRepo.GetByID(ctx, applicationID)
	if err != nil {
		return domain.PipelineBinding{}, err
	}

	if !input.TriggerMode.Valid() {
		return domain.PipelineBinding{}, ErrInvalidTriggerMode
	}

	status := input.Status
	if status == "" {
		status = domain.StatusActive
	}
	if !status.Valid() {
		return domain.PipelineBinding{}, ErrInvalidStatus
	}
	provider, pipelineID, externalRef, err := normalizeBindingTarget(
		input.BindingType,
		input.Provider,
		input.PipelineID,
		input.ExternalRef,
	)
	if err != nil {
		return domain.PipelineBinding{}, err
	}
	duplicated, err := uc.bindingTypeExists(ctx, applicationID, input.BindingType)
	if err != nil {
		return domain.PipelineBinding{}, err
	}
	if duplicated {
		return domain.PipelineBinding{}, domain.ErrBindingDuplicated
	}

	if pipelineID != "" {
		if err := uc.ensurePipelineProvider(ctx, pipelineID, provider); err != nil {
			return domain.PipelineBinding{}, err
		}
	}

	name, err := uc.defaultBindingName(ctx, provider, pipelineID, externalRef)
	if err != nil {
		return domain.PipelineBinding{}, err
	}
	if name == "" {
		name = string(input.BindingType)
	}

	now := uc.now()
	binding := domain.PipelineBinding{
		ID:              generateID("pb"),
		Name:            name,
		ApplicationID:   applicationID,
		ApplicationName: app.Name,
		BindingType:     input.BindingType,
		Provider:        provider,
		PipelineID:      pipelineID,
		ExternalRef:     externalRef,
		TriggerMode:     input.TriggerMode,
		Status:          status,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := uc.repo.CreateBinding(ctx, binding); err != nil {
		return domain.PipelineBinding{}, err
	}
	return uc.repo.GetBindingByID(ctx, binding.ID)
}

// ListByApplication 查询并返回列表数据。
func (uc *PipelineBindingManager) ListByApplication(ctx context.Context, filter domain.BindingListFilter) ([]domain.PipelineBinding, int64, error) {
	const (
		defaultPage     = 1
		defaultPageSize = 20
		maxPageSize     = 100
	)
	filter.ApplicationID = strings.TrimSpace(filter.ApplicationID)
	if filter.ApplicationID == "" {
		return nil, 0, ErrInvalidID
	}
	if filter.BindingType != "" && !filter.BindingType.Valid() {
		return nil, 0, ErrInvalidBindingType
	}
	if filter.Provider != "" && !filter.Provider.Valid() {
		return nil, 0, ErrInvalidProvider
	}
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, 0, ErrInvalidStatus
	}
	if filter.Page <= 0 {
		filter.Page = defaultPage
	}
	if filter.PageSize <= 0 {
		filter.PageSize = defaultPageSize
	}
	if filter.PageSize > maxPageSize {
		filter.PageSize = maxPageSize
	}

	if _, err := uc.appRepo.GetByID(ctx, filter.ApplicationID); err != nil {
		return nil, 0, err
	}
	return uc.repo.ListBindingsByApplication(ctx, filter)
}

// GetByID 查询并返回指定资源数据。
func (uc *PipelineBindingManager) GetByID(ctx context.Context, id string) (domain.PipelineBinding, error) {
	if strings.TrimSpace(id) == "" {
		return domain.PipelineBinding{}, ErrInvalidID
	}
	return uc.repo.GetBindingByID(ctx, id)
}

// Update 更新业务资源并返回处理结果。
func (uc *PipelineBindingManager) Update(ctx context.Context, id string, input domain.BindingUpdateInput) (domain.PipelineBinding, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.PipelineBinding{}, ErrInvalidID
	}

	existing, err := uc.repo.GetBindingByID(ctx, id)
	if err != nil {
		return domain.PipelineBinding{}, err
	}
	if _, err := uc.appRepo.GetByID(ctx, existing.ApplicationID); err != nil {
		return domain.PipelineBinding{}, err
	}

	bindingType := existing.BindingType
	if bindingType == "" {
		bindingType = domain.BindingTypeCI
	}

	triggerMode := input.TriggerMode
	if triggerMode == "" {
		triggerMode = existing.TriggerMode
	}
	if !triggerMode.Valid() {
		return domain.PipelineBinding{}, ErrInvalidTriggerMode
	}

	status := input.Status
	if status == "" {
		status = existing.Status
	}
	if !status.Valid() {
		return domain.PipelineBinding{}, ErrInvalidStatus
	}

	providerInput := input.Provider
	if providerInput == "" {
		providerInput = existing.Provider
	}
	pipelineIDInput := input.PipelineID
	if strings.TrimSpace(pipelineIDInput) == "" {
		pipelineIDInput = existing.PipelineID
	}
	externalRefInput := input.ExternalRef
	if strings.TrimSpace(externalRefInput) == "" {
		externalRefInput = existing.ExternalRef
	}
	provider, pipelineID, externalRef, err := normalizeBindingTarget(
		bindingType,
		providerInput,
		pipelineIDInput,
		externalRefInput,
	)
	if err != nil {
		return domain.PipelineBinding{}, err
	}
	if pipelineID != "" {
		if err := uc.ensurePipelineProvider(ctx, pipelineID, provider); err != nil {
			return domain.PipelineBinding{}, err
		}
	}

	name, err := uc.defaultBindingName(ctx, provider, pipelineID, externalRef)
	if err != nil {
		return domain.PipelineBinding{}, err
	}
	if name == "" {
		name = string(bindingType)
	}

	return uc.repo.UpdateBinding(ctx, id, domain.BindingUpdateInput{
		Name:        name,
		Provider:    provider,
		PipelineID:  pipelineID,
		ExternalRef: externalRef,
		TriggerMode: triggerMode,
		Status:      status,
	}, uc.now())
}

// Delete 删除业务资源并返回处理结果。
func (uc *PipelineBindingManager) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrInvalidID
	}
	return uc.repo.DeleteBinding(ctx, id)
}

// ensurePipelineProvider 校验前置条件，不满足时写入对应错误响应。
func (uc *PipelineBindingManager) ensurePipelineProvider(ctx context.Context, pipelineID string, expectProvider domain.Provider) error {
	pipeline, err := uc.repo.GetPipelineByID(ctx, pipelineID)
	if err != nil {
		return err
	}
	if err := ensureActivePipelineRecord(pipeline, "所选管线"); err != nil {
		return err
	}
	if pipeline.Provider != expectProvider {
		return fmt.Errorf("%w: pipeline provider mismatch", ErrInvalidInput)
	}
	return nil
}

// normalizeBindingTarget 标准化输入值，保证后续逻辑使用统一格式。
func normalizeBindingTarget(
	bindingType domain.BindingType,
	provider domain.Provider,
	pipelineID string,
	externalRef string,
) (domain.Provider, string, string, error) {
	if !bindingType.Valid() {
		return "", "", "", ErrInvalidBindingType
	}

	pipelineID = strings.TrimSpace(pipelineID)
	externalRef = strings.TrimSpace(externalRef)

	switch bindingType {
	case domain.BindingTypeCI:
		if provider != "" && provider != domain.ProviderJenkins {
			return "", "", "", fmt.Errorf("%w: ci binding provider must be jenkins", ErrInvalidInput)
		}
		if pipelineID == "" {
			return "", "", "", fmt.Errorf("%w: ci binding requires pipeline_id", ErrInvalidInput)
		}
		if externalRef != "" {
			return "", "", "", fmt.Errorf("%w: ci binding does not support external_ref", ErrInvalidInput)
		}
		return domain.ProviderJenkins, pipelineID, "", nil

	case domain.BindingTypeCD:
		if provider == "" {
			provider = domain.ProviderJenkins
		}
		if !provider.Valid() {
			return "", "", "", ErrInvalidProvider
		}

		switch provider {
		case domain.ProviderJenkins:
			if pipelineID == "" {
				return "", "", "", fmt.Errorf("%w: cd jenkins binding requires pipeline_id", ErrInvalidInput)
			}
			if externalRef != "" {
				return "", "", "", fmt.Errorf("%w: cd jenkins binding does not support external_ref", ErrInvalidInput)
			}
			return provider, pipelineID, "", nil
		case domain.ProviderArgoCD:
			return "", "", "", fmt.Errorf("%w: cd argocd has moved to release template configuration", ErrInvalidInput)
		default:
			return "", "", "", ErrInvalidProvider
		}
	default:
		return "", "", "", ErrInvalidBindingType
	}
}

// shouldAutoFillArgoCDExternalRef 封装当前模块的业务处理逻辑。
func shouldAutoFillArgoCDExternalRef(bindingType domain.BindingType, provider domain.Provider) bool {
	if bindingType != domain.BindingTypeCD {
		return false
	}
	return provider == "" || provider == domain.ProviderArgoCD
}

// deriveArgoCDExternalRef 封装当前模块的业务处理逻辑。
func deriveArgoCDExternalRef(app appdomain.Application) (string, error) {
	appKey := strings.TrimSpace(app.Key)
	if appKey == "" {
		return "", fmt.Errorf("%w: application app_key is required when cd provider is argocd", ErrInvalidInput)
	}
	return "apps/" + appKey, nil
}

// defaultBindingName 封装当前模块的业务处理逻辑。
func (uc *PipelineBindingManager) defaultBindingName(
	ctx context.Context,
	provider domain.Provider,
	pipelineID string,
	externalRef string,
) (string, error) {
	switch provider {
	case domain.ProviderJenkins:
		if strings.TrimSpace(pipelineID) == "" {
			return "", nil
		}
		pipeline, err := uc.repo.GetPipelineByID(ctx, pipelineID)
		if err != nil {
			return "", err
		}
		if name := strings.TrimSpace(pipeline.JobName); name != "" {
			return name, nil
		}
		if fullName := strings.TrimSpace(pipeline.JobFullName); fullName != "" {
			return fullName, nil
		}
		return strings.TrimSpace(pipelineID), nil
	case domain.ProviderArgoCD:
		value := strings.Trim(strings.TrimSpace(externalRef), "/")
		if value == "" {
			return "", nil
		}
		if idx := strings.LastIndex(value, "/"); idx >= 0 && idx < len(value)-1 {
			value = value[idx+1:]
		}
		return value, nil
	default:
		if ref := strings.TrimSpace(externalRef); ref != "" {
			return ref, nil
		}
		return strings.TrimSpace(pipelineID), nil
	}
}

// bindingTypeExists 封装当前模块的业务处理逻辑。
func (uc *PipelineBindingManager) bindingTypeExists(ctx context.Context, applicationID string, bindingType domain.BindingType) (bool, error) {
	items, total, err := uc.repo.ListBindingsByApplication(ctx, domain.BindingListFilter{
		ApplicationID: applicationID,
		BindingType:   bindingType,
		Page:          1,
		PageSize:      1,
	})
	if err != nil {
		return false, err
	}
	return total > 0 || len(items) > 0, nil
}
