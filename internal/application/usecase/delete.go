package usecase

import (
	"context"
	"strings"

	domain "gos/internal/domain/application"
)

type DeleteApplication struct {
	repo domain.Repository
}

// NewDeleteApplication 创建并返回对应组件实例。
func NewDeleteApplication(repo domain.Repository) *DeleteApplication {
	return &DeleteApplication{repo: repo}
}

// Execute 封装当前模块的业务处理逻辑。
func (uc *DeleteApplication) Execute(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrInvalidID
	}
	return uc.repo.Delete(ctx, id)
}
