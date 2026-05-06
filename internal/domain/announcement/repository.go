package announcement

import (
	"context"
)

type ListFilter struct {
	Keyword string
	Active  *bool // nil=全部, true=当前有效, false=已过期
	Page    int
	PageSize int
}

type Repository interface {
	InitSchema(ctx context.Context) error
	Create(ctx context.Context, item Announcement) (Announcement, error)
	Update(ctx context.Context, item Announcement) (Announcement, error)
	GetByID(ctx context.Context, id string) (Announcement, error)
	List(ctx context.Context, filter ListFilter) ([]Announcement, int64, error)
	Delete(ctx context.Context, id string) error
}
