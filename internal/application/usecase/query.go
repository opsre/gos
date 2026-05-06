package usecase

import (
	"context"
	"strings"

	domain "gos/internal/domain/application"
)

type QueryApplication struct {
	repo domain.Repository
}

// NewQueryApplication 创建并返回对应组件实例。
func NewQueryApplication(repo domain.Repository) *QueryApplication {
	return &QueryApplication{repo: repo}
}

// GetByID 查询并返回指定资源数据。
func (uc *QueryApplication) GetByID(ctx context.Context, id string) (domain.Application, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Application{}, ErrInvalidID
	}
	return uc.repo.GetByID(ctx, id)
}

// List 查询并返回列表数据。
func (uc *QueryApplication) List(ctx context.Context, filter domain.ListFilter) ([]domain.Application, int64, error) {
	const (
		defaultPage     = 1
		defaultPageSize = 20
		maxPageSize     = 100
	)

	filter.Key = strings.TrimSpace(filter.Key)
	filter.Name = strings.TrimSpace(filter.Name)
	filter.Keyword = strings.TrimSpace(filter.Keyword)
	filter.ProjectID = strings.TrimSpace(filter.ProjectID)
	filter.ApplicationIDs = normalizeApplicationIDs(filter.ApplicationIDs)
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
	return uc.repo.List(ctx, filter)
}

// normalizeApplicationIDs 标准化输入值，保证后续逻辑使用统一格式。
func normalizeApplicationIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
