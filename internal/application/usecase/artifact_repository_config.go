package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/artifactrepo"
)

type ArtifactRepositoryManager struct {
	repo             domain.Repository
	now              func() time.Time
	connectionTester ArtifactRepositoryConnectionTester
}

type CreateArtifactRepositoryInput struct {
	Name            string
	RepositoryType  domain.RepositoryType
	Endpoint        string
	Bucket          string
	Directory       string
	AccessKeyID     string
	AccessKeySecret string
	ACL             domain.ACL
	Status          domain.Status
}

func NewArtifactRepositoryManager(repo domain.Repository) *ArtifactRepositoryManager {
	return &ArtifactRepositoryManager{
		repo: repo,
		now: func() time.Time {
			return time.Now().UTC()
		},
		connectionTester: newOSSArtifactRepositoryConnectionTester(nil, nil),
	}
}

func (uc *ArtifactRepositoryManager) Create(ctx context.Context, input CreateArtifactRepositoryInput) (domain.ArtifactRepository, error) {
	clean, err := normalizeArtifactRepositoryInput(domain.UpdateInput{
		Name:            input.Name,
		RepositoryType:  input.RepositoryType,
		Endpoint:        input.Endpoint,
		Bucket:          input.Bucket,
		Directory:       input.Directory,
		AccessKeyID:     input.AccessKeyID,
		AccessKeySecret: input.AccessKeySecret,
		ACL:             input.ACL,
		Status:          input.Status,
	}, true, "")
	if err != nil {
		return domain.ArtifactRepository{}, err
	}

	now := uc.now()
	item := domain.ArtifactRepository{
		ID:              generateID("arc"),
		Name:            clean.Name,
		RepositoryType:  clean.RepositoryType,
		Endpoint:        clean.Endpoint,
		Bucket:          clean.Bucket,
		Directory:       clean.Directory,
		AccessKeyID:     clean.AccessKeyID,
		AccessKeySecret: clean.AccessKeySecret,
		ACL:             clean.ACL,
		Status:          clean.Status,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := uc.repo.Create(ctx, item); err != nil {
		return domain.ArtifactRepository{}, err
	}
	return uc.repo.GetByID(ctx, item.ID)
}

func (uc *ArtifactRepositoryManager) List(ctx context.Context, filter domain.ListFilter) ([]domain.ArtifactRepository, int64, error) {
	const (
		defaultPage     = 1
		defaultPageSize = 20
		maxPageSize     = 100
	)
	filter.Keyword = strings.TrimSpace(filter.Keyword)
	if filter.RepositoryType != "" && !filter.RepositoryType.Valid() {
		return nil, 0, fmt.Errorf("%w: repository_type is invalid", ErrInvalidInput)
	}
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

func (uc *ArtifactRepositoryManager) GetByID(ctx context.Context, id string) (domain.ArtifactRepository, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.ArtifactRepository{}, ErrInvalidID
	}
	return uc.repo.GetByID(ctx, id)
}

func (uc *ArtifactRepositoryManager) Update(ctx context.Context, id string, input domain.UpdateInput) (domain.ArtifactRepository, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.ArtifactRepository{}, ErrInvalidID
	}

	current, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return domain.ArtifactRepository{}, err
	}

	clean, err := normalizeArtifactRepositoryInput(input, false, current.AccessKeySecret)
	if err != nil {
		return domain.ArtifactRepository{}, err
	}
	return uc.repo.Update(ctx, id, clean, uc.now())
}

func (uc *ArtifactRepositoryManager) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidID
	}
	return uc.repo.Delete(ctx, id)
}

func (uc *ArtifactRepositoryManager) TestConnection(ctx context.Context, input CreateArtifactRepositoryInput) (ArtifactRepositoryConnectionTestResult, error) {
	clean, err := normalizeArtifactRepositoryConnectionInput(input)
	if err != nil {
		return ArtifactRepositoryConnectionTestResult{}, err
	}
	tester := uc.connectionTester
	if tester == nil {
		tester = newOSSArtifactRepositoryConnectionTester(nil, nil)
	}
	return tester.TestConnection(ctx, clean)
}

func normalizeArtifactRepositoryInput(input domain.UpdateInput, requireSecret bool, currentSecret string) (domain.UpdateInput, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.UpdateInput{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}

	repositoryType := input.RepositoryType
	if repositoryType == "" {
		repositoryType = domain.RepositoryTypeOSS
	}
	if !repositoryType.Valid() {
		return domain.UpdateInput{}, fmt.Errorf("%w: repository_type is invalid", ErrInvalidInput)
	}

	endpoint := strings.TrimSpace(input.Endpoint)
	if endpoint == "" {
		return domain.UpdateInput{}, fmt.Errorf("%w: endpoint is required", ErrInvalidInput)
	}
	bucket := strings.TrimSpace(input.Bucket)
	if bucket == "" {
		return domain.UpdateInput{}, fmt.Errorf("%w: bucket is required", ErrInvalidInput)
	}
	accessKeyID := strings.TrimSpace(input.AccessKeyID)
	if accessKeyID == "" {
		return domain.UpdateInput{}, fmt.Errorf("%w: access_key_id is required", ErrInvalidInput)
	}
	accessKeySecret := strings.TrimSpace(input.AccessKeySecret)
	if accessKeySecret == "" {
		accessKeySecret = strings.TrimSpace(currentSecret)
	}
	if requireSecret && accessKeySecret == "" {
		return domain.UpdateInput{}, fmt.Errorf("%w: access_key_secret is required", ErrInvalidInput)
	}

	acl := input.ACL
	if acl == "" {
		acl = domain.ACLPrivate
	}
	if !acl.Valid() {
		return domain.UpdateInput{}, fmt.Errorf("%w: acl is invalid", ErrInvalidInput)
	}

	status := input.Status
	if status == "" {
		status = domain.StatusEnabled
	}
	if !status.Valid() {
		return domain.UpdateInput{}, ErrInvalidStatus
	}

	return domain.UpdateInput{
		Name:            name,
		RepositoryType:  repositoryType,
		Endpoint:        endpoint,
		Bucket:          bucket,
		Directory:       normalizeArtifactRepositoryDirectory(input.Directory),
		AccessKeyID:     accessKeyID,
		AccessKeySecret: accessKeySecret,
		ACL:             acl,
		Status:          status,
	}, nil
}

func normalizeArtifactRepositoryDirectory(value string) string {
	raw := strings.TrimSpace(value)
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return "/"
	}
	return raw
}

func normalizeArtifactRepositoryConnectionInput(input CreateArtifactRepositoryInput) (domain.UpdateInput, error) {
	repositoryType := input.RepositoryType
	if repositoryType == "" {
		repositoryType = domain.RepositoryTypeOSS
	}
	if !repositoryType.Valid() {
		return domain.UpdateInput{}, fmt.Errorf("%w: repository_type is invalid", ErrInvalidInput)
	}

	endpoint := strings.TrimSpace(input.Endpoint)
	if endpoint == "" {
		return domain.UpdateInput{}, fmt.Errorf("%w: endpoint is required", ErrInvalidInput)
	}
	bucket := strings.TrimSpace(input.Bucket)
	if bucket == "" {
		return domain.UpdateInput{}, fmt.Errorf("%w: bucket is required", ErrInvalidInput)
	}
	accessKeyID := strings.TrimSpace(input.AccessKeyID)
	if accessKeyID == "" {
		return domain.UpdateInput{}, fmt.Errorf("%w: access_key_id is required", ErrInvalidInput)
	}
	accessKeySecret := strings.TrimSpace(input.AccessKeySecret)
	if accessKeySecret == "" {
		return domain.UpdateInput{}, fmt.Errorf("%w: access_key_secret is required", ErrInvalidInput)
	}

	acl := input.ACL
	if acl == "" {
		acl = domain.ACLPrivate
	}
	if !acl.Valid() {
		return domain.UpdateInput{}, fmt.Errorf("%w: acl is invalid", ErrInvalidInput)
	}

	status := input.Status
	if status == "" {
		status = domain.StatusEnabled
	}
	if !status.Valid() {
		return domain.UpdateInput{}, ErrInvalidStatus
	}

	return domain.UpdateInput{
		Name:            strings.TrimSpace(input.Name),
		RepositoryType:  repositoryType,
		Endpoint:        endpoint,
		Bucket:          bucket,
		Directory:       normalizeArtifactRepositoryDirectory(input.Directory),
		AccessKeyID:     accessKeyID,
		AccessKeySecret: accessKeySecret,
		ACL:             acl,
		Status:          status,
	}, nil
}
