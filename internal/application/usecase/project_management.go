package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	projectdomain "gos/internal/domain/project"
)

type ProjectManager struct {
	repo projectdomain.Repository
	now  func() time.Time
}

// NewProjectManager 创建并返回对应组件实例。
func NewProjectManager(repo projectdomain.Repository) *ProjectManager {
	return &ProjectManager{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

type CreateProjectInput struct {
	Name        string
	Key         string
	Description string
	Status      projectdomain.Status
}

// Create 创建业务资源并返回处理结果。
func (uc *ProjectManager) Create(ctx context.Context, input CreateProjectInput) (projectdomain.Project, error) {
	if uc == nil || uc.repo == nil {
		return projectdomain.Project{}, fmt.Errorf("%w: project repository is not configured", ErrInvalidInput)
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Key) == "" {
		return projectdomain.Project{}, fmt.Errorf("%w: name and key are required", ErrInvalidInput)
	}
	status := input.Status
	if status == "" {
		status = projectdomain.StatusActive
	}
	if !status.Valid() {
		return projectdomain.Project{}, ErrInvalidStatus
	}
	now := uc.now()
	item := projectdomain.Project{
		ID:          generateID("prj"),
		Name:        strings.TrimSpace(input.Name),
		Key:         strings.TrimSpace(input.Key),
		Description: strings.TrimSpace(input.Description),
		Status:      status,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := uc.repo.Create(ctx, item); err != nil {
		return projectdomain.Project{}, err
	}
	return item, nil
}

// GetByID 查询并返回指定资源数据。
func (uc *ProjectManager) GetByID(ctx context.Context, id string) (projectdomain.Project, error) {
	if uc == nil || uc.repo == nil {
		return projectdomain.Project{}, fmt.Errorf("%w: project repository is not configured", ErrInvalidInput)
	}
	if strings.TrimSpace(id) == "" {
		return projectdomain.Project{}, ErrInvalidID
	}
	return uc.repo.GetByID(ctx, id)
}

// List 查询并返回列表数据。
func (uc *ProjectManager) List(ctx context.Context, filter projectdomain.ListFilter) ([]projectdomain.Project, int64, error) {
	if uc == nil || uc.repo == nil {
		return nil, 0, fmt.Errorf("%w: project repository is not configured", ErrInvalidInput)
	}
	filter.Key = strings.TrimSpace(filter.Key)
	filter.Name = strings.TrimSpace(filter.Name)
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, 0, ErrInvalidStatus
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}
	return uc.repo.List(ctx, filter)
}

// Update 更新业务资源并返回处理结果。
func (uc *ProjectManager) Update(ctx context.Context, id string, input projectdomain.UpdateInput) (projectdomain.Project, error) {
	if uc == nil || uc.repo == nil {
		return projectdomain.Project{}, fmt.Errorf("%w: project repository is not configured", ErrInvalidInput)
	}
	if strings.TrimSpace(id) == "" {
		return projectdomain.Project{}, ErrInvalidID
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Key) == "" {
		return projectdomain.Project{}, fmt.Errorf("%w: name and key are required", ErrInvalidInput)
	}
	if !input.Status.Valid() {
		return projectdomain.Project{}, ErrInvalidStatus
	}
	clean := projectdomain.UpdateInput{
		Name:        strings.TrimSpace(input.Name),
		Key:         strings.TrimSpace(input.Key),
		Description: strings.TrimSpace(input.Description),
		Status:      input.Status,
	}
	return uc.repo.Update(ctx, id, clean, uc.now())
}

// Delete 删除业务资源并返回处理结果。
func (uc *ProjectManager) Delete(ctx context.Context, id string) error {
	if uc == nil || uc.repo == nil {
		return fmt.Errorf("%w: project repository is not configured", ErrInvalidInput)
	}
	if strings.TrimSpace(id) == "" {
		return ErrInvalidID
	}
	return uc.repo.Delete(ctx, id)
}
