package application

import (
	"context"
	"time"
)

type Repository interface {
	Create(ctx context.Context, app Application) error
	GetByID(ctx context.Context, id string) (Application, error)
	List(ctx context.Context, filter ListFilter) ([]Application, int64, error)
	Update(ctx context.Context, id string, input UpdateInput, updatedAt time.Time) (Application, error)
	Delete(ctx context.Context, id string) error
	InitSchema(ctx context.Context) error
}

type ListFilter struct {
	Keyword        string
	Key            string
	Name           string
	ProjectID      string
	Status         Status
	ApplicationIDs []string
	Page           int
	PageSize       int
}

type UpdateInput struct {
	Name                 string
	Key                  string
	ProjectID            string
	RepoURL              string
	Description          string
	OwnerUserID          string
	Owner                string
	Status               Status
	ArtifactType         string
	ArtifactRepositoryID string
	ArtifactDirectory    string
	Language             string
	GitOpsBranchMappings []GitOpsBranchMapping
	ReleaseBranches      []ReleaseBranchOption
}
