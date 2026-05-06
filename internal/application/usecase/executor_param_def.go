package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	appdomain "gos/internal/domain/application"
	domain "gos/internal/domain/executorparam"
	pipelinedomain "gos/internal/domain/pipeline"
	platformparamdomain "gos/internal/domain/platformparam"
)

type ExecutorParamDefManager struct {
	repo         domain.Repository
	appRepo      appdomain.Repository
	pipelineRepo pipelinedomain.Repository
	platformRepo platformparamdomain.Repository
	now          func() time.Time
}

// NewExecutorParamDefManager 负责“执行器参数定义”的查询与平台字段映射维护。
//
// 这里继续复用 pipelineRepo，是因为当前参数定义仍然依赖应用绑定到具体执行记录；
// 但对上层业务来说，这里暴露的已经是更抽象的执行器参数模型，而不是某个 Jenkins 专属概念。
func NewExecutorParamDefManager(
	repo domain.Repository,
	appRepo appdomain.Repository,
	pipelineRepo pipelinedomain.Repository,
	platformRepo platformparamdomain.Repository,
) *ExecutorParamDefManager {
	return &ExecutorParamDefManager{
		repo:         repo,
		appRepo:      appRepo,
		pipelineRepo: pipelineRepo,
		platformRepo: platformRepo,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// ListByPipeline 查询并返回列表数据。
func (uc *ExecutorParamDefManager) ListByPipeline(ctx context.Context, filter domain.ListFilter) ([]domain.ExecutorParamDef, int64, error) {
	const (
		defaultPage     = 1
		defaultPageSize = 20
		maxPageSize     = 100
	)

	filter.PipelineID = strings.TrimSpace(filter.PipelineID)
	if filter.PipelineID == "" {
		return nil, 0, ErrInvalidID
	}
	pipeline, err := uc.pipelineRepo.GetPipelineByID(ctx, filter.PipelineID)
	if err != nil {
		return nil, 0, err
	}
	if err := ensureActivePipelineRecord(pipeline, "当前管线"); err != nil {
		return nil, 0, err
	}
	if filter.ExecutorType != "" && !filter.ExecutorType.Valid() {
		return nil, 0, ErrInvalidExecutorType
	}
	filter.ParamKey = strings.TrimSpace(filter.ParamKey)
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
	return uc.repo.ListByPipeline(ctx, filter)
}

// ListByApplication 查询并返回列表数据。
func (uc *ExecutorParamDefManager) ListByApplication(
	ctx context.Context,
	applicationID string,
	bindingType pipelinedomain.BindingType,
	bindingID string,
	filter domain.ListFilter,
) ([]domain.ExecutorParamDef, int64, error) {
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return nil, 0, ErrInvalidID
	}
	application, err := uc.appRepo.GetByID(ctx, applicationID)
	if err != nil {
		return nil, 0, err
	}

	if bindingType == "" {
		bindingType = pipelinedomain.BindingTypeCI
	}
	if !bindingType.Valid() {
		return nil, 0, ErrInvalidBindingType
	}

	bindingID = strings.TrimSpace(bindingID)
	var binding pipelinedomain.PipelineBinding
	if bindingID != "" {
		binding, err = uc.pipelineRepo.GetBindingByID(ctx, bindingID)
		if err != nil {
			return nil, 0, err
		}
		if strings.TrimSpace(binding.ApplicationID) != applicationID {
			return nil, 0, fmt.Errorf("%w: pipeline binding does not belong to current application", ErrInvalidInput)
		}
		if binding.BindingType != bindingType {
			return nil, 0, fmt.Errorf("%w: pipeline binding type mismatch", ErrInvalidInput)
		}
		if binding.Status != pipelinedomain.StatusActive {
			return nil, 0, fmt.Errorf("%w: jenkins pipeline binding is inactive", pipelinedomain.ErrBindingNotFound)
		}
	} else {
		bindings, total, err := uc.pipelineRepo.ListBindingsByApplication(ctx, pipelinedomain.BindingListFilter{
			ApplicationID: applicationID,
			BindingType:   bindingType,
			Provider:      pipelinedomain.ProviderJenkins,
			Page:          1,
			PageSize:      1,
		})
		if err != nil {
			return nil, 0, err
		}
		if total == 0 || len(bindings) == 0 {
			return nil, 0, fmt.Errorf("%w: jenkins pipeline binding not found", pipelinedomain.ErrBindingNotFound)
		}
		binding = bindings[0]
	}
	if binding.Provider != pipelinedomain.ProviderJenkins {
		return nil, 0, fmt.Errorf("%w: bound pipeline provider is not jenkins", ErrInvalidInput)
	}
	if strings.TrimSpace(binding.PipelineID) == "" {
		return nil, 0, fmt.Errorf("%w: bound pipeline id is empty", ErrInvalidInput)
	}
	pipeline, err := uc.pipelineRepo.GetPipelineByID(ctx, binding.PipelineID)
	if err != nil {
		return nil, 0, err
	}
	if err := ensureActivePipelineRecord(pipeline, "绑定管线"); err != nil {
		return nil, 0, err
	}

	filter.PipelineID = binding.PipelineID
	filter.ExecutorType = domain.ExecutorTypeJenkins
	items, total, err := uc.ListByPipeline(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	for index := range items {
		items[index].ApplicationID = application.ID
		items[index].ApplicationName = application.Name
		items[index].ApplicationKey = application.Key
		items[index].BindingType = string(binding.BindingType)
		items[index].PipelineName = strings.TrimSpace(binding.Name)
		if items[index].PipelineName == "" {
			items[index].PipelineName = binding.PipelineID
		}
	}
	return items, total, nil
}

// ListByApplications 查询并返回列表数据。
func (uc *ExecutorParamDefManager) ListByApplications(
	ctx context.Context,
	filter domain.ApplicationListFilter,
) ([]domain.ExecutorParamDef, int64, error) {
	const (
		defaultPage     = 1
		defaultPageSize = 20
		maxPageSize     = 100
	)

	filter.Keyword = strings.TrimSpace(filter.Keyword)
	if filter.BindingType == "" {
		filter.BindingType = pipelinedomain.BindingTypeCI
	}
	if !filter.BindingType.Valid() {
		return nil, 0, ErrInvalidBindingType
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

	applicationIDs := make([]string, 0, len(filter.ApplicationIDs))
	seen := make(map[string]struct{}, len(filter.ApplicationIDs))
	for _, item := range filter.ApplicationIDs {
		applicationID := strings.TrimSpace(item)
		if applicationID == "" {
			continue
		}
		if _, exists := seen[applicationID]; exists {
			continue
		}
		seen[applicationID] = struct{}{}
		applicationIDs = append(applicationIDs, applicationID)
	}
	filter.ApplicationIDs = applicationIDs

	return uc.repo.ListByApplications(ctx, filter)
}

// GetByID 查询并返回指定资源数据。
func (uc *ExecutorParamDefManager) GetByID(ctx context.Context, id string) (domain.ExecutorParamDef, error) {
	if strings.TrimSpace(id) == "" {
		return domain.ExecutorParamDef{}, ErrInvalidID
	}
	return uc.repo.GetByID(ctx, id)
}

// UpdateParamKey 更新业务资源并返回处理结果。
func (uc *ExecutorParamDefManager) UpdateParamKey(ctx context.Context, id string, paramKey string) (domain.ExecutorParamDef, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.ExecutorParamDef{}, ErrInvalidID
	}

	if _, err := uc.repo.GetByID(ctx, id); err != nil {
		return domain.ExecutorParamDef{}, err
	}

	paramKey = strings.TrimSpace(paramKey)
	if paramKey == "" {
		return uc.repo.UpdateParamKey(ctx, id, "", uc.now())
	}

	normalized, err := normalizePlatformParamKey(paramKey)
	if err != nil {
		return domain.ExecutorParamDef{}, err
	}

	platformParam, err := uc.platformRepo.GetByParamKey(ctx, normalized)
	if err != nil {
		return domain.ExecutorParamDef{}, err
	}
	if platformParam.Status != platformparamdomain.StatusEnabled {
		return domain.ExecutorParamDef{}, fmt.Errorf("%w: platform param dict is disabled", ErrInvalidInput)
	}
	if platformParam.CDSelfFill {
		return domain.ExecutorParamDef{}, fmt.Errorf("%w: cd 自填字段不能映射到执行器参数", ErrInvalidInput)
	}

	return uc.repo.UpdateParamKey(ctx, id, normalized, uc.now())
}
