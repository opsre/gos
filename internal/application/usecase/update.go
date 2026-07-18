package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/application"
	projectdomain "gos/internal/domain/project"
)

type UpdateApplication struct {
	repo        domain.Repository
	projectRepo projectdomain.Repository
	now         func() time.Time
}

// NewUpdateApplication 创建并返回对应组件实例。
func NewUpdateApplication(repo domain.Repository, projectRepo projectdomain.Repository) *UpdateApplication {
	return &UpdateApplication{
		repo:        repo,
		projectRepo: projectRepo,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// Execute 封装当前模块的业务处理逻辑。
func (uc *UpdateApplication) Execute(ctx context.Context, id string, input domain.UpdateInput) (domain.Application, error) {
	if uc.repo == nil || uc.projectRepo == nil {
		return domain.Application{}, fmt.Errorf("%w: application repository is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Application{}, ErrInvalidID
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Key) == "" {
		return domain.Application{}, fmt.Errorf("%w: name and key are required", ErrInvalidInput)
	}
	current, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Application{}, err
	}
	if strings.TrimSpace(input.Key) != strings.TrimSpace(current.Key) {
		return domain.Application{}, fmt.Errorf("%w: application key cannot be changed", ErrReferencedConflict)
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
	if !input.Status.Valid() {
		return domain.Application{}, ErrInvalidStatus
	}
	artifactRepositoryID, artifactDirectory, err := normalizeApplicationArtifactBinding(input.ArtifactRepositoryID, input.ArtifactDirectory)
	if err != nil {
		return domain.Application{}, err
	}
	project, err := uc.projectRepo.GetByID(ctx, strings.TrimSpace(input.ProjectID))
	if err != nil {
		return domain.Application{}, err
	}

	clean := domain.UpdateInput{
		Name:                 strings.TrimSpace(input.Name),
		Key:                  strings.TrimSpace(current.Key),
		ProjectID:            project.ID,
		RepoURL:              strings.TrimSpace(input.RepoURL),
		Description:          strings.TrimSpace(input.Description),
		OwnerUserID:          strings.TrimSpace(input.OwnerUserID),
		Owner:                strings.TrimSpace(input.Owner),
		Status:               input.Status,
		ArtifactType:         strings.TrimSpace(input.ArtifactType),
		ArtifactRepositoryID: artifactRepositoryID,
		ArtifactDirectory:    artifactDirectory,
		Language:             strings.TrimSpace(input.Language),
		GitOpsBranchMappings: normalizeGitOpsBranchMappings(input.GitOpsBranchMappings),
		ReleaseBranches:      normalizeReleaseBranchOptions(input.ReleaseBranches),
	}
	return uc.repo.Update(ctx, id, clean, uc.now())
}

func normalizeApplicationArtifactBinding(repositoryID string, directory string) (string, string, error) {
	repositoryID = strings.TrimSpace(repositoryID)
	rawDirectory := strings.TrimSpace(directory)
	if repositoryID == "" {
		if rawDirectory != "" {
			return "", "", fmt.Errorf("%w: artifact_repository_id is required when artifact_directory is set", ErrInvalidInput)
		}
		return "", "", nil
	}
	if rawDirectory == "" {
		return "", "", fmt.Errorf("%w: artifact_directory is required when artifact_repository_id is set", ErrInvalidInput)
	}
	return repositoryID, normalizeApplicationArtifactDirectory(rawDirectory), nil
}

func normalizeApplicationArtifactDirectory(value string) string {
	raw := strings.TrimSpace(value)
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return "/"
	}
	return raw
}

// normalizeGitOpsBranchMappings 标准化输入值，保证后续逻辑使用统一格式。
func normalizeGitOpsBranchMappings(values []domain.GitOpsBranchMapping) []domain.GitOpsBranchMapping {
	if len(values) == 0 {
		return nil
	}
	result := make([]domain.GitOpsBranchMapping, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		envCode := strings.TrimSpace(item.EnvCode)
		branch := strings.TrimSpace(item.Branch)
		if envCode == "" || branch == "" {
			continue
		}
		key := strings.ToLower(envCode)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, domain.GitOpsBranchMapping{
			EnvCode: envCode,
			Branch:  branch,
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// normalizeReleaseBranchOptions 标准化输入值，保证后续逻辑使用统一格式。
func normalizeReleaseBranchOptions(values []domain.ReleaseBranchOption) []domain.ReleaseBranchOption {
	if len(values) == 0 {
		return nil
	}
	result := make([]domain.ReleaseBranchOption, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		name := strings.TrimSpace(item.Name)
		branch := strings.TrimSpace(item.Branch)
		if branch == "" {
			continue
		}
		key := strings.ToLower(branch)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if name == "" {
			name = branch
		}
		result = append(result, domain.ReleaseBranchOption{
			Name:   name,
			Branch: branch,
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
