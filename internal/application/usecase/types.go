package usecase

import domain "gos/internal/domain/application"

type CreateInput struct {
	Name                 string
	Key                  string
	ProjectID            string
	RepoURL              string
	Description          string
	OwnerUserID          string
	Owner                string
	Status               domain.Status
	ArtifactType         string
	ArtifactRepositoryID string
	ArtifactDirectory    string
	Language             string
	GitOpsBranchMappings []domain.GitOpsBranchMapping
	ReleaseBranches      []domain.ReleaseBranchOption
}
