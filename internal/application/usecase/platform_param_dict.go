package usecase

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	pipelineparamdomain "gos/internal/domain/executorparam"
	domain "gos/internal/domain/platformparam"
)

var platformParamKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type PlatformParamDictManager struct {
	repo      domain.Repository
	paramRepo pipelineparamdomain.Repository
	now       func() time.Time
}

type CreatePlatformParamDictInput struct {
	ParamKey      string
	Name          string
	Description   string
	ParamType     domain.ParamType
	Required      bool
	GitOpsLocator bool
	CDSelfFill    bool
	Status        domain.Status
}

// NewPlatformParamDictManager 创建并返回对应组件实例。
func NewPlatformParamDictManager(repo domain.Repository, paramRepo pipelineparamdomain.Repository) *PlatformParamDictManager {
	return &PlatformParamDictManager{
		repo:      repo,
		paramRepo: paramRepo,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// Create 创建业务资源并返回处理结果。
func (uc *PlatformParamDictManager) Create(ctx context.Context, input CreatePlatformParamDictInput) (domain.PlatformParamDict, error) {
	paramKey, err := normalizePlatformParamKey(input.ParamKey)
	if err != nil {
		return domain.PlatformParamDict{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.PlatformParamDict{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if !input.ParamType.Valid() {
		return domain.PlatformParamDict{}, ErrInvalidParamType
	}

	status := input.Status
	if !status.Valid() {
		return domain.PlatformParamDict{}, ErrInvalidStatus
	}

	now := uc.now()
	item := domain.PlatformParamDict{
		ID:            generateID("ppd"),
		ParamKey:      paramKey,
		Name:          name,
		Description:   strings.TrimSpace(input.Description),
		ParamType:     input.ParamType,
		Required:      input.Required,
		GitOpsLocator: input.GitOpsLocator,
		CDSelfFill:    input.CDSelfFill,
		// Manual entries are always non-builtin. Builtin keys are seeded by the platform.
		Builtin:   false,
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := uc.repo.Create(ctx, item); err != nil {
		return domain.PlatformParamDict{}, err
	}
	return uc.repo.GetByID(ctx, item.ID)
}

// List 查询并返回列表数据。
func (uc *PlatformParamDictManager) List(ctx context.Context, filter domain.ListFilter) ([]domain.PlatformParamDict, int64, error) {
	const (
		defaultPage     = 1
		defaultPageSize = 20
		maxPageSize     = 100
	)

	filter.ParamKey = strings.TrimSpace(filter.ParamKey)
	filter.Name = strings.TrimSpace(filter.Name)
	if filter.Status != nil && !filter.Status.Valid() {
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
	return uc.repo.List(ctx, filter)
}

// GetByID 查询并返回指定资源数据。
func (uc *PlatformParamDictManager) GetByID(ctx context.Context, id string) (domain.PlatformParamDict, error) {
	if strings.TrimSpace(id) == "" {
		return domain.PlatformParamDict{}, ErrInvalidID
	}
	return uc.repo.GetByID(ctx, id)
}

// Update 更新业务资源并返回处理结果。
func (uc *PlatformParamDictManager) Update(ctx context.Context, id string, input domain.UpdateInput) (domain.PlatformParamDict, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.PlatformParamDict{}, ErrInvalidID
	}

	current, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return domain.PlatformParamDict{}, err
	}
	if current.Builtin {
		return domain.PlatformParamDict{}, ErrBuiltinProtected
	}

	paramKey, err := normalizePlatformParamKey(input.ParamKey)
	if err != nil {
		return domain.PlatformParamDict{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return domain.PlatformParamDict{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if !input.ParamType.Valid() {
		return domain.PlatformParamDict{}, ErrInvalidParamType
	}
	if !input.Status.Valid() {
		return domain.PlatformParamDict{}, ErrInvalidStatus
	}

	if current.ParamKey != paramKey {
		referenced, refErr := uc.paramRepo.CountByParamKey(ctx, current.ParamKey)
		if refErr != nil {
			return domain.PlatformParamDict{}, refErr
		}
		if referenced > 0 {
			return domain.PlatformParamDict{}, ErrReferencedConflict
		}
	}

	clean := domain.UpdateInput{
		ParamKey:      paramKey,
		Name:          strings.TrimSpace(input.Name),
		Description:   strings.TrimSpace(input.Description),
		ParamType:     input.ParamType,
		Required:      input.Required,
		GitOpsLocator: input.GitOpsLocator,
		CDSelfFill:    input.CDSelfFill,
		// Builtin fields are not editable; manual fields remain non-builtin.
		Builtin: false,
		Status:  input.Status,
	}
	return uc.repo.Update(ctx, id, clean, uc.now())
}

// Delete 删除业务资源并返回处理结果。
func (uc *PlatformParamDictManager) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidID
	}

	current, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if current.Builtin {
		return ErrBuiltinProtected
	}

	referenced, err := uc.paramRepo.CountByParamKey(ctx, current.ParamKey)
	if err != nil {
		return err
	}
	if referenced > 0 {
		return ErrReferencedConflict
	}
	return uc.repo.Delete(ctx, id)
}

// normalizePlatformParamKey 标准化输入值，保证后续逻辑使用统一格式。
func normalizePlatformParamKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: param_key is required", ErrInvalidInput)
	}
	if value != strings.ToLower(value) {
		return "", ErrInvalidParamKey
	}
	if !platformParamKeyPattern.MatchString(value) {
		return "", ErrInvalidParamKey
	}
	return value, nil
}
