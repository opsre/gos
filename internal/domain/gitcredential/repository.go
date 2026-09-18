package gitcredential

import (
	"context"
	"time"
)

type Repository interface {
	InitSchema(ctx context.Context) error
	Create(ctx context.Context, item Credential) error
	GetByID(ctx context.Context, id string) (Credential, error)
	List(ctx context.Context, filter ListFilter) ([]Credential, int64, error)
	Update(ctx context.Context, id string, input UpdateInput, updatedAt time.Time) (Credential, error)
	Delete(ctx context.Context, id string) error
}
