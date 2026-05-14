package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/application"
	projectdomain "gos/internal/domain/project"
)

type CreateApplication struct {
	repo        domain.Repository
	projectRepo projectdomain.Repository
	now         func() time.Time
}

// NewCreateApplication 创建并返回对应组件实例。
func NewCreateApplication(repo domain.Repository, projectRepo projectdomain.Repository) *CreateApplication {
	return &CreateApplication{
		repo:        repo,
		projectRepo: projectRepo,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// Execute 封装当前模块的业务处理逻辑。
func (uc *CreateApplication) Execute(ctx context.Context, input CreateInput) (domain.Application, error) {
	if uc.repo == nil || uc.projectRepo == nil {
		return domain.Application{}, fmt.Errorf("%w: application repository is not configured", ErrInvalidInput)
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Key) == "" {
		return domain.Application{}, fmt.Errorf("%w: name and key are required", ErrInvalidInput)
	}
	if strings.TrimSpace(input.ProjectID) == "" {
		return domain.Application{}, fmt.Errorf("%w: project_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(input.ArtifactType) == "" || strings.TrimSpace(input.Language) == "" {
		return domain.Application{}, fmt.Errorf("%w: artifact_type and language are required", ErrInvalidInput)
	}
	if strings.TrimSpace(input.OwnerUserID) == "" {
		return domain.Application{}, fmt.Errorf("%w: owner_user_id is required", ErrInvalidInput)
	}
	if input.Status != "" && !input.Status.Valid() {
		return domain.Application{}, ErrInvalidStatus
	}
	artifactRepositoryID, artifactDirectory, err := normalizeApplicationArtifactBinding(input.ArtifactRepositoryID, input.ArtifactDirectory)
	if err != nil {
		return domain.Application{}, err
	}

	status := input.Status
	if status == "" {
		status = domain.StatusActive
	}
	project, err := uc.projectRepo.GetByID(ctx, strings.TrimSpace(input.ProjectID))
	if err != nil {
		return domain.Application{}, err
	}

	now := uc.now()
	app := domain.Application{
		ID:                   generateID("app"),
		Name:                 strings.TrimSpace(input.Name),
		Key:                  strings.TrimSpace(input.Key),
		ProjectID:            project.ID,
		ProjectName:          project.Name,
		ProjectKey:           project.Key,
		RepoURL:              strings.TrimSpace(input.RepoURL),
		Description:          strings.TrimSpace(input.Description),
		OwnerUserID:          strings.TrimSpace(input.OwnerUserID),
		Owner:                strings.TrimSpace(input.Owner),
		Status:               status,
		ArtifactType:         strings.TrimSpace(input.ArtifactType),
		ArtifactRepositoryID: artifactRepositoryID,
		ArtifactDirectory:    artifactDirectory,
		GitOpsBranchMappings: normalizeGitOpsBranchMappings(input.GitOpsBranchMappings),
		ReleaseBranches:      normalizeReleaseBranchOptions(input.ReleaseBranches),
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	app.SetLanguage(strings.TrimSpace(input.Language))

	if err := uc.repo.Create(ctx, app); err != nil {
		return domain.Application{}, err
	}
	return app, nil
}
