package artifactrepo

import (
	"context"
	"time"
)

type Repository interface {
	InitSchema(ctx context.Context) error
	Create(ctx context.Context, item ArtifactRepository) error
	GetByID(ctx context.Context, id string) (ArtifactRepository, error)
	List(ctx context.Context, filter ListFilter) ([]ArtifactRepository, int64, error)
	Update(ctx context.Context, id string, input UpdateInput, updatedAt time.Time) (ArtifactRepository, error)
	Delete(ctx context.Context, id string) error
}
