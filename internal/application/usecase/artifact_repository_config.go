package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/artifactrepo"
)

type ArtifactRepositoryManager struct {
	repo domain.Repository
	now  func() time.Time
	// connectionTester is keyed by repository type so each backend can be
	// exercised with its own protocol.
	connectionTester map[domain.RepositoryType]ArtifactRepositoryConnectionTester
}

// ArtifactRepositoryInput is the single shape accepted by Create, Update and
// TestConnection. Sharing one type keeps the per-type field rules in exactly
// one normalizer instead of one per entry point.
type ArtifactRepositoryInput struct {
	Name               string
	RepositoryType     domain.RepositoryType
	Endpoint           string
	Port               int
	Bucket             string
	Directory          string
	AccessKeyID        string
	AccessKeySecret    string
	Username           string
	Password           string
	PrivateKey         string
	DisableEPSV        bool
	HostKeyFingerprint string
	ACL                domain.ACL
	Status             domain.Status
}

func NewArtifactRepositoryManager(repo domain.Repository) *ArtifactRepositoryManager {
	return &ArtifactRepositoryManager{
		repo: repo,
		now: func() time.Time {
			return time.Now().UTC()
		},
		connectionTester: map[domain.RepositoryType]ArtifactRepositoryConnectionTester{
			domain.RepositoryTypeOSS:  newOSSArtifactRepositoryConnectionTester(nil, nil),
			domain.RepositoryTypeFTP:  newFTPArtifactRepositoryConnectionTester(nil),
			domain.RepositoryTypeSFTP: newSFTPArtifactRepositoryConnectionTester(nil),
		},
	}
}

func (uc *ArtifactRepositoryManager) Create(ctx context.Context, input ArtifactRepositoryInput) (domain.ArtifactRepository, error) {
	clean, err := normalizeArtifactRepositoryInput(input, artifactRepositoryNormalizeOptions{
		requireName:       true,
		requireCredential: true,
	})
	if err != nil {
		return domain.ArtifactRepository{}, err
	}

	now := uc.now()
	item := domain.ArtifactRepository{
		ID:                 generateID("arc"),
		Name:               clean.Name,
		RepositoryType:     clean.RepositoryType,
		Endpoint:           clean.Endpoint,
		Port:               clean.Port,
		Bucket:             clean.Bucket,
		Directory:          clean.Directory,
		AccessKeyID:        clean.AccessKeyID,
		AccessKeySecret:    clean.AccessKeySecret,
		Username:           clean.Username,
		Password:           clean.Password,
		PrivateKey:         clean.PrivateKey,
		DisableEPSV:        clean.DisableEPSV,
		HostKeyFingerprint: clean.HostKeyFingerprint,
		ACL:                clean.ACL,
		Status:             clean.Status,
		CreatedAt:          now,
		UpdatedAt:          now,
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

func (uc *ArtifactRepositoryManager) Update(ctx context.Context, id string, input ArtifactRepositoryInput) (domain.ArtifactRepository, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.ArtifactRepository{}, ErrInvalidID
	}

	current, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return domain.ArtifactRepository{}, err
	}

	clean, err := normalizeArtifactRepositoryInput(input, artifactRepositoryNormalizeOptions{
		requireName: true,
		current: artifactRepositoryCredential{
			accessKeySecret: current.AccessKeySecret,
			password:        current.Password,
			privateKey:      current.PrivateKey,
		},
	})
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

func (uc *ArtifactRepositoryManager) TestConnection(ctx context.Context, input ArtifactRepositoryInput) (ArtifactRepositoryConnectionTestResult, error) {
	clean, err := normalizeArtifactRepositoryInput(input, artifactRepositoryNormalizeOptions{requireCredential: true})
	if err != nil {
		return ArtifactRepositoryConnectionTestResult{}, err
	}
	tester := uc.connectionTester[clean.RepositoryType]
	if tester == nil {
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: repository_type does not support connection testing", ErrInvalidInput)
	}
	return tester.TestConnection(ctx, clean)
}

// artifactRepositoryCredential carries the stored secrets so an update can keep
// the ones the request left blank.
type artifactRepositoryCredential struct {
	accessKeySecret string
	password        string
	privateKey      string
}

type artifactRepositoryNormalizeOptions struct {
	// requireName rejects a blank name. Connection testing posts only the
	// connection fields, so it leaves this off.
	requireName bool
	// requireCredential rejects a blank credential instead of falling back to
	// the stored one. Creating and testing require it; updating does not.
	requireCredential bool
	// current supplies the stored secrets for the update path.
	current artifactRepositoryCredential
}

// normalizeArtifactRepositoryInput validates and defaults the request for every
// entry point. Each repository type only keeps the fields it actually uses, so
// changing a repository's type cannot leave the previous type's credentials
// behind.
func normalizeArtifactRepositoryInput(input ArtifactRepositoryInput, options artifactRepositoryNormalizeOptions) (domain.UpdateInput, error) {
	name := strings.TrimSpace(input.Name)
	if options.requireName && name == "" {
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
	if input.Port < 0 || input.Port > 65535 {
		return domain.UpdateInput{}, fmt.Errorf("%w: port is invalid", ErrInvalidInput)
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

	clean := domain.UpdateInput{
		Name:           name,
		RepositoryType: repositoryType,
		Endpoint:       endpoint,
		Port:           input.Port,
		Directory:      normalizeArtifactRepositoryDirectory(input.Directory),
		ACL:            acl,
		Status:         status,
	}

	switch repositoryType {
	case domain.RepositoryTypeFTP:
		username := strings.TrimSpace(input.Username)
		if username == "" {
			return domain.UpdateInput{}, fmt.Errorf("%w: username is required", ErrInvalidInput)
		}
		password := resolveArtifactRepositorySecret(input.Password, options.current.password)
		if options.requireCredential && password == "" {
			return domain.UpdateInput{}, fmt.Errorf("%w: password is required", ErrInvalidInput)
		}
		clean.Username = username
		clean.Password = password
		clean.DisableEPSV = input.DisableEPSV

	case domain.RepositoryTypeSFTP:
		username := strings.TrimSpace(input.Username)
		if username == "" {
			return domain.UpdateInput{}, fmt.Errorf("%w: username is required", ErrInvalidInput)
		}
		password := resolveArtifactRepositorySecret(input.Password, options.current.password)
		privateKey := resolveArtifactRepositorySecret(input.PrivateKey, options.current.privateKey)
		if options.requireCredential && password == "" && privateKey == "" {
			return domain.UpdateInput{}, fmt.Errorf("%w: password or private_key is required", ErrInvalidInput)
		}
		clean.Username = username
		clean.Password = password
		clean.PrivateKey = privateKey
		clean.HostKeyFingerprint = strings.TrimSpace(input.HostKeyFingerprint)

	default:
		bucket := strings.TrimSpace(input.Bucket)
		if bucket == "" {
			return domain.UpdateInput{}, fmt.Errorf("%w: bucket is required", ErrInvalidInput)
		}
		accessKeyID := strings.TrimSpace(input.AccessKeyID)
		if accessKeyID == "" {
			return domain.UpdateInput{}, fmt.Errorf("%w: access_key_id is required", ErrInvalidInput)
		}
		accessKeySecret := resolveArtifactRepositorySecret(input.AccessKeySecret, options.current.accessKeySecret)
		if options.requireCredential && accessKeySecret == "" {
			return domain.UpdateInput{}, fmt.Errorf("%w: access_key_secret is required", ErrInvalidInput)
		}
		clean.Bucket = bucket
		clean.AccessKeyID = accessKeyID
		clean.AccessKeySecret = accessKeySecret
	}

	return clean, nil
}

// resolveArtifactRepositorySecret keeps the stored credential when the request
// omits it, which is how the UI expresses "leave the secret unchanged".
func resolveArtifactRepositorySecret(value, current string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return strings.TrimSpace(current)
	}
	return trimmed
}

func normalizeArtifactRepositoryDirectory(value string) string {
	raw := strings.TrimSpace(value)
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return "/"
	}
	return raw
}
